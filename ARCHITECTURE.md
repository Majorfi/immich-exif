# Architecture

## Upload strategy

The tool uses a verify-before-delete process:

1. **GET /assets/{id}** — Re-fetch the asset right before upload; if `updatedAt` changed since the initial scan (metadata edited server-side mid-run), the asset is skipped so a re-run picks up the fresh state
2. **POST /assets** — Upload the modified file as a new asset (forwarding `livePhotoVideoId` so live-photo pairs survive)
3. **GET /assets/{id}** — By default, re-fetch the new asset and verify its stored checksum matches the local file. A mismatch aborts before any delete, leaving the original intact. Skipped with `-no-verify-upload`.
4. **PUT /assets/copy** — Copy all associations (albums, favorites, shared links, sidecars, stacks) from old to new (no PATCH alias exists for this endpoint on v3.0.1)
5. **PATCH /assets** (PUT on legacy servers) — Restore visibility if the original was archived or had non-default visibility
6. **DELETE /assets** — Move the original to Immich's trash (`force=false`, recoverable). The delete is never permanent: checksum verification proves the transfer, not exiftool's output, so the trash window is kept as the recovery path.

Immich v3 deprecated PUT on the bulk asset update endpoint (removed in v4); the client sends PATCH there on v3+ and PUT on legacy servers, selected by `writeMethod()`. `/assets/copy` stays PUT everywhere: v3.0.1 has no PATCH alias for it — a PATCH is routed into `PATCH /assets/:id` and fails UUID validation (found by a live run).

Upload is sent as a streamed multipart request (chunked), so large files are not buffered fully in memory.

The download itself is checksum-verified (Content-Length + SHA-1 against the asset's stored checksum) so a corrupt or truncated transfer never reaches the upload/delete steps. An asset the server returns no checksum for is refused outright.

Hidden videos are skipped entirely: they are almost always the motion half of a live photo, and replacing one would give it a new ID and permanently sever the still photo's `livePhotoVideoId` link.

With `-faces`, the asset's face boxes are fetched from `GET /api/faces` (the v3 asset response no longer inlines them) and written as MWG regions (`XMP-mwg-rs:RegionInfo`). Immich's boxes are pixels on the orientation-corrected preview the ML pipeline analyzed; the file stores the unrotated image, so the coordinates are normalized and mapped back through the exact inverse of the orientation transform Immich's own metadata importer applies (`orientRegionInfo` in metadata.service.ts) — a written region re-imports to the same displayed box when the server's "import faces from metadata" setting is on. Only named, visible people are written (the importer ignores nameless regions anyway). A mismatching `RegionInfo` is replaced wholesale; when Immich has no named faces, the file's regions are left alone since "cleared" and "never recognized" are indistinguishable.

Two mechanisms keep the round trip stable. First, faces with `sourceType: exif` are echoes of the file's own regions (the server's metadata import); they are dropped whenever a detected face carries the same normalized name, because counting both would grow the region set by one on every replace. The match is by name rather than person ID since names are how the importer links regions back to people — with two person records sharing a name, the echo can land on the record recognition did not pick. Echoes still count when they are a name's only face (server without ML). Second, the coordinate comparison widens its per-axis tolerance to two raster pixels: Immich's importer floors region corners to whole pixels, which on small images shifts a round-tripped coordinate by more than the base tolerance. Face edits also get their own pre-upload freshness check (mirroring the `updatedAt` one, which face operations do not bump): the boxes are re-fetched right before upload and the asset is skipped if they moved.

Videos embed the same MWG regions, through the same `BuildFaceRegions` path — Immich's metadata importer reads `RegionInfo` from a video container just as it does from an image (verified against a live server), so a re-uploaded video's people survive exactly like a photo's. The one difference is the orientation source: a video has no EXIF `Orientation`, so `regionOrientation` reads the QuickTime `Rotation` tag and maps it to the equivalent orientation value (`videoRotationToOrientation`: 0°→1, 90°→6, 270°→8) that feeds the same `rasterRegion` inverse. A 180° or non-cardinal rotation returns not-anchorable and the video's regions are skipped: Immich was observed to re-orient 90°/270° video regions on import but not 180°, so writing a 180° region would misplace the box. Only `mp4`/`mov`/`m4v` are eligible (`SupportsVideoMetadataEmbedding`); other containers exiftool cannot write are skipped.

External-library assets (`libraryId` set; null means internal) are skipped on replace runs: API uploads always land in the internal library, so a replacement would migrate the asset and duplicate it at the next library scan. Read-only modes (dry-run, export) still process them. The `libraryId` semantics hold on every server the client can address: the plural `/api/assets` routes and nullable `libraryId` both arrived in Immich 1.106. The practical floors are higher anyway — `/api/assets/copy` (used by every replace that yields a new ID) first appears around v2.2, and `/api/server/about` (auto-detection) in 1.113 — so a set `libraryId` reliably means external wherever the tool can operate.

If the upload returns the same asset ID, copy/delete is skipped.

If upload returns `duplicate`/`replaced`:

- default behavior: copy/delete is skipped and the result is marked as skipped (not cached)
- with `-resolve-duplicate`: for `duplicate` with different ID, the tool copies associations to that duplicate asset and trashes the old asset — unless the duplicate itself sits in the trash, in which case it refuses and asks you to restore it first (deleting the original against a trashed duplicate would leave the only surviving copy pending auto-purge)

When duplicates are skipped (default mode), a final summary lists them and prints a command you can rerun with `-resolve-duplicate`. If running in an interactive terminal, the tool also prompts to patch them immediately without re-running.

If the copy or visibility step fails, the old asset is **not** deleted to avoid data loss. If the delete step fails, the new asset is already live and a warning is emitted. A failure that occurs after the new asset has been created is **not** retried, so the upload is never replayed into a duplicate; only a transient upload failure (before any new asset exists) is retried.

## Processing pipeline

```
main.go
  |
  +-- parseConfig()                CLI flags, env vars, .env file
  +-- state.OpenStateDB()          SQLite state cache (--all / -album all)
  +-- resolveAssetIDs()            --all / --album / positional IDs
  |                                (shouldSkip callback filters cached assets)
  |
  +-- runPipeline()                Console mode + ui.LogEmitter
  |    |
  |    +-- process.WorkerPool.Process(assetIDs)
  |         |
  |         +-- process.ProcessAsset()          per asset, in worker goroutine
  |              |
  |              +-- api.GetAsset()                 fetch metadata
  |              +-- exif.CompareAssetMetadata()     early skip if nothing writable
  |              +-- api.DownloadAsset()             download original file (checksum-verified)
  |              +-- exif.ReadExifTagsFn()            exiftool -json -n
  |              +-- exif.CompareAssetMetadata()     diff Immich vs file metadata
  |              +-- EmitDiff()                      show diff, wait for user
  |              +-- exif.WriteExifTagsFn()          exiftool -overwrite_original
  |              +-- uploader.Upload()               POST + verify + copy + visibility + delete
  |
  +-- state.SaveProcessedState()   persist results to state DB
```

## File structure

```
src/
  main.go             Entry point, mode selection, orchestration
  config.go           CLI parsing, env vars, validation
  utils.go            Shared helpers (dedup, tmp dir, duplicate resolution)

  model/
    types.go          Data structures (Config, AssetResponse, ExifInfo, etc.)
    events.go         Event types and EventEmitter interface
    assetType.go      Asset classification (video detection, live-photo motion)
    people.go         Person/face types, named-people helpers
    checksum.go       SHA-1 checksum decoding (base64/hex)
    helpers.go        ShortID, TruncateFilename, SanitizeForTerminal

  exif/
    tool.go           EXIF read and write (exiftool subprocess)
    compare.go        Metadata comparison, diff generation, exiftool arg building
    regions.go        MWG face regions: orientation mapping, comparison, serialization
    compareDateTime.go Date/offset comparison, time-zone anchoring
    match.go          Value matching helpers (float, string, int, datetime, zones)
    video.go          Video-specific metadata comparison and routing

  api/
    client.go         HTTP client base (transport, redirects, API version detection)
    assets.go         Asset CRUD (get, download, upload, copy, delete)
    faces.go          Face boxes for one asset
    search.go         Search, list albums, paginated asset listing

  state/
    db.go             SQLite state cache for incremental all-assets runs

  process/
    pipeline.go       Per-asset processing orchestration
    faces.go          Face-region change collection (-faces)
    worker.go         Worker pool with cancellation
    uploader.go       Upload interface and ModernUploader
    verify.go         Post-upload checksum verification

  ui/
    emitterLog.go     Console emitter with single-keypress input
    color.go          ANSI color helpers with terminal detection
```

## Immich API endpoints used

| Method      | Endpoint                    | Purpose                                                      |
| ----------- | --------------------------- | ------------------------------------------------------------ |
| GET         | `/api/server/about`         | Server version detection (best-effort in forced modes)       |
| GET         | `/api/assets/{id}`          | Fetch asset metadata and EXIF                                |
| GET         | `/api/assets/{id}/original` | Download original file                                       |
| GET         | `/api/faces`                | Face boxes for one asset (only with `-faces`)                |
| POST        | `/api/assets`               | Upload new asset (multipart)                                 |
| PATCH / PUT | `/api/assets`               | Update asset visibility (PATCH on v3+, PUT on legacy)        |
| PUT         | `/api/assets/copy`          | Copy associations between assets (no PATCH alias on v3)      |
| DELETE      | `/api/assets`               | Batch delete assets (always `force=false`, trash)            |
| POST        | `/api/search/metadata`      | Paginated asset listing + album enumeration (per visibility) |
| GET         | `/api/albums`               | List all albums                                              |
| GET         | `/api/albums/{id}`          | Get album with asset list                                    |

All requests are authenticated via `x-api-key` header. v3 is the primary API contract: version detection assumes v3 when the reported version is unrecognizable, and `-immich-api legacy` forces the older contract. Redirects that leave the configured host (or downgrade https to http) are refused so the key cannot leak.

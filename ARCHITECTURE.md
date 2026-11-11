# Architecture

## Upload strategy

The tool uses a verify-before-delete process:

1. **GET /assets/{id}** — Re-fetch the asset right before upload; if `updatedAt` changed since the initial scan (metadata edited server-side mid-run), the asset is skipped so a re-run picks up the fresh state
2. **GET /assets/{id}/edits** — For an asset edited in Immich's editor (crop, rotate, mirror), read the edits before uploading anything; skipped when they are baked into the file (`-apply-edits`)
3. **POST /assets** — Upload the modified file as a new asset (forwarding `livePhotoVideoId` so live-photo pairs survive)
4. **GET /assets/{id}** — By default, re-fetch the new asset and verify its stored checksum matches the local file. A mismatch aborts before any delete, leaving the original intact. Skipped with `-no-verify-upload`.
5. **PUT /assets/{id}/edits** — Re-apply the edits read in step 2 to the replacement, since `/assets/copy` does not carry them. Immich validates a crop against the asset's extracted dimensions, so the new asset is polled until metadata extraction has filled them in. The replacement must resolve the same orientation as the original (a sidecar that alone rotates the original would put the crop on other coordinates), and the applied edits are compared with the original's before going on.
6. **PUT /assets/copy** — Copy all associations (albums, favorites, shared links, sidecars, stacks) from old to new (no PATCH alias exists for this endpoint on v3.0.1)
7. **GET /assets/{id}** + **PATCH /stacks/{id}** (PUT on legacy servers) — Promote the replacement if it takes a stack primary's place
8. **PATCH /assets** (PUT on legacy servers) — Restore visibility if the original was archived or had non-default visibility
9. **DELETE /assets** — Move the original to Immich's trash (`force=false`, recoverable). The delete is never permanent: checksum verification proves the transfer, not exiftool's output, so the trash window is kept as the recovery path.

Step 7 prevents a trashed primary from hiding its stack (issue #34). It runs before visibility restoration and deletion. `-repair-stacks` fixes existing stacks.

With `-apply-edits`, an edited image is baked instead: the original is downloaded (checksum-verified, as the metadata source) along with Immich's render (`GET /assets/{id}/original?edited=true`). The render is already oriented, cropped, rotated and mirrored, and carries no EXIF. Immich serves the original instead when no render exists, so bytes matching the original's checksum are refused. While a new render is generated, Immich keeps serving the previous one, and no API reports which edits a render belongs to. Three guards cover that window. An asset whose `updatedAt` is less than a minute old on the server's clock (`Date` header of `/server/ping`) is skipped, since every edit and every finished render bumps it. The render's pixel size must equal the size the current edits produce on the server's oriented dimensions (crop size, axes swapped by a quarter turn), mirroring Immich's `getOutputDimensions`. And the render's content is checked against the unedited preview (`GET /assets/{id}/thumbnail?size=preview&edited=false`): a 24×24 grid of the render is mapped back through the edits (Immich applies them in list order after the crop; `rotate` turns clockwise, a `horizontal` mirror flips left-right) onto the preview, and the mean colour difference must stay under 12 (0-255), with none of the seven other orientations fitting clearly better. Measured on real renders, a current render scores 0.2-0.3 (6 on pure noise) and a stale mirror or turn 19-83; a live stale render, produced by pausing Immich's editor queue, scored 73.8 and was refused. JPEG XL renders cannot be decoded and are skipped. The original's metadata is copied onto the render without the tags that describe the original's pixels, and the render then goes through the normal diff, write and replace path. Face boxes need no remapping: for an edited asset, `GET /api/faces` already returns them on the edited image. A baked replacement is copied without the sidecar (`sidecar: false` on `/assets/copy`). Immich merges sidecar tags over the file's, and a foreign sidecar's orientation, dimensions (`tiff:ImageWidth`, verified live: a 800x500 bake registered as 1200x800 with its sidecar) or regions describe the pre-edit image; no endpoint serves a sidecar, so it cannot be cleaned and kept. Immich's tags, which otherwise survive a replace through that sidecar, are written into the file as `XMP-digiKam:TagsList` (the list Immich's importer reads first), and the pre-upload re-check skips the asset if they changed mid-run, since tag changes do not bump `updatedAt`. A duplicate resolution onto an asset carrying edits the replacement would not reproduce is refused, and an `-apply-edits` run refuses to start unless the server confirms its trash is enabled.

External-library assets carry a path checksum (SHA-1 of `path:` plus `originalPath`), not a content hash. The tool recognises one by recomputing it, since Immich 3.3 does not expose `checksumAlgorithm`, and then checks the download by Content-Length only.

With `-in-place-map SERVER=LOCAL`, an external asset under a mapped path is updated in place instead of replaced. The pipeline still downloads it, and the mapped local file must be byte-identical to that download before anything else happens. The usual diff and tag write run on the temporary copy. In place of the upload, the commit re-checks the sidecars and the local file (no symlink under the root, one hard link, owned by the running user, same inode, size, mtime and SHA-1 as when read). It then writes the rewritten copy to a hidden sibling carrying the original's group, mode and extended attributes (POSIX ACLs on Linux), fsyncs it, renames it over the original and fsyncs the directory, reporting a failed directory sync. A write by another program between the last check and the rename is not detected; there is no portable compare-and-swap. Last, it asks Immich to re-read the file (`POST /assets/jobs` `refresh-metadata`) until `fileModifiedAt` changes, repeating the request every ten seconds because a network or Docker file share can serve Immich stale attributes right after the rename. A sidecar Immich would apply over the file is read and compared with Immich's values: one that disagrees, or holds a description, position, date or rating Immich does not have, skips the asset, while the sidecar Immich itself writes for edited external files agrees and passes. Sidecars come from `GET /asset-files` (Immich 3.2+) plus the `<file>.xmp` and `<stem>.xmp` names. Nothing after the rename is retried, and the result is not cached, so the next run re-checks the file once.

`writeMethod()` uses PATCH on v3+ and PUT on legacy servers. `/assets/copy` always uses PUT.

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
    edits.go          Editor edit types, edited-size and description helpers
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
    edits.go          Immich editor edits and the edited render
    download.go       Streamed, watchdog-guarded downloads
    inplace.go        Sidecar listing and metadata refresh for in-place updates
    search.go         Search, list albums, paginated asset listing

  state/
    db.go             SQLite state cache for incremental all-assets runs

  process/
    pipeline.go       Per-asset processing orchestration
    faces.go          Face-region change collection (-faces)
    edits.go          Baking edits (-apply-edits) and carrying them to replacements
    renderCheck.go    Checks an edited render's content against the unedited preview
    export.go         Export-mode file copies
    replace.go        Upload with retries and the replace outcome
    inplace.go        In-place updates of external-library files (-in-place-map)
    replaceFile.go    Atomic swap of a library file
    worker.go         Worker pool with cancellation
    uploader.go       Upload interface and ModernUploader
    verify.go         Post-upload checksum verification

  ui/
    emitterLog.go     Console emitter with single-keypress input
    color.go          ANSI color helpers with terminal detection
```

## Immich API endpoints used

| Method      | Endpoint                                | Purpose                                                      |
| ----------- | --------------------------------------- | ------------------------------------------------------------ |
| GET         | `/api/server/about`                     | Server version detection (best-effort in forced modes)       |
| GET         | `/api/assets/{id}`                      | Fetch asset metadata and EXIF                                |
| GET         | `/api/assets/{id}/original`             | Download original file                                       |
| GET         | `/api/assets/{id}/original?edited=true` | Download Immich's edited render (only with `-apply-edits`)   |
| GET / PUT   | `/api/assets/{id}/edits`                | Read edits; re-apply them to the replacement                 |
| GET         | `/api/faces`                            | Face boxes for one asset (only with `-faces`)                |
| GET         | `/api/asset-files`                      | An asset's sidecars (only with `-in-place-map`)              |
| POST        | `/api/assets/jobs`                      | Ask Immich to re-read a file updated in place                |
| POST        | `/api/assets`                           | Upload new asset (multipart)                                 |
| PATCH / PUT | `/api/assets`                           | Update asset visibility (PATCH on v3+, PUT on legacy)        |
| PUT         | `/api/assets/copy`                      | Copy associations between assets (no PATCH alias on v3)      |
| DELETE      | `/api/assets`                           | Batch delete assets (always `force=false`, trash)            |
| POST        | `/api/search/metadata`                  | Paginated asset listing + album enumeration (per visibility) |
| GET         | `/api/albums`                           | List all albums                                              |
| GET         | `/api/albums/{id}`                      | Get album with asset list                                    |

All requests are authenticated via `x-api-key` header. v3 is the primary API contract: version detection assumes v3 when the reported version is unrecognizable, and `-immich-api legacy` forces the older contract. Redirects that leave the configured host (or downgrade https to http) are refused so the key cannot leak.

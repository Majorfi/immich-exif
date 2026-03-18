# Architecture

## Upload strategy

The tool uses a four-step process:

1. **POST /assets** — Upload the modified file as a new asset
2. **PUT /assets/copy** — Copy all associations (albums, favorites, shared links, sidecars, stacks) from old to new
3. **PUT /assets** — Restore visibility if the original was archived or had non-default visibility
4. **DELETE /assets** — Force-delete the original asset

Upload is sent as a streamed multipart request (chunked), so large files are not buffered fully in memory.

If the upload returns the same asset ID, copy/delete is skipped.

If upload returns `duplicate`/`replaced`:

- default behavior: copy/delete is skipped and the result is marked as skipped (not cached)
- with `-resolve-duplicate`: for `duplicate` with different ID, the tool copies associations to that duplicate asset and deletes the old asset

When duplicates are skipped (default mode), a final summary lists them and prints a command you can rerun with `-resolve-duplicate`. If running in an interactive terminal, the tool also prompts to patch them immediately without re-running.

If the copy or visibility step fails, the old asset is **not** deleted to avoid data loss. If the delete step fails, the new asset is already live and a warning is emitted.

## Processing pipeline

```
main.go
  |
  +-- parseConfig()                CLI flags, env vars, .env file
  +-- state.OpenStateDB()          SQLite state cache (--all / -album all)
  +-- resolveAssetIDs()            --all / --album / positional IDs
  |                                (shouldSkip callback filters cached assets)
  |
  +-- runClassic()                 Console mode + ui.LogEmitter
  +-- ui.RunTUI()                  Bubble Tea mode + ui.TUIEmitter
  |    |
  |    +-- process.WorkerPool.Process(assetIDs)
  |         |
  |         +-- process.ProcessAsset()          per asset, in worker goroutine
  |              |
  |              +-- api.GetAsset()                 fetch metadata
  |              +-- exif.CompareAssetMetadata()     early skip if nothing writable
  |              +-- api.DownloadAsset()             download original file
  |              +-- exif.ReadExifTagsFn()            exiftool -json -n
  |              +-- exif.CompareAssetMetadata()     diff Immich vs file metadata
  |              +-- EmitDiff()                      show diff, wait for user
  |              +-- exif.WriteExifTagsFn()          exiftool -overwrite_original
  |              +-- uploader.Upload()               POST + copy + visibility + DELETE
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
    assetType.go      Asset classification (video detection, mime types)
    helpers.go        ShortID, TruncateFilename

  exif/
    tool.go           EXIF read and write (exiftool subprocess)
    compare.go        Metadata comparison, diff generation, exiftool arg building
    match.go          Value matching helpers (float, string, int, datetime)
    video.go          Video-specific metadata comparison and routing

  api/
    client.go         HTTP client base (request, JSON, ping)
    assets.go         Asset CRUD (get, download, upload, copy, delete)
    search.go         Search, list albums, paginated asset listing

  state/
    db.go             SQLite state cache for incremental all-assets runs

  process/
    pipeline.go       Per-asset processing orchestration
    worker.go         Worker pool with cancellation
    uploader.go       Upload interface and ModernUploader

  ui/
    emitterLog.go     Classic console emitter with single-keypress input
    emitterTUI.go     TUI event bridge (channel-based blocking)
    tui.go            Bubble Tea model and update logic
    tuiView.go        TUI rendering (progress bar, diff, results)
```

## Immich API endpoints used

| Method | Endpoint                    | Purpose                          |
| ------ | --------------------------- | -------------------------------- |
| GET    | `/api/server/about`         | Ping / connectivity check        |
| GET    | `/api/assets/{id}`          | Fetch asset metadata and EXIF    |
| GET    | `/api/assets/{id}/original` | Download original file           |
| POST   | `/api/assets`               | Upload new asset (multipart)     |
| PUT    | `/api/assets`               | Update asset visibility          |
| PUT    | `/api/assets/copy`          | Copy associations between assets |
| DELETE | `/api/assets`               | Batch delete assets              |
| POST   | `/api/search/metadata`      | Paginated asset listing          |
| GET    | `/api/albums`               | List all albums                  |
| GET    | `/api/albums/{id}`          | Get album with asset list        |

All requests are authenticated via `x-api-key` header.

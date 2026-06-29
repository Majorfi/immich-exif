# immich-exif

## Beta

Validated end-to-end against real Immich servers (download → embed → verify → replace), but still pre-1.0. The tool replaces real assets, so always preview a run with `-dry-run` first.

A CLI tool that synchronizes metadata from an [Immich](https://immich.app) photo server back into the original files.

Immich stores rich metadata (GPS, descriptions, ratings, camera info, dates) in its database, but this metadata isn't always embedded in the file itself. This tool bridges that gap by downloading each asset, embedding the missing tags via `exiftool`, and re-uploading the modified file.

## How it works

```
For each asset:
  1. Fetch metadata from Immich API
  2. Skip early if Immich has no metadata fields to embed
  3. Download the original file (checksum-verified against the server)
  4. Read existing metadata tags (exiftool)
  5. Diff Immich metadata vs file metadata
  6. Show the diff, ask to confirm / skip / quit
  7. Write missing tags into the file (exiftool)
  8. Re-upload, copy associations, restore visibility
  9. Verify the re-uploaded asset's checksum, then delete the original
```

Assets that already have matching metadata are skipped automatically, making it safe to run repeatedly.
Video metadata writing is supported for `mp4`, `mov`, and `m4v`. Other video containers are skipped.

## Safety

The destructive path is the careful path. By default the tool will not delete an original it cannot prove was replaced intact:

- **Downloads are checksum-verified** — a corrupt or truncated download is rejected before any tag is written or uploaded.
- **Uploads are checksum-verified by default** — the re-uploaded asset is re-fetched and its checksum compared to the local file before the original is touched. A mismatch refuses to delete the original.
- **Permanent only when verified** — a verified original is deleted permanently (the new copy is provably byte-identical). With `-no-verify-upload` the check is skipped and the original is moved to Immich's **trash** instead, where it stays recoverable.
- **`-dry-run`** writes nothing and shows every change first.
- The API key is refused over plaintext `http://` unless you pass `-allow-http`.

An interruption (Ctrl-C) or a failed step leaves a duplicate, never a hole: the original is removed only after a successful replacement.

## Quick wins

| Scenario                                                    | Command                                                 |
| ----------------------------------------------------------- | ------------------------------------------------------- |
| I want to test one photo safely                             | `immich-exif -dry-run <asset-id>`                       |
| I want to process one album interactively                   | `immich-exif -album <album-id>`                         |
| I want to process everything without prompts                | `immich-exif -y -all`                                   |
| I want to process everything faster (4 workers)             | `immich-exif -y -workers 4 -all`                        |
| I want eligible files exported to album folders (no upload) | `immich-exif -y -export-dir ./out -all`                 |
| I want to export one album in its own folder                | `immich-exif -y -export-dir ./export -album <album-id>` |
| I want to export all albums                                 | `immich-exif -y -export-dir ./export -album all`        |
| I want duplicates auto-resolved                             | `immich-exif -y -resolve-duplicate -album <album-id>`   |
| I want to ignore cache and re-check everything              | `immich-exif -y -force -all`                            |

When using `-export-dir` with exactly one `-album`, files are exported to `/<export-dir>/<album-id>/`.
With `-export-dir` and `-all` or `-album all`, exported assets are mirrored per album folder (`/<export-dir>/<album-id>/...`), including shared assets in each album folder. Assets with no album go to `/<export-dir>/no-album/` by default and can be omitted with `-include-no-album=false`.
With `-export-dir` and multiple explicit `-album` flags, assets are mirrored per album folder (`/<export-dir>/<album-id>/...`).
This is not a full-library backup mode: assets with no writable metadata to embed and assets whose metadata already matches are still skipped.

### Incremental mode (`--all` / `-album all`)

When using `--all` or `-album all`, the tool maintains a local SQLite state cache that tracks which assets have already been processed. On subsequent runs, assets whose Immich metadata hasn't changed are skipped entirely, avoiding the expensive download/compare/upload cycle.

- State is stored in `~/.config/immich-exif/state.db` (macOS: `~/Library/Application Support/`)
- Cache is keyed per server URL, so multiple Immich instances don't interfere
- Only assets with finalized outcomes are cached:
  - uploaded and migrated successfully
  - replaced in-place (new ID equals old ID)
  - confirmed as already matching metadata
- `dry-run`, `export-dir`, and `duplicate`/`replaced` upload statuses are never cached
- Use `--force` to ignore the cache and re-process everything (state is still saved for the next run)
- Delete `state.db` to fully reset the cache

## Prerequisites

- An Immich server with a valid API key
- [exiftool](https://exiftool.org/) on your `PATH` (the Docker image already bundles it)
- [Go 1.24+](https://golang.org/dl/) only if you build from source

## Installation

### Prebuilt binary

Download the archive for your OS and architecture from the [latest release](https://github.com/Majorfi/immich-exif/releases/latest), extract it, and put `immich-exif` on your `PATH`. Install `exiftool` separately.

### Docker (bundles exiftool)

```bash
docker run --rm \
  -e IMMICH_URL=https://your-immich-server.com \
  -e IMMICH_API_KEY=your-api-key \
  ghcr.io/majorfi/immich-exif:latest -dry-run <asset-id>
```

Mount a volume (`-v "$PWD/out:/out"`) when using `-export-dir /out`.

### Build from source

```bash
cd src
go build -o immich-exif .
```

## Configuration

The tool reads credentials from CLI flags or environment variables. A `.env` file is also supported.

```bash
# .env
IMMICH_URL=https://your-immich-server.com
IMMICH_API_KEY=your-api-key
```

## API key permissions

On Immich 1.113+ you can scope the API key to exactly what the tool needs (older servers issue all-or-nothing keys). A normal run that re-uploads and replaces assets uses:

| Permission       | Why                                                       |
| ---------------- | --------------------------------------------------------- |
| `server.about`   | Connectivity and server-version detection                 |
| `asset.read`     | Read asset metadata and page the library and albums       |
| `asset.download` | Download the original file                                |
| `asset.upload`   | Re-upload the metadata-corrected file                     |
| `asset.copy`     | Copy associations (albums, favorites, …) to the new asset |
| `asset.update`   | Restore visibility for archived or hidden assets          |
| `asset.delete`   | Remove the old original after a verified replacement      |
| `album.read`     | Resolve `-album` / `-album all` selections                |

Read-only modes need less: `-dry-run` and `-export-dir` never write to the server, so they only require `server.about`, `asset.read`, `asset.download`, and `album.read` (drop `album.read` too if you only pass asset IDs).

## Usage

```
immich-exif [flags] [asset-ids...]
```

### Flags

| Flag                 | Default           | Description                                                                                                     |
| -------------------- | ----------------- | --------------------------------------------------------------------------------------------------------------- |
| `-url`               | `$IMMICH_URL`     | Immich server URL                                                                                               |
| `-api-key`           | `$IMMICH_API_KEY` | API key                                                                                                         |
| `-immich-api`        | `auto`            | API contract: `auto` (detect from server version), `legacy`, or `v3`                                            |
| `-workers`           | `1`               | Number of parallel workers                                                                                      |
| `-dry-run`           | `false`           | Embed EXIF locally but skip re-upload                                                                           |
| `-export-dir`        |                   | Save modified files to a directory instead of re-uploading (fails if file exists)                               |
| `-y`                 | `false`           | Auto-confirm all changes                                                                                        |
| `-no-verify-upload`  | `false`           | Skip the post-upload checksum verification; the original is moved to trash instead of being permanently deleted |
| `-allow-http`        | `false`           | Allow a plaintext `http://` server URL (the API key is sent in clear text)                                      |
| `-list-albums`       | `false`           | List your albums (ID and name) and exit                                                                         |
| `-resolve-duplicate` | `false`           | On duplicate upload status, copy associations to duplicate asset and delete old asset                           |
| `-include-no-album`  | `true`            | With album-mirrored export, include assets with no album under `no-album/`                                      |
| `-all`               | `false`           | Select the all-assets mode; equivalent to `-album all`                                                          |
| `-force`             | `false`           | Force re-processing all assets, ignoring state cache                                                            |
| `-album`             |                   | Album ID to process (repeatable), or `all` as an alias of `-all`                                                |

### Asset selection

One of these is required:

```bash
# Process specific assets
immich-exif asset-id-1 asset-id-2

# Process all assets in an album
immich-exif -album <album-id>

# Process multiple albums
immich-exif -album <id1> -album <id2>

# Process assets from all albums
immich-exif -album all

# Select the all-assets mode
immich-exif -all
```

To find album IDs, list them first:

```bash
immich-exif -list-albums
# 4c1f…  Vacation 2024 (312)
# 9ab2…  Family (87)
```

`-all` and `-album all` are equivalent selectors. The tool still only exports/processes assets that pass its normal filters.

### Examples

```bash
# Interactive dry-run on a single asset
immich-exif -dry-run abc123

# Non-interactive, export eligible files to album folders
immich-exif -y -export-dir ./out -all

# Auto-confirm everything, 4 workers
immich-exif -y -workers 4 -all

# Force re-process everything (ignore cache)
immich-exif -y -force -all
```

## Output

Console output with interactive single-keypress prompts. Each asset shows a diff and waits for input:

```
[1/5] 2 EXIF mismatch found for IMG_1234.jpg:
    + OffsetTimeOriginal    (none)               -> +01:00
    ~ Rating                3                    -> 5

[y] confirm  [s] skip  [q] quit:
```

No Enter key needed. Use `-y` to auto-confirm.
Interactive mode forces single-worker to avoid mixed prompts; parallel workers apply when using `-y`. Under `-y` each asset prints only its diff block (the per-step upload progress is omitted), and each block prints atomically, so multiple workers never interleave their output. Final outcomes and any failures are reported in the closing summary.

Output is colorized when stdout is a terminal (added tags in green, changed tags in amber, failures in red). Colors are disabled automatically when the output is piped or redirected, and when `NO_COLOR` is set.

## Metadata tags

### Tags written

Images use the full tag set below. Supported video containers (`mp4`, `mov`, `m4v`) use a compatible subset: description, rating, GPS, XMP location, dates, and camera fields.

| Category    | Tags                                                                                                                                         | Notes                                                                                                    |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| GPS         | `GPSLatitude`, `GPSLatitudeRef`, `GPSLongitude`, `GPSLongitudeRef`, `XMP-exif:GPSLatitude`, `XMP-exif:GPSLongitude`                          | Ref derived from coordinate sign; XMP uses signed                                                        |
| Description | `ImageDescription`, `XPComment`, `XMP-dc:Description`, `IPTC:Caption-Abstract`                                                               | EXIF + Windows + XMP Dublin Core + IPTC                                                                  |
| Rating      | `Rating`, `RatingPercent`, `XMP-xmp:Rating`                                                                                                  | Percent = rating x 20; skipped when rating is 0; `RatingPercent` omitted for negative (rejected) ratings |
| Location    | `IPTC:City`, `XMP-photoshop:City`, `IPTC:Province-State`, `XMP-photoshop:State`, `IPTC:Country-PrimaryLocationName`, `XMP-photoshop:Country` | Dual IPTC + XMP-photoshop                                                                                |
| DateTime    | `DateTimeOriginal`, `OffsetTimeOriginal`, `TimeZoneOffset`, `XMP-exif:DateTimeOriginal`, `XMP-xmp:CreateDate`                                | See below; XMP uses ISO 8601                                                                             |
| Camera      | `Make`, `Model`, `LensModel`                                                                                                                 | Only written if file has no existing value                                                               |

### DateTime and timezone handling

Immich returns ISO 8601 dates (e.g. `2025-12-10T16:56:36+00:00`). EXIF stores local time with a separate offset (e.g. `2025:12:10 17:56:36` + `OffsetTimeOriginal: +01:00`).

The tool handles this carefully:

- **If the file already has `DateTimeOriginal` but no offset**: the existing local time is preserved. The offset is computed from the difference between the file's local time and Immich's UTC time, then written as `OffsetTimeOriginal` and `TimeZoneOffset`.
- **If the file has no date at all**: `DateTimeOriginal` is written in EXIF format (`YYYY:MM:DD HH:MM:SS`) along with the offset tags.
- **If everything matches**: the asset is skipped.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for upload strategy, processing pipeline, file structure, and API endpoints.

## License

Licensed under the [GNU General Public License v3.0](LICENSE).

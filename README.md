# Immich Exif

A CLI tool that synchronizes metadata from an [Immich](https://immich.app) photo server back into the original files.

Immich stores rich metadata (GPS, descriptions, ratings, camera info, dates, named people) in its database, but this metadata isn't always embedded in the file itself. This tool bridges that gap by downloading each asset, embedding the missing tags via `exiftool`, and re-uploading the modified file.

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
  9. Verify the re-uploaded asset's checksum, then move the original to Immich's trash
```

Assets that already have matching metadata are skipped automatically, making it safe to run repeatedly.
Video metadata writing is supported for `mp4`, `mov`, and `m4v`. Other video containers are skipped.

## Safety

The destructive path is the careful path. By default the tool will not delete an original it cannot prove was replaced intact:

- **Downloads are checksum-verified** — a corrupt or truncated download is rejected before any tag is written or uploaded.
- **Uploads are checksum-verified by default** — the re-uploaded asset is re-fetched and its checksum compared to the local file before the original is touched. A mismatch refuses to delete the original.
- **Trash, never permanent delete** — the replaced original always goes to Immich's **trash**, where it stays recoverable. Checksum verification proves the transfer was intact, not that exiftool produced a valid file, so the trash window is kept as the last-resort recovery path.
- **Live photos stay paired** — the still's `livePhotoVideoId` is forwarded on re-upload, and hidden videos (live-photo motion parts) are skipped rather than replaced, which would sever the pair.
- **Mid-run edits are respected** — the asset is re-checked right before upload; if its metadata changed on the server while the tool was working, it is skipped instead of overwritten.
- **External libraries are protected** — replacing an external-library asset would migrate a copy into Immich's internal library (and duplicate it at the next scan), so those assets are skipped on replace runs. `-dry-run` and `-export-dir` still cover them (Immich hashes an external file's path, not its content, so their downloads are checked by size only), and `-in-place-map` updates them in place (see [below](#updating-external-libraries-in-place--in-place-map)).
- **Immich edits survive** — Immich's editor keeps crops, rotations and mirrors apart from the file, and does not copy them to a new asset. The replacement gets the original's edits re-applied before the original is trashed; if that fails, the original stays. `-apply-edits` bakes them into the file instead (see [below](#baking-immich-edits--apply-edits)).
- **Face regions are opt-in** — `-faces` replaces the file's `XMP-mwg-rs` region structure with Immich's named people; without the flag, region metadata is never touched.
- **Trash disabled on the server?** The tool warns you loudly: without the trash feature there is no recovery window. `-apply-edits` refuses to run unless the server confirms its trash is enabled, since the unedited original would be lost for good.
- **`-dry-run`** writes nothing and shows every change first.
- The API key is refused over plaintext `http://` unless you pass `-allow-http`.

An interruption (Ctrl-C) or a failed step leaves a duplicate, never a hole: the original is removed only after a successful replacement.

## Quick wins

| Scenario                                                    | Command                                                         |
| ----------------------------------------------------------- | --------------------------------------------------------------- |
| I want to test one photo safely                             | `immich-exif -dry-run <asset-id>`                               |
| I want to process one album interactively                   | `immich-exif -album <album-id>`                                 |
| I want to process everything without prompts                | `immich-exif -y -all`                                           |
| I want to process everything faster (4 workers)             | `immich-exif -y -workers 4 -all`                                |
| I want eligible files exported to album folders (no upload) | `immich-exif -y -export-dir ./out -all`                         |
| I want to export one album in its own folder                | `immich-exif -y -export-dir ./export -album <album-id>`         |
| I want to export all albums                                 | `immich-exif -y -export-dir ./export -album all`                |
| I want duplicates auto-resolved                             | `immich-exif -y -resolve-duplicate -album <album-id>`           |
| I want to ignore cache and re-check everything              | `immich-exif -y -force -all`                                    |
| I want named people written as face regions too             | `immich-exif -y -faces -all`                                    |
| I want external-library files updated where they are        | `immich-exif -y -in-place-map /mnt/photos=/volume1/photos -all` |
| I want Immich crops and rotations baked into the files      | `immich-exif -y -apply-edits -all`                              |

When using `-export-dir` with exactly one `-album`, files are exported to `/<export-dir>/<album-id>/`.
With `-export-dir` and `-all` or `-album all`, exported assets are mirrored per album folder (`/<export-dir>/<album-id>/...`), including shared assets in each album folder. Assets with no album go to `/<export-dir>/no-album/` by default and can be omitted with `-include-no-album=false`.
With `-export-dir` and multiple explicit `-album` flags, assets are mirrored per album folder (`/<export-dir>/<album-id>/...`).
This is not a full-library backup mode: assets with no writable metadata to embed and assets whose metadata already matches are still skipped.

#### Renaming exported files (`-rename`)

Messaging apps like WhatsApp and Signal strip EXIF on send, so a shared photo loses the date this tool just fixed. Add `-rename` (export mode only) to also stamp the corrected capture date into the filename, keeping the date readable even after the metadata is gone:

```bash
immich-exif -y -export-dir ./out -rename -all
# IMG_45698.JPG -> 20040605-142532.JPG
```

The name is built from the same corrected date and time zone written into the file's EXIF. The default pattern is `%Y%m%d-%H%M%S`; override it with `-rename-pattern` (or `RENAME_PATTERN`) using the tokens `%Y %y %m %d %H %M %S` (and `%%` for a literal percent). The original extension is always preserved. If the server has no date for an asset, its original name is used. If the destination name already exists, a `-001`, `-002` suffix is added so nothing is overwritten.

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

- An Immich server with a valid API key. The full replace flow needs **Immich v2.2+** (`/api/assets/copy`); auto-detection needs 1.113+ (`/api/server/about`, or force `-immich-api`); read-only modes (`-list-albums`, `-dry-run`, `-export-dir`) work from 1.106
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

Mount a volume (`-v "$PWD/out:/out"`) when using `-export-dir /out`. The container runs as uid 1000, so mounted directories must be writable by that user.
For incremental `-all` runs, persist the state cache with `-v immich-exif-state:/home/appuser`.

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
# Optional: rename exported files from the corrected capture date (export mode)
RENAME=true
RENAME_PATTERN=%Y%m%d-%H%M%S
```

## API key permissions

On Immich 1.113+ you can scope the API key to exactly what the tool needs (older servers issue all-or-nothing keys). A normal run that re-uploads and replaces assets uses:

| Permission          | Why                                                                                 |
| ------------------- | ----------------------------------------------------------------------------------- |
| `server.about`      | Server-version detection (optional with a forced `-immich-api`)                     |
| `asset.read`        | Read asset metadata and page the library and albums                                 |
| `asset.download`    | Download the original file                                                          |
| `asset.upload`      | Re-upload the metadata-corrected file                                               |
| `asset.copy`        | Copy associations (albums, favorites, …) to the new asset                           |
| `asset.update`      | Restore visibility for archived or hidden assets                                    |
| `asset.delete`      | Trash the old original after a verified replacement                                 |
| `stack.update`      | Hand the replacement the stack primary slot the original held                       |
| `album.read`        | Resolve `-album` / `-album all` selections                                          |
| `face.read`         | Fetch face boxes — only needed with `-faces`                                        |
| `asset.file.read`   | List an asset's sidecars — only needed with `-in-place-map` (Immich 3.2+)           |
| `job.create`        | Ask Immich to re-read a file — only needed with `-in-place-map`                     |
| `asset.edit.get`    | Read Immich editor edits — only needed for edited assets                            |
| `asset.edit.create` | Re-apply edits to the replacement — edited assets without `-apply-edits`            |
| `asset.view`        | Read the unedited preview that verifies an edited render — only with `-apply-edits` |
| `stack.read`        | List stacks — only needed with `-repair-stacks`                                     |

Read-only modes need less: `-dry-run` and `-export-dir` never write to the server, so they only require `server.about`, `asset.read`, `asset.download`, and `album.read` (drop `album.read` too if you only pass asset IDs; add `face.read` if you combine them with `-faces`, and `asset.edit.get` plus `asset.view` with `-apply-edits`). `-in-place-map` runs need `asset.read`, `asset.download`, `asset.update`, `asset.file.read` and `job.create`.

## Usage

```
immich-exif [flags] [asset-ids...]
```

### Flags

| Flag                 | Default           | Description                                                                                                |
| -------------------- | ----------------- | ---------------------------------------------------------------------------------------------------------- |
| `-url`               | `$IMMICH_URL`     | Immich server URL                                                                                          |
| `-api-key`           | `$IMMICH_API_KEY` | API key                                                                                                    |
| `-immich-api`        | `auto`            | API contract: `auto` (detect; assumes `v3` when unsure), `v3`, or `legacy`                                 |
| `-workers`           | `1`               | Number of parallel workers                                                                                 |
| `-dry-run`           | `false`           | Embed EXIF locally but skip re-upload                                                                      |
| `-export-dir`        |                   | Save modified files to a directory instead of re-uploading (fails if file exists)                          |
| `-y`                 | `false`           | Auto-confirm all changes                                                                                   |
| `-no-verify-upload`  | `false`           | Skip the post-upload checksum verification that gates the replacement                                      |
| `-allow-http`        | `false`           | Allow a plaintext `http://` server URL (the API key is sent in clear text)                                 |
| `-list-albums`       | `false`           | List your albums (ID and name) and exit                                                                    |
| `-repair-stacks`     | `false`           | Re-point stacks whose primary asset is in the trash to a live member, and exit                             |
| `-resolve-duplicate` | `false`           | On duplicate upload status, copy associations to the duplicate asset and trash the old one                 |
| `-include-no-album`  | `true`            | With album-mirrored export, include assets with no album under `no-album/`                                 |
| `-all`               | `false`           | Select the all-assets mode (timeline, archived and hidden; external-library assets only on read-only runs) |
| `-force`             | `false`           | Ignore the state cache and re-process (only valid with `-all` / `-album all`)                              |
| `-faces`             | `false`           | Also write named Immich people as MWG face regions (`XMP-mwg-rs`), readable by digiKam and others          |
| `-in-place-map`      |                   | Update external-library files in place: `SERVER_PATH=LOCAL_DIR`, repeatable; see below                     |
| `-apply-edits`       | `false`           | Bake Immich editor edits (crop, rotate, mirror) into the file (env: `APPLY_EDITS=true`); see below         |
| `-rename`            | `false`           | With `-export-dir`, rename exported files from the corrected capture date (env: `RENAME=true`)             |
| `-rename-pattern`    | `%Y%m%d-%H%M%S`   | strftime-style pattern for `-rename` (env: `RENAME_PATTERN`)                                               |
| `-album`             |                   | Album ID to process (repeatable), or `all` as an alias of `-all`                                           |
| `-version`           | `false`           | Print the version and exit                                                                                 |

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

### Repairing stacks hidden by an older version

`-repair-stacks` promotes a live member when a stack's primary is trashed. Use `-dry-run` to preview changes:

```bash
immich-exif -repair-stacks -dry-run
immich-exif -repair-stacks
```

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

Console output with interactive single-keypress prompts. While an asset is processed, a live counter line updates in place through each step — the percentage tracks the download of that file (skipped assets tick the counter too):

```
[37/128] Downloading IMG_1234.jpg... 45%
```

Each asset with changes then shows a diff and waits for input (the counter line is erased first):

```
[1/5] 2 EXIF mismatch found for IMG_1234.jpg:
    + OffsetTimeOriginal    (none)               -> +01:00
    ~ Rating                3                    -> 5

[y] confirm  [s] skip  [q] quit:
```

No Enter key needed. Use `-y` to auto-confirm. When output is piped or redirected, the counter prints one line per step instead of updating in place, and percent-only updates are omitted.
Interactive mode forces single-worker to avoid mixed prompts; parallel workers apply when using `-y`. Under `-y` on a terminal, the live counter still shows between diff blocks — one line per file in flight when running several workers; the per-step upload logs stay omitted. When `-y` output is piped or redirected, each asset prints only its diff block, and each block prints atomically, so multiple workers never interleave their output. Final outcomes and any failures are reported in the closing summary, which also groups skipped assets by reason, most frequent first, with up to five IDs each (duplicates have their own summary):

```
Done: 12 succeeded, 4 skipped, 0 failed

Skipped assets:
  3 × metadata already matches (4c1f2a9e, 9ab2d771, e04f1c3b)
  1 × asset changed less than a minute ago; Immich may still be rendering its edits, re-run shortly (7d3e8b20)
```

Output is colorized when stdout is a terminal (added tags in green, changed tags in amber, failures in red). Colors are disabled automatically when the output is piped or redirected, and when `NO_COLOR` is set.

## Metadata tags

### Tags written

Images use the full tag set below. Supported video containers (`mp4`, `mov`, `m4v`) use a compatible subset: description, rating, GPS, XMP location, dates, and camera fields.

| Category    | Tags                                                                                                                                         | Notes                                                                                                      |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| GPS         | `GPSLatitude`, `GPSLatitudeRef`, `GPSLongitude`, `GPSLongitudeRef`, `XMP-exif:GPSLatitude`, `XMP-exif:GPSLongitude`                          | Ref derived from coordinate sign; XMP uses signed                                                          |
| Description | `ImageDescription`, `XPComment`, `XMP-dc:Description`, `IPTC:Caption-Abstract`                                                               | EXIF + Windows + XMP Dublin Core + IPTC                                                                    |
| Rating      | `Rating`, `RatingPercent`, `XMP-xmp:Rating`                                                                                                  | Percent = rating x 20; an explicit rating of 0 clears stale rating tags (null = never rated = untouched)   |
| Location    | `IPTC:City`, `XMP-photoshop:City`, `IPTC:Province-State`, `XMP-photoshop:State`, `IPTC:Country-PrimaryLocationName`, `XMP-photoshop:Country` | Dual IPTC + XMP-photoshop                                                                                  |
| DateTime    | `DateTimeOriginal`, `OffsetTimeOriginal`, `TimeZoneOffset`, `XMP-exif:DateTimeOriginal`, `XMP-xmp:CreateDate`                                | See below; XMP uses ISO 8601                                                                               |
| Camera      | `Make`, `Model`, `LensModel`                                                                                                                 | Only written if file has no existing value                                                                 |
| Faces       | `XMP-mwg-rs:RegionInfo` (MWG face regions)                                                                                                   | Opt-in via `-faces`; images and supported videos, named people only; see below                             |
| Tags        | `XMP-digiKam:TagsList`                                                                                                                       | Only on files baked by `-apply-edits`, which do not inherit the sidecar that otherwise carries Immich tags |

### Face regions (`-faces`)

With `-faces`, every person you have **named** in Immich is written into the file as an MWG face region (`XMP-mwg-rs:RegionInfo`) — the standard read by digiKam, XnView and others. Details worth knowing:

- Only **named, visible** people are written. Unnamed ML clusters and people you hid in Immich are left out — the same rule Immich applies when importing regions from files.
- Regions are written in the stored image's coordinate space, applying the exact inverse of the orientation transform Immich uses on import, so rotated photos round-trip correctly.
- When the file's regions disagree with Immich, the whole `RegionInfo` structure is **replaced** — Immich is the source of truth, like for every other synced tag. When Immich has no named faces for an asset, existing file regions are left untouched (never cleared).
- Round-trip bonus: with Immich's _"Import faces from metadata"_ server setting enabled, the names embedded by `-faces` are re-imported when the replaced file is scanned — face names survive the replace. Re-imported regions (`sourceType: exif`) are recognized as echoes of the file's own content and never counted next to a detected face of the same name (names are how the importer links regions to people), so repeated runs converge instead of duplicating regions.
- **Supported videos** (`mp4`, `mov`, `m4v`) embed the same MWG regions as images — exiftool writes XMP into the container, and Immich re-imports them when _"Import faces from metadata"_ is enabled, exactly as for photos. Regions are anchored through the video's display `Rotation` (0°, 90°, 270°); a 180° or non-cardinal rotation is left un-embedded rather than risk a misplaced box, and other containers are skipped.
- The key needs the `face.read` permission (see [API key permissions](#api-key-permissions)).
- Incremental-cache caveats: the `-all` state cache keys on the set of _names_, so correcting a face's box or reassigning a face between two already-named people does not invalidate it — use `-force` to re-check. Toggling `-faces` on or off changes the cache key, so the first run after a toggle re-checks the library once.

### Baking Immich edits (`-apply-edits`)

Immich's editor (2.5+) is non-destructive: crops, rotations and mirrors live in Immich's database, so other apps see the unedited file. With `-apply-edits`, every edited photo is replaced by the edited version:

- The tool downloads Immich's own render of the edits, copies the original's metadata onto it (camera, lens, dates, GPS, keywords…), then writes Immich's metadata as usual. Tags that describe the original's pixels are left behind: orientation, dimensions, embedded previews, colour profile, face regions and crop settings.
- **The render is re-encoded by your server**, with the format and quality set in Immich's _Administration › Settings › Image Settings › Full-size image_ (JPEG, quality 80 by default). Raise the quality there before baking. The extension follows the render (a `.heic` or RAW original becomes `.jpeg`).
- The replacement does not inherit the original's XMP sidecar: Immich applies a sidecar over the file, and the orientation, dimensions or face regions in it describe the photo before the edits. What Immich itself keeps in the sidecar (description, date, GPS, rating) is embedded anyway, and Immich's tags are written to `XMP-digiKam:TagsList`. Other sidecar-only XMP (for example a Lightroom copyright) stays with the trashed original: Immich's API cannot read a sidecar, so it cannot be cleaned and kept.
- The unedited original goes to the trash like any replaced asset, and the replacement carries no edits. Re-editing it in Immich starts from the baked pixels.
- An asset is skipped, never replaced, when it changed less than a minute ago, when Immich has not rendered its edits yet, or when its render does not show the current edits: the render's size and content are checked against Immich's unedited preview, so a render left over from earlier edits (Immich still re-rendering) is never baked. Re-run later. Renders in JPEG XL cannot be checked and are skipped; use JPEG or WebP.
- With `-faces`, the regions are written for the edited image. Without `-apply-edits`, face regions are skipped for edited assets: Immich reports their boxes on the edited image, not on the original's pixels.
- Works with `-dry-run` and `-export-dir`. On `-all` runs, edited assets bypass the state cache.

### Updating external libraries in place (`-in-place-map`)

Replacing an asset gives it a new ID. For an **external library**, whose files you own, the tool can instead rewrite the file where it lives and keep the asset as it is. Give it the library path as Immich sees it and where this machine mounts the same files:

```bash
immich-exif -y -in-place-map /mnt/photos=/volume1/photos -all
```

- The asset keeps its ID, albums, faces and everything else; nothing is uploaded, duplicated or trashed. Immich identifies external files by path, so its checksum stays valid, and the tool asks it to re-read the file (repeating the request until Immich sees the new version, since network and Docker file shares can serve old attributes for a few seconds). If Immich is still busy after half a minute, the asset is reported as updated anyway: the file's new modification time makes the next library scan re-read it.
- **There is no trash window**: the file is rewritten for good. Run `-dry-run` first and keep backups.
- Before writing, the local file must be byte-identical to the one Immich serves (a wrong mapping can never rewrite another file), and it is re-checked right before the swap. The new file is written next to the original and renamed over it, so the swap is atomic; its group, mode and extended attributes (which hold POSIX ACLs on Linux) are kept, and an asset whose attributes cannot be reproduced is skipped. A change made by another program in the instant between that last check and the swap would be overwritten, so do not run it while something else edits the same files.
- An asset is skipped, never forced, when the file is a symlink or sits under one, has other hard links, belongs to another user, carries a macOS access control list, changed while processing, or when a sidecar disagrees with Immich. Immich writes its own sidecar next to external files you edit in Immich; that one agrees and does not block. A sidecar holding another value would be applied over the file by Immich, so it is reported instead.
- Only external-library assets under a mapped path are updated in place; everything else follows the usual replace flow. Not available on Windows, and it cannot be combined with `-export-dir` or `-apply-edits` (baking in place would destroy the original pixels).
- Uploaded (non-external) assets cannot be updated in place: Immich stores a checksum of their content and has no API to replace it, so a rewritten file would break Immich's integrity checks and the mobile app's backup matching.

### Things worth knowing

- **`duplicate key value violates unique constraint "UQ_assets_owner_checksum"` in the database logs** is Immich detecting an upload identical to an existing asset, usually a replacement left by an interrupted run. Immich answers "duplicate"; the tool skips the asset and suggests `-resolve-duplicate`.
- **Replacing an asset changes its ID.** Albums, favorites, shared links, stacks and the sidecar follow, but bookmarks and external tools referring to the old ID do not. External libraries can avoid this with `-in-place-map`.
- **Immich applies a sidecar over the file.** If an asset's `.xmp` sidecar holds a value that no longer matches Immich (for example after editing Immich's database directly), that value wins again at the next metadata refresh, whatever the tool embedded. Immich keeps its own sidecars in sync with edits made through Immich.

### DateTime and timezone handling

Immich returns ISO 8601 dates (e.g. `2025-12-10T16:56:36+00:00`). EXIF stores local time with a separate offset (e.g. `2025:12:10 17:56:36` + `OffsetTimeOriginal: +01:00`).

The tool handles this carefully:

- **The asset's own time zone is honored**: when Immich knows it (`exifInfo.timeZone`, either `Europe/Rome` or `UTC+2` form), the UTC date is re-anchored so the wall-clock time written into the file matches what you see in Immich.
- **If the file already has `DateTimeOriginal` but no offset**: the existing local time is preserved. The offset is computed from the difference between the file's local time and Immich's UTC time, then written as `OffsetTimeOriginal` and `TimeZoneOffset`.
- **If the file has no date at all**: `DateTimeOriginal` is written in EXIF format (`YYYY:MM:DD HH:MM:SS`) along with the offset tags.
- **If everything matches**: the asset is skipped.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for upload strategy, processing pipeline, file structure, and API endpoints.

## License

Licensed under the [GNU General Public License v3.0](LICENSE).

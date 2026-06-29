import type { Metadata } from "next";

const GITHUB = "https://github.com/Majorfi/immich-exif";

const DOCS_TITLE = "Documentation — immich-exif";
const DOCS_DESCRIPTION =
  "Install, configure and run immich-exif: flags, the metadata tags it writes, export mode, the incremental cache, and how it keeps your photos safe.";

export const metadata: Metadata = {
  title: DOCS_TITLE,
  description: DOCS_DESCRIPTION,
  alternates: { canonical: "/docs" },
  openGraph: {
    title: DOCS_TITLE,
    description: DOCS_DESCRIPTION,
    type: "article",
  },
  twitter: {
    card: "summary_large_image",
    title: DOCS_TITLE,
    description: DOCS_DESCRIPTION,
  },
};

const TOC = [
  { id: "install", label: "Installation" },
  { id: "configure", label: "Configuration" },
  { id: "permissions", label: "API key permissions" },
  { id: "quick-start", label: "Quick start" },
  { id: "selecting", label: "Selecting assets" },
  { id: "flags", label: "Flags" },
  { id: "writes", label: "What it writes" },
  { id: "dates", label: "Dates & timezones" },
  { id: "export", label: "Export mode" },
  { id: "cache", label: "Incremental cache" },
  { id: "safety", label: "Safety" },
  { id: "ui", label: "Interactive mode" },
];

const FLAGS = [
  { flag: "-url", def: "$IMMICH_URL", desc: "Immich server URL." },
  {
    flag: "-api-key",
    def: "$IMMICH_API_KEY",
    desc: "API key from Account Settings → API Keys.",
  },
  {
    flag: "-immich-api",
    def: "auto",
    desc: "API contract: auto, legacy, or v3. auto detects the server version.",
  },
  {
    flag: "-dry-run",
    def: "false",
    desc: "Embed tags into a local copy and show the diff, but write nothing back.",
  },
  {
    flag: "-y",
    def: "false",
    desc: "Auto-confirm every change (no prompts). Required for parallel workers.",
  },
  {
    flag: "-workers",
    def: "1",
    desc: "Number of assets processed in parallel. Only applies with -y.",
  },
  {
    flag: "-album",
    def: "",
    desc: "Album ID to process. Repeatable, or all to cover every album.",
  },
  {
    flag: "-all",
    def: "false",
    desc: "Process the whole library. Same selector as -album all.",
  },
  {
    flag: "-export-dir",
    def: "",
    desc: "Write modified files to a directory instead of re-uploading.",
  },
  {
    flag: "-include-no-album",
    def: "true",
    desc: "In mirrored export, put assets with no album under no-album/.",
  },
  {
    flag: "-no-verify-upload",
    def: "false",
    desc: "Skip checksum verification; the original is moved to Immich trash instead of being permanently deleted.",
  },
  {
    flag: "-allow-http",
    def: "false",
    desc: "Allow a plaintext http:// server URL — the API key is sent in clear text.",
  },
  {
    flag: "-list-albums",
    def: "false",
    desc: "List your albums (ID and name) and exit.",
  },
  {
    flag: "-resolve-duplicate",
    def: "false",
    desc: "On a duplicate upload, move associations to the duplicate and delete the old asset.",
  },
  {
    flag: "-force",
    def: "false",
    desc: "Ignore the state cache and re-check every asset.",
  },
];

const PERMISSIONS = [
  { perm: "server.about", why: "Connectivity and server-version detection." },
  {
    perm: "asset.read",
    why: "Read asset metadata and page the library and albums.",
  },
  { perm: "asset.download", why: "Download the original file." },
  { perm: "asset.upload", why: "Re-upload the metadata-corrected file." },
  {
    perm: "asset.copy",
    why: "Copy associations (albums, favorites, …) to the new asset.",
  },
  {
    perm: "asset.update",
    why: "Restore visibility for archived or hidden assets.",
  },
  {
    perm: "asset.delete",
    why: "Remove the old original after a verified replacement.",
  },
  { perm: "album.read", why: "Resolve -album and -album all selections." },
];

const TAGS = [
  {
    cat: "GPS",
    tags: "GPSLatitude, GPSLatitudeRef, GPSLongitude, GPSLongitudeRef, XMP-exif:GPSLatitude, XMP-exif:GPSLongitude",
    note: "Ref derived from the coordinate sign; XMP uses signed values.",
  },
  {
    cat: "Description",
    tags: "ImageDescription, XPComment, XMP-dc:Description, IPTC:Caption-Abstract",
    note: "EXIF, Windows, XMP Dublin Core and IPTC at once.",
  },
  {
    cat: "Rating",
    tags: "Rating, RatingPercent, XMP-xmp:Rating",
    note: "Percent = rating x 20; skipped when the rating is 0.",
  },
  {
    cat: "Location",
    tags: "IPTC:City, XMP-photoshop:City, IPTC:Province-State, XMP-photoshop:State, IPTC:Country-PrimaryLocationName, XMP-photoshop:Country",
    note: "Dual IPTC and XMP-photoshop.",
  },
  {
    cat: "DateTime",
    tags: "DateTimeOriginal, OffsetTimeOriginal, TimeZoneOffset, XMP-exif:DateTimeOriginal, XMP-xmp:CreateDate",
    note: "XMP uses ISO 8601. See Dates & timezones below.",
  },
  {
    cat: "Camera",
    tags: "Make, Model, LensModel",
    note: "Only written when the file has no existing value.",
  },
];

function GitHubIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 16 16"
      fill="currentColor"
      className={className}
      aria-hidden
    >
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z" />
    </svg>
  );
}

function Code({ children }: { children: React.ReactNode }) {
  return (
    <pre className="card overflow-x-auto p-4 font-mono text-[12.5px] leading-relaxed text-fog">
      {children}
    </pre>
  );
}

function H2({ id, children }: { id: string; children: React.ReactNode }) {
  return (
    <h2 id={id} className="scroll-mt-8 text-2xl font-semibold tracking-tight">
      {children}
    </h2>
  );
}

export default function Docs() {
  return (
    <main>
      {/* Nav */}
      <nav className="mx-auto flex max-w-6xl items-center justify-between px-6 py-5">
        <a
          href="/"
          className="flex items-center gap-2 text-[15px] font-semibold"
        >
          <img src="/logo.svg" alt="" className="h-7 w-7" />
          <span className="font-mono">immich-exif</span>
        </a>
        <div className="flex items-center gap-2">
          <a
            href="/"
            className="btn-ghost hidden rounded-full px-4 py-1.5 text-[13px] font-medium sm:block"
          >
            Home
          </a>
          <a
            href={GITHUB}
            className="btn-primary flex items-center gap-2 rounded-full px-4 py-1.5 text-[13px] font-medium"
          >
            <GitHubIcon className="h-4 w-4" />
            GitHub
          </a>
        </div>
      </nav>

      <div className="mx-auto max-w-6xl px-6 pb-24 pt-6 lg:grid lg:grid-cols-[200px_1fr] lg:gap-12">
        {/* TOC */}
        <aside className="mb-10 lg:mb-0">
          <nav className="lg:sticky lg:top-8">
            <p className="mb-3 text-[12px] font-semibold uppercase tracking-[0.12em] text-fog-2">
              Documentation
            </p>
            <ul className="space-y-1.5 text-[13.5px]">
              {TOC.map((item) => (
                <li key={item.id}>
                  <a
                    href={`#${item.id}`}
                    className="text-fog transition-colors hover:text-ink"
                  >
                    {item.label}
                  </a>
                </li>
              ))}
            </ul>
          </nav>
        </aside>

        {/* Content */}
        <article className="max-w-2xl space-y-14 text-[14.5px] leading-relaxed text-fog">
          <header>
            <h1 className="text-4xl font-semibold tracking-tight text-ink">
              Documentation
            </h1>
            <p className="mt-3 text-[15px]">
              immich-exif is a command-line tool. Point it at your Immich
              server, pick what to process, and it writes the metadata Immich
              knows back into your files.
            </p>
          </header>

          <section className="space-y-4">
            <H2 id="install">Installation</H2>
            <p>
              You need{" "}
              <a
                href="https://exiftool.org"
                className="font-medium text-accent hover:underline"
              >
                exiftool
              </a>{" "}
              on your PATH, an Immich server, and an API key. To build from
              source you also need{" "}
              <a
                href="https://go.dev/dl/"
                className="font-medium text-accent hover:underline"
              >
                Go 1.24+
              </a>
              .
            </p>
            <Code>{`git clone https://github.com/Majorfi/immich-exif
cd immich-exif/src
go build -o immich-exif .`}</Code>
            <p>
              Or grab a prebuilt binary from the{" "}
              <a
                href={`${GITHUB}/releases`}
                className="font-medium text-accent hover:underline"
              >
                releases page
              </a>
              .
            </p>
          </section>

          <section className="space-y-4">
            <H2 id="configure">Configuration</H2>
            <p>
              Credentials come from flags, environment variables, or a{" "}
              <span className="kbd">.env</span> file in the working directory.
              The key is sent only to the server you configure.
            </p>
            <Code>{`# .env
IMMICH_URL=https://your-immich-server.com
IMMICH_API_KEY=your-api-key`}</Code>
          </section>

          <section className="space-y-4">
            <H2 id="permissions">API key permissions</H2>
            <p>
              On Immich 1.113+ you can scope the key to exactly what the tool
              needs (older servers issue all-or-nothing keys). A normal run that
              re-uploads and replaces assets uses:
            </p>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-[13px]">
                <thead>
                  <tr className="border-b border-line text-left text-ink">
                    <th className="py-2 pr-4 font-semibold">Permission</th>
                    <th className="py-2 font-semibold">Why</th>
                  </tr>
                </thead>
                <tbody>
                  {PERMISSIONS.map((p) => (
                    <tr key={p.perm} className="border-b border-line align-top">
                      <td className="py-2 pr-4 font-mono text-ink">{p.perm}</td>
                      <td className="py-2">{p.why}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <p>
              <span className="kbd">-dry-run</span> and{" "}
              <span className="kbd">-export-dir</span> never write to the
              server, so they only need{" "}
              <span className="kbd">server.about</span>,{" "}
              <span className="kbd">asset.read</span>,{" "}
              <span className="kbd">asset.download</span>, and{" "}
              <span className="kbd">album.read</span> (drop the last one if you
              pass asset IDs directly).
            </p>
          </section>

          <section className="space-y-4">
            <H2 id="quick-start">Quick start</H2>
            <p>
              Preview a single photo first. Nothing is written until you are
              happy with the diff.
            </p>
            <Code>{`# preview one asset, write nothing
immich-exif -dry-run <asset-id>

# then process the whole library, no prompts
immich-exif -y -all`}</Code>
          </section>

          <section className="space-y-4">
            <H2 id="selecting">Selecting assets</H2>
            <p>Every run needs exactly one selector:</p>
            <Code>{`immich-exif <asset-id> <asset-id>   # specific assets
immich-exif -album <album-id>       # one album
immich-exif -album <id1> -album <id2>  # several albums
immich-exif -album all              # every album
immich-exif -all                    # the whole library`}</Code>
            <p>
              <span className="kbd">-all</span> and{" "}
              <span className="kbd">-album all</span> are the same selector.
              Assets with no metadata to embed, and assets already in sync, are
              skipped either way.
            </p>
            <p>
              Need an album ID? List them with{" "}
              <span className="kbd">-list-albums</span> — it prints each
              album&apos;s ID, name and asset count, then exits.
            </p>
          </section>

          <section className="space-y-4">
            <H2 id="flags">Flags</H2>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-[13px]">
                <thead>
                  <tr className="border-b border-line text-left text-ink">
                    <th className="py-2 pr-4 font-semibold">Flag</th>
                    <th className="py-2 pr-4 font-semibold">Default</th>
                    <th className="py-2 font-semibold">Description</th>
                  </tr>
                </thead>
                <tbody>
                  {FLAGS.map((f) => (
                    <tr key={f.flag} className="border-b border-line align-top">
                      <td className="py-2 pr-4 font-mono text-ink">{f.flag}</td>
                      <td className="py-2 pr-4 font-mono text-fog-2">
                        {f.def || "—"}
                      </td>
                      <td className="py-2">{f.desc}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

          <section className="space-y-4">
            <H2 id="writes">What it writes</H2>
            <p>
              Images get the full set below. Supported videos (
              <span className="kbd">mp4</span>, <span className="kbd">mov</span>
              , <span className="kbd">m4v</span>) get a compatible subset:
              description, rating, GPS, location, dates and camera fields.
            </p>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-[12.5px]">
                <thead>
                  <tr className="border-b border-line text-left text-ink">
                    <th className="py-2 pr-4 font-semibold">Category</th>
                    <th className="py-2 pr-4 font-semibold">Tags</th>
                    <th className="py-2 font-semibold">Notes</th>
                  </tr>
                </thead>
                <tbody>
                  {TAGS.map((t) => (
                    <tr key={t.cat} className="border-b border-line align-top">
                      <td className="py-2 pr-4 font-semibold text-ink">
                        {t.cat}
                      </td>
                      <td className="py-2 pr-4 font-mono text-fog">{t.tags}</td>
                      <td className="py-2">{t.note}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

          <section className="space-y-4">
            <H2 id="dates">Dates & timezones</H2>
            <p>
              Immich returns ISO 8601 dates like{" "}
              <span className="kbd">2025-12-10T16:56:36+00:00</span>. EXIF
              stores local time plus a separate offset. immich-exif reconciles
              the two:
            </p>
            <ul className="space-y-2">
              {[
                "If the file has a date but no offset, the local time is kept and the offset is computed from Immich's UTC time, then written as OffsetTimeOriginal and TimeZoneOffset.",
                "If the file has no date at all, DateTimeOriginal is written in EXIF format with the offset tags.",
                "If everything already matches, the asset is skipped.",
              ].map((line) => (
                <li key={line} className="flex gap-3">
                  <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-accent" />
                  {line}
                </li>
              ))}
            </ul>
          </section>

          <section className="space-y-4">
            <H2 id="export">Export mode</H2>
            <p>
              With <span className="kbd">-export-dir</span>, modified files are
              written to disk instead of being re-uploaded. Nothing on the
              server is touched.
            </p>
            <ul className="space-y-2">
              {[
                "One -album: files go to /<export-dir>/<album-id>/.",
                "-all or -album all: assets are mirrored per album folder, shared assets appearing in each. Assets with no album go to no-album/ (disable with -include-no-album=false).",
                "copyFile refuses to overwrite an existing file, so an export never clobbers what's already there.",
              ].map((line) => (
                <li key={line} className="flex gap-3">
                  <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-accent" />
                  {line}
                </li>
              ))}
            </ul>
          </section>

          <section className="space-y-4">
            <H2 id="cache">Incremental cache</H2>
            <p>
              In <span className="kbd">-all</span> /{" "}
              <span className="kbd">-album all</span> runs, a local SQLite cache
              records which assets are done, so the next run skips anything
              Immich hasn&apos;t changed.
            </p>
            <ul className="space-y-2">
              {[
                "Stored at ~/.config/immich-exif/state.db (macOS: ~/Library/Application Support/), keyed per server URL.",
                "Only finalized outcomes are cached: migrated, replaced in-place, or confirmed already-matching.",
                "dry-run, export, and duplicate/replaced statuses are never cached.",
                "Use -force to re-check everything; delete state.db to reset.",
              ].map((line) => (
                <li key={line} className="flex gap-3">
                  <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-accent" />
                  {line}
                </li>
              ))}
            </ul>
          </section>

          <section className="space-y-4">
            <H2 id="safety">Safety</H2>
            <p>
              immich-exif re-uploads and deletes real assets, so the destructive
              path is the careful one.
            </p>
            <ul className="space-y-2">
              {[
                "The new asset is uploaded and its associations copied before the old one is removed. An interruption leaves a duplicate, never a hole.",
                "Checksum verification is on by default: the uploaded asset is re-fetched and its checksum compared to the local file, and downloads are verified the same way. A mismatch refuses to delete the original.",
                "When verification passes (the default), the original is permanently deleted, because the new copy is provably byte-identical. Pass -no-verify-upload to skip the check and the original is moved to Immich's trash instead, where it stays recoverable.",
                "By default a plaintext http:// server URL is rejected so the API key never travels in clear text; pass -allow-http to override that.",
                "-dry-run shows every change and writes nothing, so you can confirm exactly what will happen first.",
              ].map((line) => (
                <li key={line} className="flex gap-3">
                  <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-accent" />
                  {line}
                </li>
              ))}
            </ul>
          </section>

          <section className="space-y-4">
            <H2 id="ui">Interactive mode</H2>
            <p>
              immich-exif prints a diff per asset and waits for a single
              keypress:
            </p>
            <Code>{`[1/5] 2 EXIF mismatch found for IMG_1234.jpg:
    + OffsetTimeOriginal  (none)  -> +01:00
    ~ Rating              3       -> 5

[y] confirm  [s] skip  [q] quit:`}</Code>
            <p>
              No Enter needed. Interactive mode runs single-worker;{" "}
              <span className="kbd">-y</span> auto-confirms and enables parallel
              workers.
            </p>
          </section>

          <footer className="border-t border-line pt-8 text-[13px]">
            <a href="/" className="font-medium text-ink hover:text-accent">
              ← Back to home
            </a>
          </footer>
        </article>
      </div>
    </main>
  );
}

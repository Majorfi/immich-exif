import TerminalWindow from "@/components/TerminalWindow";
import RunWindow from "@/components/RunWindow";

const GITHUB = "https://github.com/Majorfi/immich-exif";
const SITE = "https://immich-exif.app";

const WRITES = [
  {
    name: "GPS & Places",
    accent: "#1e83f7",
    description:
      "Latitude and longitude with hemisphere refs, written to both EXIF and XMP so maps and other tools agree on where a shot was taken.",
  },
  {
    name: "Dates & Timezones",
    accent: "#ffb400",
    description:
      "DateTimeOriginal with the right offset: the exact moment your camera should have recorded, timezone included.",
  },
  {
    name: "Descriptions",
    accent: "#18c249",
    description:
      "Captions written across EXIF, XMP and IPTC at once, so every app that reads metadata shows the same text.",
  },
  {
    name: "Ratings",
    accent: "#ec4899",
    description:
      "Your stars, as Rating, RatingPercent and XMP-xmp:Rating. The photos you picked stay picked, anywhere they land.",
  },
  {
    name: "Camera & Lens",
    accent: "#8b5cf6",
    description:
      "Make, model and lens model put back on shots that lost them in an import or an edit round-trip.",
  },
  {
    name: "Video",
    accent: "#ef4444",
    description:
      "The same pipeline writes metadata into mp4, mov and m4v. Other containers are skipped, never mangled.",
  },
];

const SAFETY = [
  {
    title: "Upload before delete",
    description:
      "The corrected file is uploaded and its album, favorite and visibility links copied before the old asset is ever removed. An interruption leaves a duplicate, never a hole.",
  },
  {
    title: "Verify the bytes",
    description:
      "With -verify-upload, the new asset is re-fetched and its checksum compared to what you sent. A mismatch refuses to delete the original. A bad upload can't cost you the photo.",
  },
  {
    title: "Dry-run anything",
    description:
      "-dry-run shows every change as a diff and writes nothing. A local SQLite cache skips assets already in sync, so re-running over a library is cheap and safe.",
  },
];

const STEPS = [
  {
    step: "1",
    title: "Install it",
    description: (
      <>
        Grab a release binary or{" "}
        <span className="kbd">go install …/immich-exif@latest</span>. It shells
        out to <span className="kbd">exiftool</span>, so make sure that&apos;s
        on your PATH.
      </>
    ),
  },
  {
    step: "2",
    title: "Point it at your server",
    description: (
      <>
        Set <span className="kbd">IMMICH_URL</span> and an API key from{" "}
        <span className="kbd">Account Settings → API Keys</span>. The key lives
        in your <span className="kbd">.env</span>, sent only to your own server.
      </>
    ),
  },
  {
    step: "3",
    title: "Run it",
    description: (
      <>
        Start with{" "}
        <span className="kbd">immich-exif -dry-run &lt;asset-id&gt;</span> to
        preview a single photo, then <span className="kbd">-y -all</span> when
        the diff looks right.
      </>
    ),
  },
];

const FAQ = [
  {
    q: "Is it free?",
    a: "Yes. immich-exif is open source: build it yourself, go install it, or grab a release binary. No paywall.",
  },
  {
    q: "Does it touch my originals?",
    a: "It embeds the missing tags into the file and re-uploads the corrected copy to Immich. Assets that already carry the right metadata are detected by a content snapshot and skipped, so it's safe to run repeatedly.",
  },
  {
    q: "Can it lose my photos?",
    a: "No. The new asset is uploaded (and optionally checksum-verified with -verify-upload) before the old one is deleted, so the worst case is a duplicate, never a loss. The original is only removed once the corrected copy is confirmed live, and -dry-run lets you see every change first.",
  },
  {
    q: "What do I need?",
    a: "exiftool on your PATH, a running Immich server, and an API key. Go if you build from source, or just a prebuilt binary otherwise.",
  },
  {
    q: "Does it work with the latest Immich?",
    a: "Yes. The -immich-api flag auto-detects whether your server speaks the legacy or the 3.x API and adjusts the requests (album lookups, upload fields) accordingly.",
  },
  {
    q: "Is this an official Immich tool?",
    a: "No. It's an independent open-source companion that talks to Immich's public REST API. Immich is a trademark of its respective owners.",
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

function TagGlyph({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 16 16"
      fill="currentColor"
      className={className}
      aria-hidden
    >
      <path d="M7.6 1.5H3a1.5 1.5 0 0 0-1.5 1.5v4.6c0 .4.16.78.44 1.06l5.9 5.9a1.5 1.5 0 0 0 2.12 0l4.04-4.04a1.5 1.5 0 0 0 0-2.12l-5.9-5.9A1.5 1.5 0 0 0 7.6 1.5ZM4.75 6a1.25 1.25 0 1 1 0-2.5 1.25 1.25 0 0 1 0 2.5Z" />
    </svg>
  );
}

const jsonLd = {
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "SoftwareApplication",
      name: "immich-exif",
      applicationCategory: "UtilitiesApplication",
      operatingSystem: "macOS, Linux, Windows",
      url: SITE,
      description:
        "A CLI that writes the metadata an Immich server knows (GPS, dates, descriptions, ratings, camera info) back into the original photo and video files using exiftool.",
      isAccessibleForFree: true,
      offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
      author: {
        "@type": "Organization",
        name: "Quub",
        url: "https://quub.tech",
      },
      sameAs: [GITHUB],
    },
    {
      "@type": "FAQPage",
      mainEntity: FAQ.map((item) => ({
        "@type": "Question",
        name: item.q,
        acceptedAnswer: { "@type": "Answer", text: item.a },
      })),
    },
  ],
};

export default function Home() {
  return (
    <main>
      <script type="application/ld+json">{JSON.stringify(jsonLd)}</script>

      {/* Nav */}
      <nav className="mx-auto flex max-w-5xl items-center justify-between px-6 py-5">
        <p className="flex items-center gap-2 text-[15px] font-semibold">
          <img src="/logo.svg" alt="" className="h-7 w-7" />
          <span className="font-mono">immich-exif</span>
        </p>
        <div className="flex items-center gap-2">
          <a
            href="/docs"
            className="btn-ghost hidden rounded-full px-4 py-1.5 text-[13px] font-medium sm:block"
          >
            Docs
          </a>
          <a
            href={GITHUB}
            className="btn-ghost hidden items-center gap-2 rounded-full px-4 py-1.5 text-[13px] font-medium sm:flex"
          >
            <GitHubIcon className="h-4 w-4" />
            GitHub
          </a>
          <a
            href="#install"
            className="btn-primary rounded-full px-4 py-1.5 text-[13px] font-medium"
          >
            Install
          </a>
        </div>
      </nav>

      {/* Hero */}
      <header className="glow px-6 pt-14 pb-10 text-center sm:pt-20">
        <p className="mb-5 text-[13px] font-semibold uppercase tracking-[0.14em] text-accent">
          Open source · CLI · exiftool-powered
        </p>
        <h1 className="mx-auto max-w-3xl text-balance text-4xl font-semibold leading-[1.08] tracking-tight sm:text-6xl">
          Your metadata belongs in your files.
        </h1>
        <p className="mx-auto mt-6 max-w-xl text-pretty text-[17px] leading-relaxed text-fog">
          Your self-hosted{" "}
          <a
            href="https://immich.app"
            className="font-medium text-accent underline decoration-accent/30 underline-offset-4 hover:decoration-accent"
          >
            Immich
          </a>{" "}
          server knows where, when and what. The original files usually
          don&apos;t. immich-exif reads that metadata and writes it back into
          the photo itself, so it travels with the file.
        </p>
        <div className="mt-8 flex flex-col items-center gap-3">
          <div className="flex items-center justify-center gap-3">
            <a
              href="#install"
              className="btn-primary rounded-full px-6 py-2.5 text-[14px] font-medium"
            >
              Install
            </a>
            <a
              href="#how"
              className="btn-ghost rounded-full px-5 py-2.5 text-[14px] font-medium"
            >
              How it works
            </a>
          </div>
          <a
            href={GITHUB}
            className="flex items-center gap-1.5 text-[13px] font-medium text-fog hover:text-ink"
          >
            <GitHubIcon className="h-3.5 w-3.5" />
            or read the source
          </a>
        </div>

        <div className="mx-auto mt-14 max-w-2xl">
          <TerminalWindow />
        </div>

        <p className="mt-8 text-[12px] font-medium tracking-wide text-fog-2">
          EXIFTOOL-BACKED · DRY-RUN SAFE · CHECKSUM-VERIFIED DELETES · DUAL-API
          · PIPELINE 94% TESTED
        </p>
      </header>

      {/* What it writes */}
      <section className="bg-paper-2">
        <div className="mx-auto max-w-5xl px-6 py-24">
          <h2 className="text-center text-3xl font-semibold tracking-tight">
            Everything Immich knows, in the file
          </h2>
          <p className="mx-auto mt-3 max-w-xl text-center text-[15px] text-fog">
            Immich keeps all that metadata in its database. immich-exif diffs it
            against what&apos;s actually in each file and embeds only
            what&apos;s missing.
          </p>
          <div className="mt-12 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {WRITES.map((write) => (
              <article key={write.name} className="card p-5">
                <span
                  className="grid h-9 w-9 place-items-center rounded-[10px]"
                  style={{
                    background: `${write.accent}1f`,
                    color: write.accent,
                  }}
                >
                  <TagGlyph className="h-[17px] w-[17px]" />
                </span>
                <h3 className="mt-3.5 text-[15px] font-semibold">
                  {write.name}
                </h3>
                <p className="mt-1.5 text-[13.5px] leading-relaxed text-fog">
                  {write.description}
                </p>
              </article>
            ))}
          </div>
        </div>
      </section>

      {/* A file, not a database */}
      <section>
        <div className="mx-auto grid max-w-5xl items-center gap-10 px-6 py-24 lg:grid-cols-2">
          <div>
            <h2 className="text-3xl font-semibold tracking-tight">
              A file, not a database.
            </h2>
            <p className="mt-4 text-[15px] leading-relaxed text-fog">
              Metadata locked in a server is metadata you lose the day you
              export, migrate, or hand a photo to someone else. immich-exif
              bakes it into the file with{" "}
              <a
                href="https://exiftool.org"
                className="font-medium text-accent underline decoration-accent/30 underline-offset-4 hover:decoration-accent"
              >
                exiftool
              </a>
              , so the photo is self-describing, anywhere it travels.
            </p>
            <ul className="mt-6 space-y-3 text-[14px] text-fog">
              {[
                "Reads the tags already in the file, writes only what's missing",
                "Skips assets whose metadata already matches, so a re-run does nothing",
                "Downloads to a temp dir, never overwrites a file outside it",
              ].map((line) => (
                <li key={line} className="flex gap-3">
                  <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-accent" />
                  {line}
                </li>
              ))}
            </ul>
          </div>
          <div className="card p-6">
            <p className="font-mono text-[12.5px] leading-7 text-fog">
              <span className="text-fog-2"># per asset</span>
              <br />
              <span className="font-medium text-accent">fetch</span>
              {"   "}metadata from Immich
              <br />
              <span className="font-medium text-accent">diff</span>
              {"    "}Immich ⇄ the file&apos;s tags
              <br />
              <span className="font-medium text-accent">embed</span>
              {"   "}only the missing tags (exiftool)
              <br />
              <span className="font-medium text-accent">replace</span>{" "}
              re-upload, copy links, delete old
              <br />
              <span className="text-fog-2">skip</span>
              {"    "}
              <span className="text-fog-2">anything already in sync</span>
              <br />
              <br />
              <span className="font-medium text-[#18c249]">
                ✓ safe to run again and again
              </span>
            </p>
          </div>
        </div>
      </section>

      {/* Safety */}
      <section className="bg-paper-2">
        <div className="mx-auto max-w-5xl px-6 py-24">
          <h2 className="text-center text-3xl font-semibold tracking-tight">
            Built to never lose a photo
          </h2>
          <p className="mx-auto mt-3 max-w-xl text-center text-[15px] text-fog">
            It re-uploads and deletes real assets, so the destructive path is
            the careful path.
          </p>
          <div className="mt-12 grid gap-4 lg:grid-cols-3">
            {SAFETY.map((item) => (
              <article key={item.title} className="card p-6">
                <h3 className="text-[15px] font-semibold">{item.title}</h3>
                <p className="mt-2 text-[13.5px] leading-relaxed text-fog">
                  {item.description}
                </p>
              </article>
            ))}
          </div>
        </div>
      </section>

      {/* How it works */}
      <section id="how">
        <div className="mx-auto max-w-5xl px-6 py-24">
          <h2 className="text-center text-3xl font-semibold tracking-tight">
            Running in three steps
          </h2>
          <p className="mx-auto mt-3 max-w-xl text-center text-[15px] text-fog">
            Install, point it at your server, and preview before you commit.
          </p>
          <div
            id="install"
            className="mt-12 grid items-center gap-12 lg:grid-cols-2"
          >
            <RunWindow />
            <ol className="space-y-7">
              {STEPS.map((item) => (
                <li key={item.step} className="flex gap-4">
                  <span className="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-accent-soft text-[14px] font-bold text-accent">
                    {item.step}
                  </span>
                  <div>
                    <h3 className="text-[15px] font-semibold">{item.title}</h3>
                    <p className="mt-1 text-[14px] leading-relaxed text-fog">
                      {item.description}
                    </p>
                  </div>
                </li>
              ))}
            </ol>
          </div>
        </div>
      </section>

      {/* FAQ */}
      <section className="bg-paper-2">
        <div className="mx-auto max-w-5xl px-6 py-24">
          <h2 className="text-center text-3xl font-semibold tracking-tight">
            Fair questions
          </h2>
          <div className="mx-auto mt-12 grid max-w-4xl gap-x-12 sm:grid-cols-2">
            {FAQ.map((item) => (
              <details key={item.q} className="group border-t border-line py-5">
                <summary className="flex cursor-pointer list-none items-center justify-between gap-4 text-[15px] font-medium marker:hidden">
                  <span>{item.q}</span>
                  <span
                    aria-hidden
                    className="text-xl leading-none text-fog-2 transition-transform group-open:rotate-45"
                  >
                    +
                  </span>
                </summary>
                <p className="mt-3 text-[14px] leading-relaxed text-fog">
                  {item.a}
                </p>
              </details>
            ))}
          </div>
        </div>
      </section>

      {/* Final CTA */}
      <section className="glow px-6 pb-24 pt-24 text-center">
        <p className="text-[13px] font-semibold uppercase tracking-[0.14em] text-accent">
          Free &amp; open source
        </p>
        <h2 className="mx-auto mt-3 max-w-xl text-balance text-3xl font-semibold tracking-tight">
          Your metadata, everywhere your photos go.
        </h2>
        <p className="mx-auto mt-4 max-w-md text-[15px] leading-relaxed text-fog">
          Embed it once and the GPS, dates and captions ride along in the file.
          No database required to read them back.
        </p>
        <div className="mt-8 flex flex-col items-center gap-3">
          <a
            href="#install"
            className="btn-primary rounded-full px-6 py-3 text-[14px] font-medium"
          >
            Install immich-exif
          </a>
          <a
            href={GITHUB}
            className="flex items-center gap-1.5 text-[13px] font-medium text-fog hover:text-ink"
          >
            <GitHubIcon className="h-3.5 w-3.5" />
            or read the source on GitHub
          </a>
        </div>
      </section>

      {/* Footer */}
      <footer className="bg-paper-2">
        <div className="mx-auto max-w-5xl px-6 py-10">
          <p className="flex items-center gap-2 text-[15px] font-semibold">
            <img src="/logo.svg" alt="" className="h-7 w-7" />
            <span className="font-mono">immich-exif</span>
          </p>
          <div className="mt-5 flex flex-col justify-between gap-3 text-[12.5px] text-fog sm:flex-row">
            <p>
              Built by{" "}
              <a
                href="https://quub.tech"
                className="font-medium text-ink hover:text-accent"
              >
                Quub
              </a>
              . Also try{" "}
              <a
                href="https://findich.app"
                className="font-medium text-ink hover:text-accent"
              >
                Findich
              </a>
              .
            </p>
            <p>
              Not affiliated with{" "}
              <a
                href="https://immich.app"
                className="font-medium text-ink hover:text-accent"
              >
                Immich
              </a>
              .
            </p>
          </div>
        </div>
      </footer>
    </main>
  );
}

# immich-exif — landing site

Marketing site for [immich-exif](https://github.com/Majorfi/immich-exif) — a CLI
that writes the metadata your Immich server knows (GPS, dates, descriptions,
ratings) back into the original files. Same brand design as
[Findich](https://findich.app), in the spirit of
[getwhimbrel.app](https://getwhimbrel.app). Next.js 15 (App Router) + Tailwind
CSS v4, static one-pager, deployed on Vercel (root directory: `site/`).

```bash
npm install
npm run dev    # http://localhost:3000
npm run build
```

The terminal windows in the hero and the "how it works" section are pure
CSS/markup — no screenshots to keep in sync.

Before going live, set the real domain in `app/layout.tsx` (`metadataBase`),
`app/robots.ts` and `app/sitemap.ts` (currently `https://immich-exif.app`).

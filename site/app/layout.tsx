import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { Analytics } from "@vercel/analytics/next";
import "./globals.css";

const geistSans = Geist({
  subsets: ["latin"],
  variable: "--font-geist-sans",
  display: "swap",
});

const geistMono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-geist-mono",
  display: "swap",
});

export const metadata: Metadata = {
  metadataBase: new URL("https://immich-exif.app"),
  title: "immich-exif: write your Immich metadata back into your files",
  description:
    "A CLI that takes the GPS, dates, descriptions and ratings your self-hosted Immich server knows and embeds them into the original photos and videos with exiftool. Dry-run by default-safe, checksum-verified, open source.",
  alternates: { canonical: "/" },
  keywords: [
    "Immich",
    "exiftool",
    "EXIF metadata",
    "self-hosted photos",
    "Immich CLI",
    "embed GPS metadata",
    "Immich metadata sync",
  ],
  openGraph: {
    title: "immich-exif: metadata back in your files",
    description:
      "Immich knows where, when and what. immich-exif writes that metadata into the file itself, so it travels with the photo.",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "immich-exif: metadata back in your files",
    description:
      "Write the GPS, dates and descriptions Immich knows back into your original files.",
  },
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className={`${geistSans.variable} ${geistMono.variable}`}>
      <body>
        {children}
        <Analytics />
      </body>
    </html>
  );
}

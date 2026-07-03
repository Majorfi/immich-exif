import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { Analytics } from "@vercel/analytics/next";
import { SITE_URL } from "@/lib/site";
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
  metadataBase: new URL(SITE_URL),
  title: "Immich Exif: write your Immich metadata back into your files",
  description:
    "Write the GPS, dates, descriptions and ratings your self-hosted Immich server knows back into your original photos and videos with exiftool. Open source.",
  alternates: { canonical: "/" },
  openGraph: {
    title: "Immich Exif: metadata back in your files",
    description:
      "Immich knows where, when and what. Immich Exif writes that metadata into the file itself, so it travels with the photo.",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "Immich Exif: metadata back in your files",
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

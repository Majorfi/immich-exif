import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: { userAgent: "*", allow: "/" },
    sitemap: "https://immich-exif.app/sitemap.xml",
    host: "https://immich-exif.app",
  };
}

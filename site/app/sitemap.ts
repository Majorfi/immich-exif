import type { MetadataRoute } from "next";

export default function sitemap(): MetadataRoute.Sitemap {
  return [
    {
      url: "https://immich-exif.app",
      changeFrequency: "monthly",
      priority: 1,
    },
    {
      url: "https://immich-exif.app/docs",
      changeFrequency: "monthly",
      priority: 0.8,
    },
  ];
}

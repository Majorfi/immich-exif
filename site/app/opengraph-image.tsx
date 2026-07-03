import { ImageResponse } from "next/og";

export const alt =
  "Immich Exif: write your Immich metadata back into your files";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

export default function Image() {
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        background: "#ffffff",
        fontFamily: "sans-serif",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          width: 132,
          height: 132,
          borderRadius: 34,
          background: "linear-gradient(180deg, #1D77F2, #18C1FA)",
        }}
      >
        <div
          style={{
            display: "flex",
            width: 78,
            height: 60,
            borderRadius: 12,
            background: "#ffffff",
          }}
        />
      </div>
      <div
        style={{
          marginTop: 44,
          fontSize: 70,
          fontWeight: 700,
          letterSpacing: -2,
          color: "#0a0a0a",
        }}
      >
        Immich Exif
      </div>
      <div
        style={{
          marginTop: 14,
          fontSize: 32,
          color: "#525252",
        }}
      >
        Your metadata belongs in your files.
      </div>
      <div
        style={{
          marginTop: 40,
          fontSize: 19,
          letterSpacing: 2,
          textTransform: "uppercase",
          color: "#1e83f7",
          fontWeight: 600,
        }}
      >
        Open source · exiftool-powered
      </div>
    </div>,
    { ...size },
  );
}

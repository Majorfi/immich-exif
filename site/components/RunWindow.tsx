type Line = {
  prompt?: boolean;
  text: string;
  comment?: string;
  accent?: string;
};

const LINES: Line[] = [
  { prompt: true, text: "go install github.com/Majorfi/immich-exif@latest" },
  { prompt: true, text: "export IMMICH_URL=https://photos.example.com" },
  { prompt: true, text: "export IMMICH_API_KEY=••••••••••••" },
  {
    prompt: true,
    text: "immich-exif -dry-run <asset-id>",
    comment: "preview one, write nothing",
  },
  {
    prompt: true,
    text: "immich-exif -y -all",
    comment: "embed across the whole library",
  },
];

export default function RunWindow() {
  return (
    <div className="terminal hairline overflow-hidden text-left">
      <div className="terminal-bar flex items-center gap-2 px-4 py-3">
        <span className="h-3 w-3 rounded-full bg-[#ff5f57]" />
        <span className="h-3 w-3 rounded-full bg-[#febc2e]" />
        <span className="h-3 w-3 rounded-full bg-[#28c840]" />
        <span className="ml-2 font-mono text-[12px] text-white/40">
          immich-exif — getting started
        </span>
      </div>

      <div className="overflow-x-auto px-5 py-4 font-mono text-[12.5px] leading-[1.9]">
        {LINES.map((line) => (
          <p key={line.text} className="whitespace-nowrap">
            {line.prompt && <span className="text-[#28c840]">$ </span>}
            <span className="text-white/85">{line.text}</span>
            {line.comment && (
              <span className="text-white/30">{"  # " + line.comment}</span>
            )}
          </p>
        ))}
      </div>
    </div>
  );
}

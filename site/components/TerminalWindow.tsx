type DiffRow = {
  symbol: "+" | "~";
  tag: string;
  old: string;
  next: string;
};

const ROWS: DiffRow[] = [
  { symbol: "+", tag: "GPSLatitude", old: "(none)", next: "48.8566" },
  { symbol: "+", tag: "GPSLongitude", old: "(none)", next: "2.3522" },
  {
    symbol: "+",
    tag: "ImageDescription",
    old: "(none)",
    next: "Dinner in Paris",
  },
  {
    symbol: "~",
    tag: "DateTimeOriginal",
    old: "2018:05:23",
    next: "…23 18:04:32+02:00",
  },
  { symbol: "+", tag: "Rating", old: "(none)", next: "★★★★★" },
  { symbol: "+", tag: "Make", old: "(none)", next: "Canon" },
];

const SYMBOL_COLOR = { "+": "#18c249", "~": "#ffb400" } as const;

export default function TerminalWindow() {
  return (
    <div className="terminal hairline overflow-hidden text-left">
      {/* Title bar */}
      <div className="terminal-bar flex items-center gap-2 px-4 py-3">
        <span className="h-3 w-3 rounded-full bg-[#ff5f57]" />
        <span className="h-3 w-3 rounded-full bg-[#febc2e]" />
        <span className="h-3 w-3 rounded-full bg-[#28c840]" />
        <span className="ml-2 font-mono text-[12px] text-white/40">
          immich-exif
        </span>
      </div>

      {/* Body */}
      <div className="overflow-x-auto px-5 py-4 font-mono text-[12.5px] leading-[1.7]">
        <p className="whitespace-nowrap text-white/55">
          <span className="text-[#28c840]">$</span> immich-exif{" "}
          <span className="text-[#1e9eff]">-y</span> 60566630-…
        </p>
        <p className="mt-2 whitespace-nowrap text-white/85">
          <span className="text-white/40">=&gt;</span> 60566630 |{" "}
          IMG_20180523_200432.jpg
        </p>
        <p className="whitespace-nowrap text-white/55">
          {"   "}embedding 12 missing tags into the file:
        </p>
        {ROWS.map((row) => (
          <p key={row.tag} className="whitespace-nowrap">
            <span style={{ color: SYMBOL_COLOR[row.symbol] }}>
              {"   "}
              {row.symbol}
            </span>{" "}
            <span className="text-white/80">{row.tag.padEnd(18, " ")}</span>
            <span className="text-white/35">{row.old.padEnd(13, " ")}</span>
            <span className="text-white/30">→ </span>
            <span className="text-white">{row.next}</span>
          </p>
        ))}
        <p className="mt-2 whitespace-nowrap">
          <span className="text-[#28c840]">{"   "}✓</span>{" "}
          <span className="text-white/70">
            embedded · re-uploaded · verified
          </span>
        </p>
      </div>
    </div>
  );
}

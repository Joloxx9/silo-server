import { cn } from "@/lib/utils";

type MarkSize = "sm" | "md";

/**
 * IMDb's mark drawn as text: black "IMDb" on its yellow badge. Drawn here
 * rather than shipped as IMDb's logo file.
 */
export function ImdbMark({ size, label }: { size: MarkSize; label: string }) {
  return (
    <span
      role="img"
      aria-label={label}
      className={cn(
        "inline-flex shrink-0 items-center rounded-[3px] bg-[#F5C518] leading-none font-black tracking-tight text-black",
        size === "sm" ? "h-3.5 px-1 text-[0.625rem]" : "h-4 px-1.5 text-[0.6875rem]",
      )}
    >
      IMDb
    </span>
  );
}

/** A tomato for a fresh critics score, a green splat for a rotten one (under 60%). */
export function TomatoMark({
  size,
  score,
  label,
}: {
  size: MarkSize;
  score: number;
  label: string;
}) {
  const cls = cn("w-auto shrink-0", size === "sm" ? "h-3.5" : "h-4");
  if (score < 60) {
    return (
      <svg viewBox="0 0 16 16" role="img" aria-label={label} className={cls}>
        <path
          fill="#0AC855"
          d="M8 1.2l1.3 3.1 2.9-1.8-.6 3.3 3.3.5-2.6 2.1 2.2 2.5-3.3.1.2 3.3L8.6 12 7 14.9 6.1 11.6l-3.1 1.3 1.4-3L1.1 9l2.9-1.6L2 4.7l3.3.3L5.6 1.7 7.5 4z"
        />
      </svg>
    );
  }
  return (
    <svg viewBox="0 0 16 16" role="img" aria-label={label} className={cls}>
      <circle cx="8" cy="9.3" r="6.2" fill="#FA320A" />
      <ellipse cx="5.8" cy="7.4" rx="1.6" ry="1" fill="#fff" opacity=".35" />
      <path
        fill="#0AC855"
        d="M8 3.6c-.6-1-1.7-1.6-3-1.5 1 .4 1.6.9 1.9 1.6-1.1-.2-2.2.1-3 .9 1.2-.1 2.3.1 3.1.6-.5.4-.8 1-.9 1.7.7-.6 1.4-.9 2-1 .4.6.6 1.3.6 2 .4-.8.6-1.5.5-2.2.8 0 1.7.3 2.4.9-.3-.8-.9-1.3-1.6-1.6.7-.6 1.6-.9 2.6-.9-.9-.6-2-.7-3-.4.2-.7.1-1.4-.3-2-.1.7-.6 1.4-1.3 1.9z"
      />
    </svg>
  );
}

/** A popcorn bucket for an audience score. */
export function PopcornMark({ size, label }: { size: MarkSize; label: string }) {
  return (
    <svg
      viewBox="0 0 16 16"
      role="img"
      aria-label={label}
      className={cn("w-auto shrink-0", size === "sm" ? "h-3.5" : "h-4")}
    >
      <g fill="#FFE9A8">
        <circle cx="5" cy="5" r="2.2" />
        <circle cx="8" cy="3.8" r="2.4" />
        <circle cx="11" cy="5" r="2.2" />
      </g>
      <path fill="#FA320A" d="M3 6.4h10l-1.3 8.4H4.3z" />
      <path fill="#fff" d="M5.6 6.4h1.4l.3 8.4H6.1zM9 6.4h1.4l-.5 8.4H8.7z" />
    </svg>
  );
}

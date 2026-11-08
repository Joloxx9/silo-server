// A separate file rather than an inlined data URI, so the logo stays out of
// the launch bundle.
import tmdbLogo from "@/assets/tmdb/tmdb-logo.svg?no-inline";
import { cn } from "@/lib/utils";

import { ImdbMark, PopcornMark, TomatoMark } from "./RatingMarks";
import type { DisplayRating } from "./ratings";

interface RatingEntryProps {
  rating: DisplayRating;
  size?: "sm" | "md";
  className?: string;
}

/**
 * A rating as its source's mark and its score. TMDB uses its approved logo;
 * IMDb and Rotten Tomatoes use marks drawn in RatingMarks; any other source
 * falls back to its plain-text name.
 */
export function RatingEntry({ rating, size = "md", className }: RatingEntryProps) {
  return (
    <span className={cn("inline-flex items-center gap-1.5 whitespace-nowrap", className)}>
      {rating.source === "tmdb" ? (
        <img
          src={tmdbLogo}
          alt={rating.name}
          className={cn("w-auto shrink-0", size === "sm" ? "h-2" : "h-2.5")}
        />
      ) : rating.source === "imdb" ? (
        <ImdbMark size={size} label={rating.name} />
      ) : rating.source === "rt_critic" ? (
        <TomatoMark size={size} score={rating.score} label={rating.name} />
      ) : rating.source === "rt_audience" ? (
        <PopcornMark size={size} label={rating.name} />
      ) : (
        <span
          className={cn(
            "font-semibold tracking-tight opacity-75",
            size === "sm" ? "text-xs" : "text-[0.8125rem]",
          )}
        >
          {rating.name}
        </span>
      )}{" "}
      <span
        className={cn("font-bold tabular-nums", size === "sm" ? "text-sm" : "text-[0.9375rem]")}
      >
        {rating.display}
      </span>
    </span>
  );
}

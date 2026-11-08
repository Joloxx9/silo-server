// A separate file rather than an inlined data URI, so the logo stays out of
// the launch bundle.
import { useState } from "react";

import tmdbLogo from "@/assets/tmdb/tmdb-logo.svg?no-inline";
import { cn } from "@/lib/utils";

import type { DisplayRating } from "./ratings";

interface RatingEntryProps {
  rating: DisplayRating;
  size?: "sm" | "md";
  className?: string;
}

/**
 * A rating as its source's mark and its score: "IMDb 8.5", "RT 93%". Marks
 * are plain text, never logo artwork Silo ships, except TMDB's approved logo,
 * which TMDB's terms allow. An image the administrator assigned to the source
 * (`icon_url`) takes the mark's place; one that fails to load falls back to
 * the text mark.
 */
export function RatingEntry({ rating, size = "md", className }: RatingEntryProps) {
  const [failedIcon, setFailedIcon] = useState<string | null>(null);
  const icon = rating.icon_url && rating.icon_url !== failedIcon ? rating.icon_url : null;
  return (
    <span className={cn("inline-flex items-center gap-1.5 whitespace-nowrap", className)}>
      {icon ? (
        <img
          src={icon}
          alt={rating.name}
          onError={() => setFailedIcon(icon)}
          className={cn(
            "w-auto shrink-0 object-contain",
            size === "sm" ? "h-3.5 max-w-14" : "h-4 max-w-16",
          )}
        />
      ) : rating.source === "tmdb" ? (
        <img
          src={tmdbLogo}
          alt={rating.name}
          className={cn("w-auto shrink-0", size === "sm" ? "h-2" : "h-2.5")}
        />
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

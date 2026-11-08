import type { RequestPersonCredit } from "@/api/types";

export type FilmographyFilter = "all" | "library" | "missing" | "movie" | "series";

export const FILMOGRAPHY_FILTERS: { id: FilmographyFilter; label: string }[] = [
  { id: "all", label: "Everything" },
  { id: "library", label: "In library" },
  { id: "missing", label: "Not in library" },
  { id: "movie", label: "Movies" },
  { id: "series", label: "TV" },
];

/** In the library: the server matched it to a catalog item. */
export function creditInLibrary(credit: RequestPersonCredit): boolean {
  return credit.media.availability === "available" || Boolean(credit.media.library_content_id);
}

/** The credits a filter shows; undated (announced) titles only when asked. */
export function filterCredits(
  credits: RequestPersonCredit[],
  filter: FilmographyFilter,
  showUndated: boolean,
): RequestPersonCredit[] {
  return credits.filter((credit) => {
    if (!showUndated && !credit.media.release_date) return false;
    switch (filter) {
      case "library":
        return creditInLibrary(credit);
      case "missing":
        return !creditInLibrary(credit);
      case "movie":
      case "series":
        return credit.media.media_type === filter;
      default:
        return true;
    }
  });
}

/** "as Geralt of Rivia · 24 episodes", "Director, Writer". */
export function creditRole(credit: RequestPersonCredit): string {
  const parts: string[] = [];
  if (credit.character) parts.push(`as ${credit.character}`);
  if (credit.jobs.length) parts.push(credit.jobs.join(", "));
  if (credit.episode_count) {
    parts.push(`${credit.episode_count} episode${credit.episode_count === 1 ? "" : "s"}`);
  }
  return parts.join(" · ");
}

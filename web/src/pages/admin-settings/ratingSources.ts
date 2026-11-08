import type { QueryClient } from "@tanstack/react-query";

import {
  RATING_POLICY_SETTLE_MS,
  refreshRatingChoice,
} from "@/hooks/queries/ratingsSurfaceRefresh";

export { RATING_POLICY_SETTLE_MS };

export const EXTRA_RATING_SOURCES_KEY = "catalog.extra_rating_sources";
export const RATING_SOURCE_ICONS_KEY = "catalog.rating_source_icons";

/** The images assigned to one rating source, by overlay image ref. */
export interface RatingSourceIcon {
  image?: string;
  low_image?: string;
}

/** Reads the icon assignments; an unreadable value assigns none. */
export function parseRatingIcons(value: string | undefined): Record<string, RatingSourceIcon> {
  if (!value) return {};
  try {
    const parsed: unknown = JSON.parse(value);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    const out: Record<string, RatingSourceIcon> = {};
    for (const [source, entry] of Object.entries(parsed as Record<string, unknown>)) {
      if (!entry || typeof entry !== "object") continue;
      const { image, low_image } = entry as Record<string, unknown>;
      out[source] = {
        ...(typeof image === "string" && image ? { image } : {}),
        ...(typeof low_image === "string" && low_image ? { low_image } : {}),
      };
    }
    return out;
  } catch {
    return {};
  }
}

/**
 * Sets or clears one of a source's images, keeping every other assignment. A
 * source left with no image is dropped; no assignments store as empty.
 */
export function setRatingIcon(
  value: string | undefined,
  source: string,
  slot: keyof RatingSourceIcon,
  ref: string | undefined,
): string {
  const icons = parseRatingIcons(value);
  const entry: RatingSourceIcon = { ...icons[source] };
  if (ref) entry[slot] = ref;
  else delete entry[slot];
  if (entry.image || entry.low_image) icons[source] = entry;
  else delete icons[source];
  return Object.keys(icons).length > 0 ? JSON.stringify(icons) : "";
}

export function parseRatingSources(value: string | undefined): string[] {
  return (value ?? "")
    .split(",")
    .map((entry) => entry.trim())
    .filter(Boolean);
}

/**
 * Turns one source on or off, keeping every other entry, including sources a
 * plugin declared that this page does not list.
 */
export function toggleRatingSource(value: string | undefined, source: string, on: boolean): string {
  const rest = parseRatingSources(value).filter((entry) => entry !== source);
  return (on ? [...rest, source] : rest).join(",");
}

interface RatingSourceEntry {
  source: string;
  label: string;
  always_shown: boolean;
  provider?: string;
}

/**
 * The ratings an administrator can turn on, grouped by the metadata plugin
 * that declares them, in the order the server lists them. IMDb and TMDB are
 * always shown, so they are left out.
 */
export function groupRatingSourcesByPlugin<T extends RatingSourceEntry>(
  sources: readonly T[],
): { provider: string; sources: T[] }[] {
  const groups = new Map<string, T[]>();
  for (const source of sources) {
    if (source.always_shown || !source.provider) continue;
    const group = groups.get(source.provider) ?? [];
    group.push(source);
    groups.set(source.provider, group);
  }
  return Array.from(groups, ([provider, entries]) => ({ provider, sources: entries }));
}

/**
 * The sources the setting turns on that no listed source declares, such as
 * one whose plugin was disabled, in setting order. They show nothing until a
 * plugin declares them again, and are listed so an administrator can turn
 * them off.
 */
export function undeclaredRatingSources(
  turnedOn: readonly string[],
  sources: readonly RatingSourceEntry[],
): string[] {
  const listed = new Set(sources.map((entry) => entry.source));
  return Array.from(new Set(turnedOn)).filter((source) => !listed.has(source));
}

/**
 * Saves the settings form and, when the rating choice was among the changes,
 * refreshes every cached rating surface (title pages, cards, the ratings
 * capability) now and again once the server's cached copy has expired.
 */
export async function saveAndRefreshRatings(
  form: { isDirty: (key: string) => boolean; save: () => Promise<void> },
  queryClient: QueryClient,
  schedule?: (run: () => void, ms: number) => unknown,
): Promise<void> {
  const ratingsChanged =
    form.isDirty(EXTRA_RATING_SOURCES_KEY) || form.isDirty(RATING_SOURCE_ICONS_KEY);
  await form.save();
  if (ratingsChanged) refreshRatingChoice(queryClient, schedule);
}

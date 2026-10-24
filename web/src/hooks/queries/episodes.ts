import {
  useCatalogItemEpisodes,
  useCatalogSeasonDetail,
  useCatalogSeasonEpisodes,
  useCatalogSeriesSeasons,
  type MissingEpisodes,
} from "./catalogRead";

// libraryId names the library the page was opened from; see
// "Library-scoped version lists" in docs/catalog-api.md.

export function useSeasons(
  seriesId: string | undefined,
  libraryId?: number,
  missing: MissingEpisodes = "none",
) {
  return useCatalogSeriesSeasons(seriesId, libraryId, missing);
}

export function useSeasonEpisodes(
  seriesId: string | undefined,
  seasonNum: number,
  libraryId?: number,
) {
  return useCatalogSeasonEpisodes(seriesId, seasonNum, libraryId);
}

export function useItemEpisodes(
  itemId: string | undefined,
  libraryId?: number,
  missing: MissingEpisodes = "none",
) {
  return useCatalogItemEpisodes(itemId, libraryId, missing);
}

export function useSeasonDetail(
  seriesId: string | undefined,
  seasonNum: number,
  libraryId?: number,
) {
  return useCatalogSeasonDetail(seriesId, seasonNum, libraryId);
}

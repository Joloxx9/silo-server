import {
  useCatalogItemEpisodes,
  useCatalogSeasonDetail,
  useCatalogSeasonEpisodes,
  useCatalogSeriesSeasons,
} from "./catalogRead";

export function useSeasons(seriesId: string | undefined, includeMissing = false) {
  return useCatalogSeriesSeasons(seriesId, undefined, includeMissing);
}

export function useSeasonEpisodes(seriesId: string | undefined, seasonNum: number) {
  return useCatalogSeasonEpisodes(seriesId, seasonNum);
}

export function useItemEpisodes(itemId: string | undefined, includeMissing = false) {
  return useCatalogItemEpisodes(itemId, undefined, includeMissing);
}

export function useSeasonDetail(seriesId: string | undefined, seasonNum: number) {
  return useCatalogSeasonDetail(seriesId, seasonNum);
}

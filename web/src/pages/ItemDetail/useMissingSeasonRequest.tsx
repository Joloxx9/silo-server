import { useMemo, useState, type ReactNode } from "react";
import { RequestSeasonsDialog } from "@/components/RequestSeasonsDialog";
import { useRequestMediaDetail } from "@/hooks/queries/useRequests";
import { useMissingSeasonsRequestable } from "@/hooks/useCanRequest";
import { useCurrentProfile } from "@/hooks/useCurrentProfile";

/**
 * Requests one season of a series from a season or episode the library lacks,
 * through the same dialog the series menu opens, with that season picked.
 * onRequestSeason is absent when this profile cannot request seasons.
 * requestedSeasons names the seasons a request already covers, read only
 * while active, so a page that shows nothing missing asks for nothing.
 */
export function useMissingSeasonRequest(
  tmdbID: number,
  title: string,
  active: boolean,
): {
  onRequestSeason?: (seasonNumber: number) => void;
  requestedSeasons: ReadonlySet<number>;
  dialog: ReactNode;
} {
  const { profile } = useCurrentProfile();
  const canRequest = useMissingSeasonsRequestable(Boolean(profile?.id) && tmdbID > 0);
  // The dialog reads the same query, and a new request refreshes it.
  const detail = useRequestMediaDetail("series", tmdbID, { enabled: canRequest && active });
  const requestedSeasons = useMemo(
    () =>
      new Set(
        (detail.data?.seasons ?? [])
          .filter((season) => season.requested)
          .map((season) => season.season_number),
      ),
    [detail.data?.seasons],
  );
  const [season, setSeason] = useState<number | null>(null);
  // The dialog keeps the last season while it animates closed.
  const [lastSeason, setLastSeason] = useState<number | null>(null);

  if (!canRequest) return { requestedSeasons, dialog: null };
  return {
    requestedSeasons,
    onRequestSeason: (seasonNumber) => {
      setSeason(seasonNumber);
      setLastSeason(seasonNumber);
    },
    dialog: (
      <RequestSeasonsDialog
        open={season !== null}
        onOpenChange={(open) => {
          if (!open) setSeason(null);
        }}
        tmdbID={tmdbID}
        title={title}
        initialSeasons={lastSeason === null ? undefined : [lastSeason]}
      />
    ),
  };
}

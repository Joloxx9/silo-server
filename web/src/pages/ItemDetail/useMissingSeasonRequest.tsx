import { useState, type ReactNode } from "react";
import { RequestSeasonsDialog } from "@/components/RequestSeasonsDialog";
import { useMissingSeasonsRequestable } from "@/hooks/useCanRequest";
import { useCurrentProfile } from "@/hooks/useCurrentProfile";

/**
 * Requests one season of a series from a season or episode the library lacks,
 * through the same dialog the series menu opens, with that season picked.
 * onRequestSeason is absent when this profile cannot request seasons.
 */
export function useMissingSeasonRequest(
  tmdbID: number,
  title: string,
): { onRequestSeason?: (seasonNumber: number) => void; dialog: ReactNode } {
  const { profile } = useCurrentProfile();
  const canRequest = useMissingSeasonsRequestable(Boolean(profile?.id) && tmdbID > 0);
  const [season, setSeason] = useState<number | null>(null);
  // The dialog keeps the last season while it animates closed.
  const [lastSeason, setLastSeason] = useState<number | null>(null);

  if (!canRequest) return { dialog: null };
  return {
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

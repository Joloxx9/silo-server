import { useCallback } from "react";
import type { ItemDetail } from "@/api/types";
import { useWatchedStateMutation } from "@/hooks/queries/items";
import { useRatingChange } from "@/hooks/queries/ratings";
import { getWatchedActionLabel } from "../watchedState";
import ActionBar, { type ActionBarProps } from "./ActionBar";

type WatchedActionProps =
  | "isWatched"
  | "watchedLabel"
  | "onToggleWatched"
  | "isUpdatingWatched"
  | "rating"
  | "onRatingChange";

interface WatchedActionBarProps extends Omit<ActionBarProps, WatchedActionProps> {
  item: ItemDetail;
}

/**
 * Keeps mutation lifecycle renders inside the season and episode action bar.
 *
 * A season and an episode are rated in the same place as a movie, because a
 * viewer who has just finished one looks for the stars where the other title
 * kinds put them. Favorites and the watchlist stay out: those lists hold
 * whole titles, so they remain on the movie and series bar.
 */
export default function WatchedActionBar({ item, ...props }: WatchedActionBarProps) {
  const { mutate: toggleWatched, isPending: isUpdatingWatched } = useWatchedStateMutation(item);
  const handleRatingChange = useRatingChange(item.content_id);
  const handleToggleWatched = useCallback(
    () => toggleWatched(!(item.user_data?.played ?? false)),
    [item.user_data?.played, toggleWatched],
  );

  return (
    <ActionBar
      {...props}
      watchedLabel={getWatchedActionLabel(item)}
      isWatched={item.user_data?.played ?? false}
      onToggleWatched={handleToggleWatched}
      isUpdatingWatched={isUpdatingWatched}
      rating={item.user_rating ?? null}
      onRatingChange={handleRatingChange}
    />
  );
}

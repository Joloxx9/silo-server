import { useRef } from "react";
import { Play } from "lucide-react";
import type { EpisodeListItem } from "@/api/types";
import { WatchedCheckIndicator } from "@/components/CardWatchedBadge";
import { toEpisodeUserState } from "@/components/episodeUserState";
import MediaItemMenu from "@/components/MediaItemMenu";
import CardOverlays from "@/components/overlays/CardOverlays";
import ViewTransitionLink from "@/components/ViewTransitionLink";
import { useOverlayPrefs } from "@/hooks/useOverlayPrefs";
import { usePrefetchCatalogItemDetail } from "@/hooks/queries/catalogRead";
import { useDwellPrefetch } from "@/hooks/useDwellPrefetch";
import { useGridRowCap } from "@/hooks/useGridRowCap";
import type { CardQuickActionMode } from "@/lib/cardQuickActions";
import { formatDate } from "@/lib/datetime";
import { overlayDataFromEpisodeListItem, type CardOverlayPrefs } from "@/lib/overlays";
import { cn } from "@/lib/utils";
import { EpisodeGridSkeleton } from "./SectionSkeletons";
import type { EpisodeNavigationState } from "../itemDetailLayout";

/**
 * How much of a season stays visible before the section scrolls, so a long one
 * cannot push the cast and crew off the page.
 *
 * Flat across breakpoints rather than scaled by column count: the cap is
 * really a height budget, and four rows is already about one screen tall on a
 * phone. Trading rows for columns there would only produce a nested scroll
 * region taller than the viewport it sits in.
 */
const VISIBLE_EPISODE_ROWS = 4;

interface SeasonEpisodeGridProps {
  episodes: EpisodeListItem[];
  isLoading: boolean;
  episodeLinkState?: EpisodeNavigationState;
}

export default function SeasonEpisodeGrid({
  episodes,
  isLoading,
  episodeLinkState,
}: SeasonEpisodeGridProps) {
  const { prefs: overlayPrefs, quickActionMode } = useOverlayPrefs();
  const prefetchEpisodeDetail = usePrefetchCatalogItemDetail();
  const setGridRef = useGridRowCap<HTMLDivElement>(VISIBLE_EPISODE_ROWS, episodes.length);

  if (isLoading) {
    return <EpisodeGridSkeleton />;
  }

  if (episodes.length === 0) {
    return (
      <div className="border-border text-muted-foreground bg-surface rounded-lg border p-5 text-sm">
        No episodes are available for this season yet.
      </div>
    );
  }

  return (
    <div
      ref={setGridRef}
      // `pt-1 -mt-1` gives the top row's 4px hover lift somewhere to go: the
      // scrollport clips both axes, and the cap adds this padding back.
      className="overlay-scroll -mt-1 grid grid-cols-2 gap-4 overflow-y-auto pt-1 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5"
    >
      {episodes.map((episode) => (
        <SeasonEpisodeCard
          key={episode.content_id}
          episode={episode}
          episodeLinkState={episodeLinkState}
          overlayPrefs={overlayPrefs}
          quickActionMode={quickActionMode}
          onPrefetch={() => prefetchEpisodeDetail(episode.content_id)}
        />
      ))}
    </div>
  );
}

function SeasonEpisodeCard({
  episode,
  episodeLinkState,
  overlayPrefs,
  quickActionMode,
  onPrefetch,
}: {
  episode: EpisodeListItem;
  episodeLinkState?: EpisodeNavigationState;
  overlayPrefs: CardOverlayPrefs | null;
  quickActionMode: CardQuickActionMode;
  onPrefetch: () => void;
}) {
  const cardRef = useRef<HTMLDivElement>(null);
  const prefetchHandlers = useDwellPrefetch(onPrefetch);
  const hasPartialProgress =
    !episode.user_data?.played &&
    (episode.user_data?.position_seconds ?? 0) > 0 &&
    (episode.user_data?.duration_seconds ?? 0) > 0;
  const episodeTitle = episode.title || `Episode ${episode.episode_number}`;
  const isMissing = episode.availability === "missing";
  const isUnaired = episode.availability === "unaired";
  const isPlaceholder = isMissing || isUnaired;

  const image = (
    <div className="media-card-image relative aspect-video">
      {isUnaired ? (
        <div className="bg-surface flex h-full w-full items-center justify-center" />
      ) : episode.still_url ? (
        <img
          src={episode.still_url}
          alt={episodeTitle}
          decoding="async"
          className={cn(
            "h-full w-full object-cover transition-transform duration-300",
            !isPlaceholder && "group-hover:scale-[1.03]",
            isMissing && "opacity-50 grayscale",
          )}
          loading="lazy"
        />
      ) : (
        <div className="flex h-full w-full items-center justify-center">
          <Play size={32} className="text-muted-foreground/30" />
        </div>
      )}
      {isPlaceholder && (
        <div className="bg-background/80 text-foreground absolute top-2 left-2 rounded px-1.5 py-0.5 text-[10px] font-semibold tracking-wide uppercase">
          {isUnaired && episode.air_date
            ? `Airs ${formatDate(episode.air_date, "medium")}`
            : "Not in library"}
        </div>
      )}
      {!isPlaceholder && overlayPrefs && (
        <CardOverlays
          data={overlayDataFromEpisodeListItem(episode)}
          prefs={overlayPrefs}
          variant="wide"
        />
      )}
      {!isPlaceholder && hasPartialProgress && (
        <div className="absolute inset-x-2 bottom-1.5 h-[3px] overflow-hidden rounded-full bg-black/40">
          <div
            className="progress-fill h-full rounded-full"
            style={{
              width: `${Math.max(
                0,
                Math.min(
                  100,
                  ((episode.user_data?.position_seconds ?? 0) /
                    (episode.user_data?.duration_seconds ?? 1)) *
                    100,
                ),
              )}%`,
              background: "var(--primary)",
            }}
          />
        </div>
      )}
    </div>
  );

  const info = (
    <>
      <div className="text-muted-foreground mt-2 flex items-center gap-2 text-xs">
        <span>Episode {episode.episode_number}</span>
        {episode.user_data?.played && <WatchedCheckIndicator className="ml-auto" />}
      </div>
      <p className="text-foreground truncate text-sm font-semibold">{episodeTitle}</p>
      <div className="mt-1.5 space-y-1">
        <div className="text-muted-foreground flex items-center gap-2 text-xs">
          {episode.runtime > 0 && <span>{episode.runtime}m</span>}
          {episode.air_date && !isUnaired && <span>{formatDate(episode.air_date, "medium")}</span>}
        </div>
        {episode.overview && (
          <p className="text-muted-foreground line-clamp-2 text-xs leading-relaxed">
            {episode.overview}
          </p>
        )}
      </div>
    </>
  );

  if (isPlaceholder) {
    return (
      <div ref={cardRef} className="media-card cursor-default opacity-90">
        <div className="relative">{image}</div>
        <div className="block">{info}</div>
      </div>
    );
  }

  return (
    <div ref={cardRef} className="group/card media-card media-card-longpress" {...prefetchHandlers}>
      <div className="relative">
        <ViewTransitionLink
          to={`/item/${episode.content_id}`}
          state={episodeLinkState}
          className="group block"
        >
          {image}
        </ViewTransitionLink>
        <MediaItemMenu
          contentId={episode.content_id}
          mediaType="episode"
          userState={toEpisodeUserState(episode.user_data)}
          variant="wide"
          showCollectionActions={false}
          showWatchedShortcut
          hasPartialProgress={hasPartialProgress}
          quickActionMode={quickActionMode}
          longPressRef={cardRef}
          itemTitle={episodeTitle}
        />
      </div>
      <ViewTransitionLink
        to={`/item/${episode.content_id}`}
        state={episodeLinkState}
        className="block"
      >
        {info}
      </ViewTransitionLink>
    </div>
  );
}

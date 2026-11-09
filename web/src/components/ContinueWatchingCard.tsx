import ViewTransitionLink from "@/components/ViewTransitionLink";
import { BookOpen, Play } from "lucide-react";
import { memo, useCallback, useRef, useState } from "react";
import { useLocation } from "react-router";
import type { ItemDetail, SectionItem } from "@/api/types";
import type { ProgressListEntry } from "@/hooks/queries/progress";
import MediaItemMenu from "@/components/MediaItemMenu";
import CardOverlays from "@/components/overlays/CardOverlays";
import { overlayDataFromSectionItem, type CardOverlayPrefs } from "@/lib/overlays";
import { formatListeningTimeLeft } from "@/lib/audiobooks/duration";
import { upcomingBadgeClass, upcomingBadgeLabel } from "@/lib/upcomingEventPresentation";
import { useWatchPlaybackController } from "@/playback/watchPlaybackContext";
import { parseWatchHref } from "@/pages/watchRouteHelpers";
import { buildItemHref, buildMediaPlayHref, isVideoWatchHref } from "@/lib/mediaNavigation";
import { type EpisodeSpoilerPrefs, useEpisodeSpoilerPrefs } from "@/hooks/useEpisodeSpoilerPrefs";
import { useShowTitleArt } from "@/hooks/useTitleArt";
import { useUICustomization } from "@/hooks/useUICustomization";
import { carouselCardWidthClasses } from "@/lib/uiCustomization";
import CardPlayOverlay from "@/components/CardPlayOverlay";
import type { CardQuickActionMode } from "@/lib/cardQuickActions";
import { isEpisodeStill, isEpisodeUnwatched, SPOILER_IMAGE_CLASS } from "@/lib/episodeSpoilers";
import { cn } from "@/lib/utils";

type ContinueWatchingCardProps = (
  | {
      detail: ItemDetail;
      progress: ProgressListEntry;
      sectionItem?: never;
    }
  | {
      sectionItem: SectionItem;
      detail?: never;
      progress?: never;
    }
) & {
  overlayPrefs?: CardOverlayPrefs | null;
  quickActionMode?: CardQuickActionMode;
  libraryId?: number;
  variant?: "wide" | "poster";
  /**
   * The card is in the first row and on screen at first paint: its artwork
   * loads eagerly at high fetch priority.
   */
  priority?: boolean;
};

type ContinueWatchingCardViewProps = ContinueWatchingCardProps & {
  spoilerPrefs: EpisodeSpoilerPrefs;
};

export default function ContinueWatchingCard(props: ContinueWatchingCardProps) {
  const spoilerPrefs = useEpisodeSpoilerPrefs();
  return <ContinueWatchingCardView {...props} spoilerPrefs={spoilerPrefs} />;
}

/**
 * A Continue Watching card whose spoiler preferences come from its row, so a
 * row of cards shares one settings lookup instead of one per card.
 */
export const ContinueWatchingRowCard = memo(function ContinueWatchingRowCard(
  props: ContinueWatchingCardViewProps,
) {
  return <ContinueWatchingCardView {...props} />;
});

function ContinueWatchingCardView({ spoilerPrefs, ...props }: ContinueWatchingCardViewProps) {
  const location = useLocation();
  const playbackController = useWatchPlaybackController();
  const { cardPresentation } = useUICustomization();
  const cardRef = useRef<HTMLDivElement>(null);
  const showTitleArt = useShowTitleArt() === true;
  const [logoFailed, setLogoFailed] = useState(false);
  const card =
    "sectionItem" in props && props.sectionItem
      ? {
          contentId: props.sectionItem.content_id,
          watchHref: buildMediaPlayHref({
            contentId: props.sectionItem.content_id,
            type: props.sectionItem.type,
            libraryId: props.libraryId,
          }),
          itemHref: buildItemHref({
            contentId: props.sectionItem.content_id,
            libraryId: props.libraryId,
          }),
          title: props.sectionItem.title,
          seriesId: props.sectionItem.series_id,
          seriesTitle: props.sectionItem.series_title,
          seasonNumber: props.sectionItem.season_number,
          episodeNumber: props.sectionItem.episode_number,
          backdropIsStill: props.sectionItem.backdrop_is_episode_still,
          posterIsStill: props.sectionItem.poster_is_episode_still,
          backdropUrl: props.sectionItem.backdrop_url,
          posterUrl: props.sectionItem.poster_url,
          positionSeconds: props.sectionItem.position_seconds ?? 0,
          durationSeconds: props.sectionItem.duration_seconds ?? 0,
          type: props.sectionItem.type,
        }
      : {
          contentId: props.detail.content_id,
          watchHref: buildMediaPlayHref({
            contentId: props.detail.content_id,
            type: props.detail.type,
            libraryId: props.libraryId,
          }),
          itemHref: buildItemHref({
            contentId: props.detail.content_id,
            libraryId: props.libraryId,
          }),
          title: props.detail.title,
          seriesId: props.detail.series_id,
          seriesTitle: props.detail.series_title,
          seasonNumber: props.detail.season_number,
          episodeNumber: props.detail.episode_number,
          backdropIsStill: props.detail.backdrop_is_episode_still ?? false,
          posterIsStill: props.detail.poster_is_episode_still ?? props.detail.type === "episode",
          backdropUrl: props.detail.backdrop_url,
          posterUrl: props.detail.poster_url,
          positionSeconds: props.progress.position_seconds,
          durationSeconds: props.progress.duration_seconds,
          type: props.detail.type,
        };

  const isNextUp = "sectionItem" in props && props.sectionItem?.item_source === "next_up";
  const dismissAction =
    "sectionItem" in props && props.sectionItem
      ? props.sectionItem.item_source === "next_up"
        ? props.sectionItem.series_id
          ? {
              itemId: props.sectionItem.content_id,
              surface: "next_up" as const,
              mediaType: props.sectionItem.type,
              seriesId: props.sectionItem.series_id,
            }
          : undefined
        : props.sectionItem.progress_updated_at
          ? {
              itemId: props.sectionItem.content_id,
              surface: "continue_watching" as const,
              mediaType: props.sectionItem.type,
              progressUpdatedAt: props.sectionItem.progress_updated_at,
            }
          : undefined
      : {
          itemId: props.detail.content_id,
          surface: "continue_watching" as const,
          mediaType: props.detail.type,
          progressUpdatedAt: props.progress.updated_at,
        };
  const progressPercent =
    card.durationSeconds > 0 ? (card.positionSeconds / card.durationSeconds) * 100 : 0;
  const hasPartialProgress = progressPercent > 0 && progressPercent < 100;
  // Drives both the bar itself and the overlay row's clearance above it.
  const showProgressBar = !isNextUp && progressPercent > 0;
  const hasEpisodeMeta = card.seasonNumber != null && card.episodeNumber != null;
  // A manga chapter is an ebook item that carries its owning series; the card
  // presents the series (heading, links) since the chapter's own item detail
  // is an internal page that loops back into the reader.
  const isMangaChapter = card.type === "ebook" && !!card.seriesId && !!card.seriesTitle;
  const headingIsSeries = (hasEpisodeMeta && !!card.seriesTitle) || isMangaChapter;
  const heading = headingIsSeries ? card.seriesTitle : card.title;
  const playTitle = heading ?? card.title ?? "item";
  // The heading shows the series title for episodes, so it should navigate to
  // the series page; everything else heads to the item's own page.
  const headingHref =
    headingIsSeries && card.seriesId
      ? buildItemHref({ contentId: card.seriesId, libraryId: props.libraryId })
      : card.itemHref;
  // Detail-page link target for the card's image and meta lines: manga
  // chapters head to the series page like the heading does.
  const detailHref = isMangaChapter ? headingHref : card.itemHref;
  const episodeLabel = hasEpisodeMeta
    ? `Season ${card.seasonNumber} Episode ${card.episodeNumber}`
    : null;
  const episodeMeta = hasEpisodeMeta
    ? card.seriesTitle && card.title
      ? `${episodeLabel} • ${card.title}`
      : episodeLabel
    : isMangaChapter
      ? card.title
      : null;
  const premiereBadge =
    "sectionItem" in props && props.sectionItem
      ? props.sectionItem.badges?.find((badge) => badge === "season_premiere")
      : undefined;
  const timeLeftLabel = isNextUp
    ? premiereBadge
      ? null
      : "Next Episode"
    : card.type === "ebook"
      ? `${Math.round(Math.min(Math.max(progressPercent, 0), 100))}% read`
      : card.durationSeconds > 0
        ? card.type === "audiobook"
          ? formatListeningTimeLeft(card.positionSeconds, card.durationSeconds)
          : `${Math.round((card.durationSeconds - card.positionSeconds) / 60)} min left`
        : "\u00A0";
  const handleWatchClick = useCallback(
    (event: React.MouseEvent<HTMLAnchorElement>) => {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.altKey ||
        event.ctrlKey ||
        event.shiftKey
      ) {
        return;
      }

      if (!isVideoWatchHref(card.watchHref)) {
        return;
      }

      const parsed = parseWatchHref(card.watchHref);
      if (!parsed) {
        return;
      }

      event.preventDefault();
      playbackController.startPlayback(
        {
          contentId: parsed.contentId,
          fileId: parsed.fileId,
          libraryId: parsed.libraryId,
          restart: parsed.restart,
          returnHref: `${location.pathname}${location.search}`,
        },
        "viewer",
      );
    },
    [card.watchHref, location.pathname, location.search, playbackController],
  );

  const variant = props.variant ?? "wide";
  const isPoster = variant === "poster";
  const playableContentId = card.contentId ?? "";
  const containerWidth = isPoster
    ? carouselCardWidthClasses(cardPresentation.poster_size)
    : // Wide resume cards are the row's focus: most of a phone's width, a
      // little larger than other wide cards on bigger screens.
      "w-[84vw] max-w-[400px] shrink-0 sm:w-[360px] lg:w-[400px]";
  const showCaption = cardPresentation.caption !== "artwork";
  const showMetadata = cardPresentation.caption === "title_metadata";
  // Audiobook covers are square (Audible-style); use 1:1 for the poster
  // variant so they don't get top/bottom-cropped into a 2:3 frame.
  const imageAspect = isPoster
    ? card.type === "audiobook"
      ? "aspect-square"
      : "aspect-[2/3]"
    : "aspect-video";
  const isSectionEpisode = "sectionItem" in props && props.sectionItem?.type === "episode";
  // Episode detail uses poster_url for its still; section cards prefer the backdrop.
  const preferPoster = isPoster || (!isSectionEpisode && card.type === "episode");
  const imagePrimary = preferPoster ? card.posterUrl : card.backdropUrl;
  const imageFallback = preferPoster ? card.backdropUrl : card.posterUrl;
  const imageSrc = imagePrimary || imageFallback;
  const selectedPoster = imagePrimary ? preferPoster : !preferPoster;
  const selectedIsStill = selectedPoster ? card.posterIsStill : card.backdropIsStill;
  const hideImage =
    spoilerPrefs.hideImages &&
    card.type === "episode" &&
    isEpisodeStill(selectedIsStill) &&
    isEpisodeUnwatched({
      played: (props.sectionItem ?? props.detail)?.user_state?.played,
      position_seconds: card.positionSeconds,
    });

  // The wide card's line under the title: "S2 E1 • 40m" for an episode, the
  // time left (or the year and length) for a movie, and the existing label
  // for books.
  const totalSeconds =
    card.durationSeconds > 0
      ? card.durationSeconds
      : ((props.sectionItem?.runtime ?? props.detail?.runtime ?? 0) as number) * 60;
  const timeText = formatCardDuration(
    showProgressBar && card.durationSeconds > 0
      ? card.durationSeconds - card.positionSeconds
      : totalSeconds,
  );
  const year = props.sectionItem?.year ?? props.detail?.year;
  const imageLine =
    card.type === "audiobook" || card.type === "ebook"
      ? timeLeftLabel && timeLeftLabel !== "\u00A0"
        ? timeLeftLabel
        : null
      : hasEpisodeMeta
        ? [`S${card.seasonNumber} E${card.episodeNumber}`, timeText].filter(Boolean).join(" • ")
        : showProgressBar
          ? timeText
          : [year ? String(year) : null, timeText].filter(Boolean).join(" • ") || null;
  const logoUrl = props.sectionItem?.logo_url ?? props.detail?.logo_url;
  const logoSrc = showTitleArt && !logoFailed && logoUrl ? logoUrl : null;

  return (
    <div ref={cardRef} className={`media-card-longpress group/card ${containerWidth}`}>
      <div className="group/media relative">
        <ViewTransitionLink to={detailHref} className="block">
          <div
            className={cn(
              "media-card-image relative overflow-hidden",
              imageAspect,
              isPoster ? "rounded-xl" : "rounded-2xl",
            )}
          >
            {imageSrc ? (
              <img
                src={imageSrc}
                alt={heading}
                className={cn(
                  "h-full w-full object-cover transition-transform duration-300",
                  hideImage ? SPOILER_IMAGE_CLASS : "group-hover/media:scale-105",
                )}
                loading={props.priority ? "eager" : "lazy"}
                fetchPriority={props.priority ? "high" : undefined}
                decoding="async"
              />
            ) : (
              <div className="text-muted-foreground bg-surface flex h-full w-full items-center justify-center text-sm">
                No Image
              </div>
            )}

            {"sectionItem" in props && props.sectionItem && props.overlayPrefs && (
              <CardOverlays
                data={overlayDataFromSectionItem(props.sectionItem)}
                prefs={props.overlayPrefs}
                variant={variant}
                hasProgressBar={showProgressBar}
                hideBottomEdge={!isPoster}
              />
            )}

            {/* Hover dim behind the play button */}
            <div className="media-card-hover-dim absolute inset-0 bg-black/0 transition-colors duration-150" />

            {isPoster ? (
              /* Progress bar — inset pill so a full bar doesn't read as a
                 stray edge along the artwork */
              showProgressBar && (
                <div className="absolute inset-x-2.5 bottom-2 h-[3px] overflow-hidden rounded-full bg-black/40">
                  <div
                    className="h-full rounded-full transition-all duration-300"
                    style={{
                      width: `${Math.min(progressPercent, 100)}%`,
                      background: "var(--primary)",
                    }}
                  />
                </div>
              )
            ) : (
              /* Wide card: the title, and a play mark, progress and time
                 left, drawn on the artwork over a dark fade. */
              <div className="pointer-events-none absolute inset-0 z-10 flex flex-col justify-end bg-gradient-to-t from-black/90 via-black/35 via-45% to-transparent to-75% px-4 pb-4 text-white sm:px-5">
                {logoSrc ? (
                  <img
                    src={logoSrc}
                    alt={heading}
                    onError={() => setLogoFailed(true)}
                    className="-ml-3 mb-2.5 max-h-9 max-w-[45%] self-start object-contain object-left-bottom drop-shadow-md"
                  />
                ) : (
                  <div className="mb-1.5 truncate text-xl leading-tight font-semibold tracking-tight drop-shadow-md">
                    {heading}
                  </div>
                )}
                <div className="flex items-center gap-3 text-[0.9375rem] font-medium drop-shadow-md">
                  {showProgressBar && (
                    <>
                      {card.type === "ebook" ? (
                        <BookOpen aria-hidden className="size-[1.125rem] shrink-0" />
                      ) : (
                        <Play
                          aria-hidden
                          className="size-[1.125rem] shrink-0"
                          fill="currentColor"
                        />
                      )}
                      <div className="h-2 w-20 shrink-0 overflow-hidden rounded-full bg-white/30">
                        <div
                          className="h-full rounded-full bg-white transition-all duration-300"
                          style={{ width: `${Math.max(Math.min(progressPercent, 100), 4)}%` }}
                        />
                      </div>
                    </>
                  )}
                  {imageLine && <span className="min-w-0 truncate">{imageLine}</span>}
                  {premiereBadge && (
                    <span
                      className={`inline-flex shrink-0 rounded-full border px-2 py-0.5 text-[0.625rem] leading-none font-semibold tracking-wide uppercase backdrop-blur-sm ${upcomingBadgeClass(
                        premiereBadge,
                      )}`}
                    >
                      {upcomingBadgeLabel(premiereBadge)}
                    </span>
                  )}
                </div>
              </div>
            )}
          </div>
        </ViewTransitionLink>
        {isPoster && playableContentId && isVideoWatchHref(card.watchHref) ? (
          <CardPlayOverlay
            contentId={playableContentId}
            title={playTitle}
            type={card.type === "movie" ? "movie" : "episode"}
            libraryId={props.libraryId}
          />
        ) : (
          <ViewTransitionLink
            to={card.watchHref}
            onClick={handleWatchClick}
            aria-label={`${card.type === "ebook" ? "Read" : "Play"} ${heading}`}
            className="media-card-play-trigger bg-primary text-primary-foreground absolute top-1/2 left-1/2 flex h-11 w-11 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full shadow-lg transition-all duration-200 hover:scale-110 hover:shadow-xl hover:brightness-110 active:scale-95"
          >
            {card.type === "ebook" ? (
              <BookOpen className="h-5 w-5" />
            ) : (
              <Play className="ml-0.5 h-5 w-5" fill="currentColor" />
            )}
          </ViewTransitionLink>
        )}
        <MediaItemMenu
          contentId={
            "sectionItem" in props && props.sectionItem
              ? props.sectionItem.content_id
              : props.detail.content_id
          }
          mediaType={
            "sectionItem" in props && props.sectionItem ? props.sectionItem.type : props.detail.type
          }
          libraryId={props.libraryId}
          userState={
            "sectionItem" in props && props.sectionItem
              ? props.sectionItem.user_state
              : props.detail.user_state
          }
          variant={variant}
          showWatchedShortcut
          showFavoriteShortcut={false}
          dismissAction={dismissAction}
          hasPartialProgress={hasPartialProgress}
          hasResumePosition={card.positionSeconds > 0}
          quickActionMode={props.quickActionMode ?? "none"}
          longPressRef={cardRef}
          itemTitle={heading}
        />
      </div>

      {/* Info — the wide card draws its title on the artwork instead. */}
      {isPoster && showCaption ? (
        <div className="px-0.5 pt-2.5">
          <ViewTransitionLink
            to={headingHref}
            className="block truncate text-[0.8125rem] font-semibold hover:underline"
          >
            {heading}
          </ViewTransitionLink>
          {showMetadata && episodeMeta && (
            <ViewTransitionLink
              to={isMangaChapter ? card.watchHref : card.itemHref}
              className="text-muted-foreground block truncate text-xs hover:underline"
            >
              {episodeMeta}
            </ViewTransitionLink>
          )}
          {showMetadata && (timeLeftLabel || premiereBadge) && (
            <div className="flex items-center gap-1.5">
              {timeLeftLabel &&
                (timeLeftLabel === "\u00A0" ? (
                  <div className="text-muted-foreground text-xs">{timeLeftLabel}</div>
                ) : (
                  <ViewTransitionLink
                    to={isMangaChapter ? card.watchHref : card.itemHref}
                    className="text-muted-foreground block w-fit truncate text-xs hover:underline"
                  >
                    {timeLeftLabel}
                  </ViewTransitionLink>
                ))}
              {premiereBadge && (
                <span
                  className={`inline-flex shrink-0 rounded-full border px-2 py-0.5 text-[0.625rem] leading-none font-semibold tracking-wide uppercase backdrop-blur-sm ${upcomingBadgeClass(
                    premiereBadge,
                  )}`}
                >
                  {upcomingBadgeLabel(premiereBadge)}
                </span>
              )}
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
}

/** A length as the wide card shows it: "1h 35m", "40m", or null when unknown. */
function formatCardDuration(seconds: number): string | null {
  if (!(seconds > 0)) return null;
  const minutes = Math.max(1, Math.round(seconds / 60));
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  if (hours === 0) return `${rest}m`;
  return rest === 0 ? `${hours}h` : `${hours}h ${rest}m`;
}

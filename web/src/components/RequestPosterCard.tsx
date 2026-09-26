import { Link } from "react-router";
import { Film, Library, Loader2, Plus, Tv } from "lucide-react";
import type { MediaRequest, RequestMediaResult } from "@/api/types";
import { cn } from "@/lib/utils";
import {
  requestDetailHref,
  requestDisplayState,
  tmdbImageURL,
  type RequestDisplayState,
} from "@/lib/mediaRequests";
import { RequestReasonBadge, RequestStatusBadge } from "@/components/RequestStatusBadge";
import { Button } from "@/components/ui/button";
import ViewTransitionLink from "@/components/ViewTransitionLink";

const POSTER_WIDTH = "w-[148px] sm:w-[164px] lg:w-[184px]";

type DiscoverProps = {
  variant: "discover";
  item: RequestMediaResult;
  /** Called when the inline hover Request button is clicked. Omit to suppress the button. */
  onRequest?: () => void;
  /** Displays the spinner state on the hover Request button. Ignored when onRequest is omitted. */
  isSubmitting?: boolean;
  /** When true, fills the parent (use inside grids). Default: fixed carousel width. */
  fluid?: boolean;
};

type MineProps = {
  variant: "mine";
  request: MediaRequest;
  fluid?: boolean;
  /** Shows a Cancel request action. Pass it only for the viewer's own cancellable request. */
  onCancel?: () => void;
  /** Disables the Cancel request action while a cancellation is in flight. */
  isCancelling?: boolean;
};

export type RequestPosterCardProps = DiscoverProps | MineProps;

export default function RequestPosterCard(props: RequestPosterCardProps) {
  if (props.variant === "mine") {
    return (
      <MineCard
        request={props.request}
        fluid={props.fluid}
        onCancel={props.onCancel}
        isCancelling={props.isCancelling}
      />
    );
  }
  return (
    <DiscoverCard
      item={props.item}
      isSubmitting={props.isSubmitting}
      onRequest={props.onRequest}
      fluid={props.fluid}
    />
  );
}

function DiscoverCard({
  item,
  isSubmitting,
  onRequest,
  fluid,
}: {
  item: RequestMediaResult;
  isSubmitting?: boolean;
  onRequest?: () => void;
  fluid?: boolean;
}) {
  const poster = tmdbImageURL(item.poster_path);
  const requestable = item.request.requestable;
  const availableInLibrary = item.availability === "available" && !item.request.status;
  const state: RequestDisplayState | undefined = item.request.status
    ? requestDisplayState(item.request.status)
    : availableInLibrary
      ? "available"
      : undefined;
  const badgeClassName = posterBadgeClassName(Boolean(item.library_content_id));

  return (
    <div
      className={cn(
        "group/req-card relative block focus-within:outline-none",
        fluid ? "w-full" : POSTER_WIDTH,
      )}
    >
      <Link
        to={requestDetailHref(item.media_type, item.tmdb_id)}
        className="block focus:outline-none focus-visible:outline-none"
      >
        <PosterFrame
          poster={poster}
          title={item.title}
          mediaType={item.media_type}
          dim={!requestable}
        >
          {state ? (
            <RequestStatusBadge state={state} overlay className={badgeClassName} />
          ) : !requestable ? (
            <RequestReasonBadge reason={item.request.reason} overlay className={badgeClassName} />
          ) : null}
          {requestable && onRequest && (
            <div
              data-testid="request-poster-hover-overlay"
              className="pointer-events-none absolute inset-0 translate-y-2 bg-gradient-to-t from-black/85 via-black/45 to-transparent opacity-0 transition-all duration-200 ease-out group-focus-within/req-card:translate-y-0 group-focus-within/req-card:opacity-100 group-hover/req-card:translate-y-0 group-hover/req-card:opacity-100"
            />
          )}
        </PosterFrame>

        <CardMeta
          title={item.title}
          year={item.year}
          rating={item.vote_average}
          mediaType={item.media_type}
        />
      </Link>

      {requestable && onRequest && (
        <div className="pointer-events-none absolute top-0 left-0 flex aspect-[2/3] w-full translate-y-2 items-end justify-center pb-3 opacity-0 transition-all duration-200 ease-out group-focus-within/req-card:translate-y-0 group-focus-within/req-card:opacity-100 group-hover/req-card:translate-y-0 group-hover/req-card:opacity-100">
          <button
            type="button"
            disabled={Boolean(isSubmitting)}
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              onRequest();
            }}
            className="pointer-events-auto inline-flex items-center gap-1.5 rounded-full bg-white px-3.5 py-1.5 text-[12px] font-semibold tracking-wide text-black shadow-lg shadow-black/40 transition-all hover:scale-[1.03] active:scale-[0.97] disabled:opacity-70"
          >
            {isSubmitting ? (
              <>
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                Sending
              </>
            ) : (
              <>
                <Plus className="h-3.5 w-3.5 stroke-[2.5]" />
                Request
              </>
            )}
          </button>
        </div>
      )}

      {item.library_content_id ? (
        <LibraryCardLink contentID={item.library_content_id} title={item.title} />
      ) : null}
    </div>
  );
}

function MineCard({
  request,
  fluid,
  onCancel,
  isCancelling,
}: {
  request: MediaRequest;
  fluid?: boolean;
  onCancel?: () => void;
  isCancelling?: boolean;
}) {
  const poster = tmdbImageURL(request.poster_path);
  const state = requestDisplayState(request.status, request.outcome);
  const isClosed =
    request.outcome === "failed" ||
    request.outcome === "declined" ||
    request.outcome === "cancelled";

  return (
    <div
      className={cn(
        "group/req-card relative block focus:outline-none focus-visible:outline-none",
        fluid ? "w-full" : POSTER_WIDTH,
      )}
    >
      <Link
        to={requestDetailHref(request.media_type, request.tmdb_id)}
        className="block focus:outline-none focus-visible:outline-none"
      >
        <PosterFrame
          poster={poster}
          title={request.title}
          mediaType={request.media_type}
          dim={isClosed}
        >
          {state ? (
            <RequestStatusBadge
              state={state}
              overlay
              className={posterBadgeClassName(Boolean(request.library_content_id))}
            />
          ) : null}
        </PosterFrame>

        <CardMeta title={request.title} year={request.year} mediaType={request.media_type} />

        {request.last_error ? (
          <p
            className="text-destructive mt-1 line-clamp-2 text-[11px] leading-tight"
            title={request.last_error}
          >
            {request.last_error}
          </p>
        ) : null}
      </Link>

      {onCancel ? (
        <Button
          type="button"
          variant="ghost"
          size="xs"
          onClick={onCancel}
          disabled={isCancelling}
          aria-label={`Cancel request for ${request.title}`}
          className="text-muted-foreground mt-1"
        >
          Cancel request
        </Button>
      ) : null}

      {request.library_content_id ? (
        <LibraryCardLink contentID={request.library_content_id} title={request.title} />
      ) : null}
    </div>
  );
}

function LibraryCardLink({ contentID, title }: { contentID: string; title: string }) {
  return (
    <ViewTransitionLink
      to={`/item/${encodeURIComponent(contentID)}`}
      aria-label={`Open ${title} in library`}
      className="absolute top-2 left-2 inline-flex max-w-[calc(100%-1rem)] items-center gap-1.5 rounded-full bg-black/70 px-2 py-[3px] text-[10px] leading-none font-semibold tracking-[0.06em] text-white uppercase shadow-sm ring-1 shadow-white/15 backdrop-blur-md transition-colors hover:bg-black/85 focus:outline-none focus-visible:ring-2 focus-visible:ring-white/70"
    >
      <Library className="h-3 w-3 shrink-0" strokeWidth={2.4} aria-hidden />
      <span className="truncate">Library</span>
    </ViewTransitionLink>
  );
}

function PosterFrame({
  poster,
  title,
  mediaType,
  dim,
  children,
}: {
  poster: string | null;
  title: string;
  mediaType: "movie" | "series";
  dim?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div className="media-card-image relative aspect-[2/3]">
      {poster ? (
        <img
          src={poster}
          alt={title ? `${title} poster` : "Poster"}
          loading="lazy"
          className={cn(
            "h-full w-full object-cover transition-[transform,filter] duration-300 group-hover/req-card:scale-[1.04]",
            dim && "brightness-[0.85] saturate-[0.8]",
          )}
        />
      ) : (
        <PosterFallback title={title} mediaType={mediaType} dim={dim} />
      )}
      {/* subtle bottom vignette for legibility behind ribbons / hover overlays */}
      <div className="pointer-events-none absolute inset-x-0 bottom-0 h-20 bg-gradient-to-t from-black/55 to-transparent opacity-90" />
      {children}
    </div>
  );
}

function PosterFallback({
  title,
  mediaType,
  dim,
}: {
  title: string;
  mediaType: "movie" | "series";
  dim?: boolean;
}) {
  const hue = stringHue(title);
  const Icon = mediaType === "series" ? Tv : Film;
  return (
    <div
      className={cn(
        "relative flex h-full w-full flex-col justify-end overflow-hidden p-3.5",
        dim && "opacity-90",
      )}
      style={{
        background: `linear-gradient(160deg, hsl(${hue} 30% 22%) 0%, hsl(${hue} 22% 11%) 60%, hsl(${(hue + 28) % 360} 18% 7%) 100%)`,
      }}
    >
      <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
        <Icon className="h-28 w-28 text-white/[0.05]" strokeWidth={1.25} />
      </div>
      <div
        className="pointer-events-none absolute inset-0"
        style={{
          backgroundImage: "radial-gradient(rgba(255,255,255,0.55) 1px, transparent 1px)",
          backgroundSize: "9px 9px",
          opacity: 0.05,
        }}
      />
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-white/15 to-transparent" />
      <div className="relative space-y-1.5">
        <span className="text-[9px] font-semibold tracking-[0.22em] text-white/45 uppercase">
          {mediaType === "series" ? "Series" : "Motion picture"}
        </span>
        <h4 className="font-display line-clamp-4 text-[15px] leading-tight font-bold tracking-tight text-balance text-white/90">
          {title}
        </h4>
      </div>
    </div>
  );
}

function stringHue(input: string): number {
  let hash = 0;
  for (let i = 0; i < input.length; i++) {
    hash = (Math.imul(hash, 31) + input.charCodeAt(i)) | 0;
  }
  return Math.abs(hash) % 360;
}

function CardMeta({
  title,
  year,
  rating,
  mediaType,
}: {
  title: string;
  year?: number;
  rating?: number;
  mediaType?: "movie" | "series";
}) {
  const Icon = mediaType === "series" ? Tv : Film;
  const hasMeta = mediaType || year !== undefined || rating !== undefined;
  return (
    <div className="mt-2.5 min-w-0 px-0.5">
      <h3 className="text-foreground line-clamp-1 text-[13px] leading-tight font-semibold tracking-tight">
        {title}
      </h3>
      {hasMeta && (
        <div className="text-muted-foreground mt-1 flex items-center gap-1.5 text-[11px]">
          {mediaType && (
            <>
              <Icon className="h-3 w-3 shrink-0 opacity-60" strokeWidth={2} aria-hidden />
              <span>{mediaType === "series" ? "Series" : "Movie"}</span>
            </>
          )}
          {mediaType && year ? (
            <span aria-hidden className="text-muted-foreground/40">
              ·
            </span>
          ) : null}
          {year ? <span className="tabular-nums">{year}</span> : null}
          {(year || mediaType) && rating ? (
            <span aria-hidden className="text-muted-foreground/40">
              ·
            </span>
          ) : null}
          {rating ? (
            <span className="tabular-nums">
              <span className="text-amber-300/90">★</span> {rating.toFixed(1)}
            </span>
          ) : null}
        </div>
      )}
    </div>
  );
}

/** Pins the status badge in the top-right corner, clear of the Library chip. */
function posterBadgeClassName(reserveLibrarySpace: boolean): string {
  return cn(
    "absolute top-2 right-2",
    reserveLibrarySpace ? "max-w-[calc(100%-5.75rem)]" : "max-w-[calc(100%-1rem)]",
  );
}

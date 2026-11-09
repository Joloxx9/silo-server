import { useMemo, useState } from "react";
import { Check, Loader2, Plus } from "lucide-react";
import type { RequestPersonCredit } from "@/api/types";
import {
  creditInLibrary,
  creditRole,
  FILMOGRAPHY_FILTERS,
  filterCredits,
  type FilmographyFilter,
} from "@/lib/personFilmography";
import MediaCardArtwork from "@/components/MediaCardArtwork";
import { RequestReasonBadge, RequestStatusBadge } from "@/components/RequestStatusBadge";
import { Button } from "@/components/ui/button";
import ViewTransitionLink from "@/components/ViewTransitionLink";
import { useCreateMediaRequest } from "@/hooks/queries/useRequests";
import {
  formatMediaType,
  requestDetailHref,
  requestDisplayState,
  requestInputFromMediaResult,
  tmdbImageURL,
} from "@/lib/mediaRequests";
import { cn } from "@/lib/utils";

/**
 * A person's whole filmography from TMDB, one title per row: the poster, the
 * title with the role and a short overview, and what the viewer can do with
 * it: open it in the library, or request it in one click.
 */
export default function PersonFilmography({ credits }: { credits: RequestPersonCredit[] }) {
  const [filter, setFilter] = useState<FilmographyFilter>("all");
  const [showUndated, setShowUndated] = useState(false);
  const createRequest = useCreateMediaRequest();

  const inLibrary = useMemo(() => credits.filter(creditInLibrary).length, [credits]);
  const undated = useMemo(() => credits.filter((c) => !c.media.release_date).length, [credits]);
  const visible = useMemo(
    () => filterCredits(credits, filter, showUndated),
    [credits, filter, showUndated],
  );
  const pendingKey =
    createRequest.isPending && createRequest.variables
      ? `${createRequest.variables.media_type}-${createRequest.variables.tmdb_id}`
      : null;

  return (
    <div className="space-y-5">
      <div className="space-y-3">
        <h2 className="text-xl font-semibold tracking-tight">
          Filmography
          <span className="text-muted-foreground ml-2 text-sm font-normal">
            {credits.length} {credits.length === 1 ? "title" : "titles"} · {inLibrary} in your
            library
          </span>
        </h2>
        <div
          className="bg-background/85 sticky top-0 z-20 -mx-1 flex flex-wrap gap-1.5 px-1 py-2 backdrop-blur-md"
          role="group"
          aria-label="Filter filmography"
        >
          {FILMOGRAPHY_FILTERS.map(({ id, label }) => (
            <FilterChip key={id} active={filter === id} onClick={() => setFilter(id)}>
              {label}
              <span className="text-muted-foreground ml-1.5 tabular-nums">
                {filterCredits(credits, id, showUndated).length}
              </span>
            </FilterChip>
          ))}
          {undated > 0 ? (
            <FilterChip active={showUndated} onClick={() => setShowUndated((v) => !v)}>
              Announced ({undated})
            </FilterChip>
          ) : null}
        </div>
      </div>

      {visible.length === 0 ? (
        <p className="text-muted-foreground py-8 text-sm">No titles match this filter.</p>
      ) : (
        <ol className="divide-foreground/15 divide-y">
          {visible.map((credit, index) => {
            const key = `${credit.media.media_type}-${credit.media.tmdb_id}`;
            const year = credit.media.year;
            const previousYear = index > 0 ? visible[index - 1]?.media.year : undefined;
            return (
              <li key={key}>
                {index === 0 || year !== previousYear ? (
                  <div className="text-muted-foreground pt-5 pb-1 text-sm font-semibold tabular-nums">
                    {year ?? "Announced"}
                  </div>
                ) : null}
                <CreditRow
                  credit={credit}
                  pending={pendingKey === key}
                  onRequest={() => createRequest.mutate(requestInputFromMediaResult(credit.media))}
                />
              </li>
            );
          })}
        </ol>
      )}
    </div>
  );
}

function FilterChip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        "rounded-full border px-3 py-1.5 text-xs font-medium transition-colors",
        active
          ? "border-border/20 bg-foreground/10 text-foreground"
          : "border-border/10 text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}

function CreditRow({
  credit,
  pending,
  onRequest,
}: {
  credit: RequestPersonCredit;
  pending: boolean;
  onRequest: () => void;
}) {
  const { media } = credit;
  const inLibrary = creditInLibrary(credit);
  const href =
    inLibrary && media.library_content_id
      ? `/item/${encodeURIComponent(media.library_content_id)}`
      : requestDetailHref(media.media_type, media.tmdb_id);
  const state = media.request.status
    ? requestDisplayState(media.request.status, undefined, media.request.state)
    : undefined;
  const role = creditRole(credit);
  const meta = [formatMediaType(media.media_type), role].filter(Boolean).join(" · ");

  let action: React.ReactNode;
  if (inLibrary) {
    action = (
      <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/15 px-3 py-1.5 text-xs font-semibold text-emerald-500">
        <Check className="size-3.5 stroke-[3]" aria-hidden />
        In library
      </span>
    );
  } else if (media.request.requestable) {
    action = (
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={pending}
        onClick={onRequest}
        aria-label={`Request ${media.title}`}
      >
        {pending ? (
          <Loader2 className="size-3.5 animate-spin" aria-hidden />
        ) : (
          <Plus className="size-3.5 stroke-[2.5]" aria-hidden />
        )}
        Request
      </Button>
    );
  } else if (state) {
    action = <RequestStatusBadge state={state} />;
  } else {
    action = <RequestReasonBadge reason={media.request.reason} />;
  }

  return (
    <div className="flex gap-4 py-3">
      <ViewTransitionLink
        to={href}
        aria-label={media.title}
        className="block w-[72px] shrink-0 overflow-hidden rounded-lg sm:w-[92px]"
      >
        <MediaCardArtwork
          src={tmdbImageURL(media.poster_path, "w185")}
          alt={media.title}
          mediaType={media.media_type}
          lazy
          dim={!inLibrary && !media.request.requestable && !state}
        />
      </ViewTransitionLink>
      <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row sm:items-start sm:gap-4">
        <div className="min-w-0 flex-1">
          <ViewTransitionLink
            to={href}
            className="line-clamp-1 text-base font-semibold hover:underline"
          >
            {media.title}
          </ViewTransitionLink>
          {meta ? <p className="text-muted-foreground mt-0.5 text-sm">{meta}</p> : null}
          {media.overview ? (
            <p className="text-muted-foreground/80 mt-1.5 line-clamp-2 text-sm leading-relaxed sm:line-clamp-3">
              {media.overview}
            </p>
          ) : null}
        </div>
        <div className="shrink-0">{action}</div>
      </div>
    </div>
  );
}

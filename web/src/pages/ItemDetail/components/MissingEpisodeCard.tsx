import { CalendarClock, Download } from "lucide-react";
import type { EpisodeListItem } from "@/api/types";
import { Button } from "@/components/ui/button";
import EpisodeCardCaption from "./EpisodeCardCaption";

interface MissingEpisodeCardProps {
  episode: EpisodeListItem;
  /** Opens a request for the episode's season; absent when requests are off. */
  onRequest?: () => void;
}

/**
 * An episode the library lacks, in a season grid that asked to show them:
 * dimmed, not a link, labeled by whether it has aired, with a request button.
 */
export default function MissingEpisodeCard({ episode, onRequest }: MissingEpisodeCardProps) {
  const unaired = episode.availability === "unaired";
  const label = unaired ? "Not aired yet" : "Not in library";

  return (
    <div className="media-card" data-availability={episode.availability}>
      <div className="media-card-image relative aspect-video overflow-hidden">
        {episode.still_url ? (
          <img
            src={episode.still_url}
            alt=""
            decoding="async"
            loading="lazy"
            className="h-full w-full object-cover opacity-40 grayscale"
          />
        ) : (
          <div className="bg-surface flex h-full w-full items-center justify-center">
            <CalendarClock size={28} className="text-muted-foreground/40" />
          </div>
        )}
        <span className="absolute top-2 left-2 rounded-md bg-black/70 px-1.5 py-0.5 text-[0.6875rem] font-medium text-white">
          {label}
        </span>
        {onRequest && (
          <Button
            type="button"
            size="sm"
            variant="secondary"
            className="absolute right-2 bottom-2 h-7 gap-1 px-2 text-xs"
            onClick={onRequest}
            aria-label={`Request season ${episode.season_number}`}
          >
            <Download className="size-3.5" />
            Request
          </Button>
        )}
      </div>
      <div className="opacity-60">
        <EpisodeCardCaption episode={episode} hideOverview={false} />
      </div>
    </div>
  );
}

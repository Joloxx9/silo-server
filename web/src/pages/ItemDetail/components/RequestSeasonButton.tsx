import { Check, Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface RequestSeasonButtonProps {
  /** What the request covers, for the accessible name, e.g. "Season 2". */
  target: string;
  /** The series' active request already covers this season. */
  requested: boolean;
  onRequest: () => void;
  className?: string;
}

/**
 * The Request button on a season or episode the library lacks. Once a request
 * covers the season it reads Requested and stays disabled.
 */
export default function RequestSeasonButton({
  target,
  requested,
  onRequest,
  className,
}: RequestSeasonButtonProps) {
  return (
    <Button
      type="button"
      size="sm"
      variant="secondary"
      className={cn("gap-1 text-xs", requested && "disabled:opacity-90", className)}
      onClick={onRequest}
      disabled={requested}
      aria-label={requested ? `${target} requested` : `Request ${target}`}
    >
      {requested ? <Check className="size-3.5" /> : <Download className="size-3.5" />}
      {requested ? "Requested" : "Request"}
    </Button>
  );
}

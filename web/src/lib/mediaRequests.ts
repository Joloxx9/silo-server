import type {
  CreateMediaRequestInput,
  MediaRequest,
  MediaRequestOutcome,
  MediaRequestStatus,
  RequestMediaResult,
  RequestMediaType,
  RequestUserState,
} from "@/api/types";
import { formatDate } from "@/lib/datetime";

export const REQUEST_STATUSES: Array<MediaRequestStatus | "all"> = [
  "all",
  "pending",
  "approved",
  "queued",
  "downloading",
  "completed",
];

export const REQUEST_OUTCOMES: Array<MediaRequestOutcome | "all"> = [
  "all",
  "active",
  "declined",
  "cancelled",
  "failed",
];

type BadgeVariant = "default" | "secondary" | "destructive" | "outline";

export function formatMediaType(mediaType: RequestMediaType): string {
  return mediaType === "series" ? "Series" : "Movie";
}

export function formatRequestStatus(status?: MediaRequestStatus): string {
  switch (status) {
    case "pending":
      return "Pending";
    case "approved":
      return "Approved";
    case "queued":
      return "Queued";
    case "downloading":
      return "Downloading";
    case "completed":
      return "Completed";
    default:
      return "Requested";
  }
}

export function requestStatusBadgeVariant(status?: MediaRequestStatus): BadgeVariant {
  switch (status) {
    case "completed":
      return "default";
    case "pending":
      return "outline";
    default:
      return "secondary";
  }
}

export function formatRequestOutcome(outcome?: MediaRequestOutcome): string {
  switch (outcome) {
    case "active":
      return "Active";
    case "declined":
      return "Declined";
    case "cancelled":
      return "Cancelled";
    case "failed":
      return "Failed";
    default:
      return "Active";
  }
}

export function requestOutcomeBadgeVariant(outcome?: MediaRequestOutcome): BadgeVariant {
  switch (outcome) {
    case "failed":
    case "declined":
    case "cancelled":
      return "destructive";
    case "active":
      return "secondary";
    default:
      return "outline";
  }
}

/**
 * The request states the user-facing request pages show. Admin views keep the
 * raw status and outcome. The server derives this state (v2 `state`), and its
 * value wins; the fallback for older servers collapses status and outcome the
 * same way: queued and downloading read as Processing, completed as
 * Available, and a closed outcome wins over the status it closed at.
 */
export type RequestDisplayState = RequestUserState;

export function requestDisplayState(
  status?: MediaRequestStatus,
  outcome?: MediaRequestOutcome,
  state?: RequestUserState,
): RequestDisplayState | undefined {
  if (state) return state;
  if (outcome === "declined" || outcome === "cancelled" || outcome === "failed") return outcome;
  switch (status) {
    case "pending":
    case "approved":
      return status;
    case "queued":
    case "downloading":
      return "processing";
    case "completed":
      return "available";
    default:
      return undefined;
  }
}

export function formatRequestDisplayState(state: RequestDisplayState): string {
  switch (state) {
    case "pending":
    case "approved":
      return formatRequestStatus(state);
    case "processing":
      return "Processing";
    case "available":
      return "Available";
    default:
      return formatRequestOutcome(state);
  }
}

/**
 * Mirrors the server's rule for owner cancellation: a request can be withdrawn
 * until something has been sent for it, so while it is pending, or approved
 * with no target yet. (The server also refuses the moment a send is in flight.)
 * Callers must already know the viewer owns the request.
 */
export function canCancelOwnRequest(
  request: Pick<MediaRequest, "status" | "outcome" | "targets">,
): boolean {
  if (request.outcome !== "active") return false;
  if (request.status === "pending") return true;
  return request.status === "approved" && (request.targets?.length ?? 0) === 0;
}

export function requestDetailHref(mediaType: RequestMediaType, tmdbID: number): string {
  return `/requests/${mediaType}/${tmdbID}`;
}

/** Request suggestions the ⌘K dialog lists below the library results. */
export const REQUEST_DIALOG_SUGGESTION_LIMIT = 4;

/** Search results worth suggesting as requests: titles not already in the library. */
export function requestSuggestions(
  results: RequestMediaResult[] | undefined,
  limit: number,
): RequestMediaResult[] {
  return (results ?? []).filter((item) => item.availability !== "available").slice(0, limit);
}

export function formatRequestReason(reason?: string): string {
  switch (reason) {
    case "already_requested":
      return "Already requested";
    case "already_available":
      return "Available";
    case "requests_disabled":
      return "Requests disabled";
    case "blocked":
      return "Blocked";
    case "quota_exceeded":
      return "Request limit reached";
    default:
      return "Unavailable";
  }
}

export function tmdbImageURL(path?: string, size = "w342"): string | null {
  if (!path) return null;
  return `https://image.tmdb.org/t/p/${size}${path}`;
}

export function requestInputFromMediaResult(item: RequestMediaResult): CreateMediaRequestInput {
  return {
    media_type: item.media_type,
    tmdb_id: item.tmdb_id,
    title: item.title,
    year: item.year || undefined,
    overview: item.overview || undefined,
    poster_path: item.poster_path || undefined,
    backdrop_path: item.backdrop_path || undefined,
  };
}

export function formatRequestDate(request: Pick<MediaRequest, "created_at">): string {
  return formatDate(request.created_at, "medium");
}

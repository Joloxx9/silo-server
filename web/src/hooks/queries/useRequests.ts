import { useMutation, useQuery, useQueryClient, type Query } from "@tanstack/react-query";
import { toast } from "sonner";
import { V2ProblemError } from "@/api/v2/request";
import { v2 } from "@/api/v2/request";
import {
  browseDiscoverV2,
  cancelMediaRequestV2,
  createMediaRequestV2,
  followRequestMediaV2,
  getDiscoverSectionV2,
  getRequestMediaDetailV2,
  unfollowRequestMediaV2,
  listDiscoverGenresV2,
  listDiscoverNetworksV2,
  listDiscoverSectionsV2,
  listDiscoverStudiosV2,
  listMyMediaRequestsV2,
  searchRequestMediaV2,
} from "@/api/v2/requests";
import { useCurrentProfile } from "@/hooks/useCurrentProfile";
import type {
  CreateMediaRequestInput,
  DiscoverBrowseKind,
  RequestDiscoverySection,
  RequestListParams,
  RequestMediaPage,
  RequestSearchMediaType,
  RequestMediaType,
} from "@/api/types";
import { adminKeys, requestKeys } from "./keys";

export const REQUESTS_STALE_TIME = 30_000;
const DISCOVER_BRAND_STALE_TIME = 24 * 60 * 60 * 1000;
const BROWSE_STALE_TIME = 60 * 1000;

function listParamsKey(params: RequestListParams) {
  return {
    status: params.status ?? "all",
    outcome: params.outcome ?? "all",
    limit: params.limit ?? 50,
    offset: params.offset ?? 0,
  };
}

export function isValidationFailure(err: unknown): boolean {
  return err instanceof V2ProblemError && err.problemType === "validation_failed";
}

export function invalidateRequestSurfaces(queryClient: ReturnType<typeof useQueryClient>) {
  // requestKeys.all = ["requests"], so invalidating it cascades to nested keys,
  // including requestKeys.search(...). Policy mutations rely on this to refresh
  // viewer-scoped search results when request eligibility changes.
  queryClient.invalidateQueries({ queryKey: requestKeys.all });
  queryClient.invalidateQueries({ queryKey: adminKeys.requestsRoot() });
}

export function useRequestDiscovery() {
  return useQuery({
    queryKey: requestKeys.discovery(),
    queryFn: listDiscoverSectionsV2,
    staleTime: REQUESTS_STALE_TIME,
  });
}

export function useRequestFeatureStatus(
  options: { enabled?: boolean; refetchOnMount?: boolean } = {},
) {
  return useQuery({
    queryKey: requestKeys.status(),
    queryFn: () => v2("GET /api/v2/requests/status"),
    staleTime: REQUESTS_STALE_TIME,
    enabled: options.enabled ?? true,
    ...(options.refetchOnMount !== undefined && { refetchOnMount: options.refetchOnMount }),
  });
}

export function useRequestDiscoverySection(section: string, page = 1) {
  return useQuery<RequestDiscoverySection>({
    queryKey: requestKeys.discoverySection(section, page),
    queryFn: () => getDiscoverSectionV2(section, page),
    enabled: section.trim().length > 0,
    staleTime: REQUESTS_STALE_TIME,
    // Paging keeps the current grid on screen; another row starts empty.
    placeholderData: (
      previous: RequestDiscoverySection | undefined,
      previousQuery?: Query<RequestDiscoverySection>,
    ) => (previousQuery?.queryKey[2] === section ? previous : undefined),
  });
}

export interface DiscoverBrandQueryOptions {
  /** When false, the list is not read. Default: true. */
  enabled?: boolean;
  /** Retry policy; the admin routing editor passes false so a refusal is not repeated. */
  retry?: boolean;
}

export function useDiscoverStudios(options: DiscoverBrandQueryOptions = {}) {
  return useQuery({
    queryKey: requestKeys.discoverStudios(),
    queryFn: listDiscoverStudiosV2,
    staleTime: DISCOVER_BRAND_STALE_TIME,
    enabled: options.enabled ?? true,
    ...(options.retry !== undefined ? { retry: options.retry } : {}),
  });
}

export function useDiscoverNetworks(options: DiscoverBrandQueryOptions = {}) {
  return useQuery({
    queryKey: requestKeys.discoverNetworks(),
    queryFn: listDiscoverNetworksV2,
    staleTime: DISCOVER_BRAND_STALE_TIME,
    enabled: options.enabled ?? true,
    ...(options.retry !== undefined ? { retry: options.retry } : {}),
  });
}

export function useDiscoverGenres() {
  return useQuery({
    queryKey: requestKeys.discoverGenres(),
    queryFn: listDiscoverGenresV2,
    staleTime: DISCOVER_BRAND_STALE_TIME,
  });
}

export interface UseRequestBrowseArgs {
  kind: DiscoverBrowseKind;
  slug: string;
  mediaType?: RequestMediaType;
  sort: "popularity" | "vote_average" | "release_date";
  page: number;
}

export function useRequestBrowse({ kind, slug, mediaType, sort, page }: UseRequestBrowseArgs) {
  return useQuery({
    queryKey: requestKeys.discoverBrowse(kind, slug, mediaType, sort, page),
    queryFn: () => browseDiscoverV2({ kind, slug, mediaType, sort, page }),
    enabled: slug.trim().length > 0 && (kind !== "genre" || Boolean(mediaType)),
    staleTime: BROWSE_STALE_TIME,
  });
}

export function useRequestMediaDetail(
  mediaType: RequestMediaType,
  tmdbID: number,
  options: { enabled?: boolean } = {},
) {
  return useQuery({
    queryKey: requestKeys.detail(mediaType, tmdbID),
    queryFn: () => getRequestMediaDetailV2(mediaType, tmdbID),
    enabled: tmdbID > 0 && (options.enabled ?? true),
    staleTime: REQUESTS_STALE_TIME,
  });
}

export interface UseRequestSearchOptions {
  /** When false, suppresses the query regardless of the query string. Default: true. */
  enabled?: boolean;
  /** When true, suppresses the query until the active profile is loaded. Default: false. */
  requireProfile?: boolean;
  /** Cache freshness window for this search surface. Default: existing Requests page timing. */
  staleTime?: number;
  /** Inactive cache lifetime for rapidly changing interactive search keys. */
  gcTime?: number;
  /** Retry policy; interactive search surfaces should not replay expensive failures. */
  retry?: boolean | number;
  /**
   * Keeps showing the previous page's results while another page of the same
   * search loads, so a paged grid does not collapse between pages.
   */
  keepPreviousPage?: boolean;
}

export function useRequestSearch(
  mediaType: RequestSearchMediaType,
  query: string,
  page = 1,
  options: UseRequestSearchOptions = {},
) {
  const normalizedQuery = query.trim();
  const { profile } = useCurrentProfile();
  // Use a sentinel viewerKey when there is no profile so the cache key is stable,
  // but suppress the actual fetch — see the `enabled` gate below. This prevents
  // any anonymous request results from being written into a bucket that could
  // later be read by a different viewer.
  const viewerKey = profile?.id ?? "anon";
  const enabledOverride = options.enabled ?? true;
  const requireProfile = options.requireProfile ?? false;

  const queryKey = requestKeys.search(mediaType, normalizedQuery, page, viewerKey);
  return useQuery<RequestMediaPage>({
    queryKey,
    queryFn: ({ signal }) => searchRequestMediaV2(mediaType, normalizedQuery, page, signal),
    enabled:
      enabledOverride && normalizedQuery.length > 1 && (!requireProfile || Boolean(profile?.id)),
    staleTime: options.staleTime ?? REQUESTS_STALE_TIME,
    ...(options.gcTime !== undefined ? { gcTime: options.gcTime } : {}),
    ...(options.retry !== undefined ? { retry: options.retry } : {}),
    // Only another page of the same search: a new query or type must not show
    // the old one's titles.
    placeholderData: options.keepPreviousPage
      ? (previous: RequestMediaPage | undefined, previousQuery?: Query<RequestMediaPage>) =>
          previousQuery && sameSearchOtherPage(previousQuery.queryKey, queryKey)
            ? previous
            : undefined
      : undefined,
  });
}

function sameSearchOtherPage(previous: readonly unknown[], next: readonly unknown[]): boolean {
  // requestKeys.search: ["requests", "search", viewerKey, mediaType, query, page]
  return previous.slice(0, 5).every((part, index) => part === next[index]);
}

export function useCreateMediaRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: (body: CreateMediaRequestInput) => createMediaRequestV2(body),
    onSuccess: () => {
      toast.success("Request submitted");
      invalidateRequestSurfaces(queryClient);
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to submit request");
    },
  });
}

/** Follows or unfollows a title someone else has already requested. */
export function useToggleRequestFollow() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: ({
      mediaType,
      tmdbID,
      follow,
    }: {
      mediaType: RequestMediaType;
      tmdbID: number;
      follow: boolean;
    }) =>
      follow
        ? followRequestMediaV2(mediaType, tmdbID).then(() => undefined)
        : unfollowRequestMediaV2(mediaType, tmdbID),
    onSuccess: (_data, { follow }) => {
      toast.success(follow ? "We'll let you know when it's available" : "Notification turned off");
      invalidateRequestSurfaces(queryClient);
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to update notification");
    },
  });
}

export function useCancelMediaRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: (id: string) => cancelMediaRequestV2(id),
    onSuccess: () => {
      toast.success("Request cancelled");
    },
    // A refused action still refreshes the queue: another admin may have
    // acted first, and the row should show what happened.
    onSettled: () => invalidateRequestSurfaces(queryClient),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to cancel request");
    },
  });
}

export function useMyMediaRequests(
  params: RequestListParams = {},
  options: { enabled?: boolean } = {},
) {
  const key = listParamsKey(params);
  return useQuery({
    queryKey: requestKeys.mine(key),
    queryFn: () => listMyMediaRequestsV2(params),
    enabled: options.enabled ?? true,
    staleTime: REQUESTS_STALE_TIME,
  });
}

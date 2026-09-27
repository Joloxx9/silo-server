import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type Query,
} from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { V2ProblemError } from "@/api/v2/request";
import { captureProfileRequestContext } from "@/api/client";
import { adminAuthorityScope, type AdminAuthority } from "@/api/v2/adminAuthority";
import {
  getAdminRequestSettingsV2,
  putAdminRequestSettingsV2,
  getAdminRequestUserLimitV2,
  putAdminRequestUserLimitV2,
  getAdminRequestGroupLimitV2,
  putAdminRequestGroupLimitV2,
  listAdminRequestIntegrationsV2,
  saveAdminRequestIntegrationV2,
  deleteAdminRequestIntegrationV2,
  listAdminRequestQueuePageV2,
  getAdminRequestCountsV2,
  listAdminRequestEventsV2,
  approveAdminRequestV2,
  cancelAdminRequestV2,
  declineAdminRequestV2,
  retryAdminRequestV2,
  loadAdminRequestIntegrationOptionsV2,
  listAdminRequestRoutesV2,
  createAdminRequestRouteV2,
  updateAdminRequestRouteV2,
  deleteAdminRequestRouteV2,
  reorderAdminRequestRoutesV2,
  previewAdminRequestRouteV2,
  type AdminRequestQueueFilter,
  type RequestGroupLimit,
  type RequestGroupLimitBody,
  type RequestRoute,
  type RequestRouteBody,
  type RequestRouteMediaType,
} from "@/api/v2/adminRequests";
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
  LoadRequestIntegrationOptionsRequest,
  MediaRequest,
  RequestIntegration,
  RequestDiscoverySection,
  RequestListParams,
  RequestMediaPage,
  RequestSearchMediaType,
  RequestMediaType,
  RequestUserLimit,
} from "@/api/types";
import { adminKeys, requestKeys } from "./keys";

const REQUESTS_STALE_TIME = 30_000;
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

function isValidationFailure(err: unknown): boolean {
  return err instanceof V2ProblemError && err.problemType === "validation_failed";
}

function invalidateRequestSurfaces(queryClient: ReturnType<typeof useQueryClient>) {
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

const ADMIN_QUEUE_STALE_TIME = 10_000;
/** Rows per queue page; the server answers at most 50. */
export const ADMIN_QUEUE_PAGE_SIZE = 25;
/** How often the view counts (and so the admin nav's badge) are read again. */
export const ADMIN_REQUEST_COUNTS_INTERVAL = 60_000;

function requestQueueKey(filter: AdminRequestQueueFilter) {
  return adminKeys.requestQueue({
    view: filter.view,
    q: filter.q?.trim() ?? "",
    mediaType: filter.mediaType ?? "all",
    requestedByUserId: filter.requestedByUserId ?? null,
  });
}

/**
 * One queue view, a page at a time. A new search or type filter keeps the
 * rows on screen until its first page arrives; a new view does not, since its
 * rows take different actions. With `enabled: false` it only reads the rows
 * another reader of the same view loads, and follows their refetches.
 */
export function useAdminRequestQueue(
  filter: AdminRequestQueueFilter,
  options: { enabled?: boolean } = {},
) {
  return useInfiniteQuery({
    queryKey: requestQueueKey(filter),
    enabled: options.enabled ?? true,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listAdminRequestQueuePageV2(filter, {
        limit: ADMIN_QUEUE_PAGE_SIZE,
        cursor: pageParam,
        signal,
      }),
    // A cursor the list already visited would loop; stop there.
    getNextPageParam: (last, _pages, _lastParam, params) =>
      last.nextCursor && !params.includes(last.nextCursor) ? last.nextCursor : undefined,
    placeholderData: (previous, previousQuery) =>
      (previousQuery?.queryKey[3] as { view?: string } | undefined)?.view === filter.view
        ? previous
        : undefined,
    staleTime: ADMIN_QUEUE_STALE_TIME,
  });
}

/** How many requests each queue view holds; polled for the admin nav badge. */
export function useAdminRequestCounts(options: { enabled?: boolean } = {}) {
  return useQuery({
    queryKey: adminKeys.requestCounts(),
    queryFn: getAdminRequestCountsV2,
    enabled: options.enabled ?? true,
    staleTime: ADMIN_QUEUE_STALE_TIME,
    // A server without request administration, or an admin session that lost
    // its rights, answers the same every time; any other error (a node
    // restarting) is worth asking again.
    refetchInterval: (query) =>
      query.state.error instanceof V2ProblemError &&
      ["permission_denied", "dependency_unavailable"].includes(query.state.error.problemType)
        ? false
        : ADMIN_REQUEST_COUNTS_INTERVAL,
    retry: false,
  });
}

/** Reads the queue's rows and view counts again. */
export function useRefreshRequestQueue() {
  const queryClient = useQueryClient();
  const [isRefreshing, setRefreshing] = useState(false);
  return {
    isRefreshing,
    refresh: () => {
      setRefreshing(true);
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: adminKeys.requestQueueRoot() }),
        queryClient.invalidateQueries({ queryKey: adminKeys.requestCounts() }),
      ]).finally(() => setRefreshing(false));
    },
  };
}

/** A request's history, newest first. */
export function useAdminRequestEvents(id: string | undefined) {
  return useQuery({
    queryKey: adminKeys.requestEvents(id ?? ""),
    queryFn: () => listAdminRequestEventsV2(id!),
    enabled: Boolean(id),
    staleTime: ADMIN_QUEUE_STALE_TIME,
  });
}

/** Where a request's quality tiers would go if it were sent now. */
export function useAdminRequestRoutePreview(
  target: { mediaType: RequestRouteMediaType; tmdbId: number; requesterUserId?: number } | null,
) {
  return useQuery({
    queryKey: adminKeys.requestRoutePreview(
      target?.mediaType ?? "",
      target?.tmdbId ?? 0,
      target?.requesterUserId,
    ),
    queryFn: () =>
      previewAdminRequestRouteV2(target!.mediaType, target!.tmdbId, target!.requesterUserId),
    enabled: target !== null,
    staleTime: REQUESTS_STALE_TIME,
    retry: false,
  });
}

export function useApproveMediaRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: (id: string) => approveAdminRequestV2(id),
    onSuccess: () => {
      toast.success("Request approved");
    },
    // A refused action still refreshes the queue: another admin may have
    // acted first, and the row should show what happened.
    onSettled: () => invalidateRequestSurfaces(queryClient),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to approve request");
    },
  });
}

export function useDeclineMediaRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: ({ id, reason }: { id: string; reason?: string }) =>
      declineAdminRequestV2(id, reason),
    onSuccess: () => {
      toast.success("Request declined");
    },
    // A refused action still refreshes the queue: another admin may have
    // acted first, and the row should show what happened.
    onSettled: () => invalidateRequestSurfaces(queryClient),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to decline request");
    },
  });
}

export function useRetryMediaRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: (id: string) => retryAdminRequestV2(id),
    onSuccess: () => {
      toast.success("Request queued for retry");
    },
    // A refused action still refreshes the queue: another admin may have
    // acted first, and the row should show what happened.
    onSettled: () => invalidateRequestSurfaces(queryClient),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to retry request");
    },
  });
}

/** An admin withdraws a request nothing has been sent for yet, or closes a failed one. */
export function useAdminCancelMediaRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: ({ id, reason }: { id: string; reason?: string }) =>
      cancelAdminRequestV2(id, reason),
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

export type BulkRequestAction = "approve" | "decline";

/** Requests a bulk action sends at once; the rest wait for a free slot. */
export const BULK_REQUEST_CONCURRENCY = 4;

export interface BulkRequestFailure {
  id: string;
  title: string;
  message: string;
}

export interface BulkRequestProgress {
  action: BulkRequestAction;
  total: number;
  /** Requests answered so far, failed ones included. */
  done: number;
  failures: BulkRequestFailure[];
}

async function forEachWithConcurrency<T>(
  items: readonly T[],
  limit: number,
  run: (item: T) => Promise<void>,
) {
  let next = 0;
  const worker = async () => {
    while (next < items.length) await run(items[next++]!);
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker));
}

/**
 * Approves or declines several requests through the per-request endpoints, a
 * few at a time. One refusal does not stop the rest: `progress` counts every
 * answer and keeps each failure with its reason. The queue and counts are
 * read again once, when the last answer is in.
 */
export function useBulkRequestAction() {
  const queryClient = useQueryClient();
  const [progress, setProgress] = useState<BulkRequestProgress | null>(null);
  const mutation = useMutation({
    retry: false,
    mutationFn: async ({
      action,
      requests,
      reason,
    }: {
      action: BulkRequestAction;
      requests: readonly Pick<MediaRequest, "id" | "title">[];
      reason?: string;
    }) => {
      const failures: BulkRequestFailure[] = [];
      let done = 0;
      setProgress({ action, total: requests.length, done, failures: [] });
      await forEachWithConcurrency(requests, BULK_REQUEST_CONCURRENCY, async (request) => {
        try {
          if (action === "approve") await approveAdminRequestV2(request.id);
          else await declineAdminRequestV2(request.id, reason);
        } catch (err) {
          failures.push({
            id: request.id,
            title: request.title,
            message: problemMessage(err, `Failed to ${action} request`),
          });
        }
        done += 1;
        setProgress({ action, total: requests.length, done, failures: [...failures] });
      });
      return { action, total: requests.length, failures };
    },
    onSuccess: ({ action, total, failures }) => {
      const verb = action === "approve" ? "approved" : "declined";
      const succeeded = total - failures.length;
      if (failures.length === 0) {
        toast.success(`${succeeded} ${succeeded === 1 ? "request" : "requests"} ${verb}`);
      } else {
        toast.error(`${failures.length} of ${total} requests couldn't be ${verb}`);
      }
    },
    onSettled: () => invalidateRequestSurfaces(queryClient),
  });
  return {
    run: mutation.mutate,
    isRunning: mutation.isPending,
    progress,
    /** Forgets the last run's progress and failures. */
    reset: () => {
      mutation.reset();
      setProgress(null);
    },
  };
}

export function useRequestSettings() {
  return useQuery({
    queryKey: adminKeys.requestSettings(),
    queryFn: getAdminRequestSettingsV2,
    staleTime: REQUESTS_STALE_TIME,
  });
}

export function useUpdateRequestSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: putAdminRequestSettingsV2,
    onSuccess: (saved) => {
      toast.success("Request settings saved");
      // The editor adopts the saved record; the cache has to hold it first,
      // or a clean editor would follow the query back to the replaced one.
      queryClient.setQueryData(adminKeys.requestSettings(), saved);
      queryClient.invalidateQueries({ queryKey: requestKeys.status() });
      invalidateRequestSurfaces(queryClient);
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to save request settings");
      // A refused save (412) means someone else saved; read their version so
      // a discarded draft starts from it.
      queryClient.invalidateQueries({ queryKey: adminKeys.requestSettings() });
    },
  });
}

export function useRequestIntegrations() {
  return useQuery({
    queryKey: adminKeys.requestIntegrations(),
    queryFn: listAdminRequestIntegrationsV2,
    staleTime: REQUESTS_STALE_TIME,
  });
}

// A saved server's connection may have changed, so the root folders, quality
// profiles, and tags read from it are stale too.
function invalidateRequestServers(queryClient: ReturnType<typeof useQueryClient>) {
  queryClient.invalidateQueries({ queryKey: adminKeys.requestIntegrations() });
  queryClient.invalidateQueries({ queryKey: adminKeys.requestIntegrationOptionsRoot() });
  invalidateRequestSurfaces(queryClient);
}

export function useCreateRequestIntegration() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: (integration: RequestIntegration) =>
      saveAdminRequestIntegrationV2(integration, true),
    onSuccess: () => {
      toast.success("Server added");
      invalidateRequestServers(queryClient);
    },
    onError: (err) => {
      if (isValidationFailure(err)) return;
      toast.error(err instanceof Error ? err.message : "Failed to add server");
    },
  });
}

export function useUpdateRequestIntegration() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: (integration: RequestIntegration) => saveAdminRequestIntegrationV2(integration),
    onSuccess: () => {
      toast.success("Server saved");
      invalidateRequestServers(queryClient);
    },
    onError: (err) => {
      if (isValidationFailure(err)) return;
      toast.error(err instanceof Error ? err.message : "Failed to save server");
    },
  });
}

export function useDeleteRequestIntegration() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: deleteAdminRequestIntegrationV2,
    onSuccess: () => {
      toast.success("Server deleted");
      invalidateRequestServers(queryClient);
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to delete server");
    },
  });
}

/**
 * The root folders, quality profiles, and tags a saved server offers, read
 * through the server's own stored connection. A routing destination picks its
 * overrides from these.
 */
export function useRequestIntegrationOptions(integrationId: string | undefined) {
  return useQuery({
    queryKey: adminKeys.requestIntegrationOptions(integrationId ?? ""),
    queryFn: () => loadAdminRequestIntegrationOptionsV2(integrationId!, { base_url: "" }),
    enabled: Boolean(integrationId),
    staleTime: 5 * 60 * 1000,
    retry: false,
  });
}

export function useRequestRoutes(enabled = true) {
  return useQuery({
    queryKey: adminKeys.requestRoutes(),
    queryFn: listAdminRequestRoutesV2,
    staleTime: REQUESTS_STALE_TIME,
    enabled,
  });
}

// Reordering, like any save, advances the revision of every route it touches,
// so the whole list is read again rather than patched in place. The promise
// is returned so a mutation stays pending until the list is fresh: a second
// reorder or toggle computed from the old list would undo the first or carry
// a replaced validator.
function invalidateRequestRoutes(queryClient: ReturnType<typeof useQueryClient>) {
  return queryClient.invalidateQueries({ queryKey: adminKeys.requestRoutes() });
}

/** Writes a saved route into the list, so an editor adopting it is not pulled back. */
function storeRequestRoute(queryClient: ReturnType<typeof useQueryClient>, saved: RequestRoute) {
  queryClient.setQueryData<RequestRoute[]>(adminKeys.requestRoutes(), (routes) =>
    routes?.map((route) => (route.id === saved.id ? saved : route)),
  );
}

/** A problem in one line: its detail and every field error it carries. */
function problemMessage(err: unknown, fallback: string): string {
  if (!(err instanceof Error)) return fallback;
  if (!(err instanceof V2ProblemError)) return err.message;
  const details = [err.message, ...(err.problem.errors ?? []).map((e) => e.detail)];
  return [...new Set(details.filter(Boolean))].join(" ");
}

/**
 * Toasts a failed route write. A caller that shows validation errors beside
 * its fields passes `inline`; anyone else (a toggle, a reorder) would
 * otherwise fail without a word.
 */
function routeMutationError(err: unknown, fallback: string, inline = false) {
  if (inline && isValidationFailure(err)) return;
  toast.error(problemMessage(err, fallback));
}

/** Adds a rule from the rule editor, which shows validation errors itself. */
export function useCreateRequestRoute() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: (body: RequestRouteBody) => createAdminRequestRouteV2(body),
    onSuccess: () => {
      toast.success("Rule added");
      return invalidateRequestRoutes(queryClient);
    },
    onError: (err) => routeMutationError(err, "Failed to add rule", true),
  });
}

/** `inlineErrors`: the caller shows validation errors beside its fields. */
export function useUpdateRequestRoute({ inlineErrors = false }: { inlineErrors?: boolean } = {}) {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: ({
      route,
      body,
    }: {
      route: Pick<RequestRoute, "id" | "etag">;
      body: RequestRouteBody;
    }) => updateAdminRequestRouteV2(route, body),
    onSuccess: (saved) => {
      toast.success(saved.is_fallback ? "Default destination saved" : "Rule saved");
      storeRequestRoute(queryClient, saved);
    },
    onError: (err) => routeMutationError(err, "Failed to save routing", inlineErrors),
    // After a refused save too: the list then carries the other admin's version.
    onSettled: () => invalidateRequestRoutes(queryClient),
  });
}

export function useDeleteRequestRoute() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: (route: Pick<RequestRoute, "id" | "etag">) => deleteAdminRequestRouteV2(route),
    onSuccess: () => {
      toast.success("Rule deleted");
    },
    onError: (err) => routeMutationError(err, "Failed to delete rule"),
    onSettled: () => invalidateRequestRoutes(queryClient),
  });
}

export function useReorderRequestRoutes() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: ({ mediaType, ids }: { mediaType: RequestRouteMediaType; ids: string[] }) =>
      reorderAdminRequestRoutesV2(mediaType, ids),
    onError: (err) => routeMutationError(err, "Failed to reorder rules"),
    onSettled: () => invalidateRequestRoutes(queryClient),
  });
}

/** Asks where each quality tier of a title would go; failures are shown inline. */
export function usePreviewRequestRoute() {
  return useMutation({
    retry: false,
    mutationFn: ({ mediaType, tmdbId }: { mediaType: RequestRouteMediaType; tmdbId: number }) =>
      previewAdminRequestRouteV2(mediaType, tmdbId),
  });
}

export function useLoadRequestIntegrationOptions() {
  return useMutation({
    retry: false,
    mutationFn: ({ id, body }: { id: string; body: LoadRequestIntegrationOptionsRequest }) =>
      loadAdminRequestIntegrationOptionsV2(id, body),
    // Silent background probe: callers surface load failures inline (no toast).
  });
}

export function useRequestUserLimit(userId?: number) {
  return useQuery({
    queryKey: adminKeys.requestUserLimit(userId ?? 0),
    queryFn: () => getAdminRequestUserLimitV2(userId!),
    enabled: Boolean(userId && userId > 0),
    staleTime: REQUESTS_STALE_TIME,
  });
}

export function useUpdateRequestUserLimit() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: ({ userId, body }: { userId: number; body: RequestUserLimit }) =>
      putAdminRequestUserLimitV2(userId, body),
    onSuccess: (saved, variables) => {
      toast.success("Request settings saved");
      // The editor adopts the saved record; the cache has to hold it first,
      // or a clean editor would follow the query back to the replaced one.
      queryClient.setQueryData(adminKeys.requestUserLimit(variables.userId), saved);
      invalidateRequestSurfaces(queryClient);
    },
    onError: (err, variables) => {
      toast.error(err instanceof Error ? err.message : "Failed to save the request settings");
      // A refused save (412) means someone else saved; read their version so
      // an explicit reload starts from it.
      queryClient.invalidateQueries({ queryKey: adminKeys.requestUserLimit(variables.userId) });
    },
  });
}

/**
 * An access group's request approval and limit. An editor passes the
 * authority it read the group under, so the limit it saves carries a
 * validator from the same profile.
 */
export function useRequestGroupLimit(groupId?: number | null, authority?: AdminAuthority) {
  const context = authority ?? captureProfileRequestContext();
  return useQuery({
    queryKey: adminKeys.requestGroupLimit(groupId ?? 0, adminAuthorityScope(context)),
    queryFn: () => getAdminRequestGroupLimitV2(groupId!, context ?? undefined),
    enabled: Boolean(groupId && groupId > 0),
    staleTime: REQUESTS_STALE_TIME,
    retry: false,
  });
}

/**
 * Saves an access group's request approval and limit. Silent: the group
 * editor saves it together with the group and reports both.
 */
export function useUpdateRequestGroupLimit() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: ({
      limit,
      body,
      profileContext,
    }: {
      limit: Pick<RequestGroupLimit, "group_id" | "etag">;
      body: RequestGroupLimitBody;
      /** The authority the limit was read under; the active one when omitted. */
      profileContext?: AdminAuthority;
    }) => putAdminRequestGroupLimitV2(limit, body, profileContext),
    onSuccess: (saved, { profileContext }) => {
      queryClient.setQueryData(
        adminKeys.requestGroupLimit(saved.group_id, adminAuthorityScope(profileContext)),
        saved,
      );
      invalidateRequestSurfaces(queryClient);
    },
    onError: (_err, { limit, profileContext }) => {
      queryClient.invalidateQueries({
        queryKey: adminKeys.requestGroupLimit(limit.group_id, adminAuthorityScope(profileContext)),
      });
    },
  });
}

export function useAdminRequestCapabilities() {
  return useQuery({
    queryKey: [...adminKeys.requestsRoot(), "capabilities"],
    queryFn: () => v2("GET /api/v2/admin/requests/capabilities"),
    staleTime: REQUESTS_STALE_TIME,
  });
}

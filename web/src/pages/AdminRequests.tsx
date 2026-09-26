import { isRequestEditorConflict } from "@/api/v2/adminRequests";
import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import { Link, Navigate, useSearchParams } from "react-router";
import {
  AlertTriangle,
  Check,
  Library,
  RefreshCw,
  Save,
  Settings2,
  SlidersHorizontal,
  X,
} from "lucide-react";
import type {
  MediaRequest,
  MediaRequestOutcome,
  MediaRequestStatus,
  RequestApprovalMode,
  RequestLimitMode,
  RequestTarget,
  RequestUserLimit,
} from "@/api/types";
import { EditorConflict } from "@/components/admin/EditorConflict";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useAdminUsers } from "@/hooks/queries/admin/users";
import {
  useAdminMediaRequests,
  useAdminRequestCapabilities,
  useApproveMediaRequest,
  useDeclineMediaRequest,
  useRequestUserLimit,
  useRetryMediaRequest,
  useUpdateRequestUserLimit,
} from "@/hooks/queries/useRequests";
import {
  formatMediaType,
  formatSeasonList,
  formatRequestDate,
  formatRequestOutcome,
  formatRequestStatus,
  requestDetailHref,
  requestOutcomeBadgeVariant,
  requestStatusBadgeVariant,
  REQUEST_OUTCOMES,
  REQUEST_STATUSES,
} from "@/lib/mediaRequests";

type StatusFilter = MediaRequestStatus | "all";
type OutcomeFilter = MediaRequestOutcome | "all";

const ADMIN_REQUEST_TABS = ["queue", "overrides"] as const;
type AdminRequestTab = (typeof ADMIN_REQUEST_TABS)[number];

/** Where request settings, servers, and routing live now. */
export const REQUEST_SETTINGS_HREF = "/admin/settings/requests";

// Tabs this page used to have. Their content moved to Settings → Requests, so
// bookmarks to them land there instead of on the queue.
const MOVED_TABS = new Set(["settings", "integrations"]);

function normalizeAdminRequestTab(value: string | null): AdminRequestTab {
  return ADMIN_REQUEST_TABS.includes(value as AdminRequestTab)
    ? (value as AdminRequestTab)
    : "queue";
}

export default function AdminRequests() {
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedTab = searchParams.get("tab");
  const activeTab = normalizeAdminRequestTab(requestedTab);
  const capabilities = useAdminRequestCapabilities();

  function setActiveTab(value: string) {
    const nextTab = normalizeAdminRequestTab(value);
    const next = new URLSearchParams(searchParams);

    if (nextTab === "queue") {
      next.delete("tab");
    } else {
      next.set("tab", nextTab);
    }

    setSearchParams(next, { replace: true });
  }

  if (requestedTab !== null && MOVED_TABS.has(requestedTab)) {
    return <Navigate to={REQUEST_SETTINGS_HREF} replace />;
  }
  if (capabilities.isLoading) return <RowsSkeleton />;
  if (
    !capabilities.data?.available ||
    (activeTab !== "queue" && !capabilities.data.guarded_configuration)
  ) {
    return (
      <EmptyPanel
        title="Request administration unavailable"
        detail="Request administration could not be enabled on this server."
      />
    );
  }

  return (
    <div className="space-y-6">
      {/* The action sits under the text, as on the other admin pages: the
          shell's search button floats over the header's top-right corner. */}
      <div className="page-header">
        <div className="space-y-2">
          <h1 className="text-3xl font-semibold tracking-normal text-balance sm:text-4xl">
            Requests
          </h1>
          <p className="text-muted-foreground max-w-2xl text-sm leading-6">
            Review media requests and give one account its own request limit.
          </p>
          <Button asChild variant="outline" size="sm">
            <Link to={REQUEST_SETTINGS_HREF}>
              <Settings2 aria-hidden="true" />
              Request settings
            </Link>
          </Button>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="gap-5">
        <div className="-mx-4 overflow-x-auto px-4 pb-1 [scrollbar-width:thin] sm:mx-0 sm:px-0">
          <TabsList
            variant="line"
            aria-label="Request administration sections"
            className="border-border w-max min-w-full justify-start border-b"
          >
            <TabsTrigger value="queue">Queue</TabsTrigger>
            <TabsTrigger value="overrides">User Overrides</TabsTrigger>
          </TabsList>
        </div>
        <TabsContent value="queue">
          <RequestQueueTab />
        </TabsContent>
        <TabsContent value="overrides">
          <UserOverridesTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function RequestQueueTab() {
  const [status, setStatus] = useState<StatusFilter>("all");
  const [outcome, setOutcome] = useState<OutcomeFilter>("all");
  const requests = useAdminMediaRequests({ status, outcome, limit: 100 });
  const users = useAdminUsers();
  const approve = useApproveMediaRequest();
  const decline = useDeclineMediaRequest();
  const retry = useRetryMediaRequest();
  const [declineTarget, setDeclineTarget] = useState<MediaRequest | null>(null);
  const [declineReason, setDeclineReason] = useState("");
  const usernamesByID = useMemo(() => {
    return new Map((users.data ?? []).map((user) => [user.id, user.username]));
  }, [users.data]);

  function handleDecline(request: MediaRequest) {
    setDeclineTarget(request);
    setDeclineReason("");
  }

  function confirmDecline() {
    if (!declineTarget) return;
    decline.mutate({ id: declineTarget.id, reason: declineReason });
    setDeclineTarget(null);
    setDeclineReason("");
  }

  return (
    <div className="space-y-4">
      <div className="border-border bg-card flex flex-wrap items-center gap-3 rounded-lg border p-3">
        <Select value={status} onValueChange={(value) => setStatus(value as StatusFilter)}>
          <SelectTrigger className="w-[170px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {REQUEST_STATUSES.map((value) => (
              <SelectItem key={value} value={value}>
                {value === "all" ? "All statuses" : formatRequestStatus(value)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={outcome} onValueChange={(value) => setOutcome(value as OutcomeFilter)}>
          <SelectTrigger className="w-[170px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {REQUEST_OUTCOMES.map((value) => (
              <SelectItem key={value} value={value}>
                {value === "all" ? "All outcomes" : formatRequestOutcome(value)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          variant="outline"
          size="sm"
          onClick={() => requests.refetch()}
          disabled={requests.isFetching}
        >
          <RefreshCw className="h-4 w-4" />
          Refresh
        </Button>
      </div>

      {requests.isLoading ? (
        <RowsSkeleton />
      ) : requests.isError ? (
        <EmptyPanel title="Requests failed" detail="The request queue could not be loaded." />
      ) : (
        <div className="border-border bg-card overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Title</TableHead>
                <TableHead>Requested</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Outcome</TableHead>
                <TableHead>Integration</TableHead>
                <TableHead className="w-[240px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(requests.data ?? []).length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-muted-foreground py-8 text-center">
                    No requests match the current filters.
                  </TableCell>
                </TableRow>
              ) : (
                requests.data?.map((request) => (
                  <RequestQueueRow
                    key={request.id}
                    request={request}
                    requesterUsername={
                      request.requested_by_user_id
                        ? usernamesByID.get(request.requested_by_user_id)
                        : undefined
                    }
                    approving={approve.isPending && approve.variables === request.id}
                    declining={decline.isPending && decline.variables?.id === request.id}
                    retrying={retry.isPending && retry.variables === request.id}
                    onApprove={() => approve.mutate(request.id)}
                    onDecline={() => handleDecline(request)}
                    onRetry={() => retry.mutate(request.id)}
                  />
                ))
              )}
            </TableBody>
          </Table>
        </div>
      )}
      <Dialog
        open={declineTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeclineTarget(null);
            setDeclineReason("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Decline request</DialogTitle>
            <DialogDescription>
              {declineTarget
                ? `"${declineTarget.title}" will be marked declined. Add an optional note for the requester.`
                : null}
            </DialogDescription>
          </DialogHeader>
          <Label htmlFor="decline-reason" className="text-sm">
            Reason (optional)
          </Label>
          <textarea
            id="decline-reason"
            className="border-input bg-background text-foreground focus-visible:ring-ring min-h-[88px] w-full rounded-md border px-3 py-2 text-sm focus-visible:ring-2 focus-visible:outline-none"
            value={declineReason}
            onChange={(event) => setDeclineReason(event.target.value)}
            placeholder="e.g. duplicate of an existing request"
          />
          <DialogFooter>
            <Button
              variant="ghost"
              onClick={() => {
                setDeclineTarget(null);
                setDeclineReason("");
              }}
            >
              Cancel
            </Button>
            <Button onClick={confirmDecline} disabled={decline.isPending}>
              Decline
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function RequestQueueRow({
  request,
  requesterUsername,
  approving,
  declining,
  retrying,
  onApprove,
  onDecline,
  onRetry,
}: {
  request: MediaRequest;
  requesterUsername?: string;
  approving: boolean;
  declining: boolean;
  retrying: boolean;
  onApprove: () => void;
  onDecline: () => void;
  onRetry: () => void;
}) {
  // The server declines only requests nothing has been sent for: pending ones,
  // and approved ones with no target yet (waiting for the library, or retrying
  // a failed send). It refuses one whose submission is in flight right now.
  const canApprove = request.status === "pending" && request.outcome === "active";
  const canDecline =
    canApprove ||
    (request.status === "approved" &&
      request.outcome === "active" &&
      (request.targets?.length ?? 0) === 0);
  const canRetry = request.outcome === "failed";
  const requesterLabel = requesterUsername ?? `User ${request.requested_by_user_id}`;

  return (
    <TableRow>
      <TableCell>
        <div className="min-w-[220px]">
          <div className="flex flex-wrap items-center gap-2">
            <Link
              to={requestDetailHref(request.media_type, request.tmdb_id)}
              className="font-medium hover:underline"
            >
              {request.title}
            </Link>
            <Badge variant="secondary">{formatMediaType(request.media_type)}</Badge>
            {request.seasons?.length ? (
              <Badge variant="outline">{formatSeasonList(request.seasons)}</Badge>
            ) : null}
          </div>
          <div className="text-muted-foreground mt-1 flex flex-wrap gap-x-3 text-xs">
            {request.year ? <span>{request.year}</span> : null}
            <span>TMDB {request.tmdb_id}</span>
            {request.requested_by_user_id ? (
              <Link
                to={`/admin/users/${request.requested_by_user_id}`}
                className="hover:text-foreground hover:underline"
              >
                {requesterLabel}
              </Link>
            ) : null}
            {request.library_content_id ? (
              <Link
                to={`/item/${encodeURIComponent(request.library_content_id)}`}
                className="hover:text-foreground inline-flex items-center gap-1 hover:underline"
              >
                <Library className="h-3 w-3" />
                Library
              </Link>
            ) : null}
          </div>
          {request.last_error ? (
            <p className="text-destructive mt-1 max-w-md text-xs">{request.last_error}</p>
          ) : null}
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground text-xs">{formatRequestDate(request)}</TableCell>
      <TableCell>
        <Badge variant={requestStatusBadgeVariant(request.status)}>
          {formatRequestStatus(request.status)}
        </Badge>
      </TableCell>
      <TableCell>
        <Badge variant={requestOutcomeBadgeVariant(request.outcome)}>
          {formatRequestOutcome(request.outcome)}
        </Badge>
      </TableCell>
      <TableCell className="text-muted-foreground text-xs">
        {request.targets?.length ? (
          <div className="flex flex-col gap-1.5">
            {request.is_anime ? (
              <Badge variant="secondary" className="w-fit">
                Anime
              </Badge>
            ) : null}
            {request.targets.map((target) => (
              <RequestTargetBadge key={target.id} target={target} />
            ))}
          </div>
        ) : (
          "Not submitted"
        )}
      </TableCell>
      <TableCell>
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={onApprove}
            disabled={!canApprove || approving}
          >
            <Check className="h-4 w-4" />
            Approve
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={onDecline}
            disabled={!canDecline || declining}
          >
            <X className="h-4 w-4" />
            Decline
          </Button>
          <Button size="sm" variant="outline" onClick={onRetry} disabled={!canRetry || retrying}>
            <RefreshCw className="h-4 w-4" />
            Retry
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}

function RequestTargetBadge({ target }: { target: RequestTarget }) {
  const qualityLabel = target.quality === "2160p" ? "2160p" : "1080p";
  const instanceLabel = target.instance_name || target.integration_kind || "Unknown";
  const failed = target.status === "failed";
  const statusLabel = target.status === "failed" ? "Failed" : formatRequestStatus(target.status);
  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-1.5">
        <Badge variant="outline">{qualityLabel}</Badge>
        <span className="text-foreground">{instanceLabel}</span>
        <Badge variant={failed ? "destructive" : "secondary"}>{statusLabel}</Badge>
        {target.external_status ? <span>{target.external_status}</span> : null}
      </div>
      {failed && target.last_error ? (
        <p className="text-destructive flex max-w-xs items-start gap-1">
          <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0" />
          <span>{target.last_error}</span>
        </p>
      ) : null}
    </div>
  );
}

type UserLimitFormState = {
  limit_mode: RequestLimitMode;
  max_requests: string;
  window_days: string;
  approval_mode: RequestApprovalMode;
};

function UserOverridesTab() {
  const users = useAdminUsers();
  const [selectedUserID, setSelectedUserID] = useState<number | undefined>();
  const effectiveUserID = selectedUserID ?? users.data?.[0]?.id;
  const limit = useRequestUserLimit(effectiveUserID);
  const [generation, setGeneration] = useState(0);

  const selectedUser = useMemo(
    () => users.data?.find((user) => user.id === effectiveUserID),
    [effectiveUserID, users.data],
  );

  if (users.isLoading) return <RowsSkeleton />;
  if (users.isError) {
    return <EmptyPanel title="Users failed" detail="Users could not be loaded." />;
  }

  return (
    <div className="border-border bg-card max-w-3xl space-y-5 rounded-lg border p-5">
      <div className="flex items-center gap-2">
        <SlidersHorizontal className="text-primary h-4 w-4" />
        <h2 className="text-lg font-semibold tracking-normal">User Overrides</h2>
      </div>

      <Field label="User">
        <Select
          value={effectiveUserID ? String(effectiveUserID) : ""}
          onValueChange={(value) => setSelectedUserID(Number(value))}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Select user" />
          </SelectTrigger>
          <SelectContent>
            {(users.data ?? []).map((user) => (
              <SelectItem key={user.id} value={String(user.id)}>
                {user.username}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>

      {limit.isLoading ? (
        <RowsSkeleton />
      ) : !limit.data || !effectiveUserID ? (
        <EmptyPanel title="Limit failed" detail="The selected user limit could not be loaded." />
      ) : (
        <UserLimitEditor
          key={`${effectiveUserID}:${generation}`}
          onReload={async () => {
            const result = await limit.refetch();
            if (result.isSuccess) setGeneration((n) => n + 1);
          }}
          userID={effectiveUserID}
          limit={limit.data}
          userAvailable={Boolean(selectedUser)}
        />
      )}
    </div>
  );
}

function UserLimitEditor({
  userID,
  limit,
  userAvailable,
  onReload,
}: {
  userID: number;
  limit: RequestUserLimit;
  userAvailable: boolean;
  onReload: () => Promise<void>;
}) {
  const updateLimit = useUpdateRequestUserLimit();
  const [etag, setETag] = useState(limit.etag);
  const [conflict, setConflict] = useState(false);
  const [form, setForm] = useState<UserLimitFormState>(() => ({
    limit_mode: limit.limit_mode,
    max_requests: limit.max_requests == null ? "" : String(limit.max_requests),
    window_days: limit.window_days == null ? "" : String(limit.window_days),
    approval_mode: limit.approval_mode,
  }));

  function saveLimit() {
    const custom = form.limit_mode === "custom";
    const payload: RequestUserLimit = {
      user_id: userID,
      etag,
      limit_mode: form.limit_mode,
      approval_mode: form.approval_mode,
      max_requests: custom ? Math.max(0, Number(form.max_requests) || 0) : undefined,
      window_days: custom ? Math.max(1, Number(form.window_days) || 1) : undefined,
    };
    updateLimit.mutate(
      { userId: userID, body: payload },
      {
        onSuccess: (saved) => setETag(saved.etag),
        onError: (error) => setConflict(isRequestEditorConflict(error)),
      },
    );
  }

  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Limit mode">
          <Select
            value={form.limit_mode}
            onValueChange={(value) =>
              setForm((current) => ({ ...current, limit_mode: value as RequestLimitMode }))
            }
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="inherit">Inherit</SelectItem>
              <SelectItem value="custom">Custom</SelectItem>
              <SelectItem value="unlimited">Unlimited</SelectItem>
              <SelectItem value="blocked">Blocked</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field label="Approval mode">
          <Select
            value={form.approval_mode}
            onValueChange={(value) =>
              setForm((current) => ({
                ...current,
                approval_mode: value as RequestApprovalMode,
              }))
            }
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="inherit">Inherit</SelectItem>
              <SelectItem value="manual">Manual</SelectItem>
              <SelectItem value="auto">Auto</SelectItem>
              <SelectItem value="blocked">Blocked</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        {form.limit_mode === "custom" ? (
          <>
            <Field label="Max requests">
              <Input
                type="number"
                min={0}
                value={form.max_requests}
                onChange={(event) =>
                  setForm((current) => ({ ...current, max_requests: event.target.value }))
                }
              />
            </Field>
            <Field label="Window days">
              <Input
                type="number"
                min={1}
                value={form.window_days}
                onChange={(event) =>
                  setForm((current) => ({ ...current, window_days: event.target.value }))
                }
              />
            </Field>
          </>
        ) : null}
      </div>

      {conflict ? <EditorConflict onReload={onReload} /> : null}
      <Button
        onClick={saveLimit}
        disabled={!userAvailable || updateLimit.isPending || conflict || !etag}
      >
        <Save className="h-4 w-4" />
        Save Override
      </Button>
    </>
  );
}

function Field({ label, children, error }: { label: string; children: ReactNode; error?: string }) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      {children}
      {error ? (
        <p role="alert" className="text-destructive text-xs">
          {error}
        </p>
      ) : null}
    </div>
  );
}

function RowsSkeleton() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 5 }).map((_, index) => (
        <Skeleton key={index} className="h-16 rounded-lg" />
      ))}
    </div>
  );
}

function EmptyPanel({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="border-border bg-card flex flex-col items-center justify-center gap-2 rounded-lg border px-4 py-10 text-center">
      <p className="text-sm font-semibold">{title}</p>
      <p className="text-muted-foreground max-w-sm text-sm">{detail}</p>
    </div>
  );
}

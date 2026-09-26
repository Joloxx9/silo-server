import { isRequestEditorConflict } from "@/api/v2/adminRequests";
import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import { Link, Navigate, useSearchParams } from "react-router";
import { Save, Settings2, SlidersHorizontal } from "lucide-react";
import type { RequestApprovalMode, RequestLimitMode, RequestUserLimit } from "@/api/types";
import { EditorConflict } from "@/components/admin/EditorConflict";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAdminUsers } from "@/hooks/queries/admin/users";
import {
  useAdminRequestCapabilities,
  useRequestUserLimit,
  useUpdateRequestUserLimit,
} from "@/hooks/queries/useRequests";
import { RequestQueue } from "@/pages/admin-requests/RequestQueue";
import { EmptyPanel, RowsSkeleton } from "@/pages/admin-requests/queueParts";

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
          <RequestQueue />
        </TabsContent>
        <TabsContent value="overrides">
          <UserOverridesTab />
        </TabsContent>
      </Tabs>
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

import { useMemo, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";

import type { RequestSettings } from "@/api/types";
import {
  isRequestEditorConflict,
  requestValidationErrors,
  type RequestRoute,
  type RequestRouteMediaType,
} from "@/api/v2/adminRequests";
import { EditorConflict } from "@/components/admin/EditorConflict";
import { SettingsPageHeader } from "@/components/settings/SettingsPageHeader";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useAdminPluginInstallations } from "@/hooks/queries/admin/plugins";
import {
  useAdminRequestCapabilities,
  useRequestIntegrations,
  useRequestRoutes,
  useRequestSettings,
  useUpdateRequestRoute,
  useUpdateRequestSettings,
} from "@/hooks/queries/useRequests";
import { useReportUnsavedChanges } from "@/hooks/useUnsavedChanges";

import { FieldGroup } from "./FieldGroup";
import { RequestRoutingGroup } from "./RequestRouting";
import { RequestServersGroup } from "./RequestServers";
import {
  fallbackBody,
  fallbackChanges,
  fallbackDraft,
  type FallbackDraft,
} from "./requestRoutingModel";
import { requestRouterInstallations } from "./requestServerModel";
import { SaveBar } from "./SaveBar";
import { SettingField, SettingFieldRow, SettingFieldStatus } from "./SettingField";
import { useStagedDraft, type StagedDraft } from "./useStagedDraft";

interface GeneralDraft {
  requests_enabled: boolean;
  auto_approve: boolean;
  max_requests: string;
  window_days: string;
  force_dual_quality: boolean;
}

function generalDraft(settings: RequestSettings): GeneralDraft {
  return {
    requests_enabled: settings.requests_enabled,
    auto_approve: settings.global_auto_approval_enabled,
    max_requests: String(settings.global_max_requests),
    window_days: String(settings.global_window_days),
    force_dual_quality: settings.force_dual_quality,
  };
}

function generalChanges(draft: GeneralDraft, base: GeneralDraft): number {
  return (Object.keys(draft) as (keyof GeneralDraft)[]).filter((key) => draft[key] !== base[key])
    .length;
}

function positiveInt(value: string): number | null {
  const n = Number(value.trim());
  return Number.isInteger(n) && n >= 1 ? n : null;
}

const MEDIA_TYPES: RequestRouteMediaType[] = ["movie", "series"];

function fallbackOf(routes: RequestRoute[] | undefined, mediaType: RequestRouteMediaType) {
  return routes?.find((route) => route.is_fallback && route.media_type === mediaType);
}

function PageSkeleton() {
  return (
    <div className="space-y-6" role="status" aria-label="Loading request settings">
      <Skeleton className="h-8 w-48" />
      <Skeleton className="h-40 w-full" />
      <Skeleton className="h-40 w-full" />
      <span className="sr-only">Loading request settings</span>
    </div>
  );
}

/**
 * Settings → Requests: whether and how people request titles, the servers
 * requests go to, and the rules that pick a server for each title.
 */
export default function RequestsSettings() {
  const capabilities = useAdminRequestCapabilities();
  if (capabilities.isLoading) return <PageSkeleton />;
  if (!capabilities.data?.available || !capabilities.data.guarded_configuration) {
    return (
      <div className="flex flex-col gap-7">
        <SettingsPageHeader title="Requests" />
        <p className="text-muted-foreground text-sm">
          Request settings are not available on this server.
        </p>
      </div>
    );
  }
  return <RequestsSettingsContent routing={capabilities.data.routing} />;
}

/** routing: whether the server offers routing rules; without them the page
 * leaves them out rather than calling the route endpoints. */
function RequestsSettingsContent({ routing }: { routing: boolean }) {
  const settingsQuery = useRequestSettings();
  const routesQuery = useRequestRoutes(routing);
  const serversQuery = useRequestIntegrations();
  const installationsQuery = useAdminPluginInstallations();
  const updateSettings = useUpdateRequestSettings();
  const updateRoute = useUpdateRequestRoute({ inlineErrors: true });
  const installations = useMemo(
    () => requestRouterInstallations(installationsQuery.data ?? []),
    [installationsQuery.data],
  );

  const general = useStagedDraft(settingsQuery.data, generalDraft, generalChanges);
  const [generalConflict, setGeneralConflict] = useState(false);
  const movie = useStagedDraft(
    fallbackOf(routesQuery.data, "movie"),
    fallbackDraft,
    fallbackChanges,
  );
  const series = useStagedDraft(
    fallbackOf(routesQuery.data, "series"),
    fallbackDraft,
    fallbackChanges,
  );
  const fallbacks: Record<RequestRouteMediaType, StagedDraft<RequestRoute, FallbackDraft>> = {
    movie,
    series,
  };
  const [fallbackConflicts, setFallbackConflicts] = useState<Record<string, boolean>>({});
  const [fallbackErrors, setFallbackErrors] = useState<Record<string, Record<string, string>>>({});
  const [saving, setSaving] = useState(false);

  const maxRequests = general.draft ? positiveInt(general.draft.max_requests) : null;
  const windowDays = general.draft ? positiveInt(general.draft.window_days) : null;
  const baseGeneral = general.base ? generalDraft(general.base) : undefined;
  const maxInvalid =
    maxRequests === null && general.draft?.max_requests !== baseGeneral?.max_requests;
  const windowInvalid =
    windowDays === null && general.draft?.window_days !== baseGeneral?.window_days;

  const dirtyCount = general.changes + movie.changes + series.changes;
  useReportUnsavedChanges(dirtyCount > 0);
  const saveable =
    (general.changes > 0 && !generalConflict && !maxInvalid && !windowInvalid) ||
    MEDIA_TYPES.some((type) => fallbacks[type].changes > 0 && !fallbackConflicts[type]);

  function editGeneral(change: Partial<GeneralDraft>) {
    general.update((current) => ({ ...current, ...change }));
  }

  async function saveGeneral() {
    const { base, draft } = general;
    if (!base || !draft || general.changes === 0 || generalConflict) return;
    if (maxInvalid || windowInvalid) return;
    try {
      const saved = await updateSettings.mutateAsync({
        requests_enabled: draft.requests_enabled,
        global_auto_approval_enabled: draft.auto_approve,
        // An untouched stored value is sent back as it is, even one the
        // controls would not offer.
        global_max_requests: maxRequests ?? base.global_max_requests,
        global_window_days: windowDays ?? base.global_window_days,
        force_dual_quality: draft.force_dual_quality,
        updated_at: base.updated_at,
        etag: base.etag,
      });
      general.adopt(saved);
    } catch (error) {
      if (isRequestEditorConflict(error)) setGeneralConflict(true);
    }
  }

  async function saveFallback(mediaType: RequestRouteMediaType) {
    const staged = fallbacks[mediaType];
    const { base, draft } = staged;
    if (!base || !draft || staged.changes === 0 || fallbackConflicts[mediaType]) return;
    setFallbackErrors((current) => ({ ...current, [mediaType]: {} }));
    try {
      const saved = await updateRoute.mutateAsync({ route: base, body: fallbackBody(base, draft) });
      staged.adopt(saved);
    } catch (error) {
      if (isRequestEditorConflict(error)) {
        setFallbackConflicts((current) => ({ ...current, [mediaType]: true }));
      }
      const validation = requestValidationErrors(error);
      if (validation) {
        // With no field to point at, the server's own sentence is the error.
        const fields =
          Object.keys(validation.fields).length > 0
            ? validation.fields
            : { "": validation.message };
        setFallbackErrors((current) => ({ ...current, [mediaType]: fields }));
      }
    }
  }

  // One save bar, three writers, one after another: the request settings and
  // each media type's default destination. A writer that fails keeps its
  // edits and says why beside them; the others still save.
  async function saveAll() {
    setSaving(true);
    try {
      await saveGeneral();
      for (const mediaType of MEDIA_TYPES) await saveFallback(mediaType);
    } finally {
      setSaving(false);
    }
  }

  // A discarded draft goes back to its record and then, being clean, follows
  // the query to whatever is newest, so a conflict it hit no longer applies.
  function discardAll() {
    general.reset();
    movie.reset();
    series.reset();
    setGeneralConflict(false);
    setFallbackConflicts({});
    setFallbackErrors({});
  }

  // An edit to one tier makes that tier's save errors, and the form-level
  // one, stale; the other tier's stay until it is edited too.
  function clearFallbackErrors(mediaType: RequestRouteMediaType, tier: "hd" | "uhd") {
    setFallbackErrors((current) => {
      const errors = current[mediaType];
      if (!errors) return current;
      const kept = Object.fromEntries(
        Object.entries(errors).filter(
          ([key]) => key !== "" && key !== tier && !key.startsWith(`${tier}.`),
        ),
      );
      return Object.keys(kept).length === Object.keys(errors).length
        ? current
        : { ...current, [mediaType]: kept };
    });
  }

  async function reloadGeneral() {
    const result = await settingsQuery.refetch();
    if (result.data && !result.isError) {
      general.adopt(result.data);
      setGeneralConflict(false);
    } else {
      toast.error(
        result.error instanceof Error
          ? `Couldn't reload request settings: ${result.error.message}`
          : "Couldn't reload request settings.",
      );
    }
  }

  // Reloads through the query, so the draft and the cache it follows agree
  // on the newest version.
  async function reloadFallback(mediaType: RequestRouteMediaType) {
    const result = await routesQuery.refetch();
    const latest = result.isError ? undefined : fallbackOf(result.data, mediaType);
    if (!latest) {
      toast.error(
        result.error instanceof Error
          ? `Couldn't reload the default destination: ${result.error.message}`
          : "Couldn't reload the default destination.",
      );
      return;
    }
    fallbacks[mediaType].adopt(latest);
    setFallbackConflicts((current) => ({ ...current, [mediaType]: false }));
    setFallbackErrors((current) => ({ ...current, [mediaType]: {} }));
  }

  const draft = general.draft;
  const servers = serversQuery.data ?? [];
  const routes = routesQuery.data ?? [];

  return (
    <div className="flex h-full flex-col">
      <SettingsPageHeader
        title="Requests"
        className="mb-8"
        actions={
          <Button asChild variant="outline" size="sm">
            <Link to="/admin/requests">Request queue</Link>
          </Button>
        }
      />

      <div className="flex-1 space-y-5">
        {/* Locked while the save bar is saving: a save adopts what the server
            returned, which would drop an edit made in the meantime. */}
        <fieldset disabled={saving} className="min-w-0">
          <FieldGroup label="General" dirty={general.changes > 0}>
            {settingsQuery.isLoading ? (
              <div className="space-y-2 py-3.5">
                <Skeleton className="h-9 w-full" />
                <Skeleton className="h-9 w-full" />
              </div>
            ) : !draft || !baseGeneral ? (
              <p className="text-destructive py-3.5 text-sm">
                Request settings could not be loaded.
              </p>
            ) : (
              <>
                <SettingField
                  label="Allow requests"
                  type="toggle"
                  description="People can ask for movies and series the server does not have."
                  value={String(draft.requests_enabled)}
                  onChange={(value) => editGeneral({ requests_enabled: value === "true" })}
                  dirty={draft.requests_enabled !== baseGeneral.requests_enabled}
                />
                <SettingField
                  label="Approval"
                  type="select"
                  options={[
                    { value: "auto", label: "Approve automatically" },
                    { value: "admin", label: "An admin approves" },
                  ]}
                  value={draft.auto_approve ? "auto" : "admin"}
                  onChange={(value) => editGeneral({ auto_approve: value === "auto" })}
                  dirty={draft.auto_approve !== baseGeneral.auto_approve}
                />
                <SettingField
                  label="Request limit"
                  type="number"
                  unit="requests"
                  description="How many titles one account can request in the window below."
                  value={draft.max_requests}
                  onChange={(value) => editGeneral({ max_requests: value })}
                  dirty={draft.max_requests !== baseGeneral.max_requests}
                  status={
                    maxInvalid ? (
                      <SettingFieldStatus tone="warn">
                        Allow at least 1. To stop requests, turn off Allow requests.
                      </SettingFieldStatus>
                    ) : undefined
                  }
                />
                <SettingField
                  label="Limit window"
                  type="number"
                  unit="days"
                  value={draft.window_days}
                  onChange={(value) => editGeneral({ window_days: value })}
                  dirty={draft.window_days !== baseGeneral.window_days}
                  status={
                    windowInvalid ? (
                      <SettingFieldStatus tone="warn">Use at least 1 day.</SettingFieldStatus>
                    ) : undefined
                  }
                />
                <SettingField
                  label="Also request a 4K copy of every title"
                  type="toggle"
                  description="Normally only requesters who can play 4K get a 4K copy, and only when a server takes 4K. With this on, every request also asks for 4K."
                  value={String(draft.force_dual_quality)}
                  onChange={(value) => editGeneral({ force_dual_quality: value === "true" })}
                  dirty={draft.force_dual_quality !== baseGeneral.force_dual_quality}
                />
                {generalConflict ? (
                  <div className="py-3.5">
                    <EditorConflict onReload={reloadGeneral} />
                  </div>
                ) : null}
              </>
            )}
          </FieldGroup>
        </fieldset>

        <RequestServersGroup
          servers={servers}
          serversLoading={serversQuery.isLoading}
          serversError={serversQuery.isError && !serversQuery.data}
          installations={installations}
          installationsLoading={installationsQuery.isLoading}
          routes={routes}
        />

        {(routing ? MEDIA_TYPES : []).map((mediaType) => (
          <RequestRoutingGroup
            key={mediaType}
            mediaType={mediaType}
            routes={routes}
            routesLoading={routesQuery.isLoading}
            routesFetching={routesQuery.isFetching}
            routesError={routesQuery.isError && !routesQuery.data}
            allServers={servers}
            installations={installations}
            requestsEnabled={general.base?.requests_enabled}
            fallback={fallbacks[mediaType]}
            fallbackSaving={saving}
            fallbackErrors={fallbackErrors[mediaType] ?? {}}
            fallbackConflict={Boolean(fallbackConflicts[mediaType])}
            onFallbackEdited={(tier) => clearFallbackErrors(mediaType, tier)}
            onReloadFallback={() => reloadFallback(mediaType)}
          />
        ))}

        <FieldGroup label="Related">
          <SettingFieldRow
            label="Per-account limits"
            description="Give one account its own request limit or approval."
          >
            <Button asChild variant="outline" size="sm">
              <Link to="/admin/requests?tab=overrides">User overrides</Link>
            </Button>
          </SettingFieldRow>
          <SettingFieldRow
            label="Request notifications"
            description="Announce submitted, approved, declined, and fulfilled requests on Discord or a webhook."
          >
            <Button asChild variant="outline" size="sm">
              <Link to="/admin/settings/notifications">Notifications</Link>
            </Button>
          </SettingFieldRow>
          <SettingFieldRow
            label="Autoscan"
            description="Autoscan can reuse these servers to import downloads as soon as they finish."
          >
            <Button asChild variant="outline" size="sm">
              <Link to="/admin/libraries?tab=autoscan">Autoscan</Link>
            </Button>
          </SettingFieldRow>
        </FieldGroup>
      </div>

      <SaveBar
        dirtyCount={dirtyCount}
        onSave={() => void saveAll()}
        onDiscard={discardAll}
        isSaving={saving}
        canSave={saveable}
      />
    </div>
  );
}

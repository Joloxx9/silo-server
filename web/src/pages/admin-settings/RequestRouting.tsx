import { useId, useMemo, useState } from "react";
import { ArrowDown, ArrowUp, Plus } from "lucide-react";

import type { RequestIntegration } from "@/api/types";
import type { RequestRoute, RequestRouteMediaType } from "@/api/v2/adminRequests";
import { EditorConflict } from "@/components/admin/EditorConflict";
import { SettingsSubheading } from "@/components/settings/SettingsSubheading";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { useAdminUsers } from "@/hooks/queries/admin/users";
import {
  useDiscoverNetworks,
  useDiscoverStudios,
  useReorderRequestRoutes,
  useUpdateRequestRoute,
} from "@/hooks/queries/useRequests";

import { FieldGroup } from "./FieldGroup";
import { DEST_NONE, RouteDestinationFields } from "./RequestRouteFields";
import { RequestRoutePreview } from "./RequestRoutePreview";
import { RequestRuleEditor } from "./RequestRuleEditor";
import {
  conditionSummary,
  destinationChanges,
  destinationDraft,
  destinationSummary,
  addRuleBlockedReason,
  routeBody,
  type FallbackDraft,
} from "./requestRoutingModel";
import {
  mediaTypePlural,
  serverServesMediaType,
  type RequestRouterInstallation,
} from "./requestServerModel";
import type { StagedDraft } from "./useStagedDraft";

const GROUP_LABELS: Record<RequestRouteMediaType, string> = {
  movie: "Movie routing",
  series: "Series routing",
};
const KIND_NAMES: Record<RequestRouteMediaType, string> = { movie: "Radarr", series: "Sonarr" };

function RuleRow({
  rule,
  index,
  count,
  servers,
  mediaType,
  names,
  busy,
  onEdit,
  onToggle,
  onMove,
}: {
  rule: RequestRoute;
  index: number;
  count: number;
  servers: RequestIntegration[];
  mediaType: RequestRouteMediaType;
  names: Parameters<typeof conditionSummary>[2];
  busy: boolean;
  onEdit: () => void;
  onToggle: (enabled: boolean) => void;
  onMove: (delta: -1 | 1) => void;
}) {
  const summaryId = useId();
  const matches = conditionSummary(rule.conditions, mediaType, names);
  const sends = [
    destinationSummary("HD", rule.hd, servers),
    destinationSummary("4K", rule.uhd, servers, rule.skip_uhd),
  ].join(" · ");
  return (
    <li className="flex items-start gap-3 py-3">
      <Switch
        className="mt-0.5 shrink-0"
        checked={rule.enabled}
        onCheckedChange={onToggle}
        disabled={busy}
        aria-label={`${rule.name} enabled`}
      />
      <button
        type="button"
        onClick={onEdit}
        aria-label={`Edit ${rule.name}`}
        aria-describedby={summaryId}
        className="focus-visible:ring-ring min-w-0 flex-1 rounded-sm text-left focus-visible:ring-2 focus-visible:outline-none"
      >
        <span className="block text-sm font-medium hover:underline">{rule.name}</span>
        <span id={summaryId} className="text-muted-foreground block text-xs leading-relaxed">
          <span className="block">{matches || "Matches every title"}</span>
          <span className="block">{sends}</span>
        </span>
      </button>
      <div className="flex shrink-0 gap-1">
        <Button
          type="button"
          size="icon-sm"
          variant="outline"
          aria-label={`Move ${rule.name} up`}
          disabled={busy || index === 0}
          onClick={() => onMove(-1)}
        >
          <ArrowUp />
        </Button>
        <Button
          type="button"
          size="icon-sm"
          variant="outline"
          aria-label={`Move ${rule.name} down`}
          disabled={busy || index === count - 1}
          onClick={() => onMove(1)}
        >
          <ArrowDown />
        </Button>
      </div>
    </li>
  );
}

/**
 * One media type's routing: the default destination (staged into the page's
 * save bar), the ordered rules (each saved from its own editor), and a way to
 * check where a title would go.
 */
export function RequestRoutingGroup({
  mediaType,
  routes,
  routesLoading,
  routesFetching,
  routesError,
  allServers,
  installations,
  requestsEnabled,
  fallback,
  fallbackSaving,
  fallbackErrors,
  fallbackConflict,
  onFallbackEdited,
  onReloadFallback,
}: {
  mediaType: RequestRouteMediaType;
  routes: RequestRoute[];
  routesLoading: boolean;
  /** The list is being read again; its order and validators may be stale. */
  routesFetching: boolean;
  routesError: boolean;
  allServers: RequestIntegration[];
  installations: RequestRouterInstallation[];
  /** The saved request switch; undefined until the settings have loaded. */
  requestsEnabled: boolean | undefined;
  fallback: StagedDraft<RequestRoute, FallbackDraft>;
  /** The page's save bar is saving; the default destination is locked meanwhile. */
  fallbackSaving: boolean;
  fallbackErrors: Record<string, string>;
  fallbackConflict: boolean;
  onFallbackEdited: (tier: "hd" | "uhd") => void;
  onReloadFallback: () => Promise<void>;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const [newKey, setNewKey] = useState(0);
  const users = useAdminUsers();
  // The curated networks and studios come from the requesters' discover API,
  // which answers only while requests are on (and refuses with a 409 when
  // they are off, which asking again does not change).
  const brandsReadable = requestsEnabled === true;
  const networks = useDiscoverNetworks({ enabled: brandsReadable, retry: false });
  const studios = useDiscoverStudios({ enabled: brandsReadable, retry: false });
  const brands = (mediaType === "series" ? networks.data : studios.data) ?? [];
  const toggleRule = useUpdateRequestRoute();
  const reorder = useReorderRequestRoutes();

  const servers = useMemo(
    () => allServers.filter((server) => serverServesMediaType(server, mediaType)),
    [allServers, mediaType],
  );
  const rules = routes
    .filter((route) => route.media_type === mediaType && !route.is_fallback)
    .sort((a, b) => a.position - b.position);
  const names = useMemo(
    () => ({
      users: new Map((users.data ?? []).map((user) => [user.id, user.username])),
      brands: new Map(
        [...(networks.data ?? []), ...(studios.data ?? [])]
          .filter((brand) => brand.tmdb_id)
          .map((brand) => [brand.tmdb_id!, brand.display_name]),
      ),
    }),
    [users.data, networks.data, studios.data],
  );

  const base = fallback.base;
  const draft = fallback.draft;
  const fallbackReady = Boolean(base?.hd.integration_id);
  // The destination rows show their own errors; anything else is listed once.
  const otherFallbackErrors = Object.entries(fallbackErrors).filter(
    ([key]) => !/^u?hd(\.|$)/.test(key),
  );
  const editingRule = editing && editing !== "new" ? rules.find((r) => r.id === editing) : null;
  const plural = mediaTypePlural(mediaType);
  // Nothing routes this media type yet: no rules, and no default server.
  const unrouted = rules.length === 0 && !base?.hd.integration_id && !base?.uhd.integration_id;

  function move(index: number, delta: -1 | 1) {
    const ids = rules.map((rule) => rule.id);
    const target = index + delta;
    if (target < 0 || target >= ids.length) return;
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    reorder.mutate({ mediaType, ids });
  }

  // Until the reread lands, the order and validators on screen are the ones a
  // reorder or save just replaced, so nothing may be computed from them.
  const listBusy = reorder.isPending || toggleRule.isPending || routesFetching;

  function choose(tier: "hd" | "uhd", value: string) {
    onFallbackEdited(tier);
    fallback.update((current) => {
      const id = value === DEST_NONE ? "" : value;
      if (current[tier].integration_id === id) return current;
      return { ...current, [tier]: { integration_id: id, overrides: {} } };
    });
  }

  function setOverrides(tier: "hd" | "uhd", overrides: Record<string, unknown>) {
    onFallbackEdited(tier);
    fallback.update((current) => ({ ...current, [tier]: { ...current[tier], overrides } }));
  }

  return (
    <FieldGroup
      label={GROUP_LABELS[mediaType]}
      description={`Which server each ${mediaType === "series" ? "series" : "movie"} request goes to.`}
      dirty={fallback.changes > 0}
    >
      {routesLoading ? (
        <div className="space-y-2 py-3.5">
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
        </div>
      ) : routesError || !base || !draft ? (
        <p className="text-destructive py-3.5 text-sm">Routing could not be loaded.</p>
      ) : (
        <>
          <SettingsSubheading caption="Titles no rule matches go here.">
            Default destination
          </SettingsSubheading>
          {servers.length === 0 ? (
            <p className="settings-field-note text-muted-foreground py-3 text-xs">
              Add a {KIND_NAMES[mediaType]} server above to send {plural} anywhere.
            </p>
          ) : unrouted ? (
            <p className="settings-field-note text-muted-foreground py-3 text-xs">
              Choose an HD server so approved requests for {plural} have somewhere to go.
            </p>
          ) : null}
          {fallbackConflict ? (
            <div className="py-3">
              <EditorConflict onReload={onReloadFallback} />
            </div>
          ) : null}
          {otherFallbackErrors.length > 0 ? (
            <div role="alert" className="text-destructive space-y-1 py-3 text-xs">
              {otherFallbackErrors.map(([key, detail]) => (
                <p key={key}>{detail}</p>
              ))}
            </div>
          ) : null}
          {/* Locked while the save bar saves it: the save adopts what the
              server returned, which would drop an edit made meanwhile. */}
          <fieldset
            disabled={fallbackSaving}
            aria-label="Default destination"
            className="settings-field-list min-w-0"
          >
            <RouteDestinationFields
              sectionId={`${base.id}.hd`}
              label="HD server"
              tierLabel="HD"
              servers={servers}
              allServers={allServers}
              installations={installations}
              value={draft.hd}
              choices={[{ value: DEST_NONE, label: "No default server" }]}
              selected={draft.hd.integration_id || DEST_NONE}
              onSelect={(value) => choose("hd", value)}
              onOverridesChange={(overrides) => setOverrides("hd", overrides)}
              errors={fallbackErrors}
              errorPrefix="hd"
              dirty={destinationChanges(draft.hd, destinationDraft(base.hd)) > 0}
            />
            <RouteDestinationFields
              sectionId={`${base.id}.uhd`}
              label="4K server"
              tierLabel="4K"
              servers={servers}
              allServers={allServers}
              installations={installations}
              value={draft.uhd}
              choices={[{ value: DEST_NONE, label: "No 4K copy" }]}
              selected={draft.uhd.integration_id || DEST_NONE}
              onSelect={(value) => choose("uhd", value)}
              onOverridesChange={(overrides) => setOverrides("uhd", overrides)}
              errors={fallbackErrors}
              errorPrefix="uhd"
              dirty={destinationChanges(draft.uhd, destinationDraft(base.uhd)) > 0}
            />
          </fieldset>

          <SettingsSubheading caption="Checked top to bottom. For each quality, the first enabled rule that matches a title and sends that quality decides.">
            Rules
          </SettingsSubheading>
          <div>
            {rules.length === 0 ? (
              <p className="text-muted-foreground py-3 text-xs">
                No rules yet, so every {mediaType === "series" ? "series" : "movie"} goes to the
                default destination.
              </p>
            ) : (
              <ol
                aria-label={`${GROUP_LABELS[mediaType]} rules`}
                className="divide-border/60 list-none divide-y"
              >
                {rules.map((rule, index) => (
                  <RuleRow
                    key={rule.id}
                    rule={rule}
                    index={index}
                    count={rules.length}
                    servers={allServers}
                    mediaType={mediaType}
                    names={names}
                    busy={listBusy}
                    onEdit={() => setEditing(rule.id)}
                    onToggle={(enabled) =>
                      toggleRule.mutate({ route: rule, body: { ...routeBody(rule), enabled } })
                    }
                    onMove={(delta) => move(index, delta)}
                  />
                ))}
              </ol>
            )}
            <div className="flex flex-wrap items-center gap-3 pt-1 pb-3.5">
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={!fallbackReady}
                onClick={() => {
                  setNewKey((key) => key + 1);
                  setEditing("new");
                }}
              >
                <Plus aria-hidden="true" />
                Add rule
              </Button>
              {!fallbackReady ? (
                <p className="text-muted-foreground text-xs">{addRuleBlockedReason(mediaType)}</p>
              ) : null}
            </div>
          </div>

          <RequestRoutePreview
            mediaType={mediaType}
            requestsEnabled={requestsEnabled !== false}
            servers={allServers}
            installations={installations}
          />
        </>
      )}

      <Dialog
        open={editing === "new" || Boolean(editingRule)}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          {editing === "new" || editingRule ? (
            <RequestRuleEditor
              // Keyed by id alone: a refresh that brings a newer revision
              // must not throw away edits; the save's 412 reports it instead.
              key={editingRule ? editingRule.id : `new-${newKey}`}
              mediaType={mediaType}
              source={editingRule ?? null}
              servers={servers}
              allServers={allServers}
              installations={installations}
              users={users.data ?? []}
              brands={brands}
              brandsHint={
                requestsEnabled === false
                  ? "Turn on requests to pick networks and studios."
                  : (mediaType === "series" ? networks : studios).isError
                    ? `Couldn't load the ${mediaType === "series" ? "network" : "studio"} list.`
                    : undefined
              }
              onDone={() => setEditing(null)}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </FieldGroup>
  );
}

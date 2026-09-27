import type {
  PluginAdminForm,
  PluginCapability,
  PluginInstallation,
  RequestIntegration,
} from "@/api/types";
import type { RequestRoute, RequestRouteMediaType } from "@/api/v2/adminRequests";

/** Every request server is fulfilled by a plugin exposing this capability. */
export const REQUEST_ROUTER_CAPABILITY = "request_router.v1";

/**
 * Plugin config keys that request routing sets itself (the server's
 * `routingOwnedConfigKeys`). The Sonarr/Radarr plugin form still declares
 * them, but once a media type has routes they decide nothing, so the server
 * editor hides them and a route cannot override them.
 */
export const ROUTING_OWNED_CONFIG_KEYS: readonly string[] = [
  "is_default",
  "is_default_4k",
  "is_4k",
  "anime_enabled",
  "anime_root_folder",
  "anime_quality_profile_id",
  "anime_tags",
];

/** The config key naming which *arr a Sonarr/Radarr server is. */
export const SERVICE_KIND_KEY = "service_kind";

export interface RequestRouterInstallation {
  installationID: number;
  pluginID: string;
  capability: PluginCapability;
}

/**
 * One entry per request_router.v1 capability across the installed plugins, so
 * one installation exposing two capabilities offers two server types.
 */
export function requestRouterInstallations(
  installations: PluginInstallation[],
): RequestRouterInstallation[] {
  const out: RequestRouterInstallation[] = [];
  for (const installation of installations) {
    for (const capability of installation.capabilities ?? []) {
      if (
        capability.type === REQUEST_ROUTER_CAPABILITY ||
        capability.id === REQUEST_ROUTER_CAPABILITY
      ) {
        out.push({ installationID: installation.id, pluginID: installation.plugin_id, capability });
      }
    }
  }
  return out;
}

export function installationOptionLabel(entry: RequestRouterInstallation): string {
  return entry.capability.display_name || entry.pluginID;
}

/**
 * The select value for a server type: installation and capability together,
 * since the installation alone collides when it exposes two capabilities.
 */
export function installationOptionValue(entry: RequestRouterInstallation): string {
  return `${entry.installationID}:${entry.capability.id}`;
}

/**
 * The installed capability a server is bound to. Matches the capability too
 * when one is recorded, and otherwise adopts the installation's capability.
 */
export function serverInstallation(
  installations: RequestRouterInstallation[],
  installationId: number | undefined,
  capabilityId: string | undefined,
): RequestRouterInstallation | undefined {
  if (!installationId) return undefined;
  return (
    installations.find(
      (entry) => entry.installationID === installationId && entry.capability.id === capabilityId,
    ) ?? installations.find((entry) => entry.installationID === installationId)
  );
}

export function serverConfigSchema(entry: RequestRouterInstallation | undefined): {
  descriptor?: PluginAdminForm;
  jsonSchema?: string;
} {
  const schema = entry?.capability.config_schema?.[0];
  return { descriptor: schema?.admin_form, jsonSchema: schema?.json_schema };
}

/** `radarr`, `sonarr`, or "" for a server whose plugin does not say. */
export function serverKind(server: Pick<RequestIntegration, "plugin_config">): string {
  const kind = server.plugin_config?.[SERVICE_KIND_KEY];
  return typeof kind === "string" ? kind.trim().toLowerCase() : "";
}

const KIND_LABELS: Record<string, string> = { radarr: "Radarr", sonarr: "Sonarr" };
const KIND_MEDIA_TYPE: Record<string, RequestRouteMediaType> = {
  radarr: "movie",
  sonarr: "series",
};

/** "Radarr", "Sonarr", or the plugin's own name for other request servers. */
export function serverTypeLabel(
  server: RequestIntegration,
  installations: RequestRouterInstallation[],
): string {
  const kind = serverKind(server);
  if (KIND_LABELS[kind]) return KIND_LABELS[kind];
  const entry = serverInstallation(installations, server.installation_id, server.capability_id);
  return entry ? installationOptionLabel(entry) : "Request server";
}

/**
 * Whether routing may send a media type to this server: Radarr takes movies
 * and Sonarr series, as the server enforces; a server of another plugin says
 * what it takes through its supported media types.
 */
export function serverServesMediaType(
  server: RequestIntegration,
  mediaType: RequestRouteMediaType,
): boolean {
  const kind = serverKind(server);
  if (KIND_MEDIA_TYPE[kind]) return KIND_MEDIA_TYPE[kind] === mediaType;
  const types = server.supported_media_types ?? [];
  return types.length === 0 || types.includes(mediaType);
}

/**
 * Whether the server could take a request right now: switched on, bound to a
 * plugin, and holding an API key. Anything less reads "Needs setup".
 */
export function serverReady(server: RequestIntegration): boolean {
  return Boolean(server.enabled && server.installation_id && server.has_api_key);
}

export function mediaTypePlural(mediaType: RequestRouteMediaType): string {
  return mediaType === "series" ? "series" : "movies";
}

/**
 * Where a server is used, for its tile: "HD default for movies · Anime (4K)".
 * Empty when no route points at it.
 */
export function serverRouteUsage(serverId: string, routes: RequestRoute[]): string {
  const uses: string[] = [];
  for (const route of routes) {
    const hd = route.hd.integration_id === serverId;
    const uhd = route.uhd.integration_id === serverId;
    if (!hd && !uhd) continue;
    if (route.is_fallback) {
      const which = hd && uhd ? "Default" : hd ? "HD default" : "4K default";
      uses.push(`${which} for ${mediaTypePlural(route.media_type)}`);
    } else {
      uses.push(hd && uhd ? route.name : `${route.name} (${hd ? "HD" : "4K"})`);
    }
  }
  return uses.join(" · ");
}

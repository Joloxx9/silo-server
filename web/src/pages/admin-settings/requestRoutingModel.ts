import type { PluginAdminForm, PluginAdminFormField, RequestIntegration } from "@/api/types";
import type {
  RequestRoute,
  RequestRouteBody,
  RequestRouteConditions,
  RequestRouteDestination,
  RequestRouteMediaType,
} from "@/api/v2/adminRequests";
import { evaluateShowWhen } from "@/components/admin/plugins/schemaFormUtils";
import { routingCountryName, routingLanguageName } from "@/lib/requestRoutingOptions";
import { tmdbGenreName } from "@/lib/tmdbGenres";

import { mediaTypePlural, ROUTING_OWNED_CONFIG_KEYS, SERVICE_KIND_KEY } from "./requestServerModel";

/** One tier's destination while it is being edited. */
export interface RouteDestinationDraft {
  integration_id: string;
  overrides: Record<string, unknown>;
}

export function destinationDraft(dest: RequestRouteDestination | undefined): RouteDestinationDraft {
  return { integration_id: dest?.integration_id ?? "", overrides: { ...(dest?.overrides ?? {}) } };
}

/** The wire form of a destination: nothing at all when no server is chosen. */
export function destinationBody(draft: RouteDestinationDraft): RequestRouteDestination {
  if (!draft.integration_id) return {};
  const overrides = Object.fromEntries(
    Object.entries(draft.overrides).filter(([, value]) => value !== undefined),
  );
  return Object.keys(overrides).length > 0
    ? { integration_id: draft.integration_id, overrides }
    : { integration_id: draft.integration_id };
}

function stableJSON(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableJSON).join(",")}]`;
  if (value && typeof value === "object") {
    const entries = Object.entries(value as Record<string, unknown>)
      .filter(([, v]) => v !== undefined)
      .sort(([a], [b]) => a.localeCompare(b));
    return `{${entries.map(([k, v]) => `${JSON.stringify(k)}:${stableJSON(v)}`).join(",")}}`;
  }
  return JSON.stringify(value ?? null);
}

/** How many of a destination's two parts (server, overrides) differ. */
export function destinationChanges(a: RouteDestinationDraft, b: RouteDestinationDraft): number {
  return (
    (a.integration_id === b.integration_id ? 0 : 1) +
    (stableJSON(a.overrides) === stableJSON(b.overrides) ? 0 : 1)
  );
}

/** A media type's default destination while it is being edited. */
export interface FallbackDraft {
  hd: RouteDestinationDraft;
  uhd: RouteDestinationDraft;
}

export function fallbackDraft(route: RequestRoute): FallbackDraft {
  return { hd: destinationDraft(route.hd), uhd: destinationDraft(route.uhd) };
}

export function fallbackChanges(a: FallbackDraft, b: FallbackDraft): number {
  return destinationChanges(a.hd, b.hd) + destinationChanges(a.uhd, b.uhd);
}

export function fallbackBody(route: RequestRoute, draft: FallbackDraft): RequestRouteBody {
  return {
    name: route.name,
    enabled: true,
    conditions: {},
    hd: destinationBody(draft.hd),
    uhd: destinationBody(draft.uhd),
    skip_uhd: false,
  };
}

/** A routing rule while it is being edited. */
export interface RouteRuleDraft {
  name: string;
  enabled: boolean;
  conditions: RequestRouteConditions;
  hd: RouteDestinationDraft;
  uhd: RouteDestinationDraft;
  skipUhd: boolean;
}

export function ruleDraft(route: RequestRoute | null): RouteRuleDraft {
  return {
    name: route?.name ?? "",
    enabled: route?.enabled ?? true,
    conditions: { ...(route?.conditions ?? {}) },
    hd: destinationDraft(route?.hd),
    uhd: destinationDraft(route?.uhd),
    skipUhd: route?.skip_uhd ?? false,
  };
}

/** Drops unset conditions so the body carries only the ones that narrow. */
export function cleanConditions(conditions: RequestRouteConditions): RequestRouteConditions {
  const out: RequestRouteConditions = {};
  if (conditions.anime !== undefined) out.anime = conditions.anime;
  const lists = [
    "genre_ids",
    "keyword_ids",
    "original_languages",
    "origin_countries",
    "network_ids",
    "company_ids",
    "requester_user_ids",
  ] as const;
  for (const key of lists) {
    const values = conditions[key];
    if (values && values.length > 0) (out as Record<string, unknown>)[key] = [...values];
  }
  if (conditions.year_from) out.year_from = conditions.year_from;
  if (conditions.year_to) out.year_to = conditions.year_to;
  return out;
}

export function ruleBody(
  draft: RouteRuleDraft,
  mediaType?: RequestRouteMediaType,
): RequestRouteBody {
  return {
    ...(mediaType ? { media_type: mediaType } : {}),
    name: draft.name.trim(),
    enabled: draft.enabled,
    conditions: cleanConditions(draft.conditions),
    hd: destinationBody(draft.hd),
    uhd: draft.skipUhd ? {} : destinationBody(draft.uhd),
    skip_uhd: draft.skipUhd,
  };
}

/** A saved rule's body, e.g. to flip its enable switch without opening it. */
export function routeBody(route: RequestRoute): RequestRouteBody {
  return ruleBody(ruleDraft(route));
}

export interface RoutingNames {
  users?: ReadonlyMap<number, string>;
  brands?: ReadonlyMap<number, string>;
}

function listSummary(values: readonly string[]): string {
  return values.join(", ");
}

export function yearRangeLabel(from?: number, to?: number): string {
  if (from && to) return from === to ? String(from) : `${from}–${to}`;
  if (from) return `${from} or later`;
  if (to) return `${to} or earlier`;
  return "";
}

/** One line naming what a rule matches: "Anime · Japanese · 1980–1989 · Animation". */
export function conditionSummary(
  conditions: RequestRouteConditions,
  mediaType: RequestRouteMediaType,
  names: RoutingNames = {},
): string {
  const parts: string[] = [];
  if (conditions.anime === true) parts.push("Anime");
  if (conditions.anime === false) parts.push("Not anime");
  if (conditions.original_languages?.length) {
    parts.push(listSummary(conditions.original_languages.map(routingLanguageName)));
  }
  const years = yearRangeLabel(conditions.year_from, conditions.year_to);
  if (years) parts.push(years);
  if (conditions.genre_ids?.length) {
    parts.push(listSummary(conditions.genre_ids.map((id) => tmdbGenreName(id, mediaType))));
  }
  if (conditions.origin_countries?.length) {
    parts.push(listSummary(conditions.origin_countries.map(routingCountryName)));
  }
  const brand = (label: string) => (id: number) => names.brands?.get(id) ?? `${label} ${id}`;
  if (conditions.network_ids?.length) {
    parts.push(listSummary(conditions.network_ids.map(brand("Network"))));
  }
  if (conditions.company_ids?.length) {
    parts.push(listSummary(conditions.company_ids.map(brand("Studio"))));
  }
  if (conditions.keyword_ids?.length) {
    parts.push(`Keywords ${conditions.keyword_ids.join(", ")}`);
  }
  if (conditions.requester_user_ids?.length) {
    const users = conditions.requester_user_ids.map(
      (id) => names.users?.get(id) ?? `Account ${id}`,
    );
    parts.push(`Requested by ${listSummary(users)}`);
  }
  return parts.join(" · ");
}

/**
 * Where one tier of a rule goes: "HD → Radarr Anime · /anime", "4K → skipped",
 * or "4K → default" when a later rule or the default destination decides.
 */
export function destinationSummary(
  tierLabel: string,
  dest: RequestRouteDestination,
  servers: readonly RequestIntegration[],
  skipped = false,
): string {
  if (skipped) return `${tierLabel} → skipped`;
  if (!dest.integration_id) return `${tierLabel} → default`;
  const server = servers.find((candidate) => candidate.id === dest.integration_id);
  const name = server?.name ?? "Missing server";
  const folder = dest.overrides?.root_folder;
  return typeof folder === "string" && folder
    ? `${tierLabel} → ${name} · ${folder}`
    : `${tierLabel} → ${name}`;
}

const OVERRIDE_LABELS: Record<string, string> = {
  root_folder: "Root folder",
  quality_profile_id: "Quality profile",
  tags: "Tags",
  series_type: "Series type",
  minimum_availability: "Minimum availability",
  search_on_add: "Search on add",
  season_folder: "Season folder",
};

/**
 * Overrides whose value already reads as a name. A root folder's option label
 * adds the free space to the path, which is noise in a summary.
 */
const SELF_NAMED_OVERRIDES = new Set(["root_folder"]);

/**
 * Override keys whose values are IDs on the server (a quality profile, tags),
 * so naming them takes the server's options.
 */
export const SERVER_NAMED_OVERRIDES: ReadonlySet<string> = new Set(["quality_profile_id", "tags"]);

export interface OverrideNames {
  /** The server's loaded options (root folders, quality profiles, tags), by field key. */
  options?: Readonly<Record<string, readonly { value: string; label: string }[]>>;
  /** The server's plugin form, for field labels and fixed choices. */
  fields?: readonly PluginAdminFormField[];
}

function overrideValueLabel(key: string, value: unknown, names: OverrideNames): string {
  if (Array.isArray(value)) return value.map((v) => overrideValueLabel(key, v, names)).join(", ");
  if (typeof value === "boolean") return value ? "on" : "off";
  const raw = String(value);
  if (SELF_NAMED_OVERRIDES.has(key)) return raw;
  const choices = names.options?.[key] ?? names.fields?.find((f) => f.key === key)?.options;
  return choices?.find((choice) => choice.value === raw)?.label ?? raw;
}

/**
 * "Root folder /anime · Quality profile HD-1080p · Tags anime". Values are
 * named from the server's options when they are loaded, and shown as the
 * stored IDs when they are not.
 */
export function overridesSummary(
  overrides: Record<string, unknown> | undefined,
  names: OverrideNames = {},
): string {
  return Object.entries(overrides ?? {})
    .filter(([, value]) => value !== undefined && value !== null)
    .map(([key, value]) => {
      const label =
        names.fields?.find((field) => field.key === key)?.label ?? OVERRIDE_LABELS[key] ?? key;
      return `${label} ${overrideValueLabel(key, value, names)}`;
    })
    .join(" · ");
}

const OVERRIDABLE_CONTROLS = new Set(["SELECT", "MULTI_SELECT", "SWITCH"]);

/**
 * The plugin fields a route can override on this server: the server's own
 * form minus the keys routing sets and the service kind, and minus fields the
 * form would not show for this server (Radarr's minimum availability on a
 * Sonarr server).
 */
export function overrideFields(
  descriptor: PluginAdminForm | undefined,
  serverConfig: Record<string, unknown>,
): PluginAdminFormField[] {
  if (!descriptor) return [];
  return descriptor.fields.filter(
    (field) =>
      OVERRIDABLE_CONTROLS.has(field.control) &&
      field.key !== SERVICE_KIND_KEY &&
      !ROUTING_OWNED_CONFIG_KEYS.includes(field.key) &&
      evaluateShowWhen(field.show_when, serverConfig, descriptor.fields),
  );
}

/**
 * Why a rule cannot be added yet, in the server's words: the first rule
 * switches the media type to Silo's routing, and titles no rule matches need
 * the default destination's HD server to go to.
 */
export function addRuleBlockedReason(mediaType: RequestRouteMediaType): string {
  return `Choose the default server for ${mediaTypePlural(mediaType)} before adding rules; titles no rule matches go there.`;
}

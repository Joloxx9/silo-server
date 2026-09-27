import { useId, useState } from "react";

import type { RequestIntegration, RequestMediaResult } from "@/api/types";
import type {
  RequestRoutePreview as RoutePreview,
  RequestRoutePreviewTier,
  RequestRouteMediaType,
} from "@/api/v2/adminRequests";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useDebounce } from "@/hooks/useDebounce";
import {
  usePreviewRequestRoute,
  useRequestIntegrationOptions,
  useRequestSearch,
} from "@/hooks/queries/useRequests";
import { routingCountryName, routingLanguageName } from "@/lib/requestRoutingOptions";
import { tmdbGenreName } from "@/lib/tmdbGenres";

import { overridesSummary, SERVER_NAMED_OVERRIDES } from "./requestRoutingModel";
import {
  mediaTypePlural,
  serverConfigSchema,
  serverInstallation,
  type RequestRouterInstallation,
} from "./requestServerModel";

const MAX_RESULTS = 6;
const TIER_LABELS: Record<string, string> = { "1080p": "HD", "2160p": "4K" };

function factsSummary(facts: RoutePreview["facts"], mediaType: RequestRouteMediaType): string {
  return [
    facts.anime ? "Anime" : null,
    facts.genre_ids.length > 0
      ? facts.genre_ids.map((id) => tmdbGenreName(id, mediaType)).join(", ")
      : null,
    facts.original_language ? routingLanguageName(facts.original_language) : null,
    facts.origin_countries.length > 0
      ? facts.origin_countries.map(routingCountryName).join(", ")
      : null,
    facts.year ? String(facts.year) : null,
  ]
    .filter(Boolean)
    .join(" · ");
}

function TierOutcome({
  tier,
  servers,
  installations,
}: {
  tier: RequestRoutePreviewTier;
  servers: readonly RequestIntegration[];
  installations: RequestRouterInstallation[];
}) {
  const server = servers.find((candidate) => candidate.id === tier.integration_id);
  const entry = server
    ? serverInstallation(installations, server.installation_id, server.capability_id)
    : undefined;
  const fields = serverConfigSchema(entry).descriptor?.fields;
  // Quality profiles and tags are IDs on the server; reading its options names
  // them. Without them (or while they load) the summary shows the IDs.
  const needsNames = Object.keys(tier.overrides ?? {}).some((key) =>
    SERVER_NAMED_OVERRIDES.has(key),
  );
  const options = useRequestIntegrationOptions(needsNames && server ? server.id : undefined);
  const overrides = overridesSummary(tier.overrides, { options: options.data, fields });
  return (
    <div className="space-y-0.5">
      {tier.integration_id ? (
        <p>
          <span className="font-medium">{tier.integration_name || "Missing server"}</span>
          {tier.route_name ? (
            <span className="text-muted-foreground"> · by {tier.route_name}</span>
          ) : null}
        </p>
      ) : null}
      {overrides ? <p className="text-muted-foreground text-xs">{overrides}</p> : null}
      {tier.note ? (
        <p className={tier.integration_id ? "text-xs text-amber-600 dark:text-amber-400" : ""}>
          {tier.note}
        </p>
      ) : null}
    </div>
  );
}

/** TMDB's facts for a title and where each quality tier of a request for it goes. */
export function RoutePreviewResult({
  preview,
  mediaType,
  servers,
  installations,
}: {
  preview: RoutePreview;
  mediaType: RequestRouteMediaType;
  servers: readonly RequestIntegration[];
  installations: RequestRouterInstallation[];
}) {
  return (
    <>
      <p className="text-muted-foreground text-xs">
        {factsSummary(preview.facts, mediaType) || "TMDB has no facts for this title."}
      </p>
      <dl className="grid grid-cols-[3rem_1fr] gap-x-3 gap-y-2">
        {preview.tiers.map((tier) => (
          <div key={tier.quality} className="contents">
            <dt className="text-muted-foreground">{TIER_LABELS[tier.quality] ?? tier.quality}</dt>
            <dd>
              <TierOutcome tier={tier} servers={servers} installations={installations} />
            </dd>
          </div>
        ))}
      </dl>
    </>
  );
}

/**
 * Pick a title and see where each quality tier of a request for it would go
 * right now, from TMDB's facts and the saved rules.
 */
export function RequestRoutePreview({
  mediaType,
  requestsEnabled,
  servers,
  installations,
}: {
  mediaType: RequestRouteMediaType;
  /** Title search is the requesters' search, which answers only while requests are on. */
  requestsEnabled: boolean;
  servers: readonly RequestIntegration[];
  installations: RequestRouterInstallation[];
}) {
  const inputId = useId();
  const idInputId = useId();
  const [query, setQuery] = useState("");
  const [tmdbId, setTmdbId] = useState("");
  const [picked, setPicked] = useState<{ tmdbId: number; label: string } | null>(null);
  const debounced = useDebounce(query, 300);
  const search = useRequestSearch(mediaType, debounced, 1, {
    enabled: requestsEnabled && picked === null,
    retry: false,
  });
  const preview = usePreviewRequestRoute();
  const results = (search.data?.results ?? [])
    .filter((result) => result.media_type === mediaType)
    .slice(0, MAX_RESULTS);
  const searching = requestsEnabled && picked === null && debounced.trim().length > 1;
  // The preview itself works with requests off; only finding the title needs
  // the search. Without it, the admin names the title by its TMDB ID.
  const byId = !requestsEnabled || search.isError;
  const parsedId = Number(tmdbId.trim());
  const validId = Number.isInteger(parsedId) && parsedId > 0;

  function check(id: number, label: string) {
    setPicked({ tmdbId: id, label });
    preview.mutate({ mediaType, tmdbId: id });
  }

  function pick(result: RequestMediaResult) {
    setQuery(result.title);
    check(result.tmdb_id, result.year ? `${result.title} (${result.year})` : result.title);
  }

  return (
    <div className="space-y-3 py-3.5">
      <div className="space-y-1.5">
        <Label htmlFor={requestsEnabled ? inputId : idInputId} className="text-sm font-medium">
          Test a title
        </Label>
        <p className="text-muted-foreground text-xs">
          See where a request would go with the rules as saved.
        </p>
        {requestsEnabled ? (
          <Input
            id={inputId}
            type="search"
            value={query}
            placeholder={`Search ${mediaTypePlural(mediaType)}`}
            onChange={(event) => {
              setQuery(event.target.value);
              setPicked(null);
              preview.reset();
            }}
            className="sm:max-w-sm"
          />
        ) : null}
      </div>

      {byId ? (
        <form
          className="space-y-1.5"
          onSubmit={(event) => {
            event.preventDefault();
            if (validId) check(parsedId, `TMDB ${parsedId}`);
          }}
        >
          <p className="text-muted-foreground text-xs">
            {requestsEnabled
              ? "Search isn't answering; enter the title's TMDB ID instead."
              : "Search works only while requests are allowed; enter the title's TMDB ID instead."}
          </p>
          <div className="flex items-center gap-2">
            <Input
              id={idInputId}
              type="number"
              inputMode="numeric"
              min={1}
              aria-label={`${mediaType === "series" ? "Series" : "Movie"} TMDB ID`}
              placeholder="TMDB ID"
              value={tmdbId}
              onChange={(event) => setTmdbId(event.target.value)}
              className="w-40"
            />
            <Button type="submit" size="sm" variant="outline" disabled={!validId}>
              Check
            </Button>
          </div>
        </form>
      ) : null}

      {searching ? (
        search.isLoading ? (
          <p className="text-muted-foreground text-xs">Searching…</p>
        ) : search.isError ? (
          <p className="text-destructive text-xs">
            {search.error instanceof Error ? search.error.message : "Search failed."}
          </p>
        ) : results.length === 0 ? (
          <p className="text-muted-foreground text-xs">No matches.</p>
        ) : (
          <ul aria-label="Matching titles" className="flex list-none flex-col items-start gap-1">
            {results.map((result) => (
              <li key={result.tmdb_id}>
                <Button type="button" variant="ghost" size="sm" onClick={() => pick(result)}>
                  {result.title}
                  {result.year ? (
                    <span className="text-muted-foreground">({result.year})</span>
                  ) : null}
                </Button>
              </li>
            ))}
          </ul>
        )
      ) : null}

      {picked ? (
        <div
          aria-live="polite"
          className="border-border/70 bg-foreground/[0.02] space-y-2 rounded-xl border p-3 text-sm"
        >
          <p className="font-medium">{picked.label}</p>
          {preview.isPending ? (
            <p className="text-muted-foreground text-xs">Checking…</p>
          ) : preview.isError ? (
            <p className="text-destructive text-xs">
              {preview.error instanceof Error ? preview.error.message : "The preview failed."}
            </p>
          ) : preview.data ? (
            <RoutePreviewResult
              preview={preview.data}
              mediaType={mediaType}
              servers={servers}
              installations={installations}
            />
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

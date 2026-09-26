import { describe, expect, it } from "vitest";

import type { PluginAdminForm, RequestIntegration } from "@/api/types";
import type { RequestRoute } from "@/api/v2/adminRequests";

import {
  cleanConditions,
  conditionSummary,
  destinationSummary,
  overrideFields,
  overridesSummary,
  ruleBody,
  ruleDraft,
} from "./requestRoutingModel";
import { serverRouteUsage, serverServesMediaType } from "./requestServerModel";

const servers = [
  { id: "r2", name: "Radarr Anime", enabled: true, base_url: "" },
] as RequestIntegration[];

function route(extra: Partial<RequestRoute>): RequestRoute {
  return {
    id: "r",
    media_type: "movie",
    position: 0,
    name: "Rule",
    enabled: true,
    is_fallback: false,
    conditions: {},
    hd: {},
    uhd: {},
    skip_uhd: false,
    etag: '"r"',
    ...extra,
  };
}

describe("request routing summaries", () => {
  it("names what a rule matches in reading order", () => {
    expect(
      conditionSummary(
        {
          anime: true,
          original_languages: ["ja"],
          year_from: 1980,
          year_to: 1989,
          genre_ids: [16],
          requester_user_ids: [2],
        },
        "movie",
        { users: new Map([[2, "kid"]]) },
      ),
    ).toBe("Anime · Japanese · 1980–1989 · Animation · Requested by kid");
    expect(conditionSummary({ anime: false, year_from: 2000 }, "series")).toBe(
      "Not anime · 2000 or later",
    );
  });

  it("says where each tier goes", () => {
    expect(
      destinationSummary(
        "HD",
        { integration_id: "r2", overrides: { root_folder: "/anime" } },
        servers,
      ),
    ).toBe("HD → Radarr Anime · /anime");
    expect(destinationSummary("4K", {}, servers)).toBe("4K → default");
    expect(destinationSummary("4K", {}, servers, true)).toBe("4K → skipped");
  });

  it("lists where a server is used", () => {
    expect(
      serverRouteUsage("r2", [
        route({ id: "fallback-movie", is_fallback: true, hd: { integration_id: "r2" } }),
        route({ name: "Anime", uhd: { integration_id: "r2" } }),
      ]),
    ).toBe("HD default for movies · Anime (4K)");
  });
});

describe("override summaries", () => {
  const overrides = {
    root_folder: "/anime",
    quality_profile_id: 4,
    tags: [2, 9],
    series_type: "anime",
  };

  it("shows stored IDs until the server's options are loaded", () => {
    expect(overridesSummary(overrides)).toBe(
      "Root folder /anime · Quality profile 4 · Tags 2, 9 · Series type anime",
    );
  });

  it("names IDs and fixed choices from the server's options and form", () => {
    expect(
      overridesSummary(overrides, {
        options: {
          root_folder: [{ value: "/anime", label: "/anime (300 GiB free)" }],
          quality_profile_id: [{ value: "4", label: "Ultra-HD" }],
          tags: [{ value: "2", label: "anime" }],
        },
        fields: [
          {
            key: "series_type",
            label: "Series type",
            control: "SELECT",
            required: false,
            secret: false,
            multiline: false,
            options: [{ value: "anime", label: "Anime" }],
          },
        ],
      }),
    ).toBe("Root folder /anime · Quality profile Ultra-HD · Tags anime, 9 · Series type Anime");
  });
});

describe("request routing bodies", () => {
  it("drops empty conditions and the 4K destination of a rule that skips 4K", () => {
    const draft = {
      ...ruleDraft(null),
      name: "  Kids  ",
      conditions: { genre_ids: [], keyword_ids: [10], year_to: 0 },
      hd: { integration_id: "r2", overrides: { root_folder: undefined } },
      uhd: { integration_id: "r3", overrides: {} },
      skipUhd: true,
    };
    expect(ruleBody(draft, "series")).toEqual({
      media_type: "series",
      name: "Kids",
      enabled: true,
      conditions: { keyword_ids: [10] },
      hd: { integration_id: "r2" },
      uhd: {},
      skip_uhd: true,
    });
    expect(cleanConditions({ anime: false })).toEqual({ anime: false });
  });
});

describe("request servers", () => {
  it("routes movies to Radarr and series to Sonarr", () => {
    const radarr = { plugin_config: { service_kind: "radarr" } } as unknown as RequestIntegration;
    expect(serverServesMediaType(radarr, "movie")).toBe(true);
    expect(serverServesMediaType(radarr, "series")).toBe(false);
    const other = { supported_media_types: ["series"] } as unknown as RequestIntegration;
    expect(serverServesMediaType(other, "series")).toBe(true);
    expect(serverServesMediaType(other, "movie")).toBe(false);
  });

  it("offers overrides for the server's own settings, never the ones routing sets", () => {
    const form = {
      fields: [
        { key: "service_kind", label: "Service", control: "SELECT" },
        { key: "root_folder", label: "Root folder", control: "SELECT", dynamic_options: true },
        { key: "is_default", label: "Default", control: "SWITCH" },
        { key: "anime_tags", label: "Anime tags", control: "MULTI_SELECT" },
        {
          key: "minimum_availability",
          label: "Minimum availability",
          control: "SELECT",
          show_when: [{ field: "service_kind", equals: ["radarr"] }],
        },
        {
          key: "series_type",
          label: "Series type",
          control: "SELECT",
          show_when: [{ field: "service_kind", equals: ["sonarr"] }],
        },
      ],
    } as unknown as PluginAdminForm;
    expect(overrideFields(form, { service_kind: "sonarr" }).map((f) => f.key)).toEqual([
      "root_folder",
      "series_type",
    ]);
  });
});

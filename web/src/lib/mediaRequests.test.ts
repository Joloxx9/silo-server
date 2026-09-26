import { describe, expect, it } from "vitest";
import type { MediaRequest, RequestMediaSeason } from "@/api/types";
import {
  canCancelOwnRequest,
  defaultRequestSeasons,
  formatRequestDisplayState,
  formatRequestSeasonMeta,
  formatSeasonList,
  formatSeasonProgress,
  parseRequestMediaType,
  requestDetailHref,
  requestDisplayState,
} from "./mediaRequests";

type CancelFields = Pick<MediaRequest, "status" | "outcome" | "targets">;

describe("canCancelOwnRequest", () => {
  const pending: CancelFields = { status: "pending", outcome: "active" };

  it("allows an active pending request", () => {
    expect(canCancelOwnRequest(pending)).toBe(true);
  });

  it("allows an approved request nothing has been sent for", () => {
    expect(canCancelOwnRequest({ ...pending, status: "approved", targets: [] })).toBe(true);
  });

  it.each<[string, CancelFields]>([
    [
      "approved and sent",
      {
        ...pending,
        status: "approved",
        targets: [{ quality: "1080p", status: "queued" }] as MediaRequest["targets"],
      },
    ],
    ["queued", { ...pending, status: "queued" }],
    ["downloading", { ...pending, status: "downloading" }],
    ["completed", { ...pending, status: "completed" }],
    ["already cancelled", { ...pending, outcome: "cancelled" }],
    ["declined", { ...pending, outcome: "declined" }],
  ])("refuses a request that is %s, as the server does", (_label, request) => {
    expect(canCancelOwnRequest(request)).toBe(false);
  });
});

describe("requestDisplayState", () => {
  it("prefers the state the server derived", () => {
    expect(requestDisplayState("completed", "active", "processing")).toBe("processing");
  });

  it("derives a state for a server that sends none", () => {
    expect(requestDisplayState("downloading", "active")).toBe("processing");
    expect(requestDisplayState("queued", "failed")).toBe("failed");
  });
});

describe("requestDetailHref", () => {
  it("builds the title detail route", () => {
    expect(requestDetailHref("movie", 603)).toBe("/title/movie/603");
    expect(requestDetailHref("series", 1399)).toBe("/title/series/1399");
  });
});

describe("parseRequestMediaType", () => {
  it("accepts the two title media types", () => {
    expect(parseRequestMediaType("movie")).toBe("movie");
    expect(parseRequestMediaType("series")).toBe("series");
  });

  it.each([undefined, "", "tv", "Movie", "browse"])("rejects %j", (value) => {
    expect(parseRequestMediaType(value)).toBeUndefined();
  });
});

describe("season requests", () => {
  const now = new Date(Date.UTC(2026, 4, 24, 12));
  const season = (overrides: Partial<RequestMediaSeason>): RequestMediaSeason => ({
    season_number: 1,
    episode_count: 10,
    air_date: "2022-01-01",
    availability: "missing",
    requested: false,
    ...overrides,
  });

  it("names seasons compactly", () => {
    expect(formatSeasonList([2])).toBe("Season 2");
    expect(formatSeasonList([5, 1, 2, 3, 3])).toBe("Seasons 1–3, 5");
  });

  it("picks the aired seasons the library lacks and nobody requested", () => {
    const seasons = [
      season({ season_number: 1, availability: "available" }),
      season({ season_number: 2, availability: "partial" }),
      season({ season_number: 3, requested: true }),
      season({ season_number: 4 }),
      season({ season_number: 5, air_date: "2026-05-24" }),
      season({ season_number: 6, air_date: "2026-09-01" }),
      season({ season_number: 7, air_date: undefined, episode_count: 0 }),
    ];
    expect(defaultRequestSeasons(seasons, now)).toEqual([2, 4, 5]);
  });

  it("describes a season and a request's progress", () => {
    expect(formatRequestSeasonMeta(season({ episode_count: 1 }))).toBe("2022 · 1 episode");
    expect(formatRequestSeasonMeta(season({ air_date: undefined, episode_count: 0 }))).toBe(
      "Not announced",
    );
    expect(
      formatSeasonProgress([
        { season_number: 1, episodes_aired: 9, episodes_available: 9 },
        { season_number: 2, episodes_aired: 10, episodes_available: 4 },
      ]),
    ).toBe("13 of 19 episodes in the library");
    expect(
      formatSeasonProgress([{ season_number: 1, episodes_aired: 0, episodes_available: 3 }]),
    ).toBe("");
    expect(formatRequestDisplayState("partially_available")).toBe("Partially available");
  });
});

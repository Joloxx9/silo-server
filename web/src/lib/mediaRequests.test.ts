import { describe, expect, it } from "vitest";
import type { MediaRequest } from "@/api/types";
import {
  canCancelOwnRequest,
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

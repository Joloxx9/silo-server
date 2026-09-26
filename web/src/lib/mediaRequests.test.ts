import { describe, expect, it } from "vitest";
import type { MediaRequest } from "@/api/types";
import { canCancelOwnRequest, requestDetailHref } from "./mediaRequests";

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

describe("requestDetailHref", () => {
  it("builds the request detail route", () => {
    expect(requestDetailHref("movie", 603)).toBe("/requests/movie/603");
    expect(requestDetailHref("series", 1399)).toBe("/requests/series/1399");
  });
});

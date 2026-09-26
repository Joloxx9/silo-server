import { describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import RequestPosterCard from "./RequestPosterCard";
import type { MediaRequest, RequestMediaResult } from "@/api/types";

const requestable: RequestMediaResult = {
  media_type: "movie",
  tmdb_id: 42,
  title: "Test Movie",
  availability: "missing",
  request: { requestable: true },
};

describe("RequestPosterCard (discover variant)", () => {
  it("renders the hover Request button when onRequest is provided", () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <RequestPosterCard
          variant="discover"
          item={requestable}
          isSubmitting={false}
          onRequest={() => {}}
        />
      </MemoryRouter>,
    );
    // Must render an actual <button> with the "Request" label, not just any "Request"
    // substring (the /requests/... URL would match a naive includes check).
    expect(markup).toMatch(/<button[^>]*>[\s\S]*?Request[\s\S]*?<\/button>/);
  });

  it("contains the hover overlay inside the poster frame", () => {
    render(
      <MemoryRouter>
        <RequestPosterCard
          variant="discover"
          item={requestable}
          isSubmitting={false}
          onRequest={() => {}}
        />
      </MemoryRouter>,
    );

    const overlay = screen.getByTestId("request-poster-hover-overlay");
    const posterFrame = overlay.closest(".media-card-image");

    expect(posterFrame).not.toBeNull();
  });

  it("does not render the hover Request button when onRequest is omitted", () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <RequestPosterCard variant="discover" item={requestable} />
      </MemoryRouter>,
    );

    // The discover variant only contains one <button> (the hover Request action);
    // its absence is the strongest signal that the button was suppressed.
    expect(markup).not.toContain("<button");
  });

  it("shows the media type so same-title movies and series stay distinguishable", () => {
    const movieMarkup = renderToStaticMarkup(
      <MemoryRouter>
        <RequestPosterCard variant="discover" item={requestable} />
      </MemoryRouter>,
    );
    const seriesMarkup = renderToStaticMarkup(
      <MemoryRouter>
        <RequestPosterCard
          variant="discover"
          item={{ ...requestable, media_type: "series", tmdb_id: 43 }}
        />
      </MemoryRouter>,
    );

    expect(movieMarkup).toContain(">Movie<");
    expect(seriesMarkup).toContain(">Series<");
  });

  it("marks a title already in the library as Available", () => {
    render(
      <MemoryRouter>
        <RequestPosterCard
          variant="discover"
          item={{
            ...requestable,
            availability: "available",
            request: { requestable: false, reason: "already_available" },
          }}
        />
      </MemoryRouter>,
    );

    expect(screen.getByText("Available").closest("[data-request-state]")).toHaveAttribute(
      "data-request-state",
      "available",
    );
  });

  it("names the reason when a title without a request cannot be requested", () => {
    render(
      <MemoryRouter>
        <RequestPosterCard
          variant="discover"
          item={{ ...requestable, request: { requestable: false, reason: "quota_exceeded" } }}
        />
      </MemoryRouter>,
    );

    expect(screen.getByText("Request limit reached")).toBeInTheDocument();
  });
});

describe("RequestPosterCard (mine variant)", () => {
  const request: MediaRequest = {
    id: "req-1",
    provider: "silo",
    media_type: "movie",
    tmdb_id: 603,
    title: "The Matrix",
    status: "queued",
    outcome: "active",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };

  it.each<[Partial<MediaRequest>, string]>([
    [{ status: "pending" }, "Pending"],
    [{ status: "queued" }, "Processing"],
    [{ status: "downloading" }, "Processing"],
    [{ status: "completed" }, "Available"],
    [{ status: "pending", outcome: "cancelled" }, "Cancelled"],
    [{ status: "approved", outcome: "failed" }, "Failed"],
  ])("labels %o as %s", (overrides, label) => {
    render(
      <MemoryRouter>
        <RequestPosterCard variant="mine" request={{ ...request, ...overrides }} />
      </MemoryRouter>,
    );

    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it("shows Cancel request only when the page passes onCancel", () => {
    const onCancel = vi.fn();
    const { rerender } = render(
      <MemoryRouter>
        <RequestPosterCard variant="mine" request={{ ...request, status: "pending" }} />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("button", { name: /Cancel request/ })).not.toBeInTheDocument();

    rerender(
      <MemoryRouter>
        <RequestPosterCard
          variant="mine"
          request={{ ...request, status: "pending" }}
          onCancel={onCancel}
        />
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Cancel request for The Matrix" }));

    expect(onCancel).toHaveBeenCalledOnce();
  });
});

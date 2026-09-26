import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import type { MediaRequest, RequestMediaDetail } from "@/api/types";

const mocks = vi.hoisted(() => ({
  detail: undefined as unknown,
  mine: [] as unknown[],
  useMyMediaRequests: vi.fn(),
  cancel: vi.fn(),
}));

vi.mock("@/hooks/queries/useRequests", () => ({
  useRequestMediaDetail: () => ({ data: mocks.detail, isLoading: false, isError: false }),
  useCreateMediaRequest: () => ({ mutate: vi.fn(), isPending: false, variables: undefined }),
  useMyMediaRequests: (...args: unknown[]) => {
    mocks.useMyMediaRequests(...args);
    return { data: mocks.mine };
  },
  useCancelMediaRequest: () => ({ mutate: mocks.cancel, isPending: false }),
}));
vi.mock("@/hooks/useDocumentTitle", () => ({ useDocumentTitle: () => {} }));
vi.mock("@/pages/ItemDetail/DetailHero", () => ({
  default: ({
    title,
    metadata,
    actions,
  }: {
    title: string;
    metadata: ReactNode;
    actions: ReactNode;
  }) => (
    <section>
      <h1>{title}</h1>
      {metadata}
      {actions}
    </section>
  ),
}));
vi.mock("@/components/CastCarousel", () => ({ default: () => null }));
vi.mock("@/components/MediaCarousel", () => ({ default: () => null }));

import RequestDetail from "./RequestDetail";

const baseDetail: RequestMediaDetail = {
  media_type: "movie",
  tmdb_id: 603,
  title: "The Matrix",
  runtime: 136,
  availability: "missing",
  request: { requestable: false, status: "pending", request_id: "req-1" },
};

const ownPending: MediaRequest = {
  id: "req-1",
  provider: "silo",
  media_type: "movie",
  tmdb_id: 603,
  title: "The Matrix",
  status: "pending",
  outcome: "active",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function renderDetail() {
  render(
    <MemoryRouter initialEntries={["/requests/movie/603"]}>
      <Routes>
        <Route path="/requests/:mediaType/:tmdbId" element={<RequestDetail />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("RequestDetail", () => {
  beforeEach(() => {
    mocks.detail = baseDetail;
    mocks.mine = [ownPending];
    mocks.useMyMediaRequests.mockReset();
    mocks.cancel.mockReset();
  });

  it("lets the viewer cancel their own pending request after confirming", () => {
    renderDetail();

    expect(screen.getByText("Pending").closest("[data-request-state]")).toHaveAttribute(
      "data-request-state",
      "pending",
    );
    fireEvent.click(screen.getByRole("button", { name: "Cancel request" }));
    fireEvent.click(
      within(screen.getByRole("alertdialog")).getByRole("button", { name: "Cancel request" }),
    );

    expect(mocks.cancel).toHaveBeenCalledExactlyOnceWith("req-1");
  });

  it("hides Cancel request when the pending request belongs to someone else", () => {
    mocks.mine = [{ ...ownPending, id: "req-other" }];
    renderDetail();

    expect(screen.getByText("Pending")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel request" })).not.toBeInTheDocument();
  });

  it("hides Cancel request once the request has been sent", () => {
    mocks.detail = { ...baseDetail, request: { ...baseDetail.request, status: "downloading" } };
    renderDetail();

    expect(screen.getByText("Processing")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel request" })).not.toBeInTheDocument();
    // The ownership lookup only runs while the request could still be withdrawn.
    expect(mocks.useMyMediaRequests).toHaveBeenCalledWith(
      { outcome: "active" },
      { enabled: false },
    );
  });

  it("marks a title already in the library as Available", () => {
    mocks.detail = {
      ...baseDetail,
      availability: "available",
      library_content_id: "movie-603",
      request: { requestable: false, reason: "already_available" },
    };
    renderDetail();

    expect(screen.getByText("Available")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Open in library/ })).toHaveAttribute(
      "href",
      "/item/movie-603",
    );
  });

  it("formats the runtime with the shared runtime formatter", () => {
    renderDetail();

    expect(screen.getByText("2h 16m")).toBeInTheDocument();
  });
});

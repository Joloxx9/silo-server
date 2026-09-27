import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { MediaRequest } from "@/api/types";

const mocks = vi.hoisted(() => ({
  mine: [] as MediaRequest[],
  cancel: vi.fn(),
}));

vi.mock("@/hooks/queries/useRequests", () => {
  const idle = { data: [], isLoading: false, isError: false, refetch: vi.fn() };
  return {
    useRequestDiscovery: () => idle,
    useDiscoverStudios: () => idle,
    useDiscoverNetworks: () => idle,
    useDiscoverGenres: () => idle,
    useRequestSearch: () => ({ data: undefined, isLoading: false, isFetching: false }),
    useMyMediaRequests: () => ({ data: mocks.mine, isLoading: false, isError: false }),
    useCreateMediaRequest: () => ({ mutate: vi.fn(), isPending: false, variables: undefined }),
    useCancelMediaRequest: () => ({ mutate: mocks.cancel, isPending: false, variables: undefined }),
  };
});
vi.mock("@/hooks/useDocumentTitle", () => ({ useDocumentTitle: () => {} }));
vi.mock("@/components/BrandCarousel", () => ({ default: () => null }));
vi.mock("@/components/MediaCarousel", () => ({
  default: ({ title, children }: { title: string; children: ReactNode }) => (
    <section aria-label={title}>{children}</section>
  ),
}));

import Requests from "./Requests";

function request(id: string, title: string, overrides: Partial<MediaRequest> = {}): MediaRequest {
  return {
    id,
    provider: "silo",
    media_type: "movie",
    tmdb_id: Number(id.replace(/\D/g, "")) || 1,
    title,
    status: "pending",
    outcome: "active",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function renderYours() {
  render(
    <MemoryRouter initialEntries={["/requests?tab=yours"]}>
      <Requests />
    </MemoryRouter>,
  );
}

describe("Requests (Yours tab)", () => {
  beforeEach(() => {
    mocks.cancel.mockReset();
    mocks.mine = [
      request("r1", "Waiting Movie"),
      request("r2", "Approved Movie", {
        status: "approved",
        targets: [{ quality: "1080p", status: "queued" }] as MediaRequest["targets"],
      }),
      request("r3", "Downloading Movie", { status: "downloading" }),
      request("r4", "Queued Movie", { status: "queued" }),
      request("r5", "Withdrawn Movie", { outcome: "cancelled" }),
    ];
  });

  it("offers Cancel request only on the viewer's pending requests", () => {
    renderYours();

    const buttons = screen.getAllByRole("button", { name: /^Cancel request for / });
    expect(buttons.map((button) => button.getAttribute("aria-label"))).toEqual([
      "Cancel request for Waiting Movie",
    ]);
  });

  it("cancels only after the viewer confirms", () => {
    renderYours();

    fireEvent.click(screen.getByRole("button", { name: "Cancel request for Waiting Movie" }));
    const dialog = screen.getByRole("alertdialog");
    expect(dialog).toHaveTextContent('Your request for "Waiting Movie" will be withdrawn');
    expect(mocks.cancel).not.toHaveBeenCalled();

    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel request" }));

    expect(mocks.cancel).toHaveBeenCalledExactlyOnceWith("r1");
  });

  it("keeps the request when the viewer backs out", () => {
    renderYours();

    fireEvent.click(screen.getByRole("button", { name: "Cancel request for Waiting Movie" }));
    fireEvent.click(screen.getByRole("button", { name: "Keep request" }));

    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(mocks.cancel).not.toHaveBeenCalled();
  });

  it("uses the shared status vocabulary and makes no live-update promise", () => {
    renderYours();

    const guide = screen.getByRole("region", { name: "Status guide" });
    for (const label of [
      "Pending",
      "Approved",
      "Processing",
      "Available",
      "Declined",
      "Cancelled",
      "Failed",
    ]) {
      expect(within(guide).getByText(label)).toBeInTheDocument();
    }
    expect(screen.queryByText(/update automatically/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/live status/i)).not.toBeInTheDocument();
    expect(within(guide).queryByText("Queued")).not.toBeInTheDocument();
    expect(within(guide).queryByText("Completed")).not.toBeInTheDocument();
  });

  it("counts requests per status in the summary", () => {
    renderYours();

    const pending = document.querySelector(
      '[data-request-state="pending"] .tabular-nums',
    ) as HTMLElement;
    expect(pending).toHaveTextContent("1");
    expect(
      document.querySelector('[data-request-state="processing"] .tabular-nums'),
    ).toHaveTextContent("2");
  });
});

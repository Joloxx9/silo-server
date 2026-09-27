import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import type { DiscoverBrowseResponse } from "@/api/types";

const mocks = vi.hoisted(() => ({
  useRequestBrowse: vi.fn(),
  mutate: vi.fn(),
}));

vi.mock("@/hooks/queries/useRequests", () => ({
  useRequestBrowse: (...args: unknown[]) => mocks.useRequestBrowse(...args),
  useCreateMediaRequest: () => ({ mutateAsync: mocks.mutate, isPending: false }),
}));
vi.mock("@/hooks/useDocumentTitle", () => ({ useDocumentTitle: () => {} }));

import RequestBrowse from "./RequestBrowse";

function browse(page: number): DiscoverBrowseResponse {
  return {
    kind: "genre",
    slug: "drama",
    display_name: "Drama",
    media_type: "movie",
    sort: "popularity",
    page,
    total_pages: 4,
    results: [
      {
        media_type: "movie",
        tmdb_id: 42,
        title: "Heat",
        availability: "missing",
        request: { requestable: true },
      },
    ],
  };
}

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location">{`${location.pathname}${location.search}`}</output>;
}

function renderAt(url: string) {
  render(
    <MemoryRouter initialEntries={[url]}>
      <Routes>
        <Route path="/requests/browse/genre/:slug" element={<RequestBrowse kind="genre" />} />
        <Route path="*" element={null} />
      </Routes>
      <LocationProbe />
    </MemoryRouter>,
  );
}

describe("RequestBrowse", () => {
  beforeEach(() => {
    mocks.mutate.mockReset();
    mocks.useRequestBrowse.mockReset();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    mocks.useRequestBrowse.mockImplementation(({ page }: { page: number }) => ({
      data: browse(page),
      isLoading: false,
      isError: false,
    }));
  });

  it("lays out a genre like the other full grids: back link, title, then posters", () => {
    renderAt("/requests/browse/genre/drama?media_type=series&page=2");

    expect(mocks.useRequestBrowse).toHaveBeenLastCalledWith({
      kind: "genre",
      slug: "drama",
      mediaType: "series",
      sort: "popularity",
      page: 2,
    });
    expect(screen.getByRole("button", { name: "Go back" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Drama" })).toBeInTheDocument();
    expect(screen.getByText("Genre · Page 2 of 4")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Series" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getAllByRole("link", { name: "Heat" })[0]).toHaveAttribute(
      "href",
      "/title/movie/42",
    );
  });

  it("pages through the results", () => {
    renderAt("/requests/browse/genre/drama?media_type=movie");

    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    expect(screen.getByTestId("location")).toHaveTextContent(
      "/requests/browse/genre/drama?media_type=movie&page=2",
    );
  });

  it("stops at TMDB's 500-page cap whatever total it reports", () => {
    mocks.useRequestBrowse.mockImplementation(({ page }: { page: number }) => ({
      data: { ...browse(page), total_pages: 1001 },
      isLoading: false,
      isError: false,
    }));
    renderAt("/requests/browse/genre/drama?media_type=movie&page=500");

    expect(screen.getByText("Genre · Page 500 of 500")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Previous" })).toBeEnabled();
  });

  it("requests a title from its card", () => {
    mocks.mutate.mockResolvedValue({});
    renderAt("/requests/browse/genre/drama");

    fireEvent.click(screen.getByRole("button", { name: /^Request Heat/ }));

    expect(mocks.mutate).toHaveBeenCalledWith(
      expect.objectContaining({ media_type: "movie", tmdb_id: 42 }),
    );
  });
});

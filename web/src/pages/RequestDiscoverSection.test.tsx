import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import type { RequestDiscoverySection } from "@/api/types";
import { V2ProblemError } from "@/api/v2/request";

const mocks = vi.hoisted(() => ({
  useRequestDiscoverySection: vi.fn(),
}));

vi.mock("@/hooks/queries/useRequests", () => ({
  useRequestDiscoverySection: (...args: unknown[]) => mocks.useRequestDiscoverySection(...args),
  useCreateMediaRequest: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));
vi.mock("@/hooks/useDocumentTitle", () => ({ useDocumentTitle: () => {} }));

import RequestDiscoverSection from "./RequestDiscoverSection";

function section(page: number, totalPages: number): RequestDiscoverySection {
  return {
    key: "trending_movies",
    title: "Trending Movies",
    page,
    total_pages: totalPages,
    total_results: totalPages * 20,
    results: [
      {
        media_type: "movie",
        tmdb_id: page * 100,
        title: `Movie on page ${page}`,
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
        <Route path="/requests/discover/:section" element={<RequestDiscoverSection />} />
        <Route path="*" element={null} />
      </Routes>
      <LocationProbe />
    </MemoryRouter>,
  );
}

describe("RequestDiscoverSection", () => {
  beforeEach(() => {
    mocks.useRequestDiscoverySection.mockReset();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    mocks.useRequestDiscoverySection.mockImplementation((_key: string, page: number) => ({
      data: section(page, 3),
      isLoading: false,
      isError: false,
      isPlaceholderData: false,
      refetch: vi.fn(),
    }));
  });

  it("shows every title of a Discover row, page by page", () => {
    renderAt("/requests/discover/trending_movies?page=2");

    expect(mocks.useRequestDiscoverySection).toHaveBeenLastCalledWith("trending_movies", 2);
    expect(screen.getByRole("heading", { level: 1, name: "Trending Movies" })).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Movie on page 2" })[0]).toHaveAttribute(
      "href",
      "/title/movie/200",
    );
    expect(screen.getByRole("navigation", { name: "Result pages" })).toHaveTextContent(
      "Page 2 of 3",
    );

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/requests/discover/trending_movies?page=3",
    );

    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/requests/discover/trending_movies");
    expect(screen.getByTestId("location")).not.toHaveTextContent("page=");
  });

  it("follows the server's next_page for a rating-restricted profile and walks back the pages visited", () => {
    // A restricted profile's page reads several TMDB pages: 1 covers 1–3,
    // 4 covers 4–8, and 9 reaches the end. page + 1 would repeat titles.
    const cursors: Record<number, number | undefined> = { 1: 4, 4: 9, 9: undefined };
    mocks.useRequestDiscoverySection.mockImplementation((_key: string, page: number) => ({
      data: { ...section(page, 40), next_page: cursors[page] },
      isLoading: false,
      isError: false,
      isPlaceholderData: false,
      refetch: vi.fn(),
    }));
    renderAt("/requests/discover/trending_movies");

    const location = () => screen.getByTestId("location").textContent;
    const pager = () => screen.getByRole("navigation", { name: "Result pages" });
    expect(pager()).toHaveTextContent("Page 1");
    expect(pager()).not.toHaveTextContent("of");

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(location()).toBe("/requests/discover/trending_movies?page=4");
    expect(pager()).toHaveTextContent("Page 2");
    expect(pager()).not.toHaveTextContent("of");

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(location()).toBe("/requests/discover/trending_movies?page=9");
    expect(pager()).toHaveTextContent("Page 3");
    // Reached by cursor and none further: the end, even though TMDB counts 40 pages.
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    expect(location()).toBe("/requests/discover/trending_movies?page=4");
    expect(pager()).toHaveTextContent("Page 2");

    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    expect(location()).toBe("/requests/discover/trending_movies");
    expect(pager()).toHaveTextContent("Page 1");
  });

  it("goes back to the start from a shared link to a cursor page", () => {
    mocks.useRequestDiscoverySection.mockImplementation((_key: string, page: number) => ({
      data: { ...section(page, 40), next_page: page === 9 ? 12 : 3 },
      isLoading: false,
      isError: false,
      isPlaceholderData: false,
      refetch: vi.fn(),
    }));
    renderAt("/requests/discover/trending_movies?page=9");

    // Which page of the visit this is can't be known, so it isn't numbered.
    expect(screen.getByRole("navigation", { name: "Result pages" })).not.toHaveTextContent("Page");

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByTestId("location")).toHaveTextContent("?page=12");
    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    expect(screen.getByTestId("location")).toHaveTextContent("?page=9");
    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    expect(screen.getByTestId("location").textContent).toBe("/requests/discover/trending_movies");
  });

  it("stops at TMDB's 500-page cap whatever total it reports", () => {
    mocks.useRequestDiscoverySection.mockImplementation((_key: string, page: number) => ({
      data: section(page, 900),
      isLoading: false,
      isError: false,
      isPlaceholderData: false,
      refetch: vi.fn(),
    }));
    renderAt("/requests/discover/trending_movies?page=500");

    expect(screen.getByRole("navigation", { name: "Result pages" })).toHaveTextContent(
      "Page 500 of 500",
    );
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
  });

  it("goes back to the Requests hub", () => {
    renderAt("/requests/discover/trending_movies");

    fireEvent.click(screen.getByRole("button", { name: "Go back" }));

    expect(screen.getByTestId("location")).toHaveTextContent(/^\/requests$/);
  });

  it("says so when the row does not exist", () => {
    mocks.useRequestDiscoverySection.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new V2ProblemError("getDiscoverSection", {
        type: "validation_failed",
        title: "Invalid",
        status: 422,
      } as ConstructorParameters<typeof V2ProblemError>[1]),
      isPlaceholderData: false,
      refetch: vi.fn(),
    });
    renderAt("/requests/discover/nope");

    expect(screen.getByRole("heading", { level: 1, name: "Not found" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
  });
});

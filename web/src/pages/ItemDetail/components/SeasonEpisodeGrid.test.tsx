import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SeasonEpisodeGrid from "./SeasonEpisodeGrid";

const capturedMenuProps: Record<string, unknown>[] = [];

vi.mock("@/components/MediaItemMenu", () => ({
  default: (props: Record<string, unknown>) => {
    capturedMenuProps.push(props);
    return null;
  },
}));

vi.mock("@/hooks/useOverlayPrefs", () => ({
  useOverlayPrefs: () => ({ prefs: null, quickActionMode: "watched" }),
}));

vi.mock("@/hooks/queries/catalogRead", () => ({
  usePrefetchCatalogItemDetail: () => vi.fn(),
}));

describe("SeasonEpisodeGrid", () => {
  beforeEach(() => {
    capturedMenuProps.length = 0;
  });

  it("enables the watched shortcut on episode cards", () => {
    render(
      <MemoryRouter>
        <SeasonEpisodeGrid
          isLoading={false}
          episodes={[
            {
              content_id: "ep-1",
              season_number: 1,
              episode_number: 1,
              title: "Pilot",
              overview: "A beginning.",
              air_date: null,
              runtime: 42,
              still_url: "",
              still_thumbhash: "",
              files: [],
            },
          ]}
        />
      </MemoryRouter>,
    );

    expect(capturedMenuProps[0]).toMatchObject({
      contentId: "ep-1",
      mediaType: "episode",
      userState: {
        played: false,
        is_favorite: false,
        in_watchlist: false,
      },
      showCollectionActions: false,
      showWatchedShortcut: true,
      hasPartialProgress: false,
      quickActionMode: "watched",
    });
  });

  it("places the watched circle-check beside the episode label instead of over the artwork", () => {
    render(
      <MemoryRouter>
        <SeasonEpisodeGrid
          isLoading={false}
          episodes={[
            {
              content_id: "ep-1",
              season_number: 1,
              episode_number: 1,
              title: "Pilot",
              overview: "A beginning.",
              air_date: null,
              runtime: 42,
              still_url: "",
              still_thumbhash: "",
              files: [],
              user_data: {
                played: true,
                position_seconds: 1800,
                duration_seconds: 1800,
              },
            },
            {
              content_id: "ep-2",
              season_number: 1,
              episode_number: 2,
              title: "Next",
              overview: "Another episode.",
              air_date: null,
              runtime: 43,
              still_url: "",
              still_thumbhash: "",
              files: [],
              user_data: {
                played: false,
                position_seconds: 0,
                duration_seconds: 1800,
              },
            },
          ]}
        />
      </MemoryRouter>,
    );

    const episodeLabel = screen.getByText("Episode 1");
    const watchedIndicator = screen.getByLabelText("Watched");

    expect(episodeLabel.parentElement).toContainElement(watchedIndicator);
    expect(watchedIndicator).toHaveAttribute("data-watched-indicator", "icon-only");
    expect(watchedIndicator.querySelector(".lucide-circle-check")).toBeTruthy();
    expect(watchedIndicator.closest(".media-card-image")).toBeNull();
    expect(screen.getByText("Episode 2").parentElement).not.toContainElement(watchedIndicator);
    expect(screen.getAllByLabelText("Watched")).toHaveLength(1);
  });
  it("caps the grid at four rows and scrolls the rest", () => {
    render(
      <MemoryRouter>
        <SeasonEpisodeGrid
          isLoading={false}
          episodes={[
            {
              content_id: "ep-1",
              season_number: 1,
              episode_number: 1,
              title: "Pilot",
              overview: "An episode.",
              air_date: null,
              runtime: 42,
              still_url: "",
              still_thumbhash: "",
              files: [],
            },
            {
              content_id: "ep-2",
              season_number: 1,
              episode_number: 2,
              title: "Second",
              overview: "An episode.",
              air_date: null,
              runtime: 42,
              still_url: "",
              still_thumbhash: "",
              files: [],
            },
          ]}
        />
      </MemoryRouter>,
    );

    const grid = screen.getByText("Pilot").closest(".grid");
    expect(grid).not.toBeNull();
    // Two columns is the narrowest layout. A single column made the capped
    // section over two viewports tall on a phone, so it scrolled inside a
    // region the reader could not see the extent of.
    expect(grid).toHaveClass("grid-cols-2", "sm:grid-cols-3", "lg:grid-cols-5");
    expect(grid).toHaveClass("overflow-y-auto");
    // Two episodes is under the cap, so nothing is clipped and the section
    // keeps its natural height rather than showing an inert scrollport.
    expect((grid as HTMLElement).style.maxHeight).toBe("");
  });

  it("renders an aired-but-missing episode as a dimmed, non-clickable placeholder", () => {
    render(
      <MemoryRouter>
        <SeasonEpisodeGrid
          isLoading={false}
          episodes={[
            {
              content_id: "ep-1",
              season_number: 1,
              episode_number: 3,
              title: "The Vault",
              overview: "Not downloaded yet.",
              air_date: "2024-01-05",
              runtime: 42,
              still_url: "https://example.com/still.jpg",
              still_thumbhash: "",
              files: [],
              availability: "missing",
            },
          ]}
        />
      </MemoryRouter>,
    );

    expect(screen.getByText("Not in library")).toBeInTheDocument();
    expect(screen.queryAllByRole("link")).toHaveLength(0);
    expect(capturedMenuProps).toHaveLength(0);
    const image = screen.getByAltText("The Vault");
    expect(image).toHaveClass("grayscale", "opacity-50");
  });

  it("renders an unaired episode as an empty placeholder with the air date, no still image", () => {
    render(
      <MemoryRouter>
        <SeasonEpisodeGrid
          isLoading={false}
          episodes={[
            {
              content_id: "ep-2",
              season_number: 1,
              episode_number: 4,
              title: "Future Episode",
              overview: "",
              air_date: "2099-09-23",
              runtime: 0,
              still_url: "",
              still_thumbhash: "",
              files: [],
              availability: "unaired",
            },
          ]}
        />
      </MemoryRouter>,
    );

    expect(screen.getByText(/Airs/)).toBeInTheDocument();
    expect(screen.queryAllByRole("link")).toHaveLength(0);
    expect(capturedMenuProps).toHaveLength(0);
    expect(screen.queryByAltText("Future Episode")).toBeNull();
  });
});

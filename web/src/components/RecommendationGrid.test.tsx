import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BrowseItem } from "@/api/types";
import RecommendationGrid from "./RecommendationGrid";

const mocks = vi.hoisted(() => ({
  ItemCard: vi.fn(),
}));

vi.mock("@/components/ItemCard", () => ({
  default: (props: { item: BrowseItem; quickActionMode?: string }) => {
    mocks.ItemCard(props);
    return <a href={`/item/${props.item.content_id}`}>{props.item.title}</a>;
  },
}));

vi.mock("@/hooks/useOverlayPrefs", () => ({
  useOverlayPrefs: () => ({ prefs: null, quickActionMode: "both" }),
}));

function card(contentId: string, played = false): BrowseItem {
  return {
    content_id: contentId,
    title: contentId,
    type: "movie",
    status: "matched",
    user_state: { played, is_favorite: false, in_watchlist: false, not_interested: false },
  } as BrowseItem;
}

describe("RecommendationGrid", () => {
  beforeEach(() => {
    mocks.ItemCard.mockReset();
  });

  it("renders at most 12 recommendation cards", () => {
    const items = Array.from({ length: 14 }, (_, index) => card(`item-${index + 1}`));

    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <RecommendationGrid items={items} maxItems={50} />
      </MemoryRouter>,
    );

    expect(mocks.ItemCard).toHaveBeenCalledTimes(12);
    expect(markup).not.toContain("item-13");
  });

  it("draws each recommendation as the library poster card with its watched state", () => {
    renderToStaticMarkup(
      <MemoryRouter>
        <RecommendationGrid items={[card("watched-movie", true)]} />
      </MemoryRouter>,
    );

    expect(mocks.ItemCard).toHaveBeenCalledWith(
      expect.objectContaining({
        item: expect.objectContaining({
          content_id: "watched-movie",
          user_state: expect.objectContaining({ played: true }),
        }),
        quickActionMode: "both",
      }),
    );
  });
});

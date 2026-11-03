import type { ReactNode } from "react";
import type { BrowseItem } from "@/api/types";
import ItemCard from "@/components/ItemCard";
import MediaCarousel from "@/components/MediaCarousel";
import { useOverlayPrefs } from "@/hooks/useOverlayPrefs";
import { useUICustomization } from "@/hooks/useUICustomization";
import { carouselCardWidthClasses } from "@/lib/uiCustomization";

const MAX_MORE_LIKE_THIS_ITEMS = 12;

interface RecommendationGridProps {
  items: BrowseItem[];
  maxItems?: number;
}

interface MoreLikeThisRowProps<T> {
  items: T[];
  itemKey: (item: T) => string;
  renderItem: (item: T) => ReactNode;
  maxItems?: number;
}

/**
 * The "More Like This" rail on detail pages: poster-width slides inside the
 * page shell, sized by the viewer's card settings. Library items and titles
 * known only from TMDB supply their own cards.
 */
export function MoreLikeThisRow<T>({
  items,
  itemKey,
  renderItem,
  maxItems = MAX_MORE_LIKE_THIS_ITEMS,
}: MoreLikeThisRowProps<T>) {
  const { cardPresentation } = useUICustomization();
  const itemLimit = Math.max(0, Math.min(maxItems, MAX_MORE_LIKE_THIS_ITEMS));
  const posterWidthClasses = carouselCardWidthClasses(cardPresentation.poster_size);

  return (
    <MediaCarousel title="More Like This" edgePadding={false}>
      {items.slice(0, itemLimit).map((item) => (
        <div key={itemKey(item)} className={posterWidthClasses}>
          {renderItem(item)}
        </div>
      ))}
    </MediaCarousel>
  );
}

export default function RecommendationGrid({ items, maxItems = 12 }: RecommendationGridProps) {
  // The library's poster card, so a recommendation shows the watched and
  // favorite state, badges and menu every other poster row does.
  const { prefs: overlayPrefs, quickActionMode } = useOverlayPrefs();
  return (
    <MoreLikeThisRow
      items={items}
      maxItems={maxItems}
      itemKey={(item) => item.content_id}
      renderItem={(item) => (
        <ItemCard item={item} overlayPrefs={overlayPrefs} quickActionMode={quickActionMode} />
      )}
    />
  );
}

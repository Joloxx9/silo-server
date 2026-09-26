import { useLocation, useParams, useSearchParams } from "react-router";
import { RefreshCw } from "lucide-react";
import type { RequestDiscoverySection } from "@/api/types";
import { V2ProblemError } from "@/api/v2/request";
import PageBack from "@/components/PageBack";
import RequestResultsGrid, {
  RequestResultsGridSkeleton,
  RequestResultsPager,
} from "@/components/RequestResultsGrid";
import { useRequestDiscoverySection } from "@/hooks/queries/useRequests";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { tmdbPageCount } from "@/lib/mediaRequests";
import { cn } from "@/lib/utils";

/**
 * How the viewer got to this page, kept in history state so Previous, the
 * browser's Back, and a reload all keep it.
 */
interface SectionPaging {
  /** The pages visited before this one since entering the row, oldest first. */
  visited: number[];
  /** This page was reached through the server's next_page cursor. */
  viaCursor: boolean;
  /** The visit began on the first page, so the visited count is the page number. */
  fromFirst: boolean;
}

/**
 * Every title of one Discover row (Trending Movies, Popular Series, ...),
 * page by page: the hub's "Explore all" destination.
 *
 * For a rating-restricted profile the server may read several TMDB pages to
 * fill one and answers with next_page, the page to ask for next; page + 1
 * would repeat titles. Such pages are not numbered, and Previous walks back
 * through the pages actually visited.
 */
export default function RequestDiscoverSection() {
  const { section = "" } = useParams<{ section: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  const page = normalizePage(searchParams.get("page"));
  const query = useRequestDiscoverySection(section, page);

  const title = query.data?.title || humanizeSectionKey(section);
  useDocumentTitle(`${title} - Requests`);

  const results = query.data?.results ?? [];
  const unknownSection =
    query.isError &&
    query.error instanceof V2ProblemError &&
    [400, 404, 422].includes(query.error.status);
  const paging = readPaging(location.state) ?? {
    visited: [],
    viaCursor: false,
    fromFirst: page === 1,
  };
  const nav = sectionNavigation(page, query.data, paging);

  function goTo(target: number, next: SectionPaging) {
    const params = new URLSearchParams(searchParams);
    if (target <= 1) {
      params.delete("page");
    } else {
      params.set("page", String(target));
    }
    setSearchParams(params, { state: next });
    window.scrollTo({ top: 0, behavior: "smooth" });
  }

  return (
    <div className="relative space-y-6 px-4 pt-6 pb-12 sm:px-6 lg:px-10 xl:px-12">
      <PageBack to="/requests" up />
      <header className="mt-10 flex flex-col gap-1.5 sm:mt-12">
        <h1 className="text-foreground text-2xl font-bold tracking-tight sm:text-3xl">
          {unknownSection ? "Not found" : title}
        </h1>
        {nav.position && !unknownSection ? (
          <p className="text-muted-foreground text-sm tabular-nums">{nav.position}</p>
        ) : null}
      </header>

      {query.isLoading ? (
        <RequestResultsGridSkeleton />
      ) : unknownSection ? (
        <p className="text-muted-foreground text-sm">This Discover row doesn&rsquo;t exist.</p>
      ) : query.isError ? (
        <div className="flex flex-col items-center justify-center gap-4 py-24 text-center">
          <p className="text-muted-foreground text-sm">
            TMDB couldn&rsquo;t be reached. Try again in a moment.
          </p>
          <button
            type="button"
            onClick={() => void query.refetch()}
            className="text-primary hover:text-primary/80 inline-flex items-center gap-2 text-sm font-medium"
          >
            <RefreshCw className="h-4 w-4" />
            Retry
          </button>
        </div>
      ) : results.length === 0 ? (
        <p className="text-muted-foreground text-sm">Nothing here right now.</p>
      ) : (
        <RequestResultsGrid
          results={results}
          className={cn("transition-opacity", query.isPlaceholderData && "opacity-60")}
        />
      )}

      {!unknownSection ? (
        <RequestResultsPager
          position={nav.position}
          hasPrevious={page > 1}
          hasNext={nav.next !== null}
          onPrevious={() => goTo(nav.previous.page, nav.previous.paging)}
          onNext={() => nav.next && goTo(nav.next.page, nav.next.paging)}
          disabled={query.isPlaceholderData}
        />
      ) : null}
    </div>
  );
}

type PageStep = { page: number; paging: SectionPaging };

/** Where Previous and Next lead from this page, and how to label it. */
function sectionNavigation(
  page: number,
  data: RequestDiscoverySection | undefined,
  paging: SectionPaging,
): { position?: string; previous: PageStep; next: PageStep | null } {
  const totalPages = tmdbPageCount(data?.total_pages);
  const cursor = data?.next_page && data.next_page > page ? data.next_page : undefined;
  // Once a page was reached by cursor, a page without one is the last: page
  // + 1 would read TMDB pages the server already used.
  const cursorMode = cursor !== undefined || paging.viaCursor;

  const next: PageStep | null =
    cursor !== undefined || (!cursorMode && page < totalPages)
      ? {
          page: cursor ?? page + 1,
          paging: {
            visited: [...paging.visited, page],
            viaCursor: cursor !== undefined,
            fromFirst: paging.fromFirst,
          },
        }
      : null;

  const last = paging.visited[paging.visited.length - 1];
  // Without the visited pages (a shared link), a cursor page can only go back
  // to the start; a numbered page steps back one.
  const previousPage = last ?? (cursorMode ? 1 : page - 1);
  const previous: PageStep = {
    page: previousPage,
    paging:
      last !== undefined
        ? { visited: paging.visited.slice(0, -1), viaCursor: false, fromFirst: paging.fromFirst }
        : { visited: [], viaCursor: false, fromFirst: previousPage === 1 },
  };

  const position = cursorMode
    ? paging.fromFirst
      ? `Page ${paging.visited.length + 1}`
      : undefined
    : totalPages > 1
      ? `Page ${page} of ${totalPages}`
      : undefined;

  return { position, previous, next };
}

function readPaging(state: unknown): SectionPaging | null {
  if (!state || typeof state !== "object") return null;
  const { visited, viaCursor, fromFirst } = state as Partial<SectionPaging>;
  if (!Array.isArray(visited) || !visited.every((p) => Number.isInteger(p) && p > 0)) {
    return null;
  }
  return { visited, viaCursor: viaCursor === true, fromFirst: fromFirst === true };
}

function normalizePage(value: string | null): number {
  const parsed = Number(value ?? "1");
  return Number.isInteger(parsed) && parsed > 0 ? parsed : 1;
}

/** "trending_movies" → "Trending movies", until the server's title arrives. */
function humanizeSectionKey(key: string): string {
  const words = key.split(/[_-]+/).filter(Boolean).join(" ");
  return words ? words.charAt(0).toUpperCase() + words.slice(1) : "Discover";
}

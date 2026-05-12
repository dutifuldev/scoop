import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";

import { getCollections, getStoryDays, getStories, getTags } from "../api";
import { extractErrorMessage } from "../lib/viewerFormat";
import type {
  CollectionSummary,
  StoryDayBucket,
  StoryFilters,
  StoryListItem,
  StoryPagination,
  Tag,
} from "../types";

interface UseViewerQueriesArgs {
  filters: StoryFilters;
}

interface UseViewerQueriesResult {
  collections: CollectionSummary[];
  tags: Tag[];
  dayBuckets: StoryDayBucket[];
  stories: StoryListItem[];
  pagination: StoryPagination;
  globalError: string;
  storiesError: string;
  isStoriesPending: boolean;
  isFetchingNextStoriesPage: boolean;
  hasNextStoriesPage: boolean;
  fetchNextStoriesPage: () => void;
}

export function useViewerQueries({ filters }: UseViewerQueriesArgs): UseViewerQueriesResult {
  const collectionsQuery = useQuery<{ items: CollectionSummary[] }>({
    queryKey: ["collections"],
    queryFn: () => getCollections(),
  });

  const tagsQuery = useQuery<{ items: Tag[] }>({
    queryKey: ["tags"],
    queryFn: () => getTags(),
  });

  const dayBucketsQuery = useQuery<{ items: StoryDayBucket[] }>({
    queryKey: ["story-days", filters.collection],
    queryFn: () => getStoryDays(filters.collection, 45),
  });

  const storiesQuery = useInfiniteQuery<{ items: StoryListItem[]; pagination: StoryPagination }>({
    queryKey: [
      "stories",
      filters.collection,
      filters.query,
      filters.from,
      filters.to,
      filters.pageSize,
      filters.lang,
      filters.tag,
    ],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      getStories({
        ...filters,
        page: Number(pageParam),
      }),
    getNextPageParam: (lastPage) => {
      const nextPage = lastPage.pagination.page + 1;
      return nextPage <= lastPage.pagination.total_pages ? nextPage : undefined;
    },
  });

  const collections = collectionsQuery.data?.items ?? [];
  const tags = tagsQuery.data?.items ?? [];
  const dayBuckets = dayBucketsQuery.data?.items ?? [];
  const storyPages = storiesQuery.data?.pages ?? [];
  const stories = useMemo(() => storyPages.flatMap((page) => page.items), [storyPages]);

  const pagination = useMemo((): StoryPagination => {
    const firstPage = storyPages[0]?.pagination;
    const lastPage = storyPages[storyPages.length - 1]?.pagination;

    return {
      page: lastPage?.page ?? 1,
      page_size: lastPage?.page_size ?? filters.pageSize,
      total_items: Number(firstPage?.total_items ?? 0),
      total_pages: Math.max(1, Number(firstPage?.total_pages ?? 1)),
    };
  }, [filters.pageSize, storyPages]);

  const fetchNextStoriesPage = useCallback(() => {
    if (!storiesQuery.hasNextPage || storiesQuery.isFetchingNextPage) {
      return;
    }
    void storiesQuery.fetchNextPage();
  }, [storiesQuery]);

  const globalError = useMemo(() => {
    if (collectionsQuery.error) return extractErrorMessage(collectionsQuery.error);
    if (tagsQuery.error) return extractErrorMessage(tagsQuery.error);
    if (dayBucketsQuery.error) return extractErrorMessage(dayBucketsQuery.error);
    return "";
  }, [collectionsQuery.error, dayBucketsQuery.error, tagsQuery.error]);

  const storiesError = storiesQuery.error ? extractErrorMessage(storiesQuery.error) : "";

  return {
    collections,
    tags,
    dayBuckets,
    stories,
    pagination,
    globalError,
    storiesError,
    isStoriesPending: storiesQuery.isPending && stories.length === 0,
    isFetchingNextStoriesPage: storiesQuery.isFetchingNextPage,
    hasNextStoriesPage: Boolean(storiesQuery.hasNextPage),
    fetchNextStoriesPage,
  };
}

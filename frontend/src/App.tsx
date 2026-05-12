import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Group, Panel, Separator } from "react-resizable-panels";

import { useAuth } from "./auth";
import { CollectionDropdown } from "./components/header/CollectionDropdown";
import { DayNavigator } from "./components/header/DayNavigator";
import { PageShell } from "./components/PageShell";
import { SettingsModal } from "./components/SettingsModal";
import { StoriesListPanel } from "./components/StoriesListPanel";
import { StoryReaderPanel } from "./components/StoryReaderPanel";
import { useCurrentCollectionLabel } from "./hooks/useCurrentCollectionLabel";
import { useDayNavigationState } from "./hooks/useDayNavigationState";
import { useViewerQueries } from "./hooks/useViewerQueries";
import {
  getDesktopFeedWidthBounds,
  getDesktopFeedWidthPct,
  setDesktopFeedWidthPct,
} from "./lib/userSettings";
import { buildStoryFilters } from "./lib/viewerFilters";
import { formatCount } from "./lib/viewerFormat";
import { normalizeLanguageTag } from "./lib/language";
import type { ViewerSearch } from "./types";
import { compactViewerSearch, normalizeViewerSearch, toStoryFilters } from "./viewerSearch";

export function StoryViewerPage(): JSX.Element {
  const { user, settings, languages, logout, updateSettings } = useAuth();
  const fallbackLanguageOptions = useMemo(
    () => [
      { code: "en", label: "English" },
      { code: "original", label: "Original" },
    ],
    [],
  );
  const allCollectionsValue = "__all_collections__";
  const navigate = useNavigate();
  const rawSearch = useSearch({ strict: false });
  const rawParams = useParams({ strict: false }) as {
    collection?: string;
    storyUUID?: string;
    itemUUID?: string;
  };

  const viewerSearch = useMemo(
    () => normalizeViewerSearch(rawSearch as unknown as Record<string, unknown>),
    [rawSearch],
  );

  const [showAdvancedSearch, setShowAdvancedSearch] = useState(() =>
    Boolean(viewerSearch.from || viewerSearch.to || viewerSearch.tag),
  );
  const routeCollection =
    typeof rawParams.collection === "string" ? rawParams.collection.trim() : "";
  const baseFilters = useMemo(() => toStoryFilters(viewerSearch), [viewerSearch]);
  const filters = useMemo(
    () =>
      buildStoryFilters({
        baseFilters,
        routeCollection,
        showAdvancedSearch,
        day: viewerSearch.day || "",
      }),
    [baseFilters, routeCollection, showAdvancedSearch, viewerSearch.day],
  );
  const selectedStoryUUID = typeof rawParams.storyUUID === "string" ? rawParams.storyUUID : "";
  const selectedItemUUID = typeof rawParams.itemUUID === "string" ? rawParams.itemUUID : "";

  const [searchInput, setSearchInput] = useState(filters.query);
  const [desktopFeedWidthPct, setDesktopFeedWidthPctState] = useState(() =>
    getDesktopFeedWidthPct(),
  );
  const [translatingStoryUUIDs, setTranslatingStoryUUIDs] = useState<string[]>([]);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [settingsError, setSettingsError] = useState("");
  const [isSavingSettings, setIsSavingSettings] = useState(false);
  const [activeReaderStoryUUID, setActiveReaderStoryUUID] = useState(selectedStoryUUID);
  const [readerScrollTargetStoryUUID, setReaderScrollTargetStoryUUID] = useState(selectedStoryUUID);
  const [readerScrollTargetRevision, setReaderScrollTargetRevision] = useState(0);
  const [isDesktopLayout, setIsDesktopLayout] = useState(() => {
    if (typeof window === "undefined") {
      return true;
    }
    return window.matchMedia("(min-width: 1021px)").matches;
  });
  useEffect(() => {
    setSearchInput(filters.query);
  }, [filters.query]);

  useEffect(() => {
    if (!selectedStoryUUID) {
      return;
    }
    setActiveReaderStoryUUID(selectedStoryUUID);
    setReaderScrollTargetStoryUUID(selectedStoryUUID);
  }, [selectedStoryUUID]);

  useEffect(() => {
    if (!showAdvancedSearch && (viewerSearch.from || viewerSearch.to || viewerSearch.tag)) {
      const fallbackDay = viewerSearch.day || viewerSearch.from || viewerSearch.to;
      applySearch({
        ...viewerSearch,
        day: fallbackDay || undefined,
        from: undefined,
        to: undefined,
        tag: undefined,
        page: undefined,
      });
    }
  }, [showAdvancedSearch, viewerSearch]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    const mediaQuery = window.matchMedia("(min-width: 1021px)");
    const updateLayout = (): void => {
      setIsDesktopLayout(mediaQuery.matches);
    };

    updateLayout();
    mediaQuery.addEventListener("change", updateLayout);
    return () => mediaQuery.removeEventListener("change", updateLayout);
  }, []);

  const feedWidthBounds = useMemo(() => getDesktopFeedWidthBounds(), []);
  const preferredLanguage = useMemo(() => {
    const value = normalizeLanguageTag(settings?.preferred_language || "");
    return value || "en";
  }, [settings?.preferred_language]);
  const languageOptions = useMemo(() => {
    if (languages.length > 0) {
      return languages;
    }
    return fallbackLanguageOptions;
  }, [fallbackLanguageOptions, languages]);
  const language = preferredLanguage;
  const apiLanguage = useMemo(() => (language === "original" ? "" : language), [language]);
  const effectiveFilters = useMemo(
    () => ({
      ...filters,
      lang: apiLanguage,
    }),
    [filters, apiLanguage],
  );
  const feedPanelSize = useMemo(
    () => `${feedWidthBounds.defaultValue}%`,
    [feedWidthBounds.defaultValue],
  );
  const feedPanelMin = useMemo(() => `${feedWidthBounds.min}%`, [feedWidthBounds.min]);
  const feedPanelMax = useMemo(() => `${feedWidthBounds.max}%`, [feedWidthBounds.max]);
  const desktopLayout = useMemo(
    () => ({
      storyFeed: desktopFeedWidthPct,
      storyDetail: 100 - desktopFeedWidthPct,
    }),
    [desktopFeedWidthPct],
  );

  function onLayoutChanged(layout: Record<string, number>): void {
    if (!isDesktopLayout) {
      return;
    }

    const nextWidth = layout.storyFeed;
    if (typeof nextWidth !== "number" || !Number.isFinite(nextWidth)) {
      return;
    }

    setDesktopFeedWidthPct(nextWidth);
    setDesktopFeedWidthPctState(getDesktopFeedWidthPct());
  }

  const {
    collections,
    tags,
    dayBuckets,
    stories,
    pagination,
    globalError,
    storiesError,
    isStoriesPending,
    isFetchingNextStoriesPage,
    hasNextStoriesPage,
    fetchNextStoriesPage,
  } = useViewerQueries({
    filters: effectiveFilters,
  });

  const { dayNav, selectedDay } = useDayNavigationState({
    dayBuckets,
    day: viewerSearch.day || "",
    from: effectiveFilters.from,
    to: effectiveFilters.to,
  });
  const readerStateKey = useMemo(
    () =>
      [
        "scoop-reader-state-v1",
        effectiveFilters.collection,
        effectiveFilters.query,
        effectiveFilters.from,
        effectiveFilters.to,
        effectiveFilters.tag,
        effectiveFilters.lang,
        selectedDay || viewerSearch.day || "",
      ].join("|"),
    [
      effectiveFilters.collection,
      effectiveFilters.from,
      effectiveFilters.lang,
      effectiveFilters.query,
      effectiveFilters.tag,
      effectiveFilters.to,
      selectedDay,
      viewerSearch.day,
    ],
  );

  const allStoriesCount = useMemo(
    () => collections.reduce((acc, row) => acc + Number(row.stories || 0), 0),
    [collections],
  );
  const allCollectionsLabel = useMemo(
    () => `All collections (${formatCount(allStoriesCount || pagination.total_items)})`,
    [allStoriesCount, pagination.total_items],
  );

  function compactSearchForCurrentPath(nextSearch: ViewerSearch): ViewerSearch {
    const keepDateFilters = showAdvancedSearch;

    return compactViewerSearch({
      ...nextSearch,
      collection: routeCollection ? undefined : nextSearch.collection,
      day: keepDateFilters ? undefined : nextSearch.day,
      from: keepDateFilters ? nextSearch.from : undefined,
      to: keepDateFilters ? nextSearch.to : undefined,
    });
  }

  function applySearch(nextSearch: ViewerSearch): void {
    void navigate({
      to: ".",
      search: compactSearchForCurrentPath(nextSearch),
      replace: true,
    });
  }

  function navigateToStoryPath(collection: string, storyUUID: string, itemUUID?: string): void {
    const currentSearch = compactSearchForCurrentPath(viewerSearch);

    if (collection) {
      if (itemUUID) {
        void navigate({
          to: "/c/$collection/s/$storyUUID/i/$itemUUID",
          params: { collection, storyUUID, itemUUID },
          search: currentSearch,
          replace: false,
        });
        return;
      }

      void navigate({
        to: "/c/$collection/s/$storyUUID",
        params: { collection, storyUUID },
        search: currentSearch,
        replace: false,
      });
      return;
    }

    void navigate({
      to: "/stories/$storyUUID",
      params: { storyUUID },
      search: currentSearch,
      replace: false,
    });
  }

  function goToStory(storyUUID: string): void {
    const story = stories.find((row) => row.story_uuid === storyUUID);
    const collection = (story?.collection || routeCollection || filters.collection || "").trim();
    setActiveReaderStoryUUID(storyUUID);
    setReaderScrollTargetStoryUUID(storyUUID);
    setReaderScrollTargetRevision((previous) => previous + 1);
    navigateToStoryPath(collection, storyUUID);
  }

  function goToItem(storyUUID: string, itemUUID: string, collectionHint?: string): void {
    if (!storyUUID || !itemUUID) {
      return;
    }

    const story = stories.find((row) => row.story_uuid === storyUUID);
    const collection = (
      story?.collection ||
      collectionHint ||
      routeCollection ||
      filters.collection ||
      ""
    ).trim();
    navigateToStoryPath(collection, storyUUID, itemUUID);
  }

  function clearSelectedItem(storyUUID: string, collectionHint?: string): void {
    if (!storyUUID) {
      return;
    }

    const story = stories.find((row) => row.story_uuid === storyUUID);
    const collection = (
      story?.collection ||
      collectionHint ||
      routeCollection ||
      filters.collection ||
      ""
    ).trim();
    navigateToStoryPath(collection, storyUUID);
  }

  useEffect(() => {
    const handle = window.setTimeout(() => {
      const trimmed = searchInput.trim();
      const current = viewerSearch.q || "";
      if (trimmed === current) {
        return;
      }

      applySearch({
        ...viewerSearch,
        q: trimmed || undefined,
        page: undefined,
      });
    }, 220);

    return () => {
      window.clearTimeout(handle);
    };
  }, [searchInput, viewerSearch]);

  function setSingleDayFilter(day: string): void {
    if (!day) {
      return;
    }

    applySearch({
      ...viewerSearch,
      day,
      from: undefined,
      to: undefined,
      page: undefined,
    });
  }

  function moveDay(offset: number): void {
    if (dayNav.currentIndex < 0) {
      return;
    }

    const nextIndex = dayNav.currentIndex + offset;
    if (nextIndex < 0 || nextIndex >= dayBuckets.length) {
      return;
    }

    const nextDay = dayBuckets[nextIndex]?.day;
    if (!nextDay) {
      return;
    }

    setSingleDayFilter(nextDay);
  }

  function onCollectionChange(collection: string): void {
    const nextSearch = compactSearchForCurrentPath({
      ...viewerSearch,
      collection: undefined,
      page: undefined,
    });

    if (collection) {
      void navigate({
        to: "/c/$collection",
        params: { collection },
        search: nextSearch,
        replace: false,
      });
      return;
    }

    void navigate({
      to: "/",
      search: nextSearch,
      replace: false,
    });
  }

  function onFromChange(value: string): void {
    applySearch({
      ...viewerSearch,
      day: undefined,
      from: value || undefined,
      page: undefined,
    });
  }

  function onToChange(value: string): void {
    applySearch({
      ...viewerSearch,
      day: undefined,
      to: value || undefined,
      page: undefined,
    });
  }

  function onTagChange(value: string): void {
    applySearch({
      ...viewerSearch,
      tag: value || undefined,
      page: undefined,
    });
  }

  function onShowAdvancedSearchChange(value: boolean): void {
    setShowAdvancedSearch(value);
    if (!value && (viewerSearch.from || viewerSearch.to || viewerSearch.tag)) {
      const fallbackDay = viewerSearch.day || viewerSearch.from || viewerSearch.to;
      applySearch({
        ...viewerSearch,
        day: fallbackDay || undefined,
        from: undefined,
        to: undefined,
        tag: undefined,
        page: undefined,
      });
      return;
    }

    if (value && !viewerSearch.from && !viewerSearch.to && viewerSearch.day) {
      applySearch({
        ...viewerSearch,
        day: undefined,
        from: viewerSearch.day,
        to: viewerSearch.day,
        page: undefined,
      });
    }
  }

  const onTranslationStateChange = useCallback(
    (storyUUID: string, isTranslating: boolean): void => {
      if (!storyUUID) {
        return;
      }

      setTranslatingStoryUUIDs((previous) => {
        if (isTranslating) {
          if (previous.includes(storyUUID)) {
            return previous;
          }
          return [...previous, storyUUID];
        }

        if (!previous.includes(storyUUID)) {
          return previous;
        }

        return previous.filter((uuid) => uuid !== storyUUID);
      });
    },
    [],
  );
  const onReaderScrollTargetSettled = useCallback((storyUUID: string) => {
    setReaderScrollTargetStoryUUID((previous) => (previous === storyUUID ? "" : previous));
  }, []);

  const currentCollectionLabel = useCurrentCollectionLabel(collections, filters.collection);

  useEffect(() => {
    if (typeof document === "undefined") {
      return;
    }
    document.title = `Scoop • ${currentCollectionLabel}`;
  }, [currentCollectionLabel]);

  const pickerDay = selectedDay || dayNav.navigatorDay;
  const headerLeft = (
    <CollectionDropdown
      selectedCollection={filters.collection}
      allCollectionsValue={allCollectionsValue}
      allCollectionsLabel={allCollectionsLabel}
      currentCollectionLabel={currentCollectionLabel}
      collections={collections}
      onCollectionChange={onCollectionChange}
    />
  );

  const headerRight = (
    <DayNavigator
      dayNav={dayNav}
      pickerDay={pickerDay}
      onMoveOlder={() => moveDay(1)}
      onMoveNewer={() => moveDay(-1)}
      onSelectDay={setSingleDayFilter}
    />
  );

  return (
    <PageShell variant="viewer" headerLeft={headerLeft} headerRight={headerRight}>
      {globalError ? <p className="banner-error">{globalError}</p> : null}

      <main className="layout">
        <Group
          key={isDesktopLayout ? "desktop-layout" : "mobile-layout"}
          orientation={isDesktopLayout ? "horizontal" : "vertical"}
          className="layout-panels"
          defaultLayout={isDesktopLayout ? desktopLayout : undefined}
          onLayoutChanged={onLayoutChanged}
        >
          <Panel
            id="storyFeed"
            defaultSize={isDesktopLayout ? feedPanelSize : "45%"}
            minSize={isDesktopLayout ? feedPanelMin : "30%"}
            maxSize={isDesktopLayout ? feedPanelMax : "70%"}
          >
            <StoriesListPanel
              searchInput={searchInput}
              from={effectiveFilters.from}
              to={effectiveFilters.to}
              selectedTag={effectiveFilters.tag}
              availableTags={tags}
              activeLang={apiLanguage}
              translatingStoryUUIDs={translatingStoryUUIDs}
              totalItems={pagination.total_items}
              loadedItems={stories.length}
              selectedStoryUUID={activeReaderStoryUUID || selectedStoryUUID}
              stories={stories}
              isLoading={isStoriesPending}
              isFetchingNextPage={isFetchingNextStoriesPage}
              hasNextPage={hasNextStoriesPage}
              error={storiesError}
              showAdvancedSearch={showAdvancedSearch}
              onSearchInputChange={setSearchInput}
              onShowAdvancedSearchChange={onShowAdvancedSearchChange}
              onFromChange={onFromChange}
              onToChange={onToChange}
              onTagChange={onTagChange}
              onLoadNextPage={fetchNextStoriesPage}
              onSelectStory={goToStory}
              currentUsername={user?.username || "User"}
              onOpenSettings={() => {
                setSettingsError("");
                setIsSettingsOpen(true);
              }}
              onLogout={() => {
                void logout();
              }}
            />
          </Panel>

          <Separator
            className={`layout-resize-handle ${isDesktopLayout ? "horizontal" : "vertical"}`.trim()}
          />

          <Panel id="storyDetail" minSize={isDesktopLayout ? "20%" : "30%"}>
            <StoryReaderPanel
              selectedStoryUUID={selectedStoryUUID}
              selectedItemUUID={selectedItemUUID}
              scrollTargetStoryUUID={readerScrollTargetStoryUUID}
              scrollTargetRevision={readerScrollTargetRevision}
              stories={stories}
              availableTags={tags}
              activeLang={apiLanguage}
              isLoadingStories={isStoriesPending}
              storiesError={storiesError}
              hasNextStoryPage={hasNextStoriesPage}
              isFetchingNextStoryPage={isFetchingNextStoriesPage}
              readerStateKey={readerStateKey}
              onLoadNextStoryPage={fetchNextStoriesPage}
              onActiveStoryChange={setActiveReaderStoryUUID}
              onSelectItem={goToItem}
              onClearSelectedItem={clearSelectedItem}
              onTranslationStateChange={onTranslationStateChange}
              onScrollTargetSettled={onReaderScrollTargetSettled}
            />
          </Panel>
        </Group>
      </main>

      <SettingsModal
        open={isSettingsOpen}
        preferredLanguage={language}
        languageOptions={languageOptions}
        isSaving={isSavingSettings}
        error={settingsError}
        onClose={() => {
          setIsSettingsOpen(false);
        }}
        onSave={async (nextLanguage) => {
          setIsSavingSettings(true);
          setSettingsError("");
          try {
            await updateSettings({ preferred_language: nextLanguage });
            setIsSettingsOpen(false);
          } catch (err) {
            const message =
              err instanceof Error ? err.message : "Failed to update translation language";
            setSettingsError(message);
          } finally {
            setIsSavingSettings(false);
          }
        }}
      />
    </PageShell>
  );
}

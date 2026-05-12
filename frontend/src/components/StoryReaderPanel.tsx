import { useQueries, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState, type RefCallback } from "react";

import { addArticleTag, getStoryDetail, removeArticleTag, requestTranslation } from "../api";
import { useStoryArticlePreviews } from "../hooks/useStoryArticlePreviews";
import {
  defaultCollectionTranslationMode,
  isCollectionTranslationEnabled,
} from "../lib/collectionTranslation";
import type { StoryDetailResponse, StoryListItem, Tag } from "../types";
import { ArticleTagEditor } from "./story-detail/ArticleTagEditor";
import {
  buildMemberGroups,
  StoryArticleGroup,
  type MemberURLGroup,
} from "./story-detail/StoryArticleGroup";
import { StoryTitleCopyButton, TitleActions, TitleSourceLink } from "./story-detail/TitleActions";

const initialReaderStoryCount = 3;
const readerPageSize = 3;
const readerStateMaxAgeMs = 1000 * 60 * 30;

interface StoryReaderPanelProps {
  selectedStoryUUID: string;
  selectedItemUUID: string;
  scrollTargetStoryUUID: string;
  scrollTargetRevision: number;
  stories: StoryListItem[];
  availableTags: Tag[];
  activeLang: string;
  isLoadingStories: boolean;
  storiesError: string;
  hasNextStoryPage: boolean;
  isFetchingNextStoryPage: boolean;
  readerStateKey: string;
  onLoadNextStoryPage: () => void;
  onActiveStoryChange: (storyUUID: string) => void;
  onSelectItem: (storyUUID: string, itemUUID: string, collection?: string) => void;
  onClearSelectedItem: (storyUUID: string, collection?: string) => void;
  onTranslationStateChange?: (storyUUID: string, isTranslating: boolean) => void;
  onScrollTargetSettled?: (storyUUID: string) => void;
}

interface StoredReaderState {
  activeStoryUUID?: string;
  scrollTop?: number;
  visibleCount?: number;
  ts?: number;
}

function readStoredReaderState(key: string): StoredReaderState | null {
  if (typeof window === "undefined" || !key) {
    return null;
  }

  try {
    const raw = window.sessionStorage.getItem(key);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as StoredReaderState | null;
    if (!parsed || typeof parsed !== "object") {
      return null;
    }
    const ageMs = Date.now() - Number(parsed.ts || 0);
    if (!Number.isFinite(ageMs) || ageMs > readerStateMaxAgeMs) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

function clearStoredReaderState(key: string): void {
  if (typeof window === "undefined" || !key) {
    return;
  }
  try {
    window.sessionStorage.removeItem(key);
  } catch {
    // Ignore storage failures.
  }
}

function writeStoredReaderState(key: string, state: StoredReaderState): void {
  if (typeof window === "undefined" || !key) {
    return;
  }
  try {
    window.sessionStorage.setItem(key, JSON.stringify({ ...state, ts: Date.now() }));
  } catch {
    // Ignore storage failures.
  }
}

function storyDetailQueryKey(storyUUID: string, language: string): [string, string, string] {
  return ["story-detail", storyUUID, language];
}

function buildReaderStoryUUIDs(stories: StoryListItem[], syntheticStoryUUID: string): string[] {
  const result: string[] = [];
  const seen = new Set<string>();

  if (syntheticStoryUUID) {
    seen.add(syntheticStoryUUID);
    result.push(syntheticStoryUUID);
  }

  for (const story of stories) {
    if (!story.story_uuid || seen.has(story.story_uuid)) {
      continue;
    }
    seen.add(story.story_uuid);
    result.push(story.story_uuid);
  }

  return result;
}

export function StoryReaderPanel({
  selectedStoryUUID,
  selectedItemUUID,
  scrollTargetStoryUUID,
  scrollTargetRevision,
  stories,
  availableTags,
  activeLang,
  isLoadingStories,
  storiesError,
  hasNextStoryPage,
  isFetchingNextStoryPage,
  readerStateKey,
  onLoadNextStoryPage,
  onActiveStoryChange,
  onSelectItem,
  onClearSelectedItem,
  onTranslationStateChange,
  onScrollTargetSettled,
}: StoryReaderPanelProps): JSX.Element {
  const contentRef = useRef<HTMLDivElement | null>(null);
  const loadMoreRef = useRef<HTMLDivElement | null>(null);
  const sectionRefs = useRef<Record<string, HTMLElement | null>>({});
  const activeStoryUUIDRef = useRef("");
  const [activeStoryUUID, setActiveStoryUUIDState] = useState("");
  const [pinnedStoryUUID, setPinnedStoryUUID] = useState("");
  const visibleCountRef = useRef(initialReaderStoryCount);
  const restoreScrollTopRef = useRef<number | null>(null);
  const restoreActiveStoryUUIDRef = useRef("");
  const restoreVisibleCountRef = useRef(0);
  const restoredStateKeyRef = useRef("");
  const handledScrollTargetRevisionRef = useRef(0);
  const handledScrollIntoViewRevisionRef = useRef(0);
  const programmaticScrollTimerRef = useRef<number | null>(null);
  const scrollFrameRef = useRef<number | null>(null);

  const selectedStoryInLoadedStories = useMemo(
    () => stories.some((story) => story.story_uuid === selectedStoryUUID),
    [selectedStoryUUID, stories],
  );
  const syntheticStoryUUID =
    pinnedStoryUUID ||
    (selectedStoryUUID && !selectedStoryInLoadedStories ? selectedStoryUUID : "");
  const storyUUIDs = useMemo(
    () => buildReaderStoryUUIDs(stories, syntheticStoryUUID),
    [syntheticStoryUUID, stories],
  );
  const selectedStoryIndex = selectedStoryUUID ? storyUUIDs.indexOf(selectedStoryUUID) : -1;
  const [visibleCount, setVisibleCount] = useState(() =>
    Math.max(initialReaderStoryCount, selectedStoryIndex + 1),
  );
  const visibleStoryUUIDs = useMemo(
    () => storyUUIDs.slice(0, Math.min(visibleCount, storyUUIDs.length)),
    [storyUUIDs, visibleCount],
  );

  visibleCountRef.current = visibleCount;

  const detailQueries = useQueries({
    queries: visibleStoryUUIDs.map((storyUUID) => ({
      queryKey: storyDetailQueryKey(storyUUID, activeLang),
      queryFn: () => getStoryDetail(storyUUID, activeLang),
      enabled: storyUUID !== "",
      staleTime: 15_000,
    })),
  });

  const detailByStoryUUID = useMemo(() => {
    const result: Record<string, StoryDetailResponse | null> = {};
    visibleStoryUUIDs.forEach((storyUUID, index) => {
      result[storyUUID] = (detailQueries[index]?.data as StoryDetailResponse | undefined) ?? null;
    });
    return result;
  }, [detailQueries, visibleStoryUUIDs]);

  const setSectionRef = useCallback(
    (storyUUID: string): RefCallback<HTMLElement> =>
      (node) => {
        sectionRefs.current[storyUUID] = node;
      },
    [],
  );

  const setActiveStoryUUID = useCallback(
    (storyUUID: string) => {
      if (!storyUUID || activeStoryUUIDRef.current === storyUUID) {
        return;
      }
      activeStoryUUIDRef.current = storyUUID;
      setActiveStoryUUIDState(storyUUID);
      onActiveStoryChange(storyUUID);
    },
    [onActiveStoryChange],
  );

  useEffect(() => {
    activeStoryUUIDRef.current = "";
    setActiveStoryUUIDState("");
    setPinnedStoryUUID("");
    setVisibleCount(initialReaderStoryCount);
    contentRef.current?.scrollTo({ top: 0, left: 0, behavior: "auto" });
  }, [readerStateKey]);

  useEffect(() => {
    if (!selectedStoryUUID) {
      setPinnedStoryUUID("");
      return;
    }

    setPinnedStoryUUID((previous) => {
      if (selectedStoryInLoadedStories) {
        return "";
      }

      if (previous === selectedStoryUUID) {
        return previous;
      }

      return selectedStoryUUID;
    });
  }, [selectedStoryInLoadedStories, selectedStoryUUID]);

  useEffect(() => {
    if (
      scrollTargetRevision <= 0 ||
      handledScrollTargetRevisionRef.current === scrollTargetRevision
    ) {
      return;
    }
    handledScrollTargetRevisionRef.current = scrollTargetRevision;

    if (!scrollTargetStoryUUID || scrollTargetStoryUUID !== selectedStoryUUID) {
      return;
    }

    if (selectedStoryInLoadedStories) {
      setPinnedStoryUUID("");
    }
  }, [
    scrollTargetRevision,
    scrollTargetStoryUUID,
    selectedStoryInLoadedStories,
    selectedStoryUUID,
  ]);

  const computeActiveStory = useCallback(() => {
    const root = contentRef.current;
    if (!root) {
      return;
    }

    const rootRect = root.getBoundingClientRect();
    const centerY = rootRect.top + rootRect.height * 0.38;
    let nextStoryUUID = "";
    let bestDistance = Number.POSITIVE_INFINITY;

    for (const storyUUID of visibleStoryUUIDs) {
      const section = sectionRefs.current[storyUUID];
      if (!section) {
        continue;
      }

      const rect = section.getBoundingClientRect();
      if (rect.bottom < rootRect.top + 12 || rect.top > rootRect.bottom - 12) {
        continue;
      }

      const containsCenter = rect.top <= centerY && rect.bottom >= centerY;
      const distance = containsCenter ? 0 : Math.abs(rect.top - centerY);
      if (distance < bestDistance) {
        bestDistance = distance;
        nextStoryUUID = storyUUID;
      }
    }

    if (nextStoryUUID) {
      setActiveStoryUUID(nextStoryUUID);
    }
  }, [setActiveStoryUUID, visibleStoryUUIDs]);

  const scheduleActiveStoryComputation = useCallback(() => {
    if (scrollFrameRef.current !== null) {
      return;
    }
    scrollFrameRef.current = window.requestAnimationFrame(() => {
      scrollFrameRef.current = null;
      computeActiveStory();
    });
  }, [computeActiveStory]);

  useEffect(() => {
    const root = contentRef.current;
    if (!root) {
      return;
    }

    root.addEventListener("scroll", scheduleActiveStoryComputation, { passive: true });
    window.addEventListener("resize", scheduleActiveStoryComputation, { passive: true });
    scheduleActiveStoryComputation();

    return () => {
      root.removeEventListener("scroll", scheduleActiveStoryComputation);
      window.removeEventListener("resize", scheduleActiveStoryComputation);
      if (scrollFrameRef.current !== null) {
        window.cancelAnimationFrame(scrollFrameRef.current);
        scrollFrameRef.current = null;
      }
    };
  }, [scheduleActiveStoryComputation, visibleStoryUUIDs.length]);

  useEffect(() => {
    const root = contentRef.current;
    const sentinel = loadMoreRef.current;
    if (!root || !sentinel) {
      return;
    }

    const loadNextReaderStories = (): void => {
      setVisibleCount((previous) => {
        if (previous < storyUUIDs.length) {
          return Math.min(storyUUIDs.length, previous + readerPageSize);
        }
        return previous;
      });

      if (
        visibleCountRef.current >= storyUUIDs.length - readerPageSize &&
        hasNextStoryPage &&
        !isFetchingNextStoryPage
      ) {
        onLoadNextStoryPage();
      }
    };

    if (typeof IntersectionObserver === "undefined") {
      const handleScroll = (): void => {
        const distanceToEnd = root.scrollHeight - root.scrollTop - root.clientHeight;
        if (distanceToEnd < 560) {
          loadNextReaderStories();
        }
      };
      root.addEventListener("scroll", handleScroll, { passive: true });
      handleScroll();
      return () => root.removeEventListener("scroll", handleScroll);
    }

    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) {
          return;
        }

        loadNextReaderStories();
      },
      {
        root,
        rootMargin: "560px 0px",
        threshold: 0,
      },
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [
    hasNextStoryPage,
    isFetchingNextStoryPage,
    onLoadNextStoryPage,
    storyUUIDs.length,
    visibleStoryUUIDs.length,
  ]);

  useEffect(() => {
    if (!readerStateKey || restoredStateKeyRef.current === readerStateKey) {
      return;
    }

    restoredStateKeyRef.current = readerStateKey;
    const stored = readStoredReaderState(readerStateKey);
    if (!stored) {
      return;
    }

    const restoredStoryIndex = stored.activeStoryUUID
      ? storyUUIDs.indexOf(stored.activeStoryUUID)
      : -1;
    setVisibleCount((previous) =>
      Math.max(
        previous,
        Number.isFinite(Number(stored.visibleCount)) ? Number(stored.visibleCount) : 0,
        restoredStoryIndex + 1,
        initialReaderStoryCount,
      ),
    );
    if (Number.isFinite(Number(stored.scrollTop))) {
      restoreScrollTopRef.current = Math.max(0, Number(stored.scrollTop));
      restoreActiveStoryUUIDRef.current = (stored.activeStoryUUID || "").trim();
      restoreVisibleCountRef.current = Math.max(0, Number(stored.visibleCount || 0));
    }
    clearStoredReaderState(readerStateKey);
  }, [readerStateKey, storyUUIDs]);

  useEffect(() => {
    if (restoreScrollTopRef.current === null || visibleStoryUUIDs.length === 0) {
      return;
    }

    const targetStoryUUID = restoreActiveStoryUUIDRef.current;
    const targetStoryIsLoaded =
      !targetStoryUUID ||
      visibleStoryUUIDs.includes(targetStoryUUID) ||
      (!hasNextStoryPage && !storyUUIDs.includes(targetStoryUUID));
    const targetVisibleCount = restoreVisibleCountRef.current;
    const enoughStoriesAreRendered =
      targetVisibleCount <= 0 ||
      visibleStoryUUIDs.length >= Math.min(targetVisibleCount, storyUUIDs.length);

    if (!targetStoryIsLoaded || !enoughStoriesAreRendered) {
      if (hasNextStoryPage && !isFetchingNextStoryPage) {
        onLoadNextStoryPage();
      }
      return;
    }

    const scrollTop = restoreScrollTopRef.current;
    const restore = (): void => {
      if (contentRef.current) {
        contentRef.current.scrollTo({ top: scrollTop, left: 0, behavior: "auto" });
      }
      computeActiveStory();
    };

    window.requestAnimationFrame(restore);
    const timeout = window.setTimeout(restore, 120);
    restoreScrollTopRef.current = null;
    restoreActiveStoryUUIDRef.current = "";
    restoreVisibleCountRef.current = 0;
    return () => window.clearTimeout(timeout);
  }, [
    computeActiveStory,
    hasNextStoryPage,
    isFetchingNextStoryPage,
    onLoadNextStoryPage,
    storyUUIDs,
    visibleStoryUUIDs,
  ]);

  useEffect(() => {
    if (!selectedStoryUUID) {
      return;
    }

    const nextIndex = storyUUIDs.indexOf(selectedStoryUUID);
    if (nextIndex < 0) {
      return;
    }

    setVisibleCount((previous) => Math.max(previous, nextIndex + 1, initialReaderStoryCount));
  }, [selectedStoryUUID, storyUUIDs]);

  useEffect(() => {
    if (!scrollTargetStoryUUID) {
      return;
    }
    if (
      scrollTargetRevision > 0 &&
      handledScrollIntoViewRevisionRef.current === scrollTargetRevision
    ) {
      return;
    }

    const targetIndex = storyUUIDs.indexOf(scrollTargetStoryUUID);
    if (targetIndex >= 0) {
      setVisibleCount((previous) => Math.max(previous, targetIndex + 1));
    }

    const targetIsPinned = pinnedStoryUUID === scrollTargetStoryUUID;
    if (targetIsPinned && selectedStoryInLoadedStories) {
      return;
    }

    const cancelProgrammaticScroll = (): void => {
      if (programmaticScrollTimerRef.current !== null) {
        window.clearTimeout(programmaticScrollTimerRef.current);
        programmaticScrollTimerRef.current = null;
      }
      onScrollTargetSettled?.(scrollTargetStoryUUID);
    };

    const scrollToTarget = (): void => {
      const target = sectionRefs.current[scrollTargetStoryUUID];
      if (!target) {
        return;
      }

      if (scrollTargetRevision > 0) {
        handledScrollIntoViewRevisionRef.current = scrollTargetRevision;
      }
      target.scrollIntoView({ block: "start", inline: "nearest", behavior: "smooth" });
      setActiveStoryUUID(scrollTargetStoryUUID);
      if (targetIsPinned) {
        return;
      }
      if (programmaticScrollTimerRef.current !== null) {
        window.clearTimeout(programmaticScrollTimerRef.current);
      }
      programmaticScrollTimerRef.current = window.setTimeout(() => {
        onScrollTargetSettled?.(scrollTargetStoryUUID);
        programmaticScrollTimerRef.current = null;
      }, 900);
    };

    const frame = window.requestAnimationFrame(scrollToTarget);
    const timeout = window.setTimeout(scrollToTarget, 80);
    const root = contentRef.current;
    root?.addEventListener("wheel", cancelProgrammaticScroll, { passive: true, once: true });
    root?.addEventListener("touchstart", cancelProgrammaticScroll, { passive: true, once: true });
    root?.addEventListener("pointerdown", cancelProgrammaticScroll, { passive: true, once: true });
    return () => {
      window.cancelAnimationFrame(frame);
      window.clearTimeout(timeout);
      root?.removeEventListener("wheel", cancelProgrammaticScroll);
      root?.removeEventListener("touchstart", cancelProgrammaticScroll);
      root?.removeEventListener("pointerdown", cancelProgrammaticScroll);
    };
  }, [
    onScrollTargetSettled,
    pinnedStoryUUID,
    scrollTargetRevision,
    scrollTargetStoryUUID,
    selectedStoryInLoadedStories,
    setActiveStoryUUID,
    storyUUIDs,
    visibleStoryUUIDs.length,
  ]);

  useEffect(() => {
    if (!readerStateKey) {
      return;
    }

    const save = (): void => {
      writeStoredReaderState(readerStateKey, {
        activeStoryUUID: activeStoryUUIDRef.current,
        scrollTop: contentRef.current?.scrollTop ?? 0,
        visibleCount: visibleCountRef.current,
      });
    };

    window.addEventListener("beforeunload", save);
    return () => {
      save();
      window.removeEventListener("beforeunload", save);
    };
  }, [readerStateKey]);

  if (isLoadingStories && storyUUIDs.length === 0) {
    return (
      <aside className="panel card detail-panel">
        <div className="detail-content">
          <p className="muted">Fetching story reader...</p>
        </div>
      </aside>
    );
  }

  return (
    <aside className="panel card detail-panel">
      <div ref={contentRef} className="detail-content reader-content">
        {storiesError ? <p className="muted">{storiesError}</p> : null}
        {!storiesError && storyUUIDs.length === 0 ? (
          <p className="muted">No stories match this filter.</p>
        ) : null}

        {visibleStoryUUIDs.map((storyUUID, index) => {
          const detail = detailByStoryUUID[storyUUID];
          const query = detailQueries[index];
          const isActive = activeStoryUUID === storyUUID;

          return (
            <StoryReaderSection
              key={storyUUID}
              refCallback={setSectionRef(storyUUID)}
              storyUUID={storyUUID}
              selectedItemUUID={selectedItemUUID}
              detail={detail}
              availableTags={availableTags}
              activeLang={activeLang}
              isActive={isActive}
              isLoading={Boolean(query?.isPending)}
              error={query?.error instanceof Error ? query.error.message : ""}
              onSelectItem={onSelectItem}
              onClearSelectedItem={onClearSelectedItem}
              onTranslationStateChange={onTranslationStateChange}
            />
          );
        })}

        <div ref={loadMoreRef} className="reader-load-sentinel" aria-hidden="true" />
        {isFetchingNextStoryPage ? (
          <p className="muted stories-status">Loading more stories...</p>
        ) : null}
        {!hasNextStoryPage && visibleCount >= storyUUIDs.length && storyUUIDs.length > 0 ? (
          <p className="muted stories-status">Reached the end of this reader feed.</p>
        ) : null}
      </div>
    </aside>
  );
}

interface StoryReaderSectionProps {
  storyUUID: string;
  selectedItemUUID: string;
  detail: StoryDetailResponse | null;
  availableTags: Tag[];
  activeLang: string;
  isActive: boolean;
  isLoading: boolean;
  error: string;
  refCallback: RefCallback<HTMLElement>;
  onSelectItem: (storyUUID: string, itemUUID: string, collection?: string) => void;
  onClearSelectedItem: (storyUUID: string, collection?: string) => void;
  onTranslationStateChange?: (storyUUID: string, isTranslating: boolean) => void;
}

function StoryReaderSection({
  storyUUID,
  selectedItemUUID,
  detail,
  availableTags,
  activeLang,
  isActive,
  isLoading,
  error,
  refCallback,
  onSelectItem,
  onClearSelectedItem,
  onTranslationStateChange,
}: StoryReaderSectionProps): JSX.Element {
  const queryClient = useQueryClient();
  const [expandedGroupKeys, setExpandedGroupKeys] = useState<string[]>([]);
  const [detailTextMode, setDetailTextMode] = useState<"translated" | "original">(
    activeLang ? "translated" : "original",
  );
  const [isTranslating, setIsTranslating] = useState(false);
  const [translationError, setTranslationError] = useState("");
  const [tagMutationKey, setTagMutationKey] = useState("");
  const [tagMutationError, setTagMutationError] = useState("");
  const translationRequestedRef = useRef("");
  const activeTranslationKeyRef = useRef("");
  const { itemPreviewByUUID, itemPreviewLoadingByUUID, itemPreviewErrorByUUID } =
    useStoryArticlePreviews(detail);

  const sectionActiveLang = useMemo(() => {
    if (!detail) {
      return "";
    }
    const mode =
      detail.story.translation_mode ?? defaultCollectionTranslationMode(detail.story.collection);
    return isCollectionTranslationEnabled(mode) ? activeLang : "";
  }, [activeLang, detail]);

  const hasPendingTranslations = useMemo(() => {
    if (!sectionActiveLang || !detail) {
      return false;
    }

    const translatedTitle = (detail.story.translated_title || "").trim();
    const hasUntranslatedBody = detail.members.some((member) => {
      const mode = member.translation_mode ?? defaultCollectionTranslationMode(member.collection);
      return isCollectionTranslationEnabled(mode) && !(member.translated_text || "").trim();
    });
    return translatedTitle === "" || hasUntranslatedBody;
  }, [detail, sectionActiveLang]);

  const memberGroups = useMemo<MemberURLGroup[]>(() => {
    return buildMemberGroups(detail);
  }, [detail]);

  const groupKeyByItemUUID = useMemo<Record<string, string>>(() => {
    const mapping: Record<string, string> = {};
    for (const group of memberGroups) {
      for (const member of group.members) {
        mapping[member.story_article_uuid] = group.key;
      }
    }
    return mapping;
  }, [memberGroups]);

  const selectedGroupKey = selectedItemUUID ? (groupKeyByItemUUID[selectedItemUUID] ?? "") : "";
  const showTranslationProgress =
    sectionActiveLang !== "" &&
    isTranslating &&
    activeTranslationKeyRef.current === `${storyUUID}:${sectionActiveLang}`;

  useEffect(() => {
    if (!detail) {
      setExpandedGroupKeys([]);
      return;
    }
    setExpandedGroupKeys(memberGroups.map((group) => group.key));
  }, [detail, memberGroups]);

  useEffect(() => {
    setDetailTextMode(sectionActiveLang ? "translated" : "original");
  }, [sectionActiveLang]);

  useEffect(() => {
    setTranslationError("");
  }, [storyUUID, sectionActiveLang]);

  useEffect(() => {
    if (!isActive || !sectionActiveLang || !detail || !hasPendingTranslations) {
      return;
    }

    const reqKey = `${storyUUID}:${sectionActiveLang}`;
    if (translationRequestedRef.current === reqKey) {
      return;
    }
    translationRequestedRef.current = reqKey;
    activeTranslationKeyRef.current = reqKey;
    setTranslationError("");
    setIsTranslating(true);
    onTranslationStateChange?.(storyUUID, true);

    void requestTranslation(storyUUID, sectionActiveLang)
      .then(() =>
        Promise.all([
          queryClient.invalidateQueries({
            queryKey: storyDetailQueryKey(storyUUID, sectionActiveLang),
            exact: true,
          }),
          queryClient.invalidateQueries({ queryKey: ["stories"] }),
        ]).then(() =>
          Promise.all([
            queryClient.refetchQueries({
              queryKey: storyDetailQueryKey(storyUUID, sectionActiveLang),
              exact: true,
              type: "active",
            }),
            queryClient.refetchQueries({ queryKey: ["stories"], type: "active" }),
          ]),
        ),
      )
      .catch((err) => {
        translationRequestedRef.current = "";
        setTranslationError(err instanceof Error ? err.message : "Failed to translate story.");
      })
      .finally(() => {
        if (activeTranslationKeyRef.current === reqKey) {
          activeTranslationKeyRef.current = "";
          setIsTranslating(false);
        }
        onTranslationStateChange?.(storyUUID, false);
      });
  }, [
    detail,
    hasPendingTranslations,
    isActive,
    onTranslationStateChange,
    queryClient,
    sectionActiveLang,
    storyUUID,
  ]);

  async function refreshTagsAfterMutation(): Promise<void> {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["story-detail", storyUUID] }),
      queryClient.invalidateQueries({ queryKey: ["stories"] }),
    ]);
  }

  async function onAddArticleTag(articleUUID: string, tagSlug: string): Promise<void> {
    if (!articleUUID || !tagSlug || tagSlug === "__add_tag__") {
      return;
    }

    const mutationKey = `${articleUUID}:${tagSlug}:add`;
    setTagMutationKey(mutationKey);
    setTagMutationError("");
    try {
      await addArticleTag(articleUUID, tagSlug);
      await refreshTagsAfterMutation();
    } catch (err) {
      setTagMutationError(err instanceof Error ? err.message : "Failed to add tag.");
      throw err;
    } finally {
      setTagMutationKey("");
    }
  }

  async function onRemoveArticleTag(articleUUID: string, tagSlug: string): Promise<void> {
    if (!articleUUID || !tagSlug) {
      return;
    }

    const mutationKey = `${articleUUID}:${tagSlug}:remove`;
    setTagMutationKey(mutationKey);
    setTagMutationError("");
    try {
      await removeArticleTag(articleUUID, tagSlug);
      await refreshTagsAfterMutation();
    } catch (err) {
      setTagMutationError(err instanceof Error ? err.message : "Failed to remove tag.");
    } finally {
      setTagMutationKey("");
    }
  }

  const originalTitle = (detail?.story.original_title || detail?.story.title || "").trim();
  const translatedTitle = (detail?.story.translated_title || "").trim();
  const showTranslatedTitle = sectionActiveLang !== "" && translatedTitle !== "";
  const displayTitle = showTranslatedTitle ? translatedTitle : originalTitle;
  const isMergedStory = detail ? detail.story.article_count > 1 || memberGroups.length > 1 : false;
  const titleLinkURL =
    detail && detail.story.article_count <= 1 && memberGroups.length === 1
      ? memberGroups[0].canonicalURL
      : "";
  const singleRepresentative =
    detail && detail.story.article_count <= 1 && memberGroups.length === 1
      ? memberGroups[0].representative
      : null;

  return (
    <section
      ref={refCallback}
      className={`reader-story-section ${isActive ? "is-active" : ""}`.trim()}
      data-story-uuid={storyUUID}
    >
      {isLoading && !detail ? <p className="muted">Fetching story detail...</p> : null}
      {!isLoading && error ? <p className="muted">{error}</p> : null}
      {detail ? (
        <>
          <div className="reader-story-header">
            <div className="detail-title-row">
              <TitleActions className="detail-title-cluster">
                <h2 className="detail-title">
                  <StoryTitleCopyButton
                    title={displayTitle}
                    collection={detail.story.collection}
                    storyUUID={detail.story.story_uuid}
                  />
                </h2>
                {titleLinkURL ? <TitleSourceLink url={titleLinkURL} /> : null}
                {singleRepresentative ? (
                  <ArticleTagEditor
                    articleUUID={singleRepresentative.article_uuid}
                    currentTags={singleRepresentative.tags ?? []}
                    availableTags={availableTags}
                    mutationKey={tagMutationKey}
                    variant="title"
                    onAddTag={onAddArticleTag}
                    onRemoveTag={onRemoveArticleTag}
                  />
                ) : null}
              </TitleActions>
            </div>
            {showTranslatedTitle ? (
              <p className="detail-title-original">Original: {originalTitle || "(untitled)"}</p>
            ) : null}
          </div>

          {sectionActiveLang ? (
            <div className="detail-text-mode-toggle" role="group" aria-label="Detail text mode">
              <button
                type="button"
                className={`detail-text-mode-btn ${detailTextMode === "translated" ? "active" : ""}`.trim()}
                onClick={() => setDetailTextMode("translated")}
              >
                Translated
              </button>
              <button
                type="button"
                className={`detail-text-mode-btn ${detailTextMode === "original" ? "active" : ""}`.trim()}
                onClick={() => setDetailTextMode("original")}
              >
                Original
              </button>
            </div>
          ) : null}
          {showTranslationProgress ? (
            <section
              className="translation-progress"
              role="status"
              aria-live="polite"
              aria-label="Translation in progress"
            >
              <div className="translation-progress-track" aria-hidden="true">
                <span className="translation-progress-bar" />
              </div>
              <p className="translation-progress-label">
                Translating to {sectionActiveLang.toUpperCase()}...
              </p>
            </section>
          ) : null}
          {translationError ? <p className="banner-error">{translationError}</p> : null}
          {tagMutationError ? <p className="banner-error">{tagMutationError}</p> : null}

          <section className="member-grid">
            {memberGroups.length === 0 ? (
              <p className="muted">No items found for this story.</p>
            ) : null}
            {memberGroups.map((group) => (
              <StoryArticleGroup
                key={group.key}
                group={group}
                selectedItemUUID={selectedItemUUID}
                selectedGroupKey={selectedGroupKey}
                expandedGroupKeys={expandedGroupKeys}
                isMergedStory={isMergedStory}
                detailTextMode={detailTextMode}
                activeLang={sectionActiveLang}
                availableTags={availableTags}
                tagMutationKey={tagMutationKey}
                itemPreviewByUUID={itemPreviewByUUID}
                itemPreviewLoadingByUUID={itemPreviewLoadingByUUID}
                itemPreviewErrorByUUID={itemPreviewErrorByUUID}
                showPrimaryTagEditor={isMergedStory}
                onExpandedGroupKeysChange={setExpandedGroupKeys}
                onSelectItem={(itemUUID) =>
                  onSelectItem(storyUUID, itemUUID, detail.story.collection)
                }
                onClearSelectedItem={() => onClearSelectedItem(storyUUID, detail.story.collection)}
                onAddArticleTag={onAddArticleTag}
                onRemoveArticleTag={onRemoveArticleTag}
              />
            ))}
          </section>
        </>
      ) : null}
    </section>
  );
}

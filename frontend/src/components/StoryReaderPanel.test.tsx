import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { getStoryDetail } from "../api";
import type { StoryDetailResponse, StoryListItem } from "../types";
import { StoryReaderPanel } from "./StoryReaderPanel";

vi.mock("../api", () => ({
  addArticleTag: vi.fn(async () => undefined),
  getStoryArticlePreview: vi.fn(async (storyMemberUUID: string) => ({
    story_article_uuid: storyMemberUUID,
    preview_text: "Preview text.",
    source: "normalized_text",
    char_count: 13,
    truncated: false,
  })),
  getStoryDetail: vi.fn(async (storyUUID: string) => makeDetail(storyUUID)),
  removeArticleTag: vi.fn(async () => undefined),
  requestTranslation: vi.fn(async () => ({
    stats: { total: 0, translated: 0, cached: 0, skipped: 0 },
  })),
}));

function makeStory(storyUUID: string, title = storyUUID): StoryListItem {
  return {
    story_id: Number.parseInt(storyUUID.replace(/\D/g, ""), 10) || 1,
    story_uuid: storyUUID,
    collection: "openclaw",
    translation_mode: "disabled",
    title,
    original_title: title,
    translated_title: null,
    detected_language: "en",
    status: "active",
    first_seen_at: "2026-05-12T08:00:00Z",
    last_seen_at: "2026-05-12T09:00:00Z",
    source_count: 1,
    article_count: 1,
  };
}

function makeDetail(storyUUID: string): StoryDetailResponse {
  const story = makeStory(storyUUID, storyUUID === "target-story" ? "Target story" : storyUUID);
  return {
    story,
    members: [
      {
        story_article_uuid: `${storyUUID}-member`,
        article_uuid: `${storyUUID}-article`,
        source: "discord_archive",
        source_item_id: storyUUID,
        collection: story.collection,
        translation_mode: "disabled",
        canonical_url: `https://example.com/${storyUUID}`,
        published_at: story.first_seen_at,
        normalized_title: story.title,
        normalized_text: `${story.title} body.`,
        detected_language: "en",
        original_title: story.title,
        translated_title: null,
        original_text: `${story.title} body.`,
        translated_text: null,
        matched_at: story.last_seen_at,
        match_type: "new_story",
      },
    ],
  };
}

function renderReader(stories: StoryListItem[], onScrollTargetSettled = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  const view = render(
    <QueryClientProvider client={queryClient}>
      <StoryReaderPanel
        selectedStoryUUID="target-story"
        selectedItemUUID=""
        scrollTargetStoryUUID="target-story"
        scrollTargetRevision={0}
        stories={stories}
        availableTags={[]}
        activeLang=""
        isLoadingStories={false}
        storiesError=""
        hasNextStoryPage
        isFetchingNextStoryPage={false}
        readerStateKey="test-reader-state"
        onLoadNextStoryPage={vi.fn()}
        onActiveStoryChange={vi.fn()}
        onTranslationStateChange={vi.fn()}
        onScrollTargetSettled={onScrollTargetSettled}
      />
    </QueryClientProvider>,
  );

  return { ...view, queryClient };
}

describe("StoryReaderPanel", () => {
  const originalRequestAnimationFrame = window.requestAnimationFrame;
  const originalCancelAnimationFrame = window.cancelAnimationFrame;
  const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;
  const originalScrollTo = HTMLElement.prototype.scrollTo;
  const scrollIntoView = vi.fn();

  beforeEach(() => {
    vi.useFakeTimers();
    vi.mocked(getStoryDetail).mockClear();
    scrollIntoView.mockClear();
    HTMLElement.prototype.scrollTo = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;
    window.requestAnimationFrame = (callback) =>
      window.setTimeout(() => callback(performance.now()), 0);
    window.cancelAnimationFrame = (handle) => window.clearTimeout(handle);
    window.sessionStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
    window.requestAnimationFrame = originalRequestAnimationFrame;
    window.cancelAnimationFrame = originalCancelAnimationFrame;
    HTMLElement.prototype.scrollIntoView = originalScrollIntoView;
    HTMLElement.prototype.scrollTo = originalScrollTo;
  });

  it("settles an initial selected story only after it renders in canonical feed order", async () => {
    const onScrollTargetSettled = vi.fn();
    const { rerender, queryClient } = renderReader(
      [makeStory("story-before", "Story before")],
      onScrollTargetSettled,
    );

    expect(getStoryDetail).toHaveBeenCalledWith("target-story", "");
    await act(async () => {
      vi.advanceTimersByTime(1000);
    });

    expect(onScrollTargetSettled).not.toHaveBeenCalled();

    rerender(
      <QueryClientProvider client={queryClient}>
        <StoryReaderPanel
          selectedStoryUUID="target-story"
          selectedItemUUID=""
          scrollTargetStoryUUID="target-story"
          scrollTargetRevision={0}
          stories={[
            makeStory("story-before", "Story before"),
            makeStory("target-story", "Target story"),
            makeStory("story-after", "Story after"),
          ]}
          availableTags={[]}
          activeLang=""
          isLoadingStories={false}
          storiesError=""
          hasNextStoryPage
          isFetchingNextStoryPage={false}
          readerStateKey="test-reader-state"
          onLoadNextStoryPage={vi.fn()}
          onActiveStoryChange={vi.fn()}
          onTranslationStateChange={vi.fn()}
          onScrollTargetSettled={onScrollTargetSettled}
        />
      </QueryClientProvider>,
    );

    await act(async () => {
      vi.advanceTimersByTime(1000);
    });
    await act(async () => {
      vi.advanceTimersByTime(1000);
    });

    expect(onScrollTargetSettled).toHaveBeenCalledWith("target-story");
  });
});

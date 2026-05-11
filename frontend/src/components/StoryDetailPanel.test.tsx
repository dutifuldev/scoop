import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getStoryArticlePreview, requestTranslation } from "../api";
import { StoryDetailPanel } from "./StoryDetailPanel";
import type { StoryDetailResponse } from "../types";

vi.mock("../api", () => ({
  getStoryArticlePreview: vi.fn(async (storyMemberUUID: string, _maxChars = 1000) => ({
    story_article_uuid: storyMemberUUID,
    preview_text: `Fetched preview for ${storyMemberUUID}.\n\nSecond paragraph for ${storyMemberUUID}.`,
    source: "normalized_text",
    char_count: 64,
    truncated: false,
  })),
  requestTranslation: vi.fn(async () => ({
    stats: { total: 1, translated: 1, cached: 0, skipped: 0 },
  })),
}));

function makeDetail(): StoryDetailResponse {
  return {
    story: {
      story_id: 1,
      story_uuid: "story-uuid-1",
      collection: "ai_news",
      translation_mode: "disabled",
      title: "Story Title",
      original_title: "Story Title",
      translated_title: null,
      detected_language: "en",
      canonical_url: "https://example.com/story",
      status: "active",
      first_seen_at: "2026-02-14T09:00:00Z",
      last_seen_at: "2026-02-14T15:13:00Z",
      source_count: 2,
      article_count: 2,
    },
    members: [
      {
        story_article_uuid: "member-1",
        article_uuid: "doc-1",
        source: "source-a",
        source_item_id: "a-1",
        collection: "ai_news",
        canonical_url: "https://a.example.com/1",
        published_at: "2026-02-14T09:00:00Z",
        normalized_title: "First item",
        normalized_text: "First expanded content body.",
        detected_language: "en",
        original_title: "First item",
        translated_title: null,
        original_text: "First expanded content body.",
        translated_text: null,
        matched_at: "2026-02-14T15:13:00Z",
        match_type: "exact_url",
        dedup_decision: "AUTO_MERGE",
      },
      {
        story_article_uuid: "member-2",
        article_uuid: "doc-2",
        source: "source-b",
        source_item_id: "b-1",
        collection: "ai_news",
        canonical_url: "https://b.example.com/1",
        published_at: "2026-02-14T10:00:00Z",
        normalized_title: "Second item",
        normalized_text: "Second expanded content body.",
        detected_language: "en",
        original_title: "Second item",
        translated_title: null,
        original_text: "Second expanded content body.",
        translated_text: null,
        matched_at: "2026-02-14T15:14:00Z",
        match_type: "semantic",
        dedup_decision: "AUTO_MERGE",
      },
    ],
  };
}

function makeDetailWithSharedURL(): StoryDetailResponse {
  return {
    story: {
      story_id: 2,
      story_uuid: "story-uuid-shared",
      collection: "ai_news",
      translation_mode: "disabled",
      title: "Shared URL Story",
      original_title: "Shared URL Story",
      translated_title: null,
      detected_language: "en",
      canonical_url: "https://shared.example.com/glm-5",
      status: "active",
      first_seen_at: "2026-02-14T09:00:00Z",
      last_seen_at: "2026-02-15T10:00:00Z",
      source_count: 2,
      article_count: 2,
    },
    members: [
      {
        story_article_uuid: "shared-member-1",
        article_uuid: "shared-doc-1",
        source: "dedup_ai-news",
        source_item_id: "simonwillison.net_2026_Feb_11_glm-5",
        collection: "ai_news",
        canonical_url: "https://shared.example.com/glm-5",
        published_at: "2026-02-13T09:00:00Z",
        normalized_title: "glm-5: from vibe coding to agentic engineering",
        normalized_text: "First source text.",
        detected_language: "en",
        original_title: "glm-5: from vibe coding to agentic engineering",
        translated_title: null,
        original_text: "First source text.",
        translated_text: null,
        matched_at: "2026-02-15T20:34:45Z",
        match_type: "seed",
        match_score: 1,
        dedup_decision: "NEW_STORY",
      },
      {
        story_article_uuid: "shared-member-2",
        article_uuid: "shared-doc-2",
        source: "simon_willison",
        source_item_id: "simonwillison.net_2026_Feb_11_glm-5",
        collection: "ai_news",
        canonical_url: "https://shared.example.com/glm-5",
        published_at: "2026-02-13T09:00:00Z",
        normalized_title: "glm-5: 754b parameter mit-licensed model released",
        normalized_text: "Second source text.",
        detected_language: "en",
        original_title: "glm-5: 754b parameter mit-licensed model released",
        translated_title: null,
        original_text: "Second source text.",
        translated_text: null,
        matched_at: "2026-02-15T20:34:46Z",
        match_type: "exact_url",
        match_score: 1,
        dedup_decision: "AUTO_MERGE",
      },
    ],
  };
}

describe("StoryDetailPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("auto-expands all items when a story is opened", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <StoryDetailPanel
          selectedStoryUUID="story-uuid-1"
          selectedItemUUID=""
          detail={makeDetail()}
          activeLang=""
          isLoading={false}
          error=""
          onSelectItem={vi.fn()}
          onClearSelectedItem={vi.fn()}
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText("Fetched preview for member-1.")).toBeInTheDocument();
      expect(screen.getByText("Fetched preview for member-2.")).toBeInTheDocument();
      expect(screen.queryByText("Fetched content by URL")).not.toBeInTheDocument();
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("member-1", 1000);
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("member-2", 1000);
    });
  });

  it("collapses same-url duplicates into one visible item while showing dedup provenance rows", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <StoryDetailPanel
          selectedStoryUUID="story-uuid-shared"
          selectedItemUUID=""
          detail={makeDetailWithSharedURL()}
          activeLang=""
          isLoading={false}
          error=""
          onSelectItem={vi.fn()}
          onClearSelectedItem={vi.fn()}
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      const toggles = screen.getAllByRole("button", { name: /Collapse item/i });
      expect(toggles).toHaveLength(1);
      expect(screen.getByText("Deduped items")).toBeInTheDocument();
      expect(
        screen.getByText("dedup_ai-news:simonwillison.net_2026_Feb_11_glm-5 • seed • score 1.000"),
      ).toBeInTheDocument();
      expect(
        screen.getByText(
          "simon_willison:simonwillison.net_2026_Feb_11_glm-5 • exact_url • score 1.000",
        ),
      ).toBeInTheDocument();
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("shared-member-1", 1000);
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("shared-member-2", 1000);
    });
  });

  it("does not request translation for disabled member collections", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    const detail = makeDetail();
    detail.story.collection = "china_news";
    detail.story.translation_mode = "enabled";
    detail.story.translated_title = "Translated story title";
    detail.members[0].collection = "openclaw";
    detail.members[0].translation_mode = "disabled";
    detail.members[0].translated_text = null;
    detail.members[1].collection = "metal_news";
    detail.members[1].translation_mode = "enabled";
    detail.members[1].translated_text = "Translated member body.";

    render(
      <QueryClientProvider client={queryClient}>
        <StoryDetailPanel
          selectedStoryUUID="story-uuid-1"
          selectedItemUUID=""
          detail={detail}
          activeLang="en"
          isLoading={false}
          error=""
          onSelectItem={vi.fn()}
          onClearSelectedItem={vi.fn()}
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("member-1", 1000);
    });
    expect(vi.mocked(requestTranslation)).not.toHaveBeenCalled();
  });
});

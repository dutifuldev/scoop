import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  addArticleTag,
  getStoryArticlePreview,
  removeArticleTag,
  requestTranslation,
} from "../api";
import { StoryDetailPanel } from "./StoryDetailPanel";
import type { StoryDetailResponse } from "../types";

vi.mock("../api", () => ({
  addArticleTag: vi.fn(async () => undefined),
  getStoryArticlePreview: vi.fn(async (storyMemberUUID: string, _maxChars = 4000) => ({
    story_article_uuid: storyMemberUUID,
    preview_text: `Fetched preview for ${storyMemberUUID}.\n\nSecond paragraph for ${storyMemberUUID}.`,
    source: "normalized_text",
    char_count: 64,
    truncated: false,
  })),
  removeArticleTag: vi.fn(async () => undefined),
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
          availableTags={[]}
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
      expect(screen.queryByText("EN")).not.toBeInTheDocument();
      expect(screen.queryByText("Original")).not.toBeInTheDocument();
      expect(screen.queryByText("Fetched content by URL")).not.toBeInTheDocument();
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("member-1", 4000);
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("member-2", 4000);
    });
  });

  it("renders same-url members as separate visible items", async () => {
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
          availableTags={[]}
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
      expect(toggles).toHaveLength(2);
      expect(screen.queryByText("Deduped items")).not.toBeInTheDocument();
      expect(
        screen.getByText("glm-5: from vibe coding to agentic engineering"),
      ).toBeInTheDocument();
      expect(
        screen.getByText("glm-5: 754b parameter mit-licensed model released"),
      ).toBeInTheDocument();
      expect(screen.getByText("Fetched preview for shared-member-1.")).toBeInTheDocument();
      expect(screen.getByText("Fetched preview for shared-member-2.")).toBeInTheDocument();
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("shared-member-1", 4000);
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("shared-member-2", 4000);
    });
  });

  it("renders markdown links with bare domain targets", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    const discordLinkText = `${"Context. ".repeat(130)}

Evidence:

- [Ephemeral watches discussion in Discord](discord.com/channels/1/2/3)`;

    vi.mocked(getStoryArticlePreview).mockImplementation(async (storyMemberUUID: string) => ({
      story_article_uuid: storyMemberUUID,
      preview_text: storyMemberUUID === "member-1" ? discordLinkText : "Other preview.",
      source: "normalized_text",
      char_count: 64,
      truncated: false,
    }));

    render(
      <QueryClientProvider client={queryClient}>
        <StoryDetailPanel
          selectedStoryUUID="story-uuid-1"
          selectedItemUUID=""
          detail={makeDetail()}
          availableTags={[]}
          activeLang=""
          isLoading={false}
          error=""
          onSelectItem={vi.fn()}
          onClearSelectedItem={vi.fn()}
        />
      </QueryClientProvider>,
    );

    const link = await screen.findByRole("link", {
      name: "Ephemeral watches discussion in Discord",
    });
    expect(link).toHaveAttribute("href", "https://discord.com/channels/1/2/3");
    expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("member-1", 4000);
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
          availableTags={[]}
          activeLang="en"
          isLoading={false}
          error=""
          onSelectItem={vi.fn()}
          onClearSelectedItem={vi.fn()}
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("member-1", 4000);
    });
    expect(vi.mocked(requestTranslation)).not.toHaveBeenCalled();
    expect(vi.mocked(addArticleTag)).not.toHaveBeenCalled();
    expect(vi.mocked(removeArticleTag)).not.toHaveBeenCalled();
  });

  it("adds an existing tag from the typeahead input", async () => {
    const user = userEvent.setup();
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
          availableTags={[
            {
              tag_id: 1,
              tag_uuid: "tag-uuid-1",
              tag: "needs-review",
              color: "#4EA1FF",
              created_at: "2026-02-14T09:00:00Z",
              updated_at: "2026-02-14T09:00:00Z",
            },
          ]}
          activeLang=""
          isLoading={false}
          error=""
          onSelectItem={vi.fn()}
          onClearSelectedItem={vi.fn()}
        />
      </QueryClientProvider>,
    );

    const [firstAddButton] = await screen.findAllByRole("button", { name: "Add article tag" });
    await user.click(firstAddButton);
    const firstTagInput = screen.getByLabelText("Article tag search");
    await user.type(firstTagInput, "NEEDS");
    expect(firstTagInput).toHaveValue("needs");

    await user.click(screen.getByRole("option", { name: "needs-review" }));

    await waitFor(() => {
      expect(vi.mocked(addArticleTag)).toHaveBeenCalledWith("doc-1", "needs-review");
    });
  });
});

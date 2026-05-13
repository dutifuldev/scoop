import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
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

function makeSingleArticleDetail(): StoryDetailResponse {
  return {
    story: {
      story_id: 3,
      story_uuid: "story-uuid-single",
      collection: "openclaw",
      translation_mode: "disabled",
      title: "Solo Story",
      original_title: "Solo Story",
      translated_title: null,
      detected_language: "en",
      canonical_url: "https://solo.example.com/item",
      status: "active",
      first_seen_at: "2026-02-14T09:00:00Z",
      last_seen_at: "2026-02-14T09:00:00Z",
      source_count: 1,
      article_count: 1,
    },
    members: [
      {
        story_article_uuid: "single-member-1",
        article_uuid: "single-doc-1",
        source: "source-a",
        source_item_id: "single-1",
        collection: "openclaw",
        canonical_url: "https://solo.example.com/item",
        published_at: "2026-02-14T09:00:00Z",
        normalized_title: "Solo Story",
        normalized_text: "Solo expanded content body.",
        detected_language: "en",
        original_title: "Solo Story",
        translated_title: null,
        original_text: "Solo expanded content body.",
        translated_text: null,
        matched_at: "2026-02-14T09:00:00Z",
        match_type: "seed",
        dedup_decision: "NEW_STORY",
        tags: [
          {
            tag_id: 10,
            tag_uuid: "tag-uuid-i0",
            tag: "i0",
            color: "#f4212e",
            created_at: "2026-02-14T09:00:00Z",
            updated_at: "2026-02-14T09:00:00Z",
          },
        ],
        person_identities: [
          {
            person_identity_id: 1,
            person_identity_uuid: "person-identity-uuid-1",
            provider: "discord",
            provider_user_id: "123456789012345678",
            handle: "alice",
            avatar_url: "https://cdn.discordapp.com/avatars/123456789012345678/avatar.webp?size=128",
            identity_ref: "id://discord/id/123456789012345678?handle=alice",
            created_at: "2026-02-14T09:00:00Z",
            updated_at: "2026-02-14T09:00:00Z",
          },
        ],
      },
    ],
  };
}

describe("StoryDetailPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders article previews when a story is opened", async () => {
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
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText(/Fetched preview for member-1\./)).toBeInTheDocument();
      expect(screen.getByText(/Fetched preview for member-2\./)).toBeInTheDocument();
      expect(screen.queryByText("EN")).not.toBeInTheDocument();
      expect(screen.queryByText("Original")).not.toBeInTheDocument();
      expect(screen.queryByText("Fetched content by URL")).not.toBeInTheDocument();
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("member-1", 4000);
      expect(vi.mocked(getStoryArticlePreview)).toHaveBeenCalledWith("member-2", 4000);
    });
  });

  it("renders single-article stories without duplicate title or article boxes", async () => {
    const user = userEvent.setup();
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });

    const { container } = render(
      <QueryClientProvider client={queryClient}>
        <StoryDetailPanel
          selectedStoryUUID="story-uuid-single"
          selectedItemUUID=""
          detail={makeSingleArticleDetail()}
          availableTags={[
            {
              tag_id: 11,
              tag_uuid: "tag-uuid-needs-review",
              tag: "needs-review",
              color: "#71767b",
              created_at: "2026-02-14T09:00:00Z",
              updated_at: "2026-02-14T09:00:00Z",
            },
          ]}
          activeLang=""
          isLoading={false}
          error=""
        />
      </QueryClientProvider>,
    );

    await screen.findByText(/Fetched preview for single-member-1\./);

    expect(screen.queryByRole("heading", { name: "Solo Story" })).toBeNull();
    expect(container.querySelector(".detail-title-row")).toBeNull();
    const singleIdentityByline = screen.getByText("@alice").closest(".article-byline");
    expect(singleIdentityByline).not.toBeNull();
    expect(within(singleIdentityByline as HTMLElement).queryByText(/matched /)).toBeNull();
    expect(within(singleIdentityByline as HTMLElement).queryByText("new_story")).toBeNull();
    expect(within(singleIdentityByline as HTMLElement).getByText("Feb 14")).toHaveAttribute(
      "title",
      expect.stringContaining("Ingested Feb 14,"),
    );
    expect(within(singleIdentityByline as HTMLElement).getByText("Feb 14")).toHaveAttribute(
      "title",
      expect.stringContaining("Decision: new story"),
    );
    const singleArticleTitle = within(singleIdentityByline as HTMLElement).getByText("Solo Story");
    expect(singleArticleTitle.closest(".article-byline-title-stack")).not.toBeNull();
    expect(
      screen.getByText("@alice").compareDocumentPosition(singleArticleTitle) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.queryByText(/Collection:/)).not.toBeInTheDocument();
    expect(screen.getByText("@alice").closest(".article-byline")).not.toBeNull();
    const avatar = container.querySelector(".article-byline-avatar");
    expect(avatar).not.toBeNull();
    expect(avatar?.querySelector("img")).toHaveAttribute(
      "src",
      "https://cdn.discordapp.com/avatars/123456789012345678/avatar.webp?size=128",
    );
    expect(container.querySelector(".article-byline-provider-icon")).not.toBeNull();
    expect(screen.getByRole("link", { name: "solo.example.com" })).toHaveClass("title-action");
    expect(
      container.querySelector(".article-byline-title-stack .member-tag-tools-title"),
    ).not.toBeNull();
    expect(screen.queryByText("discord")).not.toBeInTheDocument();
    expect(screen.getByText("i0")).toHaveClass("title-tag");
    expect(screen.queryByRole("button", { name: "Add article person identity" })).toBeNull();
    expect(screen.queryByLabelText("Article person identity controls")).toBeNull();
    expect(screen.getByRole("button", { name: "Add article tag" })).toHaveClass("title-action");
    expect(container.querySelector(".article-entry")).not.toBeNull();
    expect(container.querySelector(".member-card-single")).toBeNull();
    expect(container.querySelector(".detail-item-content-single")).toBeNull();
    expect(container.querySelector(".detail-text-block-single")).toBeNull();
    expect(container.querySelector(".member-toggle")).toBeNull();
    expect(container.querySelector(".member-expanded-url")).toBeNull();
    expect(container.querySelector(".member-sub")).toBeNull();

    const writeText = vi.fn(async () => undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    expect(screen.queryByRole("button", { name: "Copy story link for Solo Story" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Copy article link for Solo Story" }));
    expect(writeText).toHaveBeenCalledWith("https://solo.example.com/item");
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Add article tag" }));

    expect(
      container.querySelector(".article-byline-title-stack .member-tag-input-shell-title"),
    ).not.toBeNull();
    expect(screen.getByLabelText("Article tag search")).toBeInTheDocument();
  });

  it("renders merged-story member titles with inline source links", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    const detail = makeDetail();
    detail.members[0].tags = [
      {
        tag_id: 12,
        tag_uuid: "tag-uuid-i0-merged",
        tag: "i0",
        color: "#f4212e",
        created_at: "2026-02-14T09:00:00Z",
        updated_at: "2026-02-14T09:00:00Z",
      },
    ];
    detail.members[0].person_identities = [
      {
        person_identity_id: 21,
        person_identity_uuid: "person-identity-uuid-merged-1",
        provider: "discord",
        provider_user_id: "123456789012345678",
        handle: "alice",
        identity_ref: "id://discord/id/123456789012345678?handle=alice",
        created_at: "2026-02-14T09:00:00Z",
        updated_at: "2026-02-14T09:00:00Z",
      },
    ];

    const { container } = render(
      <QueryClientProvider client={queryClient}>
        <StoryDetailPanel
          selectedStoryUUID="story-uuid-1"
          selectedItemUUID=""
          detail={detail}
          availableTags={[
            {
              tag_id: 13,
              tag_uuid: "tag-uuid-needs-review-merged",
              tag: "needs-review",
              color: "#71767b",
              created_at: "2026-02-14T09:00:00Z",
              updated_at: "2026-02-14T09:00:00Z",
            },
          ]}
          activeLang=""
          isLoading={false}
          error=""
        />
      </QueryClientProvider>,
    );

    await screen.findByText(/Fetched preview for member-1\./);

    expect(screen.queryByRole("heading", { name: "Story title" })).toBeNull();
    expect(screen.getByText("First item")).toBeInTheDocument();
    expect(screen.getByText("Second item")).toBeInTheDocument();
    const identityByline = screen.getByText("@alice").closest(".article-byline");
    expect(identityByline).not.toBeNull();
    expect(within(identityByline as HTMLElement).queryByText(/matched /)).toBeNull();
    expect(within(identityByline as HTMLElement).queryByText("auto_merge")).toBeNull();
    expect(within(identityByline as HTMLElement).getByText("Feb 14")).toHaveAttribute(
      "title",
      expect.stringContaining("Decision: auto merge"),
    );
    const inlineMemberTitle = within(identityByline as HTMLElement).getByText("First item");
    expect(inlineMemberTitle.closest(".article-byline-title-stack")).not.toBeNull();
    expect(
      screen.getByText("@alice").compareDocumentPosition(inlineMemberTitle) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.getByRole("link", { name: "a.example.com" })).toHaveClass("title-action");
    expect(screen.getByRole("link", { name: "b.example.com" })).toHaveClass("title-action");
    expect(container.querySelector(".member-title-row .member-tag-tools-title")).not.toBeNull();
    expect(screen.getByText("i0")).toHaveClass("title-tag");
    expect(container.querySelector(".article-byline-connector")).toBeNull();
    expect(container.querySelector(".article-entry-has-next")).not.toBeNull();
    expect(container.querySelector(".article-entry-has-prev")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
    expect(container.querySelector(".member-card-single")).toBeNull();
    expect(container.querySelector(".member-toggle")).toBeNull();
    expect(container.querySelector(".member-expanded-url")).toBeNull();
  });

  it("shows the article expansion control only when text is actually truncated", async () => {
    const user = userEvent.setup();
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    const longPreview = `Start. ${"Long article content. ".repeat(190)}Tail marker.`;
    vi.mocked(getStoryArticlePreview).mockImplementationOnce(async (storyMemberUUID: string) => ({
      story_article_uuid: storyMemberUUID,
      preview_text: longPreview,
      source: "normalized_text",
      char_count: longPreview.length,
      truncated: false,
    }));

    render(
      <QueryClientProvider client={queryClient}>
        <StoryDetailPanel
          selectedStoryUUID="story-uuid-single"
          selectedItemUUID=""
          detail={makeSingleArticleDetail()}
          availableTags={[]}
          activeLang=""
          isLoading={false}
          error=""
        />
      </QueryClientProvider>,
    );

    const showMore = await screen.findByRole("button", { name: "Show more" });
    expect(screen.queryByText(/Tail marker/)).toBeNull();
    await user.click(showMore);
    expect(screen.getByRole("button", { name: "Show less" })).toBeInTheDocument();
    expect(screen.getByText(/Tail marker/)).toBeInTheDocument();
  });

  it("does not show the article expansion control based on element dimensions", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    const scrollHeightDescriptor = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "scrollHeight",
    );
    const clientHeightDescriptor = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "clientHeight",
    );
    Object.defineProperty(HTMLElement.prototype, "scrollHeight", {
      configurable: true,
      get() {
        return this.classList.contains("article-body-collapsed") ? 900 : 0;
      },
    });
    Object.defineProperty(HTMLElement.prototype, "clientHeight", {
      configurable: true,
      get() {
        return this.classList.contains("article-body-collapsed") ? 100 : 0;
      },
    });

    try {
      render(
        <QueryClientProvider client={queryClient}>
          <StoryDetailPanel
            selectedStoryUUID="story-uuid-single"
            selectedItemUUID=""
            detail={makeSingleArticleDetail()}
            availableTags={[]}
            activeLang=""
            isLoading={false}
            error=""
          />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
      });
    } finally {
      if (scrollHeightDescriptor) {
        Object.defineProperty(HTMLElement.prototype, "scrollHeight", scrollHeightDescriptor);
      } else {
        Reflect.deleteProperty(HTMLElement.prototype, "scrollHeight");
      }
      if (clientHeightDescriptor) {
        Object.defineProperty(HTMLElement.prototype, "clientHeight", clientHeightDescriptor);
      } else {
        Reflect.deleteProperty(HTMLElement.prototype, "clientHeight");
      }
    }
  });

  it("renders Discord title source links with shared title action geometry", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    const detail = makeSingleArticleDetail();
    detail.story.canonical_url = "https://discord.com/channels/1/2/3";
    detail.members[0].canonical_url = "https://discord.com/channels/1/2/3";

    render(
      <QueryClientProvider client={queryClient}>
        <StoryDetailPanel
          selectedStoryUUID="story-uuid-single"
          selectedItemUUID=""
          detail={detail}
          availableTags={[
            {
              tag_id: 14,
              tag_uuid: "tag-uuid-needs-review-discord",
              tag: "needs-review",
              color: "#71767b",
              created_at: "2026-02-14T09:00:00Z",
              updated_at: "2026-02-14T09:00:00Z",
            },
          ]}
          activeLang=""
          isLoading={false}
          error=""
        />
      </QueryClientProvider>,
    );

    const discordSourceLink = await screen.findByRole("link", { name: "Discord message" });
    expect(discordSourceLink).toHaveClass("title-action");
    expect(discordSourceLink.querySelector("img.discord-link-icon")).not.toBeNull();
    expect(screen.getByText("i0")).toHaveClass("title-tag");
    expect(screen.getByRole("button", { name: "Add article tag" })).toHaveClass("title-action");
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
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      const titleCopyButtons = screen.getAllByRole("button", { name: /Copy article link for/i });
      expect(titleCopyButtons).toHaveLength(2);
      expect(screen.queryByText("Deduped items")).not.toBeInTheDocument();
      expect(
        screen.getByText("glm-5: from vibe coding to agentic engineering"),
      ).toBeInTheDocument();
      expect(
        screen.getByText("glm-5: 754b parameter mit-licensed model released"),
      ).toBeInTheDocument();
      expect(screen.getByText(/Fetched preview for shared-member-1\./)).toBeInTheDocument();
      expect(screen.getByText(/Fetched preview for shared-member-2\./)).toBeInTheDocument();
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
        />
      </QueryClientProvider>,
    );

    const link = await screen.findByRole("link", {
      name: "Ephemeral watches discussion in Discord",
    });
    expect(link).toHaveAttribute("href", "discord://-/channels/1/2/3");
    expect(link.querySelector("img.discord-link-icon")).not.toBeNull();
    expect(screen.getByRole("link", { name: "Open Discord message in browser" })).toHaveAttribute(
      "href",
      "https://discord.com/channels/1/2/3",
    );
    const copyButton = screen.getByRole("button", { name: "Copy Discord message link" });
    expect(copyButton).toBeInTheDocument();

    const writeText = vi.fn(async () => {
      throw new Error("blocked");
    });
    const execCommand = vi.fn(() => true);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: execCommand,
    });
    await userEvent.click(copyButton);
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith("https://discord.com/channels/1/2/3"),
    );
    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(screen.getByRole("button", { name: "Copy Discord message link" })).toHaveTextContent(
      "Copied",
    );
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

    const { container } = render(
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
        />
      </QueryClientProvider>,
    );

    expect(screen.queryByLabelText("Article tag search")).not.toBeInTheDocument();
    expect(container.querySelector(".member-tag-input-shell")).toBeNull();

    const [firstAddButton] = await screen.findAllByRole("button", { name: "Add article tag" });
    await user.click(firstAddButton);
    const firstTagInput = screen.getByLabelText("Article tag search");
    expect(container.querySelector(".member-tag-input-shell")).not.toBeNull();
    await user.type(firstTagInput, "NEEDS");
    expect(firstTagInput).toHaveValue("needs");

    await user.click(screen.getByRole("option", { name: "needs-review" }));

    await waitFor(() => {
      expect(vi.mocked(addArticleTag)).toHaveBeenCalledWith("doc-1", "needs-review");
    });
  });
});

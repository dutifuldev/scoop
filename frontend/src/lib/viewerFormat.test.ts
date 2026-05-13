import { afterEach, describe, expect, it, vi } from "vitest";

import type { StoryListItem } from "../types";

import {
  buildFeedMetaText,
  buildFeedSourceText,
  formatBylineDate,
  formatDateTime,
} from "./viewerFormat";

function makeStory(overrides: Partial<StoryListItem>): StoryListItem {
  return {
    story_id: 1,
    story_uuid: "00000000-0000-0000-0000-000000000001",
    collection: "test",
    translation_mode: "disabled",
    title: "Test story",
    original_title: "Test story",
    translated_title: null,
    detected_language: "en",
    status: "active",
    first_seen_at: "2026-02-15T00:00:00Z",
    last_seen_at: "2026-02-15T00:00:00Z",
    source_count: 1,
    article_count: 1,
    ...overrides,
  };
}

describe("buildFeedSourceText", () => {
  it("shows domain for single-source stories", () => {
    const story = makeStory({
      canonical_url: "https://www.nytimes.com/2026/02/15/world/example.html",
      source_count: 1,
    });

    expect(buildFeedSourceText(story)).toBe("nytimes.com");
  });

  it("shows domain and others for multi-source stories", () => {
    const story = makeStory({
      canonical_url: "https://news.ycombinator.com/item?id=123",
      source_count: 4,
    });

    expect(buildFeedSourceText(story)).toBe("news.ycombinator.com and 3 others");
  });

  it("falls back to representative source when URL is missing", () => {
    const story = makeStory({
      canonical_url: undefined,
      source_count: 2,
      representative: {
        article_uuid: "00000000-0000-0000-0000-000000000002",
        source: "reuters",
        source_item_id: "abc",
      },
    });

    expect(buildFeedSourceText(story)).toBe("reuters and 1 other");
  });

  it("falls back to count text when neither URL nor source exists", () => {
    const story = makeStory({
      canonical_url: undefined,
      source_count: 2,
      representative: undefined,
    });

    expect(buildFeedSourceText(story)).toBe("2 sources");
  });
});

describe("buildFeedMetaText", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns source text by default (non-search mode)", () => {
    const story = makeStory({
      canonical_url: "https://news.ycombinator.com/item?id=123",
      last_seen_at: "2026-02-14T15:13:19Z",
      source_count: 1,
    });

    expect(buildFeedMetaText(story)).toBe("news.ycombinator.com");
  });

  it("includes date and source text when timestamp mode is enabled", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-02-15T12:00:00Z"));

    const story = makeStory({
      canonical_url: "https://news.ycombinator.com/item?id=123",
      last_seen_at: "2026-02-14T15:13:19Z",
      source_count: 1,
    });

    expect(buildFeedMetaText(story, true)).toMatch(
      /^Feb 14, \d{2}:\d{2} • news\.ycombinator\.com$/,
    );
  });
});

describe("formatDateTime", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("omits year for dates in the current year and uses 24-hour time without seconds", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-02-15T12:00:00Z"));

    const text = formatDateTime("2026-01-13T15:04:59Z");
    expect(text).toMatch(/^[A-Z][a-z]{2} \d{1,2}, \d{2}:\d{2}$/);
    expect(text).not.toMatch(/\bAM\b|\bPM\b/);
    expect(text).not.toMatch(/:\d{2}:\d{2}$/);
    expect(text).not.toContain("2026");
  });

  it("includes year for dates outside the current year", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-02-15T12:00:00Z"));

    const text = formatDateTime("2025-01-13T15:04:59Z");
    expect(text).toMatch(/^[A-Z][a-z]{2} \d{1,2}, \d{4}, \d{2}:\d{2}$/);
    expect(text).toContain("2025");
    expect(text).not.toMatch(/\bAM\b|\bPM\b/);
    expect(text).not.toMatch(/:\d{2}:\d{2}$/);
  });
});

describe("formatBylineDate", () => {
  const now = new Date("2026-05-13T12:00:00Z");

  it("uses compact relative time for recent dates", () => {
    expect(formatBylineDate("2026-05-13T11:59:45Z", now)).toBe("now");
    expect(formatBylineDate("2026-05-13T11:48:00Z", now)).toBe("12m");
    expect(formatBylineDate("2026-05-13T06:30:00Z", now)).toBe("5h");
  });

  it("uses month and day for older dates in the same year", () => {
    expect(formatBylineDate("2026-05-11T10:00:00Z", now)).toBe("May 11");
  });

  it("adds the year for dates outside the current year", () => {
    expect(formatBylineDate("2025-06-01T10:00:00Z", now)).toBe("Jun 1, 2025");
  });
});

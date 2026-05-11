import { describe, expect, it } from "vitest";

import type { StoryFilters } from "../types";
import { buildStoryFilters } from "./viewerFilters";

function makeBaseFilters(overrides: Partial<StoryFilters> = {}): StoryFilters {
  return {
    page: 1,
    pageSize: 25,
    collection: "ai_news",
    query: "",
    from: "",
    to: "",
    lang: "",
    tag: "",
    ...overrides,
  };
}

describe("buildStoryFilters", () => {
  it("applies day filter when advanced is off and query is empty", () => {
    const filters = buildStoryFilters({
      baseFilters: makeBaseFilters(),
      routeCollection: "",
      showAdvancedSearch: false,
      day: "2026-02-14",
    });

    expect(filters.from).toBe("2026-02-14");
    expect(filters.to).toBe("2026-02-14");
  });

  it("does not apply day filter when searching with advanced off", () => {
    const filters = buildStoryFilters({
      baseFilters: makeBaseFilters({ query: "anthropic" }),
      routeCollection: "",
      showAdvancedSearch: false,
      day: "2026-02-14",
    });

    expect(filters.from).toBe("");
    expect(filters.to).toBe("");
  });

  it("keeps advanced from/to filters when advanced is on", () => {
    const filters = buildStoryFilters({
      baseFilters: makeBaseFilters({ query: "anthropic", from: "2026-02-10", to: "2026-02-14" }),
      routeCollection: "",
      showAdvancedSearch: true,
      day: "2026-02-13",
    });

    expect(filters.from).toBe("2026-02-10");
    expect(filters.to).toBe("2026-02-14");
  });

  it("prefers route collection over base collection", () => {
    const filters = buildStoryFilters({
      baseFilters: makeBaseFilters({ collection: "all" }),
      routeCollection: "china_news",
      showAdvancedSearch: false,
      day: "",
    });

    expect(filters.collection).toBe("china_news");
  });
});

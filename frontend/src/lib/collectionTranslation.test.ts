import { describe, expect, it } from "vitest";

import {
  defaultCollectionTranslationMode,
  isCollectionTranslationEnabled,
  normalizeCollectionTranslationMode,
} from "./collectionTranslation";

describe("collection translation policy", () => {
  it("enables only China and Metal news by default", () => {
    expect(defaultCollectionTranslationMode("china_news")).toBe("enabled");
    expect(defaultCollectionTranslationMode("metal_news")).toBe("enabled");
    expect(defaultCollectionTranslationMode("openclaw")).toBe("disabled");
    expect(defaultCollectionTranslationMode("ai_news")).toBe("disabled");
  });

  it("normalizes unknown modes to disabled", () => {
    expect(normalizeCollectionTranslationMode("enabled")).toBe("enabled");
    expect(normalizeCollectionTranslationMode("manual_only")).toBe("disabled");
    expect(isCollectionTranslationEnabled("disabled")).toBe(false);
  });
});

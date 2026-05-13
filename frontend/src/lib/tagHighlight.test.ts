import { describe, expect, it } from "vitest";

import type { Tag } from "../types";
import { effectiveTagHighlight, tagHighlightStyle } from "./tagHighlight";

function makeTag(tag: string, highlightColor?: string): Tag {
  return {
    tag_id: tag.length,
    tag_uuid: `tag-${tag}`,
    tag,
    color: "#71767b",
    highlight_color: highlightColor,
    created_at: "2026-05-13T00:00:00Z",
    updated_at: "2026-05-13T00:00:00Z",
  };
}

describe("tagHighlight", () => {
  it("selects the first highlighted tag by tag value", () => {
    expect(
      effectiveTagHighlight([
        makeTag("zeta", "#ff00ff"),
        makeTag("alpha", "#faa61a"),
        makeTag("plain"),
      ]),
    ).toBe("#faa61a");
  });

  it("returns CSS custom properties when a highlight exists", () => {
    expect(tagHighlightStyle([makeTag("i0", "#faa61a")])).toEqual({
      "--tag-highlight-color": "#faa61a",
    });
    expect(tagHighlightStyle([makeTag("plain")])).toBeUndefined();
  });
});

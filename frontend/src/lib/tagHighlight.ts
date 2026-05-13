import type { CSSProperties } from "react";

import type { Tag } from "../types";

export function effectiveTagHighlight(tags: Tag[] | undefined): string {
  if (!tags || tags.length === 0) {
    return "";
  }

  return (
    [...tags]
      .sort((a, b) => a.tag.localeCompare(b.tag))
      .find((tag) => tag.highlight_color?.trim())
      ?.highlight_color?.trim() || ""
  );
}

export function tagHighlightStyle(tags: Tag[] | undefined): CSSProperties | undefined {
  const highlightColor = effectiveTagHighlight(tags);
  if (!highlightColor) {
    return undefined;
  }
  return { "--tag-highlight-color": highlightColor } as CSSProperties;
}

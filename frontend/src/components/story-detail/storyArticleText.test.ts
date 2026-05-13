import { describe, expect, it } from "vitest";

import { truncateArticleTextBlocks, type ArticleTextBlock } from "./storyArticleText";

describe("truncateArticleTextBlocks", () => {
  it("leaves short article text unchanged", () => {
    const blocks: ArticleTextBlock[] = [
      { key: "original", label: "Original", paragraphs: ["Short paragraph."] },
    ];

    const result = truncateArticleTextBlocks(blocks, 100);

    expect(result.isTruncated).toBe(false);
    expect(result.blocks).toBe(blocks);
  });

  it("truncates at paragraph content instead of component dimensions", () => {
    const blocks: ArticleTextBlock[] = [
      { key: "original", label: "Original", paragraphs: ["First paragraph.", "Second paragraph."] },
    ];

    const result = truncateArticleTextBlocks(blocks, 18);

    expect(result.isTruncated).toBe(true);
    expect(result.blocks).toEqual([
      { key: "original", label: "Original", paragraphs: ["First paragraph."] },
    ]);
  });

  it("uses unicode code points when truncating a long paragraph", () => {
    const blocks: ArticleTextBlock[] = [
      { key: "original", label: "Original", paragraphs: ["abcdef🙂gh"] },
    ];

    const result = truncateArticleTextBlocks(blocks, 8);

    expect(result.isTruncated).toBe(true);
    expect(result.blocks[0]?.paragraphs).toEqual(["abcde..."]);
  });
});

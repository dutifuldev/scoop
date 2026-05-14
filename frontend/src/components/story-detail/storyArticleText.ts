export interface ArticleTextBlock {
  key: string;
  paragraphs: string[];
  label: string;
}

export interface ArticleBodyModel {
  blocks: ArticleTextBlock[];
  isTruncated: boolean;
}

export const collapsedArticleTextMaxChars = 3200;

const truncationSuffix = "...";

function runeLength(value: string): number {
  return Array.from(value).length;
}

function truncateRunes(value: string, maxChars: number): string {
  if (maxChars <= truncationSuffix.length) {
    return truncationSuffix;
  }

  const runes = Array.from(value);
  if (runes.length <= maxChars) {
    return value;
  }

  return `${runes
    .slice(0, maxChars - truncationSuffix.length)
    .join("")
    .trimEnd()}${truncationSuffix}`;
}

export function truncateArticleTextBlocks(
  blocks: ArticleTextBlock[],
  maxChars: number,
): ArticleBodyModel {
  let remainingChars = maxChars;
  let isTruncated = false;
  const nextBlocks: ArticleTextBlock[] = [];

  for (const block of blocks) {
    if (remainingChars <= 0) {
      isTruncated = true;
      break;
    }

    const nextParagraphs: string[] = [];

    for (const paragraph of block.paragraphs) {
      const paragraphLength = runeLength(paragraph);
      const paragraphSeparatorLength = nextParagraphs.length > 0 ? 2 : 0;
      const neededLength = paragraphLength + paragraphSeparatorLength;

      if (neededLength <= remainingChars) {
        nextParagraphs.push(paragraph);
        remainingChars -= neededLength;
        continue;
      }

      const availableForParagraph = Math.max(0, remainingChars - paragraphSeparatorLength);
      if (availableForParagraph > truncationSuffix.length) {
        nextParagraphs.push(truncateRunes(paragraph, availableForParagraph));
      }
      isTruncated = true;
      break;
    }

    if (nextParagraphs.length > 0) {
      nextBlocks.push({ ...block, paragraphs: nextParagraphs });
    }

    if (isTruncated) {
      break;
    }
  }

  return { blocks: isTruncated ? nextBlocks : blocks, isTruncated };
}

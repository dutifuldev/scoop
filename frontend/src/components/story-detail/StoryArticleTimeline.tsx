import { useState } from "react";

import { tagHighlightStyle } from "../../lib/tagHighlight";
import { getViewerTimeZone } from "../../lib/viewerTimeZone";
import { formatDateTime } from "../../lib/viewerFormat";
import type { StoryArticlePreview, Tag } from "../../types";
import { ArticleByline } from "./ArticleByline";
import { ArticleTitleRow } from "./ArticleTitleRow";
import {
  collapsedArticleTextMaxChars,
  truncateArticleTextBlocks,
  type ArticleTextBlock,
} from "./storyArticleText";
import type { MemberURLGroup } from "./storyMemberGroups";
import { renderTextBlock, toParagraphs } from "./storyTextRendering";

interface StoryArticleTimelineProps {
  collection: string;
  storyUUID: string;
  groups: MemberURLGroup[];
  selectedItemUUID: string;
  detailTextMode: "translated" | "original";
  activeLang: string;
  availableTags: Tag[];
  tagMutationKey: string;
  itemPreviewByUUID: Record<string, StoryArticlePreview>;
  itemPreviewLoadingByUUID: Record<string, boolean>;
  itemPreviewErrorByUUID: Record<string, string>;
  onAddArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
  onRemoveArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
}

interface StoryArticleEntryProps {
  collection: string;
  storyUUID: string;
  group: MemberURLGroup;
  isSelected: boolean;
  hasPrevious: boolean;
  hasNext: boolean;
  detailTextMode: "translated" | "original";
  activeLang: string;
  availableTags: Tag[];
  tagMutationKey: string;
  itemPreviewByUUID: Record<string, StoryArticlePreview>;
  itemPreviewLoadingByUUID: Record<string, boolean>;
  itemPreviewErrorByUUID: Record<string, string>;
  onAddArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
  onRemoveArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
}

export function StoryArticleTimeline({
  collection,
  storyUUID,
  groups,
  selectedItemUUID,
  detailTextMode,
  activeLang,
  availableTags,
  tagMutationKey,
  itemPreviewByUUID,
  itemPreviewLoadingByUUID,
  itemPreviewErrorByUUID,
  onAddArticleTag,
  onRemoveArticleTag,
}: StoryArticleTimelineProps): JSX.Element {
  return (
    <section className="article-timeline">
      {groups.length === 0 ? <p className="muted">No items found for this story.</p> : null}
      {groups.map((group, index) => (
        <StoryArticleEntry
          key={group.key}
          collection={collection}
          storyUUID={storyUUID}
          group={group}
          isSelected={selectedItemUUID === group.representative.story_article_uuid}
          hasPrevious={groups.length > 1 && index > 0}
          hasNext={groups.length > 1 && index < groups.length - 1}
          detailTextMode={detailTextMode}
          activeLang={activeLang}
          availableTags={availableTags}
          tagMutationKey={tagMutationKey}
          itemPreviewByUUID={itemPreviewByUUID}
          itemPreviewLoadingByUUID={itemPreviewLoadingByUUID}
          itemPreviewErrorByUUID={itemPreviewErrorByUUID}
          onAddArticleTag={onAddArticleTag}
          onRemoveArticleTag={onRemoveArticleTag}
        />
      ))}
    </section>
  );
}

function StoryArticleEntry({
  collection,
  storyUUID,
  group,
  isSelected,
  hasPrevious,
  hasNext,
  detailTextMode,
  activeLang,
  availableTags,
  tagMutationKey,
  itemPreviewByUUID,
  itemPreviewLoadingByUUID,
  itemPreviewErrorByUUID,
  onAddArticleTag,
  onRemoveArticleTag,
}: StoryArticleEntryProps): JSX.Element {
  const [isBodyExpanded, setIsBodyExpanded] = useState(false);
  const representative = group.representative;
  const highlightStyle = tagHighlightStyle(representative.tags);
  const decisionText = representative.dedup_decision
    ? representative.dedup_decision.toLowerCase()
    : "";
  const decisionLabel = decisionText.replace(/_/g, " ");

  const previews = group.members
    .map((member) => itemPreviewByUUID[member.story_article_uuid])
    .filter((preview): preview is StoryArticlePreview => Boolean(preview?.preview_text?.trim()));
  const previewTexts = previews
    .map((preview) => preview.preview_text.trim())
    .filter((text) => text.length > 0);
  const completePreviewTexts = previews
    .filter((preview) => !preview.truncated)
    .map((preview) => preview.preview_text.trim())
    .filter((text) => text.length > 0);
  const truncatedPreviewTexts = previews
    .filter((preview) => preview.truncated)
    .map((preview) => preview.preview_text.trim())
    .filter((text) => text.length > 0);
  const originalTexts = group.members
    .map((member) => member.original_text?.trim() || member.normalized_text?.trim() || "")
    .filter((text) => text.length > 0);
  const translatedTexts = group.members
    .map((member) => member.translated_text?.trim() ?? "")
    .filter((text) => text.length > 0);

  const resolvedOriginalText =
    completePreviewTexts[0] || originalTexts[0] || truncatedPreviewTexts[0] || "";
  const resolvedTranslatedText = translatedTexts[0] || "";
  const originalParagraphs = toParagraphs(resolvedOriginalText);
  const translatedParagraphs = toParagraphs(resolvedTranslatedText);
  const hasOriginalContent = originalParagraphs.length > 0;
  const hasTranslatedContent = translatedParagraphs.length > 0;
  const hasExpandableContent = hasOriginalContent || hasTranslatedContent;
  const isPreviewLoading = group.members.some((member) =>
    Boolean(itemPreviewLoadingByUUID[member.story_article_uuid]),
  );
  const previewError = group.members.some((member) =>
    Boolean(itemPreviewErrorByUUID[member.story_article_uuid]),
  );
  const showTextModeToggle = hasOriginalContent && hasTranslatedContent;
  const showTextBlockLabels = showTextModeToggle;
  const orderedBlocks: ArticleTextBlock[] =
    detailTextMode === "translated"
      ? [
          { key: "translated", paragraphs: translatedParagraphs, label: "Translated" },
          { key: "original", paragraphs: originalParagraphs, label: "Original" },
        ]
      : [
          { key: "original", paragraphs: originalParagraphs, label: "Original" },
          { key: "translated", paragraphs: translatedParagraphs, label: "Translated" },
        ];
  const collapsedBodyModel = truncateArticleTextBlocks(orderedBlocks, collapsedArticleTextMaxChars);
  const renderedBlocks = isBodyExpanded ? orderedBlocks : collapsedBodyModel.blocks;
  const isArticleBodyTruncated = collapsedBodyModel.isTruncated;
  const representativeOriginalTitle = (
    representative.original_title ||
    representative.normalized_title ||
    ""
  ).trim();
  const representativeTranslatedTitle = (representative.translated_title || "").trim();
  const representativeDisplayTitle =
    activeLang !== "" && representativeTranslatedTitle !== ""
      ? representativeTranslatedTitle
      : representativeOriginalTitle;
  const viewerTimeZone = getViewerTimeZone();
  const bylineDateTitle = [
    representative.published_at
      ? `Published ${formatDateTime(representative.published_at, viewerTimeZone)}`
      : "",
    decisionLabel ? `Decision: ${decisionLabel}` : "",
    group.members.length > 1
      ? `Merged ${group.members.length} items from ${group.sourceCount} sources`
      : "",
  ]
    .filter(Boolean)
    .join(" · ");

  const renderedArticleBody = (
    <>
      {isPreviewLoading && !hasOriginalContent ? (
        <p className="muted">Fetching reader preview...</p>
      ) : null}
      {!isPreviewLoading && !hasOriginalContent && !hasTranslatedContent ? (
        <p className="muted">No content captured for this item.</p>
      ) : null}

      {showTextModeToggle ? (
        <p className="detail-item-content-mode-hint">
          Showing {detailTextMode === "translated" ? "translated first" : "original first"}.
        </p>
      ) : null}

      <div className="detail-item-content-body">
        {renderedBlocks.map((block) =>
          block.paragraphs.length > 0 ? (
            <section
              key={`${group.key}-${block.key}`}
              className={`detail-text-block detail-text-block-${block.key}`}
            >
              {showTextBlockLabels ? <p className="detail-text-label">{block.label}</p> : null}
              {block.paragraphs.map((paragraph, index) =>
                renderTextBlock(paragraph, `${group.key}-${block.key}-paragraph-${index}`),
              )}
            </section>
          ) : null,
        )}
      </div>

      {!isPreviewLoading && previewError && previewTexts.length === 0 && hasOriginalContent ? (
        <p className="muted">
          Reader preview unavailable. Showing captured content when available.
        </p>
      ) : null}
    </>
  );

  return (
    <article
      className={`article-entry ${isSelected ? "article-entry-selected" : ""} ${
        hasPrevious ? "article-entry-has-prev" : ""
      } ${hasNext ? "article-entry-has-next" : ""} ${
        highlightStyle ? "article-entry-highlighted" : ""
      }`.trim()}
      style={highlightStyle}
      data-item-uuid={representative.story_article_uuid}
    >
      <ArticleByline
        identities={representative.person_identities ?? []}
        publishedAt={representative.published_at}
        source={representative.source}
        dateTitle={bylineDateTitle}
      >
        <div className="article-byline-title-stack">
          <ArticleTitleRow
            article={representative}
            title={representativeDisplayTitle}
            canonicalURL={group.canonicalURL}
            collection={collection}
            storyUUID={storyUUID}
            showArticleActions
            availableTags={availableTags}
            tagMutationKey={tagMutationKey}
            className="member-title-row-byline"
            onAddArticleTag={onAddArticleTag}
            onRemoveArticleTag={onRemoveArticleTag}
          />
        </div>

        <article
          className={`detail-item-content article-body ${
            isBodyExpanded ? "article-body-expanded" : "article-body-collapsed"
          }`.trim()}
        >
          {renderedArticleBody}
        </article>

        {hasExpandableContent && (isBodyExpanded || isArticleBodyTruncated) ? (
          <button
            type="button"
            className="article-show-more"
            onClick={() => setIsBodyExpanded((previous) => !previous)}
          >
            {isBodyExpanded ? "Show less" : "Show more"}
          </button>
        ) : null}
      </ArticleByline>
    </article>
  );
}

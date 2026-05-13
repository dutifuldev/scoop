import { useEffect, useRef, useState } from "react";

import { formatDateTime } from "../../lib/viewerFormat";
import type { StoryArticle, StoryArticlePreview, StoryDetailResponse, Tag } from "../../types";
import { ArticleByline } from "./ArticleByline";
import { ArticleTitleRow } from "./ArticleTitleRow";
import { renderTextBlock, toParagraphs } from "./storyTextRendering";

export interface MemberURLGroup {
  key: string;
  canonicalURL: string;
  members: StoryArticle[];
  representative: StoryArticle;
  sourceCount: number;
}

export function memberGroupKey(member: StoryArticle): string {
  return `member:${member.story_article_uuid}`;
}

export function buildMemberGroups(detail: StoryDetailResponse | null): MemberURLGroup[] {
  if (!detail) {
    return [];
  }

  return detail.members.map((member) => ({
    key: memberGroupKey(member),
    canonicalURL: member.canonical_url?.trim() ?? "",
    members: [member],
    representative: member,
    sourceCount: 1,
  }));
}

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
  isLast: boolean;
  isMultiArticle: boolean;
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
          isLast={index === groups.length - 1}
          isMultiArticle={groups.length > 1}
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
  isLast,
  isMultiArticle,
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
  const [isBodyOverflowing, setIsBodyOverflowing] = useState(false);
  const bodyRef = useRef<HTMLElement | null>(null);
  const representative = group.representative;
  const decisionText = representative.dedup_decision
    ? representative.dedup_decision.toLowerCase()
    : "";
  const decisionLabel = decisionText.replace(/_/g, " ");

  const previewTexts = group.members
    .map((member) => itemPreviewByUUID[member.story_article_uuid]?.preview_text?.trim() ?? "")
    .filter((text) => text.length > 0);
  const originalTexts = group.members
    .map((member) => member.original_text?.trim() || member.normalized_text?.trim() || "")
    .filter((text) => text.length > 0);
  const translatedTexts = group.members
    .map((member) => member.translated_text?.trim() ?? "")
    .filter((text) => text.length > 0);

  const resolvedOriginalText = previewTexts[0] || originalTexts[0] || "";
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
  const orderedBlocks =
    detailTextMode === "translated"
      ? [
          { key: "translated", paragraphs: translatedParagraphs, label: "Translated" },
          { key: "original", paragraphs: originalParagraphs, label: "Original" },
        ]
      : [
          { key: "original", paragraphs: originalParagraphs, label: "Original" },
          { key: "translated", paragraphs: translatedParagraphs, label: "Translated" },
        ];
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
  const bylineDateTitle = [
    representative.published_at ? `Published ${formatDateTime(representative.published_at)}` : "",
    representative.matched_at ? `Ingested ${formatDateTime(representative.matched_at)}` : "",
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
        {orderedBlocks.map((block) =>
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

  useEffect(() => {
    if (!hasExpandableContent) {
      setIsBodyOverflowing(false);
      return;
    }

    const body = bodyRef.current;
    if (!body) {
      return;
    }

    const measure = (): void => {
      if (isBodyExpanded) {
        return;
      }
      setIsBodyOverflowing(body.scrollHeight - body.clientHeight > 8);
    };

    measure();
    const frame = window.requestAnimationFrame(measure);
    const timeout = window.setTimeout(measure, 120);
    let resizeObserver: ResizeObserver | null = null;
    if (typeof ResizeObserver !== "undefined") {
      resizeObserver = new ResizeObserver(measure);
      resizeObserver.observe(body);
    }

    return () => {
      window.cancelAnimationFrame(frame);
      window.clearTimeout(timeout);
      resizeObserver?.disconnect();
    };
  }, [
    detailTextMode,
    hasExpandableContent,
    isBodyExpanded,
    isPreviewLoading,
    resolvedOriginalText,
    resolvedTranslatedText,
  ]);

  return (
    <article
      className={`article-entry ${isSelected ? "article-entry-selected" : ""}`.trim()}
      data-item-uuid={representative.story_article_uuid}
    >
      <ArticleByline
        identities={representative.person_identities ?? []}
        publishedAt={representative.published_at}
        source={representative.source}
        dateTitle={bylineDateTitle}
        showConnector={isMultiArticle && !isLast}
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
          ref={bodyRef}
          className={`detail-item-content article-body ${
            isBodyExpanded ? "article-body-expanded" : "article-body-collapsed"
          }`.trim()}
        >
          {renderedArticleBody}
        </article>

        {hasExpandableContent && (isBodyExpanded || isBodyOverflowing) ? (
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

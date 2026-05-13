import type { Dispatch, ReactNode, SetStateAction } from "react";

import { hasActivePersonIdentity } from "../../lib/identityFormat";
import { buildMemberSubtitle, formatDateTime } from "../../lib/viewerFormat";
import type { StoryArticle, StoryArticlePreview, StoryDetailResponse, Tag } from "../../types";
import { ArticleTagEditor } from "./ArticleTagEditor";
import { ArticleByline } from "./ArticleByline";
import { ArticleTitleRow } from "./ArticleTitleRow";
import { buildMemberPreview, renderTextBlock, toParagraphs } from "./storyTextRendering";

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

interface StoryArticleGroupProps {
  group: MemberURLGroup;
  selectedItemUUID: string;
  selectedGroupKey: string;
  expandedGroupKeys: string[];
  isMergedStory: boolean;
  detailTextMode: "translated" | "original";
  activeLang: string;
  availableTags: Tag[];
  tagMutationKey: string;
  itemPreviewByUUID: Record<string, StoryArticlePreview>;
  itemPreviewLoadingByUUID: Record<string, boolean>;
  itemPreviewErrorByUUID: Record<string, string>;
  showArticleTitleActions?: boolean;
  onExpandedGroupKeysChange: Dispatch<SetStateAction<string[]>>;
  onSelectItem: (itemUUID: string) => void;
  onClearSelectedItem: () => void;
  onAddArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
  onRemoveArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
}

export function StoryArticleGroup({
  group,
  selectedItemUUID,
  selectedGroupKey,
  expandedGroupKeys,
  isMergedStory,
  detailTextMode,
  activeLang,
  availableTags,
  tagMutationKey,
  itemPreviewByUUID,
  itemPreviewLoadingByUUID,
  itemPreviewErrorByUUID,
  showArticleTitleActions = true,
  onExpandedGroupKeysChange,
  onSelectItem,
  onClearSelectedItem,
  onAddArticleTag,
  onRemoveArticleTag,
}: StoryArticleGroupProps): JSX.Element {
  const representative = group.representative;
  const isExpanded = !isMergedStory || expandedGroupKeys.includes(group.key);
  const hasSelectedMember = selectedGroupKey === group.key;
  const decisionText = representative.dedup_decision
    ? representative.dedup_decision.toLowerCase()
    : "";

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
  const collapsedPreviewText =
    detailTextMode === "translated"
      ? resolvedTranslatedText || resolvedOriginalText
      : resolvedOriginalText || resolvedTranslatedText;

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
  const routeItemUUID = hasSelectedMember ? selectedItemUUID : representative.story_article_uuid;
  const hasRepresentativeIdentity = hasActivePersonIdentity(representative.person_identities);
  const shouldPlaceTitleInByline = hasRepresentativeIdentity;
  const toggleGroup = (): void => {
    if (isExpanded) {
      onExpandedGroupKeysChange((previous) =>
        previous.filter((existingGroupKey) => existingGroupKey !== group.key),
      );
      if (hasSelectedMember) {
        onClearSelectedItem();
      }
      return;
    }

    onExpandedGroupKeysChange((previous) => {
      if (previous.includes(group.key)) {
        return previous;
      }
      return [...previous, group.key];
    });
    onSelectItem(routeItemUUID);
  };
  const renderMemberTitleRow = (className = ""): JSX.Element => (
    <ArticleTitleRow
      article={representative}
      title={representativeDisplayTitle}
      canonicalURL={group.canonicalURL}
      isExpanded={isExpanded}
      isMergedStory={isMergedStory}
      showArticleActions={showArticleTitleActions}
      availableTags={availableTags}
      tagMutationKey={tagMutationKey}
      className={className}
      onToggle={toggleGroup}
      onAddArticleTag={onAddArticleTag}
      onRemoveArticleTag={onRemoveArticleTag}
    />
  );
  const bylineMetaItems: ReactNode[] = [
    <span>matched {formatDateTime(representative.matched_at)}</span>,
    decisionText ? <span className="member-decision-inline">{decisionText}</span> : null,
    group.members.length > 1 ? (
      <span>
        merged {group.members.length} items from {group.sourceCount} sources
      </span>
    ) : null,
  ];
  const renderedArticleContent = (
    <article
      className={`detail-item-content member-expanded-content ${!isMergedStory ? "detail-item-content-single" : ""}`.trim()}
    >
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
              className={`detail-text-block detail-text-block-${block.key} ${!isMergedStory ? "detail-text-block-single" : ""}`.trim()}
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
    </article>
  );

  return (
    <article
      className={`member-card ${isExpanded ? "member-card-expanded" : ""} ${!isMergedStory ? "member-card-single" : ""}`.trim()}
    >
      {isMergedStory && !shouldPlaceTitleInByline ? (
        renderMemberTitleRow()
      ) : null}
      <ArticleByline
        identities={representative.person_identities ?? []}
        publishedAt={representative.published_at}
        source={representative.source}
        metaItems={bylineMetaItems}
      >
        {shouldPlaceTitleInByline ? (
          <div className="article-byline-title-stack">
            {renderMemberTitleRow("member-title-row-byline")}
          </div>
        ) : null}
        {isExpanded ? (
          renderedArticleContent
        ) : (
          <p className="member-preview member-preview-collapsed">
            {buildMemberPreview(collapsedPreviewText)}
          </p>
        )}
      </ArticleByline>
      {isExpanded ? (
        <>
          {group.members.length > 1 ? (
            <section className="member-merge-provenance">
              <p className="member-merge-provenance-title">Deduped items</p>
              <ul className="member-merge-provenance-list">
                {group.members.map((groupMember) => {
                  const memberDecision = groupMember.dedup_decision
                    ? groupMember.dedup_decision.toLowerCase()
                    : "";
                  const isSelected = selectedItemUUID === groupMember.story_article_uuid;
                  const memberBylineMetaItems: ReactNode[] = [
                    <span>matched {formatDateTime(groupMember.matched_at)}</span>,
                    memberDecision ? (
                      <span className="member-decision-inline">{memberDecision}</span>
                    ) : null,
                  ];

                  return (
                    <li
                      key={groupMember.story_article_uuid}
                      className={`member-merge-provenance-row ${isSelected ? "is-selected" : ""}`.trim()}
                    >
                      <button
                        type="button"
                        className="member-merge-provenance-link"
                        onClick={() => onSelectItem(groupMember.story_article_uuid)}
                      >
                        {buildMemberSubtitle(groupMember)}
                      </button>
                      <ArticleByline
                        identities={groupMember.person_identities ?? []}
                        publishedAt={groupMember.published_at}
                        source={groupMember.source}
                        metaItems={memberBylineMetaItems}
                      />
                      <ArticleTagEditor
                        articleUUID={groupMember.article_uuid}
                        currentTags={groupMember.tags ?? []}
                        availableTags={availableTags}
                        mutationKey={tagMutationKey}
                        onAddTag={onAddArticleTag}
                        onRemoveTag={onRemoveArticleTag}
                      />
                    </li>
                  );
                })}
              </ul>
            </section>
          ) : null}
        </>
      ) : null}
    </article>
  );
}

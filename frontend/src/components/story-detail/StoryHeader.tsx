import { hasActivePersonIdentity } from "../../lib/identityFormat";
import type { StoryDetailResponse, Tag } from "../../types";
import { ArticleTagEditor } from "./ArticleTagEditor";
import type { MemberURLGroup } from "./StoryArticleGroup";
import { StoryTitleCopyButton, TitleActions, TitleSourceLink } from "./TitleActions";

interface StoryHeaderProps {
  detail: StoryDetailResponse;
  memberGroups: MemberURLGroup[];
  activeLang: string;
  availableTags: Tag[];
  tagMutationKey: string;
  wrapperClassName?: string;
  onAddArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
  onRemoveArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
}

export function shouldHideSingleArticleHeader(
  detail: StoryDetailResponse | null,
  memberGroups: MemberURLGroup[],
): boolean {
  const singleRepresentative =
    detail && detail.story.article_count <= 1 && memberGroups.length === 1
      ? memberGroups[0].representative
      : null;
  return hasActivePersonIdentity(singleRepresentative?.person_identities);
}

export function StoryHeader({
  detail,
  memberGroups,
  activeLang,
  availableTags,
  tagMutationKey,
  wrapperClassName = "",
  onAddArticleTag,
  onRemoveArticleTag,
}: StoryHeaderProps): JSX.Element | null {
  if (shouldHideSingleArticleHeader(detail, memberGroups)) {
    return null;
  }

  const originalTitle = (detail.story.original_title || detail.story.title || "").trim();
  const translatedTitle = (detail.story.translated_title || "").trim();
  const showTranslatedTitle = activeLang !== "" && translatedTitle !== "";
  const displayTitle = showTranslatedTitle ? translatedTitle : originalTitle;
  const singleRepresentative =
    detail.story.article_count <= 1 && memberGroups.length === 1
      ? memberGroups[0].representative
      : null;
  const titleLinkURL = singleRepresentative ? memberGroups[0].canonicalURL : "";
  const content = (
    <>
      <div className="detail-title-row">
        <TitleActions className="detail-title-cluster">
          <h2 className="detail-title" aria-label={displayTitle}>
            <StoryTitleCopyButton
              title={displayTitle}
              collection={detail.story.collection}
              storyUUID={detail.story.story_uuid}
            />
          </h2>
          {titleLinkURL ? <TitleSourceLink url={titleLinkURL} /> : null}
          {singleRepresentative ? (
            <ArticleTagEditor
              articleUUID={singleRepresentative.article_uuid}
              currentTags={singleRepresentative.tags ?? []}
              availableTags={availableTags}
              mutationKey={tagMutationKey}
              variant="title"
              onAddTag={onAddArticleTag}
              onRemoveTag={onRemoveArticleTag}
            />
          ) : null}
        </TitleActions>
      </div>
      {showTranslatedTitle ? (
        <p className="detail-title-original">Original: {originalTitle || "(untitled)"}</p>
      ) : null}
    </>
  );

  return wrapperClassName ? <div className={wrapperClassName}>{content}</div> : content;
}

import { ChevronDown, ChevronRight } from "lucide-react";

import type { StoryArticle, Tag } from "../../types";
import { ArticleTagEditor } from "./ArticleTagEditor";
import { TitleActions, TitleSourceLink } from "./TitleActions";

interface ArticleTitleRowProps {
  article: StoryArticle;
  title: string;
  canonicalURL: string;
  isExpanded: boolean;
  isMergedStory: boolean;
  showArticleActions: boolean;
  availableTags: Tag[];
  tagMutationKey: string;
  className?: string;
  onToggle?: () => void;
  onAddArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
  onRemoveArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
}

export function ArticleTitleRow({
  article,
  title,
  canonicalURL,
  isExpanded,
  isMergedStory,
  showArticleActions,
  availableTags,
  tagMutationKey,
  className = "",
  onToggle,
  onAddArticleTag,
  onRemoveArticleTag,
}: ArticleTitleRowProps): JSX.Element {
  const displayTitle = title || "(no title)";

  return (
    <div className={`member-title-row ${className}`.trim()}>
      <TitleActions className="member-title-cluster">
        {isMergedStory ? (
          <button
            type="button"
            className={`member-toggle ${isExpanded ? "expanded" : ""}`.trim()}
            onClick={onToggle}
            aria-expanded={isExpanded}
            aria-label={`${isExpanded ? "Collapse" : "Expand"} item ${displayTitle}`}
          >
            <p className="member-head">{displayTitle}</p>
            {isExpanded ? (
              <ChevronDown className="member-toggle-icon" aria-hidden="true" />
            ) : (
              <ChevronRight className="member-toggle-icon" aria-hidden="true" />
            )}
          </button>
        ) : (
          <p className="member-head member-head-static">{displayTitle}</p>
        )}
        {canonicalURL ? <TitleSourceLink url={canonicalURL} /> : null}
        {showArticleActions ? (
          <ArticleTagEditor
            articleUUID={article.article_uuid}
            currentTags={article.tags ?? []}
            availableTags={availableTags}
            mutationKey={tagMutationKey}
            variant="title"
            onAddTag={onAddArticleTag}
            onRemoveTag={onRemoveArticleTag}
          />
        ) : null}
      </TitleActions>
    </div>
  );
}

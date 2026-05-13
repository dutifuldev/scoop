import { useState } from "react";

import type { StoryArticle, Tag } from "../../types";
import { ArticleTagEditor } from "./ArticleTagEditor";
import { buildStoryShareURL, TitleActions, TitleSourceLink } from "./TitleActions";
import { copyTextToClipboard } from "./storyTextRendering";

interface ArticleTitleRowProps {
  article: StoryArticle;
  title: string;
  canonicalURL: string;
  collection: string;
  storyUUID: string;
  showArticleActions: boolean;
  availableTags: Tag[];
  tagMutationKey: string;
  className?: string;
  onAddArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
  onRemoveArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
}

export function ArticleTitleRow({
  article,
  title,
  canonicalURL,
  collection,
  storyUUID,
  showArticleActions,
  availableTags,
  tagMutationKey,
  className = "",
  onAddArticleTag,
  onRemoveArticleTag,
}: ArticleTitleRowProps): JSX.Element {
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">("idle");
  const displayTitle = title || "(no title)";
  const normalizedCanonicalURL = canonicalURL.trim();
  const copyTitle =
    copyState === "copied"
      ? "Copied article link"
      : copyState === "failed"
        ? "Failed to copy article link"
        : "Copy article link";

  async function copyArticleLink(): Promise<void> {
    const link = normalizedCanonicalURL || buildStoryShareURL(collection, storyUUID);
    if (!link) {
      setCopyState("failed");
      window.setTimeout(() => setCopyState("idle"), 1400);
      return;
    }

    const copied = await copyTextToClipboard(link);
    setCopyState(copied ? "copied" : "failed");
    window.setTimeout(() => setCopyState("idle"), 1400);
  }

  return (
    <div className={`member-title-row ${className}`.trim()}>
      <TitleActions className="member-title-cluster">
        <button
          type="button"
          className={`member-title-copy ${copyState !== "idle" ? "is-active" : ""}`.trim()}
          onClick={() => {
            void copyArticleLink();
          }}
          aria-label={`Copy article link for ${displayTitle}`}
          title={copyTitle}
        >
          <span className="member-head">{displayTitle}</span>
        </button>
        {normalizedCanonicalURL ? <TitleSourceLink url={normalizedCanonicalURL} /> : null}
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

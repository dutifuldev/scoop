import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { addArticleTag, removeArticleTag, requestTranslation } from "../api";
import { useStoryArticlePreviews } from "../hooks/useStoryArticlePreviews";
import {
  defaultCollectionTranslationMode,
  isCollectionTranslationEnabled,
} from "../lib/collectionTranslation";
import type { StoryDetailResponse, Tag } from "../types";
import { ArticleTagEditor } from "./story-detail/ArticleTagEditor";
import {
  buildMemberGroups,
  StoryArticleGroup,
  type MemberURLGroup,
} from "./story-detail/StoryArticleGroup";
import { StoryTitleCopyButton, TitleActions, TitleSourceLink } from "./story-detail/TitleActions";

interface StoryDetailPanelProps {
  selectedStoryUUID: string;
  selectedItemUUID: string;
  detail: StoryDetailResponse | null;
  availableTags: Tag[];
  activeLang: string;
  isLoading: boolean;
  error: string;
  onSelectItem: (itemUUID: string) => void;
  onClearSelectedItem: () => void;
  onTranslationStateChange?: (storyUUID: string, isTranslating: boolean) => void;
}

export function StoryDetailPanel({
  selectedStoryUUID,
  selectedItemUUID,
  detail,
  availableTags,
  activeLang,
  isLoading,
  error,
  onSelectItem,
  onClearSelectedItem,
  onTranslationStateChange,
}: StoryDetailPanelProps): JSX.Element {
  const [expandedGroupKeys, setExpandedGroupKeys] = useState<string[]>([]);
  const [detailTextMode, setDetailTextMode] = useState<"translated" | "original">(
    activeLang ? "translated" : "original",
  );
  const [isTranslating, setIsTranslating] = useState(false);
  const [translationError, setTranslationError] = useState("");
  const [tagMutationKey, setTagMutationKey] = useState("");
  const [tagMutationError, setTagMutationError] = useState("");
  const translationRequestedRef = useRef<string>("");
  const activeTranslationKeyRef = useRef<string>("");
  const previousStoryUUIDRef = useRef<string>("");
  const queryClient = useQueryClient();
  const { itemPreviewByUUID, itemPreviewLoadingByUUID, itemPreviewErrorByUUID } =
    useStoryArticlePreviews(detail);
  const hasPendingTranslations = useMemo(() => {
    if (!activeLang || !detail) {
      return false;
    }

    const translatedTitle = (detail.story.translated_title || "").trim();
    const hasUntranslatedBody = detail.members.some((member) => {
      const mode = member.translation_mode ?? defaultCollectionTranslationMode(member.collection);
      return isCollectionTranslationEnabled(mode) && !(member.translated_text || "").trim();
    });
    return translatedTitle === "" || hasUntranslatedBody;
  }, [activeLang, detail]);

  // On-demand translation: when a language is selected and translations are missing, trigger translation
  useEffect(() => {
    if (!activeLang || !detail || !selectedStoryUUID) return;
    if (!hasPendingTranslations) return;

    const targetStoryUUID = selectedStoryUUID;
    const targetLang = activeLang;
    const reqKey = `${selectedStoryUUID}:${activeLang}`;
    if (translationRequestedRef.current === reqKey) return; // already requested
    translationRequestedRef.current = reqKey;
    activeTranslationKeyRef.current = reqKey;
    setTranslationError("");
    setIsTranslating(true);
    onTranslationStateChange?.(targetStoryUUID, true);

    void requestTranslation(targetStoryUUID, targetLang)
      .then(() => {
        // Keep the in-flight indicator visible until fresh translated content is loaded.
        return Promise.all([
          queryClient.invalidateQueries({
            queryKey: ["story-detail", targetStoryUUID, targetLang],
            exact: true,
          }),
          queryClient.invalidateQueries({ queryKey: ["stories"] }),
        ]).then(() =>
          Promise.all([
            queryClient.refetchQueries({
              queryKey: ["story-detail", targetStoryUUID, targetLang],
              exact: true,
              type: "active",
            }),
            queryClient.refetchQueries({ queryKey: ["stories"], type: "active" }),
          ]),
        );
      })
      .catch((err) => {
        translationRequestedRef.current = "";
        const message = err instanceof Error ? err.message : "Failed to translate story.";
        setTranslationError(message);
      })
      .finally(() => {
        if (activeTranslationKeyRef.current === reqKey) {
          activeTranslationKeyRef.current = "";
          setIsTranslating(false);
        }
        onTranslationStateChange?.(targetStoryUUID, false);
      });
  }, [
    activeLang,
    detail,
    hasPendingTranslations,
    onTranslationStateChange,
    queryClient,
    selectedStoryUUID,
  ]);

  useEffect(() => {
    setTranslationError("");
  }, [selectedStoryUUID, activeLang]);

  const memberGroups = useMemo<MemberURLGroup[]>(() => {
    return buildMemberGroups(detail);
  }, [detail]);

  const groupKeyByItemUUID = useMemo<Record<string, string>>(() => {
    const mapping: Record<string, string> = {};
    for (const group of memberGroups) {
      for (const member of group.members) {
        mapping[member.story_article_uuid] = group.key;
      }
    }
    return mapping;
  }, [memberGroups]);

  const selectedGroupKey = selectedItemUUID ? (groupKeyByItemUUID[selectedItemUUID] ?? "") : "";
  const showTranslationProgress =
    activeLang !== "" &&
    isTranslating &&
    activeTranslationKeyRef.current === `${selectedStoryUUID}:${activeLang}`;

  useEffect(() => {
    if (!detail) {
      setExpandedGroupKeys([]);
      previousStoryUUIDRef.current = "";
      return;
    }

    const validGroupKeys = new Set(memberGroups.map((group) => group.key));
    const isNewStorySelection = previousStoryUUIDRef.current !== detail.story.story_uuid;
    previousStoryUUIDRef.current = detail.story.story_uuid;

    setExpandedGroupKeys((previous) => {
      if (isNewStorySelection) {
        const next = memberGroups.map((group) => group.key);

        if (
          selectedGroupKey &&
          validGroupKeys.has(selectedGroupKey) &&
          !next.includes(selectedGroupKey)
        ) {
          next.push(selectedGroupKey);
        }

        return next;
      }

      const next = previous.filter((groupKey) => validGroupKeys.has(groupKey));

      if (
        selectedGroupKey &&
        validGroupKeys.has(selectedGroupKey) &&
        !next.includes(selectedGroupKey)
      ) {
        next.push(selectedGroupKey);
      }

      if (
        next.length === previous.length &&
        next.every((groupKey, index) => groupKey === previous[index])
      ) {
        return previous;
      }

      return next;
    });
  }, [detail, memberGroups, selectedGroupKey]);

  useEffect(() => {
    setDetailTextMode(activeLang ? "translated" : "original");
  }, [activeLang]);

  async function refreshTagsAfterMutation(): Promise<void> {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["story-detail", selectedStoryUUID] }),
      queryClient.invalidateQueries({ queryKey: ["stories"] }),
    ]);
  }

  async function onAddArticleTag(articleUUID: string, tagSlug: string): Promise<void> {
    if (!articleUUID || !tagSlug || tagSlug === "__add_tag__") {
      return;
    }

    const mutationKey = `${articleUUID}:${tagSlug}:add`;
    setTagMutationKey(mutationKey);
    setTagMutationError("");
    try {
      await addArticleTag(articleUUID, tagSlug);
      await refreshTagsAfterMutation();
    } catch (err) {
      setTagMutationError(err instanceof Error ? err.message : "Failed to add tag.");
      throw err;
    } finally {
      setTagMutationKey("");
    }
  }

  async function onRemoveArticleTag(articleUUID: string, tagSlug: string): Promise<void> {
    if (!articleUUID || !tagSlug) {
      return;
    }

    const mutationKey = `${articleUUID}:${tagSlug}:remove`;
    setTagMutationKey(mutationKey);
    setTagMutationError("");
    try {
      await removeArticleTag(articleUUID, tagSlug);
      await refreshTagsAfterMutation();
    } catch (err) {
      setTagMutationError(err instanceof Error ? err.message : "Failed to remove tag.");
    } finally {
      setTagMutationKey("");
    }
  }

  function renderStoryHeader(): JSX.Element {
    if (!detail) {
      return <></>;
    }

    const originalTitle = (detail.story.original_title || detail.story.title || "").trim();
    const translatedTitle = (detail.story.translated_title || "").trim();
    const showTranslatedTitle = activeLang !== "" && translatedTitle !== "";
    const displayTitle = showTranslatedTitle ? translatedTitle : originalTitle;
    const titleLinkURL =
      detail.story.article_count <= 1 && memberGroups.length === 1
        ? memberGroups[0].canonicalURL
        : "";
    const singleRepresentative =
      detail.story.article_count <= 1 && memberGroups.length === 1
        ? memberGroups[0].representative
        : null;

    return (
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
  }

  function renderStoryView(): JSX.Element {
    if (!detail) {
      return <></>;
    }
    const isMergedStory = detail.story.article_count > 1 || memberGroups.length > 1;

    return (
      <>
        {renderStoryHeader()}
        {activeLang ? (
          <div className="detail-text-mode-toggle" role="group" aria-label="Detail text mode">
            <button
              type="button"
              className={`detail-text-mode-btn ${detailTextMode === "translated" ? "active" : ""}`.trim()}
              onClick={() => setDetailTextMode("translated")}
            >
              Translated
            </button>
            <button
              type="button"
              className={`detail-text-mode-btn ${detailTextMode === "original" ? "active" : ""}`.trim()}
              onClick={() => setDetailTextMode("original")}
            >
              Original
            </button>
          </div>
        ) : null}
        {showTranslationProgress ? (
          <section
            className="translation-progress"
            role="status"
            aria-live="polite"
            aria-label="Translation in progress"
          >
            <div className="translation-progress-track" aria-hidden="true">
              <span className="translation-progress-bar" />
            </div>
            <p className="translation-progress-label">
              Translating to {activeLang.toUpperCase()}...
            </p>
          </section>
        ) : null}
        {translationError ? <p className="banner-error">{translationError}</p> : null}
        <section className="member-grid">
          {memberGroups.length === 0 ? (
            <p className="muted">No items found for this story.</p>
          ) : null}
          {memberGroups.map((group) => (
            <StoryArticleGroup
              key={group.key}
              group={group}
              selectedItemUUID={selectedItemUUID}
              selectedGroupKey={selectedGroupKey}
              expandedGroupKeys={expandedGroupKeys}
              isMergedStory={isMergedStory}
              detailTextMode={detailTextMode}
              activeLang={activeLang}
              availableTags={availableTags}
              tagMutationKey={tagMutationKey}
              itemPreviewByUUID={itemPreviewByUUID}
              itemPreviewLoadingByUUID={itemPreviewLoadingByUUID}
              itemPreviewErrorByUUID={itemPreviewErrorByUUID}
              showPrimaryTagEditor={isMergedStory}
              onExpandedGroupKeysChange={setExpandedGroupKeys}
              onSelectItem={onSelectItem}
              onClearSelectedItem={onClearSelectedItem}
              onAddArticleTag={onAddArticleTag}
              onRemoveArticleTag={onRemoveArticleTag}
            />
          ))}
        </section>
      </>
    );
  }

  return (
    <aside className="panel card detail-panel">
      <div className="detail-content">
        {!selectedStoryUUID ? (
          <p className="muted">Pick a story to inspect merged articles.</p>
        ) : null}
        {selectedStoryUUID && isLoading ? <p className="muted">Fetching story detail...</p> : null}
        {selectedStoryUUID && !isLoading && error ? <p className="muted">{error}</p> : null}
        {tagMutationError ? <p className="banner-error">{tagMutationError}</p> : null}
        {selectedStoryUUID && !isLoading && !error && detail ? renderStoryView() : null}
      </div>
    </aside>
  );
}

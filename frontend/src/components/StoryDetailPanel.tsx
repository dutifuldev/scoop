import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { addArticleTag, removeArticleTag, requestTranslation } from "../api";
import { useStoryArticlePreviews } from "../hooks/useStoryArticlePreviews";
import {
  defaultCollectionTranslationMode,
  isCollectionTranslationEnabled,
} from "../lib/collectionTranslation";
import type { StoryDetailResponse, Tag } from "../types";
import {
  buildMemberGroups,
  StoryArticleTimeline,
  type MemberURLGroup,
} from "./story-detail/StoryArticleTimeline";

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
  onTranslationStateChange,
}: StoryDetailPanelProps): JSX.Element {
  const [detailTextMode, setDetailTextMode] = useState<"translated" | "original">(
    activeLang ? "translated" : "original",
  );
  const [isTranslating, setIsTranslating] = useState(false);
  const [translationError, setTranslationError] = useState("");
  const [tagMutationKey, setTagMutationKey] = useState("");
  const [tagMutationError, setTagMutationError] = useState("");
  const translationRequestedRef = useRef<string>("");
  const activeTranslationKeyRef = useRef<string>("");
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

  const showTranslationProgress =
    activeLang !== "" &&
    isTranslating &&
    activeTranslationKeyRef.current === `${selectedStoryUUID}:${activeLang}`;

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

  function renderStoryView(): JSX.Element {
    if (!detail) {
      return <></>;
    }

    return (
      <>
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
        <StoryArticleTimeline
          collection={detail.story.collection}
          storyUUID={detail.story.story_uuid}
          groups={memberGroups}
          selectedItemUUID={selectedItemUUID}
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

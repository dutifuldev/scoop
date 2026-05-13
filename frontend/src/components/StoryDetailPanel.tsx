import { useStoryArticleDetailController } from "../hooks/useStoryArticleDetailController";
import type { StoryDetailResponse, Tag } from "../types";
import { StoryArticleTimeline } from "./story-detail/StoryArticleTimeline";

interface StoryDetailPanelProps {
  selectedStoryUUID: string;
  selectedItemUUID: string;
  detail: StoryDetailResponse | null;
  availableTags: Tag[];
  activeLang: string;
  isLoading: boolean;
  error: string;
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
  const {
    detailTextMode,
    setDetailTextMode,
    translationError,
    tagMutationKey,
    tagMutationError,
    showTranslationProgress,
    memberGroups,
    itemPreviewByUUID,
    itemPreviewLoadingByUUID,
    itemPreviewErrorByUUID,
    onAddArticleTag,
    onRemoveArticleTag,
  } = useStoryArticleDetailController({
    storyUUID: selectedStoryUUID,
    detail,
    activeLang,
    isTranslationActive: Boolean(selectedStoryUUID),
    onTranslationStateChange,
  });

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

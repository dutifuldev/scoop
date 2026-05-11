import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";

import { getStoryArticlePreview, requestTranslation } from "../api";
import {
  defaultCollectionTranslationMode,
  isCollectionTranslationEnabled,
} from "../lib/collectionTranslation";
import { buildMemberSubtitle, formatDateTime } from "../lib/viewerFormat";
import type { StoryDetailResponse, StoryArticlePreview, StoryArticle } from "../types";

interface StoryDetailPanelProps {
  selectedStoryUUID: string;
  selectedItemUUID: string;
  detail: StoryDetailResponse | null;
  activeLang: string;
  isLoading: boolean;
  error: string;
  onSelectItem: (itemUUID: string) => void;
  onClearSelectedItem: () => void;
  onTranslationStateChange?: (storyUUID: string, isTranslating: boolean) => void;
}

function pruneRecord<T>(record: Record<string, T>, validIDs: Set<string>): Record<string, T> {
  const next: Record<string, T> = {};
  let changed = false;

  for (const [key, value] of Object.entries(record)) {
    if (validIDs.has(key)) {
      next[key] = value;
      continue;
    }
    changed = true;
  }

  if (!changed && Object.keys(next).length === Object.keys(record).length) {
    return record;
  }
  return next;
}

interface MemberURLGroup {
  key: string;
  canonicalURL: string;
  members: StoryArticle[];
  representative: StoryArticle;
  sourceCount: number;
}

function memberGroupKey(member: StoryArticle): string {
  const canonicalURL = member.canonical_url?.trim().toLowerCase() ?? "";
  if (canonicalURL) {
    return `url:${canonicalURL}`;
  }
  return `member:${member.story_article_uuid}`;
}

const previewRequestBatchSize = 4;
const previewRequestDebounceMs = 120;

export function StoryDetailPanel({
  selectedStoryUUID,
  selectedItemUUID,
  detail,
  activeLang,
  isLoading,
  error,
  onSelectItem,
  onClearSelectedItem,
  onTranslationStateChange,
}: StoryDetailPanelProps): JSX.Element {
  const [expandedGroupKeys, setExpandedGroupKeys] = useState<string[]>([]);
  const [itemPreviewByUUID, setItemPreviewByUUID] = useState<Record<string, StoryArticlePreview>>(
    {},
  );
  const [itemPreviewLoadingByUUID, setItemPreviewLoadingByUUID] = useState<Record<string, boolean>>(
    {},
  );
  const [itemPreviewRequestedByUUID, setItemPreviewRequestedByUUID] = useState<
    Record<string, boolean>
  >({});
  const [itemPreviewErrorByUUID, setItemPreviewErrorByUUID] = useState<Record<string, string>>({});
  const [detailTextMode, setDetailTextMode] = useState<"translated" | "original">(
    activeLang ? "translated" : "original",
  );
  const [isTranslating, setIsTranslating] = useState(false);
  const [translationError, setTranslationError] = useState("");
  const translationRequestedRef = useRef<string>("");
  const activeTranslationKeyRef = useRef<string>("");
  const previousStoryUUIDRef = useRef<string>("");
  const queryClient = useQueryClient();
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
    if (!detail) {
      return [];
    }

    const grouped = new Map<string, StoryArticle[]>();
    for (const member of detail.members) {
      const key = memberGroupKey(member);
      const members = grouped.get(key);
      if (members) {
        members.push(member);
        continue;
      }
      grouped.set(key, [member]);
    }

    return Array.from(grouped.entries()).map(([key, members]) => {
      const sourceCount = new Set(members.map((member) => member.source)).size;
      const representative = members[0];

      return {
        key,
        canonicalURL: representative.canonical_url?.trim() ?? "",
        members,
        representative,
        sourceCount,
      };
    });
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
      setItemPreviewByUUID({});
      setItemPreviewLoadingByUUID({});
      setItemPreviewRequestedByUUID({});
      setItemPreviewErrorByUUID({});
      previousStoryUUIDRef.current = "";
      return;
    }

    const validItemIDs = new Set(detail.members.map((member) => member.story_article_uuid));
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

    setItemPreviewByUUID((previous) => pruneRecord(previous, validItemIDs));
    setItemPreviewLoadingByUUID((previous) => pruneRecord(previous, validItemIDs));
    setItemPreviewRequestedByUUID((previous) => pruneRecord(previous, validItemIDs));
    setItemPreviewErrorByUUID((previous) => pruneRecord(previous, validItemIDs));
  }, [detail, memberGroups, selectedGroupKey]);

  useEffect(() => {
    setDetailTextMode(activeLang ? "translated" : "original");
  }, [activeLang]);

  useEffect(() => {
    if (!detail) {
      return;
    }

    const pendingUUIDs = detail.members
      .map((member) => member.story_article_uuid)
      .filter((itemUUID) => !itemPreviewRequestedByUUID[itemUUID]);
    if (pendingUUIDs.length === 0) {
      return;
    }

    const timer = window.setTimeout(() => {
      const batch = pendingUUIDs.slice(0, previewRequestBatchSize);
      if (batch.length === 0) {
        return;
      }

      setItemPreviewRequestedByUUID((previous) => {
        const next = { ...previous };
        for (const itemUUID of batch) {
          next[itemUUID] = true;
        }
        return next;
      });
      setItemPreviewLoadingByUUID((previous) => {
        const next = { ...previous };
        for (const itemUUID of batch) {
          next[itemUUID] = true;
        }
        return next;
      });
      setItemPreviewErrorByUUID((previous) => {
        let changed = false;
        const next = { ...previous };
        for (const itemUUID of batch) {
          if (next[itemUUID]) {
            delete next[itemUUID];
            changed = true;
          }
        }
        return changed ? next : previous;
      });

      for (const itemUUID of batch) {
        void getStoryArticlePreview(itemUUID, 1000)
          .then((preview) => {
            setItemPreviewByUUID((previous) => ({
              ...previous,
              [itemUUID]: preview,
            }));
          })
          .catch((fetchErr) => {
            const message =
              fetchErr instanceof Error ? fetchErr.message : "Failed to fetch reader preview.";
            setItemPreviewErrorByUUID((previous) => ({
              ...previous,
              [itemUUID]: message,
            }));
          })
          .finally(() => {
            setItemPreviewLoadingByUUID((previous) => {
              if (!previous[itemUUID]) {
                return previous;
              }
              const next = { ...previous };
              delete next[itemUUID];
              return next;
            });
          });
      }
    }, previewRequestDebounceMs);

    return () => {
      window.clearTimeout(timer);
    };
  }, [detail, itemPreviewRequestedByUUID]);

  function buildMemberPreview(text?: string): string {
    const collapsed = (text ?? "").replace(/\s+/g, " ").trim();
    if (!collapsed) {
      return "No content captured for this item.";
    }

    const maxChars = 260;
    if (collapsed.length <= maxChars) {
      return collapsed;
    }
    return `${collapsed.slice(0, maxChars).trimEnd()}...`;
  }

  function toParagraphs(text: string): string[] {
    return text
      .split(/\n{2,}/)
      .map((paragraph) => paragraph.trim())
      .filter((paragraph) => paragraph.length > 0);
  }

  function renderStoryHeader(): JSX.Element {
    if (!detail) {
      return <></>;
    }

    const originalTitle = (detail.story.original_title || detail.story.title || "").trim();
    const translatedTitle = (detail.story.translated_title || "").trim();
    const showTranslatedTitle = activeLang !== "" && translatedTitle !== "";
    const displayTitle = showTranslatedTitle ? translatedTitle : originalTitle;
    const detectedLanguage = (detail.story.detected_language || "").trim().toLowerCase();
    const showDetectedLanguage = detectedLanguage !== "" && detectedLanguage !== "und";

    return (
      <>
        <div className="detail-title-row">
          <h2 className="detail-title">{displayTitle || "(untitled)"}</h2>
          {showDetectedLanguage ? (
            <span className="detail-language-badge">{detectedLanguage.toUpperCase()}</span>
          ) : null}
        </div>
        {showTranslatedTitle ? (
          <p className="detail-title-original">Original: {originalTitle || "(untitled)"}</p>
        ) : null}
        <p className="detail-meta">
          Collection: {detail.story.collection} • {detail.story.article_count} items •{" "}
          {detail.story.source_count} sources
        </p>
      </>
    );
  }

  function renderStoryView(): JSX.Element {
    if (!detail) {
      return <></>;
    }

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
          {memberGroups.map((group) => {
            const representative = group.representative;
            const isExpanded = expandedGroupKeys.includes(group.key);
            const hasSelectedMember = selectedGroupKey === group.key;
            const decisionText = representative.dedup_decision
              ? representative.dedup_decision.toLowerCase()
              : "";

            const previewTexts = group.members
              .map(
                (member) =>
                  itemPreviewByUUID[member.story_article_uuid]?.preview_text?.trim() ?? "",
              )
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
            const routeItemUUID = hasSelectedMember
              ? selectedItemUUID
              : representative.story_article_uuid;

            return (
              <article
                key={group.key}
                className={`member-card ${isExpanded ? "member-card-expanded" : ""}`.trim()}
              >
                <button
                  type="button"
                  className={`member-toggle ${isExpanded ? "expanded" : ""}`.trim()}
                  onClick={() => {
                    if (isExpanded) {
                      setExpandedGroupKeys((previous) =>
                        previous.filter((existingGroupKey) => existingGroupKey !== group.key),
                      );
                      if (hasSelectedMember) {
                        onClearSelectedItem();
                      }
                      return;
                    }

                    setExpandedGroupKeys((previous) => {
                      if (previous.includes(group.key)) {
                        return previous;
                      }
                      return [...previous, group.key];
                    });
                    onSelectItem(routeItemUUID);
                  }}
                  aria-expanded={isExpanded}
                  aria-label={`${isExpanded ? "Collapse" : "Expand"} item ${representativeDisplayTitle || "(no title)"}`}
                >
                  <p className="member-head">{representativeDisplayTitle || "(no title)"}</p>
                  {isExpanded ? (
                    <ChevronDown className="member-toggle-icon" aria-hidden="true" />
                  ) : (
                    <ChevronRight className="member-toggle-icon" aria-hidden="true" />
                  )}
                </button>
                <p className="member-sub">
                  matched {formatDateTime(representative.matched_at)} • published{" "}
                  {formatDateTime(representative.published_at)}
                  {decisionText ? (
                    <>
                      {" "}
                      • <span className="member-decision-inline">{decisionText}</span>
                    </>
                  ) : null}
                  {group.members.length > 1 ? (
                    <>
                      {" "}
                      • merged {group.members.length} items from {group.sourceCount} sources
                    </>
                  ) : null}
                </p>
                {isExpanded ? (
                  <>
                    {group.canonicalURL ? (
                      <a
                        className="member-expanded-url"
                        href={group.canonicalURL}
                        target="_blank"
                        rel="noreferrer"
                      >
                        {group.canonicalURL}
                      </a>
                    ) : null}
                    <article className="detail-item-content member-expanded-content">
                      {isPreviewLoading && !hasOriginalContent ? (
                        <p className="muted">Fetching reader preview...</p>
                      ) : null}
                      {!isPreviewLoading && !hasOriginalContent && !hasTranslatedContent ? (
                        <p className="muted">No content captured for this item.</p>
                      ) : null}

                      {showTextModeToggle ? (
                        <p className="detail-item-content-mode-hint">
                          Showing{" "}
                          {detailTextMode === "translated" ? "translated first" : "original first"}.
                        </p>
                      ) : null}

                      <div className="detail-item-content-body">
                        {orderedBlocks.map((block) =>
                          block.paragraphs.length > 0 ? (
                            <section
                              key={`${group.key}-${block.key}`}
                              className={`detail-text-block detail-text-block-${block.key}`.trim()}
                            >
                              <p className="detail-text-label">{block.label}</p>
                              {block.paragraphs.map((paragraph, index) => (
                                <p
                                  key={`${group.key}-${block.key}-paragraph-${index}`}
                                  className="detail-item-content-text"
                                >
                                  {paragraph}
                                </p>
                              ))}
                            </section>
                          ) : null,
                        )}
                      </div>

                      {!isPreviewLoading &&
                      previewError &&
                      previewTexts.length === 0 &&
                      hasOriginalContent ? (
                        <p className="muted">
                          Reader preview unavailable. Showing captured content when available.
                        </p>
                      ) : null}
                    </article>
                    {group.members.length > 1 ? (
                      <section className="member-merge-provenance">
                        <p className="member-merge-provenance-title">Deduped items</p>
                        <ul className="member-merge-provenance-list">
                          {group.members.map((groupMember) => {
                            const memberDecision = groupMember.dedup_decision
                              ? groupMember.dedup_decision.toLowerCase()
                              : "";
                            const isSelected = selectedItemUUID === groupMember.story_article_uuid;

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
                                <p className="member-sub">
                                  matched {formatDateTime(groupMember.matched_at)} • published{" "}
                                  {formatDateTime(groupMember.published_at)}
                                  {memberDecision ? (
                                    <>
                                      {" "}
                                      •{" "}
                                      <span className="member-decision-inline">
                                        {memberDecision}
                                      </span>
                                    </>
                                  ) : null}
                                </p>
                              </li>
                            );
                          })}
                        </ul>
                      </section>
                    ) : null}
                  </>
                ) : null}
                {!isExpanded ? (
                  <p className="member-preview member-preview-collapsed">
                    {buildMemberPreview(collapsedPreviewText)}
                  </p>
                ) : null}
              </article>
            );
          })}
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
        {selectedStoryUUID && !isLoading && !error && detail ? renderStoryView() : null}
      </div>
    </aside>
  );
}

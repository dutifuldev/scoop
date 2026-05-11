import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { ChevronDown, ChevronRight, Plus, X } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";

import {
  addArticleTag,
  getStoryArticlePreview,
  removeArticleTag,
  requestTranslation,
} from "../api";
import {
  defaultCollectionTranslationMode,
  isCollectionTranslationEnabled,
} from "../lib/collectionTranslation";
import { buildMemberSubtitle, formatDateTime } from "../lib/viewerFormat";
import type { StoryDetailResponse, StoryArticlePreview, StoryArticle, Tag } from "../types";
import { Input } from "./ui/input";

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
  return `member:${member.story_article_uuid}`;
}

const previewRequestBatchSize = 4;
const previewRequestDebounceMs = 120;
const expandedArticlePreviewMaxChars = 4000;
const inlineLinkPattern =
  /\[([^\]]+)\]\(([^)\s]+)\)|(https?:\/\/[^\s)\]}>,]+|(?:[a-z0-9-]+\.)+[a-z]{2,}(?:\/[^\s)\]}>,]*)?)/gi;
const discordMessagePattern = /^https?:\/\/discord\.com\/channels\/([^/]+)\/([^/]+)\/([^/?#]+)/i;

function trimTrailingURLPunctuation(rawURL: string): { url: string; trailing: string } {
  const match = rawURL.match(/[.,;:!?]+$/);
  if (!match) {
    return { url: rawURL, trailing: "" };
  }
  return { url: rawURL.slice(0, -match[0].length), trailing: match[0] };
}

function normalizeURLTarget(rawURL: string): { url: string; trailing: string } | null {
  const { url, trailing } = trimTrailingURLPunctuation(rawURL);

  if (/^https?:\/\//i.test(url)) {
    return { url, trailing };
  }

  if (/^(?:www\.|(?:[a-z0-9-]+\.)+[a-z]{2,}(?:[/:?#]|$))/i.test(url)) {
    return { url: `https://${url}`, trailing };
  }

  return null;
}

function labelForURL(url: string): string {
  const discordMatch = url.match(discordMessagePattern);
  if (discordMatch) {
    return "Discord message";
  }

  try {
    const parsed = new URL(url);
    return parsed.hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

function renderInlineLinks(text: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  let lastIndex = 0;

  for (const match of text.matchAll(inlineLinkPattern)) {
    const start = match.index ?? 0;
    const markdownLabel = match[1];
    const markdownURL = match[2];
    const rawURL = match[3];
    const rawMatch = match[0];
    const normalizedURL = normalizeURLTarget(markdownURL || rawURL || "");

    if (!normalizedURL) {
      continue;
    }
    const { url, trailing } = normalizedURL;

    if (start > lastIndex) {
      nodes.push(text.slice(lastIndex, start));
    }

    const isDiscordMessage = discordMessagePattern.test(url);
    nodes.push(
      <a
        key={`${url}-${start}`}
        className={isDiscordMessage ? "inline-discord-link" : "inline-rich-link"}
        href={url}
        target="_blank"
        rel="noreferrer"
        title={url}
      >
        {markdownLabel || labelForURL(url)}
      </a>,
    );
    if (!markdownURL && trailing) {
      nodes.push(trailing);
    }
    lastIndex = start + rawMatch.length;
  }

  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }

  return nodes;
}

function renderTextBlock(paragraph: string, key: string): JSX.Element {
  const heading = paragraph.match(/^(#{1,3})\s+(.+)$/);
  if (heading) {
    const levelClass = `detail-markdown-heading detail-markdown-heading-${heading[1].length}`;
    return (
      <p key={key} className={levelClass}>
        {renderInlineLinks(heading[2])}
      </p>
    );
  }

  const lines = paragraph
    .split(/\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  const bulletLines = lines
    .map((line) => line.match(/^[-*]\s+(.+)$/)?.[1])
    .filter((line): line is string => Boolean(line));

  if (bulletLines.length === lines.length) {
    return (
      <ul key={key} className="detail-markdown-list">
        {bulletLines.map((line, index) => (
          <li key={`${key}-bullet-${index}`}>{renderInlineLinks(line)}</li>
        ))}
      </ul>
    );
  }

  return (
    <p key={key} className="detail-item-content-text">
      {renderInlineLinks(paragraph)}
    </p>
  );
}

function tagChipStyle(tag: Tag): CSSProperties | undefined {
  return tag.color ? ({ "--tag-color": tag.color } as CSSProperties) : undefined;
}

function normalizeTagInput(raw: string): string {
  return raw
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, "")
    .replace(/-{2,}/g, "-")
    .slice(0, 64);
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
  const [tagMutationKey, setTagMutationKey] = useState("");
  const [tagMutationError, setTagMutationError] = useState("");
  const [activeTagArticleUUID, setActiveTagArticleUUID] = useState("");
  const [tagInputValue, setTagInputValue] = useState("");
  const translationRequestedRef = useRef<string>("");
  const activeTranslationKeyRef = useRef<string>("");
  const previousStoryUUIDRef = useRef<string>("");
  const tagInputBlurTimerRef = useRef<number | null>(null);
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

  useEffect(() => {
    setActiveTagArticleUUID("");
    setTagInputValue("");
  }, [selectedStoryUUID]);

  useEffect(() => {
    return () => {
      if (tagInputBlurTimerRef.current !== null) {
        window.clearTimeout(tagInputBlurTimerRef.current);
      }
    };
  }, []);

  const memberGroups = useMemo<MemberURLGroup[]>(() => {
    if (!detail) {
      return [];
    }

    return detail.members.map((member) => {
      return {
        key: memberGroupKey(member),
        canonicalURL: member.canonical_url?.trim() ?? "",
        members: [member],
        representative: member,
        sourceCount: 1,
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
        void getStoryArticlePreview(itemUUID, expandedArticlePreviewMaxChars)
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
      .split(/\n+/)
      .map((paragraph) => paragraph.trim())
      .filter((paragraph) => paragraph.length > 0);
  }

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
      setTagInputValue("");
      setActiveTagArticleUUID("");
      await refreshTagsAfterMutation();
    } catch (err) {
      setTagMutationError(err instanceof Error ? err.message : "Failed to add tag.");
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

  function renderArticleTags(member: StoryArticle): JSX.Element {
    const currentTags = member.tags ?? [];
    const currentTagsSet = new Set(currentTags.map((tag) => tag.tag));
    const addableTags = availableTags.filter((tag) => !currentTagsSet.has(tag.tag));
    const isInputActive = activeTagArticleUUID === member.article_uuid;
    const normalizedInputValue = isInputActive ? tagInputValue : "";
    const visibleSuggestions = addableTags
      .filter((tag) => !normalizedInputValue || tag.tag.includes(normalizedInputValue))
      .slice(0, 8);
    const exactTag = addableTags.find((tag) => tag.tag === normalizedInputValue);
    const hasSuggestions = isInputActive && visibleSuggestions.length > 0;

    return (
      <div className="member-tag-tools">
        <div
          className={`member-tag-input-shell ${isInputActive ? "is-active" : ""}`.trim()}
          aria-label="Article tags"
          onMouseDown={() => {
            if (tagInputBlurTimerRef.current !== null) {
              window.clearTimeout(tagInputBlurTimerRef.current);
              tagInputBlurTimerRef.current = null;
            }
          }}
        >
          {currentTags.map((tag) => {
            const mutationKey = `${member.article_uuid}:${tag.tag}:remove`;
            return (
              <span key={tag.tag} className="tag-chip" style={tagChipStyle(tag)}>
                {tag.tag}
                <button
                  type="button"
                  className="tag-chip-remove"
                  onClick={() => {
                    void onRemoveArticleTag(member.article_uuid, tag.tag);
                  }}
                  disabled={tagMutationKey === mutationKey}
                  aria-label={`Remove ${tag.tag} tag`}
                >
                  <X className="h-3 w-3" aria-hidden="true" />
                </button>
              </span>
            );
          })}
          {addableTags.length > 0 && !isInputActive ? (
            <button
              type="button"
              className="member-tag-add-button"
              aria-label="Add article tag"
              title="Add tag"
              onClick={() => {
                if (tagInputBlurTimerRef.current !== null) {
                  window.clearTimeout(tagInputBlurTimerRef.current);
                  tagInputBlurTimerRef.current = null;
                }
                setActiveTagArticleUUID(member.article_uuid);
                setTagInputValue("");
              }}
            >
              <Plus className="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          ) : null}
          {addableTags.length > 0 && isInputActive ? (
            <div className="member-tag-input-wrap">
              <Input
                value={normalizedInputValue}
                onFocus={() => {
                  if (tagInputBlurTimerRef.current !== null) {
                    window.clearTimeout(tagInputBlurTimerRef.current);
                    tagInputBlurTimerRef.current = null;
                  }
                  setActiveTagArticleUUID(member.article_uuid);
                }}
                onBlur={() => {
                  tagInputBlurTimerRef.current = window.setTimeout(() => {
                    setActiveTagArticleUUID("");
                    setTagInputValue("");
                  }, 120);
                }}
                onChange={(event) => {
                  setActiveTagArticleUUID(member.article_uuid);
                  setTagInputValue(normalizeTagInput(event.target.value));
                }}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    if (exactTag) {
                      void onAddArticleTag(member.article_uuid, exactTag.tag);
                    }
                    return;
                  }
                  if (event.key === "Escape") {
                    setActiveTagArticleUUID("");
                    setTagInputValue("");
                  }
                }}
                className="member-tag-input"
                placeholder="Add tag"
                aria-label="Article tag search"
                autoComplete="off"
                autoFocus
                spellCheck={false}
              />
              {hasSuggestions ? (
                <div className="member-tag-suggestions" role="listbox" aria-label="Matching tags">
                  {visibleSuggestions.map((tag) => {
                    const mutationKey = `${member.article_uuid}:${tag.tag}:add`;
                    return (
                      <button
                        key={tag.tag}
                        type="button"
                        className="member-tag-suggestion"
                        style={tagChipStyle(tag)}
                        onMouseDown={(event) => event.preventDefault()}
                        onClick={() => {
                          void onAddArticleTag(member.article_uuid, tag.tag);
                        }}
                        disabled={tagMutationKey === mutationKey}
                        role="option"
                      >
                        {tag.tag}
                      </button>
                    );
                  })}
                </div>
              ) : null}
            </div>
          ) : currentTags.length === 0 ? (
            <span className="member-tag-empty">No tags available</span>
          ) : null}
        </div>
      </div>
    );
  }

  function renderStoryHeader(): JSX.Element {
    if (!detail) {
      return <></>;
    }

    const originalTitle = (detail.story.original_title || detail.story.title || "").trim();
    const translatedTitle = (detail.story.translated_title || "").trim();
    const showTranslatedTitle = activeLang !== "" && translatedTitle !== "";
    const displayTitle = showTranslatedTitle ? translatedTitle : originalTitle;

    return (
      <>
        <div className="detail-title-row">
          <h2 className="detail-title">{displayTitle || "(untitled)"}</h2>
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
                {renderArticleTags(representative)}
                {isExpanded ? (
                  <>
                    {group.canonicalURL ? (
                      <a
                        className={`member-expanded-url ${discordMessagePattern.test(group.canonicalURL) ? "member-expanded-url-discord" : ""}`.trim()}
                        href={group.canonicalURL}
                        target="_blank"
                        rel="noreferrer"
                        title={group.canonicalURL}
                      >
                        {labelForURL(group.canonicalURL)}
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
                              {showTextBlockLabels ? (
                                <p className="detail-text-label">{block.label}</p>
                              ) : null}
                              {block.paragraphs.map((paragraph, index) =>
                                renderTextBlock(
                                  paragraph,
                                  `${group.key}-${block.key}-paragraph-${index}`,
                                ),
                              )}
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
                                {renderArticleTags(groupMember)}
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
        {tagMutationError ? <p className="banner-error">{tagMutationError}</p> : null}
        {selectedStoryUUID && !isLoading && !error && detail ? renderStoryView() : null}
      </div>
    </aside>
  );
}

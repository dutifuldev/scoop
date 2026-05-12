import { useEffect, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { Plus, X } from "lucide-react";

import type { Tag } from "../../types";
import { Input } from "../ui/input";
import { TitleActionButton, TitleTag, TitleTagInput } from "./TitleActions";

interface ArticleTagEditorProps {
  articleUUID: string;
  currentTags: Tag[];
  availableTags: Tag[];
  mutationKey: string;
  variant?: "default" | "title";
  onAddTag: (articleUUID: string, tagSlug: string) => Promise<void>;
  onRemoveTag: (articleUUID: string, tagSlug: string) => Promise<void>;
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

export function ArticleTagEditor({
  articleUUID,
  currentTags,
  availableTags,
  mutationKey,
  variant = "default",
  onAddTag,
  onRemoveTag,
}: ArticleTagEditorProps): JSX.Element {
  const [isInputActive, setIsInputActive] = useState(false);
  const [inputValue, setInputValue] = useState("");
  const blurTimerRef = useRef<number | null>(null);

  useEffect(() => {
    setIsInputActive(false);
    setInputValue("");
  }, [articleUUID]);

  useEffect(() => {
    return () => {
      if (blurTimerRef.current !== null) {
        window.clearTimeout(blurTimerRef.current);
      }
    };
  }, []);

  const currentTagsSet = new Set(currentTags.map((tag) => tag.tag));
  const addableTags = availableTags.filter((tag) => !currentTagsSet.has(tag.tag));
  const normalizedInputValue = isInputActive ? inputValue : "";
  const visibleSuggestions = addableTags
    .filter((tag) => !normalizedInputValue || tag.tag.includes(normalizedInputValue))
    .slice(0, 8);
  const exactTag = addableTags.find((tag) => tag.tag === normalizedInputValue);
  const hasSuggestions = isInputActive && visibleSuggestions.length > 0;

  if (!isInputActive && currentTags.length === 0 && addableTags.length === 0) {
    return <></>;
  }

  function clearBlurTimer(): void {
    if (blurTimerRef.current !== null) {
      window.clearTimeout(blurTimerRef.current);
      blurTimerRef.current = null;
    }
  }

  function openInput(): void {
    clearBlurTimer();
    setIsInputActive(true);
    setInputValue("");
  }

  function closeInput(): void {
    setIsInputActive(false);
    setInputValue("");
  }

  async function addTag(tagSlug: string): Promise<void> {
    try {
      await onAddTag(articleUUID, tagSlug);
      closeInput();
    } catch {
      // The parent owns the visible mutation error; keep the input open for retry.
    }
  }

  const isTitleVariant = variant === "title";
  const renderedCurrentTags = currentTags.map((tag) => {
    const removeMutationKey = `${articleUUID}:${tag.tag}:remove`;
    const renderedRemoveButton = (
      <button
        type="button"
        className={`tag-chip-remove ${isTitleVariant ? "title-tag-remove" : ""}`.trim()}
        onClick={() => {
          void onRemoveTag(articleUUID, tag.tag);
        }}
        disabled={mutationKey === removeMutationKey}
        aria-label={`Remove ${tag.tag} tag`}
      >
        <X className="tag-chip-remove-icon" aria-hidden="true" />
      </button>
    );

    if (isTitleVariant) {
      return (
        <TitleTag key={tag.tag} className="tag-chip-removable" style={tagChipStyle(tag)}>
          {tag.tag}
          {renderedRemoveButton}
        </TitleTag>
      );
    }

    return (
      <span key={tag.tag} className="tag-chip tag-chip-removable" style={tagChipStyle(tag)}>
        {tag.tag}
        {renderedRemoveButton}
      </span>
    );
  });

  const renderedTagRow = (
    <div
      className={`member-tag-row ${isTitleVariant ? "member-tag-row-title" : ""}`.trim()}
      aria-label="Article tags"
    >
      {renderedCurrentTags}
      {addableTags.length > 0 ? (
        isTitleVariant ? (
          <TitleActionButton ariaLabel="Add article tag" title="Add tag" onClick={openInput}>
            <Plus className="title-action-icon" aria-hidden="true" />
          </TitleActionButton>
        ) : (
          <button
            type="button"
            className="member-tag-add-button"
            aria-label="Add article tag"
            title="Add tag"
            onClick={openInput}
          >
            <Plus className="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        )
      ) : null}
    </div>
  );
  const renderedInputBody = (
    <>
      {isTitleVariant ? null : renderedCurrentTags}
      <div className="member-tag-input-wrap">
        <Input
          value={normalizedInputValue}
          onFocus={() => {
            clearBlurTimer();
            setIsInputActive(true);
          }}
          onBlur={() => {
            blurTimerRef.current = window.setTimeout(closeInput, 120);
          }}
          onChange={(event) => {
            setIsInputActive(true);
            setInputValue(normalizeTagInput(event.target.value));
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              if (exactTag) {
                void addTag(exactTag.tag);
              }
              return;
            }
            if (event.key === "Escape") {
              closeInput();
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
              const addMutationKey = `${articleUUID}:${tag.tag}:add`;
              return (
                <button
                  key={tag.tag}
                  type="button"
                  className="member-tag-suggestion"
                  style={tagChipStyle(tag)}
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => {
                    void addTag(tag.tag);
                  }}
                  disabled={mutationKey === addMutationKey}
                  role="option"
                >
                  {tag.tag}
                </button>
              );
            })}
          </div>
        ) : null}
      </div>
    </>
  );
  const renderedInput = isTitleVariant ? (
    <TitleTagInput className="member-tag-input-shell-title" onMouseDown={clearBlurTimer}>
      {renderedInputBody}
    </TitleTagInput>
  ) : (
    <div
      className="member-tag-input-shell is-active"
      aria-label="Article tag input"
      onMouseDown={clearBlurTimer}
    >
      {renderedInputBody}
    </div>
  );

  if (isTitleVariant) {
    return (
      <div className="member-tag-tools member-tag-tools-title">
        {renderedTagRow}
        {isInputActive ? renderedInput : null}
      </div>
    );
  }

  return <div className="member-tag-tools">{!isInputActive ? renderedTagRow : renderedInput}</div>;
}

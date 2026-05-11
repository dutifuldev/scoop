import { useEffect, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { ChevronDown, LogOut, Settings2, X } from "lucide-react";

import { buildFeedMetaText, formatCalendarDay, formatCount } from "../lib/viewerFormat";
import type { StoryListItem, Tag } from "../types";
import { Button } from "./ui/button";
import { DayPickerPopover } from "./ui/day-picker-popover";
import { Input } from "./ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";

interface StoriesListPanelProps {
  searchInput: string;
  from: string;
  to: string;
  selectedTag: string;
  availableTags: Tag[];
  activeLang: string;
  translatingStoryUUIDs: string[];
  totalItems: number;
  loadedItems: number;
  selectedStoryUUID: string;
  stories: StoryListItem[];
  isLoading: boolean;
  isFetchingNextPage: boolean;
  hasNextPage: boolean;
  error: string;
  showAdvancedSearch: boolean;
  onSearchInputChange: (value: string) => void;
  onShowAdvancedSearchChange: (value: boolean) => void;
  onFromChange: (value: string) => void;
  onToChange: (value: string) => void;
  onTagChange: (value: string) => void;
  onLoadNextPage: () => void;
  onSelectStory: (storyUUID: string) => void;
  currentUsername: string;
  onOpenSettings: () => void;
  onLogout: () => void;
}

export function StoriesListPanel({
  searchInput,
  from,
  to,
  selectedTag,
  availableTags,
  activeLang,
  translatingStoryUUIDs,
  totalItems,
  loadedItems,
  selectedStoryUUID,
  stories,
  isLoading,
  isFetchingNextPage,
  hasNextPage,
  error,
  showAdvancedSearch,
  onSearchInputChange,
  onShowAdvancedSearchChange,
  onFromChange,
  onToChange,
  onTagChange,
  onLoadNextPage,
  onSelectStory,
  currentUsername,
  onOpenSettings,
  onLogout,
}: StoriesListPanelProps): JSX.Element {
  const listRef = useRef<HTMLDivElement | null>(null);
  const loadTriggerRef = useRef<HTMLDivElement | null>(null);
  const showTimestampInFeed = searchInput.trim() !== "";
  const translatingStoryUUIDSet = new Set(translatingStoryUUIDs);
  const [isUserMenuOpen, setIsUserMenuOpen] = useState(false);

  useEffect(() => {
    if (!hasNextPage || isLoading || isFetchingNextPage || Boolean(error)) {
      return;
    }

    const root = listRef.current;
    const target = loadTriggerRef.current;
    if (!root || !target) {
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          onLoadNextPage();
        }
      },
      {
        root,
        rootMargin: "220px 0px",
        threshold: 0,
      },
    );

    observer.observe(target);
    return () => observer.disconnect();
  }, [error, hasNextPage, isFetchingNextPage, isLoading, onLoadNextPage, stories.length]);

  return (
    <section className="panel card story-feed-panel">
      <div className="feed-search">
        <div className="finder-row">
          <div className="finder-input-wrap">
            <span className="finder-icon" aria-hidden="true">
              /
            </span>
            <Input
              value={searchInput}
              onChange={(event) => onSearchInputChange(event.target.value)}
              type="text"
              placeholder="Search stories (title or URL)"
              aria-label="Search stories"
              className="finder-input !h-auto !border-0 !bg-transparent !p-0 focus-visible:!ring-0"
            />
            {searchInput ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="finder-clear-btn !h-6 !w-6"
                onClick={() => onSearchInputChange("")}
                aria-label="Clear search"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </Button>
            ) : null}
          </div>

          <Button
            className={`finder-advanced-icon ${showAdvancedSearch ? "active" : ""}`.trim()}
            type="button"
            variant="ghost"
            size="icon"
            aria-label="Toggle advanced search"
            aria-pressed={showAdvancedSearch}
            onClick={() => onShowAdvancedSearchChange(!showAdvancedSearch)}
          >
            <Settings2 className="h-5 w-5" strokeWidth={1.9} aria-hidden="true" />
          </Button>
        </div>

        {showAdvancedSearch ? (
          <div className="advanced-row">
            <label className="field field-small">
              <span>From</span>
              <div className="advanced-field-control">
                <DayPickerPopover
                  value={from}
                  onChange={onFromChange}
                  align="start"
                  trigger={
                    <Button type="button" variant="outline" className="advanced-day-trigger">
                      <span className="advanced-day-trigger-label">
                        {from ? formatCalendarDay(from) : "Pick start day"}
                      </span>
                    </Button>
                  }
                />
                {from ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="advanced-day-clear"
                    onClick={() => onFromChange("")}
                    aria-label="Clear From date"
                  >
                    <X className="h-4 w-4" aria-hidden="true" />
                  </Button>
                ) : null}
              </div>
            </label>

            <label className="field field-small">
              <span>To</span>
              <div className="advanced-field-control">
                <DayPickerPopover
                  value={to}
                  onChange={onToChange}
                  align="start"
                  trigger={
                    <Button type="button" variant="outline" className="advanced-day-trigger">
                      <span className="advanced-day-trigger-label">
                        {to ? formatCalendarDay(to) : "Pick end day"}
                      </span>
                    </Button>
                  }
                />
                {to ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="advanced-day-clear"
                    onClick={() => onToChange("")}
                    aria-label="Clear To date"
                  >
                    <X className="h-4 w-4" aria-hidden="true" />
                  </Button>
                ) : null}
              </div>
            </label>

            <label className="field field-small">
              <span>Tag</span>
              <Select
                value={selectedTag || "__all_tags__"}
                onValueChange={(value) => onTagChange(value === "__all_tags__" ? "" : value)}
              >
                <SelectTrigger className="advanced-tag-trigger" aria-label="Filter by tag">
                  <SelectValue placeholder="All tags" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all_tags__">All tags</SelectItem>
                  {availableTags.map((tag) => (
                    <SelectItem key={tag.tag} value={tag.tag}>
                      {tag.tag}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
          </div>
        ) : null}
      </div>

      <div className="stories-list-wrap">
        <div ref={listRef} className="stories-list">
          {isLoading ? <p className="muted">Loading stories...</p> : null}
          {!isLoading && error ? <p className="muted">{error}</p> : null}
          {!isLoading && !error && stories.length === 0 ? (
            <p className="muted">No stories match this filter.</p>
          ) : null}

          {!isLoading && !error
            ? stories.map((story) => {
                const originalTitle = (story.original_title || story.title || "").trim();
                const translatedTitle = (story.translated_title || "").trim();
                const showTranslated = activeLang !== "" && translatedTitle !== "";
                const displayTitle = showTranslated ? translatedTitle : originalTitle;
                const isTranslatingStory = translatingStoryUUIDSet.has(story.story_uuid);

                return (
                  <article
                    key={story.story_uuid}
                    className={`story-card ${story.story_uuid === selectedStoryUUID ? "active" : ""}`.trim()}
                    onClick={() => onSelectStory(story.story_uuid)}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        onSelectStory(story.story_uuid);
                      }
                    }}
                  >
                    <header className="story-title-row">
                      <h3 className="story-title">{displayTitle || "(untitled)"}</h3>
                      <div className="story-title-flags">
                        {isTranslatingStory ? (
                          <span
                            className="story-translating-indicator"
                            aria-label="Translation in progress"
                          >
                            <span className="story-translating-dot" aria-hidden="true" />
                            Translating
                          </span>
                        ) : null}
                        {showTranslated ? (
                          <span
                            className="story-translation-badge"
                            aria-label={`Translated to ${activeLang}`}
                          >
                            [{activeLang.toUpperCase()}]
                          </span>
                        ) : null}
                      </div>
                    </header>
                    {story.tags && story.tags.length > 0 ? (
                      <div className="story-tag-row" aria-label="Story tags">
                        {story.tags.map((tag) => (
                          <span
                            key={tag.tag}
                            className="tag-chip tag-chip-small"
                            style={
                              tag.color
                                ? ({ "--tag-color": tag.color } as CSSProperties)
                                : undefined
                            }
                          >
                            {tag.tag}
                          </span>
                        ))}
                      </div>
                    ) : null}
                    <p className="story-meta">{buildFeedMetaText(story, showTimestampInFeed)}</p>
                  </article>
                );
              })
            : null}

          {!isLoading && !error ? (
            <div ref={loadTriggerRef} className="stories-load-sentinel" aria-hidden="true" />
          ) : null}
          {isFetchingNextPage ? (
            <p className="muted stories-status">Loading more stories...</p>
          ) : null}
          {!isFetchingNextPage && !hasNextPage && stories.length > 0 ? (
            <p className="muted stories-status">Reached the end of this story feed.</p>
          ) : null}
        </div>

        <div className="feed-count-overlay" aria-live="polite">
          {formatCount(Math.min(loadedItems, totalItems))}/{formatCount(totalItems)}
        </div>
      </div>

      <div className="sidebar-user">
        <button
          type="button"
          className="sidebar-user-trigger"
          onClick={() => {
            setIsUserMenuOpen((previous) => !previous);
          }}
          aria-label="Open user menu"
        >
          <span className="sidebar-user-bubble">
            {(currentUsername || "U").slice(0, 1).toUpperCase()}
          </span>
          <span className="sidebar-user-name">{currentUsername || "User"}</span>
          <ChevronDown
            className={`sidebar-user-chevron ${isUserMenuOpen ? "open" : ""}`.trim()}
            aria-hidden="true"
          />
        </button>

        {isUserMenuOpen ? (
          <div className="sidebar-user-menu" role="menu" aria-label="User menu">
            <button
              type="button"
              className="sidebar-user-menu-item"
              onClick={() => {
                setIsUserMenuOpen(false);
                onOpenSettings();
              }}
            >
              <Settings2 className="h-4 w-4" aria-hidden="true" />
              Settings
            </button>

            <button
              type="button"
              className="sidebar-user-menu-item"
              onClick={() => {
                setIsUserMenuOpen(false);
                onLogout();
              }}
            >
              <LogOut className="h-4 w-4" aria-hidden="true" />
              Log out
            </button>
          </div>
        ) : null}
      </div>
    </section>
  );
}

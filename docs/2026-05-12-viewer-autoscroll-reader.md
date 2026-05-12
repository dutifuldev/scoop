---
title: Viewer Autoscroll Reader
author: Scoop Maintainers
date: 2026-05-12
---

# Viewer Autoscroll Reader

## Summary

The long-term viewer model should behave like a Twitter-style reader.

The right pane owns reading position. The left pane reflects that position. Clicking a story in the left pane scrolls the right pane to that story.

The production-ready version is a virtualized, route-aware reader with scroll-spy, progressive loading, and deliberate URL/session restoration.

## Goals

- Let the user read continuously in the right pane.
- Highlight the current story in the left pane while the right pane scrolls.
- Let left-pane clicks scroll the right pane to the matching story.
- Keep scrolling smooth with many stories.
- Restore reading position after navigation, refresh, or back/forward.
- Avoid noisy URL/history updates while the user scrolls.

## Non-Goals

- Do not make every scroll tick push a history entry.
- Do not require all story details to render at once.
- Do not make the left pane the source of truth for reading position.
- Do not mix explicit click selection with passive scroll position.

## Core Model

The right pane is the reader.

Each rendered story section in the right pane has a stable anchor:

- `data-story-uuid`
- optional `data-item-uuid` for later article-level anchors

The left pane is an index.

It receives the active story UUID from the reader and highlights the matching story card.

Clicking a left-pane story card expresses intent. It should request or reveal the target story section, then scroll the right pane to it.

## State

Use separate state for separate meanings:

- `selectedStoryUUID`: route or explicit user intent.
- `activeStoryUUID`: story currently visible in the reader.
- `scrollTargetStoryUUID`: temporary programmatic scroll target after a click.
- `loadedStoryUUIDs`: story details currently loaded into the reader.
- `visibleStoryWindow`: virtualized or progressive range around the active story.

`activeStoryUUID` should update from scroll-spy.

`selectedStoryUUID` should update from explicit navigation and settled reader position, not every scroll event.

## Scroll-Spy

Use `IntersectionObserver` with the right pane scroll container as the root.

Observe each story section.

Pick the active story by one of these rules:

- section closest to the vertical center of the reader
- or highest intersection ratio above a threshold

Center-line selection is usually better for long story sections because a huge section can otherwise dominate the intersection ratio for too long.

When `activeStoryUUID` changes:

- update left-pane highlight immediately
- optionally debounce URL replacement
- do not trigger a new story click action

## Click-To-Scroll

When the user clicks a story in the left pane:

1. Set `scrollTargetStoryUUID`.
2. Ensure the target story detail is loaded or included in the virtual window.
3. Wait until the target section exists in the DOM.
4. Call `scrollIntoView({ block: "start", behavior: "smooth" })`.
5. During the programmatic scroll, let scroll-spy update highlight but avoid fighting the target.
6. Clear `scrollTargetStoryUUID` once the target is active or after a timeout.

Explicit clicks may use `pushState`.

Passive scroll-derived updates should use debounced `replaceState`.

## Progressive Loading

The first version can progressively render story detail sections:

- render the selected story plus nearby stories
- add a sentinel near the bottom
- when the sentinel intersects, load the next page of stories
- keep enough overscan above and below the viewport

This matches the Twitter-style behavior without needing full virtualization immediately.

## Virtualization

The long-term version should virtualize the reader.

Use virtualization when the right pane can contain enough rendered story details to make DOM size, image/media loading, or preview fetching expensive.

Requirements:

- stable item keys by story UUID
- measured or estimated row heights
- overscan large enough to avoid visible pop-in
- anchor preservation when older content is inserted above the viewport
- scroll-to-index or scroll-to-key support

If virtualization makes text selection, browser find, or anchor links worse, keep progressive rendering until the dataset size proves virtualization is needed.

## URL And Session Restoration

Persist reader state in `sessionStorage` before navigation:

- active story UUID
- scroll offset
- loaded window or visible count
- timestamp

Expire restored state after a short window, for example 30 minutes.

On restore:

1. Read the saved state.
2. Load enough story sections to contain the saved active story.
3. Render the target window.
4. Restore scroll offset.
5. Clear the saved state.

Support `?story=<uuid>` or route params for direct entry, but avoid writing a new history entry for every passive active-story change.

## Suggested Phases

### Phase 1: Scroll-Spy In Current Detail Pane

- Add refs/data attributes for right-pane member/story sections.
- Add a `useScrollSpy` hook.
- Track `activeStoryUUID`.
- Highlight the left pane based on `activeStoryUUID`.

### Phase 2: Left Click Scrolls Right

- Add a right-pane scroll target API.
- On left card click, scroll to the matching right-pane section.
- Add a programmatic-scroll guard.

### Phase 3: Continuous Reader

- Render multiple story sections in the right pane.
- Load details progressively.
- Add a bottom sentinel for additional story sections.

### Phase 4: Restoration

- Save active story, scroll offset, and loaded count/window.
- Restore on back/forward and refresh.
- Debounce route replacement for scroll-derived active story changes.

### Phase 5: Virtualization

- Add virtualization only when progressive rendering is not enough.
- Preserve anchor behavior and reader continuity.

## Implementation Notes

Keep the scroll logic in hooks rather than embedding it directly in `StoriesListPanel` or `StoryDetailPanel`.

Suggested modules:

- `useReaderScrollSpy`
- `useReaderScrollRestore`
- `ViewerReadingPane`
- `ViewerStorySection`

The left pane should stay dumb: it receives `activeStoryUUID` and calls `onSelectStory`.

The right pane should own the scroll container ref, observed section refs, and restoration behavior.

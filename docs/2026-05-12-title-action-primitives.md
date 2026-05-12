---
title: Title Action Primitives
author: Scoop Maintainers
date: 2026-05-12
---

# Title Action Primitives

## Summary

Story and article titles can have nearby actions: source links, Discord links, tag chips, tag add buttons, and future controls.

These controls should not each define their own size, radius, padding, icon size, hover state, or wrapping behavior. They should use one small local primitive system for title-adjacent actions.

This is not a new design system. It is a focused geometry contract for controls that sit inline with titles.

## Problem

The current title controls can drift visually:

- Discord source buttons can have a different circle size than normal source links.
- Link icons and plus buttons can sit on slightly different baselines.
- Tag chips can feel detached from the title controls.
- Each local fix adds more hand-tuned spacing.

The result is fragile. Every new title-adjacent action risks creating another slightly different button.

## Goal

Make all title-adjacent controls mechanically consistent.

All title actions should share:

- action hit area
- icon size
- chip height
- border radius
- focus ring
- hover background
- horizontal gap
- wrapping behavior

The title text, source link, tag chips, and plus button should behave as one wrapped title cluster.

## Non-Goals

- Do not replace shadcn components globally.
- Do not build a full app-wide design system.
- Do not rewrite tag behavior.
- Do not rewrite Discord link behavior.
- Do not make article/member expand buttons copy story links.

## Component Shape

Add local primitives for this exact UI pattern:

- `TitleActions`: layout wrapper for title-adjacent controls.
- `TitleActionButton`: fixed-size icon button.
- `TitleActionLink`: fixed-size icon link, including normal source links and Discord links.
- `TitleTag`: compact tag chip that shares the title-action height.
- `TitleTagInput`: input shell that opens below the title action row when the plus button is clicked.

These primitives can live near the story-detail components unless the same pattern appears elsewhere.

## Geometry Contract

Use CSS variables or Tailwind-backed classes with one source of truth:

- action size: `1.35rem`
- icon size: `0.75rem`
- tag height: `1.35rem`
- group gap: `0.375rem`
- radius: full for icon actions, compact pill for tags

Example shape:

```css
.title-action {
  width: var(--title-action-size);
  height: var(--title-action-size);
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.title-action-icon {
  width: var(--title-action-icon-size);
  height: var(--title-action-icon-size);
}
```

Tags should share the same vertical rhythm:

```css
.title-tag {
  min-height: var(--title-action-size);
  padding-inline: 0.35rem;
}
```

## Layout Rules

The title row should be a wrapping inline cluster:

1. title text
2. source link icon
3. existing tags
4. plus button

The source link and plus button should align because they use the same `TitleActionLink` / `TitleActionButton` geometry.

When the plus button is clicked, the tag input should appear below the title cluster. It should align to the tag action area, not push the title text around unpredictably.

## Accessibility Rules

Every icon-only action needs:

- a stable accessible name
- visible focus state
- hover state that does not change dimensions
- no layout shift when feedback text changes

For story-title copy, the title text itself can be a button styled as text. Its accessible name can change briefly from `Copy story link` to `Copied story link`.

## Implementation Checklist

- [ ] Create the title action primitives.
- [ ] Convert normal source links to `TitleActionLink`.
- [ ] Convert Discord source links to `TitleActionLink`.
- [ ] Convert title-level tag plus buttons to `TitleActionButton`.
- [ ] Convert title-level tag chips to `TitleTag`.
- [ ] Keep tag input hidden until plus is clicked.
- [ ] Make wrapping work for long titles and multiple tags.
- [ ] Verify detail panel and reader panel.
- [ ] Verify combinations: source only, tags only, source plus tags, Discord plus tags, multiple tags, no tags.
- [ ] Add tests for structure and copy behavior where practical.

## Acceptance Criteria

- Title-adjacent source icons, Discord icons, plus buttons, and tag chips are visually aligned.
- No title action uses one-off width, height, radius, or icon-size rules.
- No hover or focus state changes the element dimensions.
- The source icon still opens the original source.
- Clicking the story title still copies the story URL.
- Clicking the plus still opens the tag input below the title cluster.

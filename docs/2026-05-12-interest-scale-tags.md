---
title: Interest Scale Tags
author: Bob <dutifulbob@gmail.com>
date: 2026-05-12
---

# Interest Scale Tags

## Summary

Scoop can use `i0`, `i1`, `i2`, and `i3` as a compact interest scale for posts and articles.

These tags are for editorial or research interest. They are not operational priority tags, and they should not imply urgency, severity, or production impact.

## Tags

- `i0`: highest interest. Must review, track, or consider for follow-up.
- `i1`: strong interest. Worth reading and likely worth acting on later.
- `i2`: moderate interest. Useful context, but not a main focus.
- `i3`: low interest. Keep for recall, filtering, or completeness.

## Rules

Use only one interest-scale tag per article.

Use the lowest number for the highest level of interest. This mirrors priority labels like `p0`, where `0` means most important.

Do not create variants like `interest-high`, `i-high`, `i4`, or `i-critical` unless the scale is deliberately changed later.

Do not mix interest with topic tags. For example, an article can have both `openclaw` and `i1`, but `i1` only says how interesting the article is, not what the article is about.

## When to Use Each Level

Use `i0` when missing the item would be costly for the current research direction.

Use `i1` when the item is clearly relevant and likely to matter.

Use `i2` when the item may help with context, examples, or later synthesis.

Use `i3` when the item is marginal but still worth keeping tagged.

## CLI Creation

Create these as normal controlled tags through the CLI:

- `scoop tags create i0`
- `scoop tags create i1`
- `scoop tags create i2`
- `scoop tags create i3`

The article UI should only attach or remove these existing tags. It should not create new interest-scale tags from the article screen.

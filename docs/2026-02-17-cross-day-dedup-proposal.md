---
title: Cross-Day Thematic Dedup Proposal
author: Scoop Maintainers
date: 2026-02-17
---

# Cross-Day Thematic Dedup Proposal

## Summary
Cross-day thematic duplication is happening because digest selection is stateless across days. The dedup layer correctly keeps distinct stories separate, but the curator receives only “today’s new stories” and has no memory of yesterday’s coverage. The fix should be in the digest stage, not in story dedup. The most robust solution is a combination of:
- **Backend memory + similarity gating** (use existing `digest_runs`/`digest_entries`, plus vector similarity to recently published stories), and
- **Prompt-level memory** (provide yesterday’s digest list and explicit anti-repetition guidance).

This proposal outlines a low-risk path that uses existing tables, minimal schema changes, and the embedding infrastructure already in use.

## Current State (from codebase)
- **Ingestion flow**: `fetch_news.py` (often `--hours 26`) → `scoop ingest` → `run-once` (normalize/embed/dedup).
- **Dedup logic**: `internal/pipeline/service.go` uses exact/lexical/semantic matching. Default semantic auto-merge is `cosine >= 0.935` (or `>= 0.965` override), gray zone `0.89–0.935`. Distinct stories intentionally remain distinct.
- **Digest selection (legacy)**: daily cron queries `dedup_events` with `decision='new_story'` and `created_at >= CURRENT_DATE` and passes those stories to the AI curator.
- **State tables exist but unused**: `story_topic_state`, `digest_runs`, and `digest_entries` are present per `IMPLEMENTATION_PLAN.md`, but currently empty.
- **Topic assignment/digest worker**: planned but not implemented in Go.

## Root Cause
The daily digest is built from today’s `new_story` rows only, with no awareness of **what was selected yesterday**. The LLM makes independent choices across days, so thematically similar stories reappear in consecutive digests even when they are distinct canonical stories.

## Goals
- Reduce cross-day thematic repetition while **preserving** distinct stories.
- Allow legitimate “update” coverage when there is a major development.
- Keep dedup thresholds stable; don’t merge distinct stories just to reduce repetition.
- Use existing schema (`digest_runs`, `digest_entries`, `story_topic_state`) to persist digest memory.

## Non-Goals
- Re-tuning dedup thresholds to merge thematically similar stories.
- Large-scale schema rewrites or replacing the LLM curator.

## Options Considered

### 1) Prompt-only memory
**What**: Provide yesterday’s digest headlines to the curator with instructions to avoid repeating themes unless a major update.

**Pros**:
- Fastest to deploy; no DB changes.
- Improves output diversity immediately.

**Cons**:
- Non-deterministic; the LLM may still select similar themes.
- No audit trail or metrics for duplication reduction.

### 2) Backend memory + similarity gating
**What**: Use `digest_runs`/`digest_entries` to record what was published, then filter or downrank new candidates if they are **semantically similar** to recently published stories (by topic) using existing embeddings.

**Pros**:
- Deterministic, measurable, and reusable across curators.
- Uses existing `document_embeddings` + pgvector (`<=>`) already in production.

**Cons**:
- Requires building the digest query logic (planned but not yet implemented in Go).

### 3) Theme clustering layer
**What**: Build a story “theme cluster” (e.g., via embeddings) and enforce a cooldown per theme in digest selection.

**Pros**:
- Most robust long-term solution.

**Cons**:
- Requires new tables/state and operational complexity.

### 4) Combination (recommended)
Use backend memory + similarity gating as the primary guardrail, and add prompt memory as a soft constraint. This yields deterministic suppression with LLM flexibility to include legitimate “updates.”

## Recommended Approach (Combination)

### A) Persist digest runs + entries (use existing tables)
Start writing to `digest_runs` and `digest_entries` **even if the curator remains in OpenClaw**.
- Create a lightweight digest-run wrapper (or a SQL script) that:
  - Inserts a `digest_runs` row for `(topic_id, run_date, window_start_utc, window_end_utc)`.
  - Inserts `digest_entries` for every candidate story with status `included` or `excluded` and a reason.
  - Stores ranking/score when available.

This creates durable memory for what was published per day, per topic.

### B) Add semantic “recent coverage” gating
When building today’s candidate set, compute similarity to **recently published** stories for the same topic.
- Source “recent” stories from `digest_entries` where `status='included'`, joining to `digest_runs` for the last N days (e.g., 1–3 days).
- Use each story’s `representative_document_id` embedding from `document_embeddings`.
- For each candidate story, compute `max_cosine_similarity` to recent stories.
- Apply a threshold (e.g., `>= 0.86–0.90`) to **flag as “recently covered”**.
- Downrank or exclude those candidates unless they are marked as significant updates.

This is **not** story dedup. It’s a digest-level novelty filter.

### C) Provide “yesterday’s digest” context to the curator
Include the recent digest list (titles + links) and the similarity flags for each candidate. Prompt guidance:
- Avoid selecting stories marked “recently covered” unless the item is a major update.
- If selected, label as “Update” and explain why it’s new.

### D) Update `story_topic_state` on successful publish
After posting:
- Update `story_topic_state.first_published_at`, `last_published_at`, and `publish_count` for included stories.
- Use `suppressed` only for manual overrides (optional).

This enables stable per-topic reporting and future logic like “don’t re-publish the same story within N days.”

## Suggested Query Shape (Conceptual)
Not implementation — just the data shape needed:

- **Candidates**: stories that are new for the window
  - Use `dedup_events.decision='new_story'`
  - Prefer windowing by `stories.last_seen_at` or `documents.published_at` (not `dedup_events.created_at`) to avoid backfill skew.

- **Recent digest set**: last N days of published stories for the topic
  - `digest_entries` + `digest_runs` + `stories` + `document_embeddings`.

- **Similarity**:
  - `recent_max_cosine = MAX(1 - (candidate_embedding <=> recent_embedding))`
  - Attach the nearest recent story title + id for curator context.

## Why Not Change Dedup Thresholds?
Dedup is currently accurate: it’s preventing **false merges** and keeping distinct stories separate. Tightening thresholds to merge “themes” would erase real story differences and reduce data quality. The fix should sit **after** dedup — in the digest selection stage.

## Implementation Outline (No Code)
1. **Short-term (1–2 days)**
   - Add prompt memory: include yesterday’s digest headlines in the curator prompt.
   - Add a thin DB write step to populate `digest_runs` and `digest_entries` for included stories.

2. **Mid-term (3–5 days)**
   - Build a digest candidate query that computes `recent_max_cosine` to recent digest entries.
   - Exclude/downrank candidates above threshold, but allow “Update” exceptions.
   - Persist both included and excluded candidates with reason in `digest_entries`.

3. **Long-term (optional)**
   - Introduce explicit “theme clusters” to allow more stable cooldowns and reporting.

## Metrics to Track
- Percent of digest entries with `recent_max_cosine >= threshold`.
- Count of “Update” items per digest.
- Cross-day overlap rate (story similarity to previous day’s digest).

## Risks and Mitigations
- **Risk**: Over-filtering breaks coverage during slow news days.
  - **Mitigation**: Use a soft downrank first; allow “Update” exceptions.
- **Risk**: Embedding similarity too aggressive.
  - **Mitigation**: Use a lower threshold than dedup (`0.86–0.90`) and validate with a small manual sample.
- **Risk**: Timezone skew in “today” window.
  - **Mitigation**: Anchor digest windows to topic timezone (`Europe/Berlin`) rather than `CURRENT_DATE` defaults.

## Bottom Line
The best fix is **digest memory + semantic novelty gating**, backed by `digest_runs`/`digest_entries`, plus prompt-level memory for the curator. This preserves true story distinctness while reducing repetitive daily themes.

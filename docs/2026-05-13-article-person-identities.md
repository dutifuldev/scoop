---
title: Article Person Identities
author: Scoop Maintainers
date: 2026-05-13
---

# Article Person Identities

## Summary

Scoop should associate articles with external person identities.

For now, there is no canonical `persons` table and no story-level person table.

Use two tables:

```text
news.person_identities
news.article_person_identities
```

Stories can show people by aggregating identities from their articles.

## Core Rule

One `person_identities` row is one external account, profile, or person reference.

Examples:

```text
id://discord/id/123456789012345678?handle=alice
id://x/id/987654321?handle=alice-ai
id://github/handle/octocat
```

Do not model cross-platform person clustering yet. If the same real-world person has both a Discord account and an X account, those are separate person identities for now.

## Tables

### `news.person_identities`

Canonical external identities.

```text
person_identity_id
person_identity_uuid
provider
provider_user_id
handle
identity_ref
archived_at
created_at
updated_at
```

Field meanings:

- `provider`: lowercase provider key, such as `discord`, `x`, or `github`
- `provider_user_id`: stable provider ID when known
- `handle`: visible provider handle when known
- `identity_ref`: canonical `id://...` ref
- `archived_at`: hides the identity from normal picker results without deleting history

The canonical identity ref format is defined separately in [Identity Ref Scheme](./2026-05-13-identity-ref-scheme.md).

### `news.article_person_identities`

Article-to-identity associations.

```text
article_id
person_identity_id
created_by_user_id
created_at
```

Field meanings:

- `article_id`: target article
- `person_identity_id`: associated external identity
- `created_by_user_id`: user who created the association when known

## Bot Flow

The bot should use the CLI to associate identities with articles.

The primary command is:

```text
scoop articles add-person <article_uuid> <identity_ref>
```

Example:

```text
scoop articles add-person \
  320c586f-ab36-4a8f-970a-465b7c3a7904 \
  'id://discord/id/123456789012345678?handle=alice'
```

The bot should not manually create identities before associating them with articles.

The CLI handler should run one backend operation:

1. Parse and normalize `identity_ref`.
2. Validate the provider and identity fields.
3. Upsert `news.person_identities`.
4. Update mutable metadata, such as `handle`.
5. Insert or update `news.article_person_identities`.
6. Return the attached identity.

This operation should be idempotent. Sending the same article and identity twice should not create duplicate associations.

## CLI Shape

Make `articles` a real command namespace.

Article listing:

```text
scoop articles list [--collection ...] [--from ...] [--to ...] [--limit ...] [--format table|json]
```

Backwards compatibility:

```text
scoop articles [flags]
```

should continue to behave like:

```text
scoop articles list [flags]
```

Article person identity commands:

```text
scoop articles add-person <article_uuid> <identity_ref> [--format table|json]
scoop articles remove-person <article_uuid> <identity_ref-or-person_identity_uuid>
scoop articles list-people <article_uuid> [--format table|json]
```

Identity management commands can live under `person-identities`:

```text
scoop person-identities list [--include-archived] [--format table|json]
scoop person-identities show <identity_ref-or-person_identity_uuid> [--format table|json]
scoop person-identities archive <identity_ref-or-person_identity_uuid>
scoop person-identities unarchive <identity_ref-or-person_identity_uuid>
```

Do not use `scoop person-identities add-article`. The article association command should be article-first.

## Identity Upsert

Stable ID refs are preferred.

For `id` refs:

```text
id://discord/id/123456789012345678?handle=alice
```

Upsert by:

```text
provider + provider_user_id
```

For `handle` refs:

```text
id://github/handle/octocat
```

Upsert by:

```text
provider + normalized handle
```

Handle-only refs are weaker than stable ID refs. They are still allowed because some providers or inputs may not expose stable IDs.

## Constraints

Recommended constraints:

```text
unique person_identity_uuid
unique identity_ref
unique(provider, provider_user_id) where provider_user_id is not null
unique(provider, handle) where provider_user_id is null and handle is not null
unique(article_id, person_identity_id)
```

The `unique(article_id, person_identity_id)` constraint keeps article associations idempotent.

## API Shape

HTTP APIs are not the first write surface. The CLI is.

If the app later needs HTTP support, it should mirror the CLI shape.

Article association:

```text
POST /api/v1/articles/:article_uuid/person-identities
DELETE /api/v1/articles/:article_uuid/person-identities/:person_identity_uuid
```

List identities:

```text
GET /api/v1/person-identities?q=<query>
```

The list endpoint should search:

- handle
- provider user ID
- identity ref

## UI Shape

Article identities should render as article bylines, not as tags.

Do not show people in the left story list for now. The left pane should stay focused on story title, tags, and timing. Person identity rendering belongs in the right pane where the article content is visible.

The byline should use the same positioning model as an X/Twitter card. The existing blog's Twitter-like timeline layout is the visual and structural reference: avatar column on the left, one main content column on the right, byline at the top of that main column, and article text continuing in the same column. Use that layout model directly in Scoop; do not document private local paths.

That means:

- the article content uses an avatar column plus a main content column
- the byline sits at the top of the main column, before the article text
- avatar, handle, provider icon, dot, and date align on one compact row
- the row wraps cleanly on narrow screens without separating the avatar from the text column

Target examples:

```text
[avatar] Display Name @handle · 5h
[avatar] @handle [discord icon] · May 11
[avatar] @handle [provider icon] · Jun 1, 2025
```

Implementation shape:

```text
ArticleByline
  identities
  publishedAt
  source
```

Render the first deterministic article identity as the primary byline identity. Multiple identities can be represented later with a compact `+N` affordance if needed; the common v1 case is one external author per article.

Rendering rules:

- Prefer display name when the model has one in the future.
- For now, display name is usually absent, so show `@handle`.
- Show the handle first. Do not prefix bylines or chips with provider text like `DISCORD`.
- For Discord identities, show a small borderless Discord icon immediately after the handle.
- For other providers, use the provider icon when one exists; otherwise omit provider decoration.
- If there is no handle, fall back to a compact provider/user label, not raw `id://...`.
- Never show raw identity refs in normal reader UI.
- Use a small circular avatar placeholder generated from display name initials, then handle initials, then provider initials.
- Keep existing add/remove controls separate from the read-only byline presentation.

Date formatting should be centralized in a frontend helper, for example `formatBylineDate(date, now)`.

Use Twitter-style compact dates:

```text
now       under 1 minute
12m       under 1 hour
5h        under 24 hours
May 11    same calendar year
Jun 1, 2025  different calendar year
```

Adding an identity should feel similar to adding a tag:

1. Click a small add button.
2. Type or paste an identity ref.
3. Save.

If the identity ref is new, the backend can create it automatically.

The normal UI can show the created identity immediately. It does not need a separate person-management screen for the first version.

## Non-Goals

Do not add these yet:

- `news.persons`
- story-level person associations
- cross-platform person clustering
- identity merge/split UI
- automatic identity extraction

Those can be added later if the product needs them.

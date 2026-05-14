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
avatar_url
identity_ref
archived_at
created_at
updated_at
```

Field meanings:

- `provider`: lowercase provider key, such as `discord`, `x`, or `github`
- `provider_user_id`: stable provider ID when known
- `handle`: visible provider handle when known
- `avatar_url`: current profile picture URL when known
- `identity_ref`: canonical `id://...` ref
- `archived_at`: hides the identity from normal picker results without deleting history

The canonical identity ref format is defined separately in [Identity Ref Scheme](./2026-05-13-identity-ref-scheme.md).

Profile pictures are display metadata on the identity row. Do not create a separate avatar table.

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
scoop person-identities refresh-avatar <identity_ref-or-person_identity_uuid> [--format table|json]
scoop person-identities refresh-avatars [--provider discord|github] [--format table|json]
scoop person-identities archive <identity_ref-or-person_identity_uuid>
scoop person-identities unarchive <identity_ref-or-person_identity_uuid>
```

Do not use `scoop person-identities add-article`. The article association command should be article-first.

Avatar refresh commands update `person_identities.avatar_url` in place. They should not change article associations.

For Discord identities, the refresh command should:

1. Require `provider = discord`.
2. Require `provider_user_id`.
3. Fetch the Discord user object with backend credentials.
4. Read the current avatar hash.
5. Store the derived CDN URL in `avatar_url`.

The stored URL should use a modest size suitable for bylines:

```text
https://cdn.discordapp.com/avatars/{discord_user_id}/{avatar_hash}.webp?size=128
```

If Discord returns no custom avatar, keep `avatar_url` empty and let the UI use initials.

For GitHub identities, the refresh command should:

1. Require `provider = github`.
2. Require `handle`.
3. Fetch the public GitHub user object.
4. Read `avatar_url`.
5. Store that URL in `avatar_url`.

The request should use the official GitHub user endpoint:

```text
GET https://api.github.com/users/{handle}
```

If `GITHUB_TOKEN` is present in the environment, send it as a bearer token. The token is optional because public GitHub profiles can be resolved without authentication. If GitHub returns no avatar URL, keep `avatar_url` empty and let the UI use initials.

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

The left pane owns story navigation. It is okay for the story title to appear only there. The right pane should not render a separate story title/header component; it should render article entries for reading.

The right pane should use an article timeline model. The existing blog's Twitter-like timeline layout is the visual and structural reference: avatar rail on the left, one main content column on the right, byline at the top of that main column, article title below the byline, and article text continuing in the same column. Use that layout model directly in Scoop; do not document private local paths.

Target layout:

```text
left pane
  story title, tags, timing, active state

right pane
  StoryArticleTimeline
    StoryArticleEntry
      timeline rail
        avatar
        vertical connector when another article follows
      content column
        ArticleByline
        ArticleTitleRow
        ArticleBodyPreview
        Show more
```

Each article entry should look like:

```text
[avatar]  @handle [provider icon] · May 11
   |      Article title [source action] [tags]
   |      Truncated article content...
   |      Show more
```

For a single-article story, do not render an extra top-level story title in the right pane. Render the article title only in the article entry, below the author line and above the content.

For a multi-article story, render the entries as a timeline. Entries before the last one should show a vertical connector line in the avatar rail. The line should visually connect profile circles like the existing blog timeline.

Render the first deterministic article identity as the primary byline identity. Multiple identities can be represented later with a compact `+N` affordance if needed; the common v1 case is one external author per article.

Byline rendering rules:

- Prefer display name when the model has one in the future.
- For now, display name is usually absent, so show `@handle`.
- Show the handle first. Do not prefix bylines or chips with provider text like `DISCORD`.
- For Discord identities, show a small borderless Discord icon immediately after the handle.
- The Discord icon in the author line and any Discord source action in the title line should use the same visual size and equivalent vertical centering.
- For other providers, use the provider icon when one exists; otherwise omit provider decoration.
- If there is no handle, fall back to a compact provider/user label, not raw `id://...`.
- Never show raw identity refs in normal reader UI.
- If `avatar_url` is present, render it in the circular avatar.
- If `avatar_url` is empty or the image fails to load, use a circular avatar placeholder generated from display name initials, then handle initials, then provider initials.
- Humans should not add or remove person identities through the normal reader UI. Identity assignment is handled by the CLI, backend jobs, or automation.

Date formatting should be centralized in a frontend helper, for example `formatBylineDate(date, now)`.

Use Twitter-style compact dates and show only one visible date in the byline:

```text
now       under 1 minute
12m       under 1 hour
5h        under 24 hours
May 11    same calendar year
Jun 1, 2025  different calendar year
```

Additional pipeline metadata belongs in the date hover text, not inline in the byline. The tooltip can include:

```text
Published May 10, 15:10 · Ingested May 11, 14:03 · Decision: new story
```

Article title behavior:

- Clicking the article title copies the article canonical URL when available.
- If the article has no canonical URL, clicking the title copies the story URL.
- The source icon remains the explicit "open source" action.
- The title should show temporary copied feedback after a successful copy.

Article content behavior:

- Remove article-level chevron/collapse buttons.
- Show article content as a truncated preview by default, similar to Twitter.
- Put a `Show more` button on a new line below truncated content.
- Expanding should happen inline for that article entry.
- A `Show less` action is optional but acceptable for long entries.

## Non-Goals

Do not add these yet:

- `news.persons`
- story-level person associations
- cross-platform person clustering
- identity merge/split UI
- automatic identity extraction
- avatar history or downloaded avatar storage

Those can be added later if the product needs them.

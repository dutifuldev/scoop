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

Article views should show person identity chips near the article title or metadata.

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

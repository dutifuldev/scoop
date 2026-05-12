---
title: Identity Ref Scheme
author: Scoop Maintainers
date: 2026-05-13
---

# Identity Ref Scheme

## Summary

An identity ref is a single string that identifies an external account or profile.

The canonical form is:

```text
id://<provider>/id/<provider_user_id>?handle=<handle>
id://<provider>/handle/<handle>
```

Examples:

```text
id://discord/id/123456789012345678
id://discord/id/123456789012345678?handle=alice
id://discord/handle/alice
id://x/id/987654321?handle=alice-ai
id://x/handle/alice-ai
id://github/handle/octocat
```

## Purpose

The scheme gives Scoop one compact, typed way to refer to provider identities.

It is independent of any storage model. It does not define people, authors, article links, story links, permissions, or database tables.

## Meaning

An identity ref identifies a provider identity, not a human being.

One human can have many identity refs:

```text
id://discord/id/123456789012345678?handle=alice
id://x/handle/alice-ai
id://github/handle/alice
```

Two identity refs should only be treated as the same identity when their canonical provider and stable provider ID match, or when a trusted resolver explicitly says they are the same.

## Syntax

```text
identity-ref = "id://" provider "/" kind "/" value [ "?" query ]
provider     = lowercase provider key
kind         = "id" | "handle"
value        = provider-specific ID or handle
query        = URL query parameters
```

Allowed providers should be lowercase ASCII keys:

```text
[a-z][a-z0-9-]*
```

Initial provider keys:

```text
discord
x
github
bluesky
mastodon
linkedin
youtube
website
email
```

Provider keys are product-neutral identifiers for parsing. Display names can differ in the UI.

## Kinds

### `id`

Use `id` when the provider has a stable native user ID.

```text
id://discord/id/123456789012345678
id://x/id/987654321
```

An `id` ref is the strongest form. If a handle is also known, store it as a query hint:

```text
id://discord/id/123456789012345678?handle=alice
```

The stable ID remains canonical. The handle is mutable metadata.

### `handle`

Use `handle` when only the visible account handle is known.

```text
id://github/handle/octocat
id://x/handle/alice-ai
```

A `handle` ref is weaker than an `id` ref because handles can be renamed, transferred, reused, or displayed differently by provider.

## Query Parameters

Supported query parameters:

- `handle`: current known visible handle for an `id` ref
- `label`: optional human-readable label when the provider ref alone is hard to recognize

Unknown query parameters must be preserved when round-tripping, but they must not affect identity equality unless a future version of this document says so.

## Normalization

When accepting an identity ref:

1. Lowercase the scheme, provider, and kind.
2. Strip one leading `@` from handles.
3. Percent-decode path values before validation.
4. Re-encode path values when producing canonical strings.
5. Sort query parameters when producing canonical strings.
6. Drop empty query parameters.

These inputs normalize to the same ref:

```text
id://x/handle/alice
id://x/handle/@alice
```

Canonical output:

```text
id://x/handle/alice
```

## Equality

Identity equality is intentionally strict.

These are equal:

```text
id://discord/id/123456789012345678
id://discord/id/123456789012345678?handle=alice
```

These are not automatically equal:

```text
id://discord/id/123456789012345678
id://discord/handle/alice
```

The first has a stable provider ID. The second only has a handle. A resolver can link them, but the string scheme itself does not.

## Validation

All identity refs must:

- use the `id://` scheme
- include provider, kind, and value
- use `id` or `handle` as kind
- have no empty path segments
- have no whitespace in provider, kind, or value

Provider-specific validation can be stricter. For example, Discord IDs are numeric snowflakes, while GitHub handles have their own character rules.

## External Inputs

Normal URLs can be accepted by UI or CLI tools as convenience input, but they are not canonical identity refs.

Examples:

```text
https://x.com/alice
https://github.com/octocat
https://discord.com/users/123456789012345678
```

Tools may translate these into canonical identity refs when the provider and identity can be parsed unambiguously.

## Non-Goals

This document does not define:

- how people are stored
- how articles or stories link to people
- how identity refs are resolved
- how conflicts are merged or split
- how permissions work
- whether unknown identity refs can create new people

Those belong in separate product and schema documents.

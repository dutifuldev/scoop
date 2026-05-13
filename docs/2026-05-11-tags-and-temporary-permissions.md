---
title: Tags and Temporary Permissions
date: 2026-05-11
---

# Tags and Temporary Permissions

## Summary

Scoop should support manual article tags, but tags should come from a controlled list.

Users should be able to add or remove existing tags on articles. Creating, renaming, archiving, or deleting tags should be done through the CLI, not through the normal app UI.

Tags have one canonical value. The same value is used for identity, display, CLI input, API input, and filtering.

Scoop does not currently have real admin roles. It has users and sessions, plus a bootstrap user named `admin`, but the app does not distinguish admin users from normal users yet. The first version of tags can avoid this problem by making tag-list management CLI-only.

## Goals

- Let users tag articles with existing tags.
- Prevent uncontrolled tag creation.
- Keep tag management separate from normal article tagging.
- Allow a coding agent to perform restricted CLI actions only when explicitly granted temporary permission.
- Record who changed article tags and who changed the tag list.

## Non-Goals

- No auto-tagging.
- No model suggestions.
- No free-text tag creation from the article screen.
- No permanent agent admin access.

## Product Rules

Tags are attached to articles.

Stories can show tags by aggregating the tags from their articles.

Normal users can add or remove existing tags on articles.

The app UI should not create new tags.

The CLI manages the tag list.

CLI-only tag-list actions are:

- create a tag
- rename a tag
- change a tag color
- change a tag highlight color
- archive or unarchive a tag
- delete an unused tag

Used tags should usually be archived instead of deleted, so old articles still make sense.

Tag values use GitHub-style plain names:

- lowercase letters
- numbers
- single dashes
- no spaces
- no underscores
- no leading or trailing dash

Examples:

- `openclaw`
- `needs-review`
- `metal-news`

## CLI Shape

Tag list management should start in the CLI.

Suggested commands:

- `scoop tags list`
- `scoop tags create <tag> [--color <hex>] [--highlight-color <hex>]`
- `scoop tags rename <old-tag> <new-tag>`
- `scoop tags update <tag> [--color <hex>] [--highlight-color <hex>]`
- `scoop tags archive <tag>`
- `scoop tags unarchive <tag>`
- `scoop tags delete <tag>`

`delete` should fail if the tag is attached to any article.

Article tag operations can also exist in the CLI:

- `scoop tags add-article <article_uuid> <tag>`
- `scoop tags remove-article <article_uuid> <tag>`

## Permission Model

The first version does not need app-level admin roles for tag-list management because tag-list management is CLI-only.

The CLI can rely on local operator access at first. That keeps the first version small and avoids inventing app admins before the app needs them.

If agents need to run restricted CLI commands, use temporary grants.

Each temporary grant should include:

- who receives the grant
- who created the grant
- the permission name
- when it expires
- optional reason
- created timestamp
- revoked timestamp

Example permission names:

- `manage_tags`
- `manage_collection_settings`

When restricted app actions are added later, add roles to users.

Add roles to users:

- `admin`
- `user`
- `agent`

Restricted API calls, when added, should check either:

- the user has the required permanent role, or
- the current user/session has an unexpired grant for the required permission.

## Agent Access

Agents should not have permanent tag-management power.

A local operator can grant an agent a restricted permission for a short window, such as 5 minutes.

Example:

> Grant agent `manage_tags` until `2026-05-11T12:05:00Z`.

During that window, the agent can perform CLI tag-list actions.

After the grant expires, the same CLI commands should fail again.

The system should log every restricted action taken through a temporary grant.

## Suggested Tables

### `news.tags`

Allowed tag definitions.

- `tag_id`
- `tag_uuid`
- `tag`
- `description`
- `color`
- `highlight_color`
- `archived_at`
- `created_at`
- `updated_at`

`color` controls the tag pill itself.

`highlight_color` is optional. When set, it controls the visual emphasis applied to articles and stories carrying the tag. When empty, the tag does not change article or story styling.

`highlight_color` should be a normal hex color. It should not encode a variant, priority, CSS class, or arbitrary style payload.

### `news.article_tags`

Tags applied to articles.

- `article_id`
- `tag_id`
- `created_by_user_id`
- `created_at`

### `news.user_roles`

Permanent user roles.

- `user_id`
- `role`
- `created_at`

### `news.temporary_permission_grants`

Short-lived permission grants.

- `grant_id`
- `grant_uuid`
- `granted_to_user_id`
- `granted_by_user_id`
- `permission`
- `reason`
- `expires_at`
- `created_at`
- `revoked_at`

### `news.audit_events`

Audit trail for restricted actions.

- `event_id`
- `actor_user_id`
- `permission`
- `temporary_grant_id`
- `action`
- `target_type`
- `target_id`
- `details`
- `created_at`

## API Shape

Tag list reads:

- `GET /api/v1/tags`

Tag list management should stay CLI-only in the first version.

These API routes should be deferred until the app has roles or temporary grants:

- `POST /api/v1/tags`
- `PATCH /api/v1/tags/:tag`
- `POST /api/v1/tags/:tag/archive`
- `POST /api/v1/tags/:tag/unarchive`
- `DELETE /api/v1/tags/:tag`

Article tagging:

- `POST /api/v1/articles/:article_uuid/tags`
- `DELETE /api/v1/articles/:article_uuid/tags/:tag`

Temporary grants:

- `POST /api/v1/temporary-permission-grants`
- `DELETE /api/v1/temporary-permission-grants/:grant_uuid`

## UI Shape

The article detail view should let users add or remove existing tags.

Adding a tag should be a compact typeahead input: click the tag area, type, and select an existing tag. Pressing Enter can add an exact existing match.

The article detail view should not allow creating new tags.

Do not add a Tags settings page in the first version.

If tag-list management is later added to the app, gate it behind roles or temporary grants.

Tag pills should use `color`.

Article and story emphasis should use `highlight_color`.

If an article has multiple tags with `highlight_color`, use the first one after sorting by tag value. This keeps the rule deterministic without adding priority or variant fields.

Stories should derive their highlight from the tags on their articles using the same rule over the aggregated tag set.

The first UI treatment should be restrained: a colored left rail and, if needed, a very subtle background tint derived from `highlight_color`. Do not hardcode special behavior for specific tag names.

## Implementation Order

1. Add tag tables.
2. Add CLI commands for tag-list management.
3. Add article tag add/remove APIs using existing tags only.
4. Show article tags in story detail.
5. Add tag filters to story search.
6. Add audit events for tag-list and article-tag changes.
7. Add roles and temporary permission grants only when restricted app actions are needed.

## Open Questions

- Should normal users be allowed to remove tags added by other users?
- Should archived tags stay visible on old articles by default?
- Should temporary grants apply only to CLI commands, or eventually to API sessions too?

package db

import (
	"context"
	"crypto/sha256"
	"os"
	"strings"
	"testing"
	"time"

	"horse.fit/scoop/internal/config"
	"horse.fit/scoop/internal/textmetrics"
)

func TestPoolUpdateStoryIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	storyUUID := "00000000-0000-4000-8000-000000000101"
	createIntegrationStory(t, pool, storyUUID, "old title", "openclaw", now.Add(-time.Hour))

	title := "  New Story Title  "
	status := " MERGED "
	collection := " Metal News "
	rawURL := "https://Example.com/Story"
	if err := pool.UpdateStory(ctx, storyUUID, UpdateStoryOptions{
		Title:      &title,
		Status:     &status,
		Collection: &collection,
		URL:        &rawURL,
	}, now); err != nil {
		t.Fatalf("UpdateStory() error = %v", err)
	}

	var got Story
	if err := pool.gdb.WithContext(ctx).Where("story_uuid = ?", storyUUID).First(&got).Error; err != nil {
		t.Fatalf("query story: %v", err)
	}
	if got.CanonicalTitle != "New Story Title" {
		t.Fatalf("title = %q", got.CanonicalTitle)
	}
	if got.Status != "merged" {
		t.Fatalf("status = %q", got.Status)
	}
	if got.Collection != "metal news" {
		t.Fatalf("collection = %q", got.Collection)
	}
	if got.CanonicalURL == nil || *got.CanonicalURL != "https://example.com/Story" {
		t.Fatalf("canonical url = %v", got.CanonicalURL)
	}
	if !got.UpdatedAt.After(now.Add(-time.Hour)) {
		t.Fatalf("updated_at = %s, want later than original timestamp", got.UpdatedAt)
	}
}

func TestPoolUpdateArticleIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	articleUUID := "00000000-0000-4000-8000-000000000201"
	createIntegrationArticle(t, pool, articleUUID, "old title", "old body text", now.Add(-time.Hour))

	title := "  New Article Title  "
	source := " github "
	collection := " OpenClaw "
	rawURL := "https://GitHub.com/owner/repo/issues/123"
	if err := pool.UpdateArticle(ctx, articleUUID, UpdateArticleOptions{
		Title:      &title,
		Source:     &source,
		Collection: &collection,
		URL:        &rawURL,
	}, now); err != nil {
		t.Fatalf("UpdateArticle() error = %v", err)
	}

	var got Article
	if err := pool.gdb.WithContext(ctx).Where("article_uuid = ?", articleUUID).First(&got).Error; err != nil {
		t.Fatalf("query article: %v", err)
	}
	if got.NormalizedTitle != "new article title" {
		t.Fatalf("title = %q", got.NormalizedTitle)
	}
	if got.Source != "github" {
		t.Fatalf("source = %q", got.Source)
	}
	if got.Collection != "openclaw" {
		t.Fatalf("collection = %q", got.Collection)
	}
	if got.CanonicalURL == nil || *got.CanonicalURL != "https://github.com/owner/repo/issues/123" {
		t.Fatalf("canonical url = %v", got.CanonicalURL)
	}
	if got.SourceDomain == nil || *got.SourceDomain != "github.com" {
		t.Fatalf("source domain = %v", got.SourceDomain)
	}
	if got.TokenCount != 6 {
		t.Fatalf("token count = %d", got.TokenCount)
	}
	if got.TitleSimhash == nil {
		t.Fatal("title simhash is nil")
	}
	if !got.UpdatedAt.After(now.Add(-time.Hour)) {
		t.Fatalf("updated_at = %s, want later than original timestamp", got.UpdatedAt)
	}
}

func TestPoolUpdateArticleReturnsNoRowsForMissingArticle(t *testing.T) {
	pool := newIntegrationPool(t)
	source := "github"
	err := pool.UpdateArticle(
		context.Background(),
		"00000000-0000-4000-8000-000000000299",
		UpdateArticleOptions{Source: &source},
		time.Now(),
	)
	if err != ErrNoRows {
		t.Fatalf("UpdateArticle() error = %v, want ErrNoRows", err)
	}
}

func TestPoolUpdateArticleRejectsInvalidTitleAfterLock(t *testing.T) {
	pool := newIntegrationPool(t)
	articleUUID := "00000000-0000-4000-8000-000000000298"
	createIntegrationArticle(t, pool, articleUUID, "old title", "old body", time.Now().Add(-time.Hour))

	title := "  "
	err := pool.UpdateArticle(
		context.Background(),
		articleUUID,
		UpdateArticleOptions{Title: &title},
		time.Now(),
	)
	if err == nil || err.Error() != "title must not be empty" {
		t.Fatalf("UpdateArticle() error = %v, want title validation error", err)
	}
}

func TestPoolUpdateArticleRejectsInvalidInputBeforeDatabase(t *testing.T) {
	t.Parallel()

	source := " "
	pool := &Pool{}
	if err := pool.UpdateArticle(context.Background(), " ", UpdateArticleOptions{Source: &source}, time.Now()); err == nil || err.Error() != "article UUID is required" {
		t.Fatalf("blank UUID error = %v", err)
	}
	if err := pool.UpdateArticle(context.Background(), "article-uuid", UpdateArticleOptions{}, time.Now()); err == nil || err.Error() != "at least one update field is required" {
		t.Fatalf("empty update error = %v", err)
	}
	if err := pool.UpdateArticle(context.Background(), "article-uuid", UpdateArticleOptions{Source: &source}, time.Now()); err == nil || err.Error() != "source must not be empty" {
		t.Fatalf("source error = %v", err)
	}
}

func TestPoolSoftDeleteAndRestoreStoryIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	storyUUID := "00000000-0000-4000-8000-000000000301"
	createIntegrationStory(t, pool, storyUUID, "story", "openclaw", now.Add(-time.Hour))

	affected, err := pool.SoftDeleteStory(ctx, storyUUID, now)
	if err != nil {
		t.Fatalf("SoftDeleteStory() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("SoftDeleteStory() affected = %d, want 1", affected)
	}
	affected, err = pool.RestoreStory(ctx, storyUUID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("RestoreStory() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("RestoreStory() affected = %d, want 1", affected)
	}
}

func TestPoolSoftDeleteAndRestoreArticleIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	articleUUID := "00000000-0000-4000-8000-000000000302"
	createIntegrationArticle(t, pool, articleUUID, "article", "body", now.Add(-time.Hour))

	affected, err := pool.SoftDeleteArticle(ctx, articleUUID, now)
	if err != nil {
		t.Fatalf("SoftDeleteArticle() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("SoftDeleteArticle() affected = %d, want 1", affected)
	}
	affected, err = pool.RestoreArticle(ctx, articleUUID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("RestoreArticle() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("RestoreArticle() affected = %d, want 1", affected)
	}
}

func TestPoolDeletedStateMutationsRejectBlankUUID(t *testing.T) {
	t.Parallel()

	pool := &Pool{}
	if _, err := pool.SoftDeleteStory(context.Background(), " ", time.Now()); err == nil || err.Error() != "story UUID is required" {
		t.Fatalf("SoftDeleteStory() error = %v", err)
	}
	if _, err := pool.SoftDeleteArticle(context.Background(), " ", time.Now()); err == nil || err.Error() != "article UUID is required" {
		t.Fatalf("SoftDeleteArticle() error = %v", err)
	}
	if _, err := pool.RestoreStory(context.Background(), " ", time.Now()); err == nil || err.Error() != "story UUID is required" {
		t.Fatalf("RestoreStory() error = %v", err)
	}
	if _, err := pool.RestoreArticle(context.Background(), " ", time.Now()); err == nil || err.Error() != "article UUID is required" {
		t.Fatalf("RestoreArticle() error = %v", err)
	}
}

func TestPoolSoftDeleteCollectionIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 12, 30, 0, 0, time.UTC)
	createIntegrationArticle(t, pool, "00000000-0000-4000-8000-000000000303", "article", "body", now.Add(-time.Hour))
	createIntegrationStory(t, pool, "00000000-0000-4000-8000-000000000304", "story", "openclaw", now.Add(-time.Hour))

	result, err := pool.SoftDeleteCollection(ctx, " OpenClaw ", now)
	if err != nil {
		t.Fatalf("SoftDeleteCollection() error = %v", err)
	}
	if result.RawArrivals != 1 || result.Articles != 1 || result.Stories != 1 {
		t.Fatalf("SoftDeleteCollection() = %+v, want one row per table", result)
	}
	if _, err := pool.SoftDeleteCollection(ctx, " ", now); err == nil || err.Error() != "collection is required" {
		t.Fatalf("blank collection error = %v", err)
	}
}

func TestPoolSoftDeleteBeforeIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 12, 45, 0, 0, time.UTC)
	oldTime := now.Add(-48 * time.Hour)
	newTime := now.Add(-time.Hour)
	createIntegrationArticle(t, pool, "00000000-0000-4000-8000-000000000305", "old article", "body", oldTime)
	createIntegrationStory(t, pool, "00000000-0000-4000-8000-000000000306", "old story", "openclaw", oldTime)
	createIntegrationArticle(t, pool, "00000000-0000-4000-8000-000000000307", "new article", "body", newTime)
	createIntegrationStory(t, pool, "00000000-0000-4000-8000-000000000308", "new story", "openclaw", newTime)

	result, err := pool.SoftDeleteBefore(ctx, now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("SoftDeleteBefore() error = %v", err)
	}
	if result.RawArrivals != 1 || result.Articles != 1 || result.Stories != 1 {
		t.Fatalf("SoftDeleteBefore() = %+v, want old rows only", result)
	}
	if _, err := pool.SoftDeleteBefore(ctx, time.Time{}, now); err == nil || err.Error() != "before time is required" {
		t.Fatalf("zero before error = %v", err)
	}
}

func TestPoolTagLifecycleIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC)
	articleUUID := "00000000-0000-4000-8000-000000000401"
	storyUUID := "00000000-0000-4000-8000-000000000402"
	createIntegrationArticle(t, pool, articleUUID, "tagged article", "body", now.Add(-time.Hour))
	story := createIntegrationStory(t, pool, storyUUID, "tagged story", "openclaw", now.Add(-time.Hour))
	linkIntegrationStoryArticle(t, pool, story.StoryID, articleUUID, now)

	color := "#ff0000"
	highlight := "#ffd34d"
	tag, err := pool.CreateTag(ctx, UpsertTagOptions{
		Slug:           "i0",
		Color:          &color,
		HighlightColor: &highlight,
	}, now)
	if err != nil {
		t.Fatalf("CreateTag() error = %v", err)
	}
	if tag.Tag != "i0" || tag.Color == nil || *tag.Color != color {
		t.Fatalf("created tag = %#v", tag)
	}
	activeTags, err := pool.ListTags(ctx, false)
	if err != nil {
		t.Fatalf("ListTags(active) error = %v", err)
	}
	if len(activeTags) != 1 || activeTags[0].Tag != "i0" {
		t.Fatalf("active tags = %#v, want i0", activeTags)
	}

	description := "important"
	renamed := "interest"
	updated, err := pool.UpdateTag(ctx, "i0", UpdateTagOptions{
		NewSlug:     &renamed,
		Description: &description,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("UpdateTag() error = %v", err)
	}
	if updated.Tag != "interest" || updated.Description == nil || *updated.Description != description {
		t.Fatalf("updated tag = %#v", updated)
	}

	if err := pool.AddArticleTag(ctx, articleUUID, "interest", nil, now); err != nil {
		t.Fatalf("AddArticleTag() error = %v", err)
	}
	articleTags, err := pool.ListTagsForArticleUUIDs(ctx, []string{articleUUID, articleUUID, " "})
	if err != nil {
		t.Fatalf("ListTagsForArticleUUIDs() error = %v", err)
	}
	if got := articleTags[articleUUID]; len(got) != 1 || got[0].Tag != "interest" {
		t.Fatalf("article tags = %#v", articleTags)
	}
	storyTags, err := pool.ListTagsForStoryIDs(ctx, []int64{story.StoryID, story.StoryID, 0})
	if err != nil {
		t.Fatalf("ListTagsForStoryIDs() error = %v", err)
	}
	if got := storyTags[story.StoryID]; len(got) != 1 || got[0].Tag != "interest" {
		t.Fatalf("story tags = %#v", storyTags)
	}

	archived, err := pool.SetTagArchived(ctx, "interest", true, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("SetTagArchived() error = %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("archived tag has nil ArchivedAt")
	}
	activeTags, err = pool.ListTags(ctx, false)
	if err != nil {
		t.Fatalf("ListTags(active after archive) error = %v", err)
	}
	if len(activeTags) != 0 {
		t.Fatalf("active tags after archive = %#v, want none", activeTags)
	}
	allTags, err := pool.ListTags(ctx, true)
	if err != nil {
		t.Fatalf("ListTags(include archived) error = %v", err)
	}
	if len(allTags) != 1 || allTags[0].ArchivedAt == nil {
		t.Fatalf("all tags = %#v, want archived tag", allTags)
	}
	if err := pool.RemoveArticleTag(ctx, articleUUID, "interest", nil); err != nil {
		t.Fatalf("RemoveArticleTag() error = %v", err)
	}
	if err := pool.DeleteTag(ctx, "interest"); err != nil {
		t.Fatalf("DeleteTag() error = %v", err)
	}
}

func TestPoolTagMutationErrorsIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 13, 30, 0, 0, time.UTC)
	articleUUID := "00000000-0000-4000-8000-000000000451"
	createIntegrationArticle(t, pool, articleUUID, "tag error article", "body", now.Add(-time.Hour))

	if _, err := pool.UpdateTag(ctx, "missing-tag", UpdateTagOptions{Description: stringPtr("missing")}, now); err != ErrNoRows {
		t.Fatalf("UpdateTag(missing) error = %v, want ErrNoRows", err)
	}
	if err := pool.DeleteTag(ctx, "missing-tag"); err != ErrNoRows {
		t.Fatalf("DeleteTag(missing) error = %v, want ErrNoRows", err)
	}

	if _, err := pool.CreateTag(ctx, UpsertTagOptions{Slug: "used-tag"}, now); err != nil {
		t.Fatalf("CreateTag(used) error = %v", err)
	}
	if err := pool.AddArticleTag(ctx, articleUUID, "used-tag", nil, now); err != nil {
		t.Fatalf("AddArticleTag(used) error = %v", err)
	}
	if err := pool.DeleteTag(ctx, "used-tag"); err == nil || !strings.Contains(err.Error(), "attached to 1 articles") {
		t.Fatalf("DeleteTag(used) error = %v, want attached error", err)
	}

	if _, err := pool.CreateTag(ctx, UpsertTagOptions{Slug: "duplicate-a"}, now); err != nil {
		t.Fatalf("CreateTag(duplicate-a) error = %v", err)
	}
	if _, err := pool.CreateTag(ctx, UpsertTagOptions{Slug: "duplicate-b"}, now); err != nil {
		t.Fatalf("CreateTag(duplicate-b) error = %v", err)
	}
	newSlug := "duplicate-b"
	if _, err := pool.UpdateTag(ctx, "duplicate-a", UpdateTagOptions{NewSlug: &newSlug}, now); err == nil {
		t.Fatalf("UpdateTag(rename duplicate) error = nil, want unique constraint failure")
	}
}

func TestPoolPersonIdentityLifecycleIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	articleUUID := "00000000-0000-4000-8000-000000000501"
	storyUUID := "00000000-0000-4000-8000-000000000502"
	createIntegrationArticle(t, pool, articleUUID, "identity article", "body", now.Add(-time.Hour))
	story := createIntegrationStory(t, pool, storyUUID, "identity story", "openclaw", now.Add(-time.Hour))
	linkIntegrationStoryArticle(t, pool, story.StoryID, articleUUID, now)

	assertPersonIdentityUpsertLifecycle(t, ctx, pool, now)
	added := addAndAssertArticlePersonIdentity(t, ctx, pool, articleUUID, story.StoryID, now)
	assertPersonIdentityAvatarAndArchive(t, ctx, pool, added, now)
	assertRemoveArticlePersonIdentity(t, ctx, pool, articleUUID, added.IdentityRef)
}

func assertPersonIdentityUpsertLifecycle(t *testing.T, ctx context.Context, pool *Pool, now time.Time) {
	t.Helper()

	identity, err := pool.UpsertPersonIdentity(ctx, "id://discord/id/12345?handle=FunCracker", now)
	if err != nil {
		t.Fatalf("UpsertPersonIdentity() error = %v", err)
	}
	if identity.Provider != "discord" || identity.ProviderUserID == nil || *identity.ProviderUserID != "12345" {
		t.Fatalf("identity = %#v", identity)
	}
	if identity.Handle == nil || *identity.Handle != "funcracker" {
		t.Fatalf("handle = %v, want funcracker", identity.Handle)
	}

	updated, err := pool.UpsertPersonIdentity(ctx, "id://discord/id/12345?handle=Peter", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("second UpsertPersonIdentity() error = %v", err)
	}
	if updated.PersonIdentityID != identity.PersonIdentityID {
		t.Fatalf("upsert created duplicate identity: got %d want %d", updated.PersonIdentityID, identity.PersonIdentityID)
	}
	if updated.Handle == nil || *updated.Handle != "peter" {
		t.Fatalf("updated handle = %v, want peter", updated.Handle)
	}
}

func addAndAssertArticlePersonIdentity(t *testing.T, ctx context.Context, pool *Pool, articleUUID string, storyID int64, now time.Time) *PersonIdentityRecord {
	t.Helper()

	added, err := pool.AddArticlePersonIdentity(ctx, articleUUID, "id://github/handle/octocat", nil, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("AddArticlePersonIdentity() error = %v", err)
	}
	if added.Provider != "github" || added.Handle == nil || *added.Handle != "octocat" {
		t.Fatalf("added identity = %#v", added)
	}

	articlePeople, err := pool.ListPersonIdentitiesForArticleUUID(ctx, articleUUID)
	if err != nil {
		t.Fatalf("ListPersonIdentitiesForArticleUUID() error = %v", err)
	}
	if len(articlePeople) != 1 || articlePeople[0].IdentityRef != "id://github/handle/octocat" {
		t.Fatalf("article identities = %#v", articlePeople)
	}

	byArticle, err := pool.ListPersonIdentitiesForArticleUUIDs(ctx, []string{articleUUID, articleUUID, " "})
	if err != nil {
		t.Fatalf("ListPersonIdentitiesForArticleUUIDs() error = %v", err)
	}
	if got := byArticle[articleUUID]; len(got) != 1 || got[0].Provider != "github" {
		t.Fatalf("article identity map = %#v", byArticle)
	}

	byStory, err := pool.ListPersonIdentitiesForStoryIDs(ctx, []int64{storyID, storyID, 0})
	if err != nil {
		t.Fatalf("ListPersonIdentitiesForStoryIDs() error = %v", err)
	}
	if got := byStory[storyID]; len(got) != 1 || got[0].IdentityRef != "id://github/handle/octocat" {
		t.Fatalf("story identity map = %#v", byStory)
	}

	listed, err := pool.ListPersonIdentities(ctx, "octo", false, 500)
	if err != nil {
		t.Fatalf("ListPersonIdentities() error = %v", err)
	}
	if len(listed) != 1 || listed[0].PersonIdentityID != added.PersonIdentityID {
		t.Fatalf("listed identities = %#v", listed)
	}
	return added
}

func assertPersonIdentityAvatarAndArchive(t *testing.T, ctx context.Context, pool *Pool, added *PersonIdentityRecord, now time.Time) {
	t.Helper()

	avatarURL := "  https://avatars.githubusercontent.com/u/583231?v=4  "
	withAvatar, err := pool.SetPersonIdentityAvatarURL(ctx, added.IdentityRef, &avatarURL, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("SetPersonIdentityAvatarURL() error = %v", err)
	}
	if withAvatar.AvatarURL == nil || *withAvatar.AvatarURL != "https://avatars.githubusercontent.com/u/583231?v=4" {
		t.Fatalf("avatar url = %v", withAvatar.AvatarURL)
	}

	archived, err := pool.SetPersonIdentityArchived(ctx, added.PersonIdentityUUID, true, now.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("SetPersonIdentityArchived() error = %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("archived identity has nil ArchivedAt")
	}

	activeOnly, err := pool.ListPersonIdentities(ctx, "", false, 0)
	if err != nil {
		t.Fatalf("ListPersonIdentities(active) error = %v", err)
	}
	for _, record := range activeOnly {
		if record.PersonIdentityID == added.PersonIdentityID {
			t.Fatalf("archived identity appeared in active list: %#v", activeOnly)
		}
	}

	includedArchived, err := pool.ListPersonIdentities(ctx, "octocat", true, 1)
	if err != nil {
		t.Fatalf("ListPersonIdentities(archived) error = %v", err)
	}
	if len(includedArchived) != 1 || includedArchived[0].ArchivedAt == nil {
		t.Fatalf("archived list = %#v", includedArchived)
	}

	fetched, err := pool.GetPersonIdentity(ctx, added.IdentityRef)
	if err != nil {
		t.Fatalf("GetPersonIdentity() error = %v", err)
	}
	if fetched.PersonIdentityUUID != added.PersonIdentityUUID {
		t.Fatalf("fetched identity UUID = %s, want %s", fetched.PersonIdentityUUID, added.PersonIdentityUUID)
	}
}

func assertRemoveArticlePersonIdentity(t *testing.T, ctx context.Context, pool *Pool, articleUUID string, identityRef string) {
	t.Helper()

	if err := pool.RemoveArticlePersonIdentity(ctx, articleUUID, identityRef, nil); err != nil {
		t.Fatalf("RemoveArticlePersonIdentity() error = %v", err)
	}
	afterRemove, err := pool.ListPersonIdentitiesForArticleUUID(ctx, articleUUID)
	if err != nil {
		t.Fatalf("ListPersonIdentitiesForArticleUUID(after remove) error = %v", err)
	}
	if len(afterRemove) != 0 {
		t.Fatalf("article identities after remove = %#v", afterRemove)
	}
}

func TestPoolReadQueriesIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 15, 0, 0, 0, time.UTC)
	firstArticle := createIntegrationArticle(t, pool, "00000000-0000-4000-8000-000000000601", "openclaw alpha", "body", now.Add(-2*time.Hour))
	secondArticle := createIntegrationArticle(t, pool, "00000000-0000-4000-8000-000000000602", "openclaw beta", "body", now.Add(-time.Hour))
	story := createIntegrationStory(t, pool, "00000000-0000-4000-8000-000000000603", "OpenClaw story", "openclaw", now.Add(-2*time.Hour))
	if err := pool.gdb.Model(&story).Updates(map[string]any{
		"representative_article_id": secondArticle.ArticleID,
		"canonical_url":             "https://example.com/story",
	}).Error; err != nil {
		t.Fatalf("update story representative: %v", err)
	}
	setIntegrationArticleReadFields(t, pool, firstArticle.ArticleID, "https://example.com/alpha", "example.com", now.Add(-2*time.Hour))
	setIntegrationArticleReadFields(t, pool, secondArticle.ArticleID, "https://example.com/beta", "example.com", now.Add(-time.Hour))
	linkIntegrationStoryArticle(t, pool, story.StoryID, firstArticle.ArticleUUID, now.Add(-2*time.Hour))
	linkIntegrationStoryArticle(t, pool, story.StoryID, secondArticle.ArticleUUID, now.Add(-time.Hour))
	createIntegrationDedupEvent(t, pool, firstArticle.ArticleID, story.StoryID, "new_story", nil, now.Add(-2*time.Hour))
	signal := "exact_url"
	createIntegrationDedupEvent(t, pool, secondArticle.ArticleID, story.StoryID, "auto_merge", &signal, now.Add(-time.Hour))
	if _, err := pool.UpsertCollectionTranslationMode(ctx, "openclaw", "enabled"); err != nil {
		t.Fatalf("UpsertCollectionTranslationMode() error = %v", err)
	}

	articles, err := pool.ListArticles(ctx, ArticleListOptions{
		Collection: "openclaw",
		From:       now.Add(-24 * time.Hour),
		To:         now.Add(24 * time.Hour),
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListArticles() error = %v", err)
	}
	if len(articles) != 2 || articles[0].ArticleUUID != secondArticle.ArticleUUID {
		t.Fatalf("articles = %#v", articles)
	}

	collections, err := pool.ListCollectionsWithCounts(ctx)
	if err != nil {
		t.Fatalf("ListCollectionsWithCounts() error = %v", err)
	}
	if len(collections) != 1 || collections[0].TranslationMode != "enabled" || collections[0].ArticleCount != 2 {
		t.Fatalf("collections = %#v", collections)
	}

	stories, err := pool.ListStoriesByDedupEventWindow(ctx, StoryEventListOptions{
		Collection: "openclaw",
		From:       now.Add(-24 * time.Hour),
		To:         now.Add(24 * time.Hour),
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListStoriesByDedupEventWindow() error = %v", err)
	}
	if len(stories) != 1 || stories[0].StoryUUID != story.StoryUUID || stories[0].ArticleCount != 2 {
		t.Fatalf("stories = %#v", stories)
	}

	searchResults, err := pool.SearchStoriesByTitle(ctx, "openclaw", "openclaw", 5)
	if err != nil {
		t.Fatalf("SearchStoriesByTitle() error = %v", err)
	}
	if len(searchResults) != 1 || searchResults[0].StoryUUID != story.StoryUUID {
		t.Fatalf("search results = %#v", searchResults)
	}

	detail, err := pool.GetStoryDetail(ctx, story.StoryUUID)
	if err != nil {
		t.Fatalf("GetStoryDetail() error = %v", err)
	}
	if detail.Story.ArticleCount != 2 || len(detail.Articles) != 2 {
		t.Fatalf("detail = %#v", detail)
	}

	digest, err := pool.ListDigestStories(ctx, "openclaw", now.Add(-24*time.Hour), now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("ListDigestStories() error = %v", err)
	}
	if len(digest) != 1 || digest[0].StoryUUID != story.StoryUUID {
		t.Fatalf("digest = %#v", digest)
	}

	stats, err := pool.QueryPipelineStats(ctx, now.Add(-24*time.Hour), now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("QueryPipelineStats() error = %v", err)
	}
	if stats.Totals.Articles != 2 || stats.Totals.Stories != 1 || stats.Throughput.PendingNotEmbedded != 2 {
		t.Fatalf("stats = %#v", stats)
	}
}

func TestPoolReadQueryEmptyResultsIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 15, 30, 0, 0, time.UTC)

	if _, err := pool.GetStoryDetail(ctx, "00000000-0000-4000-8000-00000000ffff"); err != ErrNoRows {
		t.Fatalf("GetStoryDetail(missing) error = %v, want ErrNoRows", err)
	}
	if stories, err := pool.ListStoriesByDedupEventWindow(ctx, StoryEventListOptions{Collection: "missing", From: now.Add(-time.Hour), To: now, Limit: 5}); err != nil || len(stories) != 0 {
		t.Fatalf("ListStoriesByDedupEventWindow(empty) = %#v, %v; want empty", stories, err)
	}
	if articles, err := pool.ListArticles(ctx, ArticleListOptions{Collection: "missing", From: now.Add(-time.Hour), To: now, Limit: 5}); err != nil || len(articles) != 0 {
		t.Fatalf("ListArticles(empty) = %#v, %v; want empty", articles, err)
	}
	if results, err := pool.SearchStoriesByTitle(ctx, "missing", "openclaw", 5); err != nil || len(results) != 0 {
		t.Fatalf("SearchStoriesByTitle(empty) = %#v, %v; want empty", results, err)
	}
	if digest, err := pool.ListDigestStories(ctx, "missing", now.Add(-time.Hour), now); err != nil || len(digest) != 0 {
		t.Fatalf("ListDigestStories(empty) = %#v, %v; want empty", digest, err)
	}
}

func TestPoolAuthLifecycleIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 16, 0, 0, 0, time.UTC)

	count, err := pool.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("initial user count = %d, want 0", count)
	}

	user, err := pool.CreateUser(ctx, " Admin ", "hash-1", true)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if user.Username != "admin" || !user.MustChangePassword {
		t.Fatalf("user = %#v", user)
	}
	if err := pool.SetUserLastLogin(ctx, user.UserID, now); err != nil {
		t.Fatalf("SetUserLastLogin() error = %v", err)
	}
	byUsername, err := pool.GetUserByUsername(ctx, "ADMIN")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if byUsername.LastLoginAt == nil || !byUsername.LastLoginAt.Equal(now) {
		t.Fatalf("last login = %v", byUsername.LastLoginAt)
	}
	byID, err := pool.GetUserByID(ctx, user.UserID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if byID.Username != "admin" {
		t.Fatalf("user by id = %#v", byID)
	}

	sessionID, err := pool.CreateSession(ctx, user.UserID, now.Add(time.Hour), now)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	session, err := pool.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if session.UserID != user.UserID || session.Username != "admin" {
		t.Fatalf("session = %#v", session)
	}
	if err := pool.TouchSession(ctx, sessionID, now.Add(time.Minute)); err != nil {
		t.Fatalf("TouchSession() error = %v", err)
	}

	settings, err := pool.EnsureUserSettings(ctx, user.UserID)
	if err != nil {
		t.Fatalf("EnsureUserSettings() error = %v", err)
	}
	if settings.PreferredLanguage != "en" || string(settings.UIPrefs) != "{}" {
		t.Fatalf("settings = %#v", settings)
	}
	updatedSettings, err := pool.UpsertUserSettings(ctx, user.UserID, "ZH-hant", []byte(`{"density":"compact"}`))
	if err != nil {
		t.Fatalf("UpsertUserSettings() error = %v", err)
	}
	if updatedSettings.PreferredLanguage != "zh-hant" {
		t.Fatalf("updated settings = %#v", updatedSettings)
	}

	if err := pool.SetUserPasswordHash(ctx, user.UserID, "hash-2", false); err != nil {
		t.Fatalf("SetUserPasswordHash() error = %v", err)
	}
	deletedExpired, err := pool.DeleteExpiredSessions(ctx, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("DeleteExpiredSessions() error = %v", err)
	}
	if deletedExpired != 1 {
		t.Fatalf("DeleteExpiredSessions() = %d, want 1", deletedExpired)
	}
	if err := pool.DeleteSession(ctx, sessionID); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
}

func TestPoolAuthNotFoundAndValidationPathsIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 16, 30, 0, 0, time.UTC)

	if _, err := pool.GetUserByUsername(ctx, "missing"); err != ErrNoRows {
		t.Fatalf("GetUserByUsername(missing) error = %v, want ErrNoRows", err)
	}
	if _, err := pool.GetUserByID(ctx, 999); err != ErrNoRows {
		t.Fatalf("GetUserByID(missing) error = %v, want ErrNoRows", err)
	}
	if _, err := pool.GetSession(ctx, "00000000-0000-4000-8000-00000000ffff"); err != ErrNoRows {
		t.Fatalf("GetSession(missing) error = %v, want ErrNoRows", err)
	}
	if err := pool.DeleteSession(ctx, "00000000-0000-4000-8000-00000000ffff"); err != nil {
		t.Fatalf("DeleteSession(missing) error = %v, want nil", err)
	}
	if err := pool.TouchSession(ctx, "00000000-0000-4000-8000-00000000ffff", now); err != ErrNoRows {
		t.Fatalf("TouchSession(missing) error = %v, want ErrNoRows", err)
	}
	if err := pool.SetUserLastLogin(ctx, 999, now); err != ErrNoRows {
		t.Fatalf("SetUserLastLogin(missing) error = %v, want ErrNoRows", err)
	}
	if err := pool.SetUserPasswordHash(ctx, 999, "hash", false); err != ErrNoRows {
		t.Fatalf("SetUserPasswordHash(missing) error = %v, want ErrNoRows", err)
	}
	if _, err := pool.CreateUser(ctx, " ", "hash", false); err == nil {
		t.Fatalf("CreateUser(blank username) error = nil, want constraint failure")
	}
	if _, err := pool.CreateSession(ctx, 0, now.Add(time.Hour), now); err == nil {
		t.Fatalf("CreateSession(blank user) error = nil, want foreign key failure")
	}
}

func TestPoolCollectionSettingsIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()

	mode, err := pool.GetCollectionTranslationMode(ctx, "openclaw")
	if err != nil {
		t.Fatalf("default GetCollectionTranslationMode() error = %v", err)
	}
	if IsCollectionTranslationEnabled(mode) {
		t.Fatalf("default mode = %q, want disabled", mode)
	}
	row, err := pool.UpsertCollectionTranslationMode(ctx, "OpenClaw", "ENABLED")
	if err != nil {
		t.Fatalf("UpsertCollectionTranslationMode() error = %v", err)
	}
	if row.Collection != "openclaw" || row.TranslationMode != "enabled" {
		t.Fatalf("settings row = %#v", row)
	}
	mode, err = pool.GetCollectionTranslationMode(ctx, "openclaw")
	if err != nil {
		t.Fatalf("GetCollectionTranslationMode() error = %v", err)
	}
	if mode != "enabled" {
		t.Fatalf("mode = %q, want enabled", mode)
	}
	if !IsCollectionTranslationEnabled(mode) {
		t.Fatalf("mode = %q, want enabled", mode)
	}
}

func TestPoolTranslationQueriesIntegration(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 15, 0, 0, 0, time.UTC)
	story := createIntegrationStory(t, pool, "00000000-0000-4000-8000-000000000901", "translation story", "metal_news", now)
	article := createIntegrationArticle(t, pool, "00000000-0000-4000-8000-000000000902", "translation article", "translation body", now)
	linkIntegrationStoryArticle(t, pool, story.StoryID, article.ArticleUUID, now)

	stories, err := pool.ListTranslationStoriesByCollection(ctx, "metal_news")
	if err != nil {
		t.Fatalf("ListTranslationStoriesByCollection() error = %v", err)
	}
	if len(stories) != 1 || stories[0].StoryID != story.StoryID {
		t.Fatalf("translation stories = %+v", stories)
	}
	storyTarget, err := pool.GetTranslationStoryByUUID(ctx, story.StoryUUID)
	if err != nil {
		t.Fatalf("GetTranslationStoryByUUID() error = %v", err)
	}
	if storyTarget.StoryID != story.StoryID || storyTarget.Title != story.CanonicalTitle {
		t.Fatalf("story target = %+v, want seeded story", storyTarget)
	}
	articles, err := pool.ListTranslationStoryArticles(ctx, story.StoryID)
	if err != nil {
		t.Fatalf("ListTranslationStoryArticles() error = %v", err)
	}
	if len(articles) != 1 || articles[0].ArticleID != article.ArticleID {
		t.Fatalf("translation articles = %+v", articles)
	}
	articleTarget, err := pool.GetTranslationArticleByUUID(ctx, article.ArticleUUID)
	if err != nil {
		t.Fatalf("GetTranslationArticleByUUID() error = %v", err)
	}
	if articleTarget.ArticleID != article.ArticleID || articleTarget.Text != article.NormalizedText {
		t.Fatalf("article target = %+v, want seeded article", articleTarget)
	}

	contentHash := sha256.Sum256([]byte("translation-story-title"))
	sourceID, err := pool.UpsertTranslationSource(ctx, UpsertTranslationSourceParams{
		SourceType:    "story_title",
		SourceID:      story.StoryID,
		SourceLang:    "en",
		ContentHash:   contentHash[:],
		OriginalText:  story.CanonicalTitle,
		ContentOrigin: "normalized",
	})
	if err != nil {
		t.Fatalf("UpsertTranslationSource() error = %v", err)
	}
	latency := 12
	model := "test-model"
	if err := pool.UpsertTranslationResult(ctx, UpsertTranslationResultParams{
		TranslationSourceID: sourceID,
		TargetLang:          "zh",
		TranslatedText:      "translated story",
		ProviderName:        "test",
		ModelName:           &model,
		LatencyMS:           &latency,
	}); err != nil {
		t.Fatalf("UpsertTranslationResult() error = %v", err)
	}
	rows, err := pool.ListStoryTranslationRows(ctx, story.StoryID)
	if err != nil {
		t.Fatalf("ListStoryTranslationRows() error = %v", err)
	}
	if len(rows) != 1 || rows[0].TranslatedText != "translated story" {
		t.Fatalf("translation rows = %+v", rows)
	}
	cached, err := pool.LookupCachedTranslationRow(ctx, sourceID, "zh")
	if err != nil {
		t.Fatalf("LookupCachedTranslationRow() error = %v", err)
	}
	if cached == nil || cached.TranslatedText != "translated story" {
		t.Fatalf("cached translation = %+v", cached)
	}
}

func newIntegrationPool(t *testing.T) *Pool {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("SCOOP_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("set SCOOP_TEST_DATABASE_URL or DATABASE_URL to run database integration tests")
	}

	pool, err := NewPool(context.Background(), &config.Config{
		DatabaseURL:       databaseURL,
		DBMinConns:        0,
		DBMaxConns:        1,
		Environment:       "test",
		LogLevel:          "silent",
		DefaultAdminUser:  "admin",
		SessionTTLHours:   1,
		SessionCookieName: "scoop_session",
	})
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	lockIntegrationDB(t, pool)
	t.Cleanup(func() {
		cleanIntegrationDB(t, pool)
		unlockIntegrationDB(t, pool)
		if err := pool.Close(); err != nil {
			t.Fatalf("close pool: %v", err)
		}
	})
	cleanIntegrationDB(t, pool)
	return pool
}

func lockIntegrationDB(t *testing.T, pool *Pool) {
	t.Helper()
	if err := pool.gdb.Exec("SELECT pg_advisory_lock(54718801)").Error; err != nil {
		t.Fatalf("lock integration db: %v", err)
	}
}

func unlockIntegrationDB(t *testing.T, pool *Pool) {
	t.Helper()
	if err := pool.gdb.Exec("SELECT pg_advisory_unlock(54718801)").Error; err != nil {
		t.Fatalf("unlock integration db: %v", err)
	}
}

func cleanIntegrationDB(t *testing.T, pool *Pool) {
	t.Helper()

	const q = `
	TRUNCATE TABLE
		news.audit_events,
		news.user_settings,
		news.sessions,
		news.users,
		news.collection_settings,
		news.translation_results,
		news.translation_sources,
		news.digest_entries,
		news.digest_runs,
		news.story_topic_state,
		news.topic_keyword_rules,
		news.topic_source_rules,
		news.topics,
		news.dedup_events,
		news.story_articles,
		news.stories,
		news.article_embeddings,
		news.article_person_identities,
		news.person_identities,
		news.article_tags,
		news.tags,
		news.articles,
		news.raw_arrivals,
		news.source_checkpoints,
		news.ingest_runs
	RESTART IDENTITY CASCADE
	`
	if err := pool.gdb.Exec(q).Error; err != nil {
		t.Fatalf("clean integration db: %v", err)
	}
}

func createIntegrationStory(t *testing.T, pool *Pool, storyUUID, title, collection string, now time.Time) Story {
	t.Helper()
	story := Story{
		StoryUUID:      storyUUID,
		CanonicalTitle: title,
		Collection:     collection,
		FirstSeenAt:    now,
		LastSeenAt:     now,
		Status:         "active",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := pool.gdb.Create(&story).Error; err != nil {
		t.Fatalf("create story: %v", err)
	}
	return story
}

func createIntegrationArticle(t *testing.T, pool *Pool, articleUUID, title, body string, now time.Time) Article {
	t.Helper()
	run := IngestRun{Source: "integration", Status: "completed", CreatedAt: now, UpdatedAt: now}
	if err := pool.gdb.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	payloadHash := sha256.Sum256([]byte(articleUUID))
	raw := RawArrival{
		RunID:        run.RunID,
		Source:       "integration",
		SourceItemID: articleUUID,
		Collection:   "openclaw",
		RawPayload:   []byte(`{"title":"old"}`),
		PayloadHash:  payloadHash[:],
		FetchedAt:    now,
		CreatedAt:    now,
	}
	if err := pool.gdb.Create(&raw).Error; err != nil {
		t.Fatalf("create raw arrival: %v", err)
	}
	contentHash := sha256.Sum256([]byte(title + "\n" + body))
	article := Article{
		ArticleUUID:        articleUUID,
		RawArrivalID:       raw.RawArrivalID,
		Source:             raw.Source,
		SourceItemID:       raw.SourceItemID,
		Collection:         raw.Collection,
		NormalizedTitle:    title,
		NormalizedText:     body,
		NormalizedLanguage: "en",
		ContentHash:        contentHash[:],
		TokenCount:         textmetrics.CountTokens(title + " " + body),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := pool.gdb.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}
	return article
}

func linkIntegrationStoryArticle(t *testing.T, pool *Pool, storyID int64, articleUUID string, now time.Time) {
	t.Helper()
	var article Article
	if err := pool.gdb.Where("article_uuid = ?", articleUUID).First(&article).Error; err != nil {
		t.Fatalf("query article for story link: %v", err)
	}
	link := StoryArticle{
		StoryID:   storyID,
		ArticleID: article.ArticleID,
		MatchType: "seed",
		MatchedAt: now,
	}
	if err := pool.gdb.Create(&link).Error; err != nil {
		t.Fatalf("create story article: %v", err)
	}
}

func setIntegrationArticleReadFields(t *testing.T, pool *Pool, articleID int64, rawURL string, sourceDomain string, publishedAt time.Time) {
	t.Helper()
	if err := pool.gdb.Model(&Article{}).Where("article_id = ?", articleID).Updates(map[string]any{
		"canonical_url": rawURL,
		"source_domain": sourceDomain,
		"published_at":  publishedAt,
	}).Error; err != nil {
		t.Fatalf("update article read fields: %v", err)
	}
}

func createIntegrationDedupEvent(t *testing.T, pool *Pool, articleID int64, storyID int64, decision string, exactSignal *string, createdAt time.Time) {
	t.Helper()
	event := DedupEvent{
		ArticleID:     articleID,
		Decision:      decision,
		ChosenStoryID: &storyID,
		ExactSignal:   exactSignal,
		CreatedAt:     createdAt,
	}
	if err := pool.gdb.Create(&event).Error; err != nil {
		t.Fatalf("create dedup event: %v", err)
	}
}

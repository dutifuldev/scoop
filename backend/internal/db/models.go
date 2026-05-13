package db

import (
	"encoding/json"
	"time"
)

// IngestRun maps news.ingest_runs.
type IngestRun struct {
	RunID            int64           `gorm:"column:run_id;primaryKey;autoIncrement"`
	IngestRunUUID    string          `gorm:"column:ingest_run_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	Source           string          `gorm:"column:source;type:text;not null"`
	TriggeredByTopic *string         `gorm:"column:triggered_by_topic;type:text"`
	StartedAt        time.Time       `gorm:"column:started_at;type:timestamptz;not null;default:now()"`
	FinishedAt       *time.Time      `gorm:"column:finished_at;type:timestamptz"`
	Status           string          `gorm:"column:status;type:news.ingest_run_status;not null;default:running"`
	ItemsFetched     int             `gorm:"column:items_fetched;type:integer;not null;default:0"`
	ItemsInserted    int             `gorm:"column:items_inserted;type:integer;not null;default:0"`
	CursorCheckpoint json.RawMessage `gorm:"column:cursor_checkpoint;type:jsonb"`
	ErrorMessage     *string         `gorm:"column:error_message;type:text"`
	CreatedAt        time.Time       `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt        time.Time       `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (IngestRun) TableName() string { return "news.ingest_runs" }

// SourceCheckpoint maps news.source_checkpoints.
type SourceCheckpoint struct {
	Source               string          `gorm:"column:source;type:text;primaryKey"`
	SourceCheckpointUUID string          `gorm:"column:source_checkpoint_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	CursorCheckpoint     json.RawMessage `gorm:"column:cursor_checkpoint;type:jsonb;not null"`
	LastSuccessfulRunID  *int64          `gorm:"column:last_successful_run_id;type:bigint"`
	UpdatedAt            time.Time       `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (SourceCheckpoint) TableName() string { return "news.source_checkpoints" }

// RawArrival maps news.raw_arrivals.
type RawArrival struct {
	RawArrivalID      int64           `gorm:"column:raw_arrival_id;primaryKey;autoIncrement"`
	RawArrivalUUID    string          `gorm:"column:raw_arrival_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	RunID             int64           `gorm:"column:run_id;type:bigint;not null"`
	Source            string          `gorm:"column:source;type:text;not null"`
	SourceItemID      string          `gorm:"column:source_item_id;type:text;not null"`
	Collection        string          `gorm:"column:collection;type:text;not null"`
	SourceItemURL     *string         `gorm:"column:source_item_url;type:text"`
	SourcePublishedAt *time.Time      `gorm:"column:source_published_at;type:timestamptz"`
	FetchedAt         time.Time       `gorm:"column:fetched_at;type:timestamptz;not null;default:now()"`
	RawPayload        json.RawMessage `gorm:"column:raw_payload;type:jsonb;not null"`
	PayloadHash       []byte          `gorm:"column:payload_hash;type:bytea;not null"`
	ResponseHeaders   json.RawMessage `gorm:"column:response_headers;type:jsonb"`
	DeletedAt         *time.Time      `gorm:"column:deleted_at;type:timestamptz"`
	CreatedAt         time.Time       `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (RawArrival) TableName() string { return "news.raw_arrivals" }

// Article maps news.articles.
type Article struct {
	ArticleID          int64      `gorm:"column:article_id;primaryKey;autoIncrement"`
	ArticleUUID        string     `gorm:"column:article_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	RawArrivalID       int64      `gorm:"column:raw_arrival_id;type:bigint;not null;unique"`
	Source             string     `gorm:"column:source;type:text;not null"`
	SourceItemID       string     `gorm:"column:source_item_id;type:text;not null"`
	Collection         string     `gorm:"column:collection;type:text;not null"`
	CanonicalURL       *string    `gorm:"column:canonical_url;type:text"`
	CanonicalURLHash   []byte     `gorm:"column:canonical_url_hash;type:bytea"`
	NormalizedTitle    string     `gorm:"column:normalized_title;type:text;not null"`
	NormalizedText     string     `gorm:"column:normalized_text;type:text;not null;default:''"`
	NormalizedLanguage string     `gorm:"column:normalized_language;type:text;not null;default:und"`
	PublishedAt        *time.Time `gorm:"column:published_at;type:timestamptz"`
	SourceDomain       *string    `gorm:"column:source_domain;type:text"`
	TitleSimhash       *int64     `gorm:"column:title_simhash;type:bigint"`
	TextSimhash        *int64     `gorm:"column:text_simhash;type:bigint"`
	TitleHash          []byte     `gorm:"column:title_hash;type:bytea"`
	ContentHash        []byte     `gorm:"column:content_hash;type:bytea;not null"`
	TokenCount         int        `gorm:"column:token_count;type:integer;not null;default:0"`
	DeletedAt          *time.Time `gorm:"column:deleted_at;type:timestamptz"`
	CreatedAt          time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (Article) TableName() string { return "news.articles" }

// Tag maps news.tags.
type Tag struct {
	TagID          int64      `gorm:"column:tag_id;primaryKey;autoIncrement"`
	TagUUID        string     `gorm:"column:tag_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	Slug           string     `gorm:"column:slug;type:text;not null;uniqueIndex:uq_tags_slug"`
	Name           string     `gorm:"column:name;type:text;not null"`
	Description    *string    `gorm:"column:description;type:text"`
	Color          *string    `gorm:"column:color;type:text"`
	HighlightColor *string    `gorm:"column:highlight_color;type:text"`
	ArchivedAt     *time.Time `gorm:"column:archived_at;type:timestamptz"`
	CreatedAt      time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (Tag) TableName() string { return "news.tags" }

// ArticleTag maps news.article_tags.
type ArticleTag struct {
	ArticleID       int64     `gorm:"column:article_id;type:bigint;primaryKey"`
	TagID           int64     `gorm:"column:tag_id;type:bigint;primaryKey"`
	CreatedByUserID *int64    `gorm:"column:created_by_user_id;type:bigint"`
	CreatedAt       time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (ArticleTag) TableName() string { return "news.article_tags" }

// PersonIdentity maps news.person_identities.
type PersonIdentity struct {
	PersonIdentityID   int64      `gorm:"column:person_identity_id;primaryKey;autoIncrement"`
	PersonIdentityUUID string     `gorm:"column:person_identity_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	Provider           string     `gorm:"column:provider;type:text;not null"`
	ProviderUserID     *string    `gorm:"column:provider_user_id;type:text"`
	Handle             *string    `gorm:"column:handle;type:text"`
	AvatarURL          *string    `gorm:"column:avatar_url;type:text"`
	IdentityRef        string     `gorm:"column:identity_ref;type:text;not null;unique"`
	ArchivedAt         *time.Time `gorm:"column:archived_at;type:timestamptz"`
	CreatedAt          time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (PersonIdentity) TableName() string { return "news.person_identities" }

// ArticlePersonIdentity maps news.article_person_identities.
type ArticlePersonIdentity struct {
	ArticleID        int64     `gorm:"column:article_id;type:bigint;primaryKey"`
	PersonIdentityID int64     `gorm:"column:person_identity_id;type:bigint;primaryKey"`
	CreatedByUserID  *int64    `gorm:"column:created_by_user_id;type:bigint"`
	CreatedAt        time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (ArticlePersonIdentity) TableName() string { return "news.article_person_identities" }

// ArticleEmbedding maps news.article_embeddings.
type ArticleEmbedding struct {
	ArticleEmbeddingID   int64     `gorm:"column:article_embedding_id;primaryKey;autoIncrement"`
	ArticleEmbeddingUUID string    `gorm:"column:article_embedding_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	ArticleID            int64     `gorm:"column:article_id;type:bigint;not null"`
	ModelName            string    `gorm:"column:model_name;type:text;not null"`
	ModelVersion         string    `gorm:"column:model_version;type:text;not null"`
	Embedding            string    `gorm:"column:embedding;type:vector(4096);not null"`
	EmbeddedAt           time.Time `gorm:"column:embedded_at;type:timestamptz;not null;default:now()"`
	ServiceEndpoint      string    `gorm:"column:service_endpoint;type:text;not null;default:http://127.0.0.1:8844"`
	LatencyMS            *int      `gorm:"column:latency_ms;type:integer"`
}

func (ArticleEmbedding) TableName() string { return "news.article_embeddings" }

// Story maps news.stories.
type Story struct {
	StoryID                 int64      `gorm:"column:story_id;primaryKey;autoIncrement"`
	StoryUUID               string     `gorm:"column:story_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	CanonicalTitle          string     `gorm:"column:canonical_title;type:text;not null"`
	CanonicalURL            *string    `gorm:"column:canonical_url;type:text"`
	CanonicalURLHash        []byte     `gorm:"column:canonical_url_hash;type:bytea"`
	Collection              string     `gorm:"column:collection;type:text;not null"`
	RepresentativeArticleID *int64     `gorm:"column:representative_article_id;type:bigint"`
	FirstSeenAt             time.Time  `gorm:"column:first_seen_at;type:timestamptz;not null"`
	LastSeenAt              time.Time  `gorm:"column:last_seen_at;type:timestamptz;not null"`
	Status                  string     `gorm:"column:status;type:text;not null;default:active"`
	DeletedAt               *time.Time `gorm:"column:deleted_at;type:timestamptz"`
	CreatedAt               time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt               time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (Story) TableName() string { return "news.stories" }

// StoryArticle maps news.story_articles.
type StoryArticle struct {
	StoryID          int64           `gorm:"column:story_id;type:bigint;primaryKey"`
	ArticleID        int64           `gorm:"column:article_id;type:bigint;primaryKey;unique"`
	StoryArticleUUID string          `gorm:"column:story_article_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	MatchType        string          `gorm:"column:match_type;type:news.story_match_type;not null"`
	MatchScore       *float64        `gorm:"column:match_score;type:double precision"`
	MatchDetails     json.RawMessage `gorm:"column:match_details;type:jsonb"`
	MatchedAt        time.Time       `gorm:"column:matched_at;type:timestamptz;not null;default:now()"`
}

func (StoryArticle) TableName() string { return "news.story_articles" }

// DedupEvent maps news.dedup_events.
type DedupEvent struct {
	DedupEventID          int64     `gorm:"column:dedup_event_id;primaryKey;autoIncrement"`
	DedupEventUUID        string    `gorm:"column:dedup_event_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	ArticleID             int64     `gorm:"column:article_id;type:bigint;not null;unique"`
	Decision              string    `gorm:"column:decision;type:news.dedup_decision;not null"`
	ChosenStoryID         *int64    `gorm:"column:chosen_story_id;type:bigint"`
	BestCandidateStoryID  *int64    `gorm:"column:best_candidate_story_id;type:bigint"`
	BestCosine            *float64  `gorm:"column:best_cosine;type:double precision"`
	TitleOverlap          *float64  `gorm:"column:title_overlap;type:double precision"`
	EntityDateConsistency *float64  `gorm:"column:entity_date_consistency;type:double precision"`
	CompositeScore        *float64  `gorm:"column:composite_score;type:double precision"`
	ExactSignal           *string   `gorm:"column:exact_signal;type:text"`
	CreatedAt             time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (DedupEvent) TableName() string { return "news.dedup_events" }

// Topic maps news.topics.
type Topic struct {
	TopicID          int       `gorm:"column:topic_id;primaryKey;autoIncrement"`
	TopicUUID        string    `gorm:"column:topic_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	TopicSlug        string    `gorm:"column:topic_slug;type:text;not null;unique"`
	TopicName        string    `gorm:"column:topic_name;type:text;not null"`
	Timezone         string    `gorm:"column:timezone;type:text;not null;default:Europe/Berlin"`
	DigestCron       string    `gorm:"column:digest_cron;type:text;not null"`
	DiscordChannelID string    `gorm:"column:discord_channel_id;type:text;not null"`
	Enabled          bool      `gorm:"column:enabled;type:boolean;not null;default:true"`
	CreatedAt        time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt        time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (Topic) TableName() string { return "news.topics" }

// TopicSourceRule maps news.topic_source_rules.
type TopicSourceRule struct {
	TopicID             int       `gorm:"column:topic_id;type:integer;primaryKey"`
	Source              string    `gorm:"column:source;type:text;primaryKey"`
	RuleType            string    `gorm:"column:rule_type;type:news.topic_rule_type;primaryKey;default:include"`
	TopicSourceRuleUUID string    `gorm:"column:topic_source_rule_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	CreatedAt           time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (TopicSourceRule) TableName() string { return "news.topic_source_rules" }

// TopicKeywordRule maps news.topic_keyword_rules.
type TopicKeywordRule struct {
	RuleID               int64     `gorm:"column:rule_id;primaryKey;autoIncrement"`
	TopicKeywordRuleUUID string    `gorm:"column:topic_keyword_rule_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	TopicID              int       `gorm:"column:topic_id;type:integer;not null"`
	RuleType             string    `gorm:"column:rule_type;type:news.topic_rule_type;not null"`
	Pattern              string    `gorm:"column:pattern;type:text;not null"`
	IsRegex              bool      `gorm:"column:is_regex;type:boolean;not null;default:false"`
	Weight               int16     `gorm:"column:weight;type:smallint;not null;default:1"`
	Enabled              bool      `gorm:"column:enabled;type:boolean;not null;default:true"`
	CreatedAt            time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (TopicKeywordRule) TableName() string { return "news.topic_keyword_rules" }

// StoryTopicState maps news.story_topic_state.
type StoryTopicState struct {
	StoryID             int64      `gorm:"column:story_id;type:bigint;primaryKey"`
	TopicID             int        `gorm:"column:topic_id;type:integer;primaryKey"`
	StoryTopicStateUUID string     `gorm:"column:story_topic_state_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	FirstSeenInTopicAt  time.Time  `gorm:"column:first_seen_in_topic_at;type:timestamptz;not null"`
	FirstPublishedAt    *time.Time `gorm:"column:first_published_at;type:timestamptz"`
	LastPublishedAt     *time.Time `gorm:"column:last_published_at;type:timestamptz"`
	PublishCount        int        `gorm:"column:publish_count;type:integer;not null;default:0"`
	Suppressed          bool       `gorm:"column:suppressed;type:boolean;not null;default:false"`
	SuppressionReason   *string    `gorm:"column:suppression_reason;type:text"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (StoryTopicState) TableName() string { return "news.story_topic_state" }

// DigestRun maps news.digest_runs.
type DigestRun struct {
	DigestRunID      int64      `gorm:"column:digest_run_id;primaryKey;autoIncrement"`
	DigestRunUUID    string     `gorm:"column:digest_run_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	TopicID          int        `gorm:"column:topic_id;type:integer;not null"`
	RunDate          time.Time  `gorm:"column:run_date;type:date;not null"`
	WindowStartUTC   time.Time  `gorm:"column:window_start_utc;type:timestamptz;not null"`
	WindowEndUTC     time.Time  `gorm:"column:window_end_utc;type:timestamptz;not null"`
	StartedAt        time.Time  `gorm:"column:started_at;type:timestamptz;not null;default:now()"`
	FinishedAt       *time.Time `gorm:"column:finished_at;type:timestamptz"`
	Status           string     `gorm:"column:status;type:news.digest_run_status;not null;default:running"`
	CandidateCount   int        `gorm:"column:candidate_count;type:integer;not null;default:0"`
	PostedCount      int        `gorm:"column:posted_count;type:integer;not null;default:0"`
	DiscordMessageID *string    `gorm:"column:discord_message_id;type:text"`
	ErrorMessage     *string    `gorm:"column:error_message;type:text"`
}

func (DigestRun) TableName() string { return "news.digest_runs" }

// DigestEntry maps news.digest_entries.
type DigestEntry struct {
	DigestEntryID    int64     `gorm:"column:digest_entry_id;primaryKey;autoIncrement"`
	DigestEntryUUID  string    `gorm:"column:digest_entry_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	DigestRunID      int64     `gorm:"column:digest_run_id;type:bigint;not null"`
	StoryID          int64     `gorm:"column:story_id;type:bigint;not null"`
	Status           string    `gorm:"column:status;type:news.digest_entry_status;not null"`
	Rank             *int      `gorm:"column:rank;type:integer"`
	Reason           *string   `gorm:"column:reason;type:text"`
	Score            *float64  `gorm:"column:score;type:double precision"`
	DiscordMessageID *string   `gorm:"column:discord_message_id;type:text"`
	CreatedAt        time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (DigestEntry) TableName() string { return "news.digest_entries" }

// TranslationSource maps news.translation_sources.
type TranslationSource struct {
	TranslationSourceID int64     `gorm:"column:translation_source_id;primaryKey;autoIncrement"`
	SourceType          string    `gorm:"column:source_type;type:text;not null;uniqueIndex:uq_translation_sources_identity,priority:1;index:idx_translation_sources_lookup,priority:1"`
	SourceID            int64     `gorm:"column:source_id;type:bigint;not null;uniqueIndex:uq_translation_sources_identity,priority:2;index:idx_translation_sources_lookup,priority:2"`
	SourceLang          string    `gorm:"column:source_lang;type:text;not null"`
	ContentHash         []byte    `gorm:"column:content_hash;type:bytea;not null;uniqueIndex:uq_translation_sources_identity,priority:3"`
	OriginalText        string    `gorm:"column:original_text;type:text;not null"`
	ContentOrigin       string    `gorm:"column:content_origin;type:text;not null;default:normalized"`
	CapturedAt          time.Time `gorm:"column:captured_at;type:timestamptz;not null;default:now();index:idx_translation_sources_lookup,priority:3,sort:desc"`
	CreatedAt           time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (TranslationSource) TableName() string { return "news.translation_sources" }

// TranslationResult maps news.translation_results.
type TranslationResult struct {
	TranslationResultID   int64     `gorm:"column:translation_result_id;primaryKey;autoIncrement"`
	TranslationResultUUID string    `gorm:"column:translation_result_uuid;type:uuid;not null;default:gen_random_uuid();unique"`
	TranslationSourceID   int64     `gorm:"column:translation_source_id;type:bigint;not null;uniqueIndex:uq_translation_results_source_target,priority:1"`
	TargetLang            string    `gorm:"column:target_lang;type:text;not null;uniqueIndex:uq_translation_results_source_target,priority:2;index:idx_translation_results_target_lang"`
	TranslatedText        string    `gorm:"column:translated_text;type:text;not null"`
	ProviderName          string    `gorm:"column:provider_name;type:text;not null"`
	ModelName             *string   `gorm:"column:model_name;type:text"`
	LatencyMS             *int      `gorm:"column:latency_ms;type:integer"`
	CreatedAt             time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (TranslationResult) TableName() string { return "news.translation_results" }

// CollectionSettings maps news.collection_settings.
type CollectionSettings struct {
	Collection      string    `gorm:"column:collection;type:text;primaryKey"`
	TranslationMode string    `gorm:"column:translation_mode;type:text;not null;default:disabled"`
	CreatedAt       time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt       time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (CollectionSettings) TableName() string { return "news.collection_settings" }

// User maps news.users.
type User struct {
	UserID             int64      `gorm:"column:user_id;primaryKey;autoIncrement"`
	Username           string     `gorm:"column:username;type:text;not null;uniqueIndex:uq_users_username"`
	PasswordHash       string     `gorm:"column:password_hash;type:text;not null"`
	MustChangePassword bool       `gorm:"column:must_change_password;type:boolean;not null;default:false"`
	CreatedAt          time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	LastLoginAt        *time.Time `gorm:"column:last_login_at;type:timestamptz"`
}

func (User) TableName() string { return "news.users" }

// Session maps news.sessions.
type Session struct {
	SessionID  string    `gorm:"column:session_id;type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID     int64     `gorm:"column:user_id;type:bigint;not null;index:idx_sessions_user_expires,priority:1"`
	ExpiresAt  time.Time `gorm:"column:expires_at;type:timestamptz;not null;index:idx_sessions_user_expires,priority:2"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	LastSeenAt time.Time `gorm:"column:last_seen_at;type:timestamptz;not null;default:now()"`
}

func (Session) TableName() string { return "news.sessions" }

// UserSettings maps news.user_settings.
type UserSettings struct {
	UserID            int64           `gorm:"column:user_id;type:bigint;primaryKey"`
	PreferredLanguage string          `gorm:"column:preferred_language;type:text;not null;default:en"`
	UIPrefs           json.RawMessage `gorm:"column:ui_prefs;type:jsonb;not null;default:'{}'::jsonb"`
}

func (UserSettings) TableName() string { return "news.user_settings" }

// AuditEvent maps news.audit_events.
type AuditEvent struct {
	EventID          int64           `gorm:"column:event_id;primaryKey;autoIncrement"`
	ActorUserID      *int64          `gorm:"column:actor_user_id;type:bigint"`
	Permission       *string         `gorm:"column:permission;type:text"`
	TemporaryGrantID *int64          `gorm:"column:temporary_grant_id;type:bigint"`
	Action           string          `gorm:"column:action;type:text;not null"`
	TargetType       string          `gorm:"column:target_type;type:text;not null"`
	TargetID         string          `gorm:"column:target_id;type:text;not null"`
	Details          json.RawMessage `gorm:"column:details;type:jsonb;not null;default:'{}'::jsonb"`
	CreatedAt        time.Time       `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (AuditEvent) TableName() string { return "news.audit_events" }

func autoMigrateModels() []any {
	return []any{
		&IngestRun{},
		&SourceCheckpoint{},
		&RawArrival{},
		&Article{},
		&Tag{},
		&ArticleTag{},
		&PersonIdentity{},
		&ArticlePersonIdentity{},
		&ArticleEmbedding{},
		&Story{},
		&StoryArticle{},
		&DedupEvent{},
		&Topic{},
		&TopicSourceRule{},
		&TopicKeywordRule{},
		&StoryTopicState{},
		&DigestRun{},
		&DigestEntry{},
		&TranslationSource{},
		&TranslationResult{},
		&CollectionSettings{},
		&User{},
		&Session{},
		&UserSettings{},
		&AuditEvent{},
	}
}

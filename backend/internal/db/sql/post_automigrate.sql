SET search_path = news, public;

CREATE UNIQUE INDEX IF NOT EXISTS uq_ingest_runs_uuid
	ON news.ingest_runs (ingest_run_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_source_checkpoints_uuid
	ON news.source_checkpoints (source_checkpoint_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_raw_arrivals_uuid
	ON news.raw_arrivals (raw_arrival_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_articles_uuid
	ON news.articles (article_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tags_uuid
	ON news.tags (tag_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tags_slug
	ON news.tags (slug);

CREATE UNIQUE INDEX IF NOT EXISTS uq_article_embeddings_uuid
	ON news.article_embeddings (article_embedding_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_stories_uuid
	ON news.stories (story_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_story_articles_uuid
	ON news.story_articles (story_article_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_dedup_events_uuid
	ON news.dedup_events (dedup_event_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_topics_uuid
	ON news.topics (topic_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_topics_topic_slug
	ON news.topics (topic_slug);

CREATE UNIQUE INDEX IF NOT EXISTS uq_topic_source_rules_uuid
	ON news.topic_source_rules (topic_source_rule_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_topic_keyword_rules_uuid
	ON news.topic_keyword_rules (topic_keyword_rule_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_story_topic_state_uuid
	ON news.story_topic_state (story_topic_state_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_digest_runs_uuid
	ON news.digest_runs (digest_run_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_digest_entries_uuid
	ON news.digest_entries (digest_entry_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_translation_results_uuid
	ON news.translation_results (translation_result_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_username
	ON news.users (username);

CREATE UNIQUE INDEX IF NOT EXISTS uq_raw_arrivals_source_item_payload
	ON news.raw_arrivals (source, source_item_id, payload_hash);

CREATE UNIQUE INDEX IF NOT EXISTS uq_articles_raw_arrival_id
	ON news.articles (raw_arrival_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_article_tags_article_tag
	ON news.article_tags (article_id, tag_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_article_embeddings_article_model
	ON news.article_embeddings (article_id, model_name, model_version);

CREATE UNIQUE INDEX IF NOT EXISTS uq_story_articles_article_id
	ON news.story_articles (article_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_dedup_events_article_id
	ON news.dedup_events (article_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_topic_keyword_rule
	ON news.topic_keyword_rules (topic_id, rule_type, pattern);

CREATE UNIQUE INDEX IF NOT EXISTS uq_digest_runs_topic_date
	ON news.digest_runs (topic_id, run_date);

CREATE UNIQUE INDEX IF NOT EXISTS uq_digest_entries_run_story
	ON news.digest_entries (digest_run_id, story_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_translation_sources_identity
	ON news.translation_sources (source_type, source_id, content_hash);

CREATE UNIQUE INDEX IF NOT EXISTS uq_translation_results_source_target
	ON news.translation_results (translation_source_id, target_lang);

CREATE INDEX IF NOT EXISTS idx_raw_arrivals_source_item_fetched
	ON news.raw_arrivals (source, source_item_id, fetched_at DESC);

CREATE INDEX IF NOT EXISTS idx_raw_arrivals_fetched_at
	ON news.raw_arrivals (fetched_at DESC);

CREATE INDEX IF NOT EXISTS idx_raw_arrivals_payload_hash
	ON news.raw_arrivals (payload_hash);

CREATE INDEX IF NOT EXISTS idx_raw_arrivals_payload_gin
	ON news.raw_arrivals USING gin (raw_payload jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_raw_arrivals_collection_fetched
	ON news.raw_arrivals (collection, fetched_at DESC);

CREATE INDEX IF NOT EXISTS idx_raw_arrivals_fetched_at_not_deleted
	ON news.raw_arrivals (fetched_at DESC)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_raw_arrivals_collection_fetched_not_deleted
	ON news.raw_arrivals (collection, fetched_at DESC)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_articles_source_item
	ON news.articles (source, source_item_id);

CREATE INDEX IF NOT EXISTS idx_articles_canonical_url_hash
	ON news.articles (canonical_url_hash)
	WHERE canonical_url_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_articles_content_hash
	ON news.articles (content_hash);

CREATE INDEX IF NOT EXISTS idx_articles_source_domain_published
	ON news.articles (source_domain, published_at DESC);

CREATE INDEX IF NOT EXISTS idx_articles_created_at
	ON news.articles (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_articles_created_at_not_deleted
	ON news.articles (created_at DESC)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_articles_title_simhash
	ON news.articles (title_simhash)
	WHERE title_simhash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_articles_collection_source_item
	ON news.articles (collection, source, source_item_id);

CREATE INDEX IF NOT EXISTS idx_articles_collection_canonical_url_hash
	ON news.articles (collection, canonical_url_hash)
	WHERE canonical_url_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_articles_collection_content_hash
	ON news.articles (collection, content_hash);

CREATE INDEX IF NOT EXISTS idx_articles_collection_created_at
	ON news.articles (collection, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_articles_collection_created_at_not_deleted
	ON news.articles (collection, created_at DESC)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_article_tags_tag_article
	ON news.article_tags (tag_id, article_id);

CREATE INDEX IF NOT EXISTS idx_tags_archived_slug
	ON news.tags (archived_at, slug);

CREATE INDEX IF NOT EXISTS idx_article_embeddings_article
	ON news.article_embeddings (article_id);

CREATE INDEX IF NOT EXISTS idx_article_embeddings_model_time
	ON news.article_embeddings (model_name, model_version, embedded_at DESC);

CREATE INDEX IF NOT EXISTS idx_article_embeddings_embedded_at
	ON news.article_embeddings (embedded_at DESC);

DO $$
DECLARE
	embedding_dims integer;
BEGIN
	SELECT a.atttypmod
	INTO embedding_dims
	FROM pg_attribute a
	JOIN pg_class c ON c.oid = a.attrelid
	JOIN pg_namespace n ON n.oid = c.relnamespace
	WHERE n.nspname = 'news'
		AND c.relname = 'article_embeddings'
		AND a.attname = 'embedding'
		AND a.attnum > 0
		AND NOT a.attisdropped;

	IF embedding_dims IS NOT NULL
		AND embedding_dims > 0
		AND embedding_dims <= 2000 THEN
		EXECUTE $stmt$
			CREATE INDEX IF NOT EXISTS idx_article_embeddings_hnsw
			ON news.article_embeddings USING hnsw (embedding vector_cosine_ops)
			WITH (m = 16, ef_construction = 128)
		$stmt$;
	END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_stories_first_seen_at
	ON news.stories (first_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_stories_last_seen_at
	ON news.stories (last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_stories_last_seen_at_not_deleted
	ON news.stories (last_seen_at DESC)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_stories_canonical_url_hash
	ON news.stories (canonical_url_hash)
	WHERE canonical_url_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_stories_collection_last_seen
	ON news.stories (collection, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_stories_collection_last_seen_not_deleted
	ON news.stories (collection, last_seen_at DESC)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_stories_collection_canonical_url_hash
	ON news.stories (collection, canonical_url_hash)
	WHERE canonical_url_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_story_articles_story
	ON news.story_articles (story_id, matched_at DESC);

CREATE INDEX IF NOT EXISTS idx_story_articles_match_type
	ON news.story_articles (match_type);

CREATE INDEX IF NOT EXISTS idx_dedup_events_decision_time
	ON news.dedup_events (decision, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_dedup_events_chosen_story
	ON news.dedup_events (chosen_story_id);

CREATE INDEX IF NOT EXISTS idx_topics_enabled
	ON news.topics (enabled);

CREATE INDEX IF NOT EXISTS idx_topic_source_rules_topic
	ON news.topic_source_rules (topic_id, rule_type);

CREATE INDEX IF NOT EXISTS idx_topic_keyword_rules_topic_enabled
	ON news.topic_keyword_rules (topic_id, enabled);

CREATE INDEX IF NOT EXISTS idx_story_topic_state_topic_seen
	ON news.story_topic_state (topic_id, first_seen_in_topic_at DESC);

CREATE INDEX IF NOT EXISTS idx_story_topic_state_topic_first_published
	ON news.story_topic_state (topic_id, first_published_at);

CREATE INDEX IF NOT EXISTS idx_story_topic_state_topic_suppressed
	ON news.story_topic_state (topic_id, suppressed);

CREATE INDEX IF NOT EXISTS idx_digest_runs_topic_date
	ON news.digest_runs (topic_id, run_date DESC);

CREATE INDEX IF NOT EXISTS idx_digest_runs_status_started
	ON news.digest_runs (status, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_digest_entries_run_status_rank
	ON news.digest_entries (digest_run_id, status, rank);

CREATE INDEX IF NOT EXISTS idx_digest_entries_story
	ON news.digest_entries (story_id);

CREATE INDEX IF NOT EXISTS idx_translation_sources_lookup
	ON news.translation_sources (source_type, source_id, captured_at DESC);

CREATE INDEX IF NOT EXISTS idx_translation_results_target_lang
	ON news.translation_results (target_lang);

CREATE INDEX IF NOT EXISTS idx_sessions_user_expires
	ON news.sessions (user_id, expires_at DESC);

CREATE INDEX IF NOT EXISTS idx_sessions_expires_at
	ON news.sessions (expires_at);

CREATE INDEX IF NOT EXISTS idx_user_settings_preferred_language
	ON news.user_settings (preferred_language);

CREATE INDEX IF NOT EXISTS idx_collection_settings_translation_mode
	ON news.collection_settings (translation_mode);

CREATE INDEX IF NOT EXISTS idx_audit_events_target_created
	ON news.audit_events (target_type, target_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_events_actor_created
	ON news.audit_events (actor_user_id, created_at DESC);

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'ingest_runs_source_check'
			AND conrelid = 'news.ingest_runs'::regclass
	) THEN
		ALTER TABLE news.ingest_runs
			ADD CONSTRAINT ingest_runs_source_check CHECK (length(trim(source)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'ingest_runs_items_fetched_check'
			AND conrelid = 'news.ingest_runs'::regclass
	) THEN
		ALTER TABLE news.ingest_runs
			ADD CONSTRAINT ingest_runs_items_fetched_check CHECK (items_fetched >= 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'ingest_runs_items_inserted_check'
			AND conrelid = 'news.ingest_runs'::regclass
	) THEN
		ALTER TABLE news.ingest_runs
			ADD CONSTRAINT ingest_runs_items_inserted_check CHECK (items_inserted >= 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'ingest_runs_finished_after_start'
			AND conrelid = 'news.ingest_runs'::regclass
	) THEN
		ALTER TABLE news.ingest_runs
			ADD CONSTRAINT ingest_runs_finished_after_start CHECK (finished_at IS NULL OR finished_at >= started_at);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'source_checkpoints_source_check'
			AND conrelid = 'news.source_checkpoints'::regclass
	) THEN
		ALTER TABLE news.source_checkpoints
			ADD CONSTRAINT source_checkpoints_source_check CHECK (length(trim(source)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'raw_arrivals_source_check'
			AND conrelid = 'news.raw_arrivals'::regclass
	) THEN
		ALTER TABLE news.raw_arrivals
			ADD CONSTRAINT raw_arrivals_source_check CHECK (length(trim(source)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'raw_arrivals_source_item_id_check'
			AND conrelid = 'news.raw_arrivals'::regclass
	) THEN
		ALTER TABLE news.raw_arrivals
			ADD CONSTRAINT raw_arrivals_source_item_id_check CHECK (length(trim(source_item_id)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'raw_arrivals_payload_hash_check'
			AND conrelid = 'news.raw_arrivals'::regclass
	) THEN
		ALTER TABLE news.raw_arrivals
			ADD CONSTRAINT raw_arrivals_payload_hash_check CHECK (octet_length(payload_hash) = 32);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'raw_arrivals_collection_nonempty'
			AND conrelid = 'news.raw_arrivals'::regclass
	) THEN
		ALTER TABLE news.raw_arrivals
			ADD CONSTRAINT raw_arrivals_collection_nonempty CHECK (length(trim(collection)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'articles_source_check'
			AND conrelid = 'news.articles'::regclass
	) THEN
		ALTER TABLE news.articles
			ADD CONSTRAINT articles_source_check CHECK (length(trim(source)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'articles_source_item_id_check'
			AND conrelid = 'news.articles'::regclass
	) THEN
		ALTER TABLE news.articles
			ADD CONSTRAINT articles_source_item_id_check CHECK (length(trim(source_item_id)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'articles_canonical_url_hash_check'
			AND conrelid = 'news.articles'::regclass
	) THEN
		ALTER TABLE news.articles
			ADD CONSTRAINT articles_canonical_url_hash_check CHECK (canonical_url_hash IS NULL OR octet_length(canonical_url_hash) = 32);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'articles_normalized_title_check'
			AND conrelid = 'news.articles'::regclass
	) THEN
		ALTER TABLE news.articles
			ADD CONSTRAINT articles_normalized_title_check CHECK (length(trim(normalized_title)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'articles_title_hash_check'
			AND conrelid = 'news.articles'::regclass
	) THEN
		ALTER TABLE news.articles
			ADD CONSTRAINT articles_title_hash_check CHECK (title_hash IS NULL OR octet_length(title_hash) = 32);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'articles_content_hash_check'
			AND conrelid = 'news.articles'::regclass
	) THEN
		ALTER TABLE news.articles
			ADD CONSTRAINT articles_content_hash_check CHECK (octet_length(content_hash) = 32);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'articles_token_count_check'
			AND conrelid = 'news.articles'::regclass
	) THEN
		ALTER TABLE news.articles
			ADD CONSTRAINT articles_token_count_check CHECK (token_count >= 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'articles_collection_nonempty'
			AND conrelid = 'news.articles'::regclass
	) THEN
		ALTER TABLE news.articles
			ADD CONSTRAINT articles_collection_nonempty CHECK (length(trim(collection)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'tags_slug_check'
			AND conrelid = 'news.tags'::regclass
	) THEN
		ALTER TABLE news.tags
			ADD CONSTRAINT tags_slug_check CHECK (slug ~ '^[a-z0-9][a-z0-9_-]{0,63}$');
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'tags_name_check'
			AND conrelid = 'news.tags'::regclass
	) THEN
		ALTER TABLE news.tags
			ADD CONSTRAINT tags_name_check CHECK (length(trim(name)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'tags_color_check'
			AND conrelid = 'news.tags'::regclass
	) THEN
		ALTER TABLE news.tags
			ADD CONSTRAINT tags_color_check CHECK (color IS NULL OR color ~ '^#[0-9a-fA-F]{6}$');
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'article_embeddings_model_name_check'
			AND conrelid = 'news.article_embeddings'::regclass
	) THEN
		ALTER TABLE news.article_embeddings
			ADD CONSTRAINT article_embeddings_model_name_check CHECK (length(trim(model_name)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'article_embeddings_model_version_check'
			AND conrelid = 'news.article_embeddings'::regclass
	) THEN
		ALTER TABLE news.article_embeddings
			ADD CONSTRAINT article_embeddings_model_version_check CHECK (length(trim(model_version)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'article_embeddings_latency_ms_check'
			AND conrelid = 'news.article_embeddings'::regclass
	) THEN
		ALTER TABLE news.article_embeddings
			ADD CONSTRAINT article_embeddings_latency_ms_check CHECK (latency_ms IS NULL OR latency_ms >= 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'stories_canonical_title_check'
			AND conrelid = 'news.stories'::regclass
	) THEN
		ALTER TABLE news.stories
			ADD CONSTRAINT stories_canonical_title_check CHECK (length(trim(canonical_title)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'stories_canonical_url_hash_check'
			AND conrelid = 'news.stories'::regclass
	) THEN
		ALTER TABLE news.stories
			ADD CONSTRAINT stories_canonical_url_hash_check CHECK (canonical_url_hash IS NULL OR octet_length(canonical_url_hash) = 32);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'stories_status_check'
			AND conrelid = 'news.stories'::regclass
	) THEN
		ALTER TABLE news.stories
			ADD CONSTRAINT stories_status_check CHECK (status IN ('active', 'suppressed', 'merged'));
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'stories_seen_window_valid'
			AND conrelid = 'news.stories'::regclass
	) THEN
		ALTER TABLE news.stories
			ADD CONSTRAINT stories_seen_window_valid CHECK (last_seen_at >= first_seen_at);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'stories_collection_nonempty'
			AND conrelid = 'news.stories'::regclass
	) THEN
		ALTER TABLE news.stories
			ADD CONSTRAINT stories_collection_nonempty CHECK (length(trim(collection)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'story_articles_score_range'
			AND conrelid = 'news.story_articles'::regclass
	) THEN
		ALTER TABLE news.story_articles
			ADD CONSTRAINT story_articles_score_range CHECK (match_score IS NULL OR (match_score >= 0 AND match_score <= 1));
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'dedup_events_best_cosine_range'
			AND conrelid = 'news.dedup_events'::regclass
	) THEN
		ALTER TABLE news.dedup_events
			ADD CONSTRAINT dedup_events_best_cosine_range CHECK (best_cosine IS NULL OR (best_cosine >= 0 AND best_cosine <= 1));
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'dedup_events_title_overlap_range'
			AND conrelid = 'news.dedup_events'::regclass
	) THEN
		ALTER TABLE news.dedup_events
			ADD CONSTRAINT dedup_events_title_overlap_range CHECK (title_overlap IS NULL OR (title_overlap >= 0 AND title_overlap <= 1));
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'dedup_events_entity_date_range'
			AND conrelid = 'news.dedup_events'::regclass
	) THEN
		ALTER TABLE news.dedup_events
			ADD CONSTRAINT dedup_events_entity_date_range CHECK (entity_date_consistency IS NULL OR (entity_date_consistency >= 0 AND entity_date_consistency <= 1));
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'dedup_events_composite_range'
			AND conrelid = 'news.dedup_events'::regclass
	) THEN
		ALTER TABLE news.dedup_events
			ADD CONSTRAINT dedup_events_composite_range CHECK (composite_score IS NULL OR (composite_score >= 0 AND composite_score <= 1));
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'topics_topic_slug_check'
			AND conrelid = 'news.topics'::regclass
	) THEN
		ALTER TABLE news.topics
			ADD CONSTRAINT topics_topic_slug_check CHECK (topic_slug ~ '^[a-z0-9_]+$');
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'topics_topic_name_check'
			AND conrelid = 'news.topics'::regclass
	) THEN
		ALTER TABLE news.topics
			ADD CONSTRAINT topics_topic_name_check CHECK (length(trim(topic_name)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'topic_source_rules_source_check'
			AND conrelid = 'news.topic_source_rules'::regclass
	) THEN
		ALTER TABLE news.topic_source_rules
			ADD CONSTRAINT topic_source_rules_source_check CHECK (length(trim(source)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'topic_keyword_rules_pattern_check'
			AND conrelid = 'news.topic_keyword_rules'::regclass
	) THEN
		ALTER TABLE news.topic_keyword_rules
			ADD CONSTRAINT topic_keyword_rules_pattern_check CHECK (length(trim(pattern)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'story_topic_state_publish_count_check'
			AND conrelid = 'news.story_topic_state'::regclass
	) THEN
		ALTER TABLE news.story_topic_state
			ADD CONSTRAINT story_topic_state_publish_count_check CHECK (publish_count >= 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'story_topic_state_first_pub_after_seen'
			AND conrelid = 'news.story_topic_state'::regclass
	) THEN
		ALTER TABLE news.story_topic_state
			ADD CONSTRAINT story_topic_state_first_pub_after_seen CHECK (first_published_at IS NULL OR first_published_at >= first_seen_in_topic_at);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'story_topic_state_last_pub_after_first'
			AND conrelid = 'news.story_topic_state'::regclass
	) THEN
		ALTER TABLE news.story_topic_state
			ADD CONSTRAINT story_topic_state_last_pub_after_first CHECK (last_published_at IS NULL OR (first_published_at IS NOT NULL AND last_published_at >= first_published_at));
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'digest_runs_candidate_count_check'
			AND conrelid = 'news.digest_runs'::regclass
	) THEN
		ALTER TABLE news.digest_runs
			ADD CONSTRAINT digest_runs_candidate_count_check CHECK (candidate_count >= 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'digest_runs_posted_count_check'
			AND conrelid = 'news.digest_runs'::regclass
	) THEN
		ALTER TABLE news.digest_runs
			ADD CONSTRAINT digest_runs_posted_count_check CHECK (posted_count >= 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'digest_runs_window_valid'
			AND conrelid = 'news.digest_runs'::regclass
	) THEN
		ALTER TABLE news.digest_runs
			ADD CONSTRAINT digest_runs_window_valid CHECK (window_end_utc > window_start_utc);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'digest_runs_finished_after_start'
			AND conrelid = 'news.digest_runs'::regclass
	) THEN
		ALTER TABLE news.digest_runs
			ADD CONSTRAINT digest_runs_finished_after_start CHECK (finished_at IS NULL OR finished_at >= started_at);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'digest_entries_rank_positive'
			AND conrelid = 'news.digest_entries'::regclass
	) THEN
		ALTER TABLE news.digest_entries
			ADD CONSTRAINT digest_entries_rank_positive CHECK (rank IS NULL OR rank > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'digest_entries_score_range'
			AND conrelid = 'news.digest_entries'::regclass
	) THEN
		ALTER TABLE news.digest_entries
			ADD CONSTRAINT digest_entries_score_range CHECK (score IS NULL OR (score >= 0 AND score <= 1));
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'translation_sources_source_type_check'
			AND conrelid = 'news.translation_sources'::regclass
	) THEN
		ALTER TABLE news.translation_sources
			ADD CONSTRAINT translation_sources_source_type_check CHECK (length(trim(source_type)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'translation_sources_source_lang_check'
			AND conrelid = 'news.translation_sources'::regclass
	) THEN
		ALTER TABLE news.translation_sources
			ADD CONSTRAINT translation_sources_source_lang_check CHECK (length(trim(source_lang)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'translation_sources_content_hash_check'
			AND conrelid = 'news.translation_sources'::regclass
	) THEN
		ALTER TABLE news.translation_sources
			ADD CONSTRAINT translation_sources_content_hash_check CHECK (octet_length(content_hash) = 32);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'translation_sources_content_origin_check'
			AND conrelid = 'news.translation_sources'::regclass
	) THEN
		ALTER TABLE news.translation_sources
			ADD CONSTRAINT translation_sources_content_origin_check CHECK (content_origin IN ('normalized', 'reader'));
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'translation_results_target_lang_check'
			AND conrelid = 'news.translation_results'::regclass
	) THEN
		ALTER TABLE news.translation_results
			ADD CONSTRAINT translation_results_target_lang_check CHECK (length(trim(target_lang)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'translation_results_provider_name_check'
			AND conrelid = 'news.translation_results'::regclass
	) THEN
		ALTER TABLE news.translation_results
			ADD CONSTRAINT translation_results_provider_name_check CHECK (length(trim(provider_name)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'translation_results_latency_ms_check'
			AND conrelid = 'news.translation_results'::regclass
	) THEN
		ALTER TABLE news.translation_results
			ADD CONSTRAINT translation_results_latency_ms_check CHECK (latency_ms IS NULL OR latency_ms >= 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'users_username_check'
			AND conrelid = 'news.users'::regclass
	) THEN
		ALTER TABLE news.users
			ADD CONSTRAINT users_username_check CHECK (length(trim(username)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'users_password_hash_check'
			AND conrelid = 'news.users'::regclass
	) THEN
		ALTER TABLE news.users
			ADD CONSTRAINT users_password_hash_check CHECK (length(trim(password_hash)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'sessions_expires_after_created'
			AND conrelid = 'news.sessions'::regclass
	) THEN
		ALTER TABLE news.sessions
			ADD CONSTRAINT sessions_expires_after_created CHECK (expires_at > created_at);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'sessions_last_seen_after_created'
			AND conrelid = 'news.sessions'::regclass
	) THEN
		ALTER TABLE news.sessions
			ADD CONSTRAINT sessions_last_seen_after_created CHECK (last_seen_at >= created_at);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'user_settings_preferred_language_check'
			AND conrelid = 'news.user_settings'::regclass
	) THEN
		ALTER TABLE news.user_settings
			ADD CONSTRAINT user_settings_preferred_language_check CHECK (length(trim(preferred_language)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'collection_settings_collection_check'
			AND conrelid = 'news.collection_settings'::regclass
	) THEN
		ALTER TABLE news.collection_settings
			ADD CONSTRAINT collection_settings_collection_check CHECK (length(trim(collection)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'collection_settings_translation_mode_check'
			AND conrelid = 'news.collection_settings'::regclass
	) THEN
		ALTER TABLE news.collection_settings
			ADD CONSTRAINT collection_settings_translation_mode_check CHECK (translation_mode IN ('enabled', 'disabled'));
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'audit_events_action_check'
			AND conrelid = 'news.audit_events'::regclass
	) THEN
		ALTER TABLE news.audit_events
			ADD CONSTRAINT audit_events_action_check CHECK (length(trim(action)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'audit_events_target_type_check'
			AND conrelid = 'news.audit_events'::regclass
	) THEN
		ALTER TABLE news.audit_events
			ADD CONSTRAINT audit_events_target_type_check CHECK (length(trim(target_type)) > 0);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'audit_events_target_id_check'
			AND conrelid = 'news.audit_events'::regclass
	) THEN
		ALTER TABLE news.audit_events
			ADD CONSTRAINT audit_events_target_id_check CHECK (length(trim(target_id)) > 0);
	END IF;
END
$$;

INSERT INTO news.collection_settings (collection, translation_mode, created_at, updated_at)
VALUES
	('china_news', 'enabled', now(), now()),
	('metal_news', 'enabled', now(), now()),
	('openclaw', 'disabled', now(), now())
ON CONFLICT (collection) DO NOTHING;

INSERT INTO news.collection_settings (collection, translation_mode, created_at, updated_at)
SELECT
	collections.collection,
	CASE
		WHEN collections.collection IN ('china_news', 'metal_news') THEN 'enabled'
		ELSE 'disabled'
	END AS translation_mode,
	now(),
	now()
FROM (
	SELECT DISTINCT collection
	FROM news.articles
	WHERE deleted_at IS NULL
	UNION
	SELECT DISTINCT collection
	FROM news.stories
	WHERE deleted_at IS NULL
) collections
WHERE length(trim(collections.collection)) > 0
ON CONFLICT (collection) DO NOTHING;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'source_checkpoints_last_successful_run_id_fkey'
			AND conrelid = 'news.source_checkpoints'::regclass
	) THEN
		ALTER TABLE news.source_checkpoints
			ADD CONSTRAINT source_checkpoints_last_successful_run_id_fkey
			FOREIGN KEY (last_successful_run_id)
			REFERENCES news.ingest_runs(run_id)
			ON DELETE SET NULL;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'raw_arrivals_run_id_fkey'
			AND conrelid = 'news.raw_arrivals'::regclass
	) THEN
		ALTER TABLE news.raw_arrivals
			ADD CONSTRAINT raw_arrivals_run_id_fkey
			FOREIGN KEY (run_id)
			REFERENCES news.ingest_runs(run_id)
			ON DELETE RESTRICT;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'articles_raw_arrival_id_fkey'
			AND conrelid = 'news.articles'::regclass
	) THEN
		ALTER TABLE news.articles
			ADD CONSTRAINT articles_raw_arrival_id_fkey
			FOREIGN KEY (raw_arrival_id)
			REFERENCES news.raw_arrivals(raw_arrival_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'article_embeddings_article_id_fkey'
			AND conrelid = 'news.article_embeddings'::regclass
	) THEN
		ALTER TABLE news.article_embeddings
			ADD CONSTRAINT article_embeddings_article_id_fkey
			FOREIGN KEY (article_id)
			REFERENCES news.articles(article_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'article_tags_article_id_fkey'
			AND conrelid = 'news.article_tags'::regclass
	) THEN
		ALTER TABLE news.article_tags
			ADD CONSTRAINT article_tags_article_id_fkey
			FOREIGN KEY (article_id)
			REFERENCES news.articles(article_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'article_tags_tag_id_fkey'
			AND conrelid = 'news.article_tags'::regclass
	) THEN
		ALTER TABLE news.article_tags
			ADD CONSTRAINT article_tags_tag_id_fkey
			FOREIGN KEY (tag_id)
			REFERENCES news.tags(tag_id)
			ON DELETE RESTRICT;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'article_tags_created_by_user_id_fkey'
			AND conrelid = 'news.article_tags'::regclass
	) THEN
		ALTER TABLE news.article_tags
			ADD CONSTRAINT article_tags_created_by_user_id_fkey
			FOREIGN KEY (created_by_user_id)
			REFERENCES news.users(user_id)
			ON DELETE SET NULL;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'stories_representative_article_id_fkey'
			AND conrelid = 'news.stories'::regclass
	) THEN
		ALTER TABLE news.stories
			ADD CONSTRAINT stories_representative_article_id_fkey
			FOREIGN KEY (representative_article_id)
			REFERENCES news.articles(article_id)
			ON DELETE SET NULL;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'story_articles_story_id_fkey'
			AND conrelid = 'news.story_articles'::regclass
	) THEN
		ALTER TABLE news.story_articles
			ADD CONSTRAINT story_articles_story_id_fkey
			FOREIGN KEY (story_id)
			REFERENCES news.stories(story_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'story_articles_article_id_fkey'
			AND conrelid = 'news.story_articles'::regclass
	) THEN
		ALTER TABLE news.story_articles
			ADD CONSTRAINT story_articles_article_id_fkey
			FOREIGN KEY (article_id)
			REFERENCES news.articles(article_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'dedup_events_article_id_fkey'
			AND conrelid = 'news.dedup_events'::regclass
	) THEN
		ALTER TABLE news.dedup_events
			ADD CONSTRAINT dedup_events_article_id_fkey
			FOREIGN KEY (article_id)
			REFERENCES news.articles(article_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'dedup_events_chosen_story_id_fkey'
			AND conrelid = 'news.dedup_events'::regclass
	) THEN
		ALTER TABLE news.dedup_events
			ADD CONSTRAINT dedup_events_chosen_story_id_fkey
			FOREIGN KEY (chosen_story_id)
			REFERENCES news.stories(story_id)
			ON DELETE SET NULL;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'dedup_events_best_candidate_story_id_fkey'
			AND conrelid = 'news.dedup_events'::regclass
	) THEN
		ALTER TABLE news.dedup_events
			ADD CONSTRAINT dedup_events_best_candidate_story_id_fkey
			FOREIGN KEY (best_candidate_story_id)
			REFERENCES news.stories(story_id)
			ON DELETE SET NULL;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'topic_source_rules_topic_id_fkey'
			AND conrelid = 'news.topic_source_rules'::regclass
	) THEN
		ALTER TABLE news.topic_source_rules
			ADD CONSTRAINT topic_source_rules_topic_id_fkey
			FOREIGN KEY (topic_id)
			REFERENCES news.topics(topic_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'topic_keyword_rules_topic_id_fkey'
			AND conrelid = 'news.topic_keyword_rules'::regclass
	) THEN
		ALTER TABLE news.topic_keyword_rules
			ADD CONSTRAINT topic_keyword_rules_topic_id_fkey
			FOREIGN KEY (topic_id)
			REFERENCES news.topics(topic_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'story_topic_state_story_id_fkey'
			AND conrelid = 'news.story_topic_state'::regclass
	) THEN
		ALTER TABLE news.story_topic_state
			ADD CONSTRAINT story_topic_state_story_id_fkey
			FOREIGN KEY (story_id)
			REFERENCES news.stories(story_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'story_topic_state_topic_id_fkey'
			AND conrelid = 'news.story_topic_state'::regclass
	) THEN
		ALTER TABLE news.story_topic_state
			ADD CONSTRAINT story_topic_state_topic_id_fkey
			FOREIGN KEY (topic_id)
			REFERENCES news.topics(topic_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'digest_runs_topic_id_fkey'
			AND conrelid = 'news.digest_runs'::regclass
	) THEN
		ALTER TABLE news.digest_runs
			ADD CONSTRAINT digest_runs_topic_id_fkey
			FOREIGN KEY (topic_id)
			REFERENCES news.topics(topic_id)
			ON DELETE RESTRICT;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'digest_entries_digest_run_id_fkey'
			AND conrelid = 'news.digest_entries'::regclass
	) THEN
		ALTER TABLE news.digest_entries
			ADD CONSTRAINT digest_entries_digest_run_id_fkey
			FOREIGN KEY (digest_run_id)
			REFERENCES news.digest_runs(digest_run_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'digest_entries_story_id_fkey'
			AND conrelid = 'news.digest_entries'::regclass
	) THEN
		ALTER TABLE news.digest_entries
			ADD CONSTRAINT digest_entries_story_id_fkey
			FOREIGN KEY (story_id)
			REFERENCES news.stories(story_id)
			ON DELETE RESTRICT;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'translation_results_translation_source_id_fkey'
			AND conrelid = 'news.translation_results'::regclass
	) THEN
		ALTER TABLE news.translation_results
			ADD CONSTRAINT translation_results_translation_source_id_fkey
			FOREIGN KEY (translation_source_id)
			REFERENCES news.translation_sources(translation_source_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'sessions_user_id_fkey'
			AND conrelid = 'news.sessions'::regclass
	) THEN
		ALTER TABLE news.sessions
			ADD CONSTRAINT sessions_user_id_fkey
			FOREIGN KEY (user_id)
			REFERENCES news.users(user_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'user_settings_user_id_fkey'
			AND conrelid = 'news.user_settings'::regclass
	) THEN
		ALTER TABLE news.user_settings
			ADD CONSTRAINT user_settings_user_id_fkey
			FOREIGN KEY (user_id)
			REFERENCES news.users(user_id)
			ON DELETE CASCADE;
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'audit_events_actor_user_id_fkey'
			AND conrelid = 'news.audit_events'::regclass
	) THEN
		ALTER TABLE news.audit_events
			ADD CONSTRAINT audit_events_actor_user_id_fkey
			FOREIGN KEY (actor_user_id)
			REFERENCES news.users(user_id)
			ON DELETE SET NULL;
	END IF;
END
$$;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_trigger
		WHERE tgname = 'trg_ingest_runs_touch_updated_at'
			AND tgrelid = 'news.ingest_runs'::regclass
	) THEN
		CREATE TRIGGER trg_ingest_runs_touch_updated_at
		BEFORE UPDATE ON news.ingest_runs
		FOR EACH ROW EXECUTE FUNCTION news.touch_updated_at();
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_trigger
		WHERE tgname = 'trg_articles_touch_updated_at'
			AND tgrelid = 'news.articles'::regclass
	) THEN
		CREATE TRIGGER trg_articles_touch_updated_at
		BEFORE UPDATE ON news.articles
		FOR EACH ROW EXECUTE FUNCTION news.touch_updated_at();
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_trigger
		WHERE tgname = 'trg_tags_touch_updated_at'
			AND tgrelid = 'news.tags'::regclass
	) THEN
		CREATE TRIGGER trg_tags_touch_updated_at
		BEFORE UPDATE ON news.tags
		FOR EACH ROW EXECUTE FUNCTION news.touch_updated_at();
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_trigger
		WHERE tgname = 'trg_stories_touch_updated_at'
			AND tgrelid = 'news.stories'::regclass
	) THEN
		CREATE TRIGGER trg_stories_touch_updated_at
		BEFORE UPDATE ON news.stories
		FOR EACH ROW EXECUTE FUNCTION news.touch_updated_at();
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_trigger
		WHERE tgname = 'trg_topics_touch_updated_at'
			AND tgrelid = 'news.topics'::regclass
	) THEN
		CREATE TRIGGER trg_topics_touch_updated_at
		BEFORE UPDATE ON news.topics
		FOR EACH ROW EXECUTE FUNCTION news.touch_updated_at();
	END IF;
END
$$;

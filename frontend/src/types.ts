export interface JSendSuccess<T> {
  status: "success";
  data: T;
  message?: string;
  code?: number;
}

export interface JSendFailure {
  status: "fail" | "error";
  message?: string;
  data?: unknown;
  code?: number;
}

export type JSendResponse<T> = JSendSuccess<T> | JSendFailure;

export interface StoryRepresentative {
  article_uuid: string;
  source: string;
  source_item_id: string;
  published_at?: string;
}

export interface StoryListItem {
  story_id: number;
  story_uuid: string;
  collection: string;
  translation_mode: "enabled" | "disabled";
  title: string;
  original_title: string;
  translated_title?: string | null;
  detected_language: string;
  canonical_url?: string;
  status: string;
  first_seen_at: string;
  last_seen_at: string;
  source_count: number;
  article_count: number;
  representative?: StoryRepresentative;
}

export interface StoryArticle {
  story_article_uuid: string;
  article_uuid: string;
  source: string;
  source_item_id: string;
  collection: string;
  translation_mode?: "enabled" | "disabled";
  canonical_url?: string;
  published_at?: string;
  normalized_title: string;
  normalized_text?: string;
  detected_language: string;
  original_title: string;
  translated_title?: string | null;
  original_text: string;
  translated_text?: string | null;
  source_domain?: string;
  matched_at: string;
  match_type: string;
  match_score?: number;
  match_details?: Record<string, unknown>;
  dedup_decision?: string;
  dedup_exact_signal?: string;
  dedup_best_cosine?: number;
  dedup_title_overlap?: number;
  dedup_date_consistency?: number;
  dedup_composite_score?: number;
}

export interface StoryDetailResponse {
  story: StoryListItem;
  members: StoryArticle[];
}

export interface StoryArticlePreview {
  story_article_uuid: string;
  preview_text: string;
  source: string;
  char_count: number;
  truncated: boolean;
  preview_error?: string;
}

export interface CollectionSummary {
  collection: string;
  translation_mode: "enabled" | "disabled";
  articles: number;
  stories: number;
  story_items: number;
  last_story_seen_at?: string;
}

export interface CollectionSettings {
  collection: string;
  translation_mode: "enabled" | "disabled";
  created_at: string;
  updated_at: string;
}

export interface StoryDayBucket {
  day: string;
  story_count: number;
}

export interface StoryPagination {
  page: number;
  page_size: number;
  total_items: number;
  total_pages: number;
}

export interface StoriesResponse {
  items: StoryListItem[];
  pagination: StoryPagination;
}

export interface StatsResponse {
  raw_arrivals: number;
  articles: number;
  stories: number;
  story_articles: number;
  dedup_events: number;
  running_ingest_runs: number;
  last_fetched_at?: string;
  last_story_updated?: string;
  dedup_decisions: Record<string, number>;
}

export interface StoryFilters {
  page: number;
  pageSize: number;
  collection: string;
  query: string;
  from: string;
  to: string;
  lang: string;
}

export interface ViewerSearch {
  collection?: string;
  q?: string;
  page?: number;
  day?: string;
  from?: string;
  to?: string;
  lang?: string;
}

export interface DayNavigationState {
  currentIndex: number;
  canGoOlder: boolean;
  canGoNewer: boolean;
  currentLabel: string;
  navigatorDay: string;
  relativeLabel: string;
}

export interface LanguageOption {
  code: string;
  label: string;
  native?: string;
}

export interface AuthUser {
  user_id: number;
  username: string;
  created_at: string;
  last_login_at?: string;
}

export interface UserSettings {
  preferred_language: string;
  password_enabled: boolean;
  ui_prefs: Record<string, unknown>;
}

export interface LoginResponse {
  user: AuthUser;
  settings: UserSettings;
  languages: LanguageOption[];
  session: {
    session_id: string;
    expires_at: string;
  };
}

export interface MeResponse {
  user: AuthUser;
  settings: UserSettings;
  languages: LanguageOption[];
}

export interface MySettingsResponse {
  settings: UserSettings;
}

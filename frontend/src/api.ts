import type {
  LoginResponse,
  MeResponse,
  MySettingsResponse,
  CollectionSettings,
  CollectionSummary,
  JSendResponse,
  LanguageOption,
  StatsResponse,
  StoryDayBucket,
  StoryDetailResponse,
  StoryFilters,
  StoryArticlePreview,
  StoriesResponse,
  Tag,
  UserSettings,
} from "./types";
import { normalizeLanguageCode } from "./lib/language";

interface JSendRequestOptions extends Omit<RequestInit, "body"> {
  bodyJson?: unknown;
}

async function fetchJSend<T>(path: string, options: JSendRequestOptions = {}): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json");

  let body: BodyInit | undefined;
  if (options.bodyJson !== undefined) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(options.bodyJson);
  }

  const response = await fetch(path, {
    ...options,
    headers,
    body,
    credentials: "include",
  });

  const payload = (await response.json().catch(() => ({}))) as Partial<JSendResponse<T>>;
  const isSuccess = payload.status === "success";

  if (!response.ok || !isSuccess || payload.data === undefined) {
    const message =
      typeof payload.message === "string" ? payload.message : `Request failed (${response.status})`;
    throw new Error(message);
  }

  return payload.data as T;
}

function appendLang(params: URLSearchParams, lang?: string): void {
  const trimmed = normalizeLanguageCode(lang || "");
  if (!trimmed) {
    return;
  }
  params.set("lang", trimmed);
}

function withLang(path: string, lang?: string): string {
  const trimmed = normalizeLanguageCode(lang || "");
  if (!trimmed) {
    return path;
  }
  const separator = path.includes("?") ? "&" : "?";
  return `${path}${separator}lang=${encodeURIComponent(trimmed)}`;
}

export async function getStats(): Promise<StatsResponse> {
  return fetchJSend<StatsResponse>("/api/v1/stats");
}

export async function login(username: string, password: string): Promise<LoginResponse> {
  return fetchJSend<LoginResponse>("/api/v1/auth/login", {
    method: "POST",
    bodyJson: { username, password },
  });
}

export async function logout(): Promise<{ logged_out: boolean }> {
  return fetchJSend<{ logged_out: boolean }>("/api/v1/auth/logout", {
    method: "POST",
  });
}

export async function getMe(): Promise<MeResponse> {
  return fetchJSend<MeResponse>("/api/v1/me");
}

export async function getLanguages(): Promise<{ items: LanguageOption[] }> {
  return fetchJSend<{ items: LanguageOption[] }>("/api/v1/languages");
}

export async function getMySettings(): Promise<MySettingsResponse> {
  return fetchJSend<MySettingsResponse>("/api/v1/me/settings");
}

export async function updateMySettings(
  payload: Partial<UserSettings>,
): Promise<MySettingsResponse> {
  return fetchJSend<MySettingsResponse>("/api/v1/me/settings", {
    method: "PUT",
    bodyJson: payload,
  });
}

export async function getCollections(): Promise<{ items: CollectionSummary[] }> {
  return fetchJSend<{ items: CollectionSummary[] }>("/api/v1/collections");
}

export async function getTags(includeArchived = false): Promise<{ items: Tag[] }> {
  const params = new URLSearchParams();
  if (includeArchived) {
    params.set("include_archived", "true");
  }
  const suffix = params.toString() ? `?${params.toString()}` : "";
  return fetchJSend<{ items: Tag[] }>(`/api/v1/tags${suffix}`);
}

export async function updateCollectionSettings(
  collection: string,
  translationMode: "enabled" | "disabled",
): Promise<{ settings: CollectionSettings }> {
  return fetchJSend<{ settings: CollectionSettings }>(
    `/api/v1/collections/${encodeURIComponent(collection)}/settings`,
    {
      method: "PATCH",
      bodyJson: { translation_mode: translationMode },
    },
  );
}

export async function getStoryDays(
  collection: string,
  limit = 45,
): Promise<{ items: StoryDayBucket[] }> {
  const params = new URLSearchParams();
  params.set("limit", String(limit));
  if (collection) {
    params.set("collection", collection);
  }
  return fetchJSend<{ items: StoryDayBucket[] }>(`/api/v1/story-days?${params.toString()}`);
}

export async function getStories(filters: StoryFilters): Promise<StoriesResponse> {
  const params = new URLSearchParams();
  params.set("page", String(filters.page));
  params.set("page_size", String(filters.pageSize));
  if (filters.collection) {
    params.set("collection", filters.collection);
  }
  if (filters.query) {
    params.set("q", filters.query);
  }
  if (filters.from) {
    params.set("from", filters.from);
  }
  if (filters.to) {
    params.set("to", filters.to);
  }
  if (filters.tag) {
    params.set("tag", filters.tag);
  }
  appendLang(params, filters.lang);

  return fetchJSend<StoriesResponse>(`/api/v1/stories?${params.toString()}`);
}

export async function getStoryDetail(storyUUID: string, lang = ""): Promise<StoryDetailResponse> {
  return fetchJSend<StoryDetailResponse>(withLang(`/api/v1/stories/${storyUUID}`, lang));
}

export async function addArticleTag(articleUUID: string, tagSlug: string): Promise<void> {
  await fetchJSend(`/api/v1/articles/${encodeURIComponent(articleUUID)}/tags`, {
    method: "POST",
    bodyJson: { tag_slug: tagSlug },
  });
}

export async function removeArticleTag(articleUUID: string, tagSlug: string): Promise<void> {
  await fetchJSend(
    `/api/v1/articles/${encodeURIComponent(articleUUID)}/tags/${encodeURIComponent(tagSlug)}`,
    {
      method: "DELETE",
    },
  );
}

export async function requestTranslation(
  storyUUID: string,
  targetLang: string,
  provider?: string,
): Promise<{ stats: { translated: number; cached: number; skipped: number; total: number } }> {
  const normalizedTargetLang = normalizeLanguageCode(targetLang);
  const body: Record<string, string> = {
    story_uuid: storyUUID,
    target_lang: normalizedTargetLang || targetLang,
  };
  if (provider) body.provider = provider;
  return fetchJSend<{
    stats: { translated: number; cached: number; skipped: number; total: number };
  }>("/api/v1/translate", {
    method: "POST",
    bodyJson: body,
  });
}

export async function getStoryArticlePreview(
  storyArticleUUID: string,
  maxChars = 1000,
): Promise<StoryArticlePreview> {
  const params = new URLSearchParams();
  params.set("max_chars", String(maxChars));
  return fetchJSend<StoryArticlePreview>(
    `/api/v1/articles/${storyArticleUUID}/preview?${params.toString()}`,
  );
}

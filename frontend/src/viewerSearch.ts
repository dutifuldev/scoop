import type { StoryFilters, ViewerSearch } from "./types";
import { normalizeLanguageCode } from "./lib/language";

export const DEFAULT_PAGE_SIZE = 25;

function normalizeString(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function normalizeDay(value: unknown): string | undefined {
  const day = normalizeString(value);
  if (!day) {
    return undefined;
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) {
    return undefined;
  }
  return day;
}

function normalizePage(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isInteger(value) && value > 0) {
    return value;
  }

  if (typeof value !== "string") {
    return undefined;
  }

  const trimmed = value.trim();
  if (trimmed === "") {
    return undefined;
  }

  const parsed = Number.parseInt(trimmed, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return undefined;
  }

  return parsed;
}

function normalizeLanguage(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const normalized = normalizeLanguageCode(value);
  if (normalized === "") {
    return undefined;
  }
  return normalized;
}

export function normalizeViewerSearch(input: Record<string, unknown>): ViewerSearch {
  const collection = normalizeString(input.collection);
  const q = normalizeString(input.q);
  const day = normalizeDay(input.day);
  const from = normalizeDay(input.from);
  const to = normalizeDay(input.to);
  const page = normalizePage(input.page);
  const lang = normalizeLanguage(input.lang);
  const tag = normalizeString(input.tag);

  const search: ViewerSearch = {};
  if (collection) {
    search.collection = collection;
  }
  if (q) {
    search.q = q;
  }
  if (day) {
    search.day = day;
  }
  if (from) {
    search.from = from;
  }
  if (to) {
    search.to = to;
  }
  if (page && page > 1) {
    search.page = page;
  }
  if (lang) {
    search.lang = lang;
  }
  if (tag) {
    search.tag = tag;
  }

  return search;
}

export function compactViewerSearch(search: ViewerSearch): ViewerSearch {
  return normalizeViewerSearch(search as unknown as Record<string, unknown>);
}

export function toStoryFilters(search: ViewerSearch): StoryFilters {
  return {
    page: search.page && search.page > 0 ? search.page : 1,
    pageSize: DEFAULT_PAGE_SIZE,
    collection: search.collection || "",
    query: search.q || "",
    from: search.from || "",
    to: search.to || "",
    lang: search.lang || "",
    tag: search.tag || "",
  };
}

import type { StoryListItem, StoryArticle, StoryPagination } from "../types";
import { parseDayString } from "./day";

function parseCalendarDay(value: string): Date | null {
  return parseDayString(value) ?? null;
}

function pluralize(value: number, unit: string): string {
  return value === 1 ? `${value} ${unit}` : `${value} ${unit}s`;
}

export function formatCount(value: number | null | undefined): string {
  return Number(value ?? 0).toLocaleString("en-US");
}

function yearInTimeZone(date: Date, timeZone?: string): number {
  const formatted = new Intl.DateTimeFormat("en-US", {
    year: "numeric",
    ...(timeZone ? { timeZone } : {}),
  }).format(date);
  return Number(formatted);
}

export function formatDateTime(value?: string, timeZone?: string): string {
  if (!value) {
    return "n/a";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "n/a";
  }

  const now = new Date();
  const sameYear = yearInTimeZone(date, timeZone) === yearInTimeZone(now, timeZone);

  const options: Intl.DateTimeFormatOptions = sameYear
    ? {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
        ...(timeZone ? { timeZone } : {}),
      }
    : {
        month: "short",
        day: "numeric",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
        ...(timeZone ? { timeZone } : {}),
      };

  return date.toLocaleString("en-US", options);
}

export function formatBylineDate(value?: string, now = new Date(), timeZone?: string): string {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  const diffMs = now.getTime() - date.getTime();
  const minuteMs = 60 * 1000;
  const hourMs = 60 * minuteMs;
  const dayMs = 24 * hourMs;

  if (diffMs >= 0 && diffMs < minuteMs) {
    return "now";
  }
  if (diffMs >= 0 && diffMs < hourMs) {
    return `${Math.max(1, Math.floor(diffMs / minuteMs))}m`;
  }
  if (diffMs >= 0 && diffMs < dayMs) {
    return `${Math.max(1, Math.floor(diffMs / hourMs))}h`;
  }

  const sameYear = yearInTimeZone(date, timeZone) === yearInTimeZone(now, timeZone);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    ...(timeZone ? { timeZone } : {}),
    ...(sameYear ? {} : { year: "numeric" }),
  });
}

export function formatCalendarDay(value: string): string {
  const date = parseCalendarDay(value);
  if (!date) {
    return "Pick a day";
  }
  return date.toLocaleDateString("en-US", {
    weekday: "short",
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export function formatRelativeDay(value: string): string {
  const date = parseCalendarDay(value);
  if (!date) {
    return "unknown day";
  }

  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const msPerDay = 24 * 60 * 60 * 1000;
  const diffDays = Math.round((today.getTime() - date.getTime()) / msPerDay);

  if (diffDays === 0) return "today";
  if (diffDays === 1) return "yesterday";
  if (diffDays === -1) return "tomorrow";

  if (diffDays > 1) {
    if (diffDays < 7) return `${diffDays} days ago`;
    if (diffDays < 30) return `${pluralize(Math.floor(diffDays / 7), "week")} ago`;
    if (diffDays < 365) return `${pluralize(Math.floor(diffDays / 30), "month")} ago`;
    return `${pluralize(Math.floor(diffDays / 365), "year")} ago`;
  }

  const future = Math.abs(diffDays);
  if (future < 7) return `in ${pluralize(future, "day")}`;
  if (future < 30) return `in ${pluralize(Math.floor(future / 7), "week")}`;
  if (future < 365) return `in ${pluralize(Math.floor(future / 30), "month")}`;
  return `in ${pluralize(Math.floor(future / 365), "year")}`;
}

export function extractErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim() !== "") {
    return error.message;
  }
  return "Request failed.";
}

export function buildStoryMetaText(lastSeenAt: string, sourceCount: number): string {
  return `last ${formatDateTime(lastSeenAt)} • ${sourceCount} sources`;
}

function domainFromURL(value?: string): string {
  if (!value) {
    return "";
  }

  const input = value.trim();
  if (!input) {
    return "";
  }

  const normalizeHost = (hostname: string): string => {
    const host = hostname.toLowerCase().trim();
    if (!host) {
      return "";
    }
    return host.startsWith("www.") ? host.slice(4) : host;
  };

  try {
    return normalizeHost(new URL(input).hostname);
  } catch {
    // Allow URLs that arrive without an explicit scheme.
    try {
      return normalizeHost(new URL(`https://${input}`).hostname);
    } catch {
      return "";
    }
  }
}

export function buildFeedSourceText(story: StoryListItem): string {
  const count = Math.max(0, Number(story.source_count || 0));
  const domain = domainFromURL(story.canonical_url);
  const sourceFallback = story.representative?.source?.trim() || "";
  const primary = domain || sourceFallback;

  if (!primary) {
    return `${formatCount(count)} source${count === 1 ? "" : "s"}`;
  }

  if (count <= 1) {
    return primary;
  }

  const others = count - 1;
  return `${primary} and ${formatCount(others)} other${others === 1 ? "" : "s"}`;
}

export function buildFeedMetaText(story: StoryListItem, includeTimestamp = false): string {
  const sourceText = buildFeedSourceText(story);
  if (!includeTimestamp) {
    return sourceText;
  }

  const timestamp =
    story.published_at ||
    story.representative?.published_at ||
    story.last_seen_at ||
    story.first_seen_at;
  const timeText = formatDateTime(timestamp);

  if (timeText === "n/a") {
    return sourceText;
  }
  if (!sourceText) {
    return timeText;
  }
  return `${timeText} • ${sourceText}`;
}

export function buildMemberSubtitle(member: StoryArticle): string {
  const scoreSuffix =
    member.match_score == null ? "" : ` • score ${Number(member.match_score).toFixed(3)}`;
  return `${member.source}:${member.source_item_id} • ${member.match_type}${scoreSuffix}`;
}

export function buildPagination(
  page: number,
  pageSize: number,
  incoming?: Partial<StoryPagination>,
): StoryPagination {
  return {
    page: incoming?.page ?? page,
    page_size: incoming?.page_size ?? pageSize,
    total_items: Number(incoming?.total_items ?? 0),
    total_pages: Math.max(1, Number(incoming?.total_pages ?? 1)),
  };
}

import { useState, type ReactNode } from "react";
import { Copy, ExternalLink } from "lucide-react";

import discordLogoURL from "../../assets/discord.svg";

export const discordMessagePattern =
  /^https?:\/\/discord\.com\/channels\/([^/]+)\/([^/]+)\/([^/?#]+)/i;

const inlineLinkPattern =
  /\[([^\]]+)\]\(([^)\s]+)\)|(https?:\/\/[^\s)\]}>,]+|(?:[a-z0-9-]+\.)+[a-z]{2,}(?:\/[^\s)\]}>,]*)?)/gi;

function trimTrailingURLPunctuation(rawURL: string): { url: string; trailing: string } {
  const match = rawURL.match(/[.,;:!?]+$/);
  if (!match) {
    return { url: rawURL, trailing: "" };
  }
  return { url: rawURL.slice(0, -match[0].length), trailing: match[0] };
}

function normalizeURLTarget(rawURL: string): { url: string; trailing: string } | null {
  const { url, trailing } = trimTrailingURLPunctuation(rawURL);

  if (/^https?:\/\//i.test(url)) {
    return { url, trailing };
  }

  if (/^(?:www\.|(?:[a-z0-9-]+\.)+[a-z]{2,}(?:[/:?#]|$))/i.test(url)) {
    return { url: `https://${url}`, trailing };
  }

  return null;
}

export function DiscordLinkIcon(): JSX.Element {
  return <img className="discord-link-icon" src={discordLogoURL} alt="" aria-hidden="true" />;
}

function discordAppURL(url: string): string {
  const match = url.match(discordMessagePattern);
  if (!match) {
    return url;
  }

  return `discord://-/channels/${match[1]}/${match[2]}/${match[3]}`;
}

export async function copyTextToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard?.writeText(text);
    if (navigator.clipboard) {
      return true;
    }
  } catch {
    // Fall back below for HTTP or denied Clipboard API contexts.
  }

  let textarea: HTMLTextAreaElement | null = null;
  try {
    textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.setAttribute("readonly", "true");
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    textarea.style.top = "0";
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();
    textarea.setSelectionRange(0, text.length);
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    textarea?.remove();
  }
}

export function buildStoryShareURL(collection: string, storyUUID: string): string {
  const normalizedStoryUUID = storyUUID.trim();
  if (!normalizedStoryUUID) {
    return "";
  }

  const normalizedCollection = collection.trim();
  const storySegment = encodeURIComponent(normalizedStoryUUID);
  const path = normalizedCollection
    ? `/c/${encodeURIComponent(normalizedCollection)}/s/${storySegment}`
    : `/stories/${storySegment}`;

  if (typeof window === "undefined" || !window.location?.origin) {
    return path;
  }

  return new URL(path, window.location.origin).toString();
}

interface StoryTitleCopyButtonProps {
  title: string;
  collection: string;
  storyUUID: string;
}

export function StoryTitleCopyButton({
  title,
  collection,
  storyUUID,
}: StoryTitleCopyButtonProps): JSX.Element {
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">("idle");
  const displayTitle = title || "(untitled)";
  const statusText =
    copyState === "copied"
      ? "Copied story link"
      : copyState === "failed"
        ? "Failed to copy story link"
        : "Copy story link";

  async function copyStoryLink(): Promise<void> {
    const shareURL = buildStoryShareURL(collection, storyUUID);
    if (!shareURL) {
      setCopyState("failed");
      window.setTimeout(() => setCopyState("idle"), 1400);
      return;
    }

    const copied = await copyTextToClipboard(shareURL);
    setCopyState(copied ? "copied" : "failed");
    window.setTimeout(() => setCopyState("idle"), 1400);
  }

  return (
    <button
      type="button"
      className={`detail-title-copy-button ${copyState !== "idle" ? "is-active" : ""}`.trim()}
      onClick={() => {
        void copyStoryLink();
      }}
      title={statusText}
      aria-label={statusText}
    >
      {displayTitle}
    </button>
  );
}

interface DiscordMessageLinkProps {
  url: string;
  label?: string;
  className?: string;
  compact?: boolean;
}

export function DiscordMessageLink({
  url,
  label,
  className = "",
  compact = false,
}: DiscordMessageLinkProps): JSX.Element {
  const visibleLabel = label || labelForURL(url);
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">("idle");

  async function copyBrowserLink(): Promise<void> {
    const copied = await copyTextToClipboard(url);
    setCopyState(copied ? "copied" : "failed");
    window.setTimeout(() => setCopyState("idle"), 1400);
  }

  return (
    <span className="discord-link-wrap">
      <a
        className={`${className || "inline-discord-link"} discord-app-link`.trim()}
        href={discordAppURL(url)}
        title={url}
        aria-label={compact ? visibleLabel : undefined}
      >
        <DiscordLinkIcon />
        {compact ? <span className="sr-only">{visibleLabel}</span> : visibleLabel}
      </a>
      <span className="discord-link-actions" aria-label="Discord link actions">
        <a
          className="discord-link-action"
          href={url}
          target="_blank"
          rel="noreferrer"
          title="Open in browser"
          aria-label="Open Discord message in browser"
        >
          <ExternalLink className="h-3 w-3" aria-hidden="true" />
          Browser
        </a>
        <button
          type="button"
          className="discord-link-action"
          onClick={() => {
            void copyBrowserLink();
          }}
          title="Copy link"
          aria-label="Copy Discord message link"
        >
          <Copy className="h-3 w-3" aria-hidden="true" />
          {copyState === "copied" ? "Copied" : copyState === "failed" ? "Failed" : "Copy"}
        </button>
      </span>
    </span>
  );
}

interface TitleSourceLinkProps {
  url: string;
}

export function TitleSourceLink({ url }: TitleSourceLinkProps): JSX.Element | null {
  const trimmedURL = url.trim();
  if (!trimmedURL) {
    return null;
  }

  const label = labelForURL(trimmedURL);
  if (discordMessagePattern.test(trimmedURL)) {
    return (
      <DiscordMessageLink
        url={trimmedURL}
        label={label}
        className="title-source-link title-source-link-discord"
        compact
      />
    );
  }

  return (
    <a
      className="title-source-link"
      href={trimmedURL}
      target="_blank"
      rel="noreferrer"
      title={trimmedURL}
      aria-label={label}
    >
      <ExternalLink className="title-source-link-icon" aria-hidden="true" />
      <span className="sr-only">{label}</span>
    </a>
  );
}

export function labelForURL(url: string): string {
  const discordMatch = url.match(discordMessagePattern);
  if (discordMatch) {
    return "Discord message";
  }

  try {
    const parsed = new URL(url);
    return parsed.hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

function renderInlineLinks(text: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  let lastIndex = 0;

  for (const match of text.matchAll(inlineLinkPattern)) {
    const start = match.index ?? 0;
    const markdownLabel = match[1];
    const markdownURL = match[2];
    const rawURL = match[3];
    const rawMatch = match[0];
    const normalizedURL = normalizeURLTarget(markdownURL || rawURL || "");

    if (!normalizedURL) {
      continue;
    }
    const { url, trailing } = normalizedURL;

    if (start > lastIndex) {
      nodes.push(text.slice(lastIndex, start));
    }

    const isDiscordMessage = discordMessagePattern.test(url);
    nodes.push(
      isDiscordMessage ? (
        <DiscordMessageLink
          key={`${url}-${start}`}
          url={url}
          label={markdownLabel || labelForURL(url)}
        />
      ) : (
        <a
          key={`${url}-${start}`}
          className="inline-rich-link"
          href={url}
          target="_blank"
          rel="noreferrer"
          title={url}
        >
          {markdownLabel || labelForURL(url)}
        </a>
      ),
    );
    if (!markdownURL && trailing) {
      nodes.push(trailing);
    }
    lastIndex = start + rawMatch.length;
  }

  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }

  return nodes;
}

export function renderTextBlock(paragraph: string, key: string): JSX.Element {
  const heading = paragraph.match(/^(#{1,3})\s+(.+)$/);
  if (heading) {
    const levelClass = `detail-markdown-heading detail-markdown-heading-${heading[1].length}`;
    return (
      <p key={key} className={levelClass}>
        {renderInlineLinks(heading[2])}
      </p>
    );
  }

  const lines = paragraph
    .split(/\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  const bulletLines = lines
    .map((line) => line.match(/^[-*]\s+(.+)$/)?.[1])
    .filter((line): line is string => Boolean(line));

  if (bulletLines.length === lines.length) {
    return (
      <ul key={key} className="detail-markdown-list">
        {bulletLines.map((line, index) => (
          <li key={`${key}-bullet-${index}`}>{renderInlineLinks(line)}</li>
        ))}
      </ul>
    );
  }

  return (
    <p key={key} className="detail-item-content-text">
      {renderInlineLinks(paragraph)}
    </p>
  );
}

export function buildMemberPreview(text?: string): string {
  const collapsed = (text ?? "").replace(/\s+/g, " ").trim();
  if (!collapsed) {
    return "No content captured for this item.";
  }

  const maxChars = 260;
  if (collapsed.length <= maxChars) {
    return collapsed;
  }
  return `${collapsed.slice(0, maxChars).trimEnd()}...`;
}

export function toParagraphs(text: string): string[] {
  return text
    .split(/\n+/)
    .map((paragraph) => paragraph.trim())
    .filter((paragraph) => paragraph.length > 0);
}

import type { ReactNode } from "react";

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
      <a
        key={`${url}-${start}`}
        className={isDiscordMessage ? "inline-discord-link" : "inline-rich-link"}
        href={url}
        target="_blank"
        rel="noreferrer"
        title={url}
      >
        {isDiscordMessage ? <DiscordLinkIcon /> : null}
        {markdownLabel || labelForURL(url)}
      </a>,
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

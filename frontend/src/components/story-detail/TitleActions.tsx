import { useState, type MouseEventHandler, type ReactNode } from "react";
import { ExternalLink } from "lucide-react";
import type { CSSProperties } from "react";

import {
  copyTextToClipboard,
  DiscordMessageLink,
  discordMessagePattern,
  labelForURL,
} from "./storyTextRendering";

interface TitleActionsProps {
  children: ReactNode;
  className?: string;
}

export function TitleActions({ children, className = "" }: TitleActionsProps): JSX.Element {
  return <div className={`title-actions ${className}`.trim()}>{children}</div>;
}

interface TitleActionButtonProps {
  children: ReactNode;
  ariaLabel: string;
  title?: string;
  disabled?: boolean;
  onClick?: MouseEventHandler<HTMLButtonElement>;
  className?: string;
}

export function TitleActionButton({
  children,
  ariaLabel,
  title,
  disabled = false,
  onClick,
  className = "",
}: TitleActionButtonProps): JSX.Element {
  return (
    <button
      type="button"
      className={`title-action title-action-button ${className}`.trim()}
      aria-label={ariaLabel}
      title={title || ariaLabel}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

interface TitleActionLinkProps {
  href: string;
  label: string;
  title?: string;
  children: ReactNode;
  className?: string;
}

export function TitleActionLink({
  href,
  label,
  title,
  children,
  className = "",
}: TitleActionLinkProps): JSX.Element {
  return (
    <a
      className={`title-action title-action-link ${className}`.trim()}
      href={href}
      target="_blank"
      rel="noreferrer"
      title={title || href}
      aria-label={label}
    >
      {children}
    </a>
  );
}

interface TitleTagProps {
  children: ReactNode;
  style?: CSSProperties;
  className?: string;
}

export function TitleTag({ children, style, className = "" }: TitleTagProps): JSX.Element {
  return (
    <span className={`tag-chip title-tag ${className}`.trim()} style={style}>
      {children}
    </span>
  );
}

interface TitleTagInputProps {
  children: ReactNode;
  onMouseDown?: MouseEventHandler<HTMLDivElement>;
  className?: string;
}

export function TitleTagInput({
  children,
  onMouseDown,
  className = "",
}: TitleTagInputProps): JSX.Element {
  return (
    <div
      className={`member-tag-input-shell is-active title-tag-input ${className}`.trim()}
      aria-label="Article tag input"
      onMouseDown={onMouseDown}
    >
      {children}
    </div>
  );
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
    >
      {displayTitle}
    </button>
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
        className="title-action title-action-link title-source-link title-source-link-discord"
        compact
      />
    );
  }

  return (
    <TitleActionLink
      href={trimmedURL}
      label={label}
      title={trimmedURL}
      className="title-source-link"
    >
      <ExternalLink className="title-action-icon title-source-link-icon" aria-hidden="true" />
      <span className="sr-only">{label}</span>
    </TitleActionLink>
  );
}

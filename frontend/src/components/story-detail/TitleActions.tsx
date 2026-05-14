import type { CSSProperties, MouseEventHandler, ReactNode } from "react";
import { ExternalLink } from "lucide-react";

import githubLogoURL from "../../assets/github.svg";
import { classifySourceLink } from "../../lib/sourceLinks";
import { DiscordMessageLink } from "./storyTextRendering";

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

interface TitleSourceLinkProps {
  url: string;
}

export function TitleSourceLink({ url }: TitleSourceLinkProps): JSX.Element | null {
  const trimmedURL = url.trim();
  if (!trimmedURL) {
    return null;
  }

  const sourceLink = classifySourceLink(trimmedURL);
  if (sourceLink.kind === "discord-message") {
    return (
      <DiscordMessageLink
        url={trimmedURL}
        label={sourceLink.label}
        className="title-action title-action-link title-source-link title-source-link-discord"
        compact
      />
    );
  }

  if (
    sourceLink.kind === "github-issue" ||
    sourceLink.kind === "github-pr" ||
    sourceLink.kind === "github"
  ) {
    const numberedLink = sourceLink.kind === "github-issue" || sourceLink.kind === "github-pr";
    return (
      <TitleActionLink
        href={sourceLink.url}
        label={sourceLink.label}
        title={sourceLink.url}
        className={`title-source-link title-source-link-github ${
          numberedLink ? "title-source-link-github-numbered" : ""
        }`.trim()}
      >
        <img
          className="title-action-icon title-source-link-icon title-source-link-github-icon"
          src={githubLogoURL}
          alt=""
          aria-hidden="true"
        />
        {numberedLink ? (
          <span className="title-source-link-number">#{sourceLink.number}</span>
        ) : null}
        <span className="sr-only">{sourceLink.label}</span>
      </TitleActionLink>
    );
  }

  return (
    <TitleActionLink
      href={trimmedURL}
      label={sourceLink.label}
      title={trimmedURL}
      className="title-source-link"
    >
      <ExternalLink className="title-action-icon title-source-link-icon" aria-hidden="true" />
      <span className="sr-only">{sourceLink.label}</span>
    </TitleActionLink>
  );
}

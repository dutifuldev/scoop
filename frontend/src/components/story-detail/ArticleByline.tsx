import { useEffect, useState, type ReactNode } from "react";

import {
  cleanIdentityHandle,
  personIdentityLabel,
  primaryPersonIdentity,
} from "../../lib/identityFormat";
import { getViewerTimeZone } from "../../lib/viewerTimeZone";
import { formatBylineDate } from "../../lib/viewerFormat";
import type { PersonIdentity } from "../../types";
import { ProviderIcon } from "./ProviderIcon";

interface ArticleBylineProps {
  identities?: PersonIdentity[];
  publishedAt?: string;
  source?: string;
  dateTitle?: string;
  children?: ReactNode;
}

function initialsFor(label: string): string {
  const words = label
    .replace(/^@+/, "")
    .split(/[^A-Za-z0-9]+/)
    .map((word) => word.trim())
    .filter(Boolean);

  if (words.length >= 2) {
    return `${words[0][0]}${words[1][0]}`.toUpperCase();
  }
  if (words.length === 1) {
    return words[0].slice(0, 2).toUpperCase();
  }
  return "?";
}

function githubProfileURL(identity: PersonIdentity | null, handle: string): string {
  if (identity?.provider.toLowerCase() !== "github" || !handle) {
    return "";
  }
  if (!/^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$/.test(handle)) {
    return "";
  }
  return `https://github.com/${handle}`;
}

export function ArticleByline({
  identities = [],
  publishedAt,
  source = "",
  dateTitle = "",
  children,
}: ArticleBylineProps): JSX.Element {
  const identity = primaryPersonIdentity(identities);
  const displayName = identity?.display_name?.trim() || "";
  const rawHandle = identity?.handle?.trim() || "";
  const handle = rawHandle ? cleanIdentityHandle(rawHandle) : "";
  const fallbackLabel = identity ? personIdentityLabel(identity) : source.trim();
  const visibleIdentity = handle ? `@${handle}` : fallbackLabel;
  const identityURL = githubProfileURL(identity, handle);
  const avatarLabel = displayName || handle || fallbackLabel || source || "article";
  const avatarURL = identity?.avatar_url?.trim() || "";
  const [avatarLoadFailed, setAvatarLoadFailed] = useState(false);
  const bylineDate = formatBylineDate(publishedAt, new Date(), getViewerTimeZone());

  useEffect(() => {
    setAvatarLoadFailed(false);
  }, [avatarURL]);

  return (
    <div className="article-byline" aria-label="Article byline">
      <span className="article-byline-rail" aria-hidden="true">
        <span className="article-byline-avatar">
          {avatarURL && !avatarLoadFailed ? (
            <img
              className="article-byline-avatar-image"
              src={avatarURL}
              alt=""
              onError={() => setAvatarLoadFailed(true)}
            />
          ) : (
            initialsFor(avatarLabel)
          )}
        </span>
      </span>
      <div className="article-byline-main">
        <div className="article-byline-identity">
          {displayName ? <span className="article-byline-name">{displayName}</span> : null}
          {identityURL ? (
            <a
              className="article-byline-handle article-byline-handle-link"
              href={identityURL}
              target="_blank"
              rel="noreferrer"
            >
              {visibleIdentity}
            </a>
          ) : visibleIdentity ? (
            <span className="article-byline-handle">{visibleIdentity}</span>
          ) : null}
          {identity ? (
            <ProviderIcon
              provider={identity.provider}
              className="discord-link-icon article-byline-provider-icon"
            />
          ) : null}
          {bylineDate ? (
            <>
              <span className="article-byline-dot" aria-hidden="true">
                &middot;
              </span>
              <time className="article-byline-date" dateTime={publishedAt} title={dateTitle}>
                {bylineDate}
              </time>
            </>
          ) : null}
        </div>
        {children ? <div className="article-byline-content">{children}</div> : null}
      </div>
    </div>
  );
}

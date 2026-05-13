import { useEffect, useState, type ReactNode } from "react";

import discordLogoURL from "../../assets/discord.svg";
import {
  cleanIdentityHandle,
  personIdentityLabel,
  primaryPersonIdentity,
} from "../../lib/identityFormat";
import { formatBylineDate } from "../../lib/viewerFormat";
import type { PersonIdentity } from "../../types";

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

function providerIcon(identity: PersonIdentity): JSX.Element | null {
  if (identity.provider.toLowerCase() !== "discord") {
    return null;
  }

  return (
    <img
      className="discord-link-icon article-byline-provider-icon"
      src={discordLogoURL}
      alt=""
      aria-hidden="true"
    />
  );
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
  const avatarLabel = displayName || handle || fallbackLabel || source || "article";
  const avatarURL = identity?.avatar_url?.trim() || "";
  const [avatarLoadFailed, setAvatarLoadFailed] = useState(false);
  const bylineDate = formatBylineDate(publishedAt);

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
          {visibleIdentity ? (
            <span className="article-byline-handle">{visibleIdentity}</span>
          ) : null}
          {identity ? providerIcon(identity) : null}
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

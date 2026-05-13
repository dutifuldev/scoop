import type { ReactNode } from "react";

import discordLogoURL from "../../assets/discord.svg";
import { formatBylineDate } from "../../lib/viewerFormat";
import type { PersonIdentity } from "../../types";
import { personIdentityLabel } from "./ArticlePersonIdentityEditor";

interface ArticleBylineProps {
  identities?: PersonIdentity[];
  publishedAt?: string;
  source?: string;
  children?: ReactNode;
}

function cleanHandle(handle: string): string {
  return handle.trim().replace(/^@+/, "");
}

function primaryIdentity(identities: PersonIdentity[] = []): PersonIdentity | null {
  const active = identities.filter((identity) => !identity.archived_at);
  if (active.length === 0) {
    return null;
  }

  return [...active].sort((a, b) => {
    const left = [
      a.handle || "",
      a.provider || "",
      a.provider_user_id || "",
      a.person_identity_uuid || "",
    ].join(":");
    const right = [
      b.handle || "",
      b.provider || "",
      b.provider_user_id || "",
      b.person_identity_uuid || "",
    ].join(":");
    return left.localeCompare(right);
  })[0];
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
    <img className="article-byline-provider-icon" src={discordLogoURL} alt="" aria-hidden="true" />
  );
}

export function ArticleByline({
  identities = [],
  publishedAt,
  source = "",
  children,
}: ArticleBylineProps): JSX.Element {
  const identity = primaryIdentity(identities);
  const displayName = identity?.display_name?.trim() || "";
  const rawHandle = identity?.handle?.trim() || "";
  const handle = rawHandle ? cleanHandle(rawHandle) : "";
  const fallbackLabel = identity ? personIdentityLabel(identity) : source.trim();
  const visibleIdentity = handle ? `@${handle}` : fallbackLabel;
  const avatarLabel = displayName || handle || fallbackLabel || source || "article";
  const bylineDate = formatBylineDate(publishedAt);

  return (
    <div className="article-byline" aria-label="Article byline">
      <span className="article-byline-avatar" aria-hidden="true">
        {initialsFor(avatarLabel)}
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
              <time className="article-byline-date" dateTime={publishedAt}>
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

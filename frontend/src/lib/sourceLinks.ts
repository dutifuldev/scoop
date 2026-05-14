export type SourceLink =
  | {
      kind: "discord-message";
      url: string;
      label: string;
    }
  | {
      kind: "github-issue";
      url: string;
      label: string;
      owner: string;
      repo: string;
      number: string;
    }
  | {
      kind: "github-pr";
      url: string;
      label: string;
      owner: string;
      repo: string;
      number: string;
    }
  | {
      kind: "github";
      url: string;
      label: string;
    }
  | {
      kind: "external";
      url: string;
      label: string;
    };

export const discordMessagePattern =
  /^https?:\/\/discord\.com\/channels\/([^/]+)\/([^/]+)\/([^/?#]+)/i;

function hostnameLabel(url: string): string {
  try {
    const parsed = new URL(url);
    return parsed.hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

export function classifySourceLink(rawURL: string): SourceLink {
  const url = rawURL.trim();
  if (discordMessagePattern.test(url)) {
    return {
      kind: "discord-message",
      url,
      label: "Discord message",
    };
  }

  try {
    const parsed = new URL(url);
    const host = parsed.hostname.toLowerCase().replace(/^www\./, "");
    if (host === "github.com") {
      const segments = parsed.pathname.split("/").filter(Boolean);
      const [owner, repo, resource, number] = segments;
      if (owner && repo && resource === "issues" && /^\d+$/.test(number ?? "")) {
        return {
          kind: "github-issue",
          url,
          label: `GitHub issue #${number} in ${owner}/${repo}`,
          owner,
          repo,
          number,
        };
      }
      if (owner && repo && resource === "pull" && /^\d+$/.test(number ?? "")) {
        return {
          kind: "github-pr",
          url,
          label: `GitHub PR #${number} in ${owner}/${repo}`,
          owner,
          repo,
          number,
        };
      }
      return {
        kind: "github",
        url,
        label: owner && repo ? `GitHub link to ${owner}/${repo}` : "GitHub link",
      };
    }
  } catch {
    // Fall through to the generic external link label.
  }

  return {
    kind: "external",
    url,
    label: hostnameLabel(url),
  };
}

export function labelForURL(url: string): string {
  return classifySourceLink(url).label;
}

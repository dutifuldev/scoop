import { describe, expect, it } from "vitest";

import { classifySourceLink, labelForURL } from "./sourceLinks";

describe("classifySourceLink", () => {
  it("classifies Discord message links", () => {
    expect(classifySourceLink("https://discord.com/channels/1/2/3")).toEqual({
      kind: "discord-message",
      url: "https://discord.com/channels/1/2/3",
      label: "Discord message",
    });
  });

  it("classifies GitHub issue links with their issue number", () => {
    expect(classifySourceLink("https://github.com/openai/codex/issues/1234")).toEqual({
      kind: "github-issue",
      url: "https://github.com/openai/codex/issues/1234",
      label: "GitHub issue #1234 in openai/codex",
      owner: "openai",
      repo: "codex",
      number: "1234",
    });
  });

  it("classifies GitHub pull request links with their pull request number", () => {
    expect(classifySourceLink("https://github.com/openai/codex/pull/5678")).toEqual({
      kind: "github-pr",
      url: "https://github.com/openai/codex/pull/5678",
      label: "GitHub PR #5678 in openai/codex",
      owner: "openai",
      repo: "codex",
      number: "5678",
    });
  });

  it("keeps generic GitHub links as GitHub links without numbers", () => {
    expect(classifySourceLink("https://github.com/openai/codex")).toEqual({
      kind: "github",
      url: "https://github.com/openai/codex",
      label: "GitHub link to openai/codex",
    });
  });

  it("uses host labels for external links", () => {
    expect(labelForURL("https://example.com/path")).toBe("example.com");
  });
});

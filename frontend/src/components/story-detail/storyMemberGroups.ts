import type { StoryArticle, StoryDetailResponse } from "../../types";

export interface MemberURLGroup {
  key: string;
  canonicalURL: string;
  members: StoryArticle[];
  representative: StoryArticle;
  sourceCount: number;
}

export function memberGroupKey(member: StoryArticle): string {
  return `member:${member.story_article_uuid}`;
}

export function buildMemberGroups(detail: StoryDetailResponse | null): MemberURLGroup[] {
  if (!detail) {
    return [];
  }

  return detail.members.map((member) => ({
    key: memberGroupKey(member),
    canonicalURL: member.canonical_url?.trim() ?? "",
    members: [member],
    representative: member,
    sourceCount: 1,
  }));
}

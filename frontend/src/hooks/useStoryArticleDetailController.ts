import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { addArticleTag, removeArticleTag, requestTranslation } from "../api";
import {
  buildMemberGroups,
  type MemberURLGroup,
} from "../components/story-detail/storyMemberGroups";
import {
  defaultCollectionTranslationMode,
  isCollectionTranslationEnabled,
} from "../lib/collectionTranslation";
import type { StoryDetailResponse, StoryArticlePreview } from "../types";
import { useStoryArticlePreviews } from "./useStoryArticlePreviews";

export type StoryDetailTextMode = "translated" | "original";

export function storyDetailQueryKey(storyUUID: string, language: string): [string, string, string] {
  return ["story-detail", storyUUID, language];
}

interface UseStoryArticleDetailControllerOptions {
  storyUUID: string;
  detail: StoryDetailResponse | null;
  activeLang: string;
  isTranslationActive: boolean;
  onTranslationStateChange?: (storyUUID: string, isTranslating: boolean) => void;
}

interface UseStoryArticleDetailControllerResult {
  detailTextMode: StoryDetailTextMode;
  setDetailTextMode: (mode: StoryDetailTextMode) => void;
  translationError: string;
  tagMutationKey: string;
  tagMutationError: string;
  showTranslationProgress: boolean;
  memberGroups: MemberURLGroup[];
  itemPreviewByUUID: Record<string, StoryArticlePreview>;
  itemPreviewLoadingByUUID: Record<string, boolean>;
  itemPreviewErrorByUUID: Record<string, string>;
  onAddArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
  onRemoveArticleTag: (articleUUID: string, tagSlug: string) => Promise<void>;
}

export function useStoryArticleDetailController({
  storyUUID,
  detail,
  activeLang,
  isTranslationActive,
  onTranslationStateChange,
}: UseStoryArticleDetailControllerOptions): UseStoryArticleDetailControllerResult {
  const queryClient = useQueryClient();
  const [detailTextMode, setDetailTextMode] = useState<StoryDetailTextMode>(
    activeLang ? "translated" : "original",
  );
  const [isTranslating, setIsTranslating] = useState(false);
  const [translationError, setTranslationError] = useState("");
  const [tagMutationKey, setTagMutationKey] = useState("");
  const [tagMutationError, setTagMutationError] = useState("");
  const translationRequestedRef = useRef("");
  const activeTranslationKeyRef = useRef("");
  const { itemPreviewByUUID, itemPreviewLoadingByUUID, itemPreviewErrorByUUID } =
    useStoryArticlePreviews(detail);

  const hasPendingTranslations = useMemo(() => {
    if (!activeLang || !detail) {
      return false;
    }

    const translatedTitle = (detail.story.translated_title || "").trim();
    const hasUntranslatedBody = detail.members.some((member) => {
      const mode = member.translation_mode ?? defaultCollectionTranslationMode(member.collection);
      return isCollectionTranslationEnabled(mode) && !(member.translated_text || "").trim();
    });
    return translatedTitle === "" || hasUntranslatedBody;
  }, [activeLang, detail]);

  const memberGroups = useMemo<MemberURLGroup[]>(() => buildMemberGroups(detail), [detail]);

  const showTranslationProgress =
    activeLang !== "" &&
    isTranslating &&
    activeTranslationKeyRef.current === `${storyUUID}:${activeLang}`;

  useEffect(() => {
    setDetailTextMode(activeLang ? "translated" : "original");
  }, [activeLang]);

  useEffect(() => {
    setTranslationError("");
  }, [storyUUID, activeLang]);

  useEffect(() => {
    if (!isTranslationActive || !activeLang || !detail || !storyUUID || !hasPendingTranslations) {
      return;
    }

    const reqKey = `${storyUUID}:${activeLang}`;
    if (translationRequestedRef.current === reqKey) {
      return;
    }
    translationRequestedRef.current = reqKey;
    activeTranslationKeyRef.current = reqKey;
    setTranslationError("");
    setIsTranslating(true);
    onTranslationStateChange?.(storyUUID, true);

    void requestTranslation(storyUUID, activeLang)
      .then(() =>
        Promise.all([
          queryClient.invalidateQueries({
            queryKey: storyDetailQueryKey(storyUUID, activeLang),
            exact: true,
          }),
          queryClient.invalidateQueries({ queryKey: ["stories"] }),
        ]).then(() =>
          Promise.all([
            queryClient.refetchQueries({
              queryKey: storyDetailQueryKey(storyUUID, activeLang),
              exact: true,
              type: "active",
            }),
            queryClient.refetchQueries({ queryKey: ["stories"], type: "active" }),
          ]),
        ),
      )
      .catch((err) => {
        translationRequestedRef.current = "";
        setTranslationError(err instanceof Error ? err.message : "Failed to translate story.");
      })
      .finally(() => {
        if (activeTranslationKeyRef.current === reqKey) {
          activeTranslationKeyRef.current = "";
          setIsTranslating(false);
        }
        onTranslationStateChange?.(storyUUID, false);
      });
  }, [
    activeLang,
    detail,
    hasPendingTranslations,
    isTranslationActive,
    onTranslationStateChange,
    queryClient,
    storyUUID,
  ]);

  async function refreshTagsAfterMutation(): Promise<void> {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["story-detail", storyUUID] }),
      queryClient.invalidateQueries({ queryKey: ["stories"] }),
    ]);
  }

  async function onAddArticleTag(articleUUID: string, tagSlug: string): Promise<void> {
    if (!articleUUID || !tagSlug || tagSlug === "__add_tag__") {
      return;
    }

    const mutationKey = `${articleUUID}:${tagSlug}:add`;
    setTagMutationKey(mutationKey);
    setTagMutationError("");
    try {
      await addArticleTag(articleUUID, tagSlug);
      await refreshTagsAfterMutation();
    } catch (err) {
      setTagMutationError(err instanceof Error ? err.message : "Failed to add tag.");
      throw err;
    } finally {
      setTagMutationKey("");
    }
  }

  async function onRemoveArticleTag(articleUUID: string, tagSlug: string): Promise<void> {
    if (!articleUUID || !tagSlug) {
      return;
    }

    const mutationKey = `${articleUUID}:${tagSlug}:remove`;
    setTagMutationKey(mutationKey);
    setTagMutationError("");
    try {
      await removeArticleTag(articleUUID, tagSlug);
      await refreshTagsAfterMutation();
    } catch (err) {
      setTagMutationError(err instanceof Error ? err.message : "Failed to remove tag.");
    } finally {
      setTagMutationKey("");
    }
  }

  return {
    detailTextMode,
    setDetailTextMode,
    translationError,
    tagMutationKey,
    tagMutationError,
    showTranslationProgress,
    memberGroups,
    itemPreviewByUUID,
    itemPreviewLoadingByUUID,
    itemPreviewErrorByUUID,
    onAddArticleTag,
    onRemoveArticleTag,
  };
}

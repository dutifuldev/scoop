import { useEffect, useState } from "react";

import { getStoryArticlePreview } from "../api";
import type { StoryArticlePreview, StoryDetailResponse } from "../types";

const previewRequestBatchSize = 4;
const previewRequestDebounceMs = 120;
const expandedArticlePreviewMaxChars = 4000;

function pruneRecord<T>(record: Record<string, T>, validIDs: Set<string>): Record<string, T> {
  const next: Record<string, T> = {};
  let changed = false;

  for (const [key, value] of Object.entries(record)) {
    if (validIDs.has(key)) {
      next[key] = value;
      continue;
    }
    changed = true;
  }

  if (!changed && Object.keys(next).length === Object.keys(record).length) {
    return record;
  }
  return next;
}

interface StoryArticlePreviewState {
  itemPreviewByUUID: Record<string, StoryArticlePreview>;
  itemPreviewLoadingByUUID: Record<string, boolean>;
  itemPreviewErrorByUUID: Record<string, string>;
}

export function useStoryArticlePreviews(
  detail: StoryDetailResponse | null,
): StoryArticlePreviewState {
  const [itemPreviewByUUID, setItemPreviewByUUID] = useState<Record<string, StoryArticlePreview>>(
    {},
  );
  const [itemPreviewLoadingByUUID, setItemPreviewLoadingByUUID] = useState<Record<string, boolean>>(
    {},
  );
  const [itemPreviewRequestedByUUID, setItemPreviewRequestedByUUID] = useState<
    Record<string, boolean>
  >({});
  const [itemPreviewErrorByUUID, setItemPreviewErrorByUUID] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!detail) {
      setItemPreviewByUUID({});
      setItemPreviewLoadingByUUID({});
      setItemPreviewRequestedByUUID({});
      setItemPreviewErrorByUUID({});
      return;
    }

    const validItemIDs = new Set(detail.members.map((member) => member.story_article_uuid));
    setItemPreviewByUUID((previous) => pruneRecord(previous, validItemIDs));
    setItemPreviewLoadingByUUID((previous) => pruneRecord(previous, validItemIDs));
    setItemPreviewRequestedByUUID((previous) => pruneRecord(previous, validItemIDs));
    setItemPreviewErrorByUUID((previous) => pruneRecord(previous, validItemIDs));
  }, [detail]);

  useEffect(() => {
    if (!detail) {
      return;
    }

    const pendingUUIDs = detail.members
      .map((member) => member.story_article_uuid)
      .filter((itemUUID) => !itemPreviewRequestedByUUID[itemUUID]);
    if (pendingUUIDs.length === 0) {
      return;
    }

    const timer = window.setTimeout(() => {
      const batch = pendingUUIDs.slice(0, previewRequestBatchSize);
      if (batch.length === 0) {
        return;
      }

      setItemPreviewRequestedByUUID((previous) => {
        const next = { ...previous };
        for (const itemUUID of batch) {
          next[itemUUID] = true;
        }
        return next;
      });
      setItemPreviewLoadingByUUID((previous) => {
        const next = { ...previous };
        for (const itemUUID of batch) {
          next[itemUUID] = true;
        }
        return next;
      });
      setItemPreviewErrorByUUID((previous) => {
        let changed = false;
        const next = { ...previous };
        for (const itemUUID of batch) {
          if (next[itemUUID]) {
            delete next[itemUUID];
            changed = true;
          }
        }
        return changed ? next : previous;
      });

      for (const itemUUID of batch) {
        void getStoryArticlePreview(itemUUID, expandedArticlePreviewMaxChars)
          .then((preview) => {
            setItemPreviewByUUID((previous) => ({
              ...previous,
              [itemUUID]: preview,
            }));
          })
          .catch((fetchErr) => {
            const message =
              fetchErr instanceof Error ? fetchErr.message : "Failed to fetch reader preview.";
            setItemPreviewErrorByUUID((previous) => ({
              ...previous,
              [itemUUID]: message,
            }));
          })
          .finally(() => {
            setItemPreviewLoadingByUUID((previous) => {
              if (!previous[itemUUID]) {
                return previous;
              }
              const next = { ...previous };
              delete next[itemUUID];
              return next;
            });
          });
      }
    }, previewRequestDebounceMs);

    return () => {
      window.clearTimeout(timer);
    };
  }, [detail, itemPreviewRequestedByUUID]);

  return {
    itemPreviewByUUID,
    itemPreviewLoadingByUUID,
    itemPreviewErrorByUUID,
  };
}

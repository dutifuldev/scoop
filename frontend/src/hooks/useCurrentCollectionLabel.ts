import { useMemo } from "react";

import type { CollectionSummary } from "../types";

export function useCurrentCollectionLabel(
  collections: CollectionSummary[],
  selectedCollection: string,
): string {
  return useMemo(() => {
    if (!selectedCollection) {
      return "All collections";
    }

    const matched = collections.find((collection) => collection.collection === selectedCollection);
    return matched?.collection || selectedCollection;
  }, [collections, selectedCollection]);
}

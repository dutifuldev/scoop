export const translationModeEnabled = "enabled";
export const translationModeDisabled = "disabled";

export type CollectionTranslationMode =
  | typeof translationModeEnabled
  | typeof translationModeDisabled;

export function normalizeCollectionTranslationMode(mode: unknown): CollectionTranslationMode {
  return mode === translationModeEnabled ? translationModeEnabled : translationModeDisabled;
}

export function defaultCollectionTranslationMode(collection: string): CollectionTranslationMode {
  switch (collection.trim().toLowerCase()) {
    case "china_news":
    case "metal_news":
      return translationModeEnabled;
    default:
      return translationModeDisabled;
  }
}

export function isCollectionTranslationEnabled(mode: unknown): boolean {
  return normalizeCollectionTranslationMode(mode) === translationModeEnabled;
}

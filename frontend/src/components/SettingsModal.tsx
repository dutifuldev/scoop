import { useEffect, useMemo, useState } from "react";

import type { LanguageOption } from "../types";
import { Button } from "./ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger } from "./ui/select";

interface SettingsModalProps {
  open: boolean;
  preferredLanguage: string;
  languageOptions: LanguageOption[];
  isSaving: boolean;
  error: string;
  onClose: () => void;
  onSave: (preferredLanguage: string) => Promise<void>;
}

export function SettingsModal({
  open,
  preferredLanguage,
  languageOptions,
  isSaving,
  error,
  onClose,
  onSave,
}: SettingsModalProps): JSX.Element | null {
  const [draftLanguage, setDraftLanguage] = useState(preferredLanguage);

  useEffect(() => {
    if (!open) {
      return;
    }
    setDraftLanguage(preferredLanguage);
  }, [open, preferredLanguage]);

  const options = useMemo(() => {
    const seen = new Set<string>();
    const items: LanguageOption[] = [];

    for (const option of languageOptions) {
      const code = option.code.trim();
      if (!code || seen.has(code)) {
        continue;
      }
      seen.add(code);
      items.push(option);
    }

    if (preferredLanguage && !seen.has(preferredLanguage)) {
      items.unshift({
        code: preferredLanguage,
        label: preferredLanguage.toUpperCase(),
      });
    }

    return items;
  }, [languageOptions, preferredLanguage]);

  if (!open) {
    return null;
  }

  return (
    <div className="settings-overlay" onClick={onClose} role="presentation">
      <section
        className="settings-modal card"
        role="dialog"
        aria-modal="true"
        aria-label="User settings"
        onClick={(event) => event.stopPropagation()}
      >
        <header className="settings-header">
          <h2>Settings</h2>
        </header>

        <div className="settings-content">
          <label className="field">
            <span>Translate articles to:</span>
            <Select value={draftLanguage} onValueChange={setDraftLanguage}>
              <SelectTrigger
                variant="default"
                className="settings-select-trigger"
                aria-label="Translate articles to"
              >
                <span>
                  {options.find((option) => option.code === draftLanguage)?.label ??
                    draftLanguage.toUpperCase()}
                </span>
              </SelectTrigger>
              <SelectContent className="settings-select-content">
                {options.map((option) => (
                  <SelectItem key={option.code} value={option.code}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </label>

          {error ? <p className="banner-error settings-error">{error}</p> : null}
        </div>

        <footer className="settings-actions">
          <Button type="button" variant="outline" onClick={onClose} disabled={isSaving}>
            Cancel
          </Button>
          <Button
            type="button"
            onClick={() => {
              void onSave(draftLanguage);
            }}
            disabled={isSaving}
          >
            {isSaving ? "Saving..." : "Save"}
          </Button>
        </footer>
      </section>
    </div>
  );
}

import { useEffect, useRef, useState } from "react";
import { Plus, X } from "lucide-react";

import type { PersonIdentity } from "../../types";
import { Input } from "../ui/input";
import { TitleActionButton, TitleTagInput } from "./TitleActions";

interface ArticlePersonIdentityEditorProps {
  articleUUID: string;
  identities: PersonIdentity[];
  mutationKey: string;
  variant?: "default" | "title";
  onAddIdentity: (articleUUID: string, identityRef: string) => Promise<void>;
  onRemoveIdentity: (articleUUID: string, identityRefOrUUID: string) => Promise<void>;
}

export function personIdentityLabel(identity: PersonIdentity): string {
  if (identity.handle?.trim()) {
    return `@${identity.handle.trim()}`;
  }
  if (identity.provider_user_id?.trim()) {
    return identity.provider_user_id.trim();
  }
  return identity.provider.trim() || "person";
}

export function personIdentityTitle(identity: PersonIdentity): string {
  const label = personIdentityLabel(identity);
  return `${identity.provider}:${label}`;
}

export function ArticlePersonIdentityEditor({
  articleUUID,
  identities,
  mutationKey,
  variant = "default",
  onAddIdentity,
  onRemoveIdentity,
}: ArticlePersonIdentityEditorProps): JSX.Element {
  const [isInputActive, setIsInputActive] = useState(false);
  const [inputValue, setInputValue] = useState("");
  const blurTimerRef = useRef<number | null>(null);
  const isTitleVariant = variant === "title";

  useEffect(() => {
    setIsInputActive(false);
    setInputValue("");
  }, [articleUUID]);

  useEffect(() => {
    return () => {
      if (blurTimerRef.current !== null) {
        window.clearTimeout(blurTimerRef.current);
      }
    };
  }, []);

  function clearBlurTimer(): void {
    if (blurTimerRef.current !== null) {
      window.clearTimeout(blurTimerRef.current);
      blurTimerRef.current = null;
    }
  }

  function openInput(): void {
    clearBlurTimer();
    setIsInputActive(true);
    setInputValue("");
  }

  function closeInput(): void {
    setIsInputActive(false);
    setInputValue("");
  }

  async function addIdentity(): Promise<void> {
    const trimmed = inputValue.trim();
    if (!trimmed) {
      return;
    }
    try {
      await onAddIdentity(articleUUID, trimmed);
      closeInput();
    } catch {
      // The parent owns the visible mutation error; keep the input open for retry.
    }
  }

  const renderedEditTokens = identities.map((identity) => {
    const label = personIdentityLabel(identity);
    const title = personIdentityTitle(identity);
    const removeMutationKey = `${articleUUID}:${identity.person_identity_uuid}:remove`;
    return (
      <span key={identity.person_identity_uuid} className="person-edit-token" title={title}>
        <span>{label}</span>
        <button
          type="button"
          className="person-edit-token-remove"
          onClick={() => {
            void onRemoveIdentity(articleUUID, identity.person_identity_uuid);
          }}
          disabled={mutationKey === removeMutationKey}
          aria-label={`Remove ${title}`}
        >
          <X className="person-chip-remove-icon" aria-hidden="true" />
        </button>
      </span>
    );
  });

  const renderedRow = (
    <div
      className={`person-identity-row ${isTitleVariant ? "person-identity-row-title" : ""}`.trim()}
      aria-label="Article person identity controls"
    >
      {isTitleVariant ? (
        <TitleActionButton
          ariaLabel="Add article person identity"
          title="Add person"
          onClick={openInput}
        >
          <Plus className="title-action-icon" aria-hidden="true" />
        </TitleActionButton>
      ) : (
        <button
          type="button"
          className="member-tag-add-button"
          aria-label="Add article person identity"
          title="Add person"
          onClick={openInput}
        >
          <Plus className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      )}
    </div>
  );

  const renderedInput = (
    <div className="person-identity-input-wrap">
      <Input
        value={inputValue}
        onFocus={() => {
          clearBlurTimer();
          setIsInputActive(true);
        }}
        onBlur={() => {
          blurTimerRef.current = window.setTimeout(closeInput, 120);
        }}
        onChange={(event) => setInputValue(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault();
            void addIdentity();
            return;
          }
          if (event.key === "Escape") {
            closeInput();
          }
        }}
        className="member-tag-input person-identity-input"
        placeholder="id://provider/..."
        aria-label="Person identity ref"
        autoComplete="off"
        autoFocus
        spellCheck={false}
      />
    </div>
  );

  if (isTitleVariant) {
    return (
      <div className="person-identity-tools person-identity-tools-title">
        {!isInputActive ? (
          renderedRow
        ) : (
          <TitleTagInput className="person-identity-input-shell-title" onMouseDown={clearBlurTimer}>
            {renderedEditTokens}
            {renderedInput}
          </TitleTagInput>
        )}
      </div>
    );
  }

  return (
    <div className="person-identity-tools">
      {!isInputActive ? (
        renderedRow
      ) : (
        <div
          className="member-tag-input-shell is-active person-identity-input-shell"
          aria-label="Article person identity input"
          onMouseDown={clearBlurTimer}
        >
          {renderedEditTokens}
          {renderedInput}
        </div>
      )}
    </div>
  );
}

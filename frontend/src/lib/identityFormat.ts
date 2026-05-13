import type { PersonIdentity } from "../types";

export function activePersonIdentities(identities: PersonIdentity[] = []): PersonIdentity[] {
  return identities.filter((identity) => !identity.archived_at);
}

export function hasActivePersonIdentity(identities: PersonIdentity[] = []): boolean {
  return activePersonIdentities(identities).length > 0;
}

export function cleanIdentityHandle(handle: string): string {
  return handle.trim().replace(/^@+/, "");
}

export function personIdentityLabel(identity: PersonIdentity): string {
  if (identity.handle?.trim()) {
    return `@${identity.handle.trim()}`;
  }
  if (identity.provider_user_id?.trim()) {
    return `${identity.provider}:${identity.provider_user_id.trim()}`;
  }
  return identity.provider.trim() || "person";
}

export function primaryPersonIdentity(identities: PersonIdentity[] = []): PersonIdentity | null {
  const active = activePersonIdentities(identities);
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

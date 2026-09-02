import type { LayoutProfileIdentity } from "@/lib/layout/layout-profiles";

const DOCKVIEW_ENV_LAYOUT_PROFILE_PREFIX = "kandev.dockview.env-layout-profile-v1.";

function isLayoutProfileIdentity(value: unknown): value is LayoutProfileIdentity {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const profile = value as { kind?: unknown; id?: unknown };
  return (
    (profile.kind === "built-in" || profile.kind === "custom") &&
    typeof profile.id === "string" &&
    profile.id.length > 0
  );
}

/** Read the profile identity saved with a task environment's layout. */
export function getEnvLayoutProfile(envId: string): LayoutProfileIdentity | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.sessionStorage.getItem(`${DOCKVIEW_ENV_LAYOUT_PROFILE_PREFIX}${envId}`);
    if (!raw) return null;
    const profile: unknown = JSON.parse(raw);
    return isLayoutProfileIdentity(profile) ? profile : null;
  } catch {
    return null;
  }
}

/** Save the profile identity that produced a task environment's layout. */
export function setEnvLayoutProfile(envId: string, profile: LayoutProfileIdentity): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(
      `${DOCKVIEW_ENV_LAYOUT_PROFILE_PREFIX}${envId}`,
      JSON.stringify(profile),
    );
  } catch {
    // Ignore write failures (storage full, blocked, SSR).
  }
}

/** Remove the profile identity saved for a task environment. */
export function removeEnvLayoutProfile(envId: string): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.removeItem(`${DOCKVIEW_ENV_LAYOUT_PROFILE_PREFIX}${envId}`);
  } catch {
    // Ignore removal failures.
  }
}

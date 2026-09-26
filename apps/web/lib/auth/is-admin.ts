import type { AuthMode } from "@/lib/state/slices/auth/types";

/**
 * Role gate for install-wide controls, mirroring `authn.RequireAdmin` on the
 * backend.
 *
 * Mode is read alongside the role because "no user" means two different
 * things. With authentication disabled the boot payload carries no user and
 * the backend injects a synthetic *admin* identity on every request, so the
 * UI must match or turning auth off would hide the controls it is meant to
 * leave untouched. With authentication enabled, "no user" instead means the
 * session was cleared: `clearAuthenticated()` nulls the user but leaves mode
 * "enabled" while the redirect to /login is in flight, and reading that as
 * admin would flash install-wide controls at a signed-out caller.
 *
 * A missing mode is the slice default, which is "disabled" for backends old
 * enough to send no auth block at all.
 */
export function isAdminIdentity(mode: AuthMode | undefined, role: string | undefined): boolean {
  if (mode === undefined || mode === "disabled") return true;
  return role === "admin";
}

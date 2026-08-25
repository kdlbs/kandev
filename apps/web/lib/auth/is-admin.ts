/**
 * Role gate for install-wide controls, mirroring `authn.RequireAdmin` on the
 * backend.
 *
 * An absent role means the boot payload carried no user, which is the
 * auth-disabled single-user mode. The backend injects a synthetic *admin*
 * identity on every request there, so the UI must treat it as admin too --
 * otherwise turning auth off would hide the controls it is meant to leave
 * untouched.
 */
export function isAdminRole(role: string | undefined): boolean {
  return role === undefined || role === "admin";
}

import { describe, expect, it } from "vitest";
import { isAdminIdentity } from "./is-admin";

describe("isAdminIdentity", () => {
  it("treats an admin role as admin while auth is enabled", () => {
    expect(isAdminIdentity("enabled", "admin")).toBe(true);
  });

  it("treats a member role as not admin", () => {
    expect(isAdminIdentity("enabled", "member")).toBe(false);
  });

  // Auth disabled: the boot payload carries no user, and the backend's
  // synthetic single-user identity is an admin. This is the case that must
  // keep every install-wide control available in single-user mode.
  it("treats the auth-disabled single user as admin", () => {
    expect(isAdminIdentity("disabled", undefined)).toBe(true);
  });

  // A backend old enough to send no auth block at all hydrates the slice
  // defaults, which are mode "disabled".
  it("treats a missing auth block as the disabled single user", () => {
    expect(isAdminIdentity(undefined, undefined)).toBe(true);
  });

  // Session expiry calls clearAuthenticated(), which nulls the user but
  // leaves mode "enabled" while the redirect to /login is in flight. Reading
  // that as admin would flash install-wide controls at a signed-out caller.
  it("does not treat a cleared session as admin", () => {
    expect(isAdminIdentity("enabled", undefined)).toBe(false);
  });

  it("does not treat setup mode as admin", () => {
    expect(isAdminIdentity("setup", undefined)).toBe(false);
  });

  it("treats an unknown role as not admin", () => {
    expect(isAdminIdentity("enabled", "guest")).toBe(false);
  });
});

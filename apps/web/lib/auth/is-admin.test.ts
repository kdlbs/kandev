import { describe, expect, it } from "vitest";
import { isAdminRole } from "./is-admin";

describe("isAdminRole", () => {
  it("treats an admin role as admin", () => {
    expect(isAdminRole("admin")).toBe(true);
  });

  it("treats a member role as not admin", () => {
    expect(isAdminRole("member")).toBe(false);
  });

  // Auth disabled: the boot payload carries no user, and the backend's
  // synthetic single-user identity is an admin. The UI must match, or
  // single-user mode would lose every install-wide control.
  it("treats an absent role as admin", () => {
    expect(isAdminRole(undefined)).toBe(true);
  });

  it("treats an unknown role as not admin", () => {
    expect(isAdminRole("guest")).toBe(false);
  });
});

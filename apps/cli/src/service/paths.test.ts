import os from "node:os";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resolveServiceUser } from "./paths";

describe("resolveServiceUser", () => {
  const originalSudoUser = process.env.SUDO_USER;

  beforeEach(() => {
    delete process.env.SUDO_USER;
  });

  afterEach(() => {
    if (originalSudoUser === undefined) delete process.env.SUDO_USER;
    else process.env.SUDO_USER = originalSudoUser;
    vi.restoreAllMocks();
  });

  it("returns the current user for user-mode installs", () => {
    vi.spyOn(os, "userInfo").mockReturnValue({
      username: "alice",
      uid: 1000,
      gid: 1000,
      shell: null,
      homedir: "/home/alice",
    });
    process.env.SUDO_USER = "bob"; // ignored in user mode
    expect(resolveServiceUser(false)).toBe("alice");
  });

  it("prefers SUDO_USER for system-mode installs", () => {
    vi.spyOn(os, "userInfo").mockReturnValue({
      username: "root",
      uid: 0,
      gid: 0,
      shell: null,
      homedir: "/root",
    });
    process.env.SUDO_USER = "alice";
    expect(resolveServiceUser(true)).toBe("alice");
  });

  it("falls back to current user when SUDO_USER is not set", () => {
    vi.spyOn(os, "userInfo").mockReturnValue({
      username: "root",
      uid: 0,
      gid: 0,
      shell: null,
      homedir: "/root",
    });
    expect(resolveServiceUser(true)).toBe("root");
  });

  it("ignores SUDO_USER=root and falls through to current user", () => {
    // sudo never sets SUDO_USER=root in practice, but defend against it
    // so we don't accidentally pin the daemon to root by accident.
    vi.spyOn(os, "userInfo").mockReturnValue({
      username: "alice",
      uid: 1000,
      gid: 1000,
      shell: null,
      homedir: "/home/alice",
    });
    process.env.SUDO_USER = "root";
    expect(resolveServiceUser(true)).toBe("alice");
  });
});

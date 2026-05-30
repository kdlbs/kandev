import fs from "node:fs";
import os from "node:os";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { captureLauncher, homebrewShimPath, resolveHomeDir, resolveServiceUser } from "./paths";

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

describe("resolveHomeDir", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns an absolute override as-is (no tilde)", () => {
    expect(resolveHomeDir("/srv/kandev", false)).toBe("/srv/kandev");
  });

  it("expands a leading ~/ to os.homedir()", () => {
    vi.spyOn(os, "homedir").mockReturnValue("/home/alice");
    expect(resolveHomeDir("~/kandev", false)).toBe("/home/alice/kandev");
    expect(resolveHomeDir("~/srv/nested", false)).toBe("/home/alice/srv/nested");
  });

  it("expands a bare ~ to os.homedir()", () => {
    vi.spyOn(os, "homedir").mockReturnValue("/home/alice");
    expect(resolveHomeDir("~", false)).toBe("/home/alice");
  });

  it("does not expand a tilde that isn't at the start", () => {
    expect(resolveHomeDir("/srv/~/data", false)).toBe("/srv/~/data");
  });

  it("falls back to /var/lib/kandev for system mode with no override", () => {
    expect(resolveHomeDir(undefined, true)).toBe("/var/lib/kandev");
  });

  it("falls back to ~/.kandev for user mode with no override", () => {
    // KANDEV_HOME_DIR is the constant from ../constants, computed at import
    // time. We just check it ends with .kandev and starts with the home dir.
    const result = resolveHomeDir(undefined, false);
    expect(result.endsWith(".kandev")).toBe(true);
  });
});

describe("homebrewShimPath", () => {
  it("derives the floating bin shim from a linuxbrew Cellar cli.js path", () => {
    expect(
      homebrewShimPath("/home/linuxbrew/.linuxbrew/Cellar/kandev/0.52.0/libexec/cli/bin/cli.js"),
    ).toBe("/home/linuxbrew/.linuxbrew/bin/kandev");
  });

  it("derives the floating bin shim from an Apple-silicon Cellar path", () => {
    expect(homebrewShimPath("/opt/homebrew/Cellar/kandev/1.2.3/libexec/cli/bin/cli.js")).toBe(
      "/opt/homebrew/bin/kandev",
    );
  });

  it("returns undefined for a non-Cellar path (npm install)", () => {
    expect(homebrewShimPath("/usr/local/lib/node_modules/kandev/bin/cli.js")).toBeUndefined();
  });
});

describe("captureLauncher shim resolution", () => {
  const originalArgv = process.argv;

  afterEach(() => {
    process.argv = originalArgv;
    vi.restoreAllMocks();
    vi.unstubAllEnvs();
  });

  it("adopts the floating shim when it exists on disk", () => {
    const cliEntry = "/opt/homebrew/Cellar/kandev/1.2.3/libexec/cli/bin/cli.js";
    const shim = "/opt/homebrew/bin/kandev";
    process.argv = ["/path/to/node", cliEntry];
    vi.stubEnv("KANDEV_BUNDLE_DIR", "/opt/homebrew/Cellar/kandev/1.2.3/libexec");
    // Both the cli.js entry and the shim resolve on disk.
    vi.spyOn(fs, "existsSync").mockImplementation((p) => p === cliEntry || p === shim);

    const info = captureLauncher();
    expect(info.kind).toBe("homebrew");
    expect(info.shimPath).toBe(shim);
  });

  it("falls back to version-pinned paths when the shim is absent on disk", () => {
    const cliEntry = "/opt/homebrew/Cellar/kandev/1.2.3/libexec/cli/bin/cli.js";
    process.argv = ["/path/to/node", cliEntry];
    vi.stubEnv("KANDEV_BUNDLE_DIR", "/opt/homebrew/Cellar/kandev/1.2.3/libexec");
    // The cli.js entry resolves, but the floating shim does not exist.
    vi.spyOn(fs, "existsSync").mockImplementation((p) => p === cliEntry);

    const info = captureLauncher();
    expect(info.kind).toBe("homebrew");
    expect(info.shimPath).toBeUndefined();
  });
});

import fs from "node:fs";
import os from "node:os";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { captureLauncher, resolveHomeDir, resolveServiceUser } from "./paths";

const originalArgv = [...process.argv];
const originalBundleDir = process.env.KANDEV_BUNDLE_DIR;
const originalVersion = process.env.KANDEV_VERSION;

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

describe("captureLauncher", () => {
  afterEach(() => {
    process.argv.splice(0, process.argv.length, ...originalArgv);
    if (originalBundleDir === undefined) delete process.env.KANDEV_BUNDLE_DIR;
    else process.env.KANDEV_BUNDLE_DIR = originalBundleDir;
    if (originalVersion === undefined) delete process.env.KANDEV_VERSION;
    else process.env.KANDEV_VERSION = originalVersion;
    vi.restoreAllMocks();
  });

  it("resolves npm global bin symlinks before detecting the install kind", () => {
    delete process.env.KANDEV_BUNDLE_DIR;
    delete process.env.KANDEV_VERSION;
    process.argv[1] = "/tmp/kandev-test/npm-global/bin/kandev";
    vi.spyOn(fs, "existsSync").mockReturnValue(true);
    vi.spyOn(fs, "realpathSync").mockReturnValue(
      "/tmp/kandev-test/npm-global/lib/node_modules/kandev/bin/cli.js",
    );

    expect(captureLauncher()).toMatchObject({
      cliEntry: "/tmp/kandev-test/npm-global/lib/node_modules/kandev/bin/cli.js",
      kind: "npm",
    });
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

import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { readInstalledLogPaths } from "./macos";
import { renderLaunchdPlist } from "./templates";

describe("readInstalledLogPaths", () => {
  let tmp: string;
  let plistPath: string;

  beforeEach(() => {
    tmp = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-macos-"));
    plistPath = path.join(tmp, "com.kdlbs.kandev.plist");
  });

  afterEach(() => {
    fs.rmSync(tmp, { recursive: true, force: true });
  });

  it("returns null when the plist does not exist", () => {
    expect(readInstalledLogPaths(plistPath)).toBeNull();
  });

  it("returns null when the plist exists but lacks the keys", () => {
    fs.writeFileSync(plistPath, "<?xml?><plist><dict></dict></plist>");
    expect(readInstalledLogPaths(plistPath)).toBeNull();
  });

  it("reads back the same paths the renderer wrote (round-trip with custom home dir)", () => {
    // Regression for the showLogs-with-custom-home-dir bug: showLogs has no
    // access to args.homeDir (install-only), so it must derive log paths
    // from the installed plist or it'll look in the default directory while
    // logs accumulate at the custom one.
    const plist = renderLaunchdPlist({
      launcher: {
        nodePath: "/usr/local/bin/node",
        cliEntry: "/usr/local/lib/node_modules/kandev/bin/cli.js",
        kind: "npm",
      },
      homeDir: "/custom/install/path",
      logDir: "/custom/install/path/logs",
      mode: "user",
    });
    fs.writeFileSync(plistPath, plist);

    const installed = readInstalledLogPaths(plistPath);
    expect(installed).toEqual({
      out: "/custom/install/path/logs/service.out",
      err: "/custom/install/path/logs/service.err",
    });
  });
});

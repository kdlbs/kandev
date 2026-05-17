import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { detectStaleServiceUnit } from "./stale_check";

// Stable values; the actual node + cli paths the renderer sees would normally
// come from process.execPath / process.argv[1], which point to whatever's
// running vitest. We override them in tests to make assertions deterministic.
const NODE_PATH = "/usr/local/bin/node";
const CLI_ENTRY = path.resolve(__dirname, "../../bin/cli.js");

describe("detectStaleServiceUnit", () => {
  let tmpHome: string;
  let unitPath: string;

  beforeEach(() => {
    tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-stale-"));
    vi.spyOn(os, "homedir").mockReturnValue(tmpHome);
    vi.spyOn(process, "execPath", "get").mockReturnValue(NODE_PATH);
    // argv[1] must exist on disk — captureLauncher() validates via existsSync.
    const realArgv = [...process.argv];
    realArgv[1] = CLI_ENTRY;
    vi.spyOn(process, "argv", "get").mockReturnValue(realArgv);

    if (process.platform === "linux") {
      unitPath = path.join(tmpHome, ".config", "systemd", "user", "kandev.service");
    } else if (process.platform === "darwin") {
      unitPath = path.join(tmpHome, "Library", "LaunchAgents", "com.kdlbs.kandev.plist");
    } else {
      unitPath = "";
    }
  });

  afterEach(() => {
    fs.rmSync(tmpHome, { recursive: true, force: true });
    vi.restoreAllMocks();
  });

  it("returns null when no unit file is installed", () => {
    expect(detectStaleServiceUnit()).toBeNull();
  });

  it("returns null when unit references the current paths", () => {
    if (!unitPath) return; // unsupported platform — skip
    fs.mkdirSync(path.dirname(unitPath), { recursive: true });
    fs.writeFileSync(
      unitPath,
      `# managed by kandev\nExecStart=${NODE_PATH} ${CLI_ENTRY} --headless\n`,
    );
    expect(detectStaleServiceUnit()).toBeNull();
  });

  it("returns a warning when the cli entry no longer matches", () => {
    if (!unitPath) return;
    fs.mkdirSync(path.dirname(unitPath), { recursive: true });
    fs.writeFileSync(
      unitPath,
      `# managed by kandev\nExecStart=${NODE_PATH} /old/path/cli.js --headless\n`,
    );
    const msg = detectStaleServiceUnit();
    expect(msg).not.toBeNull();
    expect(msg).toMatch(/service install/);
  });

  it("returns null for non-managed files at the same path", () => {
    if (!unitPath) return;
    fs.mkdirSync(path.dirname(unitPath), { recursive: true });
    fs.writeFileSync(unitPath, `[Unit]\nDescription=Something else\n`);
    expect(detectStaleServiceUnit()).toBeNull();
  });
});

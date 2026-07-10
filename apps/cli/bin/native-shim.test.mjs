import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import shim from "./native-shim.js";

const { binaryName, platformPackage, resolveRuntime, validateRuntime } = shim;

function createBundle(dir) {
  fs.mkdirSync(path.join(dir, "bin"), { recursive: true });
  fs.writeFileSync(path.join(dir, "bin", "kandev"), "fake");
  fs.writeFileSync(path.join(dir, "bin", "agentctl"), "fake");
}

describe("native npm shim", () => {
  let tmpDir;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-native-shim-"));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it("maps supported platforms to runtime packages", () => {
    expect(platformPackage("linux", "x64")).toBe("@kdlbs/runtime-linux-x64");
    expect(platformPackage("darwin", "arm64")).toBe("@kdlbs/runtime-darwin-arm64");
    expect(platformPackage("win32", "x64")).toBe("@kdlbs/runtime-win32-x64");
  });

  it("uses exe suffix on Windows only", () => {
    expect(binaryName("kandev", "win32")).toBe("kandev.exe");
    expect(binaryName("kandev", "linux")).toBe("kandev");
  });

  it("resolves KANDEV_BUNDLE_DIR before npm packages", () => {
    createBundle(tmpDir);

    const runtime = resolveRuntime({ KANDEV_BUNDLE_DIR: tmpDir }, () => {
      throw new Error("should not resolve npm package");
    });

    expect(runtime.bundleDir).toBe(tmpDir);
    expect(runtime.executable).toBe(path.join(tmpDir, "bin", "kandev"));
  });

  it("resolves the installed runtime package", () => {
    createBundle(tmpDir);
    const pkgJSON = path.join(tmpDir, "package.json");
    fs.writeFileSync(pkgJSON, "{}");

    const runtime = resolveRuntime({}, () => pkgJSON);

    expect(runtime.bundleDir).toBe(tmpDir);
  });

  it("rejects bundles without the native kandev binary", () => {
    fs.mkdirSync(path.join(tmpDir, "bin"), { recursive: true });
    fs.writeFileSync(path.join(tmpDir, "bin", "agentctl"), "fake");

    expect(() => validateRuntime(tmpDir)).toThrow(/Kandev native binary not found/);
  });
});

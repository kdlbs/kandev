import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, it } from "node:test";

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
    assert.equal(platformPackage("linux", "x64"), "@kdlbs/runtime-linux-x64");
    assert.equal(platformPackage("darwin", "arm64"), "@kdlbs/runtime-darwin-arm64");
    assert.equal(platformPackage("win32", "x64"), "@kdlbs/runtime-win32-x64");
  });

  it("uses exe suffix on Windows only", () => {
    assert.equal(binaryName("kandev", "win32"), "kandev.exe");
    assert.equal(binaryName("kandev", "linux"), "kandev");
  });

  it("resolves KANDEV_BUNDLE_DIR before npm packages", () => {
    createBundle(tmpDir);

    const runtime = resolveRuntime({ KANDEV_BUNDLE_DIR: tmpDir }, () => {
      throw new Error("should not resolve npm package");
    });

    assert.equal(runtime.bundleDir, tmpDir);
    assert.equal(runtime.executable, path.join(tmpDir, "bin", "kandev"));
  });

  it("resolves the installed runtime package", () => {
    createBundle(tmpDir);
    const pkgJSON = path.join(tmpDir, "package.json");
    fs.writeFileSync(pkgJSON, "{}");

    const runtime = resolveRuntime({}, () => pkgJSON);

    assert.equal(runtime.bundleDir, tmpDir);
  });

  it("rejects bundles without the native kandev binary", () => {
    fs.mkdirSync(path.join(tmpDir, "bin"), { recursive: true });
    fs.writeFileSync(path.join(tmpDir, "bin", "agentctl"), "fake");

    assert.throws(() => validateRuntime(tmpDir), /Kandev native binary not found/);
  });
});

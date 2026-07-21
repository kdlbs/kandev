import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { findBundleRoot } from "./bundle";

describe("findBundleRoot", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-bundle-test-"));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it("returns kandev/ subdir when it exists", () => {
    const kandevDir = path.join(tmpDir, "kandev");
    fs.mkdirSync(kandevDir);
    expect(findBundleRoot(tmpDir)).toBe(kandevDir);
  });

  it("returns cacheDir itself when kandev/ does not exist", () => {
    expect(findBundleRoot(tmpDir)).toBe(tmpDir);
  });
});

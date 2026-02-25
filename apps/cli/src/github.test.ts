import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { readSha256 } from "./github";

describe("readSha256", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-sha-test-"));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it("returns null when file does not exist", () => {
    expect(readSha256(path.join(tmpDir, "nonexistent.sha256"))).toBeNull();
  });

  it("reads a plain hash", () => {
    const shaPath = path.join(tmpDir, "test.sha256");
    fs.writeFileSync(shaPath, "abc123def456");
    expect(readSha256(shaPath)).toBe("abc123def456");
  });

  it("reads hash from sha256sum format (hash + filename)", () => {
    const shaPath = path.join(tmpDir, "test.sha256");
    fs.writeFileSync(shaPath, "abc123def456  kandev-macos-arm64.tar.gz\n");
    expect(readSha256(shaPath)).toBe("abc123def456");
  });

  it("lowercases uppercase hash", () => {
    const shaPath = path.join(tmpDir, "test.sha256");
    fs.writeFileSync(shaPath, "ABC123DEF456");
    expect(readSha256(shaPath)).toBe("abc123def456");
  });

  it("trims whitespace", () => {
    const shaPath = path.join(tmpDir, "test.sha256");
    fs.writeFileSync(shaPath, "  abc123def456  \n");
    expect(readSha256(shaPath)).toBe("abc123def456");
  });

  it("returns null for empty file", () => {
    const shaPath = path.join(tmpDir, "test.sha256");
    fs.writeFileSync(shaPath, "");
    expect(readSha256(shaPath)).toBeNull();
  });
});

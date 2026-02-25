import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { cleanOldReleases, findCachedRelease } from "./run";

// Mock CACHE_DIR to use a temp directory.
const mockCacheDir = { value: "" };
vi.mock("./constants", () => ({
  get CACHE_DIR() {
    return mockCacheDir.value;
  },
  DATA_DIR: "/tmp/kandev-test-data",
  DEFAULT_BACKEND_PORT: 8080,
  DEFAULT_WEB_PORT: 3000,
  DEFAULT_AGENTCTL_PORT: 9999,
  HEALTH_TIMEOUT_MS_RELEASE: 15000,
  RANDOM_PORT_MIN: 10000,
  RANDOM_PORT_MAX: 60000,
  RANDOM_PORT_RETRIES: 10,
}));

const PLATFORM = "macos-arm64";

function createFakeCachedRelease(cacheDir: string, tag: string, platform: string) {
  const binDir = path.join(cacheDir, tag, platform, "kandev", "bin");
  fs.mkdirSync(binDir, { recursive: true });
  fs.writeFileSync(path.join(binDir, "kandev"), "fake-binary");
}

describe("findCachedRelease", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-test-"));
    mockCacheDir.value = tmpDir;
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  describe("with specific version", () => {
    it("returns cache info when version exists", () => {
      createFakeCachedRelease(tmpDir, "v0.2", PLATFORM);
      const result = findCachedRelease(PLATFORM, "v0.2");
      expect(result).not.toBeNull();
      expect(result!.tag).toBe("v0.2");
      expect(result!.cacheDir).toBe(path.join(tmpDir, "v0.2", PLATFORM));
    });

    it("returns null when version does not exist", () => {
      const result = findCachedRelease(PLATFORM, "v9.9");
      expect(result).toBeNull();
    });

    it("returns null when tag dir exists but no binary", () => {
      fs.mkdirSync(path.join(tmpDir, "v0.1", PLATFORM), { recursive: true });
      const result = findCachedRelease(PLATFORM, "v0.1");
      expect(result).toBeNull();
    });
  });

  describe("without version (latest)", () => {
    it("picks the highest semver tag", () => {
      createFakeCachedRelease(tmpDir, "v0.1", PLATFORM);
      createFakeCachedRelease(tmpDir, "v0.9", PLATFORM);
      createFakeCachedRelease(tmpDir, "v1.2", PLATFORM);
      const result = findCachedRelease(PLATFORM);
      expect(result).not.toBeNull();
      expect(result!.tag).toBe("v1.2");
    });

    it("sorts major version correctly: v2.0 > v1.9", () => {
      createFakeCachedRelease(tmpDir, "v1.9", PLATFORM);
      createFakeCachedRelease(tmpDir, "v2.0", PLATFORM);
      const result = findCachedRelease(PLATFORM);
      expect(result!.tag).toBe("v2.0");
    });

    it("sorts numerically not lexicographically: v1.10 > v1.9", () => {
      createFakeCachedRelease(tmpDir, "v1.9", PLATFORM);
      createFakeCachedRelease(tmpDir, "v1.10", PLATFORM);
      const result = findCachedRelease(PLATFORM);
      expect(result!.tag).toBe("v1.10");
    });

    it("skips tags without extracted binary", () => {
      createFakeCachedRelease(tmpDir, "v0.1", PLATFORM);
      // v0.2 exists as dir but no binary inside
      fs.mkdirSync(path.join(tmpDir, "v0.2", PLATFORM), { recursive: true });
      const result = findCachedRelease(PLATFORM);
      expect(result!.tag).toBe("v0.1");
    });

    it("returns null when cache dir does not exist", () => {
      mockCacheDir.value = path.join(tmpDir, "nonexistent");
      const result = findCachedRelease(PLATFORM);
      expect(result).toBeNull();
    });

    it("returns null when cache dir is empty", () => {
      const result = findCachedRelease(PLATFORM);
      expect(result).toBeNull();
    });

    it("ignores non-v prefixed directories", () => {
      fs.mkdirSync(path.join(tmpDir, "latest", PLATFORM), { recursive: true });
      fs.mkdirSync(path.join(tmpDir, "temp", PLATFORM), { recursive: true });
      const result = findCachedRelease(PLATFORM);
      expect(result).toBeNull();
    });

    it("picks valid release when highest tag has no binary", () => {
      createFakeCachedRelease(tmpDir, "v0.1", PLATFORM);
      createFakeCachedRelease(tmpDir, "v0.2", PLATFORM);
      // v1.0 dir exists but is empty
      fs.mkdirSync(path.join(tmpDir, "v1.0", PLATFORM), { recursive: true });
      const result = findCachedRelease(PLATFORM);
      expect(result!.tag).toBe("v0.2");
    });
  });
});

describe("cleanOldReleases", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-clean-test-"));
    mockCacheDir.value = tmpDir;
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it("keeps the 2 most recent tags and removes the rest", () => {
    for (const tag of ["v0.1", "v0.2", "v0.3", "v0.4"]) {
      fs.mkdirSync(path.join(tmpDir, tag), { recursive: true });
    }
    cleanOldReleases("v0.4");
    const remaining = fs.readdirSync(tmpDir).sort();
    expect(remaining).toEqual(["v0.3", "v0.4"]);
  });

  it("does nothing when 2 or fewer tags exist", () => {
    fs.mkdirSync(path.join(tmpDir, "v0.1"), { recursive: true });
    fs.mkdirSync(path.join(tmpDir, "v0.2"), { recursive: true });
    cleanOldReleases("v0.2");
    const remaining = fs.readdirSync(tmpDir).sort();
    expect(remaining).toEqual(["v0.1", "v0.2"]);
  });

  it("always keeps currentTag even if not in top 2 by semver", () => {
    for (const tag of ["v0.1", "v1.0", "v2.0"]) {
      fs.mkdirSync(path.join(tmpDir, tag), { recursive: true });
    }
    // currentTag is v0.1 (old), but top 2 by semver are v2.0, v1.0
    cleanOldReleases("v0.1");
    const remaining = fs.readdirSync(tmpDir).sort();
    expect(remaining).toEqual(["v0.1", "v1.0", "v2.0"]);
  });

  it("does nothing when cache dir does not exist", () => {
    mockCacheDir.value = path.join(tmpDir, "nonexistent");
    expect(() => cleanOldReleases("v1.0")).not.toThrow();
  });

  it("ignores non-v prefixed directories", () => {
    fs.mkdirSync(path.join(tmpDir, "latest"), { recursive: true });
    fs.mkdirSync(path.join(tmpDir, "temp"), { recursive: true });
    for (const tag of ["v0.1", "v0.2", "v0.3"]) {
      fs.mkdirSync(path.join(tmpDir, tag), { recursive: true });
    }
    cleanOldReleases("v0.3");
    const remaining = fs.readdirSync(tmpDir).sort();
    expect(remaining).toEqual(["latest", "temp", "v0.2", "v0.3"]);
  });

  it("sorts numerically: keeps v1.10 over v1.9 when both are old", () => {
    for (const tag of ["v1.1", "v1.9", "v1.10", "v2.0"]) {
      fs.mkdirSync(path.join(tmpDir, tag), { recursive: true });
    }
    cleanOldReleases("v2.0");
    const remaining = fs.readdirSync(tmpDir).sort();
    expect(remaining).toEqual(["v1.10", "v2.0"]);
  });
});

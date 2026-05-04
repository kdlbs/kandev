import fs from "node:fs";
import Module from "node:module";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resolveRuntime, validateBundle } from "./runtime";

// Mock constants so CACHE_DIR points to a temp directory.
const mockCacheDir = { value: "" };
vi.mock("./constants", () => ({
  get CACHE_DIR() {
    return mockCacheDir.value;
  },
  DATA_DIR: "/tmp/kandev-test-data",
  DEFAULT_BACKEND_PORT: 38429,
  DEFAULT_WEB_PORT: 37429,
  DEFAULT_AGENTCTL_PORT: 39429,
  HEALTH_TIMEOUT_MS_RELEASE: 15000,
  RANDOM_PORT_MIN: 10000,
  RANDOM_PORT_MAX: 60000,
  RANDOM_PORT_RETRIES: 10,
}));

// Keep getPlatformDir deterministic across test environments.
vi.mock("./platform", () => ({
  getPlatformDir: () => "macos-arm64" as const,
  getBinaryName: (base: string) => base,
}));

const PLATFORM = "macos-arm64";

function createFakeBundle(dir: string) {
  fs.mkdirSync(path.join(dir, "bin"), { recursive: true });
  fs.writeFileSync(path.join(dir, "bin", "kandev"), "fake");
  fs.writeFileSync(path.join(dir, "bin", "agentctl"), "fake");
  fs.mkdirSync(path.join(dir, "web"), { recursive: true });
  fs.writeFileSync(path.join(dir, "web", "server.js"), "fake");
}

describe("validateBundle", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-validate-"));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it("passes for a complete bundle", () => {
    createFakeBundle(tmpDir);
    expect(() => validateBundle(tmpDir)).not.toThrow();
  });

  it("throws when backend binary is missing", () => {
    fs.mkdirSync(path.join(tmpDir, "bin"), { recursive: true });
    fs.writeFileSync(path.join(tmpDir, "bin", "agentctl"), "fake");
    fs.mkdirSync(path.join(tmpDir, "web"), { recursive: true });
    fs.writeFileSync(path.join(tmpDir, "web", "server.js"), "fake");
    expect(() => validateBundle(tmpDir)).toThrow(/Backend binary not found/);
  });

  it("throws when agentctl is missing", () => {
    fs.mkdirSync(path.join(tmpDir, "bin"), { recursive: true });
    fs.writeFileSync(path.join(tmpDir, "bin", "kandev"), "fake");
    fs.mkdirSync(path.join(tmpDir, "web"), { recursive: true });
    fs.writeFileSync(path.join(tmpDir, "web", "server.js"), "fake");
    expect(() => validateBundle(tmpDir)).toThrow(/agentctl binary not found/);
  });

  it("throws when web server.js is missing", () => {
    fs.mkdirSync(path.join(tmpDir, "bin"), { recursive: true });
    fs.writeFileSync(path.join(tmpDir, "bin", "kandev"), "fake");
    fs.writeFileSync(path.join(tmpDir, "bin", "agentctl"), "fake");
    expect(() => validateBundle(tmpDir)).toThrow(/Web server.*not found/);
  });
});

describe("resolveRuntime", () => {
  let tmpDir: string;
  const origEnv = { ...process.env };

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-runtime-"));
    mockCacheDir.value = tmpDir;
    // Clear relevant env vars before each test
    delete process.env.KANDEV_BUNDLE_DIR;
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
    // Restore the original env exactly. Don't `delete process.env.KANDEV_BUNDLE_DIR`
    // afterwards — if a developer had that env var set when the suite started,
    // it's already in origEnv and should remain.
    process.env = { ...origEnv };
  });

  describe("KANDEV_BUNDLE_DIR (step 1)", () => {
    it("resolves from KANDEV_BUNDLE_DIR when set and valid", () => {
      const bundleDir = path.join(tmpDir, "bundle");
      createFakeBundle(bundleDir);
      process.env.KANDEV_BUNDLE_DIR = bundleDir;

      const result = resolveRuntime();
      expect(result.source).toBe("env");
      expect(result.bundleDir).toBe(bundleDir);
    });

    it("throws when KANDEV_BUNDLE_DIR points to an invalid bundle", () => {
      const bundleDir = path.join(tmpDir, "empty-bundle");
      fs.mkdirSync(bundleDir);
      process.env.KANDEV_BUNDLE_DIR = bundleDir;

      expect(() => resolveRuntime()).toThrow(/Backend binary not found/);
    });
  });

  describe("installed npm runtime package (step 2)", () => {
    // Step 2 normally relies on require.resolve('@kdlbs/runtime-darwin-arm64')
    // succeeding because the npm package is installed alongside the kandev
    // package. In this test environment that package isn't present, so we
    // intercept Node's module resolver to point at a fake bundle inside tmpDir.
    function mockResolve(target: string): void {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const ModuleAny = Module as unknown as { _resolveFilename: any };
      vi.spyOn(ModuleAny, "_resolveFilename").mockImplementationOnce(() => target);
    }

    it("resolves from the installed npm runtime package", () => {
      const pkgRoot = path.join(tmpDir, "npm-runtime");
      createFakeBundle(pkgRoot);
      // require.resolve('<pkg>/package.json') normally returns the path to the
      // package.json inside node_modules. Point it at our fake bundle instead.
      const pkgJsonPath = path.join(pkgRoot, "package.json");
      fs.writeFileSync(pkgJsonPath, "{}");
      mockResolve(pkgJsonPath);

      const result = resolveRuntime();
      expect(result.source).toBe("npm");
      expect(result.bundleDir).toBe(pkgRoot);
    });

    it("surfaces validateBundle errors when the npm package is installed but corrupt", () => {
      const pkgRoot = path.join(tmpDir, "broken-npm-runtime");
      fs.mkdirSync(pkgRoot);
      // package.json present (so require.resolve succeeds), but no bin/ or web/
      const pkgJsonPath = path.join(pkgRoot, "package.json");
      fs.writeFileSync(pkgJsonPath, "{}");
      mockResolve(pkgJsonPath);

      expect(() => resolveRuntime()).toThrow(/Backend binary not found/);
    });
  });

  describe("no runtime found", () => {
    it("throws an actionable error message mentioning install paths", () => {
      let error: Error | null = null;
      try {
        resolveRuntime();
      } catch (e) {
        error = e as Error;
      }
      expect(error).not.toBeNull();
      expect(error!.message).toMatch(/npx kandev@latest/);
      expect(error!.message).toMatch(/brew install/);
      expect(error!.message).toMatch(/--runtime-version/);
    });
  });
});

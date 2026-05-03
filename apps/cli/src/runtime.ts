import path from "node:path";
import fs from "node:fs";

import { findBundleRoot, resolveWebServerPath } from "./bundle";
import { CACHE_DIR } from "./constants";
import { getBinaryName, getPlatformDir, type PlatformDir } from "./platform";

// Maps platform.ts dir strings to npm @kdlbs/runtime-* package names.
// platform.ts uses internal names (macos, windows); npm uses os identifiers (darwin, win32).
const PLATFORM_TO_NPM_PACKAGE: Record<PlatformDir, string> = {
  "linux-x64": "@kdlbs/runtime-linux-x64",
  "linux-arm64": "@kdlbs/runtime-linux-arm64",
  "macos-x64": "@kdlbs/runtime-darwin-x64",
  "macos-arm64": "@kdlbs/runtime-darwin-arm64",
  "windows-x64": "@kdlbs/runtime-win32-x64",
};

export type RuntimeSource = "env" | "npm" | "cache";

export type ResolvedRuntime = {
  bundleDir: string;
  source: RuntimeSource;
  tag?: string;
};

/**
 * Resolve the runtime bundle directory using a three-step priority chain:
 *
 * 1. KANDEV_BUNDLE_DIR env var — set by the Homebrew formula wrapper and useful
 *    for local testing. Skips all other resolution.
 * 2. Installed npm runtime package — looks for @kdlbs/runtime-{platform} in
 *    node_modules via Node module resolution. Only works after `npx kandev@latest`
 *    or `npm install -g kandev` (npm 7+).
 * 3. Cached download — only checked when runtimeVersion is explicitly specified
 *    via --runtime-version. Used by the explicit version download path in run.ts.
 *
 * Throws with an actionable error message if no runtime is found.
 */
export function resolveRuntime(runtimeVersion?: string): ResolvedRuntime {
  const envBundleDir = process.env.KANDEV_BUNDLE_DIR;
  if (envBundleDir) {
    validateBundle(envBundleDir);
    return { bundleDir: envBundleDir, source: "env" };
  }

  const platformDir = getPlatformDir();
  const packageName = PLATFORM_TO_NPM_PACKAGE[platformDir];
  let pkgJsonPath: string | null = null;
  try {
    pkgJsonPath = require.resolve(`${packageName}/package.json`);
  } catch {
    // MODULE_NOT_FOUND — npm runtime package is not installed. Fall through
    // to step 3 (cache fallback only if --runtime-version is set).
  }
  if (pkgJsonPath) {
    // The package IS installed. If validateBundle throws here, the bundle is
    // present but corrupt — surface the error rather than the generic
    // "no runtime found" message below.
    const packageRoot = path.dirname(pkgJsonPath);
    validateBundle(packageRoot);
    return { bundleDir: packageRoot, source: "npm" };
  }

  if (runtimeVersion) {
    const cacheDir = path.join(CACHE_DIR, runtimeVersion, platformDir);
    const bundleDir = findBundleRoot(cacheDir);
    validateBundle(bundleDir);
    return { bundleDir, source: "cache", tag: runtimeVersion };
  }

  throw new Error(
    `No Kandev runtime found for ${platformDir}.\n` +
      `  Install via npm (requires npm 7+): npx kandev@latest\n` +
      `  Install via Homebrew: brew install kdlbs/kandev/kandev\n` +
      `  Download a specific version (debug): kandev --runtime-version <tag>`,
  );
}

export function validateBundle(bundleDir: string): void {
  const backendBin = path.join(bundleDir, "bin", getBinaryName("kandev"));
  if (!fs.existsSync(backendBin)) {
    throw new Error(`Backend binary not found in bundle at ${bundleDir}`);
  }
  const agentctlBin = path.join(bundleDir, "bin", getBinaryName("agentctl"));
  if (!fs.existsSync(agentctlBin)) {
    throw new Error(`agentctl binary not found in bundle at ${bundleDir}`);
  }
  const webServerPath = resolveWebServerPath(bundleDir);
  if (!webServerPath) {
    throw new Error(`Web server (server.js) not found in bundle at ${bundleDir}`);
  }
}

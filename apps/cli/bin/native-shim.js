"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

const PLATFORM_PACKAGES = {
  "linux-x64": "@kdlbs/runtime-linux-x64",
  "linux-arm64": "@kdlbs/runtime-linux-arm64",
  "darwin-x64": "@kdlbs/runtime-darwin-x64",
  "darwin-arm64": "@kdlbs/runtime-darwin-arm64",
  "win32-x64": "@kdlbs/runtime-win32-x64",
};

function platformPackage(platform = process.platform, arch = process.arch) {
  const packageName = PLATFORM_PACKAGES[`${platform}-${arch}`];
  if (!packageName) {
    throw new Error(`Unsupported Kandev platform: ${platform}-${arch}`);
  }
  return packageName;
}

function binaryName(name, platform = process.platform) {
  return platform === "win32" ? `${name}.exe` : name;
}

function resolveRuntime(env = process.env, resolver = require.resolve) {
  if (env.KANDEV_BUNDLE_DIR) {
    return validateRuntime(env.KANDEV_BUNDLE_DIR);
  }

  const packageName = platformPackage();
  let packageJSON;
  try {
    packageJSON = resolver(`${packageName}/package.json`);
  } catch (error) {
    const message =
      `No Kandev runtime package found for ${process.platform}-${process.arch}.\n` +
      `Install with npm 7+ so optional dependencies are installed, or use Homebrew.`;
    const runtimeError = new Error(message);
    runtimeError.cause = error;
    throw runtimeError;
  }
  return validateRuntime(path.dirname(packageJSON));
}

function validateRuntime(bundleDir) {
  const executable = path.join(bundleDir, "bin", binaryName("kandev"));
  if (!fs.existsSync(executable)) {
    throw new Error(`Kandev native binary not found at ${executable}`);
  }

  const agentctl = path.join(bundleDir, "bin", binaryName("agentctl"));
  if (!fs.existsSync(agentctl)) {
    throw new Error(`agentctl binary not found at ${agentctl}`);
  }

  return { bundleDir, executable };
}

function run(argv, env = process.env) {
  let runtime;
  try {
    runtime = resolveRuntime(env);
  } catch (error) {
    console.error(`[kandev] ${error.message}`);
    return 1;
  }

  const child = spawnSync(runtime.executable, argv, {
    stdio: "inherit",
    env: {
      ...env,
      KANDEV_BUNDLE_DIR: runtime.bundleDir,
    },
  });

  if (child.error) {
    console.error(`[kandev] failed to launch native binary: ${child.error.message}`);
    return 1;
  }
  if (child.signal) {
    return 1;
  }
  return child.status === null ? 1 : child.status;
}

module.exports = {
  binaryName,
  platformPackage,
  resolveRuntime,
  run,
  validateRuntime,
};

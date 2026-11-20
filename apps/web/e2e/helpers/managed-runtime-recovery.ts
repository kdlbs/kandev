import { expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { DatabaseSync } from "./node-sqlite";
import { createHash, randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import type { AgentProfile } from "../../lib/types/http-agents";
import {
  startManagedRuntimeTestRegistry,
  type ManagedRuntimeTestRegistry,
} from "./managed-runtime-test-registry";

export const MANAGED_RUNTIME_CACHE_ROOT = "/tmp/kandev-managed-npm-cache";
const MANAGED_RUNTIME_AGENT_NAME = "opencode-acp";
const MANAGED_RUNTIME_TEST_MODEL = "opencode/big-pickle";
const OPENCODE_SELECTION_KEY = "managed_runtime.opencode.selection";
const originalRuntimeSelections = new WeakMap<BackendContext, string>();

function selectManagedRuntime(backend: BackendContext, restore = false): void {
  const database = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    database.exec("PRAGMA busy_timeout = 10000");
    if (restore) {
      const original = originalRuntimeSelections.get(backend);
      if (original === undefined) return;
      database
        .prepare("UPDATE settings SET value = ? WHERE key = ?")
        .run(original, OPENCODE_SELECTION_KEY);
      originalRuntimeSelections.delete(backend);
      return;
    }
    const row = database
      .prepare("SELECT value FROM settings WHERE key = ?")
      .get(OPENCODE_SELECTION_KEY) as { value: string } | undefined;
    if (!row) throw new Error("OpenCode runtime selection must exist before managed-runtime setup");
    const selection = JSON.parse(row.value);
    if (selection.source !== "native") return;
    originalRuntimeSelections.set(backend, row.value);
    selection.source = "managed";
    selection.selected_version = "";
    selection.revision += 1;
    database
      .prepare("UPDATE settings SET value = ? WHERE key = ?")
      .run(JSON.stringify(selection), OPENCODE_SELECTION_KEY);
  } finally {
    database.close();
  }
}

function selectManagedRuntimeFamily(backend: BackendContext, family?: "v1" | "v2"): void {
  if (!family) {
    selectManagedRuntime(backend);
    return;
  }
  const database = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    database.exec("PRAGMA busy_timeout = 10000");
    const row = database
      .prepare("SELECT value FROM settings WHERE key = ?")
      .get(OPENCODE_SELECTION_KEY) as { value: string };
    if (!originalRuntimeSelections.has(backend)) originalRuntimeSelections.set(backend, row.value);
    const selection = JSON.parse(row.value);
    const versions = JSON.parse(
      fs.readFileSync(
        path.resolve(
          __dirname,
          "../../../backend/internal/agent/agents/managed_npm_runtime_versions.json",
        ),
        "utf8",
      ),
    ) as Record<string, string>;
    selection.family = family;
    selection.package = family === "v2" ? "@opencode/cli" : "opencode-ai";
    selection.source = "managed";
    selection.selected_version = versions[selection.package];
    selection.applied_default_version = selection.selected_version;
    selection.revision += 1;
    database
      .prepare("UPDATE settings SET value = ? WHERE key = ?")
      .run(JSON.stringify(selection), OPENCODE_SELECTION_KEY);
  } finally {
    database.close();
  }
}

export function managedRuntimeExecutionCacheKey(packageSpec: string): string {
  return createSha512(packageSpec).slice(0, 16);
}

export function managedRuntimeStartupAttemptFile(launchId: string): string {
  const safeLaunchId = launchId.replace(/[^A-Za-z0-9_-]/g, "_");
  return path.join(MANAGED_RUNTIME_CACHE_ROOT, "kandev-e2e-attempts", safeLaunchId);
}

export function managedRuntimeStartupSentinels(
  packageSpec: string,
  launchId: string,
  cacheRoot = MANAGED_RUNTIME_CACHE_ROOT,
) {
  const safeLaunchId = launchId.replace(/[^A-Za-z0-9_-]/g, "_");
  const npxRoot = path.join(cacheRoot, "_npx");
  return {
    selected: path.join(
      npxRoot,
      managedRuntimeExecutionCacheKey(packageSpec),
      `retry-sentinel-${safeLaunchId}`,
    ),
    sibling: path.join(npxRoot, "0123456789abcdef", `sibling-sentinel-${safeLaunchId}`),
  };
}

export type ManagedRuntimePreparation = {
  profile: AgentProfile;
  packageSpec: string;
  hostFixturePath?: string;
};

export type ManagedRuntimeStartupFixtureMode =
  | "success"
  | "transient"
  | "silent-exit"
  | "permanent"
  | "repeat-failure"
  | "missing-root-manifest";

export type ManagedRuntimePreparationOptions = {
  startupMode?: ManagedRuntimeStartupFixtureMode;
  launchId?: string;
  hostSubprocess?: boolean;
  retryReleaseFile?: string;
  cacheRoot?: string;
  registryHost?: string;
  runtimeFamily?: "v1" | "v2";
  cliFlags?: string[];
};

export type IncompleteManagedRuntimeFixture = {
  profile: AgentProfile;
  packageSpec: string;
  cacheRoot: string;
  registry: ManagedRuntimeTestRegistry;
  sharedRoot: string;
  sentinels: ReturnType<typeof managedRuntimeStartupSentinels>;
  npmInvocationLog?: string;
  npmArgvCapture?: string;
  npmPidCapture?: string;
  hostFixturePaths: string[];
  releaseBackendEnv: () => Promise<void>;
};

function readOpenCodePackageSpec(backend: BackendContext): string {
  const database = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    const row = database
      .prepare("SELECT value FROM settings WHERE key = ?")
      .get(OPENCODE_SELECTION_KEY) as { value: string } | undefined;
    if (!row) throw new Error("OpenCode runtime selection must exist before managed-runtime setup");
    const selection = JSON.parse(row.value) as {
      family?: string;
      selected_version?: string;
      applied_default_version?: string;
    };
    const packageName = selection.family === "v2" ? "@opencode/cli" : "opencode-ai";
    const versionsPath = path.resolve(
      __dirname,
      "../../../backend/internal/agent/agents/managed_npm_runtime_versions.json",
    );
    const versions = JSON.parse(fs.readFileSync(versionsPath, "utf8")) as Record<string, string>;
    const version =
      selection.selected_version || selection.applied_default_version || versions[packageName];
    if (!version) throw new Error(`managed OpenCode package ${packageName} has no exact version`);
    return `${packageName}@${version}`;
  } finally {
    database.close();
  }
}

/**
 * Starts one isolated managed runtime with a private local registry and a
 * per-test execution cache. The wrapper only models npm's first ENOENT; the
 * agentctl owner performs the private install through the real npm executable.
 */
function appendHostSubprocessProfileEnv(input: {
  profileEnv: Array<{ key: string; value: string }>;
  hostSubprocess: boolean | undefined;
  backendEnv: { E2E_REAL_NPX: string };
  mockAgentPath: string;
  npmInvocationLog: string | undefined;
  npmArgvCapture: string | undefined;
}): void {
  if (!input.hostSubprocess) return;
  input.profileEnv.push({ key: "E2E_MOCK_AGENT_PATH", value: input.mockAgentPath });
  input.profileEnv.push({ key: "E2E_REAL_NPX", value: input.backendEnv.E2E_REAL_NPX });
  input.profileEnv.push({ key: "E2E_NPM_EXEC_LOG", value: input.npmInvocationLog ?? "" });
  if (input.npmArgvCapture) {
    input.profileEnv.push({ key: "E2E_NPX_ARGV_CAPTURE", value: input.npmArgvCapture });
  }
}

function containerCapturePaths(cacheRoot: string, launchId: string) {
  return {
    argv: path.join(cacheRoot, `managed-runtime-argv-${launchId}.log`),
    npm: path.join(cacheRoot, `managed-runtime-npm-${launchId}.log`),
    pids: path.join(cacheRoot, `managed-runtime-pids-${launchId}.log`),
  };
}

function appendContainerCaptureEnv(
  profileEnv: Array<{ key: string; value: string }>,
  capture: { argv: string; npm: string; pids: string },
): void {
  profileEnv.push({ key: "E2E_NPX_ARGV_CAPTURE", value: capture.argv });
  profileEnv.push({ key: "E2E_NPM_EXEC_LOG", value: capture.npm });
  profileEnv.push({ key: "E2E_NPX_PID_CAPTURE", value: capture.pids });
}

export function parseProviderPIDLog(text: string): string[] {
  return text.trim() ? text.trim().split(/\r?\n/).filter(Boolean) : [];
}

export function parseProviderArgvLog(text: string): string[][] {
  const trimmed = text.trim();
  if (!trimmed) return [];
  return trimmed.split(/\r?\n/).map((line) => line.split("\t").filter(Boolean));
}

export function countInstallerCalls(text: string, packageSpec: string): number {
  return text.split(/\r?\n/).filter((line) => line.includes(`--package=${packageSpec}`)).length;
}

export function findOwnedProviderPID(containerId: string, matchArg: string): string {
  const script = `last=""; for p in /proc/[0-9]*; do pid="\${p#/proc/}"; [ "$pid" = "1" ] && continue; grep -qa "mock-agent" "$p/cmdline" 2>/dev/null || continue; grep -qa -- "${matchArg}" "$p/cmdline" 2>/dev/null || continue; last="$pid"; done; printf '%s\n' "$last"`;
  return execFileSync("docker", ["exec", containerId, "sh", "-c", script], {
    encoding: "utf8",
  }).trim();
}

export function killOwnedProviderPID(containerId: string, pid: string): void {
  const script = `get_ppid() { awk '/^PPid:/{print $2}' "/proc/$1/status" 2>/dev/null; }; kill_tree() { for child in /proc/[0-9]*; do cpid="\${child#/proc/}"; [ "$cpid" = "1" ] && continue; cppid=$(get_ppid "$cpid"); [ "$cppid" = "$1" ] && kill_tree "$cpid"; done; kill -9 "$1" 2>/dev/null || true; }; current="$1"; while [ "$current" -gt 1 ]; do ppid=$(get_ppid "$current"); [ -z "$ppid" ] && break; [ "$ppid" = "1" ] && break; if grep -qa "agentctl" "/proc/$ppid/cmdline" 2>/dev/null; then break; fi; current="$ppid"; done; kill_tree "$current"`;
  execFileSync("docker", ["exec", containerId, "sh", "-c", script, "sh", pid]);
}

function managedRuntimeProfileCLIFlags(flags: string[] | undefined) {
  return (flags ?? []).map((flag) => ({
    description: "E2E managed npm profile flag",
    flag,
    enabled: true,
  }));
}

export async function prepareIncompleteManagedRuntimeProfile(
  apiClient: ApiClient,
  backend: BackendContext,
  options: ManagedRuntimePreparationOptions & { seedHealthyCache?: boolean } = {},
): Promise<IncompleteManagedRuntimeFixture> {
  const launchId = options.launchId ?? randomUUID();
  const cacheRoot =
    options.cacheRoot ?? `/tmp/kandev-managed-npm-cache-${createSha512(launchId).slice(0, 12)}`;
  selectManagedRuntimeFamily(backend, options.runtimeFamily);
  const packageSpec = readOpenCodePackageSpec(backend);
  const registry = await startManagedRuntimeTestRegistry({
    root: backend.tmpDir,
    packageSpec,
    binName: "opencode",
    advertisedHost: options.registryHost,
  });
  const sharedRoot = path.join(cacheRoot, "_npx", managedRuntimeExecutionCacheKey(packageSpec));
  const binDirectory = path.join(backend.tmpDir, "managed-runtime-bin");
  const hostFixturePaths: string[] = [];
  const npxPath = path.join(binDirectory, "npx");
  fs.mkdirSync(binDirectory, { recursive: true });
  fs.copyFileSync(path.resolve(__dirname, "../fixtures/managed-runtime-npx.sh"), npxPath);
  fs.chmodSync(npxPath, 0o755);
  hostFixturePaths.push(npxPath);

  let npmInvocationLog: string | undefined;
  let npmArgvCapture: string | undefined;
  let npmPidCapture: string | undefined;
  let containerCapture: { argv: string; npm: string; pids: string } | undefined;
  if (!options.hostSubprocess) {
    containerCapture = containerCapturePaths(cacheRoot, launchId);
    npmArgvCapture = containerCapture.argv;
    npmInvocationLog = containerCapture.npm;
    npmPidCapture = containerCapture.pids;
  }
  if (options.hostSubprocess) {
    npmArgvCapture = path.join(backend.tmpDir, `managed-runtime-argv-${launchId}.log`);
    hostFixturePaths.push(npmArgvCapture);
    const realNpmPath = execFileSync("which", ["npm"], { encoding: "utf8" }).trim();
    // Keep Node available when the native OpenCode directory is excluded.
    const nodePath = path.join(binDirectory, "node");
    fs.symlinkSync(process.execPath, nodePath);
    hostFixturePaths.push(nodePath);
    const npmPath = path.join(binDirectory, "npm");
    npmInvocationLog = path.join(backend.tmpDir, `managed-runtime-npm-${launchId}.log`);
    fs.writeFileSync(
      npmPath,
      `#!/bin/sh\nnode -e 'require("node:fs").appendFileSync(process.env.E2E_NPM_EXEC_LOG, JSON.stringify(process.argv.slice(1))+"\\n")' -- "$@"\nexec "${realNpmPath}" "$@" 2>> "$E2E_NPM_EXEC_LOG.stderr"\n`,
      { mode: 0o755 },
    );
    hostFixturePaths.push(npmPath);
  }

  seedManagedRuntimeExecutionTree(cacheRoot, packageSpec, "opencode", launchId);
  if (!options.seedHealthyCache) fs.rmSync(path.join(sharedRoot, "package.json"));

  const inheritedPath = (process.env.PATH ?? "").split(path.delimiter).filter(Boolean);
  const pathWithoutNativeOpenCode = inheritedPath.filter(
    (directory) => !fs.existsSync(path.join(directory, "opencode")),
  );
  const mockAgentPath = path.resolve(__dirname, "../../../backend/bin/mock-agent");
  const backendEnv = {
    KANDEV_MOCK_AGENT: "true",
    NPM_CONFIG_CACHE: cacheRoot,
    NPM_CONFIG_REGISTRY: registry.url,
    // npm prefers lowercase config inherited from package-manager scripts.
    npm_config_registry: registry.url,
    NPM_CONFIG_AUDIT: "false",
    NPM_CONFIG_FUND: "false",
    E2E_NPX_MODE: "missing-root-manifest",
    KANDEV_E2E_NPX_MOCK_OTHERS: "true",
    E2E_NPX_LAUNCH_ID: "managed-runtime-host-probe",
    E2E_MOCK_AGENT_PATH: mockAgentPath,
    E2E_REAL_NPX: execFileSync("which", ["npx"], { encoding: "utf8" }).trim(),
    E2E_NPM_EXEC_LOG: npmInvocationLog ?? "",
    PATH: [
      binDirectory,
      path.resolve(__dirname, "../../../backend/bin"),
      ...pathWithoutNativeOpenCode,
    ].join(path.delimiter),
  };
  const releaseBackendEnv = await backend.useEnv(backendEnv);

  let profileId = "";
  try {
    const { agents: persistedAgents } = await apiClient.listAgents();
    const managedAgent = persistedAgents.find((agent) => agent.name === MANAGED_RUNTIME_AGENT_NAME);
    if (!managedAgent)
      throw new Error("OpenCode managed runtime is missing from the agent catalogue");
    const profileEnv = [
      { key: "NPM_CONFIG_CACHE", value: cacheRoot },
      { key: "NPM_CONFIG_REGISTRY", value: registry.url },
      { key: "npm_config_registry", value: registry.url },
      { key: "NPM_CONFIG_AUDIT", value: "false" },
      { key: "NPM_CONFIG_FUND", value: "false" },
      { key: "E2E_NPX_MODE", value: options.startupMode ?? "missing-root-manifest" },
      { key: "E2E_NPX_LAUNCH_ID", value: launchId },
      {
        key: "E2E_PRIVATE_ROOT_MARKER",
        value: path.join(cacheRoot, "kandev-e2e-private-roots"),
      },
    ];
    if (containerCapture) appendContainerCaptureEnv(profileEnv, containerCapture);
    appendHostSubprocessProfileEnv({
      profileEnv,
      hostSubprocess: options.hostSubprocess,
      backendEnv,
      mockAgentPath,
      npmInvocationLog,
      npmArgvCapture,
    });
    const profile = await apiClient.createAgentProfile(
      managedAgent.id,
      "E2E incomplete npm runtime",
      {
        model: MANAGED_RUNTIME_TEST_MODEL,
        env_vars: profileEnv,
        cli_flags: managedRuntimeProfileCLIFlags(options.cliFlags),
      },
    );
    profileId = profile.id;
    return {
      profile,
      packageSpec,
      cacheRoot,
      registry,
      sharedRoot,
      sentinels: managedRuntimeStartupSentinels(packageSpec, launchId, cacheRoot),
      npmInvocationLog,
      npmArgvCapture,
      npmPidCapture,
      hostFixturePaths,
      releaseBackendEnv: async () => {
        selectManagedRuntime(backend, true);
        await releaseBackendEnv();
      },
    };
  } catch (error) {
    if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
    selectManagedRuntime(backend, true);
    await releaseBackendEnv().catch(() => undefined);
    await registry.close().catch(() => undefined);
    for (const fixturePath of hostFixturePaths) fs.rmSync(fixturePath, { force: true });
    throw error;
  }
}

/** Seed a complete exact-package npx execution tree that can later be damaged. */
export function seedManagedRuntimeExecutionTree(
  cacheRoot: string,
  packageSpec: string,
  binName: string,
  launchId: string,
) {
  const separator = packageSpec.lastIndexOf("@");
  const packageName = packageSpec.slice(0, separator);
  const version = packageSpec.slice(separator + 1);
  const executionRoot = path.join(cacheRoot, "_npx", managedRuntimeExecutionCacheKey(packageSpec));
  const packageDirectory = path.join(executionRoot, "node_modules", ...packageName.split("/"));
  const binDirectory = path.join(executionRoot, "node_modules", ".bin");
  fs.mkdirSync(path.join(packageDirectory, "bin"), { recursive: true });
  fs.mkdirSync(binDirectory, { recursive: true });
  fs.writeFileSync(path.join(executionRoot, "package.json"), JSON.stringify({ private: true }));
  fs.writeFileSync(
    path.join(packageDirectory, "package.json"),
    JSON.stringify({ name: packageName, version, bin: { [binName]: `bin/${binName}.js` } }),
  );
  fs.writeFileSync(path.join(packageDirectory, "bin", `${binName}.js`), "#!/bin/sh\nexit 1\n", {
    mode: 0o755,
  });
  const link = path.join(binDirectory, binName);
  if (!fs.existsSync(link)) {
    fs.symlinkSync(
      path.relative(binDirectory, path.join(packageDirectory, "bin", `${binName}.js`)),
      link,
    );
  }
  const safeLaunchId = launchId.replace(/[^A-Za-z0-9_-]/g, "_");
  fs.mkdirSync(path.join(cacheRoot, "_npx", "0123456789abcdef"), { recursive: true });
  const sentinels = managedRuntimeStartupSentinels(packageSpec, launchId, cacheRoot);
  fs.writeFileSync(sentinels.selected, "preserved\n");
  fs.writeFileSync(sentinels.sibling, "preserved\n");
  return { executionRoot, packageDirectory, binDirectory, safeLaunchId };
}

/**
 * The real managed OpenCode agent is enabled only for this container-backed
 * test. Its command runs through the image's npx wrapper, while the wrapper
 * starts the Linux mock ACP binary on the online retry.
 */
export async function prepareManagedRuntimeProfile(
  apiClient: ApiClient,
  backend: BackendContext,
  options: ManagedRuntimePreparationOptions = {},
): Promise<ManagedRuntimePreparation> {
  const mockAgentPath = path.resolve(__dirname, "../../../backend/bin/mock-agent");
  let hostFixturePath: string | undefined;
  if (options.hostSubprocess) {
    const fixtureBin = path.join(backend.tmpDir, "bin");
    hostFixturePath = path.join(fixtureBin, "npx");
    fs.mkdirSync(fixtureBin, { recursive: true });
    fs.copyFileSync(path.resolve(__dirname, "../fixtures/managed-runtime-npx.sh"), hostFixturePath);
    fs.chmodSync(hostFixturePath, 0o755);
  }
  const inheritedPath = (process.env.PATH ?? "").split(path.delimiter).filter(Boolean);
  const pathWithoutNativeOpenCode = inheritedPath.filter(
    (directory) => !fs.existsSync(path.join(directory, "opencode")),
  );
  // Native choice survives PATH changes; this fixture specifically exercises managed npm.
  selectManagedRuntime(backend);
  await backend.restart({
    KANDEV_MOCK_AGENT: "true",
    NPM_CONFIG_CACHE: MANAGED_RUNTIME_CACHE_ROOT,
    PATH: [
      path.join(backend.tmpDir, "bin"),
      path.resolve(__dirname, "../../../backend/bin"),
      ...pathWithoutNativeOpenCode,
    ].join(path.delimiter),
  });

  let agentId = "";
  let packageSpec = "";
  let observedAgents = "";
  try {
    await expect
      .poll(
        async () => {
          const [{ agents: availableAgents }, { agents: persistedAgents }] = await Promise.all([
            apiClient.listAvailableAgents(),
            apiClient.listAgents(),
          ]);
          observedAgents = availableAgents
            .map((agent) => {
              const runtime = agent.runtime_update;
              const version = runtime?.effective_version ? `@${runtime.effective_version}` : "";
              const model = agent.model_config.available_models[0]?.id ?? "none";
              return `${agent.name}:${agent.available ? "available" : "unavailable"}${version}:model=${model}`;
            })
            .join(", ");
          const managedAgent = availableAgents.find(
            (agent) =>
              agent.name === MANAGED_RUNTIME_AGENT_NAME &&
              agent.available &&
              agent.runtime_update?.supported &&
              agent.runtime_update.package &&
              agent.runtime_update.effective_version,
          );
          const persistedAgent = persistedAgents.find(
            (agent) => agent.name === MANAGED_RUNTIME_AGENT_NAME,
          );
          agentId = persistedAgent?.id ?? "";
          packageSpec = managedAgent?.runtime_update
            ? `${managedAgent.runtime_update.package}@${managedAgent.runtime_update.effective_version}`
            : "";
          return agentId && packageSpec ? `${agentId}:${packageSpec}` : "";
        },
        {
          timeout: 30_000,
          message: "OpenCode managed runtime metadata should be available for container recovery",
        },
      )
      .not.toBe("");
  } catch (error) {
    throw new Error(
      `${error instanceof Error ? error.message : String(error)}; agents=${observedAgents}`,
    );
  }

  const envVars = [{ key: "NPM_CONFIG_CACHE", value: MANAGED_RUNTIME_CACHE_ROOT }];
  if (options.startupMode) {
    envVars.push({ key: "E2E_NPX_MODE", value: options.startupMode });
    envVars.push({ key: "E2E_NPX_LAUNCH_ID", value: options.launchId ?? randomUUID() });
  }
  if (options.retryReleaseFile) {
    envVars.push({ key: "E2E_NPX_RETRY_RELEASE_FILE", value: options.retryReleaseFile });
  }
  if (options.hostSubprocess) {
    envVars.push({
      key: "E2E_MOCK_AGENT_PATH",
      value: mockAgentPath,
    });
  }

  return {
    profile: await apiClient.createAgentProfile(agentId, "E2E managed npm recovery", {
      model: MANAGED_RUNTIME_TEST_MODEL,
      env_vars: envVars,
      cli_flags: managedRuntimeProfileCLIFlags(options.cliFlags),
    }),
    packageSpec,
    hostFixturePath,
  };
}

/** Restore the normal e2e-only mock registry after a managed-runtime test. */
export async function restoreE2EAgentRegistry(backend: BackendContext): Promise<void> {
  selectManagedRuntime(backend, true);
  await backend.restart();
}

function createSha512(value: string): string {
  return createHash("sha512").update(value).digest("hex");
}

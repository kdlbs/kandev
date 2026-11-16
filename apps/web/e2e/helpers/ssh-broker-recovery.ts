import { spawnSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../fixtures/backend";
import type { SSHSeedData } from "../fixtures/ssh-test-base";
import type { ApiClient } from "./api-client";
import {
  credentialBrokerPublicURL,
  fixtureGitHost,
  fixtureGitProvider,
  installFixturePlugin,
  startPluginGitFixture,
  uninstallFixturePlugin,
} from "./plugin-git-credentials";
import { DatabaseSync } from "./node-sqlite";
import { execInContainer, type SSHServerHandle } from "./ssh";

const brokerURLKey = "KANDEV_GITHUB_CREDENTIAL_BROKER_URL";
const brokerLeaseKey = "KANDEV_GITHUB_CREDENTIAL_LEASE";
export const brokerAgentctlEnvironmentKeys = [brokerURLKey, brokerLeaseKey].sort();

type BrokerGitFixture = Awaited<ReturnType<typeof startPluginGitFixture>>;

export type BrokerSSHSetup = {
  repositoryId: string;
  cleanup: () => Promise<void>;
};

export type SSHReceiptState = {
  generation: string;
  pid: number;
  birthToken: string;
};

export async function setupBrokerBackedSSH(
  backend: BackendContext,
  apiClient: ApiClient,
  seedData: SSHSeedData,
): Promise<BrokerSSHSetup> {
  const publicBrokerURL = credentialBrokerPublicURL(backend.port);
  if (!publicBrokerURL) throw new Error("credential broker public URL is required");

  const releaseBrokerEnv = await backend.useEnv({
    KANDEV_GITHUB_CREDENTIAL_BROKER_PUBLIC_BASE_URL: publicBrokerURL,
  });
  let fixture: BrokerGitFixture | undefined;
  let pluginInstalled = false;
  let proxyConfigInstalled = false;
  let sslVerifyConfigInstalled = false;
  let originalProxyConfig: string[] | undefined;
  let originalSSLVerifyConfig: string[] | undefined;
  const fixtureRoot = path.join(backend.tmpDir, "ssh-broker-recovery", randomUUID());
  const cleanup = async () => {
    const errors: unknown[] = [];
    const runCleanup = async (action: () => void | Promise<void>) => {
      try {
        await action();
      } catch (error) {
        errors.push(error);
      }
    };
    if (proxyConfigInstalled) {
      await runCleanup(() =>
        restoreSystemGitConfig(seedData.sshTarget, "http.proxy", originalProxyConfig),
      );
    }
    if (sslVerifyConfigInstalled) {
      await runCleanup(() =>
        restoreSystemGitConfig(seedData.sshTarget, "http.sslVerify", originalSSLVerifyConfig),
      );
    }
    if (pluginInstalled) await runCleanup(() => uninstallFixturePlugin(backend.baseUrl));
    await runCleanup(async () => {
      await fixture?.close();
    });
    await runCleanup(() => fs.rmSync(fixtureRoot, { recursive: true, force: true }));
    await runCleanup(releaseBrokerEnv);
    if (errors.length === 1) throw errors[0];
    if (errors.length > 1) {
      throw new AggregateError(errors, "failed to clean up broker-backed SSH fixture");
    }
  };
  try {
    fixture = await startPluginGitFixture(fixtureRoot);
    originalProxyConfig = readSystemGitConfig(seedData.sshTarget, "http.proxy");
    originalSSLVerifyConfig = readSystemGitConfig(seedData.sshTarget, "http.sslVerify");
    execInContainer(seedData.sshTarget, [
      "git",
      "config",
      "--system",
      "--replace-all",
      "http.proxy",
      fixture.proxyURL,
    ]);
    proxyConfigInstalled = true;
    execInContainer(seedData.sshTarget, [
      "git",
      "config",
      "--system",
      "--replace-all",
      "http.sslVerify",
      "false",
    ]);
    sslVerifyConfigInstalled = true;

    await installFixturePlugin(backend.baseUrl);
    pluginInstalled = true;
    const repository = await apiClient.createRepository(
      seedData.workspaceId,
      fixture.checkoutPath,
      "main",
      {
        name: "TEAM/fixture",
        provider: fixtureGitProvider,
        provider_host: `https://${fixtureGitHost}`,
        provider_owner: "TEAM",
        provider_name: "fixture",
      },
    );

    return {
      repositoryId: repository.id,
      cleanup,
    };
  } catch (error) {
    await cleanup().catch(() => undefined);
    throw error;
  }
}

export function remoteAgentctlEnvironmentKeys(
  target: SSHServerHandle,
  agentctlPID: number,
): string[] {
  const output = execInContainer(target, [
    "sh",
    "-c",
    `tr '\\000' '\\n' < /proc/${agentctlPID}/environ | awk -F= '$1 == "${brokerURLKey}" && length(substr($0, length($1) + 2)) > 0 { print $1 } $1 == "${brokerLeaseKey}" && length(substr($0, length($1) + 2)) > 0 { print $1 }'`,
  ]);
  return output
    .split(/\r?\n/)
    .map((key) => key.trim())
    .filter(Boolean)
    .sort();
}

export function sshSessionControllerPID(
  target: SSHServerHandle,
  remoteTaskDir: string,
  sessionId: string,
): number {
  const raw = execInContainer(target, [
    "sh",
    "-c",
    `cat ${shellQuote(`${remoteTaskDir}/.kandev/sessions/${sessionId}/agentctl.pid`)}`,
  ]).trim();
  const pid = Number.parseInt(raw, 10);
  if (!Number.isSafeInteger(pid) || pid <= 0) {
    throw new Error(`SSH session ${sessionId} has an invalid controller PID`);
  }
  return pid;
}

export function remoteProcessChildren(target: SSHServerHandle, pid: number): number[] {
  const raw = execInContainer(target, [
    "sh",
    "-c",
    `cat /proc/${pid}/task/${pid}/children 2>/dev/null || true`,
  ]).trim();
  return raw
    .split(/\s+/)
    .map((value) => Number.parseInt(value, 10))
    .filter((child) => Number.isSafeInteger(child) && child > 0);
}

export function remoteProcessTree(target: SSHServerHandle, rootPID: number): number[] {
  const descendants: number[] = [];
  const pending = [rootPID];
  const visited = new Set<number>();
  while (pending.length > 0) {
    const parent = pending.shift()!;
    if (visited.has(parent)) continue;
    visited.add(parent);
    const children = remoteProcessChildren(target, parent);
    descendants.push(...children);
    pending.push(...children);
  }
  return [...new Set(descendants)];
}

export function isRemoteProcessAlive(target: SSHServerHandle, pid: number): boolean {
  if (!Number.isSafeInteger(pid) || pid <= 0) {
    throw new Error(`invalid remote process PID: ${pid}`);
  }
  const state = execInContainer(target, [
    "sh",
    "-c",
    `if kill -0 "$1" 2>/dev/null; then printf alive; elif [ -d "/proc/$1" ]; then printf alive; else printf gone; fi`,
    "sh",
    String(pid),
  ]).trim();
  if (state === "alive") return true;
  if (state === "gone") return false;
  throw new Error(`unexpected remote process state for PID ${pid}: ${state}`);
}

function readSystemGitConfig(target: SSHServerHandle, key: string): string[] | undefined {
  const result = spawnSync(
    "docker",
    ["exec", target.containerName, "git", "config", "--system", "--get-all", key],
    { encoding: "utf8" },
  );
  if (result.error) throw result.error;
  if (result.status === 1 && !result.stdout && !result.stderr) return undefined;
  if (result.status !== 0) {
    throw new Error(
      `reading system Git config ${key} failed (status=${result.status}): ${result.stderr}`,
    );
  }
  return result.stdout.replace(/\r?\n$/, "").split(/\r?\n/);
}

function restoreSystemGitConfig(
  target: SSHServerHandle,
  key: string,
  values: string[] | undefined,
): void {
  execInContainer(target, ["git", "config", "--system", "--unset-all", key]);
  for (const value of values ?? []) {
    execInContainer(target, ["git", "config", "--system", "--add", key, value]);
  }
}

export function writeRemoteWorkspaceSentinel(
  target: SSHServerHandle,
  workspacePath: string,
  name: string,
  contents: string,
): string {
  const sentinelPath = path.posix.join(workspacePath, name);
  execInContainer(target, ["sh", "-c", `printf '%s' "$1" > "$2"`, "sh", contents, sentinelPath]);
  return sentinelPath;
}

export function readSSHLaunchReceipt(
  backend: Pick<BackendContext, "tmpDir">,
  sessionId: string,
  executionId: string,
): SSHReceiptState {
  const database = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    database.exec("PRAGMA busy_timeout=5000");
    const rows = database
      .prepare(
        "SELECT metadata FROM executors_running WHERE session_id = ? AND agent_execution_id = ?",
      )
      .all(sessionId, executionId) as Array<{ metadata: string }>;
    if (rows.length !== 1) {
      throw new Error(
        `expected one SSH execution row for session ${sessionId} and execution ${executionId}, got ${rows.length}`,
      );
    }
    const metadata = JSON.parse(rows[0].metadata) as Record<string, unknown>;
    return receiptFromMetadata(metadata, sessionId);
  } finally {
    database.close();
  }
}

export function rewriteSSHReceiptForPreviousBoot(
  backend: Pick<BackendContext, "tmpDir">,
  sessionId: string,
  executionId: string,
): string {
  const database = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    database.exec("PRAGMA busy_timeout=5000");
    const rows = database
      .prepare(
        "SELECT metadata FROM executors_running WHERE session_id = ? AND agent_execution_id = ?",
      )
      .all(sessionId, executionId) as Array<{ metadata: string }>;
    if (rows.length !== 1) {
      throw new Error(
        `expected one SSH execution row for session ${sessionId} and execution ${executionId}, got ${rows.length}`,
      );
    }

    const metadata = JSON.parse(rows[0].metadata) as Record<string, unknown>;
    const receipt = metadata.ssh_agentctl_launch_receipt as
      | { process?: { birth_token?: unknown } }
      | undefined;
    const birthToken = receipt?.process?.birth_token;
    if (typeof birthToken !== "string") {
      throw new Error(`SSH session ${sessionId} has no recorded process birth token`);
    }
    const parts = birthToken.split(":");
    if (parts.length !== 3 || parts[0] !== "linux" || !parts[1] || !parts[2]) {
      throw new Error(`SSH session ${sessionId} has an unsupported Linux process birth token`);
    }

    const previousBoot = `synthetic-previous-${parts[1]}`;
    const newBirthToken = `linux:${previousBoot}:${parts[2]}`;
    receipt!.process!.birth_token = newBirthToken;
    const update = database
      .prepare(
        "UPDATE executors_running SET metadata = ? WHERE session_id = ? AND agent_execution_id = ?",
      )
      .run(JSON.stringify(metadata), sessionId, executionId);
    if (Number(update.changes) !== 1) {
      throw new Error(
        `synthetic boot update matched ${Number(update.changes)} rows for session ${sessionId} and execution ${executionId}`,
      );
    }
    return newBirthToken;
  } finally {
    database.close();
  }
}

function receiptFromMetadata(
  metadata: Record<string, unknown>,
  sessionId: string,
): SSHReceiptState {
  const receipt = metadata.ssh_agentctl_launch_receipt as
    | {
        generation?: unknown;
        process?: { pid?: unknown; birth_token?: unknown };
      }
    | undefined;
  if (
    typeof receipt?.generation !== "string" ||
    typeof receipt.process?.pid !== "number" ||
    typeof receipt.process.birth_token !== "string"
  ) {
    throw new Error(`SSH session ${sessionId} has an incomplete controller launch receipt`);
  }
  return {
    generation: receipt.generation,
    pid: receipt.process.pid,
    birthToken: receipt.process.birth_token,
  };
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

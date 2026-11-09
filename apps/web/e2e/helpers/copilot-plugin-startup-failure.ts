import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import {
  COPILOT_RECOVERY_PLUGIN_ID,
  createCopilotPluginFixture,
  type CopilotPluginFixture,
} from "./copilot-plugin-fixture";
import { waitForSessionState } from "./session";

const PRIVATE_PLUGIN_ID = "private-plugin-id";
const PRIVATE_PLUGIN_PATH = "/Users/alice/.config/copilot/plugins/private";
const PRIVATE_NATIVE_DIAGNOSTIC = "private-provider-diagnostic-427";

type PluginFailureCause = {
  code?: string;
  reason?: string;
  runtime_version?: string;
};

type PluginFailureSession = {
  metadata?: {
    last_agent_error?: {
      causes?: PluginFailureCause[];
    };
  };
};

type PluginFailureEvidence = {
  taskId: string;
  sessionId: string;
  runtimeVersion: string;
};

export type CopilotPluginStartupFailureFixture = PluginFailureEvidence & {
  fixture: CopilotPluginFixture;
  nativeTrace: string;
  dispose(): Promise<void>;
};

/** Run the real selected-plugin preparation path against a private fake CLI. */
export async function createCopilotPluginStartupFailureFixture(
  apiClient: ApiClient,
  backend: BackendContext,
  seedData: SeedData,
  title: string,
): Promise<CopilotPluginStartupFailureFixture | null> {
  const binDir = path.join(backend.tmpDir, "copilot-plugin-failure-bin");
  const nativeTrace = path.join(backend.tmpDir, "copilot-plugin-failure-calls.log");
  prepareFakeNpx(path.join(binDir, "npx"), nativeTrace);

  let releaseEnv: (() => Promise<void>) | null = null;
  let fixture: CopilotPluginFixture | null = null;
  try {
    const inheritedPath = (process.env.PATH ?? "").split(path.delimiter).filter(Boolean);
    releaseEnv = await backend.useEnv({
      KANDEV_MOCK_AGENT: "false",
      PATH: [binDir, ...inheritedPath].join(path.delimiter),
      E2E_COPILOT_PLUGIN_TRACE: nativeTrace,
    });
    fixture = await createCopilotPluginFixture(apiClient);
    if (!fixture) {
      await releaseEnv();
      return null;
    }

    const evidence = await launchFailingCopilotTask(apiClient, seedData, fixture, title);
    assertNativeProbeCalls(nativeTrace);
    await assertDurablePrivacy(apiClient, evidence.taskId, evidence.sessionId);

    let disposed = false;
    return {
      ...evidence,
      fixture,
      nativeTrace,
      async dispose() {
        if (disposed) return;
        disposed = true;
        try {
          await fixture?.dispose();
        } finally {
          await releaseEnv?.();
        }
      },
    };
  } catch (error) {
    try {
      await fixture?.dispose();
    } finally {
      await releaseEnv?.();
    }
    throw error;
  }
}

function prepareFakeNpx(fakeNpx: string, nativeTrace: string) {
  fs.mkdirSync(path.dirname(fakeNpx), { recursive: true });
  fs.writeFileSync(nativeTrace, "");
  fs.writeFileSync(
    fakeNpx,
    `#!/bin/sh
set -eu
printf '%s\\n' "$*" >> "$E2E_COPILOT_PLUGIN_TRACE"
case "$*" in
  *--help*)
    printf 'Options:\\n  --plugin-dir <path>\\n'
    ;;
  *'plugin list'*--json*)
    printf '%s\\n' '${PRIVATE_NATIVE_DIAGNOSTIC} ${PRIVATE_PLUGIN_ID} ${PRIVATE_PLUGIN_PATH}'
    ;;
  *)
    printf '%s\\n' '${PRIVATE_NATIVE_DIAGNOSTIC}'
    exit 97
    ;;
esac
`,
    { mode: 0o755 },
  );
  fs.chmodSync(fakeNpx, 0o755);
}

async function launchFailingCopilotTask(
  apiClient: ApiClient,
  seedData: SeedData,
  fixture: CopilotPluginFixture,
  title: string,
): Promise<PluginFailureEvidence> {
  await apiClient.updateAgentProfile(fixture.profileId, {
    copilot_selected_plugins: [COPILOT_RECOVERY_PLUGIN_ID],
  });
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, fixture.profileId, {
    description: "Inspect the selected workspace.",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  if (!task.session_id) throw new Error("Copilot startup task has no session ID");

  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "FAILED",
    message: "Wait for Copilot plugin inventory preparation to fail",
    timeout: 45_000,
  });

  const { sessions } = await apiClient.listTaskSessions(task.id);
  const failed = sessions.find((session) => session.id === task.session_id) as
    | PluginFailureSession
    | undefined;
  const cause = requirePluginFailureCause(failed);
  return { taskId: task.id, sessionId: task.session_id, runtimeVersion: cause.runtime_version };
}

function requirePluginFailureCause(
  session: PluginFailureSession | undefined,
): PluginFailureCause & {
  runtime_version: string;
} {
  const cause = session?.metadata?.last_agent_error?.causes?.find(
    (candidate) => candidate.code === "plugin_preparation_failed",
  );
  if (cause?.reason !== "invalid_inventory" || !cause.runtime_version) {
    throw new Error(`Copilot plugin failure evidence was incomplete: ${JSON.stringify(cause)}`);
  }
  return cause as PluginFailureCause & { runtime_version: string };
}

function assertNativeProbeCalls(nativeTrace: string) {
  const calls = fs.readFileSync(nativeTrace, "utf8");
  if (calls.includes("--help") && calls.includes("plugin list") && calls.includes("--json")) return;
  throw new Error(`Fake native CLI did not observe help and inventory calls: ${calls}`);
}

async function assertDurablePrivacy(apiClient: ApiClient, taskId: string, sessionId: string) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const session = sessions.find((candidate) => candidate.id === sessionId) as
    | PluginFailureSession
    | undefined;
  const { messages } = await apiClient.listSessionMessages(sessionId);
  const persisted = JSON.stringify({ error: session?.metadata?.last_agent_error, messages });
  for (const secret of [PRIVATE_PLUGIN_ID, PRIVATE_PLUGIN_PATH, PRIVATE_NATIVE_DIAGNOSTIC]) {
    if (persisted.includes(secret)) {
      throw new Error(`Raw native plugin diagnostic escaped into durable session data: ${secret}`);
    }
  }
}

export const copilotPluginStartupFailurePrivacySentinels = [
  PRIVATE_PLUGIN_ID,
  PRIVATE_PLUGIN_PATH,
  PRIVATE_NATIVE_DIAGNOSTIC,
];

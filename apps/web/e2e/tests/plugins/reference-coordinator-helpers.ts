import { randomUUID } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { expect, test } from "../../fixtures/test-base";

export const COORDINATOR_PLUGIN_ID = "kandev-plugin-coordinator-template";

const PLUGIN_ROOTS = [
  path.resolve(__dirname, "../../../../../kandev-plugin-coordinator-template"),
  path.resolve(__dirname, "../../../../../../kandev-plugin-coordinator-template"),
];
const PLUGIN_ROOT =
  PLUGIN_ROOTS.find((root) => existsSync(path.join(root, "manifest.yaml"))) ?? PLUGIN_ROOTS[0];
const MANIFEST_PATH = path.join(PLUGIN_ROOT, "manifest.yaml");

function readPackagePath() {
  if (!existsSync(MANIFEST_PATH)) {
    throw new Error(`Packaged reference coordinator checkout is missing: ${PLUGIN_ROOT}`);
  }
  const manifest = readFileSync(MANIFEST_PATH, "utf8");
  const id = manifest.match(/^id:\s*["']?([a-z0-9-]+)["']?\s*$/m)?.[1];
  const version = manifest.match(/^version:\s*["']?([^"'\s]+)["']?\s*$/m)?.[1];
  if (!id || !version)
    throw new Error(`Could not read plugin id and version from ${MANIFEST_PATH}`);
  if (id !== COORDINATOR_PLUGIN_ID) {
    throw new Error(`Expected coordinator plugin id ${COORDINATOR_PLUGIN_ID}, found ${id}`);
  }
  return path.join(PLUGIN_ROOT, `${id}-${version}.tar.gz`);
}

const COORDINATOR_PACKAGE_AVAILABLE = (() => {
  try {
    return existsSync(readPackagePath());
  } catch {
    return false;
  }
})();

export async function installAndGrantCoordinator(
  page: Page,
  apiClient: ApiClient,
  workspaceId: string,
): Promise<void> {
  test.skip(
    !COORDINATOR_PACKAGE_AVAILABLE,
    "Reference coordinator E2E requires the packaged sibling checkout",
  );
  const packagePath = readPackagePath();
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").click();
  await expect(page.getByTestId("install-plugin-dialog")).toBeVisible();
  await page.getByTestId("install-plugin-tab-upload").click();
  await page.getByTestId("install-plugin-file-input").setInputFiles(packagePath);
  await page.getByTestId("install-plugin-upload-submit").click();
  await expect(page.getByTestId("install-plugin-dialog")).toBeHidden({ timeout: 30_000 });
  await expect(page.getByTestId(`plugin-row-${COORDINATOR_PLUGIN_ID}`)).toBeVisible({
    timeout: 30_000,
  });

  const contextResponse = await apiClient.rawRequest(
    "GET",
    `/api/plugins/${COORDINATOR_PLUGIN_ID}/capability-approvals?workspace_id=${encodeURIComponent(workspaceId)}`,
  );
  if (!contextResponse.ok) {
    throw new Error(`Could not read coordinator capability context: ${contextResponse.status}`);
  }
  const context = (await contextResponse.json()) as {
    manifest_digest: string;
    approval?: { revision: number };
    declared_capability_ids: string[];
  };
  const approvalResponse = await apiClient.rawRequest(
    "PUT",
    `/api/plugins/${COORDINATOR_PLUGIN_ID}/capability-approvals`,
    {
      workspace_id: workspaceId,
      expected_revision: context.approval?.revision ?? 0,
      manifest_digest: context.manifest_digest,
      capability_ids: context.declared_capability_ids,
      reason: "Exercise the reference coordinator's declared Host APIs",
      audit_id: `reference-coordinator-${randomUUID()}`,
    },
  );
  if (!approvalResponse.ok) {
    throw new Error(`Could not grant coordinator Host access: ${approvalResponse.status}`);
  }
}

export async function uninstallCoordinator(apiClient: ApiClient): Promise<void> {
  await apiClient
    .rawRequest("DELETE", `/api/plugins/${COORDINATOR_PLUGIN_ID}`)
    .catch(() => undefined);
}

export async function verifyCoordinatorRetryStaysWithInstance(
  page: Page,
  input: { sourceKey: string; sourceName: string; targetName: string; gesture: "click" | "tap" },
): Promise<void> {
  async function selectInstance(name: string) {
    const picker = page.getByTestId("managed-chat-instance-picker");
    await picker[input.gesture]();
    await page.getByRole("dialog").getByRole("button", { name })[input.gesture]();
  }

  await selectInstance(input.sourceName);
  let releaseLateReads: () => void = () => undefined;
  let markStatusRead: () => void = () => undefined;
  let markInputRead: () => void = () => undefined;
  let markLateStatusFinished: () => void = () => undefined;
  let markLateInputsFinished: () => void = () => undefined;
  const lateReadGate = new Promise<void>((resolve) => {
    releaseLateReads = resolve;
  });
  const statusReadSeen = new Promise<void>((resolve) => {
    markStatusRead = resolve;
  });
  const inputReadSeen = new Promise<void>((resolve) => {
    markInputRead = resolve;
  });
  const lateStatusFinished = new Promise<void>((resolve) => {
    markLateStatusFinished = resolve;
  });
  const lateInputsFinished = new Promise<void>((resolve) => {
    markLateInputsFinished = resolve;
  });
  let deferredStatus = false;
  let deferredInputs = false;
  await page.route(
    `**/api/plugins/${COORDINATOR_PLUGIN_ID}/actions/conversation.status`,
    async (route) => {
      const envelope = route.request().postDataJSON() as { body?: Record<string, unknown> };
      const body = envelope.body ?? envelope;
      const delayedSourceRead = !deferredStatus && body.instance_key === input.sourceKey;
      if (delayedSourceRead) {
        deferredStatus = true;
        markStatusRead();
        await lateReadGate;
      }
      await route.continue();
      if (delayedSourceRead) markLateStatusFinished();
    },
  );
  await page.route(
    `**/api/plugins/${COORDINATOR_PLUGIN_ID}/actions/conversation.inputs`,
    async (route) => {
      const envelope = route.request().postDataJSON() as { body?: Record<string, unknown> };
      const body = envelope.body ?? envelope;
      const delayedSourceRead = !deferredInputs && body.instance_key === input.sourceKey;
      if (delayedSourceRead) {
        deferredInputs = true;
        markInputRead();
        await lateReadGate;
        const now = new Date().toISOString();
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            inputs: [
              {
                hostInputId: "stale-source-input",
                occurrenceKey: "stale-source-occurrence",
                sequence: 1,
                origin: "user",
                payload: "stale source input",
                conversationRevision: 1,
                state: "accepted",
                createdAt: now,
                updatedAt: now,
              },
            ],
            nextSequenceCursor: 1,
            hasMore: false,
          }),
        });
        markLateInputsFinished();
        return;
      }
      await route.continue();
    },
  );

  const attempts: Array<Record<string, unknown>> = [];
  let releaseFirst: () => void = () => undefined;
  let markFirstSeen: () => void = () => undefined;
  const firstSeen = new Promise<void>((resolve) => {
    markFirstSeen = resolve;
  });
  const firstResponseGate = new Promise<void>((resolve) => {
    releaseFirst = resolve;
  });
  await page.route(
    `**/api/plugins/${COORDINATOR_PLUGIN_ID}/actions/conversation.enqueue`,
    async (route) => {
      const envelope = route.request().postDataJSON() as { body?: Record<string, unknown> };
      const body = envelope.body ?? envelope;
      attempts.push(body);
      if (attempts.length === 1) {
        markFirstSeen();
        await firstResponseGate;
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "uncertain" }),
        });
        return;
      }
      const now = new Date().toISOString();
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          status: "APPLIED",
          input: {
            hostInputId: "e2e-retried-input",
            occurrenceKey: body.occurrence_key,
            sequence: 1,
            origin: "user",
            payload: body.payload,
            conversationRevision: 1,
            state: "accepted",
            createdAt: now,
            updatedAt: now,
          },
        }),
      });
    },
  );

  await selectInstance(input.sourceName);
  await Promise.all([statusReadSeen, inputReadSeen]);
  await selectInstance(input.targetName);
  releaseLateReads();
  await Promise.all([lateStatusFinished, lateInputsFinished]);
  await expect(page.getByTestId("managed-chat-instance-picker")).toContainText(input.targetName);
  await expect(page.getByTestId("managed-chat-queued-count")).toBeHidden();

  await selectInstance(input.sourceName);
  const message = `Keep retry with ${input.sourceName}`;
  await page.getByLabel("Message").fill(message);
  await page.getByRole("button", { name: "Send" })[input.gesture]();
  await firstSeen;
  await selectInstance(input.targetName);
  releaseFirst();
  await expect(page.getByTestId("managed-chat-instance-picker")).toContainText(input.targetName);
  await expect(page.getByRole("button", { name: "Retry" })).toBeHidden();
  await expect.poll(() => attempts.length).toBe(1);

  await selectInstance(input.sourceName);
  await expect(page.getByLabel("Message")).toHaveValue(message);
  await expect(page.getByRole("button", { name: "Retry" })).toBeVisible();
  await page.getByRole("button", { name: "Retry" })[input.gesture]();
  await expect.poll(() => attempts.length).toBe(2);
  expect(attempts[0]).toEqual(
    expect.objectContaining({ instance_key: input.sourceKey, payload: message }),
  );
  expect(attempts[1]).toEqual(
    expect.objectContaining({ instance_key: input.sourceKey, payload: message }),
  );
  for (const key of ["request_id", "idempotency_key", "occurrence_key"]) {
    expect(attempts[1]?.[key]).toBe(attempts[0]?.[key]);
  }
}

export async function createLinkedJiraTask(
  apiClient: ApiClient,
  input: {
    workspaceId: string;
    workflowId: string;
    workflowStepId: string;
    agentProfileId: string;
  },
): Promise<{ taskId: string; watchId: string }> {
  await apiClient.setJiraConfig({
    workspaceId: input.workspaceId,
    siteUrl: "https://acme.atlassian.net",
    email: "alice@example.com",
    secret: "api-token-value",
  });
  await apiClient.waitForIntegrationAuthHealthy("jira", { workspaceId: input.workspaceId });

  const suffix = Math.floor(Date.now() / 1000)
    .toString()
    .slice(-7);
  const ticketKey = `KCOORD-${suffix}`;
  const transitions = [
    {
      id: "start",
      name: "Start progress",
      toStatusId: "in-progress",
      toStatusName: "In Progress",
    },
  ];
  const ticket = {
    key: ticketKey,
    summary: `Coordinator linked issue ${suffix}`,
    description: "Exercise the reference coordinator's linked-issue tools.",
    statusId: "open",
    statusName: "Open",
    statusCategory: "To Do",
    projectKey: "KCOORD",
    url: `https://acme.atlassian.net/browse/${ticketKey}`,
    transitions,
  };
  await apiClient.mockJiraAddTickets([ticket]);
  await apiClient.mockJiraSetSearchHits([ticket]);
  await apiClient.mockJiraAddTransitions(ticketKey, transitions);

  const before = new Set(
    (await apiClient.listTasks(input.workspaceId)).tasks.map((task) => task.id),
  );
  const watchResponse = await apiClient.rawRequest("POST", "/api/v1/jira/watches/issue", {
    workspaceId: input.workspaceId,
    workflowId: input.workflowId,
    workflowStepId: input.workflowStepId,
    jql: `key = ${ticketKey}`,
    agentProfileId: input.agentProfileId,
    prompt: "Review the linked issue and keep its task evidence current.",
    pollIntervalSeconds: 60,
  });
  const watchBody = await watchResponse.text();
  if (!watchResponse.ok)
    throw new Error(`Could not create linked Jira watch (${watchResponse.status}): ${watchBody}`);
  const watch = JSON.parse(watchBody) as { id: string };
  const watchPath = `/api/v1/jira/watches/issue/${watch.id}`;
  const workspaceQuery = `?workspace_id=${encodeURIComponent(input.workspaceId)}`;
  const scopedWatchPath = `${watchPath}${workspaceQuery}`;
  try {
    const trigger = await apiClient.rawRequest("POST", `${watchPath}/trigger${workspaceQuery}`);
    if (!trigger.ok) throw new Error(`Could not trigger the linked Jira watch: ${trigger.status}`);
    let taskId = "";
    await expect
      .poll(
        async () => {
          const current = await apiClient.listTasks(input.workspaceId);
          taskId = current.tasks.find((task) => !before.has(task.id))?.id ?? "";
          return taskId;
        },
        { timeout: 30_000 },
      )
      .not.toBe("");
    return { taskId, watchId: watch.id };
  } catch (error) {
    await apiClient.rawRequest("DELETE", scopedWatchPath).catch(() => undefined);
    throw error;
  }
}

export async function invokeCoordinatorAction<T>(
  apiClient: ApiClient,
  workspaceId: string,
  action: string,
  body: Record<string, unknown> = {},
): Promise<T> {
  const response = await apiClient.rawRequest(
    "POST",
    `/api/plugins/${COORDINATOR_PLUGIN_ID}/actions/${action}`,
    { workspaceId, body },
  );
  const text = await response.text();
  if (!response.ok) throw new Error(`Coordinator ${action} failed (${response.status}): ${text}`);
  return JSON.parse(text) as T;
}

import { expect, type Locator, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import type { ApiClient, QueueSessionIdentityInput } from "./api-client";
import { watchWs, type WsWatcher } from "./causal-waits";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import { mockTurnGate, type MockTurnGate } from "./mock-turn-gate";
import { requestMessageQueueSettings, restoreMessageQueueSettings } from "./message-queue-settings";
import { waitForSessionDone } from "./session";

type Options = {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  tmpDir: string;
  mobile: boolean;
};
type QueueFixture = {
  identity: QueueSessionIdentityInput;
  senderA: { id: string; session_id?: string };
  gates: {
    parent: MockTurnGate;
    a2: MockTurnGate;
    a3: MockTurnGate;
    a4: MockTurnGate;
    b2: MockTurnGate;
    b3: MockTurnGate;
  };
};
const bodies = {
  a1: "ASM A1",
  a2: "ASM A2",
  a3: "ASM A3",
  a4: "ASM A4",
  b1: "ASM B1",
  b2: "ASM B2",
  b3: "ASM B3",
};
const initial = [`${bodies.a1}\n\n${bodies.a2}`, bodies.b1];
const combined = (keys: Array<keyof typeof bodies>) => keys.map((key) => bodies[key]).join("\n\n");
const peerMessage = (taskId: string, prompt: string) =>
  `e2e:mcp:kandev:message_task_kandev(${JSON.stringify({ task_id: taskId, prompt })})`;

async function expectQueue(
  apiClient: ApiClient,
  identity: QueueSessionIdentityInput,
  expected: string[],
) {
  await expect
    .poll(
      async () => {
        const status = await apiClient.getQueueStatus(identity);
        return status.entries.map((entry) =>
          (entry.content.match(/ASM (?:[AB][1-4]|incompatible)/g) ?? []).join("\n\n"),
        );
      },
      { message: `Persisted queue should contain ${JSON.stringify(expected)}`, timeout: 30_000 },
    )
    .toEqual(expected);
}

async function seedQueue(options: Options, gates: QueueFixture["gates"]): Promise<QueueFixture> {
  const { apiClient, seedData } = options;
  const taskOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    executor_profile_id: seedData.worktreeExecutorProfileId,
  };
  const parent = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Auto-sort-merge receiver",
    seedData.agentProfileId,
    { ...taskOptions, description: gates.parent.prompt },
  );
  expect(parent.session_id).toBeTruthy();
  const identity = await apiClient.getQueueSessionIdentity(parent.id, parent.session_id!);
  await apiClient.setQueueAutoRun(identity, false);
  const childOptions = {
    ...taskOptions,
    parent_id: parent.id,
    workspace_mode: "inherit_parent" as const,
  };
  const senderA = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Auto-sort-merge sender A",
    seedData.agentProfileId,
    {
      ...childOptions,
      description: [
        peerMessage(parent.id, bodies.a1),
        gates.a2.prompt,
        peerMessage(parent.id, bodies.a2),
        gates.a3.prompt,
        peerMessage(parent.id, bodies.a3),
        gates.a4.prompt,
        peerMessage(parent.id, bodies.a4),
        'e2e:message("Sender A finished")',
      ].join("\n"),
    },
  );
  await expectQueue(apiClient, identity, [bodies.a1]);
  await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Auto-sort-merge sender B",
    seedData.agentProfileId,
    {
      ...childOptions,
      description: [
        peerMessage(parent.id, bodies.b1),
        gates.b2.prompt,
        peerMessage(parent.id, bodies.b2),
        gates.b3.prompt,
        peerMessage(parent.id, bodies.b3),
        'e2e:message("Sender B finished")',
      ].join("\n"),
    },
  );
  await expectQueue(apiClient, identity, [bodies.a1, bodies.b1]);
  await gates.a2.release();
  await expectQueue(apiClient, identity, initial);
  return { identity, senderA, gates };
}

async function openQueue(page: Page, identity: QueueSessionIdentityInput) {
  await page.goto(`/t/${identity.taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  const chat = session.activeChat();
  await chat.getByTestId("queue-chip").click();
  const panel = chat.getByTestId("queued-ghost-list");
  await expect(panel).toBeVisible();
  return panel;
}

async function togglePolicy(panel: Locator, watcher: WsWatcher, mobile: boolean, enabled: boolean) {
  const pill = panel.getByTestId("queue-auto-merge");
  await expect(pill).toHaveCount(1);
  await expect(pill).toBeEnabled();
  const response = watcher.waitForResponse("message.queue.auto_merge.set");
  await (mobile ? pill.tap() : pill.click());
  await response;
  await expect(pill).toBeChecked({ checked: enabled });
}

async function expectRows(panel: Locator, expected: string[]) {
  const rows = panel.getByTestId("queue-entry-text");
  await expect(rows).toHaveCount(expected.length);
  for (const [index, content] of expected.entries()) {
    for (const body of content.split("\n\n")) await expect(rows.nth(index)).toContainText(body);
  }
}

async function checkAdmissions(options: Options, fixture: QueueFixture) {
  const { page, apiClient } = options;
  const { identity, gates } = fixture;
  const panel = await openQueue(page, identity);
  await expect(panel.getByTestId("queue-auto-merge")).toBeChecked();
  await expectRows(panel, initial);
  await gates.b2.release();
  const expected = [combined(["a1", "a2"]), combined(["b1", "b2"])];
  await expectQueue(apiClient, identity, expected);
  await expectRows(panel, expected);
  return panel;
}

async function checkCompaction(
  options: Options,
  fixture: QueueFixture,
  panel: Locator,
  watcher: WsWatcher,
) {
  const { apiClient, page, mobile } = options;
  const { identity, gates } = fixture;
  await requestMessageQueueSettings(apiClient, "PATCH", { auto_merge_enabled: false });
  await gates.b3.release();
  await expectQueue(apiClient, identity, [
    combined(["a1", "a2"]),
    combined(["b1", "b2"]),
    bodies.b3,
  ]);
  await gates.a3.release();
  const separate = [combined(["a1", "a2"]), combined(["b1", "b2"]), bodies.b3, bodies.a3];
  await expectQueue(apiClient, identity, separate);
  await requestMessageQueueSettings(apiClient, "PATCH", { auto_merge_enabled: true });
  await expectQueue(apiClient, identity, separate);
  panel = await openQueue(page, identity);
  await expectRows(panel, separate);
  await togglePolicy(panel, watcher, mobile, false);
  await togglePolicy(panel, watcher, mobile, true);
  const compacted = [combined(["a1", "a2", "a3"]), combined(["b1", "b2", "b3"])];
  await expectQueue(apiClient, identity, compacted);
  await expectRows(panel, compacted);
  await gates.a4.release();
  const admitted = [combined(["a1", "a2", "a3", "a4"]), compacted[1]];
  await expectQueue(apiClient, identity, admitted);
  await expectRows(panel, admitted);
  panel = await openQueue(page, identity);
  await expect(panel.getByTestId("queue-auto-merge")).toBeChecked();
  await expectRows(panel, admitted);
  expect((await apiClient.getQueueStatus(identity)).auto_run).toBe(false);
  return { panel, admitted };
}

async function checkMobileTargets(page: Page, panel: Locator) {
  for (const id of ["queue-auto-run", "queue-auto-merge"]) {
    const target = panel.getByTestId(id).locator("xpath=..");
    const box = await target.boundingBox();
    expect(box?.width).toBeGreaterThanOrEqual(44);
    expect(box?.height).toBeGreaterThanOrEqual(44);
  }
  await assertNoDocumentHorizontalOverflow(page);
}

export async function exerciseAutoSortMergeQueue(options: Options) {
  const { page, apiClient, tmpDir, mobile, seedData } = options;
  const baseline = (await requestMessageQueueSettings(apiClient, "GET")).settings;
  const watcher = watchWs(page);
  const gate = (name: string) => mockTurnGate(tmpDir, `Released ${name}`, { announceStart: false });
  const gates = {
    parent: gate("parent"),
    a2: gate("A2"),
    a3: gate("A3"),
    a4: gate("A4"),
    b2: gate("B2"),
    b3: gate("B3"),
  };
  try {
    const fixture = await seedQueue(options, gates);
    let panel = await checkAdmissions(options, fixture);
    const result = await checkCompaction(options, fixture, panel, watcher);
    panel = result.panel;
    if (mobile) await checkMobileTargets(page, panel);
    await waitForSessionDone(
      apiClient,
      fixture.senderA.id,
      fixture.senderA.session_id!,
      "Sender A completed its scripted turn",
    );
    await apiClient.launchSession({
      task_id: fixture.senderA.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: peerMessage(fixture.identity.taskId, "ASM incompatible"),
    });
    await expectQueue(apiClient, fixture.identity, [...result.admitted, "ASM incompatible"]);
    await expectRows(panel, [...result.admitted, "ASM incompatible"]);
    await page.screenshot({
      path: `test-results/auto-sort-merge-${mobile ? "mobile" : "desktop"}.png`,
    });
  } finally {
    await Promise.all(Object.values(gates).map((entry) => entry.release()));
    await restoreMessageQueueSettings(apiClient, baseline);
  }
}

import { execFile } from "node:child_process";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { promisify } from "node:util";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { PrAssetCapture } from "./pr-asset-capture";
import type { SeedData } from "../fixtures/test-base";
import type { BackendContext } from "../fixtures/backend";
import { SessionPage } from "../pages/session-page";
import { routeSessionEntryRecovery } from "./session-entry-recovery";
import { attachGatewayTrafficCapture } from "./ws-traffic";
import { assertDeliveryRecoveryGeometry } from "./delivery-recovery-controls";

type RecoveryBrowserContext = {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  testInfo: TestInfo;
  mobile: boolean;
  prCapture?: PrAssetCapture;
};

const execute = promisify(execFile);

type FixtureAction =
  | "seed"
  | "seed_ambiguous"
  | "seed_unblocked"
  | "assert_reconstructed"
  | "assert_ambiguous_unrepaired";

export async function deliveryRecordRecoveryFixture(
  fixtureRoot: string,
  sessionId: string,
  action: FixtureAction,
) {
  expect(path.basename(fixtureRoot)).toMatch(/^kandev-e2e-/);
  const result = await execute(
    path.resolve(
      process.cwd(),
      `../backend/bin/e2e-delivery-fixture${process.platform === "win32" ? ".exe" : ""}`,
    ),
    ["-test.run=^TestDeliveryRecordRecoveryFixture$", "-test.timeout=2m", "-test.v"],
    {
      cwd: path.resolve(process.cwd(), "../backend"),
      env: {
        ...process.env,
        KANDEV_E2E_RECORD_ROOT: fixtureRoot,
        KANDEV_E2E_RECORD_SESSION: sessionId,
        KANDEV_E2E_RECORD_ACTION: action,
      },
      timeout: 150_000,
      maxBuffer: 1 << 20,
    },
  ).catch((failure: unknown) => {
    const output = failure as { stdout?: string; stderr?: string };
    throw new Error(
      `Delivery record fixture failed:\n${output.stdout ?? ""}\n${output.stderr ?? ""}`,
    );
  });
  const marker = {
    seed: "KANDEV_E2E_RECORD_FIXTURE:seeded",
    seed_ambiguous: "KANDEV_E2E_RECORD_FIXTURE:seeded",
    seed_unblocked: "KANDEV_E2E_RECORD_FIXTURE:seeded",
    assert_reconstructed: "KANDEV_E2E_RECORD_FIXTURE:reconstructed_once",
    assert_ambiguous_unrepaired: "KANDEV_E2E_RECORD_FIXTURE:ambiguous_unrepaired",
  }[action];
  expect(result.stdout).toContain(marker);
}

export async function createDeliveryRecordRecoverySession(
  apiClient: ApiClient,
  seedData: SeedData,
  fixtureRoot: string,
  title: string,
  options: { ambiguous?: boolean; unblocked?: boolean } = {},
) {
  const { ambiguous = false, unblocked = false } = options;
  const { agents } = await apiClient.listAgents();
  const owner = agents.find((agent) =>
    agent.profiles?.some((profile) => profile.id === seedData.agentProfileId),
  );
  if (!owner) throw new Error("seed mock-agent profile owner is missing");

  const taskId = `delivery-record-${randomUUID()}`;
  const sessionId = `${taskId}-session`;
  const tracePath = path.join(fixtureRoot, `${taskId}-acp-trace.jsonl`);
  fs.writeFileSync(tracePath, "");
  const profile = await apiClient.createAgentProfile(owner.id, taskId, {
    model: "mock-fast",
    env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
  });
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: profile.id,
    repository_ids: [seedData.repositoryId],
  });
  const seeded = unblocked
    ? await apiClient.launchSession({
        task_id: task.id,
        agent_profile_id: profile.id,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        intent: "prepare",
        launch_workspace: true,
      })
    : await apiClient.seedTaskSession(task.id, {
        state: "WAITING_FOR_INPUT",
        sessionId,
        agentProfileId: profile.id,
        repositoryId: seedData.repositoryId,
      });
  if (unblocked) {
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status, {
        timeout: 60_000,
      })
      .toBe("ready");
    await apiClient.stopSession({ session_id: seeded.session_id, force: true });
  }
  let action: FixtureAction = "seed";
  if (ambiguous) action = "seed_ambiguous";
  else if (unblocked) action = "seed_unblocked";
  await deliveryRecordRecoveryFixture(fixtureRoot, seeded.session_id, action);
  const promptMessage = ambiguous
    ? "Ambiguous saved instruction 1"
    : "Inspect the saved work and continue from the same conversation";
  await expect
    .poll(async () => {
      const { messages } = await apiClient.listSessionMessages(seeded.session_id);
      return messages.some((message) => message.content === promptMessage);
    })
    .toBe(true);
  return { task, sessionId: seeded.session_id, tracePath, promptMessage };
}

export function mockPromptCount(tracePath: string): number {
  const contents = fs.readFileSync(tracePath, "utf8");
  return contents.split("\n").filter((line) => {
    if (!line.trim()) return false;
    try {
      return (JSON.parse(line) as { event?: string }).event === "prompt";
    } catch {
      return false;
    }
  }).length;
}

export async function exerciseDeliveryRecordRecovery({
  page,
  apiClient,
  seedData,
  backend,
  testInfo,
  mobile,
  prCapture,
}: RecoveryBrowserContext) {
  testInfo.setTimeout(240_000);
  const capture = attachGatewayTrafficCapture(page);
  const proxy = await routeSessionEntryRecovery(page);
  const broken = await createDeliveryRecordRecoverySession(
    apiClient,
    seedData,
    backend.tmpDir,
    mobile ? "Phone durable record recovery" : "Desktop durable record recovery",
  );
  const navigationTask = await apiClient.createTask(
    seedData.workspaceId,
    "Recovery navigation target",
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await apiClient.seedTaskSession(navigationTask.id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
  });

  await page.goto(`/t/${broken.task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  const banner = page.getByTestId("failed-session-banner");
  await expect(banner).toBeVisible({ timeout: 30_000 });
  await expect(page.getByTestId("user-message-delivery-blocked")).toContainText(
    "Delivery is blocked while session recovery is pending.",
  );
  await expect(page.getByText(broken.promptMessage, { exact: true })).toBeVisible();
  await expect(banner).toContainText("The saved prompt record is missing. Recovery is blocked.");
  await expect(banner.getByTestId("recovery-stop-button")).toBeVisible();
  await expect(banner.getByTestId("recovery-resume-button")).toHaveCount(0);
  await expect(page.getByTestId("interrupted-session-continuation")).toHaveCount(0);
  if (mobile) {
    await expect(page.locator("[data-testid='mobile-task-layout']:visible")).toBeVisible();
    await assertDeliveryRecoveryGeometry(page);
  }
  await page.screenshot({
    path: testInfo.outputPath(
      mobile ? "phone-record-recovery-before.png" : "desktop-record-recovery-before.png",
    ),
  });

  await prCapture?.screenshot("missing-record", {
    caption: "Retry and Stop remain available while the saved prompt record is missing.",
  });

  const promptCountBefore = mockPromptCount(broken.tracePath);
  expect(promptCountBefore).toBe(0);
  proxy.holdResponses("session.recover", { sessionId: broken.sessionId });
  const retryButton = banner.getByTestId("recovery-retry-connection-button");
  if (mobile) await retryButton.tap();
  else await retryButton.click();
  await expect.poll(() => proxy.heldResponseCount("session.recover")).toBe(1);
  await expect.poll(() => proxy.requestCount("session.recover")).toBe(1);
  await expect(
    banner.getByRole("status").filter({ hasText: "Retrying connection..." }),
  ).toBeVisible();
  await deliveryRecordRecoveryFixture(backend.tmpDir, broken.sessionId, "assert_reconstructed");
  expect(mockPromptCount(broken.tracePath)).toBe(promptCountBefore);
  expect(
    capture.frames.some(
      (frame) =>
        frame.direction === "sent" &&
        frame.action === "session.recover" &&
        frame.sessionId === broken.sessionId,
    ),
  ).toBe(true);

  await page.goto(`/t/${navigationTask.id}`);
  await new SessionPage(page).waitForLoad();
  proxy.releaseHeldResponses("session.recover");
  await expect(page.getByTestId("delivery-recovery-result")).toHaveCount(0);
  await page.goto(`/t/${broken.task.id}`);
  await page.reload();
  await session.waitForLoad();
  await expect(page.getByTestId("user-message-delivery-blocked")).toBeVisible();
  await expect(page.getByText(broken.promptMessage, { exact: true })).toBeVisible();
  await expect(banner).toContainText(
    "The record was restored, but the original process identity is missing. Continuing stays blocked.",
  );
  await expect(page.getByTestId("interrupted-session-continuation")).toHaveCount(0);
  if (mobile) await assertDeliveryRecoveryGeometry(page);
  await page.screenshot({
    path: testInfo.outputPath(
      mobile ? "phone-record-recovery-after.png" : "desktop-record-recovery-after.png",
    ),
  });

  await prCapture?.screenshot("record-restored", {
    caption:
      "Retry restored the saved record. Missing original process proof keeps continuation blocked.",
  });

  const firstRecovery = await recoverySnapshot(apiClient, broken.task.id, broken.sessionId);
  expect(firstRecovery.revision).toBe(1);
  expect(firstRecovery.messageCount).toBe(1);
  await retryIncompleteRecovery(page, banner, mobile);
  await deliveryRecordRecoveryFixture(backend.tmpDir, broken.sessionId, "assert_reconstructed");
  expect(mockPromptCount(broken.tracePath)).toBe(promptCountBefore);
  expect((await recoverySnapshot(apiClient, broken.task.id, broken.sessionId)).revision).toBe(1);

  await backend.restart();
  await backend.ensureReady();
  await page.reload();
  await session.waitForLoad();
  await expect(page.getByTestId("user-message-delivery-blocked")).toBeVisible();
  await expect(page.getByText(broken.promptMessage, { exact: true })).toBeVisible();
  await expect(banner).toContainText(
    "The record was restored, but the original process identity is missing. Continuing stays blocked.",
  );
  await retryIncompleteRecovery(page, banner, mobile);
  await deliveryRecordRecoveryFixture(backend.tmpDir, broken.sessionId, "assert_reconstructed");
  expect(mockPromptCount(broken.tracePath)).toBe(promptCountBefore);
  const afterRestart = await recoverySnapshot(apiClient, broken.task.id, broken.sessionId);
  expect(afterRestart.revision).toBe(1);
  expect(afterRestart.messageCount).toBe(1);

  const ambiguous = await createDeliveryRecordRecoverySession(
    apiClient,
    seedData,
    backend.tmpDir,
    mobile ? "Phone ambiguous durable records" : "Desktop ambiguous durable records",
    { ambiguous: true },
  );
  await page.goto(`/t/${ambiguous.task.id}`);
  await session.waitForLoad();
  const ambiguousBanner = page.getByTestId("failed-session-banner");
  await expect(ambiguousBanner).toBeVisible({ timeout: 30_000 });
  await expect(page.getByTestId("user-message-delivery-blocked")).toHaveCount(2);
  const ambiguousPromptCount = mockPromptCount(ambiguous.tracePath);
  expect(ambiguousPromptCount).toBe(0);
  const ambiguousRetry = ambiguousBanner.getByTestId("recovery-retry-connection-button");
  if (mobile) await ambiguousRetry.tap();
  else await ambiguousRetry.click();
  await expect(ambiguousBanner.getByTestId("delivery-recovery-result")).toContainText(
    "More than one saved prompt could own this delivery. Recovery remains blocked.",
    { timeout: 30_000 },
  );
  await expect(page.getByTestId("interrupted-session-continuation")).toHaveCount(0);
  await deliveryRecordRecoveryFixture(
    backend.tmpDir,
    ambiguous.sessionId,
    "assert_ambiguous_unrepaired",
  );
  expect(mockPromptCount(ambiguous.tracePath)).toBe(ambiguousPromptCount);
}

async function retryIncompleteRecovery(
  page: Page,
  banner: ReturnType<Page["getByTestId"]>,
  mobile: boolean,
) {
  const retry = banner.getByTestId("recovery-retry-connection-button");
  if (mobile) await retry.tap();
  else await retry.click();
  await expect(banner.getByTestId("delivery-recovery-result")).toContainText(
    "The record was restored, but the original process identity is missing. Continuing stays blocked.",
    { timeout: 30_000 },
  );
  await expect(page.getByTestId("interrupted-session-continuation")).toHaveCount(0);
}

async function recoverySnapshot(apiClient: ApiClient, taskId: string, sessionId: string) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const session = sessions.find((candidate) => candidate.id === sessionId);
  const recovery = session?.metadata?.agent_delivery_recovery as { revision?: number } | undefined;
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return {
    revision: recovery?.revision,
    messageCount: messages.filter((message) => message.author_type === "user").length,
  };
}

export async function exerciseLiveDeliveryRecordBlock({
  page,
  apiClient,
  seedData,
  backend,
  testInfo,
  mobile,
}: RecoveryBrowserContext) {
  testInfo.setTimeout(180_000);
  const broken = await createDeliveryRecordRecoverySession(
    apiClient,
    seedData,
    backend.tmpDir,
    "Live delivery recovery",
    { unblocked: true },
  );
  const { sessions } = await apiClient.listTaskSessions(broken.task.id);
  const initial = sessions.find((candidate) => candidate.id === broken.sessionId);
  expect(initial?.metadata?.last_agent_error).toBeUndefined();
  expect(initial?.metadata?.agent_delivery_recovery).toBeUndefined();
  expect(initial?.session_recovery_blocks ?? []).toEqual([]);
  const traffic = attachGatewayTrafficCapture(page);
  await page.goto(`/t/${broken.task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await apiClient
    .addUserMessage(broken.task.id, broken.sessionId, "Inspect the saved work before continuing")
    .catch((error: unknown) => {
      expect(String(error)).toMatch(/recovery|delivery|unresolved/i);
    });
  const banner = page.getByTestId("failed-session-banner");
  await expect(banner.getByTestId("recovery-retry-connection-button")).toBeVisible({
    timeout: 30_000,
  });
  await expect(banner.getByTestId("recovery-stop-button")).toBeVisible();
  await expect
    .poll(() =>
      traffic.frames.some(
        (frame) => frame.direction === "received" && frame.action === "session.state_changed",
      ),
    )
    .toBe(true);
  if (mobile) await assertDeliveryRecoveryGeometry(page);
  expect(mockPromptCount(broken.tracePath)).toBe(0);
}

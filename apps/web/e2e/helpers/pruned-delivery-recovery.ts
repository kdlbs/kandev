import { expect, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import type { BackendContext } from "../fixtures/backend";
import { SessionPage } from "../pages/session-page";
import { attachGatewayTrafficCapture } from "./ws-traffic";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import {
  createDeliveryRecordRecoverySession,
  deliveryRecordRecoveryFixture,
  mockPromptCount,
} from "./delivery-record-recovery";

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.4
// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.5
// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.6
// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.7
export async function exercisePrunedDeliveryRecovery({
  page,
  api,
  seed,
  backend,
  info,
  mobile,
}: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  info: TestInfo;
  mobile: boolean;
}) {
  info.setTimeout(240_000);
  const broken = await createDeliveryRecordRecoverySession(
    api,
    seed,
    backend.tmpDir,
    "Recover pruned delivery history",
    { unblocked: true, pruned: true },
  );
  const before = (await api.getTaskSession(broken.sessionId)).session;
  expect(before.metadata?.agent_delivery_recovery).toBeUndefined();
  const initialPrompts = mockPromptCount(broken.tracePath);
  const traffic = attachGatewayTrafficCapture(page);
  await backend.restart();
  // Passive backend evidence proves recovery before visiting the session can launch it.
  await expect
    .poll(
      async () => {
        const recovery = (await api.getTaskSession(broken.sessionId)).session.metadata
          ?.agent_delivery_recovery as { phase?: string } | undefined;
        return recovery?.phase;
      },
      { timeout: 90_000 },
    )
    .toBe("restored");
  await deliveryRecordRecoveryFixture(backend.tmpDir, broken.sessionId, "assert_pruned_restored");
  const restored = (await api.getTaskSession(broken.sessionId)).session;
  expect(restored.id).toBe(before.id);
  expect(restored.task_environment_id).toBe(before.task_environment_id);
  expect(restored.workspace_path).toBe(before.workspace_path);
  expect(restored.worktree_path).toBe(before.worktree_path);
  expect(restored.worktree_branch).toBe(before.worktree_branch);
  expect(mockPromptCount(broken.tracePath)).toBe(initialPrompts);
  await page.goto(`/t/${broken.task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(page.getByTestId("delivery-history-notice")).toBeVisible();
  await expect(page.getByTestId("failed-session-banner")).toHaveCount(0);
  await expect(session.activeChat().locator(".tiptap.ProseMirror:visible")).toBeEditable();
  if (mobile) {
    await expect(page.locator("[data-testid='mobile-task-layout']:visible")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(page, "restored delivery history");
  }
  await page.screenshot({
    path: info.outputPath(mobile ? "phone-pruned-restored.png" : "desktop-pruned-restored.png"),
  });
  await backend.restart();
  await page.reload();
  await session.waitForLoad();
  await expect(page.getByTestId("delivery-history-notice")).toBeVisible();
  await expect(session.activeChat().locator(".tiptap.ProseMirror:visible")).toBeEditable({
    timeout: 60_000,
  });
  expect(mockPromptCount(broken.tracePath)).toBe(initialPrompts);
  expect(
    traffic.frames.filter(
      (frame) =>
        frame.direction === "sent" &&
        frame.action === "session.recover" &&
        frame.sessionId === broken.sessionId,
    ),
  ).toHaveLength(0);
  const newPrompt = "New instruction after honest history recovery";
  await session.sendMessageViaButton(newPrompt);
  await expect
    .poll(() => mockPromptCount(broken.tracePath), { timeout: 30_000 })
    .toBe(initialPrompts + 1);
  await expect
    .poll(async () => (await api.getTaskSession(broken.sessionId)).session.state, {
      timeout: 30_000,
    })
    .toBe("WAITING_FOR_INPUT");
  await deliveryRecordRecoveryFixture(backend.tmpDir, broken.sessionId, "assert_pruned_restored");
  const users = (await api.listSessionMessages(broken.sessionId)).messages.filter(
    (message) => message.author_type === "user",
  );
  expect(users.map((message) => message.content).slice(-2)).toEqual([
    broken.promptMessage,
    newPrompt,
  ]);
  await expect(page.getByTestId("delivery-history-notice")).toBeVisible();
}

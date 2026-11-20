import { execFile } from "node:child_process";
import path from "node:path";
import { promisify } from "node:util";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { PrAssetCapture } from "./pr-asset-capture";
import type { SeedData } from "../fixtures/test-base";
import type { BackendContext } from "../fixtures/backend";
import { SessionPage } from "../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

const execute = promisify(execFile);

export async function runSendNowNotDeliveredFixture(
  fixtureRoot: string,
  sessionId: string,
): Promise<void> {
  expect(path.basename(fixtureRoot)).toMatch(/^kandev-e2e-/);
  const result = await execute(
    path.resolve(
      process.cwd(),
      `../backend/bin/e2e-send-now-fixture${process.platform === "win32" ? ".exe" : ""}`,
    ),
    ["-test.run=^TestSendNowNotDeliveredFixture$", "-test.timeout=2m", "-test.v"],
    {
      cwd: path.resolve(process.cwd(), "../backend"),
      env: {
        ...process.env,
        KANDEV_E2E_SEND_NOW_ROOT: fixtureRoot,
        KANDEV_E2E_SEND_NOW_SESSION: sessionId,
      },
      timeout: 150_000,
      maxBuffer: 1 << 20,
    },
  ).catch((failure: unknown) => {
    const output = failure as { stdout?: string; stderr?: string };
    throw new Error(
      `Send Now not-delivered fixture failed:\n${output.stdout ?? ""}\n${output.stderr ?? ""}`,
    );
  });
  expect(result.stdout).toContain("KANDEV_E2E_SEND_NOW_FIXTURE:released");
}

export async function createSendNowNotDeliveredSession(
  apiClient: ApiClient,
  seedData: SeedData,
  fixtureRoot: string,
  title: string,
) {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  const seeded = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
  });
  await runSendNowNotDeliveredFixture(fixtureRoot, seeded.session_id);
  return { task, sessionId: seeded.session_id };
}

type NotDeliveredContext = {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  testInfo: TestInfo;
  mobile: boolean;
  prCapture?: PrAssetCapture;
};

export async function exerciseSendNowNotDelivered({
  page,
  apiClient,
  seedData,
  backend,
  testInfo,
  mobile,
  prCapture,
}: NotDeliveredContext) {
  testInfo.setTimeout(180_000);
  const broken = await createSendNowNotDeliveredSession(
    apiClient,
    seedData,
    backend.tmpDir,
    mobile ? "Phone Send Now claim loss" : "Desktop Send Now claim loss",
  );

  await page.goto(`/t/${broken.task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();

  const badge = page.getByTestId("user-message-delivery-not-delivered");
  await expect(badge).toBeVisible({ timeout: 30_000 });
  await expect(badge).toHaveAttribute("role", "status");
  await expect(badge).toContainText("Not delivered. Send it again.");
  await expect(page.getByText("Fix the button spacing (2 notes)", { exact: true })).toBeVisible();
  if (mobile) {
    await expect(page.locator("[data-testid='mobile-task-layout']:visible")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(page, "Send Now not-delivered badge");
  }

  await page.reload();
  await session.waitForLoad();
  await expect(page.getByTestId("user-message-delivery-not-delivered")).toBeVisible({
    timeout: 30_000,
  });
  await expect(page.getByText("Fix the button spacing (2 notes)", { exact: true })).toBeVisible();
  if (mobile) {
    await assertNoDocumentHorizontalOverflow(page, "Send Now not-delivered badge after reload");
  }

  await page.screenshot({
    path: testInfo.outputPath(
      mobile ? "phone-send-now-not-delivered.png" : "desktop-send-now-not-delivered.png",
    ),
  });
  await prCapture?.screenshot("send-now-not-delivered", {
    caption: "A Send Now message that could not be delivered stays in history with a badge.",
  });
}

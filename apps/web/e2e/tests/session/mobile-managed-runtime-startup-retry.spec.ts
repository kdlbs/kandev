// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect, test } from "../../fixtures/test-base";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import {
  managedRuntimeStartupAttemptFile,
  prepareManagedRuntimeProfile,
  restoreE2EAgentRegistry,
} from "../../helpers/managed-runtime-recovery";
import { waitForSessionDone } from "../../helpers/session";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

test("shows startup retry progress on phone and finishes in the same conversation", async ({
  apiClient,
  backend,
  seedData,
  testPage,
}, testInfo) => {
  test.skip(process.platform === "win32", "the deterministic npx fixture is a POSIX script");
  test.setTimeout(180_000);

  const launchId = randomUUID();
  let profileId = "";
  let hostFixturePath = "";
  try {
    const prepared = await prepareManagedRuntimeProfile(apiClient, backend, {
      startupMode: "silent-exit",
      launchId,
      hostSubprocess: true,
    });
    profileId = prepared.profile.id;
    hostFixturePath = prepared.hostFixturePath ?? "";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Phone managed runtime startup retry",
      prepared.profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("managed runtime task did not return a session ID");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const progress = session.activeChat().getByText("Retrying agent startup (attempt 2 of 2)", {
      exact: true,
    });
    await expect(progress).toBeVisible({ timeout: 30_000 });
    await expect(progress).toBeInViewport();
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "Wait for silent startup exit recovery on phone",
    );
    expect(fs.readFileSync(managedRuntimeStartupAttemptFile(launchId), "utf8").trim()).toBe("2");
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.map((candidate) => candidate.id)).toContain(task.session_id);
    const { messages } = await apiClient.listSessionMessages(task.session_id);
    expect(
      messages.some(
        (message) =>
          message.author_type === "agent" && message.content.includes("simple mock response"),
      ),
    ).toBe(true);
    await assertNoDocumentHorizontalOverflow(testPage, "phone managed runtime startup retry");
    await testPage.screenshot({
      path: testInfo.outputPath("managed-runtime-startup-retry-mobile.png"),
      fullPage: true,
    });
  } finally {
    if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
    if (hostFixturePath) fs.rmSync(hostFixturePath, { force: true });
    await restoreE2EAgentRegistry(backend);
  }
});

test("keeps the exhausted startup recovery card touch-safe on phone", async ({
  apiClient,
  backend,
  seedData,
  testPage,
}, testInfo) => {
  test.skip(process.platform === "win32", "the deterministic npx fixture is a POSIX script");
  test.setTimeout(180_000);

  const launchId = randomUUID();
  let profileId = "";
  let hostFixturePath = "";
  try {
    const prepared = await prepareManagedRuntimeProfile(apiClient, backend, {
      startupMode: "repeat-failure",
      launchId,
      hostSubprocess: true,
    });
    profileId = prepared.profile.id;
    hostFixturePath = prepared.hostFixturePath ?? "";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Phone managed runtime startup failure",
      prepared.profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("managed runtime task did not return a session ID");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const card = session.activeChat().getByTestId("session-recovery-card");
    await expect(card).toBeVisible({ timeout: 45_000 });
    await expect(
      card.getByRole("heading", { name: "npm could not prepare the runtime" }),
    ).toBeVisible();
    const retry = card.getByTestId("managed-runtime-npm-retry-button");
    await expect(retry).toHaveCount(1);
    await expect(retry).toBeInViewport();
    const box = await retry.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    await expect(card.locator("details")).not.toHaveAttribute("open");
    await expect(testPage.getByRole("dialog")).toHaveCount(0);
    expect(fs.readFileSync(managedRuntimeStartupAttemptFile(launchId), "utf8").trim()).toBe("2");
    await assertNoDocumentHorizontalOverflow(testPage, "phone managed runtime startup failure");
    await testPage.screenshot({
      path: testInfo.outputPath("managed-runtime-startup-failure-mobile.png"),
      fullPage: true,
    });
  } finally {
    if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
    if (hostFixturePath) fs.rmSync(hostFixturePath, { force: true });
    await restoreE2EAgentRegistry(backend);
  }
});

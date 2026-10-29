import { test, expect } from "../../fixtures/test-base";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";
import path from "node:path";

test.describe.configure({ retries: 0 });

test("copies the full identifier by touch within the phone detail", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  const profile = await createStandardProfile(apiClient, "Copy commit phone profile");
  await apiClient.createTaskWithAgent(seedData.workspaceId, "Copy commit phone", profile.id, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile("copy-commit-phone.txt", "Synthetic phone commit\n");
  git.stageAll();
  const sha = git.commit("Synthetic copy identifier phone");
  const session = await openTaskSession(testPage, "Copy commit phone");
  await session.waitForChatIdle({ timeout: 30_000 });
  await testPage.getByRole("button", { name: "Changes" }).tap();
  const toggle = testPage.getByTestId("commits-section-collapse-toggle");
  await expect(toggle).toBeVisible();
  if ((await toggle.getAttribute("aria-expanded")) !== "true") await toggle.tap();
  await testPage.getByTestId(`commit-open-${sha.slice(0, 7)}`).tap();
  const detail = testPage.getByTestId("commit-detail-content");
  await expect(detail).toBeVisible();
  const action = detail.getByRole("button", { name: "Copy identifier" });
  await expect(action).toBeVisible();
  const box = await action.boundingBox();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
    true,
  );
  await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await action.tap();
  await expect.poll(() => testPage.evaluate(() => navigator.clipboard.readText())).toBe(sha);
  await expect(detail.getByRole("status")).toHaveText("Copied!");
  await prCapture.screenshot("commit-copy-identifier-mobile", {
    caption: "Touch action copies the full commit identifier",
  });
});

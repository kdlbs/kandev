import path from "node:path";
import { expect, resetSeedRepositoryCheckout, test as base } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const test = base.extend<{ cleanSeedCheckout: void }>({
  cleanSeedCheckout: [
    async ({ backend, seedData }, use) => {
      resetSeedRepositoryCheckout(seedData, backend.tmpDir);
      try {
        await use();
      } finally {
        resetSeedRepositoryCheckout(seedData, backend.tmpDir);
      }
    },
    { auto: true },
  ],
});

// @covers AC-UI-FILE-TREE-NATURAL-ORDER-001.1
// @covers AC-UI-FILE-TREE-NATURAL-ORDER-001.3
test("renders natural order and selects the displayed range", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile("folder10/keep.txt", "folder ten\n");
  git.createFile("folder2/part10/keep.txt", "part ten\n");
  git.createFile("folder2/part2/keep.txt", "part two\n");
  git.createFile("folder2/step10.txt", "nested ten\n");
  git.createFile("folder2/step2.txt", "nested two sentinel\n");
  git.createFile("step10.txt", "root ten\n");
  git.createFile("step3.txt", "root three\n");
  git.createFile("step2.txt", "root two\n");
  git.stageAll();
  git.commit("add numbered fixture");
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Natural file order",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  await session.clickTab("Files");
  await expect(session.fileTreeNode("folder2")).toBeVisible();
  const paths = async () =>
    session.files
      .locator('[data-testid="file-tree-node"]')
      .evaluateAll((rows) => rows.map((row) => row.getAttribute("data-path")));
  const assertOrder = async (expected: string[]) => {
    await expect
      .poll(async () => (await paths()).filter((p) => p && expected.includes(p)))
      .toEqual(expected);
  };
  await assertOrder(["folder2", "folder10", "step2.txt", "step3.txt", "step10.txt"]);
  await session.fileTreeNode("folder2").click();
  await expect(session.fileTreeNode("folder2/step10.txt")).toBeVisible();
  await assertOrder([
    "folder2",
    "folder2/part2",
    "folder2/part10",
    "folder2/step2.txt",
    "folder2/step10.txt",
    "folder10",
    "step2.txt",
    "step3.txt",
    "step10.txt",
  ]);
  await prCapture.screenshot("desktop-file-tree-natural-order", {
    caption: "Numbered files and folders in natural order",
  });
  await session.fileTreeNode("step2.txt").click();
  await testPage.keyboard.down("Shift");
  await session.fileTreeNode("step10.txt").click();
  await testPage.keyboard.up("Shift");
  for (const name of ["step2.txt", "step3.txt", "step10.txt"]) {
    await expect(session.fileTreeNode(name)).toHaveAttribute("data-selected", "true");
  }
  await expect(session.fileTreeNode("folder10")).toHaveAttribute("data-selected", "false");
});

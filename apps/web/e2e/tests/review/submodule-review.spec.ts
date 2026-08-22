import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createSubmoduleReviewFixture,
  expectStickyReviewHeaderClearance,
  readGitValue,
} from "./submodule-review-helpers";

test.describe("Submodule review fixture", () => {
  test("cleans source repositories when setup fails", async ({ apiClient, seedData, backend }) => {
    const tempRoot = fs.mkdtempSync(path.join(backend.tmpDir, "submodule-review-failure-"));
    try {
      await expect(
        createSubmoduleReviewFixture(
          apiClient,
          { ...seedData, workspaceId: crypto.randomUUID() },
          tempRoot,
          "Failing submodule fixture E2E",
        ),
      ).rejects.toThrow();
      expect(fs.readdirSync(tempRoot)).toEqual([]);
    } finally {
      fs.rmSync(tempRoot, { recursive: true, force: true });
    }
  });
});

test.describe("Nested submodule Review", () => {
  test.describe.configure({ timeout: 180_000 });

  test("shows nested scopes and commits child gitlinks through the UI", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fixture = await createSubmoduleReviewFixture(
      apiClient,
      seedData,
      backend.tmpDir,
      "Nested submodule Review E2E",
    );

    try {
      await testPage.goto(`/t/${fixture.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 45_000 });

      const worktreePath = await fixture.waitForWorktree(apiClient);
      await fixture.applyNestedChanges(worktreePath);
      const parentBaseSha = readGitValue(worktreePath, ["rev-parse", "HEAD"], backend.tmpDir);

      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(() => session.changes.getByTestId("file-row-README.md").count(), {
          timeout: 45_000,
        })
        .toBeGreaterThan(0);

      await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
      const review = session.reviewDialog();
      await expect(review).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(() => review.getByTestId("submodule-node").count(), { timeout: 45_000 })
        .toBeGreaterThan(0);
      const outerNode = review.locator(
        '[data-testid="submodule-node"][data-repository-name="vendor/outer"]',
      );
      await expect(outerNode).toBeVisible();
      const innerNode = review.locator(
        '[data-testid="submodule-node"][data-repository-name="vendor/outer/vendor/inner"]',
      );
      if ((await innerNode.count()) === 0) {
        await outerNode.getByRole("button").click();
      }
      await expect(innerNode).toBeVisible();

      const readmeRows = review.locator(
        '[data-testid="review-file-row"][data-file-path="README.md"]',
      );
      await expect(readmeRows).toHaveCount(3);
      await expect
        .poll(() => session.reviewDiffText(), { timeout: 45_000 })
        .toEqual(expect.stringContaining("parent working-tree change"));
      for (const expected of ["outer committed change", "inner committed change"]) {
        await expect.poll(() => session.reviewDiffText(), { timeout: 45_000 }).toContain(expected);
      }
      await expectStickyReviewHeaderClearance(review, "mouse");

      await expect(
        review.locator('[data-testid="review-file-row"][data-file-path="vendor/outer"]'),
      ).toHaveCount(0);
      await expect(
        review.locator(
          '[data-testid="review-file-row"][data-file-path="vendor/inner"][data-repository-name="vendor/outer"]',
        ),
      ).toHaveCount(0);

      await review.getByRole("button", { name: "Close review" }).click();
      await expect(review).not.toBeVisible();

      // Stage one root file to expose the global staged Commit action. The
      // commit dialog's "Stage all" option then reruns staging in dependency
      // order immediately before committing each scope.
      const unstaged = session.changes.getByTestId("unstaged-files-section");
      const stageAllButton = unstaged.getByRole("button", { name: "Stage all", exact: true });
      await expect(stageAllButton).toBeVisible({ timeout: 20_000 });
      await expect(stageAllButton).toBeEnabled({ timeout: 20_000 });
      await stageAllButton.click();

      const staged = session.changes.getByTestId("staged-files-section");
      const commitButton = staged.getByRole("button", { name: "Commit", exact: true });
      await expect(commitButton).toBeVisible({ timeout: 20_000 });
      await expect(commitButton).toBeEnabled({ timeout: 20_000 });
      await commitButton.click();

      const commitDialog = testPage.getByRole("dialog");
      await expect(commitDialog.getByTestId("commit-title-input")).toBeVisible();
      await commitDialog.getByTestId("commit-title-input").fill("e2e nested submodule commit");
      await commitDialog.getByRole("checkbox").check();
      await commitDialog.getByRole("button", { name: "Commit", exact: true }).click();
      await expect(commitDialog).not.toBeVisible({ timeout: 45_000 });

      const outerPath = path.join(worktreePath, "vendor/outer");
      const innerPath = path.join(outerPath, "vendor/inner");
      const expectedOuterSha = readGitValue(outerPath, ["rev-parse", "HEAD"], backend.tmpDir);
      const expectedInnerSha = readGitValue(innerPath, ["rev-parse", "HEAD"], backend.tmpDir);
      // The dialog closes as soon as the request starts. Wait for the deepest
      // commits and their parent gitlinks to settle before asserting the
      // filesystem state, otherwise a successful dependency-wave commit can
      // look flaky while the root request is still in flight.
      await expect
        .poll(
          () => readGitValue(worktreePath, ["rev-parse", "HEAD:vendor/outer"], backend.tmpDir),
          { timeout: 45_000 },
        )
        .toBe(expectedOuterSha);
      await expect
        .poll(() => readGitValue(outerPath, ["rev-parse", "HEAD:vendor/inner"], backend.tmpDir), {
          timeout: 45_000,
        })
        .toBe(expectedInnerSha);
      await expect
        .poll(() => readGitValue(worktreePath, ["status", "--porcelain"], backend.tmpDir), {
          timeout: 45_000,
        })
        .toBe("");
      await expect
        .poll(() => readGitValue(outerPath, ["status", "--porcelain"], backend.tmpDir), {
          timeout: 45_000,
        })
        .toBe("");
      await expect
        .poll(() => readGitValue(innerPath, ["status", "--porcelain"], backend.tmpDir), {
          timeout: 45_000,
        })
        .toBe("");
      expect(readGitValue(worktreePath, ["rev-parse", "HEAD"], backend.tmpDir)).not.toBe(
        parentBaseSha,
      );
    } finally {
      fixture.cleanup();
    }
  });
});

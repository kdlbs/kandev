import { execFileSync } from "node:child_process";
import { test } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { assertJiraWatcherQualifiedBranchPersists } from "./jira-watcher-branch-flow";

test.describe("Jira watcher branches on mobile", () => {
  test("qualified remote branch persists through touch dialog flow", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const branchNames = Array.from({ length: 28 }, (_, index) => `mobile/branch-${index + 1}`);
    const createdBranchNames: string[] = [];
    try {
      for (const branchName of branchNames) {
        execFileSync("git", ["-C", seedData.repositoryPath, "branch", branchName], {
          env: makeGitEnv(backend.tmpDir),
          stdio: "ignore",
        });
        createdBranchNames.push(branchName);
      }
      await assertJiraWatcherQualifiedBranchPersists({
        page: testPage,
        apiClient,
        seedData,
        inputMode: "touch",
        branchNames,
        prCapture,
      });
    } finally {
      for (const branchName of createdBranchNames) {
        execFileSync("git", ["-C", seedData.repositoryPath, "branch", "-D", branchName], {
          env: makeGitEnv(backend.tmpDir),
          stdio: "ignore",
        });
      }
    }
  });
});

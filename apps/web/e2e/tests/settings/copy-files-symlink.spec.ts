import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { configureSymlinkAndCreateTask } from "./copy-files-symlink-helpers";

useRegularMode();

test("configures symlink mode and creates a task", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(60_000);
  const sourcePath = path.join(backend.tmpDir, "repos", "e2e-repo", ".env");
  fs.writeFileSync(sourcePath, "COPY_FILES_E2E=1\n");

  try {
    const taskId = await configureSymlinkAndCreateTask({
      page: testPage,
      apiClient,
      seedData,
      title: "Copy Files Symlink Flow",
    });
    await expect
      .poll(
        async () => {
          const environment = await apiClient.getTaskEnvironment(taskId);
          const worktreePath = environment?.worktree_path;
          if (!worktreePath) return false;
          const linkPath = path.join(worktreePath, ".env");
          if (!fs.existsSync(linkPath)) return false;
          const target = fs.readlinkSync(linkPath);
          return (
            !path.isAbsolute(target) && path.resolve(path.dirname(linkPath), target) === sourcePath
          );
        },
        { timeout: 30_000 },
      )
      .toBe(true);
  } finally {
    fs.rmSync(sourcePath, { force: true });
    await apiClient.updateRepository(seedData.repositoryId, { copy_files: "" }).catch(() => {});
  }
});

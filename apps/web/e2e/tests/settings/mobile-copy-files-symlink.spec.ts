import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { configureSymlinkAndCreateTask } from "./copy-files-symlink-helpers";

useRegularMode();

test("configures symlink mode and creates a task on mobile", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(60_000);
  const sourcePath = path.join(backend.tmpDir, "repos", "e2e-repo", ".env");
  fs.writeFileSync(sourcePath, "COPY_FILES_MOBILE_E2E=1\n");

  try {
    const taskId = await configureSymlinkAndCreateTask({
      page: testPage,
      apiClient,
      seedData,
      title: "Mobile Copy Files Symlink Flow",
    });
    await expect
      .poll(async () => (await apiClient.getTask(taskId)).title)
      .toBe("Mobile Copy Files Symlink Flow");
  } finally {
    fs.rmSync(sourcePath, { force: true });
    await apiClient.updateRepository(seedData.repositoryId, { copy_files: "" }).catch(() => {});
  }
});

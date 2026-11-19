import { expect, test } from "../../fixtures/test-base";
import {
  createDockerStorageProfile,
  deleteDockerStorageProfile,
  type DockerExecutorType,
} from "./docker-storage-limits-helpers";

test.describe("Docker profile storage limits", () => {
  test("saves, reloads, and clears limits on both Docker profile types", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    for (const type of ["local_docker", "remote_docker"] as DockerExecutorType[]) {
      const { executor, profile } = await createDockerStorageProfile(apiClient, type, "desktop");
      try {
        await testPage.goto(`/settings/executors/${profile.id}`);
        const tmpSize = testPage.locator("#docker-tmp-size");
        const logMaxSize = testPage.locator("#docker-log-max-size");
        const logMaxFiles = testPage.locator("#docker-log-max-files");
        await expect(tmpSize).toBeVisible();
        await expect(tmpSize).toHaveValue("");
        await tmpSize.fill("2g");
        await logMaxSize.fill("100m");
        await logMaxFiles.fill("3");

        const save = testPage
          .getByTestId("settings-floating-save")
          .getByRole("button", { name: "Save changes" });
        await expect(save).toBeEnabled();
        await save.click();
        await expect
          .poll(async () => (await apiClient.getExecutorProfile(executor.id, profile.id)).config)
          .toMatchObject({
            docker_tmp_size: "2g",
            docker_log_max_size: "100m",
            docker_log_max_files: "3",
            custom_storage_test_key: "keep-me",
          });

        await testPage.reload();
        await expect(tmpSize).toHaveValue("2g");
        await expect(logMaxSize).toHaveValue("100m");
        await expect(logMaxFiles).toHaveValue("3");

        await tmpSize.fill("");
        await testPage
          .getByTestId("settings-floating-save")
          .getByRole("button", { name: "Save changes" })
          .click();
        await expect
          .poll(async () => {
            const config = (await apiClient.getExecutorProfile(executor.id, profile.id)).config;
            return {
              tmpSize: config?.docker_tmp_size,
              logMaxSize: config?.docker_log_max_size,
              logMaxFiles: config?.docker_log_max_files,
              customStorageKey: config?.custom_storage_test_key,
            };
          })
          .toEqual({
            tmpSize: undefined,
            logMaxSize: "100m",
            logMaxFiles: "3",
            customStorageKey: "keep-me",
          });

        await testPage.reload();
        await expect(tmpSize).toHaveValue("");
        await expect(logMaxSize).toHaveValue("100m");
        await expect(logMaxFiles).toHaveValue("3");
      } finally {
        await deleteDockerStorageProfile(apiClient, executor.id, profile.id);
      }
    }
  });
});

import { expect, test } from "../../fixtures/test-base";
import {
  createDockerStorageProfile,
  deleteDockerStorageProfile,
  expectNoHorizontalOverflow,
  expectTouchTarget,
  type DockerExecutorType,
} from "./docker-storage-limits-helpers";

test.describe("Docker profile storage limits on mobile", () => {
  test("resets and saves limits by touch on both Docker profile types", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    for (const type of ["local_docker", "remote_docker"] as DockerExecutorType[]) {
      const { executor, profile } = await createDockerStorageProfile(apiClient, type, "mobile", {
        docker_tmp_size: "2g",
      });
      try {
        await testPage.goto(`/settings/executors/${profile.id}`);
        const tmpSize = testPage.locator("#docker-tmp-size");
        const logMaxSize = testPage.locator("#docker-log-max-size");
        await expect(tmpSize).toHaveValue("2g");
        await tmpSize.fill("4g");

        const floatingSave = testPage.getByTestId("settings-floating-save");
        const reset = floatingSave.getByRole("button", { name: "Reset" });
        await expectTouchTarget(reset);
        await expectNoHorizontalOverflow(testPage);
        await reset.tap();
        await expect(tmpSize).toHaveValue("2g");

        await logMaxSize.fill("150m");
        const save = floatingSave.getByRole("button", { name: "Save changes" });
        await expectTouchTarget(save);
        await expectNoHorizontalOverflow(testPage);
        await save.tap();
        await expect
          .poll(async () => (await apiClient.getExecutorProfile(executor.id, profile.id)).config)
          .toMatchObject({
            docker_tmp_size: "2g",
            docker_log_max_size: "150m",
            custom_storage_test_key: "keep-me",
          });

        await testPage.reload();
        await expect(tmpSize).toHaveValue("2g");
        await expect(logMaxSize).toHaveValue("150m");
        await expectNoHorizontalOverflow(testPage);
      } finally {
        await deleteDockerStorageProfile(apiClient, executor.id, profile.id);
      }
    }
  });
});

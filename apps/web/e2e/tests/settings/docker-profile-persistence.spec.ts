import { test, expect } from "../../fixtures/test-base";

/**
 * Regression test for the Docker executor profile UI persisting Dockerfile
 * content and image tag. The original bug was that DockerSections owned the
 * `dockerfile` and `image_tag` state via its own useState (instead of lifting
 * to useProfileFormState), so saving the profile never wrote them back to
 * `profile.config`.
 *
 * The "Use defaults" button is the most deterministic exercise of the form
 * because it fills both fields with known values via the same callbacks the
 * editor would use — bypassing CodeMirror, which is awkward to drive.
 */
test.describe("Docker executor profile persistence", () => {
  test("Dockerfile content and image tag round-trip through Save", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    // Create a fresh local_docker executor + profile via the API so we land
    // on a known-empty edit page without polluting workspace defaults.
    const exec = await apiClient.createExecutor("e2e-docker-persistence", "local_docker");
    const profile = await apiClient.createExecutorProfile(exec.id, "default");

    try {
      // Sanity check: profile starts with no docker config persisted.
      const before = await apiClient.getExecutorProfile(exec.id, profile.id);
      expect(before.config?.dockerfile ?? "").toBe("");
      expect(before.config?.image_tag ?? "").toBe("");

      await testPage.goto(`/settings/executors/${profile.id}`);
      // Wait for the page to render — the Image Tag input is a stable anchor.
      await expect(testPage.locator("#image-tag")).toBeVisible({ timeout: 10_000 });

      // Click "Use defaults" → fills both Image Tag input and Dockerfile editor.
      // Visible only when at least one of those fields is empty (which is true
      // for a fresh profile).
      await testPage.getByRole("button", { name: "Use defaults" }).click();

      // Image tag should now be populated; verify via the input value.
      const imageTagInput = testPage.locator("#image-tag");
      await expect(imageTagInput).not.toHaveValue("", { timeout: 5_000 });
      const populatedTag = await imageTagInput.inputValue();
      expect(populatedTag).toMatch(/.+:.+/); // shape "name:tag"

      // Save the profile.
      await testPage.getByRole("button", { name: "Save Changes" }).click();

      // Wait for Save to complete: button text reverts from "Saving...".
      await expect(testPage.getByRole("button", { name: "Save Changes" })).toBeVisible({
        timeout: 10_000,
      });

      // Verify the values landed on profile.config.
      await expect
        .poll(
          async () => {
            const after = await apiClient.getExecutorProfile(exec.id, profile.id);
            return {
              dockerfile: (after.config?.dockerfile ?? "").trim(),
              imageTag: after.config?.image_tag ?? "",
            };
          },
          {
            timeout: 10_000,
            message: "Dockerfile + image_tag should persist on profile.config after Save",
          },
        )
        .toEqual(expect.objectContaining({ imageTag: populatedTag }));

      const after = await apiClient.getExecutorProfile(exec.id, profile.id);
      expect((after.config?.dockerfile ?? "").length).toBeGreaterThan(0);
      expect(after.config?.image_tag).toBe(populatedTag);

      // Reload and confirm the editor renders with the saved value.
      await testPage.reload();
      await expect(testPage.locator("#image-tag")).toHaveValue(populatedTag, { timeout: 10_000 });
    } finally {
      await apiClient.deleteExecutor(exec.id).catch(() => {});
    }
  });
});

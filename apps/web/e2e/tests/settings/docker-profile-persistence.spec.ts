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
  test("new Docker profile defaults the name and requires a successful image build", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    let createdProfileId: string | null = null;

    await testPage.route("**/api/v1/docker/build", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/x-ndjson",
        body: JSON.stringify({ stream: "Successfully built\n" }) + "\n",
      });
    });

    try {
      await testPage.goto("/settings/executors/new/local_docker");

      await expect(testPage.locator("#profile-name")).toHaveValue("Docker", {
        timeout: 10_000,
      });

      await testPage.getByRole("button", { name: "Use defaults" }).click();
      const createButton = testPage.getByRole("button", { name: "Create Profile" });
      await expect(createButton).toBeDisabled();

      await testPage.getByTestId("create-profile-disabled-tooltip").hover();
      await expect(
        testPage.getByText("Build this Docker image before creating the profile."),
      ).toBeVisible();

      await testPage.getByRole("button", { name: "Build Image" }).click();
      await expect(testPage.getByText("Success", { exact: true })).toBeVisible({
        timeout: 10_000,
      });

      await expect(createButton).toBeEnabled();
      await createButton.click();

      await expect(testPage).toHaveURL(/\/settings\/executors\/[^/]+$/);
      createdProfileId = testPage.url().split("/").pop() ?? null;
      expect(createdProfileId).toBeTruthy();
    } finally {
      if (createdProfileId) {
        await apiClient.deleteExecutorProfile(createdProfileId).catch(() => {});
      }
    }
  });

  test("profile page lists related Docker containers and can remove them during profile deletion", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    const exec = await apiClient.createExecutor("e2e-docker-containers", "local_docker");
    const profile = await apiClient.createExecutorProfile(exec.id, {
      name: "containers",
      config: {
        image_tag: "kandev/e2e:test",
        dockerfile: "FROM busybox\nWORKDIR /workspace\n",
      },
    });
    const containers = [
      {
        id: "container-1",
        name: "kandev-agent-test",
        image: "kandev/e2e:test",
        state: "running",
        status: "Up 5 seconds",
        labels: {
          "kandev.executor_profile_id": profile.id,
          "kandev.task_id": "task-abc123",
          "kandev.task_title": "Readable Task Title",
          "kandev.task_environment_id": "env-123",
        },
      },
    ];
    let visibleContainers = [...containers];
    const removed: string[] = [];

    await testPage.route("**/api/v1/docker/containers?*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ containers: visibleContainers }),
      });
    });
    await testPage.route("**/api/v1/docker/containers/container-1", async (route) => {
      if (route.request().method() !== "DELETE") {
        await route.fallback();
        return;
      }
      removed.push("container-1");
      visibleContainers = [];
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true }),
      });
    });

    try {
      await testPage.goto(`/settings/executors/${profile.id}`);

      const row = testPage.getByTestId("docker-container-row-container-1");
      await expect(row).toBeVisible({ timeout: 10_000 });
      await expect(row.getByTestId("docker-container-task")).toContainText("Readable Task Title");
      await expect(row.getByText("Up 5 seconds")).toBeVisible();

      await testPage.getByRole("button", { name: "Delete Profile" }).click();
      await expect(
        testPage.getByText("1 related Docker container will also be removed."),
      ).toBeVisible();
      await expect(
        testPage.getByRole("checkbox", { name: "Remove related Docker containers" }),
      ).toBeChecked();
      await testPage.getByRole("button", { name: "Delete" }).click();

      await expect.poll(() => removed).toContain("container-1");
    } finally {
      await apiClient.deleteExecutor(exec.id).catch(() => {});
    }
  });

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
      await expect(testPage.getByText("Profile saved")).toBeVisible({ timeout: 10_000 });

      // Wait for Save to complete: the stable status button returns to its ready label.
      await expect(testPage.getByRole("button", { name: /Save Changes|Saved/ })).toBeVisible({
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

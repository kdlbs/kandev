import { execFileSync } from "node:child_process";
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/docker-test-base";
import { E2E_IMAGE_TAG, removeKandevContainers } from "../../fixtures/docker-probe";
import { dockerInspectExists, dockerRemove } from "../../helpers/docker";

function createStoppedContainer(labels: string[]): string {
  const args = ["create"];
  for (const label of labels) args.push("--label", label);
  args.push(E2E_IMAGE_TAG, "sh", "-c", "printf managed-data > /managed-storage-fixture");
  const id = execFileSync("docker", args, { encoding: "utf8" }).trim();
  execFileSync("docker", ["start", "-a", id]);
  return id;
}

async function openStorageSettings(page: Page): Promise<void> {
  const storagePage = page.getByTestId("storage-settings-page");
  let lastError: unknown;

  // Wait only for the document commit. The Go-served SPA can keep
  // DOMContentLoaded pending while a dynamic Settings chunk is resolving, so
  // let the test-id assertion own application readiness and retry the full
  // document request once if the first load is interrupted under CI load.
  for (let attempt = 0; attempt < 2; attempt += 1) {
    try {
      await page.goto("/settings/system/storage", {
        waitUntil: "commit",
        timeout: 20_000,
      });
      await expect(storagePage).toBeVisible({ timeout: 60_000 });
      return;
    } catch (error) {
      lastError = error;
    }
  }

  throw lastError;
}

async function refreshStorageOverview(page: Page): Promise<void> {
  const analyze = page.getByTestId("storage-analyze");
  const managedContainers = page.getByTestId("storage-resource-managed-containers-trigger");
  await analyze.click();
  await expect(analyze).toHaveAttribute("data-job-state", "succeeded", {
    timeout: 60_000,
  });
  // The terminal job state arrives before the overview reload completes. Give
  // the hook's bounded refresh retries time to replace a transient unavailable
  // snapshot before asserting the Docker measurement.
  await expect(managedContainers).toContainText("Kandev containers<0.01 GB", {
    timeout: 60_000,
  });
}

test("removes only stopped Kandev-labeled containers and gates daemon-wide cleanup", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(240_000);

  // This test asserts the *global* count of kandev.managed=true containers the
  // daemon reports ("2 managed containers"). The containers project shares one
  // Docker daemon across all specs in the worker and only sweeps managed
  // containers at worker teardown, so a sibling Docker-executor spec (or one of
  // its retries) can leave managed containers behind and inflate the count —
  // the flake seen in CI was "5 managed containers" instead of 2. Sweep any
  // stray managed containers first so this test counts only its own fixtures.
  removeKandevContainers();
  const activeTask = await apiClient.createTask(seedData.workspaceId, "Retain active container", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const managed = createStoppedContainer([
    "kandev.managed=true",
    `kandev.task_id=e2e-storage-missing-${Date.now()}`,
  ]);
  const active = createStoppedContainer(["kandev.managed=true", `kandev.task_id=${activeTask.id}`]);
  const unrelated = createStoppedContainer(["e2e.storage=unrelated"]);
  try {
    await openStorageSettings(testPage);
    // The first overview can race Docker client startup and cache an
    // unavailable result. Analyze after creating the fixtures so this test
    // observes the current daemon state instead of that transient snapshot.
    await refreshStorageOverview(testPage);
    await expect(testPage.getByTestId("storage-docker-build-cache")).toBeDisabled();
    await testPage.getByTestId("storage-resource-managed-containers-trigger").click();
    await expect(testPage.getByTestId("storage-resource-managed-containers")).toContainText(
      "2 managed containers",
    );
    await testPage.getByTestId("storage-run-now").click();
    await expect(testPage.getByTestId("storage-run-now")).toHaveAttribute(
      "data-job-state",
      "succeeded",
    );
    await expect.poll(() => dockerInspectExists(managed)).toBe(false);
    expect(dockerInspectExists(active)).toBe(true);
    expect(dockerInspectExists(unrelated)).toBe(true);

    await testPage.getByTestId("storage-docker-dedicated").click();
    await testPage.getByTestId("storage-docker-confirm-confirmation").fill("DEDICATED");
    await testPage.getByTestId("storage-docker-confirm").click();
    await expect(testPage.getByTestId("storage-docker-build-cache")).toBeEnabled();
  } finally {
    if (dockerInspectExists(managed)) dockerRemove(managed);
    if (dockerInspectExists(active)) dockerRemove(active);
    if (dockerInspectExists(unrelated)) dockerRemove(unrelated);
  }
});

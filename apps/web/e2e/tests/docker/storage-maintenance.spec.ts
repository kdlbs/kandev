import { execFileSync } from "node:child_process";
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/docker-test-base";
import {
  E2E_DOCKER_SCOPE,
  E2E_IMAGE_TAG,
  waitForScopedKandevContainersRemoved,
} from "../../fixtures/docker-probe";
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
      await page.goto("/settings/system/data-storage", {
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

test.describe.serial("process-scoped container cleanup", () => {
  let previousTestContainer = "";

  // Keep this boundary local to the serial regression too. The first test
  // intentionally leaves a stopped container behind, and the next test must
  // begin only after the process-scoped sweep has completed.
  test.afterEach(async () => {
    await waitForScopedKandevContainersRemoved(E2E_DOCKER_SCOPE, 60_000);
  });

  test.afterAll(() => {
    if (previousTestContainer && dockerInspectExists(previousTestContainer)) {
      dockerRemove(previousTestContainer);
    }
  });

  test("allows a test to leave a process-owned container", () => {
    previousTestContainer = createStoppedContainer([
      "kandev.managed=true",
      `kandev.e2e.run=${E2E_DOCKER_SCOPE}`,
      "kandev.task_id=e2e-cleanup-boundary",
    ]);
    expect(dockerInspectExists(previousTestContainer)).toBe(true);
  });

  test("starts the next test without the previous process-owned container", async () => {
    await expect
      .poll(() => dockerInspectExists(previousTestContainer), {
        timeout: 30_000,
        message: "previous process-owned container should be removed before the next test",
      })
      .toBe(false);
  });
});

test("removes only stopped Kandev-labeled containers and gates daemon-wide cleanup", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  // Resources carry a process-unique ownership label. The storage API scopes
  // managed-container usage to this process, so the count excludes another
  // shard's containers while still exercising the exact cleanup contract.
  const scopeLabel = `kandev.e2e.run=${E2E_DOCKER_SCOPE}`;
  test.setTimeout(240_000);
  const activeTask = await apiClient.createTask(seedData.workspaceId, "Retain active container", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const managed = createStoppedContainer([
    "kandev.managed=true",
    scopeLabel,
    `kandev.task_id=e2e-storage-missing-${Date.now()}`,
  ]);
  const active = createStoppedContainer([
    "kandev.managed=true",
    scopeLabel,
    `kandev.task_id=${activeTask.id}`,
  ]);
  const unrelated = createStoppedContainer([scopeLabel, "e2e.storage=unrelated"]);
  try {
    expect(dockerInspectExists(managed)).toBe(true);
    expect(dockerInspectExists(active)).toBe(true);
    expect(dockerInspectExists(unrelated)).toBe(true);
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

import { execFileSync } from "node:child_process";
import { test, expect } from "../../fixtures/docker-test-base";
import { E2E_DOCKER_SCOPE, E2E_IMAGE_TAG } from "../../fixtures/docker-probe";
import { dockerInspectExists, dockerRemove } from "../../helpers/docker";

function createStoppedContainer(labels: string[]): string {
  const args = ["create"];
  for (const label of labels) args.push("--label", label);
  args.push(E2E_IMAGE_TAG, "sh", "-c", "printf managed-data > /managed-storage-fixture");
  const id = execFileSync("docker", args, { encoding: "utf8" }).trim();
  execFileSync("docker", ["start", "-a", id]);
  return id;
}

test.describe.serial("process-scoped container cleanup", () => {
  let previousTestContainer = "";

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

  test("starts the next test without the previous process-owned container", () => {
    expect(dockerInspectExists(previousTestContainer)).toBe(false);
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
    await testPage.goto("/settings/system/storage");
    await expect(testPage.getByTestId("storage-docker-build-cache")).toBeDisabled();
    await expect(testPage.getByTestId("storage-resource-managed-containers-trigger")).toContainText(
      "Kandev containers<0.01 GB",
    );
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

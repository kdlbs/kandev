import { execFileSync } from "node:child_process";
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

test("removes only stopped Kandev-labeled containers and gates daemon-wide cleanup", async ({
  testPage,
  apiClient,
  seedData,
}) => {
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

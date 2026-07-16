import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { seedManagedGoCache } from "../../helpers/storage-maintenance";

function seedOrphanWorkspace(tmpDir: string): { root: string; artifact: string } {
  const root = path.join(tmpDir, ".kandev", "tasks", "e2e-storage-orphan_abc");
  const artifact = path.join(root, "repo", "node_modules", "fixture", "index.js");
  fs.mkdirSync(path.dirname(artifact), { recursive: true });
  fs.writeFileSync(artifact, "orphan-node-modules-fixture");
  fs.writeFileSync(
    path.join(root, ".kandev-workspace.json"),
    JSON.stringify({
      task_id: "e2e-storage-orphan",
      workspace_id: "e2e-orphan-workspace",
      task_dir_name: "e2e-storage-orphan_abc",
      layout_version: 1,
      created_at: "2026-06-01T00:00:00Z",
    }),
  );
  const old = new Date(Date.now() - 8 * 24 * 60 * 60 * 1000);
  fs.utimesSync(root, old, old);
  return { root, artifact };
}

test.describe("System storage maintenance", () => {
  test("cleans a disabled managed Go cache only through its explicit action", async ({
    testPage,
    backend,
  }) => {
    const cache = seedManagedGoCache(backend.tmpDir);
    expect(fs.statSync(cache.artifact).size).toBeGreaterThan(15 * 1024 * 1024 * 1024);
    const overviewResponse = testPage.waitForResponse(
      (response) => new URL(response.url()).pathname === "/api/v1/system/storage",
    );
    await testPage.goto("/settings/system/storage");
    const overview = await (await overviewResponse).json();
    expect(overview.summary.go_cache).toMatchObject({ owned: true });
    expect(overview.summary.go_cache.size_bytes).toBeGreaterThan(15 * 1024 * 1024 * 1024);
    await testPage.getByTestId("storage-resource-go-cache-trigger").click();
    const cleanButton = testPage.getByTestId("storage-go-cache-clean");
    await expect(cleanButton).toBeEnabled();

    const globalRequest = testPage.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname === "/api/v1/system/storage/run",
    );
    await testPage.getByTestId("storage-run-now").click();
    expect((await globalRequest).postDataJSON()).toEqual({});
    await expect(testPage.getByTestId("storage-cleanup-job")).toHaveAttribute(
      "data-state",
      "succeeded",
    );
    expect(fs.existsSync(cache.artifact)).toBe(true);

    const explicitRequest = testPage.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname === "/api/v1/system/storage/run",
    );
    await cleanButton.click();
    expect((await explicitRequest).postDataJSON()).toEqual({ resources: ["go_cache"] });
    await expect.poll(() => fs.existsSync(cache.artifact)).toBe(false);
  });

  test("persists policy and analyzes, quarantines, and restores an orphan workspace", async ({
    testPage,
    backend,
  }) => {
    const orphan = seedOrphanWorkspace(backend.tmpDir);
    await testPage.goto("/settings/system/storage");
    const overviewBox = await testPage.getByTestId("storage-overview-card").boundingBox();
    const policyBox = await testPage.getByTestId("storage-policy-card").boundingBox();
    expect(overviewBox).not.toBeNull();
    expect(policyBox).not.toBeNull();
    expect(policyBox!.y).toBeGreaterThanOrEqual(overviewBox!.y + overviewBox!.height);
    const scheduling = testPage.getByTestId("storage-scheduling-enabled");
    await expect(scheduling).toHaveAttribute("data-state", "unchecked");
    await expect(testPage.getByTestId("storage-check-interval")).toHaveValue("24");

    await scheduling.click();
    await testPage.getByTestId("storage-idle-period").fill("11");
    await testPage.getByTestId("storage-save-settings").click();
    await expect(testPage.getByText("Storage policy saved")).toBeVisible();
    await testPage.reload();
    await expect(scheduling).toHaveAttribute("data-state", "checked");
    await expect(testPage.getByTestId("storage-idle-period")).toHaveValue("11");

    // Stop the newly enabled scheduler before exercising a deterministic manual run.
    await scheduling.click();
    await testPage.getByTestId("storage-save-settings").click();
    await expect(scheduling).toHaveAttribute("data-state", "unchecked");

    await testPage.getByTestId("storage-analyze").click();
    await expect(testPage.getByTestId("storage-analysis-job")).toHaveAttribute(
      "data-state",
      "succeeded",
    );
    await testPage.getByTestId("storage-resource-workspaces-trigger").click();
    await expect(testPage.getByTestId("storage-resource-workspaces-trigger")).toContainText(
      "Task workspaces<0.01 GB",
    );
    expect(fs.existsSync(orphan.artifact)).toBe(true);

    await backend.restart();
    await testPage.reload();
    await expect(testPage.getByTestId("storage-idle-period")).toHaveValue("11");

    await testPage.getByTestId("storage-run-now").click();
    await expect(testPage.getByTestId("storage-cleanup-job")).toHaveAttribute(
      "data-state",
      "succeeded",
    );
    const quarantineCard = testPage.getByTestId("storage-quarantine-card");
    const entry = quarantineCard
      .locator('[data-testid^="storage-quarantine-"]')
      .filter({ hasText: orphan.root })
      .last();
    await expect(entry).toBeVisible();
    expect(fs.existsSync(orphan.root)).toBe(false);

    await entry.getByRole("button", { name: "Restore" }).click();
    await expect.poll(() => fs.existsSync(orphan.artifact)).toBe(true);
    await expect(quarantineCard.getByText(orphan.root)).toHaveCount(0);
  });

  test("shows busy feedback instead of forcing maintenance over an active task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Storage activity gate",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return session_id");
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.find((session) => session.id === task.session_id)?.state ?? "";
        },
        { timeout: 20_000, message: "Waiting for initial task turn to finish" },
      )
      .toBe("WAITING_FOR_INPUT");
    await apiClient.addUserMessage(
      task.id,
      task.session_id,
      'e2e:delay(15000)\ne2e:message("activity finished")',
    );
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.find((session) => session.id === task.session_id)?.state ?? "";
        },
        { timeout: 20_000, message: "Waiting for active task storage gate" },
      )
      .toBe("RUNNING");

    await testPage.goto("/settings/system/storage");
    const responsePromise = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname === "/api/v1/system/storage/run",
    );
    await testPage.getByTestId("storage-run-now").click();
    const response = await responsePromise;
    const responseBody = await response.json();
    expect({ status: response.status(), body: responseBody }).toMatchObject({
      status: 409,
      body: { busy_resources: expect.any(Array) },
    });
    await expect(testPage.getByTestId("storage-error")).toContainText(/busy|active/i);
    await expect(testPage.getByTestId("storage-cleanup-job")).toHaveCount(0);
  });
});

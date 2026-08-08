import fs from "node:fs";
import path from "node:path";
import type { Route } from "@playwright/test";
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
    await expect(testPage.getByTestId("storage-run-now")).toHaveAttribute(
      "data-job-state",
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

  test("reuses the cached snapshot until Analyze refreshes it", async ({ testPage }) => {
    await testPage.goto("/settings/system/storage");
    const analyzedTime = testPage.locator("time[datetime]").filter({ hasText: "Last analyzed" });
    await expect(analyzedTime).toHaveText(/^Last analyzed .+/);
    const initialAnalyzedAt = await analyzedTime.getAttribute("datetime");
    expect(initialAnalyzedAt).toBeTruthy();

    await testPage.reload();
    await expect(analyzedTime).toHaveAttribute("datetime", initialAnalyzedAt!);

    const scheduling = testPage.getByTestId("storage-scheduling-enabled");
    const initialSchedulingState = await scheduling.getAttribute("data-state");
    try {
      await scheduling.click();
      await testPage.getByRole("button", { name: "Save changes" }).click();
      await expect(testPage.getByText("Storage policy saved")).toBeVisible();
      await expect(analyzedTime).toHaveAttribute("datetime", initialAnalyzedAt!);

      await testPage.getByTestId("storage-analyze").click();
      await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
        "data-job-state",
        "succeeded",
      );
      await expect.poll(() => analyzedTime.getAttribute("datetime")).not.toBe(initialAnalyzedAt);
    } finally {
      if ((await scheduling.getAttribute("data-state")) !== initialSchedulingState) {
        await scheduling.click();
        await testPage.getByRole("button", { name: "Save changes" }).click();
        await expect(testPage.getByText("Storage policy saved")).toBeVisible();
      }
    }
  });

  test("shows fast storage sections while the overview scan is pending", async ({
    testPage,
    prCapture,
  }) => {
    let overviewRequestStarted = false;
    let releaseOverview: () => void = () => {};
    let markOverviewObserved: () => void = () => {};
    let markOverviewSettled: () => void = () => {};
    const overviewGate = new Promise<void>((resolve) => {
      releaseOverview = resolve;
    });
    const overviewObserved = new Promise<void>((resolve) => {
      markOverviewObserved = resolve;
    });
    const overviewSettled = new Promise<void>((resolve) => {
      markOverviewSettled = resolve;
    });
    const overviewPattern = "**/api/v1/system/storage";
    const holdOverview = async (route: Route) => {
      const request = route.request();
      const pathname = new URL(request.url()).pathname;
      if (request.method() !== "GET" || pathname !== "/api/v1/system/storage") {
        await route.continue();
        return;
      }
      overviewRequestStarted = true;
      markOverviewObserved();
      await overviewGate;
      try {
        await route.continue();
      } finally {
        markOverviewSettled();
      }
    };

    await testPage.route(overviewPattern, holdOverview);
    try {
      await testPage.goto("/settings/system/storage");
      await overviewObserved;

      await expect(testPage.getByTestId("storage-policy-card")).toBeVisible();
      await expect(testPage.getByTestId("storage-run-history")).toBeVisible();
      await expect(testPage.getByTestId("storage-quarantine-card")).toBeVisible();
      await expect(testPage.getByTestId("storage-disk-capacity-card")).toBeVisible();
      await expect(testPage.getByRole("progressbar")).toBeVisible();
      await expect(testPage.getByTestId("storage-dependency-allowlist")).toContainText(
        "node_modules",
      );
      await expect(testPage.getByTestId("storage-overview-spinner")).toBeVisible();
      await expect(testPage.getByTestId("storage-analysis-total")).toHaveCount(0);
      await expect(testPage.getByTestId("toast-message")).toHaveCount(0);
      await prCapture.screenshot("progressive-loading", {
        caption: "Desktop storage shows policy, history, and quarantine while analysis scans",
        fullPage: true,
      });

      releaseOverview();
      await expect(testPage.getByTestId("storage-analysis-total")).toBeVisible();
      await expect(testPage.getByTestId("storage-quarantine-total")).toBeVisible();
      await expect(testPage.getByTestId("storage-overview-spinner")).toHaveCount(0);
    } finally {
      releaseOverview();
      if (overviewRequestStarted) await overviewSettled;
      await testPage.unroute(overviewPattern, holdOverview);
    }
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
    await expect(testPage.getByTestId("storage-check-interval")).toBeDisabled();
    await expect(testPage.getByTestId("storage-idle-period")).toBeDisabled();

    await scheduling.click();
    await expect(testPage.getByTestId("storage-check-interval")).toBeEnabled();
    await expect(testPage.getByTestId("storage-idle-period")).toBeEnabled();
    await testPage.getByTestId("storage-idle-period").fill("11");
    await expect(testPage.getByTestId("storage-idle-period")).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );
    await expect(testPage.getByTestId("storage-policy-section-schedule")).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );
    await testPage.getByRole("button", { name: "Save changes" }).click();
    await expect(testPage.getByText("Storage policy saved")).toBeVisible();
    await testPage.reload();
    await expect(scheduling).toHaveAttribute("data-state", "checked");
    await expect(testPage.getByTestId("storage-idle-period")).toHaveValue("11");

    // Stop the newly enabled scheduler before exercising a deterministic manual run.
    await scheduling.click();
    await testPage.getByRole("button", { name: "Save changes" }).click();
    await expect(scheduling).toHaveAttribute("data-state", "unchecked");

    await testPage.getByTestId("storage-analyze").click();
    await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
      "data-job-state",
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
    await expect(testPage.getByTestId("storage-run-now")).toHaveAttribute(
      "data-job-state",
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
    prCapture,
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
    expect(responseBody.force_available).toBe(true);
    expect(responseBody.busy_resources[0]).toMatchObject({
      kind: expect.any(String),
      label: expect.any(String),
    });
    await expect(testPage.getByTestId("storage-busy")).toContainText(
      responseBody.busy_resources[0].label,
    );
    await expect(testPage.getByTestId("storage-busy")).toContainText(/may disrupt/i);
    await prCapture.screenshot("busy-feedback", {
      caption: "Desktop storage cleanup explains the active work and offers Run anyway",
      fullPage: true,
    });

    const forceRequest = testPage.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname === "/api/v1/system/storage/run",
    );
    await testPage.getByTestId("storage-run-anyway").click();
    expect((await forceRequest).postDataJSON()).toEqual({ force: true });
    await expect(testPage.getByTestId("storage-run-now")).toHaveAttribute(
      "data-job-state",
      "succeeded",
      { timeout: 20_000 },
    );
  });

  test("shows quarantine deadlines and clears only eligible entries", async ({
    testPage,
    prCapture,
  }) => {
    const entries = [
      {
        id: "eligible-entry",
        resource_type: "task_workspace",
        original_path: "/tmp/eligible",
        quarantine_path: "/tmp/trash/eligible",
        size_bytes: 1024,
        state: "quarantined",
        quarantined_at: "2026-07-20T00:00:00Z",
        delete_after: new Date(Date.now() - 60_000).toISOString(),
        last_error: "",
        metadata: {},
      },
      {
        id: "protected-entry",
        resource_type: "task_workspace",
        original_path: "/tmp/protected",
        quarantine_path: "/tmp/trash/protected",
        size_bytes: 2048,
        state: "quarantined",
        quarantined_at: "2026-07-29T00:00:00Z",
        delete_after: new Date(Date.now() + 86_400_000).toISOString(),
        last_error: "",
        metadata: {},
      },
    ];
    await testPage.route("**/api/v1/system/storage/quarantine", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ entries }),
        });
        return;
      }
      expect(route.request().method()).toBe("DELETE");
      expect(route.request().postDataJSON()).toEqual({
        scope: "eligible",
        confirm: "DELETE ELIGIBLE",
      });
      await route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ job_id: "eligible-purge" }),
      });
    });
    await testPage.goto("/settings/system/storage");
    await expect(
      testPage.getByTestId("storage-quarantine-eligible-entry").getByText("Eligible now"),
    ).toBeVisible();
    await expect(testPage.getByText(/Protected until/)).toBeVisible();
    await expect(testPage.getByTestId("storage-quarantine-protected-entry-delete")).toBeDisabled();
    await testPage.getByTestId("storage-quarantine-card").scrollIntoViewIfNeeded();
    await prCapture.screenshot("quarantine-deadlines", {
      caption: "Desktop quarantine shows eligibility and protected retention deadlines",
    });
    await testPage.getByTestId("storage-quarantine-clear-eligible").click();
    await testPage
      .getByTestId("storage-quarantine-clear-eligible-confirm-confirmation")
      .fill("DELETE ELIGIBLE");
    await testPage.getByTestId("storage-quarantine-clear-eligible-confirm").click();
    await expect(testPage.getByText("Eligible quarantine cleanup started")).toBeVisible();
    await prCapture.screenshot("quarantine-clear-eligible", {
      caption: "Desktop typed confirmation starts eligible-only quarantine cleanup",
    });
  });
});

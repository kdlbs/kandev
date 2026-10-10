import { test, expect } from "../../fixtures/test-base";
import { dwell, waitForHttp } from "../../helpers/causal-waits";

const listPath = /\/api\/v1\/workspaces\/[^/]+\/tasks$/;

// @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.13
// @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.14
test("latest task search wins and clear cancels queued input", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  for (const title of ["Debounce Alpha", "Debounce Beta"]) {
    await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
  }
  await testPage.goto("/tasks?group=none");
  const search = testPage.getByTestId("kanban-header-search");
  const input = search.getByRole("textbox");
  const rows = testPage.getByTestId("tasks-list-row-title");
  const beta = waitForHttp(testPage, "GET", listPath, {
    predicate: (r) => new URL(r.url()).searchParams.get("query") === "Debounce Beta",
  });
  await input.fill("Debounce Alpha");
  await input.fill("Debounce Beta");
  await beta;
  await expect(rows).toHaveText(["Debounce Beta"]);
  const empty = waitForHttp(testPage, "GET", listPath, {
    predicate: (r) => !new URL(r.url()).searchParams.has("query"),
  });
  await search.getByRole("button").click();
  await empty;
  await expect(
    testPage.getByTestId("tasks-list").getByText("Debounce Alpha", { exact: true }),
  ).toBeVisible();

  // Freeze only the browser timer during this queued-input cancellation control.
  await testPage.clock.install();
  await testPage.clock.pauseAt(new Date());
  const queries: string[] = [];
  testPage.on("request", (request) => {
    const url = new URL(request.url());
    if (listPath.test(url.pathname)) queries.push(url.searchParams.get("query") ?? "");
  });
  await input.fill("Cancelled input");
  await search.getByRole("button").click();
  await testPage.clock.runFor(1000);
  expect(queries).not.toContain("Cancelled input");
  await expect(input).toHaveValue("");
});

for (const outcome of ["success", "error"] as const) {
  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.15
  test(`older search ${outcome} cannot replace newer results`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    for (const title of ["Reply Alpha", "Reply Beta"]) {
      await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      });
    }
    let release!: () => void;
    let admitted!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const started = new Promise<void>((resolve) => {
      admitted = resolve;
    });
    await testPage.route(/\/api\/v1\/workspaces\/[^/]+\/tasks\?/, async (route) => {
      if (new URL(route.request().url()).searchParams.get("query") !== "Reply Alpha") {
        await route.continue();
        return;
      }
      const response = await route.fetch();
      admitted();
      await held;
      if (outcome === "success") await route.fulfill({ response });
      else
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "obsolete search failure" }),
        });
    });
    try {
      await testPage.goto("/tasks?group=none");
      const input = testPage.getByTestId("kanban-header-search").getByRole("textbox");
      await input.fill("Reply Alpha");
      await started;
      const beta = waitForHttp(testPage, "GET", listPath, {
        predicate: (r) => new URL(r.url()).searchParams.get("query") === "Reply Beta",
      });
      await input.fill("Reply Beta");
      await beta;
      await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText(["Reply Beta"]);
      const alpha = waitForHttp(testPage, "GET", listPath, {
        predicate: (r) => new URL(r.url()).searchParams.get("query") === "Reply Alpha",
      });
      release();
      await (await alpha).finished();
      await dwell(
        testPage,
        100,
        "negative-assertion",
        "Observe that the obsolete reply cannot publish rows or an error after the newer reply.",
      );
      await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText(["Reply Beta"]);
      await expect(testPage.getByTestId("tasks-pagination")).toContainText("1 to 1 of 1");
      await expect(testPage.getByText("obsolete search failure", { exact: true })).toHaveCount(0);
      await expect(testPage.getByText("Failed to load tasks", { exact: true })).toHaveCount(0);
    } finally {
      release();
      await testPage.unrouteAll({ behavior: "wait" });
    }
  });
}

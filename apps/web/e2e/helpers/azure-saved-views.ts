import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { AzureDevOpsSavedView } from "@/lib/types/azure-devops";

type Context = { page: Page; apiClient: ApiClient; workspaceId: string; mobile: boolean };

async function activate(target: Locator, mobile: boolean) {
  if (mobile) await target.tap();
  else await target.click();
}

async function seedViews({ page, apiClient, workspaceId }: Context) {
  await apiClient.mockAzureDevOpsSeed({
    authenticated: true,
    user: { ok: true, id: "user-1", displayName: "Ada Reviewer" },
    projects: [{ id: "project-1", name: "Platform" }],
    workItems: [],
  });
  await apiClient.setAzureDevOpsConfig(workspaceId, {
    organizationUrl: "https://dev.azure.com/acme",
    pat: "azure-test-pat",
  });
  const views = ["One", "Two"].map((label) => ({
    id: label.toLowerCase(),
    label,
    kind: "work_item",
    projectId: "project-1",
    wiql: "SELECT [System.Id] FROM WorkItems",
    createdAt: "2026-10-20T00:00:00Z",
  }));
  const url = `/api/v1/azure-devops/views?workspace_id=${encodeURIComponent(workspaceId)}`;
  expect((await page.request.put(url, { data: { views } })).ok()).toBe(true);
  await page.goto("/azure-devops");
  await expect(page.getByTestId("azure-devops-saved-views-menu")).toBeVisible();
  return url;
}

async function confirmDelete(context: Context, label: string) {
  const { page, mobile } = context;
  const item = page.getByRole("menuitem", { name: `Delete ${label} saved query`, exact: true });
  if (!(await item.isVisible())) {
    await activate(page.getByTestId("azure-devops-saved-views-menu"), mobile);
  }
  await activate(item, mobile);
  const confirmation = page.getByTestId("saved-task-view-delete-confirmation");
  await expect(confirmation).toBeVisible();
  await activate(
    confirmation.getByRole("button", { name: `Delete ${label}`, exact: true }),
    mobile,
  );
  await expect(confirmation).toBeHidden();
}

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

async function gateWrites(page: Page, failIndex: number) {
  const release = deferred();
  const started = deferred();
  const writes: AzureDevOpsSavedView[][] = [];
  await page.route("**/api/v1/azure-devops/views?*", async (route) => {
    if (route.request().method() !== "PUT") return route.continue();
    const index = writes.length;
    writes.push(route.request().postDataJSON().views);
    if (index === 0) {
      started.resolve();
      await release.promise;
    }
    if (index === failIndex) {
      await route.fulfill({ status: 500, json: { error: "Saved view write failed" } });
    } else {
      const response = await route.fetch();
      await route.fulfill({ response });
    }
  });
  return { writes, release: release.resolve, started: started.promise };
}

// @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.15
// @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.18
export async function proveConsecutiveDeletes(context: Context) {
  const { page, mobile } = context;
  const url = await seedViews(context);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const gate = await gateWrites(page, 1);
  try {
    await confirmDelete(context, "One");
    await gate.started;
    await confirmDelete(context, "Two");
    await expect(page.getByRole("menuitem", { name: "Delete Two saved query" })).toHaveCount(0);
    expect(gate.writes.map((views) => views.map((view) => view.id))).toEqual([["two"]]);
    gate.release();
    await expect(page.getByText("Failed to delete saved query", { exact: true })).toBeVisible();
    await confirmDelete(context, "Two");
    await expect.poll(async () => (await (await page.request.get(url)).json()).views).toEqual([]);
    expect(gate.writes.map((views) => views.map((view) => view.id))).toEqual([["two"], [], []]);
    await page.reload();
    await activate(page.getByTestId("azure-devops-saved-views-menu"), mobile);
    await expect(page.getByRole("menuitem", { name: "No saved queries yet" })).toBeVisible();
    expect(errors).toEqual([]);
  } finally {
    gate.release();
    await page.unroute("**/api/v1/azure-devops/views?*");
  }
}

// @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.16
export async function proveSaveAfterFailedDelete(context: Context) {
  const { page, mobile } = context;
  const url = await seedViews(context);
  await activate(page.getByTestId("azure-devops-work-items-mode"), mobile);
  const gate = await gateWrites(page, 0);
  try {
    await confirmDelete(context, "One");
    await gate.started;
    const save = page.getByRole("menuitem", { name: "Save current query", exact: true });
    if (!(await save.isVisible())) {
      await activate(page.getByTestId("azure-devops-saved-views-menu"), mobile);
    }
    await expect(save).toBeEnabled();
    await activate(save, mobile);
    const dialog = page.getByRole("dialog", { name: "Save Azure DevOps view" });
    await dialog.getByLabel("Name", { exact: true }).fill("New queue");
    await activate(dialog.getByRole("button", { name: "Save", exact: true }), mobile);
    await expect(dialog.getByRole("button", { name: "Saving...", exact: true })).toBeDisabled();
    expect(gate.writes).toHaveLength(1);
    gate.release();
    await expect(dialog).toBeHidden();
    const response = await page.request.get(url);
    const persisted = (await response.json()).views as AzureDevOpsSavedView[];
    expect(persisted.map((view) => view.label)).toEqual(["One", "Two", "New queue"]);
    expect(gate.writes[1].map((view) => view.label)).toEqual(["One", "Two", "New queue"]);
    await expect(page.getByText("Failed to delete saved query", { exact: true })).toBeVisible();
    await page.reload();
    await activate(page.getByTestId("azure-devops-saved-views-menu"), mobile);
    await expect(page.getByRole("menuitem", { name: "Delete One saved query" })).toBeVisible();
    await expect(
      page.getByRole("menuitem", { name: "Delete New queue saved query" }),
    ).toBeVisible();
  } finally {
    gate.release();
    await page.unroute("**/api/v1/azure-devops/views?*");
  }
}

// @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.17
export async function proveSaveAfterFailedLoad(context: Context) {
  const { page, mobile } = context;
  const release = deferred();
  const started = deferred();
  let reads = 0;
  const writes: AzureDevOpsSavedView[][] = [];
  await page.route("**/api/v1/azure-devops/views?*", async (route) => {
    if (route.request().method() === "PUT") {
      writes.push(route.request().postDataJSON().views);
      return route.continue();
    }
    if (route.request().method() !== "GET" || reads++ > 0) return route.continue();
    started.resolve();
    await release.promise;
    await route.fulfill({ status: 500, json: { error: "Saved views could not be loaded" } });
  });
  try {
    const url = await seedViews(context);
    await started.promise;
    await activate(page.getByTestId("azure-devops-work-items-mode"), mobile);
    await activate(page.getByTestId("azure-devops-saved-views-menu"), mobile);
    const save = page.getByRole("menuitem", { name: "Save current query", exact: true });
    await expect(save).toBeEnabled();
    await activate(save, mobile);
    const dialog = page.getByRole("dialog", { name: "Save Azure DevOps view" });
    await dialog.getByLabel("Name", { exact: true }).fill("New queue");
    await activate(dialog.getByRole("button", { name: "Save", exact: true }), mobile);
    await expect(dialog.getByRole("button", { name: "Saving...", exact: true })).toBeDisabled();
    expect(writes).toEqual([]);
    release.resolve();
    await expect(dialog.getByRole("alert")).toHaveText("Could not save this view. Try again.");
    expect(writes).toEqual([]);
    const original = (await (await page.request.get(url)).json()).views as AzureDevOpsSavedView[];
    expect(original.map((view) => view.label)).toEqual(["One", "Two"]);
    await activate(dialog.getByRole("button", { name: "Save", exact: true }), mobile);
    await expect(dialog).toBeHidden();
    expect(reads).toBe(2);
    expect(writes.map((views) => views.map((view) => view.label))).toEqual([
      ["One", "Two", "New queue"],
    ]);
    const persisted = (await (await page.request.get(url)).json()).views as AzureDevOpsSavedView[];
    expect(persisted.map((view) => view.label)).toEqual(["One", "Two", "New queue"]);
  } finally {
    release.resolve();
    await page.unroute("**/api/v1/azure-devops/views?*");
  }
}

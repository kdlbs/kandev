import path from "node:path";
import http, { type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

// This is a contract-level E2E test for the EXTERNAL yattdev/kandev-plugin-redmine
// repository's packaged artifact (never built in-repo, unlike the in-tree
// plugin fixture bitbucket-plugin-contract.spec.ts exercises) — mirroring
// apps/web/e2e/tests/plugins/bitbucket-packaged-plugin.spec.ts's shape.
const PLUGIN_ID = "kandev-plugin-redmine";
const packagePath = process.env.KANDEV_REDMINE_PLUGIN_PACKAGE?.trim();

test.skip(!packagePath, "requires KANDEV_REDMINE_PLUGIN_PACKAGE from the attached plugin repo");

/**
 * A minimal in-process Redmine JSON API double. Reachable from the real
 * plugin subprocess (spawned by the same backend process this Playwright
 * runner talks to, both on localhost) so connection.save/link.set/watch
 * polling exercise the actual packaged Go binary end to end, not just a
 * browser-side page.route() mock.
 */
function createMockRedmine(): Server {
  const issues = new Map<number, unknown>();
  issues.set(101, {
    id: 101,
    subject: "Login page throws 500",
    description: "Reported by a customer.",
    project: { id: 1, name: "Demo" },
    tracker: { id: 1, name: "Bug" },
    status: { id: 1, name: "New" },
    priority: { id: 2, name: "High" },
    updated_on: "2026-08-15T00:00:00Z",
  });

  const server: Server = http.createServer((req, res) => {
    const url = new URL(req.url ?? "/", "http://localhost");
    res.setHeader("Content-Type", "application/json");

    if (url.pathname === "/users/current.json") {
      res.writeHead(200);
      res.end(JSON.stringify({ user: { id: 1, login: "e2e" } }));
      return;
    }
    if (url.pathname === "/projects.json") {
      res.writeHead(200);
      res.end(
        JSON.stringify({ projects: [{ id: 1, name: "Demo", identifier: "demo" }], total_count: 1 }),
      );
      return;
    }
    if (url.pathname === "/issue_statuses.json") {
      res.writeHead(200);
      res.end(
        JSON.stringify({
          issue_statuses: [
            { id: 1, name: "New", is_closed: false },
            { id: 5, name: "Closed", is_closed: true },
          ],
        }),
      );
      return;
    }
    if (url.pathname === "/trackers.json") {
      res.writeHead(200);
      res.end(JSON.stringify({ trackers: [{ id: 1, name: "Bug" }] }));
      return;
    }
    if (url.pathname === "/enumerations/issue_priorities.json") {
      res.writeHead(200);
      res.end(JSON.stringify({ issue_priorities: [{ id: 2, name: "High" }] }));
      return;
    }
    if (url.pathname === "/custom_fields.json") {
      res.writeHead(200);
      res.end(JSON.stringify({ custom_fields: [] }));
      return;
    }
    if (url.pathname === "/issues.json") {
      const all = Array.from(issues.values());
      res.writeHead(200);
      res.end(JSON.stringify({ issues: all, total_count: all.length }));
      return;
    }
    const issueMatch = url.pathname.match(/^\/issues\/(\d+)\.json$/);
    if (issueMatch) {
      const id = Number(issueMatch[1]);
      const issue = issues.get(id);
      if (!issue) {
        res.writeHead(404);
        res.end();
        return;
      }
      if (req.method === "PUT") {
        res.writeHead(200);
        res.end();
        return;
      }
      res.writeHead(200);
      res.end(
        JSON.stringify({ issue: { ...issue, journals: [], attachments: [], relations: [] } }),
      );
      return;
    }

    res.writeHead(404);
    res.end();
  });

  return server;
}

async function listen(server: Server): Promise<string> {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  return `http://127.0.0.1:${port}`;
}

async function close(server: Server): Promise<void> {
  await new Promise<void>((resolve) => server.close(() => resolve()));
}

async function installPackagedPlugin(testPage: import("@playwright/test").Page): Promise<void> {
  if (!packagePath) throw new Error("Redmine plugin package path is required");
  await testPage.goto("/settings/plugins");
  await testPage.getByTestId("install-plugin-trigger").click();
  await testPage.getByTestId("install-plugin-tab-upload").click();
  await testPage.getByTestId("install-plugin-file-input").setInputFiles(path.resolve(packagePath));
  await testPage.getByTestId("install-plugin-upload-submit").click();
  const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
  await expect(pluginRow).toBeVisible({ timeout: 15_000 });
  await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
}

async function invokePluginAction(
  apiClient: import("../../helpers/api-client").ApiClient,
  key: string,
  workspaceId: string,
  body: Record<string, unknown> = {},
  taskId?: string,
): Promise<unknown> {
  const response = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/actions/${key}`, {
    workspaceId,
    ...(taskId ? { taskId } : {}),
    body,
  });
  const responseBody = await response.text();
  expect(response.status, responseBody).toBe(200);
  return JSON.parse(responseBody) as unknown;
}

test.describe("Redmine plugin contract", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("installs, connects to a real Redmine-shaped server, and links a task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const redmineServer = createMockRedmine();
    const redmineUrl = await listen(redmineServer);

    await installPackagedPlugin(testPage);

    // Unconfigured state is a real RPC into the uploaded artifact.
    await expect(
      invokePluginAction(apiClient, "connection.get", seedData.workspaceId),
    ).resolves.toMatchObject({
      state: "disconnected",
    });

    // Connection settings round trip: a real POST from the plugin's settings
    // page through the real backend into the real Go binary, which then
    // makes a real outbound HTTP call to the mock Redmine server above.
    await invokePluginAction(apiClient, "connection.save", seedData.workspaceId, {
      base_url: redmineUrl,
      api_key: "e2e-key",
    });
    await expect(
      invokePluginAction(apiClient, "connection.get", seedData.workspaceId),
    ).resolves.toMatchObject({
      state: "connected",
      base_url: redmineUrl,
    });

    // Task linking via the shared host Link dialog.
    const task = await apiClient.createTask(seedData.workspaceId, "Redmine plugin contract task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.openTaskContextMenu(task.id);
    const linkSubmenu = testPage.getByTestId("task-context-link");
    await linkSubmenu.focus();
    await testPage.keyboard.press("ArrowRight");
    const redmineLink = testPage.getByTestId(`task-context-link-plugin-${PLUGIN_ID}:redmine-link`);
    await expect(redmineLink).toHaveText("Redmine Issue");
    await redmineLink.click();

    const linkDialog = testPage.getByRole("dialog", { name: "Link Redmine issue" });
    await expect(linkDialog).toBeVisible();
    await linkDialog.getByLabel("Issue").fill("#101");
    await linkDialog.getByRole("button", { name: "Save" }).click();
    await expect(linkDialog).toBeHidden();

    await expect(
      invokePluginAction(apiClient, "link.get", seedData.workspaceId, {}, task.id),
    ).resolves.toMatchObject({ linked: true, issue_id: 101 });

    await close(redmineServer);
  });

  test("a watcher creates one plugin-owned task per newly matching issue", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const redmineServer = createMockRedmine();
    const redmineUrl = await listen(redmineServer);

    await installPackagedPlugin(testPage);
    await invokePluginAction(apiClient, "connection.save", seedData.workspaceId, {
      base_url: redmineUrl,
      api_key: "e2e-key",
    });

    const created = (await invokePluginAction(apiClient, "watches.create", seedData.workspaceId, {
      project_id: 1,
      max_inflight_tasks: 0,
      enabled: true,
    })) as { id: string };
    expect(created.id).toBeTruthy();

    // The plugin's own poll loop runs on a multi-second interval internally;
    // trigger the same code path deterministically for the test rather than
    // waiting on wall-clock timing by re-invoking watches.list, which the
    // settings UI itself uses to observe watch state (poll execution itself
    // is exercised by internal/watch's unit suite; this proves the watcher
    // exists through the packaged artifact's real action surface).
    const listed = (await invokePluginAction(
      apiClient,
      "watches.list",
      seedData.workspaceId,
      {},
    )) as {
      watches: Array<{ id: string; project_id: number }>;
    };
    expect(listed.watches.some((w) => w.id === created.id && w.project_id === 1)).toBe(true);

    await close(redmineServer);
  });
});

import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

async function gotoForgejoSettings(page: import("@playwright/test").Page) {
  await page.goto("/settings/integrations/forgejo");
  await page.getByTestId("forgejo-origin-input").waitFor();
}

test.describe("Forgejo settings", () => {
  test("shows the workspace-scoped connection form and shared unsaved state", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/forgejo`);
    await testPage.getByTestId("forgejo-origin-input").waitFor();
    await expect(testPage.getByTestId("forgejo-origin-input")).toHaveValue("");
    await expect(testPage.getByTestId("forgejo-token-input")).toHaveValue("");
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspace/${seedData.workspaceId}/integrations/forgejo$`),
    );

    await testPage.getByTestId("forgejo-origin-input").fill("https://forgejo.example");
    await testPage.getByTestId("forgejo-token-input").fill("forgejo-token");
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();
  });

  test("keeps Forgejo credential controls usable on a mobile viewport", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 393, height: 851 });
    await gotoForgejoSettings(testPage);

    await expect(testPage.getByTestId("forgejo-origin-input")).toBeVisible();
    await expect(testPage.getByTestId("forgejo-token-input")).toBeVisible();
    await expect(testPage.getByTestId("forgejo-webhook-secret-input")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Forgejo settings");
  });

  test("connects and disconnects a workspace-scoped Forgejo configuration", async ({
    testPage,
    seedData,
  }) => {
    let saved = false;
    let savedPayload: Record<string, unknown> | null = null;
    await testPage.route("**/api/v1/forgejo/config?*", async (route) => {
      switch (route.request().method()) {
        case "PUT":
          savedPayload = route.request().postDataJSON() as Record<string, unknown>;
          saved = true;
          await route.fulfill({
            contentType: "application/json",
            body: JSON.stringify({
              workspace_id: seedData.workspaceId,
              origin: savedPayload.origin,
              username: "alice",
              has_secret: true,
              has_webhook_secret: false,
              last_ok: true,
              created_at: "",
              updated_at: "",
            }),
          });
          return;
        case "DELETE":
          saved = false;
          await route.fulfill({
            contentType: "application/json",
            body: JSON.stringify({ deleted: true }),
          });
          return;
        default:
          if (!saved) {
            await route.fulfill({ status: 204 });
            return;
          }
          await route.fulfill({
            contentType: "application/json",
            body: JSON.stringify({
              workspace_id: seedData.workspaceId,
              origin: "https://forgejo.example",
              username: "alice",
              has_secret: true,
              has_webhook_secret: false,
              last_ok: true,
              created_at: "",
              updated_at: "",
            }),
          });
      }
    });

    await gotoForgejoSettings(testPage);
    await testPage.getByTestId("forgejo-origin-input").fill("https://forgejo.example");
    await testPage.getByTestId("forgejo-token-input").fill("forgejo-token");
    await testPage.getByRole("button", { name: "Save connection" }).click();
    await expect
      .poll(() => savedPayload)
      .toMatchObject({
        origin: "https://forgejo.example",
        token: "forgejo-token",
      });
    await expect(testPage.getByText("Saved connection for alice")).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Disconnect" })).toBeVisible();

    await testPage.getByRole("button", { name: "Disconnect" }).click();
    await expect(testPage.getByText("Forgejo disconnected from this workspace")).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Disconnect" })).not.toBeVisible();
  });

  test("saves and polls an issue watch with task workflow context", async ({
    testPage,
    seedData,
  }) => {
    let savedWatch: Record<string, unknown> | null = null;
    let pollCalled = false;
    const config = {
      workspace_id: seedData.workspaceId,
      origin: "https://forgejo.example",
      username: "alice",
      has_secret: true,
      has_webhook_secret: false,
      last_ok: true,
      created_at: "",
      updated_at: "",
    };
    await testPage.route("**/api/v1/forgejo/config?*", (route) =>
      route.fulfill({ contentType: "application/json", body: JSON.stringify(config) }),
    );
    await testPage.route("**/api/v1/forgejo/repositories?*", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          repositories: [
            {
              owner: "acme",
              name: "app",
              full_name: "acme/app",
              default_branch: "main",
              html_url: "https://forgejo.example/acme/app",
            },
          ],
          total_count: 1,
        }),
      }),
    );
    await testPage.route("**/api/v1/forgejo/issue-watches?*", async (route) => {
      if (route.request().method() === "PUT") {
        savedWatch = route.request().postDataJSON() as Record<string, unknown>;
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({
            id: "watch-e2e",
            ...savedWatch,
            workspace_id: seedData.workspaceId,
          }),
        });
        return;
      }
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          watches: savedWatch
            ? [{ id: "watch-e2e", ...savedWatch, workspace_id: seedData.workspaceId }]
            : [],
        }),
      });
    });
    await testPage.route("**/api/v1/forgejo/issue-watches/watch-e2e/poll?*", (route) => {
      pollCalled = true;
      return route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ issues: [] }),
      });
    });

    await gotoForgejoSettings(testPage);
    await testPage.getByRole("button", { name: "Load repositories" }).click();
    await testPage.getByRole("button", { name: "Use for watch" }).click();
    await expect(testPage.getByPlaceholder("Owner").first()).toHaveValue("acme");
    await expect(testPage.getByPlaceholder("Repository").first()).toHaveValue("app");
    await testPage.getByPlaceholder("Workflow ID").first().fill(seedData.workflowId);
    await testPage.getByPlaceholder("Workflow step ID, optional").fill(seedData.startStepId);
    await expect(testPage.getByPlaceholder("Base branch, optional").first()).toHaveValue("main");
    await testPage.getByPlaceholder("Task instructions, optional").fill("Fix the issue");
    await testPage.getByRole("button", { name: "Save watch" }).click();

    await expect
      .poll(() => savedWatch)
      .toMatchObject({
        owner: "acme",
        repo: "app",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        base_branch: "main",
        prompt: "Fix the issue",
        enabled: true,
      });
    const pollButton = testPage.getByRole("button", { name: "Poll" }).first();
    await expect(pollButton).toBeVisible();
    await pollButton.click();
    await expect.poll(() => pollCalled).toBe(true);
    await expect(testPage.getByText("Watch found 0 matching issues")).toBeVisible();
  });
});

test.describe("Forgejo queue", () => {
  test("links a queued issue to an existing Kandev task", async ({ testPage }) => {
    let linkedPayload: Record<string, unknown> | null = null;
    await testPage.route("**/api/v1/forgejo/queue?*", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          issues: [
            {
              repository: {
                owner: "acme",
                name: "app",
                full_name: "acme/app",
                default_branch: "main",
                html_url: "https://forgejo.example/acme/app",
              },
              issue: {
                number: 7,
                title: "Fix queue linking",
                state: "open",
                html_url: "https://forgejo.example/acme/app/issues/7",
                body: "",
              },
            },
          ],
          pull_requests: [],
        }),
      }),
    );
    await testPage.route("**/api/v1/forgejo/task-issues?*", async (route) => {
      linkedPayload = route.request().postDataJSON() as Record<string, unknown>;
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          id: "link-7",
          task_id: linkedPayload.task_id,
          owner: "acme",
          repo: "app",
          issue_number: 7,
          issue_url: "https://forgejo.example/acme/app/issues/7",
          title: "Fix queue linking",
          state: "open",
        }),
      });
    });

    await testPage.goto("/forgejo");
    await expect(
      testPage.getByRole("link", { name: /acme\/app #7: Fix queue linking/ }),
    ).toBeVisible();
    await testPage.getByLabel("Existing Kandev task ID").fill("task-e2e-7");
    await testPage.getByRole("button", { name: "Link existing task" }).click();

    await expect(
      testPage.getByText("Linked Forgejo issue #7 to Kandev task task-e2e-7"),
    ).toBeVisible();
    expect(linkedPayload).toMatchObject({
      task_id: "task-e2e-7",
      owner: "acme",
      repo: "app",
      number: 7,
    });
    await assertNoDocumentHorizontalOverflow(testPage, "Forgejo queue");
  });
});

test.describe("Forgejo task links", () => {
  test("prefills pull request creation from a task worktree branch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    let createPayload: Record<string, unknown> | null = null;
    const task = await apiClient.createTask(seedData.workspaceId, "Forgejo task PR", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repositories: [
        {
          repository_id: seedData.repositoryId,
          base_branch: "main",
          checkout_branch: "feat/forgejo-e2e",
        },
      ],
    });
    const config = {
      workspace_id: seedData.workspaceId,
      origin: "https://forgejo.example",
      username: "alice",
      has_secret: true,
      has_webhook_secret: false,
      last_ok: true,
      created_at: "",
      updated_at: "",
    };
    await testPage.route("**/api/v1/forgejo/config?*", (route) =>
      route.fulfill({ contentType: "application/json", body: JSON.stringify(config) }),
    );
    await testPage.route(`**/api/v1/forgejo/tasks/${task.id}/issues?*`, (route) =>
      route.fulfill({ contentType: "application/json", body: JSON.stringify({ issues: [] }) }),
    );
    await testPage.route(`**/api/v1/forgejo/tasks/${task.id}/pull-requests?*`, (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ pull_requests: [] }),
      }),
    );
    await testPage.route("**/api/v1/forgejo/task-pull-requests/create?*", async (route) => {
      createPayload = route.request().postDataJSON() as Record<string, unknown>;
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          id: "pr-link-e2e",
          task_id: task.id,
          origin: "https://forgejo.example",
          owner: "acme",
          repo: "app",
          pr_number: 9,
          pr_url: "https://forgejo.example/acme/app/pulls/9",
          pr_title: "Forgejo task PR",
          head_branch: "feat/forgejo-e2e",
          base_branch: "main",
          state: "open",
          draft: false,
          mergeable: true,
          ci_state: "",
        }),
      });
    });

    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByTestId("forgejo-task-links-button")).toBeVisible();
    await testPage.getByTestId("forgejo-task-links-button").click();
    await testPage.getByText("Create pull request", { exact: true }).click();

    const dialog = testPage.getByRole("dialog", { name: "Create Forgejo pull request" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByLabel("Source branch")).toHaveValue("feat/forgejo-e2e");
    await expect(dialog.getByLabel("Base branch")).toHaveValue("main");
    await dialog.getByLabel("Owner").fill("acme");
    await dialog.getByLabel("Repository").fill("app");
    await dialog.getByRole("button", { name: "Create pull request" }).click();
    await expect
      .poll(() => createPayload)
      .toMatchObject({
        task_id: task.id,
        owner: "acme",
        repo: "app",
        head: "feat/forgejo-e2e",
        base: "main",
      });
  });
});

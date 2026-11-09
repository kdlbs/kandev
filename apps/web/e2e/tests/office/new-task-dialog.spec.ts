import { test as base, expect } from "../../fixtures/test-base";
import { OfficeApiClient } from "../../helpers/office-api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import {
  addProjectSourceThroughPicker,
  expectProjectSourceAfterReload,
  GITLAB_PROJECT_HOST,
  GITLAB_PROJECT_ORIGIN,
  GITLAB_PROJECT_SOURCE,
  listWorkspaceRepositories,
  openProject,
  PLUGIN_PROJECT_CLONE_URL,
  PLUGIN_PROJECT_PROVIDER,
  PLUGIN_PROJECT_REPOSITORY_ID,
  PLUGIN_PROJECT_SOURCE,
  removeRepositoriesByProvider,
} from "./project-provider-source-helpers";

type DialogSeed = {
  workspaceId: string;
  workspaceName: string;
  agentId: string;
};

const test = base.extend<{ dialogSeed: DialogSeed }, { officeApi: OfficeApiClient }>({
  officeApi: [
    async ({ backend }, use) => {
      await use(new OfficeApiClient(backend.baseUrl));
    },
    { scope: "worker" },
  ],
  dialogSeed: async ({ officeApi, apiClient, seedData, testPage }, use) => {
    // Wait for the base page fixture to restore the current seed profile before
    // creating the Office agent that uses it.
    void testPage;
    const workspaceName = `New Task Dialog E2E ${Date.now()}`;
    const result = await officeApi.completeOnboarding({
      workspaceName,
      taskPrefix: "NTD",
      agentName: "CEO",
      agentProfileId: seedData.agentProfileId,
      executorPreference: "local_pc",
    });
    await apiClient.saveUserSettings({
      workspace_id: result.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
    });

    try {
      await use({
        workspaceId: result.workspaceId,
        workspaceName,
        agentId: result.agentId,
      });
    } finally {
      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: seedData.workflowId,
      });
      await officeApi.deleteWorkspace(result.workspaceId, workspaceName);
    }
  },
});

/**
 * Regression coverage for the "New Task" dialog (ISSUE-7): it used to drop
 * the chosen assignee entirely (metadata carried it, the create request
 * never did) and wrote the reviewer/approver "Stages" picker into the dead
 * `execution_policy` metadata field, so a created task got no runner and a
 * misleading success toast. The fix sends `assignee_agent_profile_id` as a
 * top-level create field and removes the Stages picker.
 *
 * No spec drove the real dialog before this file — see
 * `new-task-dialog-create-payload.test.tsx` for the unit-level payload pin,
 * which mounts the component directly and never exercises the "New Task"
 * button, the pickers, or the resulting toast/backend round trip.
 */

async function openNewTaskDialog(testPage: import("@playwright/test").Page) {
  await testPage.goto("/office/tasks");
  // The global sidebar also has a "New Task" trigger (icon-only, aria-label
  // only) that opens this same dialog on office routes, so a bare
  // getByRole("button", { name: "New Task" }) is ambiguous. Disambiguate via
  // the toolbar button's icon (IconPlus / tabler-icon-plus) versus the
  // sidebar's IconSquarePlus.
  await testPage.locator('button:has(svg.tabler-icon-plus):has-text("New Task")').click();
  const dialog = testPage.getByTestId("office-new-issue-dialog");
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  return dialog;
}

async function createProject(
  officeApi: OfficeApiClient,
  workspaceId: string,
  name: string,
): Promise<{ id: string; name: string }> {
  const project = (await officeApi.createProject(workspaceId, name)) as {
    id: string;
    name: string;
  };
  expect(project.id).toBeTruthy();
  return project;
}

test.describe("Office New Task dialog", () => {
  test("project sources attach before launch", async ({
    testPage,
    officeApi,
    dialogSeed,
    apiClient,
    seedData,
  }) => {
    const officeRepository = await apiClient.createRepository(
      dialogSeed.workspaceId,
      seedData.repositoryPath,
    );
    const project = (await officeApi.createProject(
      dialogSeed.workspaceId,
      "New Task Dialog Project Source E2E",
      [seedData.repositoryPath],
    )) as { id: string; name: string };
    expect(project.id).toBeTruthy();

    const dialog = await openNewTaskDialog(testPage);
    await dialog.getByPlaceholder("Task title").fill("New Task Dialog Project Source E2E");
    await dialog.getByRole("button", { name: "Project" }).click();
    await testPage.getByRole("button", { name: project.name, exact: true }).click();

    const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
    await dialog.getByTestId("new-task-create-button").click();
    const response = await created;
    const body = (await response.json()) as {
      id?: string;
      repositories?: Array<{ repository_id: string }>;
    };
    expect(body.id).toBeTruthy();
    expect(body.repositories?.map((repository) => repository.repository_id)).toEqual([
      officeRepository.id,
    ]);

    const stored = await apiClient.getTask(body.id as string);
    expect(stored.workspace_id).toBe(dialogSeed.workspaceId);
    expect(stored.repositories?.map((repository) => repository.repository_id)).toEqual([
      officeRepository.id,
    ]);
  });

  test("creating a task with an assignee seats the runner and shows no stages picker", async ({
    testPage,
    officeApi,
    dialogSeed,
  }) => {
    // Onboarding does not seed a default project (see
    // office-api-client.ts's createProject doc comment), so the picker needs
    // a real project created up front.
    const project = await createProject(
      officeApi,
      dialogSeed.workspaceId,
      "New Task Dialog E2E Success Project",
    );

    const dialog = await openNewTaskDialog(testPage);

    // Regression: the reviewer/approver "Stages" picker (which wrote into
    // the dead `execution_policy` metadata field) is fully removed.
    await expect(dialog.getByText(/review stages/i)).toHaveCount(0);

    await dialog.getByPlaceholder("Task title").fill("New Task Dialog E2E Success");

    await dialog.getByRole("button", { name: "Project" }).click();
    await testPage.getByRole("button", { name: project.name, exact: true }).click();

    await dialog.getByRole("button", { name: "Assignee" }).click();
    const assignedAgent = await officeApi.getAgent(dialogSeed.agentId);
    const agentName = typeof assignedAgent.name === "string" ? assignedAgent.name : "";
    expect(agentName).not.toBe("");
    await testPage.getByRole("button", { name: agentName, exact: true }).click();

    const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
    await dialog.getByTestId("new-task-create-button").click();
    const response = await created;
    const body = (await response.json()) as { id?: string };
    expect(body.id).toBeTruthy();
    const taskId = body.id as string;

    await expect(testPage.locator("[data-sonner-toast]")).toContainText(/task created/i, {
      timeout: 10_000,
    });
    await expect(dialog).toBeHidden();

    // The create request carried the assignee at all (the pre-fix bug: the
    // dialog built `assignee_agent_profile_id` into `metadata` instead of
    // sending it as a top-level create field, so the backend never saw it
    // and seated no runner). GET /api/v1/office/tasks/:id wraps the office
    // dashboard's camelCase TaskDTO under a "task" key, distinct from the
    // snake_case shape the create POST itself returns.
    const stored = (await officeApi.getTask(taskId)) as {
      task: { assigneeAgentProfileId?: string; projectId?: string };
    };
    expect(stored.task.assigneeAgentProfileId).toBe(dialogSeed.agentId);
    expect(stored.task.projectId).toBe(project.id);
  });

  test("a rejected create surfaces an error toast, not a success toast", async ({
    testPage,
    officeApi,
    dialogSeed,
  }) => {
    const project = await createProject(
      officeApi,
      dialogSeed.workspaceId,
      "New Task Dialog E2E Failure Project",
    );

    const dialog = await openNewTaskDialog(testPage);

    await dialog.getByPlaceholder("Task title").fill("New Task Dialog E2E Failure");
    await dialog.getByRole("button", { name: "Project" }).click();
    await testPage.getByRole("button", { name: project.name, exact: true }).click();
    await dialog.getByRole("button", { name: "Assignee" }).click();
    const assignedAgent = await officeApi.getAgent(dialogSeed.agentId);
    const agentName = typeof assignedAgent.name === "string" ? assignedAgent.name : "";
    expect(agentName).not.toBe("");
    await testPage.getByRole("button", { name: agentName, exact: true }).click();

    // Force the backend to reject the create so the dialog's failure branch
    // (an error toast, draft preserved, dialog stays open) is exercised
    // without depending on a specific server-side validation rule.
    await testPage.route(
      (url) => url.pathname === "/api/v1/tasks",
      async (route) => {
        if (route.request().method() === "POST") {
          await route.fulfill({
            status: 422,
            contentType: "application/json",
            body: JSON.stringify({ error: "assignee agent profile not found" }),
          });
          return;
        }
        await route.continue();
      },
    );

    await dialog.getByTestId("new-task-create-button").click();

    const toast = testPage.locator("[data-sonner-toast]");
    await expect(toast).toContainText(/assignee agent profile not found/i, { timeout: 10_000 });
    await expect(toast).not.toContainText(/task created/i);
    // Failure never closes the dialog or clears the draft (only success does).
    await expect(dialog).toBeVisible();
    await expect(dialog.getByPlaceholder("Task title")).toHaveValue("New Task Dialog E2E Failure");
  });
});

/**
 * Provider-neutral project sources. A project stores literal source strings
 * without provider hints, so these suites prove the server resolves a
 * self-managed GitLab origin and an installed repository plugin from the bare
 * URL alone.
 *
 * @covers AC-TASKS-PROJECT-REPOSITORIES-001.1, AC-TASKS-PROJECT-REPOSITORIES-001.3,
 * AC-TASKS-PROJECT-REPOSITORIES-001.9, AC-TASKS-PROJECT-REPOSITORIES-001.10,
 * AC-TASKS-PROJECT-REPOSITORIES-001.12
 */
test.describe("Office New Task provider-neutral project sources", () => {
  test.afterEach(async ({ apiClient, dialogSeed }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
    await removeRepositoriesByProvider(
      apiClient,
      dialogSeed.workspaceId,
      (repository) =>
        repository.provider === PLUGIN_PROJECT_PROVIDER || repository.provider === "gitlab",
    ).catch(() => undefined);
  });

  test("self-managed GitLab project sources attach before launch", async ({
    testPage,
    officeApi,
    dialogSeed,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    await apiClient.configureGitLab(dialogSeed.workspaceId, GITLAB_PROJECT_HOST);
    const project = (await officeApi.createProject(
      dialogSeed.workspaceId,
      `GitLab Project Source ${Date.now()}`,
    )) as { id: string; name: string };

    await openProject(testPage, project.id);
    await addProjectSourceThroughPicker(testPage, project.id, GITLAB_PROJECT_SOURCE);
    await expectProjectSourceAfterReload(testPage, project.id, GITLAB_PROJECT_SOURCE);

    const dialog = await openNewTaskDialog(testPage);
    await dialog.getByPlaceholder("Task title").fill("GitLab Project Source Task");
    await dialog.getByRole("button", { name: "Project" }).click();
    await testPage.getByRole("button", { name: project.name, exact: true }).click();

    const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
    await dialog.getByTestId("new-task-create-button").click();
    const response = await created;
    const body = (await response.json()) as {
      id?: string;
      repositories?: Array<{ repository_id: string }>;
    };
    expect(body.id).toBeTruthy();
    expect(body.repositories).toHaveLength(1);
    const repositoryID = body.repositories?.[0]?.repository_id as string;
    expect(repositoryID).toBeTruthy();

    const stored = await apiClient.getTask(body.id as string);
    expect(stored.repositories?.map((repository) => repository.repository_id)).toEqual([
      repositoryID,
    ]);
    const repositories = await listWorkspaceRepositories(apiClient, dialogSeed.workspaceId);
    expect(repositories.find((repository) => repository.id === repositoryID)).toMatchObject({
      provider: "gitlab",
      provider_host: GITLAB_PROJECT_ORIGIN,
      provider_owner: "group/subgroup",
      provider_name: "project",
      remote_url: `${GITLAB_PROJECT_ORIGIN}/group/subgroup/project.git`,
    });
  });

  test("a mismatched GitLab origin shows the existing error without a task", async ({
    testPage,
    officeApi,
    dialogSeed,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    await apiClient.configureGitLab(dialogSeed.workspaceId, "https://gitlab.other.example.test");
    const project = (await officeApi.createProject(
      dialogSeed.workspaceId,
      `GitLab Mismatch ${Date.now()}`,
    )) as { id: string; name: string };
    await openProject(testPage, project.id);
    await addProjectSourceThroughPicker(testPage, project.id, GITLAB_PROJECT_SOURCE);

    const dialog = await openNewTaskDialog(testPage);
    const title = "GitLab Mismatch Task";
    await dialog.getByPlaceholder("Task title").fill(title);
    await dialog.getByRole("button", { name: "Project" }).click();
    await testPage.getByRole("button", { name: project.name, exact: true }).click();
    await dialog.getByTestId("new-task-create-button").click();

    await expect(
      testPage.getByText("The selected repository could not be verified.", { exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(dialog).toBeVisible();
    const tasks = await apiClient.listTasks(dialogSeed.workspaceId);
    expect(tasks.tasks.some((task) => task.title === title)).toBe(false);
  });

  test("installed plugin project URL resolves without a provider hint", async ({
    testPage,
    officeApi,
    dialogSeed,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const project = (await officeApi.createProject(
      dialogSeed.workspaceId,
      `Plugin Project Source ${Date.now()}`,
    )) as { id: string; name: string };

    await openProject(testPage, project.id);
    await addProjectSourceThroughPicker(testPage, project.id, PLUGIN_PROJECT_SOURCE);
    await expectProjectSourceAfterReload(testPage, project.id, PLUGIN_PROJECT_SOURCE);

    const dialog = await openNewTaskDialog(testPage);
    await dialog.getByPlaceholder("Task title").fill("Plugin Project Source Task");
    await dialog.getByRole("button", { name: "Project" }).click();
    await testPage.getByRole("button", { name: project.name, exact: true }).click();

    const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
    await dialog.getByTestId("new-task-create-button").click();
    const response = await created;
    const body = (await response.json()) as {
      id?: string;
      repositories?: Array<{ repository_id: string }>;
    };
    expect(body.id).toBeTruthy();
    expect(body.repositories).toHaveLength(1);
    const repositoryID = body.repositories?.[0]?.repository_id as string;

    const repositories = await listWorkspaceRepositories(apiClient, dialogSeed.workspaceId);
    expect(repositories.find((repository) => repository.id === repositoryID)).toMatchObject({
      provider: PLUGIN_PROJECT_PROVIDER,
      provider_repo_id: PLUGIN_PROJECT_REPOSITORY_ID,
      provider_owner: "TEAM",
      provider_name: "fixture",
      remote_url: PLUGIN_PROJECT_CLONE_URL,
      default_branch: "main",
    });
    const stored = await apiClient.getTask(body.id as string);
    expect(stored.repositories?.map((repository) => repository.repository_id)).toEqual([
      repositoryID,
    ]);
  });

  test("a disabled plugin refuses a new project source without writes", async ({
    testPage,
    officeApi,
    dialogSeed,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const project = (await officeApi.createProject(
      dialogSeed.workspaceId,
      `Disabled Plugin ${Date.now()}`,
    )) as { id: string; name: string };
    await openProject(testPage, project.id);
    await addProjectSourceThroughPicker(testPage, project.id, PLUGIN_PROJECT_SOURCE);

    const disable = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/disable`);
    expect(disable.ok, await disable.text()).toBe(true);

    const dialog = await openNewTaskDialog(testPage);
    const title = "Disabled Plugin Task";
    await dialog.getByPlaceholder("Task title").fill(title);
    await dialog.getByRole("button", { name: "Project" }).click();
    await testPage.getByRole("button", { name: project.name, exact: true }).click();
    await dialog.getByTestId("new-task-create-button").click();

    await expect(
      testPage.getByText("The selected repository could not be verified.", { exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(dialog).toBeVisible();
    const tasks = await apiClient.listTasks(dialogSeed.workspaceId);
    expect(tasks.tasks.some((task) => task.title === title)).toBe(false);
    const repositories = await listWorkspaceRepositories(apiClient, dialogSeed.workspaceId);
    expect(repositories.some((repository) => repository.provider === PLUGIN_PROJECT_PROVIDER)).toBe(
      false,
    );
  });
});

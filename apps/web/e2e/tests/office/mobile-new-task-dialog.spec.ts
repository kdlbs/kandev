import { test, expect } from "../../fixtures/office-fixture";
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

test("project sources attach before launch", async ({
  testPage,
  officeApi,
  officeSeed,
  apiClient,
  seedData,
}) => {
  const officeRepository = await apiClient.createRepository(
    officeSeed.workspaceId,
    seedData.repositoryPath,
  );
  const project = (await officeApi.createProject(officeSeed.workspaceId, "Mobile Project Source", [
    seedData.repositoryPath,
  ])) as { id: string; name: string };
  expect(project.id).toBeTruthy();

  await testPage.goto("/office/tasks");
  await testPage.getByRole("main").getByRole("button", { name: "New Task", exact: true }).tap();
  const dialog = testPage.getByTestId("office-new-issue-dialog");
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  await dialog.getByPlaceholder("Task title").fill("Mobile Project Source Task");
  await dialog.getByRole("button", { name: "Project" }).tap();
  await testPage.getByRole("button", { name: project.name, exact: true }).tap();

  const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
  await dialog.getByTestId("new-task-create-button").tap();
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
  expect(stored.workspace_id).toBe(officeSeed.workspaceId);
  expect(stored.repositories?.map((repository) => repository.repository_id)).toEqual([
    officeRepository.id,
  ]);
});

test("mobile Office task creation only shows supported assignee controls", async ({
  testPage,
  officeApi,
  officeSeed,
}) => {
  const runId = Date.now();
  const projectName = `Mobile New Task Dialog Project ${runId}`;
  const taskTitle = `Mobile New Task Dialog Assignment ${runId}`;
  const project = (await officeApi.createProject(officeSeed.workspaceId, projectName)) as {
    id: string;
    name: string;
  };
  expect(project.id).toBeTruthy();

  await testPage.goto("/office/tasks");
  await testPage.getByRole("main").getByRole("button", { name: "New Task", exact: true }).tap();

  const dialog = testPage.getByTestId("office-new-issue-dialog");
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  await expect(dialog.getByRole("button", { name: "Attach files" })).toBeVisible();
  await expect(dialog.locator("button:has(svg.tabler-icon-dots-vertical)")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "Upload", exact: true })).toHaveCount(0);

  await dialog.getByPlaceholder("Task title").fill(taskTitle);
  await dialog.getByRole("button", { name: "Project" }).tap();
  await testPage.getByRole("button", { name: project.name, exact: true }).tap();
  await dialog.getByRole("button", { name: "Assignee" }).tap();
  await testPage.getByRole("button", { name: "CEO", exact: true }).tap();

  const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
  await dialog.getByTestId("new-task-create-button").tap();
  const response = await created;
  const body = (await response.json()) as { id?: string };
  expect(body.id).toBeTruthy();

  await expect(testPage.locator("[data-sonner-toast]")).toContainText(/task created/i, {
    timeout: 10_000,
  });
  const stored = (await officeApi.getTask(body.id as string)) as {
    task: { assigneeAgentProfileId?: string; projectId?: string };
  };
  expect(stored.task.assigneeAgentProfileId).toBe(officeSeed.agentId);
  expect(stored.task.projectId).toBe(project.id);
});

/**
 * Provider-neutral project sources on the phone surface. The project editor and
 * New Task form share the desktop composition; these suites prove the same
 * admission path through the mobile route.
 *
 * @covers AC-TASKS-PROJECT-REPOSITORIES-001.1, AC-TASKS-PROJECT-REPOSITORIES-001.9,
 * AC-TASKS-PROJECT-REPOSITORIES-001.10, AC-TASKS-PROJECT-REPOSITORIES-001.12
 */
test.describe("mobile provider-neutral project sources", () => {
  test.afterEach(async ({ apiClient, officeSeed }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
    await apiClient
      .rawRequest(
        "DELETE",
        `/api/v1/gitlab/config?workspace_id=${encodeURIComponent(officeSeed.workspaceId)}`,
      )
      .catch(() => undefined);
    await removeRepositoriesByProvider(
      apiClient,
      officeSeed.workspaceId,
      (repository) =>
        repository.provider === PLUGIN_PROJECT_PROVIDER || repository.provider === "gitlab",
    ).catch(() => undefined);
  });

  test("self-managed GitLab project sources attach before launch", async ({
    testPage,
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    await apiClient.configureGitLab(officeSeed.workspaceId, GITLAB_PROJECT_HOST);
    const project = (await officeApi.createProject(
      officeSeed.workspaceId,
      `Mobile GitLab Source ${Date.now()}`,
    )) as { id: string; name: string };

    await openProject(testPage, project.id);
    await addProjectSourceThroughPicker(testPage, project.id, GITLAB_PROJECT_SOURCE);
    await expectProjectSourceAfterReload(testPage, project.id, GITLAB_PROJECT_SOURCE);

    await testPage.goto("/office/tasks");
    await testPage.getByRole("main").getByRole("button", { name: "New Task", exact: true }).tap();
    const dialog = testPage.getByTestId("office-new-issue-dialog");
    await expect(dialog).toBeVisible({ timeout: 10_000 });
    await dialog.getByPlaceholder("Task title").fill("Mobile GitLab Source Task");
    await dialog.getByRole("button", { name: "Project" }).tap();
    await testPage.getByRole("button", { name: project.name, exact: true }).tap();

    const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
    await dialog.getByTestId("new-task-create-button").tap();
    const response = await created;
    const body = (await response.json()) as {
      id?: string;
      repositories?: Array<{ repository_id: string }>;
    };
    expect(body.id).toBeTruthy();
    expect(body.repositories).toHaveLength(1);
    const repositoryID = body.repositories?.[0]?.repository_id as string;

    const stored = await apiClient.getTask(body.id as string);
    expect(stored.workspace_id).toBe(officeSeed.workspaceId);
    expect(stored.repositories?.map((repository) => repository.repository_id)).toEqual([
      repositoryID,
    ]);
    const repositories = await listWorkspaceRepositories(apiClient, officeSeed.workspaceId);
    expect(repositories.find((repository) => repository.id === repositoryID)).toMatchObject({
      provider: "gitlab",
      provider_host: GITLAB_PROJECT_ORIGIN,
      provider_owner: "group/subgroup",
      provider_name: "project",
      remote_url: `${GITLAB_PROJECT_ORIGIN}/group/subgroup/project.git`,
    });
  });

  test("installed plugin project URL resolves without a provider hint", async ({
    testPage,
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const project = (await officeApi.createProject(
      officeSeed.workspaceId,
      `Mobile Plugin Source ${Date.now()}`,
    )) as { id: string; name: string };

    await openProject(testPage, project.id);
    await addProjectSourceThroughPicker(testPage, project.id, PLUGIN_PROJECT_SOURCE);
    await expectProjectSourceAfterReload(testPage, project.id, PLUGIN_PROJECT_SOURCE);

    await testPage.goto("/office/tasks");
    await testPage.getByRole("main").getByRole("button", { name: "New Task", exact: true }).tap();
    const dialog = testPage.getByTestId("office-new-issue-dialog");
    await expect(dialog).toBeVisible({ timeout: 10_000 });
    await dialog.getByPlaceholder("Task title").fill("Mobile Plugin Source Task");
    await dialog.getByRole("button", { name: "Project" }).tap();
    await testPage.getByRole("button", { name: project.name, exact: true }).tap();

    const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
    await dialog.getByTestId("new-task-create-button").tap();
    const response = await created;
    const body = (await response.json()) as {
      id?: string;
      repositories?: Array<{ repository_id: string }>;
    };
    expect(body.id).toBeTruthy();
    expect(body.repositories).toHaveLength(1);
    const repositoryID = body.repositories?.[0]?.repository_id as string;

    const repositories = await listWorkspaceRepositories(apiClient, officeSeed.workspaceId);
    expect(repositories.find((repository) => repository.id === repositoryID)).toMatchObject({
      provider: PLUGIN_PROJECT_PROVIDER,
      provider_repo_id: PLUGIN_PROJECT_REPOSITORY_ID,
      remote_url: PLUGIN_PROJECT_CLONE_URL,
    });
    const stored = await apiClient.getTask(body.id as string);
    expect(stored.repositories?.map((repository) => repository.repository_id)).toEqual([
      repositoryID,
    ]);
  });

  test("a disabled plugin refuses a new project source without writes", async ({
    testPage,
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const project = (await officeApi.createProject(
      officeSeed.workspaceId,
      `Mobile Disabled Plugin ${Date.now()}`,
    )) as { id: string; name: string };
    await openProject(testPage, project.id);
    await addProjectSourceThroughPicker(testPage, project.id, PLUGIN_PROJECT_SOURCE);

    const disable = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/disable`);
    expect(disable.ok, await disable.text()).toBe(true);

    await testPage.goto("/office/tasks");
    await testPage.getByRole("main").getByRole("button", { name: "New Task", exact: true }).tap();
    const dialog = testPage.getByTestId("office-new-issue-dialog");
    await expect(dialog).toBeVisible({ timeout: 10_000 });
    const title = "Mobile Disabled Plugin Task";
    await dialog.getByPlaceholder("Task title").fill(title);
    await dialog.getByRole("button", { name: "Project" }).tap();
    await testPage.getByRole("button", { name: project.name, exact: true }).tap();
    // officeSeed is worker-scoped and cleanup is best-effort, so compare the
    // exact repository identities rather than assuming an empty workspace.
    const beforeRepositoryIds = (await listWorkspaceRepositories(apiClient, officeSeed.workspaceId))
      .filter((repository) => repository.provider === PLUGIN_PROJECT_PROVIDER)
      .map((repository) => repository.id)
      .sort();
    await dialog.getByTestId("new-task-create-button").tap();

    await expect(
      testPage.getByText("The selected repository could not be verified.", { exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(dialog).toBeVisible();
    const tasks = await apiClient.listTasks(officeSeed.workspaceId);
    expect(tasks.tasks.some((task) => task.title === title)).toBe(false);
    const repositories = await listWorkspaceRepositories(apiClient, officeSeed.workspaceId);
    expect(
      repositories
        .filter((repository) => repository.provider === PLUGIN_PROJECT_PROVIDER)
        .map((repository) => repository.id)
        .sort(),
    ).toEqual(beforeRepositoryIds);
  });
});

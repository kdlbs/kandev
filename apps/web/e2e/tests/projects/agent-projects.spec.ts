import { test, expect } from "../../fixtures/test-base";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import {
  readAgentProject,
  setupAgentProjectFixture,
  type AgentProjectView,
} from "./agent-projects-fixture";
import type { ApiClient } from "../../helpers/api-client";

async function listProjects(apiClient: ApiClient, workspaceId: string) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/agent-projects`,
  );
  if (!response.ok) throw new Error(`Agent Project list failed (${response.status})`);
  return (await response.json()) as { projects: AgentProjectView[] };
}

test("desktop users create a project, start a coordinator worker, share context, and manage the tree", async ({
  testPage,
  apiClient,
  backend,
  seedData,
  prCapture,
}) => {
  test.setTimeout(240_000);
  const fixtureName = `agent-project-desktop-${Date.now()}`;
  const fixture = await setupAgentProjectFixture(apiClient, backend, seedData, fixtureName);
  let projectId = "";
  let projectName = "Agent Project desktop";
  try {
    await testPage.goto("/");
    const createButton = testPage.getByTestId("agent-project-create-open");
    await expect(createButton).toBeVisible();
    await createButton.click();
    const form = testPage.getByTestId("agent-project-form-desktop");
    await expect(form).toBeVisible();
    await form.getByTestId("agent-project-name").fill(projectName);
    await form
      .locator("label")
      .filter({ hasText: `fixture/${fixtureName}` })
      .locator("input")
      .check();
    await form.locator("select").nth(0).selectOption(fixture.repositoryId);
    for (const profileSelect of [1, 2, 3]) {
      await form.locator("select").nth(profileSelect).selectOption(fixture.profileId);
    }
    const submit = form.getByTestId("agent-project-submit");
    await expect(submit).toBeEnabled();
    await submit.click();

    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects[0]?.id ?? "")
      .not.toBe("");
    let project = (await listProjects(apiClient, seedData.workspaceId)).projects[0]!;
    projectId = project.id;
    await expect(testPage.getByTestId(`agent-project-row-${projectId}`)).toBeVisible();

    const projectRow = testPage.getByTestId(`agent-project-row-${projectId}`);
    await projectRow.getByRole("button", { name: `Actions for ${projectName}` }).click();
    await testPage.getByRole("menuitem", { name: "Edit project" }).click();
    const editForm = testPage.getByTestId("agent-project-form-desktop");
    await editForm.getByTestId("agent-project-name").fill(`${projectName} edited`);
    await editForm.getByTestId("agent-project-submit").click();
    projectName = `${projectName} edited`;
    await expect(testPage.getByTestId(`agent-project-open-${projectId}`)).toContainText(
      projectName,
    );

    project = await readAgentProject(apiClient, seedData.workspaceId, projectId);
    await testPage.getByTestId(`agent-project-open-${projectId}`).click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${project.main_task_id}$`));
    const coordinator = new SessionPage(testPage);
    await coordinator.waitForLoad();
    await coordinator.openNewSessionDialog();
    await coordinator.newSessionPromptInput().fill("/e2e:agent-project-create-worker");
    await coordinator.newSessionStartButton().click();
    await waitForLatestSessionDone(
      apiClient,
      project.main_task_id,
      1,
      "Waiting for the coordinator to create its worker through MCP",
      120_000,
    );
    const coordinatorSessions = await apiClient.listTaskSessions(project.main_task_id);
    const coordinatorMessages = await apiClient.listSessionMessages(
      coordinatorSessions.sessions[0]!.id,
    );
    expect(coordinatorMessages.messages.map((message) => message.content).join("\n")).toContain(
      "The project worker was created.",
    );

    await expect
      .poll(async () => {
        const view = await readAgentProject(apiClient, seedData.workspaceId, projectId);
        return view.tasks.filter((task) => task.parent_id === project.main_task_id).length;
      })
      .toBe(1);
    project = await readAgentProject(apiClient, seedData.workspaceId, projectId);
    const worker = project.tasks.find((task) => task.parent_id === project.main_task_id);
    expect(worker).toBeTruthy();
    await waitForLatestSessionDone(
      apiClient,
      worker!.id,
      1,
      "Waiting for the project worker",
      120_000,
    );
    const completedProject = await readAgentProject(apiClient, seedData.workspaceId, projectId);
    const completedWorker = completedProject.tasks.find((task) => task.id === worker!.id);
    expect(completedWorker).toBeTruthy();

    await coordinator.clickTab("Files");
    const fileRoots = testPage.getByTestId("agent-project-file-roots");
    await expect(fileRoots.getByRole("tab", { name: "Context" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await fileRoots.getByRole("button", { name: "notes.md", exact: true }).click();
    const contextEditor = fileRoots.getByTestId("agent-project-context-editor");
    await contextEditor.fill("Shared project context from the desktop E2E.\n");
    await fileRoots.getByRole("button", { name: "Save context" }).click();
    await expect
      .poll(async () => {
        const response = await apiClient.rawRequest(
          "GET",
          `/api/v1/workspaces/${seedData.workspaceId}/agent-projects/${projectId}/context/content?path=notes.md`,
        );
        if (!response.ok) return "";
        return ((await response.json()) as { content: string }).content;
      })
      .toContain("Shared project context from the desktop E2E.");

    await fileRoots.getByRole("tab", { name: "Workspace" }).click();
    await expect(fileRoots.getByRole("tab", { name: "Workspace" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await coordinator.clickTab("Changes");
    await expect(coordinator.changes).toBeVisible();
    await expect(coordinator.changes).not.toContainText("notes.md");

    await expect(testPage.getByTestId(`agent-project-expand-${projectId}`)).toBeVisible();
    await testPage.getByTestId(`agent-project-expand-${projectId}`).click();
    const workerRow = testPage.getByTestId(`agent-project-worker-${worker!.id}`);
    await expect(workerRow).toBeVisible();
    await expect(workerRow.getByText(completedWorker!.state, { exact: true })).toBeVisible();
    await prCapture.screenshot("agent-project-desktop-worker-row", {
      caption: "Project sidebar with a coordinator-created worker",
    });
    await workerRow.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${worker!.id}$`));
    const workerSession = new SessionPage(testPage);
    await workerSession.waitForLoad();
    await workerSession.clickTab("Files");
    const workerRoots = testPage.getByTestId("agent-project-file-roots");
    await workerRoots.getByRole("tab", { name: "Context" }).click();
    await workerRoots.getByRole("button", { name: "notes.md", exact: true }).click();
    await expect(workerRoots.getByTestId("agent-project-context-editor")).toHaveValue(
      "Shared project context from the desktop E2E.\n",
    );

    const actions = testPage
      .getByTestId(`agent-project-row-${projectId}`)
      .getByRole("button", { name: `Actions for ${projectName}` });
    await actions.click();
    await testPage.getByRole("menuitem", { name: "Archive project" }).click();
    await testPage.getByTestId("agent-project-archive-confirm").click();
    await expect(testPage.getByTestId("agent-project-archive-confirmation")).toBeHidden({
      timeout: 30_000,
    });
    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects.length, {
        timeout: 30_000,
      })
      .toBe(0);
    const sidebar = testPage.locator('[data-testid="app-sidebar-scroll"]:visible');
    await expect(sidebar.getByText("No projects yet")).toBeVisible();
    await sidebar.getByTestId("agent-project-archived-toggle").click();
    const archivedRow = sidebar.getByTestId(`agent-project-row-${projectId}`);
    await expect(archivedRow).toBeVisible();
    const restoreResponsePromise = testPage.waitForResponse(
      (response) =>
        response.url().includes(`/agent-projects/${projectId}/restore`) &&
        response.request().method() === "POST",
    );
    await archivedRow.getByRole("button", { name: `Restore ${projectName}` }).click();
    const restoreResponse = await restoreResponsePromise;
    expect(restoreResponse.status(), await restoreResponse.text()).toBe(200);
    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects.length, {
        timeout: 30_000,
      })
      .toBe(1);
    await sidebar.getByTestId("agent-project-archived-toggle").click();

    const activeRow = sidebar.getByTestId(`agent-project-row-${projectId}`);
    await expect(activeRow).toBeVisible({ timeout: 30_000 });
    await activeRow.getByRole("button", { name: `Actions for ${projectName}` }).click();
    await testPage.getByRole("menuitem", { name: "Delete project" }).click();
    const confirmation = testPage.getByTestId("agent-project-delete-confirmation");
    await confirmation.getByRole("checkbox", { name: "Also delete shared context" }).check();
    await confirmation
      .getByRole("checkbox", { name: /Permanently discard tracked and untracked changes/ })
      .check();
    const deleteResponsePromise = testPage.waitForResponse(
      (response) =>
        response.url().includes(`/agent-projects/${projectId}?`) &&
        response.request().method() === "DELETE",
      { timeout: 120_000 },
    );
    await confirmation.getByTestId("agent-project-delete-confirm").click();
    const deleteResponse = await deleteResponsePromise;
    expect(deleteResponse.status()).toBe(204);
    await expect(testPage.getByTestId("agent-project-delete-confirmation")).toBeHidden({
      timeout: 120_000,
    });
    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects.length, {
        timeout: 30_000,
      })
      .toBe(0);
    const deletedContext = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${seedData.workspaceId}/agent-projects/${projectId}/context/tree`,
    );
    expect(deletedContext.status).toBe(404);
    projectId = "";
  } finally {
    await fixture.cleanup(projectId || undefined);
  }
});

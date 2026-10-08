import { test, expect } from "../../fixtures/test-base";
import { waitForLatestSessionDone, waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import {
  readAgentProject,
  restoreArchivedAgentProject,
  setupAgentProjectFixture,
  type AgentProjectView,
} from "./agent-projects-fixture";
import type { ApiClient } from "../../helpers/api-client";
import {
  assertProjectFormGeometry,
  assertProjectRemotePickerGeometry,
  assertProjectTouchTarget,
  assertProjectWorkerProfilesGeometry,
  settleProjectSurface,
} from "./agent-projects-geometry";

async function listProjects(apiClient: ApiClient, workspaceId: string) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/agent-projects`,
  );
  if (!response.ok) throw new Error(`Agent Project list failed (${response.status})`);
  return (await response.json()) as { projects: AgentProjectView[] };
}

async function chooseConfiguredRepository(
  form: import("@playwright/test").Locator,
  repositoryId: string,
  repositoryName: string,
  touch = false,
) {
  const addRepository = form.getByTestId("agent-project-add-repository");
  if (touch) await addRepository.tap();
  else await addRepository.click();
  const option = form
    .page()
    .getByTestId("agent-project-existing-repository-option")
    .filter({ hasText: repositoryName });
  if (touch) await option.tap();
  else await option.click();
  await form.getByRole("combobox", { name: "Primary repository" }).selectOption(repositoryId);
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
    await testPage.setViewportSize({ width: 1280, height: 900 });
    await testPage.goto("/");
    const projectsHeader = testPage.locator('[aria-controls="sidebar-section-projects"]');
    await expect(projectsHeader).toHaveAttribute("aria-expanded", "false");
    const createButton = testPage.getByTestId("agent-project-create-open");
    await expect(createButton).toBeVisible();
    const createButtonBox = await createButton.boundingBox();
    expect(createButtonBox).not.toBeNull();
    expect(Math.abs(createButtonBox!.width - 24)).toBeLessThanOrEqual(1);
    expect(Math.abs(createButtonBox!.height - 24)).toBeLessThanOrEqual(1);
    await expect(
      testPage.locator("#sidebar-section-projects").getByText("No projects yet"),
    ).not.toBeVisible();
    await createButton.click();
    const form = testPage.getByTestId("agent-project-form-desktop");
    await expect(form).toBeVisible();
    await expect(form.getByRole("combobox", { name: "Primary repository" })).toHaveValue("");
    await form.getByTestId("agent-project-name").fill(projectName);
    await expect(form.getByText("Initial prompt (optional)", { exact: true })).toBeVisible();
    await chooseConfiguredRepository(form, fixture.repositoryId, `fixture/${fixtureName}`);
    await form
      .getByRole("combobox", { name: "Coordinator profile" })
      .selectOption(fixture.profileId);
    const submit = form.getByTestId("agent-project-submit");
    await expect(submit).toBeEnabled();
    await assertProjectFormGeometry(form, false);
    await testPage.screenshot({ path: test.info().outputPath("desktop-project-form.png") });
    await submit.click();

    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects[0]?.id ?? "")
      .not.toBe("");
    let project = (await listProjects(apiClient, seedData.workspaceId)).projects[0]!;
    projectId = project.id;
    expect((await apiClient.listTaskSessions(project.main_task_id)).sessions).toHaveLength(0);
    await expect(testPage.getByTestId(`agent-project-row-${projectId}`)).toBeVisible();
    await settleProjectSurface(testPage.locator('[data-testid="app-sidebar-scroll"]:visible'));
    await testPage.screenshot({ path: test.info().outputPath("desktop-project-sidebar.png") });
    await expect(
      testPage.locator(
        `[data-testid="sidebar-task-item"][data-task-row-id="${project.main_task_id}"]`,
      ),
    ).toHaveCount(0);

    const projectRow = testPage.getByTestId(`agent-project-row-${projectId}`);
    await projectRow.getByRole("button", { name: `Actions for ${projectName}` }).click();
    await testPage.getByRole("menuitem", { name: "Edit project" }).click();
    const editForm = testPage.getByTestId("agent-project-form-desktop");
    await editForm.getByTestId("agent-project-name").fill(`${projectName} edited`);
    await assertProjectFormGeometry(editForm, false);
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
    await expect(
      testPage.locator(`[data-testid="sidebar-task-item"][data-task-row-id="${worker!.id}"]`),
    ).toHaveCount(0);

    await coordinator.clickTab("Files");
    const fileRoots = testPage.getByTestId("agent-project-file-roots");
    await expect(fileRoots.getByRole("tab", { name: "Context" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await fileRoots.getByRole("button", { name: "index.md", exact: true }).click();
    const contextEditor = fileRoots.getByTestId("agent-project-context-editor");
    await expect(contextEditor).toHaveValue(/okf_version: "0\.2"/);
    await fileRoots.getByRole("button", { name: "Back to project context" }).click();
    await fileRoots.getByRole("button", { name: "notes.md", exact: true }).click();
    const contextDraft =
      "---\ntitle: Desktop draft\n---\nShared project context from the desktop E2E.\n";
    await contextEditor.fill(contextDraft);
    const formatFeedback = fileRoots.getByTestId("agent-project-context-format");
    await expect(formatFeedback).toContainText("Concept type is missing");
    await expect(formatFeedback).toContainText("You can still save this draft.");
    await expect(contextEditor).toBeFocused();
    await expect(fileRoots.getByRole("button", { name: "Save context" })).toBeEnabled();
    const contextSurface = fileRoots.getByTestId("agent-project-context");
    const [backBox, saveBox, contextBounds] = await Promise.all([
      contextSurface.getByRole("button", { name: "Back to project context" }).boundingBox(),
      contextSurface.getByRole("button", { name: "Save context" }).boundingBox(),
      contextSurface.evaluate((element) => {
        const surface = element.getBoundingClientRect();
        const editor = element
          .querySelector("[data-testid='agent-project-context-editor']")!
          .getBoundingClientRect();
        return {
          surfaceRight: surface.right,
          editorRight: editor.right,
          editorBottom: editor.bottom,
          viewportWidth: window.innerWidth,
          viewportHeight: window.innerHeight,
        };
      }),
    ]);
    expect(backBox).not.toBeNull();
    expect(saveBox).not.toBeNull();
    expect(Math.abs(backBox!.height - 28)).toBeLessThanOrEqual(1);
    expect(backBox!.width).toBeGreaterThanOrEqual(28);
    expect(backBox!.width).toBeLessThanOrEqual(32);
    expect(Math.abs(saveBox!.height - 28)).toBeLessThanOrEqual(1);
    expect(contextBounds.surfaceRight).toBeLessThanOrEqual(contextBounds.viewportWidth);
    expect(contextBounds.editorRight).toBeLessThanOrEqual(contextBounds.viewportWidth);
    expect(contextBounds.editorBottom).toBeLessThanOrEqual(contextBounds.viewportHeight);
    await testPage.screenshot({ path: test.info().outputPath("desktop-context-advice.png") });
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
      .toBe(contextDraft);

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
    const workerEditor = workerRoots.getByTestId("agent-project-context-editor");
    await expect(workerEditor).toHaveValue(contextDraft);
    const workerDraft = "---\ntype: Decision\n---\nWorker draft that must survive a conflict.\n";
    await workerEditor.fill(workerDraft);
    const contextFileUrl = `/api/v1/workspaces/${seedData.workspaceId}/agent-projects/${projectId}/context/content?path=notes.md`;
    const currentFileResponse = await apiClient.rawRequest("GET", contextFileUrl);
    expect(currentFileResponse.ok).toBeTruthy();
    const currentFile = (await currentFileResponse.json()) as { hash: string };
    const archivedNotes = "---\ntype: Reference\n---\nUpdated by another project task.\n";
    const concurrentWrite = await apiClient.rawRequest(
      "PUT",
      `/api/v1/workspaces/${seedData.workspaceId}/agent-projects/${projectId}/context/content`,
      {
        path: "notes.md",
        content: archivedNotes,
        expected_hash: currentFile.hash,
      },
    );
    expect(concurrentWrite.status).toBe(200);
    await workerRoots.getByRole("button", { name: "Save context" }).click();
    await expect(workerRoots.getByRole("alert")).toBeVisible();
    await expect(workerEditor).toHaveValue(workerDraft);
    await expect(workerRoots.getByRole("button", { name: "Save context" })).toBeEnabled();

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
    const sidebarProjectsHeader = sidebar.locator('[aria-controls="sidebar-section-projects"]');
    await expect(sidebarProjectsHeader).toHaveAttribute("aria-expanded", "false");
    await expect(sidebar.getByText("No projects yet")).not.toBeVisible();
    await expect(sidebar.getByTestId("agent-project-archived-toggle")).toHaveCount(0);
    await sidebarProjectsHeader.click();
    await expect(sidebarProjectsHeader).toHaveAttribute("aria-expanded", "true");
    await expect(sidebar.getByText("No projects yet")).toBeVisible();
    await restoreArchivedAgentProject(apiClient, seedData.workspaceId, projectId, archivedNotes);
    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects.length, {
        timeout: 30_000,
      })
      .toBe(1);

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

test("a nonblank initial prompt starts exactly one coordinator session", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(120_000);
  const fixtureName = `project-prompt-${Date.now()}`;
  const fixture = await setupAgentProjectFixture(apiClient, backend, seedData, fixtureName);
  let projectId = "";
  try {
    await testPage.setViewportSize({ width: 1280, height: 900 });
    await testPage.goto("/");
    await testPage.getByTestId("agent-project-create-open").click();
    const form = testPage.getByTestId("agent-project-form-desktop");
    await form.getByTestId("agent-project-name").fill("Initial prompt project");
    await chooseConfiguredRepository(form, fixture.repositoryId, `fixture/${fixtureName}`);
    await form
      .getByRole("combobox", { name: "Coordinator profile" })
      .selectOption(fixture.profileId);
    const prompt = "Inspect the project and report the starting state.";
    await form.getByTestId("agent-project-initial-prompt").fill(prompt);
    await form.getByTestId("agent-project-submit").click();

    await expect(form).toBeHidden();
    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects[0]?.id ?? "")
      .not.toBe("");
    const project = (await listProjects(apiClient, seedData.workspaceId)).projects[0]!;
    projectId = project.id;
    await expect(testPage).toHaveURL(new RegExp(`/t/${project.main_task_id}$`));
    const { sessions } = await apiClient.listTaskSessions(project.main_task_id);
    expect(sessions).toHaveLength(1);
    const sessionId = sessions[0]!.id;
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(sessionId);
          return messages.filter(
            (message) =>
              message.author_type === "user" &&
              message.content.endsWith(prompt) &&
              message.content.split(prompt).length - 1 === 1,
          ).length;
        },
        { timeout: 30_000 },
      )
      .toBeGreaterThan(0);
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(sessionId);
          return messages.some((message) => message.author_type === "agent");
        },
        { timeout: 90_000 },
      )
      .toBe(true);
    await waitForSessionDone(
      apiClient,
      project.main_task_id,
      sessionId,
      "Waiting for the initial coordinator prompt",
      90_000,
    );
    const { messages } = await apiClient.listSessionMessages(sessions[0]!.id);
    expect(
      messages.filter(
        (message) =>
          message.author_type === "user" &&
          message.content.endsWith(prompt) &&
          message.content.split(prompt).length - 1 === 1,
      ),
    ).toHaveLength(1);
  } finally {
    await fixture.cleanup(projectId || undefined);
  }
});

test("pasting an unlisted repository uses its verified default branch", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixtureName = `project-paste-${Date.now()}`;
  const fixture = await setupAgentProjectFixture(apiClient, backend, seedData, fixtureName);
  let projectId = "";
  let importedRepositoryId = "";
  try {
    await testPage.goto("/");
    await testPage.getByTestId("agent-project-create-open").click();
    const form = testPage.getByTestId("agent-project-form-desktop");
    await form.getByTestId("agent-project-name").fill("Unlisted repository project");
    await form.getByTestId("agent-project-add-repository").click();
    const pickerInput = testPage.getByTestId("remote-repo-input");
    await expect(pickerInput).toBeVisible();
    await expect(
      testPage.getByTestId("remote-repo-option").filter({ hasText: "fixture/unlisted-project" }),
    ).toHaveCount(0);

    await apiClient.mockGitHubAddRepos("fixture", [
      {
        full_name: "fixture/unlisted-project",
        owner: "fixture",
        name: "unlisted-project",
        private: true,
      },
    ]);
    await apiClient.mockGitHubAddRepositoryDetails([
      {
        full_name: "fixture/unlisted-project",
        owner: "fixture",
        name: "unlisted-project",
        clone_url: "https://github.com/fixture/unlisted-project.git",
        html_url: "https://github.com/fixture/unlisted-project",
        default_branch: "trunk",
      },
    ]);
    await pickerInput.fill("https://github.com/fixture/unlisted-project");
    await pickerInput.press("Enter");
    await expect(form.getByTestId("agent-project-repository-chip")).toContainText(
      "fixture/unlisted-project",
    );
    await form
      .getByRole("combobox", { name: "Primary repository" })
      .selectOption({ label: "fixture/unlisted-project" });
    await form
      .getByRole("combobox", { name: "Coordinator profile" })
      .selectOption(fixture.profileId);
    await form.getByTestId("agent-project-submit").click();

    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects[0]?.id ?? "")
      .not.toBe("");
    const project = (await listProjects(apiClient, seedData.workspaceId)).projects[0]!;
    projectId = project.id;
    const repositoriesResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${seedData.workspaceId}/repositories`,
    );
    expect(repositoriesResponse.ok).toBe(true);
    const repositories = (await repositoriesResponse.json()) as {
      repositories: Array<{ id: string; name: string; default_branch: string; remote_url: string }>;
    };
    const imported = repositories.repositories.find((repository) =>
      repository.name.includes("unlisted-project"),
    );
    expect(imported).toMatchObject({
      default_branch: "trunk",
      remote_url: "https://github.com/fixture/unlisted-project.git",
    });
    importedRepositoryId = imported!.id;
  } finally {
    await fixture.cleanup(projectId || undefined);
    if (importedRepositoryId) {
      await apiClient
        .rawRequest("DELETE", `/api/v1/repositories/${importedRepositoryId}`)
        .catch(() => undefined);
    }
  }
});

test("worker profiles inherit the coordinator until explicitly overridden", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixtureName = `project-profile-inheritance-${Date.now()}`;
  const fixture = await setupAgentProjectFixture(apiClient, backend, seedData, fixtureName);
  let projectId = "";
  let alternateProfileId = "";
  try {
    await testPage.setViewportSize({ width: 1280, height: 900 });
    const { agents } = await apiClient.listAgents();
    const mockAgent = agents.find((agent) => agent.name === "mock-agent")!;
    const alternate = await apiClient.createAgentProfile(
      mockAgent.id,
      "E2E inherited worker profile",
      { model: "mock-fast", cli_passthrough: false },
    );
    alternateProfileId = alternate.id;
    await testPage.goto("/");
    await testPage.getByTestId("agent-project-create-open").click();
    const form = testPage.getByTestId("agent-project-form-desktop");
    await form.getByTestId("agent-project-name").fill("Inherited worker profile project");
    await chooseConfiguredRepository(form, fixture.repositoryId, `fixture/${fixtureName}`);
    const coordinator = form.getByRole("combobox", { name: "Coordinator profile" });
    await coordinator.selectOption(fixture.profileId);
    const advanced = form.getByTestId("agent-project-advanced");
    await advanced.locator("summary").click();
    await assertProjectWorkerProfilesGeometry(form, false);
    await advanced.scrollIntoViewIfNeeded();
    await form.getByTestId("agent-project-frontier-profile-select").scrollIntoViewIfNeeded();
    await settleProjectSurface(form);
    await testPage.screenshot({
      path: test.info().outputPath("desktop-project-form-advanced.png"),
    });
    const economy = form.getByTestId("agent-project-economy-profile-select");
    await expect(economy.locator("option:checked")).toHaveText("Same as coordinator");

    await coordinator.selectOption(alternateProfileId);
    await expect(economy.locator("option:checked")).toHaveText("Same as coordinator");
    await economy.selectOption(alternateProfileId);
    await coordinator.selectOption(fixture.profileId);
    await expect(economy).toHaveValue(alternateProfileId);
    await form.getByTestId("agent-project-submit").click();

    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects[0]?.id ?? "")
      .not.toBe("");
    projectId = (await listProjects(apiClient, seedData.workspaceId)).projects[0]!.id;
    const detailResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${seedData.workspaceId}/agent-projects/${projectId}`,
    );
    expect(detailResponse.ok).toBe(true);
    const saved = (await detailResponse.json()) as {
      coordinator_profile_id: string;
      economy_profile_id: string;
    };
    expect(saved).toMatchObject({
      coordinator_profile_id: fixture.profileId,
      economy_profile_id: alternateProfileId,
    });
  } finally {
    await fixture.cleanup(projectId || undefined);
    if (alternateProfileId) await apiClient.deleteAgentProfile(alternateProfileId, true);
  }
});

test("project creation preserves a failed draft and adapts to narrow viewports", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixtureName = `project-draft-${Date.now()}`;
  const fixture = await setupAgentProjectFixture(apiClient, backend, seedData, fixtureName);
  await apiClient.mockGitHubAddRepos("fixture", [
    {
      full_name: "fixture/geometry-picker",
      owner: "fixture",
      name: "geometry-picker",
      private: true,
    },
  ]);
  let projectId = "";
  try {
    await testPage.goto("/");
    await testPage.getByTestId("agent-project-create-open").click();
    let form = testPage.getByTestId("agent-project-form-desktop");
    const name = "Saved after a temporary error";
    await form.getByTestId("agent-project-name").fill("   ");
    await expect(form.getByTestId("agent-project-submit")).toBeDisabled();
    await form.getByTestId("agent-project-name").fill(name);
    await chooseConfiguredRepository(form, fixture.repositoryId, `fixture/${fixtureName}`);
    await form
      .getByRole("combobox", { name: "Coordinator profile" })
      .selectOption(fixture.profileId);
    for (const width of [767, 393, 768]) {
      await testPage.setViewportSize({ width, height: 600 });
      form = testPage.getByTestId(
        width < 768 ? "agent-project-form-mobile" : "agent-project-form-desktop",
      );
      await expect(form).toBeVisible();
      await expect(form.getByTestId("agent-project-name")).toHaveValue(name);
      await assertProjectFormGeometry(form, width < 768);
      if (width === 393) {
        await testPage.screenshot({ path: test.info().outputPath("narrow-fine-project-form.png") });
      }
      const overflow = await testPage.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow).toBeLessThanOrEqual(1);
      if (width < 768) {
        const advanced = form.getByTestId("agent-project-advanced");
        await advanced.locator("summary").scrollIntoViewIfNeeded();
        await expect(advanced.locator("summary")).toBeInViewport();
        await expect(form.getByTestId("agent-project-submit")).toBeInViewport();
      }
    }
    await testPage.setViewportSize({ width: 1280, height: 900 });
    form = testPage.getByTestId("agent-project-form-desktop");
    await assertProjectRemotePickerGeometry(
      form,
      false,
      test.info().outputPath("desktop-project-repository-picker.png"),
    );
    const createRoute = `**/api/v1/workspaces/${seedData.workspaceId}/agent-projects`;
    await testPage.route(createRoute, async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "Temporary project creation failure" }),
      });
    });
    await form.getByTestId("agent-project-submit").click();
    await expect(form.getByRole("alert")).toContainText("Temporary project creation failure");
    await expect(form.getByTestId("agent-project-name")).toHaveValue(name);
    await expect(form.getByRole("combobox", { name: "Coordinator profile" })).toHaveValue(
      fixture.profileId,
    );
    await expect(form.getByTestId("agent-project-submit")).toBeEnabled();
    await testPage.unroute(createRoute);
    await form.getByTestId("agent-project-submit").click();
    await expect(form).toBeHidden();
    const projects = (await listProjects(apiClient, seedData.workspaceId)).projects;
    expect(projects).toHaveLength(1);
    projectId = projects[0]!.id;
    await testPage.reload();
    await expect(testPage.getByTestId(`agent-project-open-${projectId}`)).toContainText(name);
  } finally {
    await fixture.cleanup(projectId || undefined);
  }
});

test("tablet project forms keep touch controls reachable", async ({
  browser,
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await setupAgentProjectFixture(
    apiClient,
    backend,
    seedData,
    `project-tablet-${Date.now()}`,
  );
  const context = await browser.newContext({
    baseURL: backend.frontendUrl,
    viewport: { width: 820, height: 900 },
    hasTouch: true,
  });
  try {
    await testPage.goto("/");
    await context.addInitScript((port) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      window.__KANDEV_API_PORT = String(port);
    }, backend.port);
    const page = await context.newPage();
    await page.goto("/");
    await page.getByTestId("agent-project-create-open").tap();
    const form = page.getByTestId("agent-project-form-desktop");
    await expect(form).toBeVisible();
    await settleProjectSurface(form);
    for (const control of await form
      .locator(
        "input:not([type=checkbox]), select, textarea, footer button, [data-slot=dialog-close]",
      )
      .all()) {
      if (!(await control.isVisible())) continue;
      await assertProjectTouchTarget(control);
    }
    await assertProjectTouchTarget(form.getByTestId("agent-project-advanced").locator("summary"));
    await assertProjectRemotePickerGeometry(
      form,
      true,
      test.info().outputPath("tablet-project-repository-picker.png"),
    );
    const coordinatorHelp = form.getByTestId("agent-project-coordinator-help");
    await coordinatorHelp.tap();
    const coordinatorHelpDrawer = page.getByTestId("agent-project-coordinator-help-drawer");
    await expect(coordinatorHelpDrawer).toBeVisible();
    await settleProjectSurface(coordinatorHelpDrawer);
    await page.screenshot({ path: test.info().outputPath("tablet-project-help.png") });
    await page.keyboard.press("Escape");
    await expect(coordinatorHelp).toBeFocused();
    await expect(coordinatorHelpDrawer).toBeHidden();
    await settleProjectSurface(page.locator("body"));
    await page.screenshot({ path: test.info().outputPath("tablet-project-form.png") });
    await form.getByRole("button", { name: "Close", exact: true }).tap();
    await expect(form).toBeHidden();
  } finally {
    await context.close();
    await fixture.cleanup();
  }
});

test("coordinator sessions retain their profile after later project edits", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(120_000);
  const fixture = await setupAgentProjectFixture(
    apiClient,
    backend,
    seedData,
    `project-pin-${Date.now()}`,
  );
  let projectId = "";
  let alternateProfileId = "";
  try {
    const { agents } = await apiClient.listAgents();
    const mockAgent = agents.find((agent) => agent.name === "mock-agent")!;
    const alternate = await apiClient.createAgentProfile(
      mockAgent.id,
      "E2E project alternate coordinator",
      { model: "mock-fast", cli_passthrough: false },
    );
    alternateProfileId = alternate.id;
    const collection = `/api/v1/workspaces/${seedData.workspaceId}/agent-projects`;
    const created = await apiClient.rawRequest("POST", collection, {
      name: "Coordinator profile pin",
      repository_ids: [fixture.repositoryId],
      primary_repository_id: fixture.repositoryId,
      coordinator_profile_id: fixture.profileId,
      economy_profile_id: fixture.profileId,
      frontier_profile_id: fixture.profileId,
      request_key: `project-pin-${Date.now()}`,
    });
    expect(created.ok).toBe(true);
    const project = (await created.json()) as AgentProjectView & { revision: number };
    projectId = project.id;
    const configuredB = await apiClient.rawRequest("PATCH", `${collection}/${projectId}`, {
      revision: project.revision,
      coordinator_profile_id: alternateProfileId,
    });
    expect(configuredB.ok).toBe(true);
    await testPage.goto("/");
    await testPage.getByTestId(`agent-project-open-${projectId}`).click();
    const coordinator = new SessionPage(testPage);
    await coordinator.waitForLoad();
    const startButton = testPage.getByTestId("task-description-start-button");
    await expect(startButton).toBeVisible();
    const { sessions } = await apiClient.listTaskSessions(project.main_task_id);
    expect(sessions).toHaveLength(1);
    const pinnedSession = sessions[0]!;
    expect(pinnedSession.agent_profile_id).toBe(alternateProfileId);
    expect(pinnedSession.state).toBe("CREATED");
    await testPage
      .getByTestId(`agent-project-row-${projectId}`)
      .getByRole("button", { name: "Actions for Coordinator profile pin" })
      .click();
    await testPage.getByRole("menuitem", { name: "Edit project" }).click();
    const editForm = testPage.getByTestId("agent-project-form-desktop");
    await editForm.getByTestId("agent-project-advanced").locator("summary").click();
    await editForm
      .getByRole("combobox", { name: "Coordinator profile" })
      .selectOption(fixture.profileId);
    await editForm.getByTestId("agent-project-submit").click();
    await expect(editForm).toBeHidden();
    await startButton.click();
    await coordinator.sendMessage("/e2e:simple-message");
    await coordinator.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
    const started = await apiClient.listTaskSessions(project.main_task_id);
    expect(
      started.sessions.find((session) => session.id === pinnedSession.id)?.agent_profile_id,
    ).toBe(alternateProfileId);
    await coordinator.openNewSessionDialog();
    await coordinator.newSessionPromptInput().fill("/e2e:simple-message");
    await coordinator.newSessionStartButton().click();
    await waitForLatestSessionDone(
      apiClient,
      project.main_task_id,
      2,
      "Waiting for the new coordinator session",
      60_000,
    );
    const current = await apiClient.listTaskSessions(project.main_task_id);
    expect(
      current.sessions.find((session) => session.id !== pinnedSession.id)?.agent_profile_id,
    ).toBe(fixture.profileId);
  } finally {
    await fixture.cleanup(projectId || undefined);
    if (alternateProfileId) await apiClient.deleteAgentProfile(alternateProfileId, true);
  }
});

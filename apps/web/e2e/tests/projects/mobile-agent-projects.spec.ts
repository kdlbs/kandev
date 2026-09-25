import { test, expect } from "../../fixtures/test-base";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import {
  readAgentProject,
  setupAgentProjectFixture,
  type AgentProjectView,
} from "./agent-projects-fixture";

async function listProjects(apiClient: ApiClient, workspaceId: string) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/agent-projects`,
  );
  if (!response.ok) throw new Error(`Agent Project list failed (${response.status})`);
  return (await response.json()) as { projects: AgentProjectView[] };
}

async function openMobileProjects(testPage: import("@playwright/test").Page) {
  const trigger = testPage.getByTestId("app-nav-trigger");
  if (!(await testPage.getByTestId("app-nav-sheet").isVisible())) await trigger.tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  const section = menu.getByRole("button", { name: "Projects", exact: true });
  await expect(section).toBeVisible();
  if ((await section.getAttribute("aria-expanded")) !== "true") await section.tap();
  return menu;
}

test("phone navigation manages Agent Projects and opens shared context in coordinator and worker", async ({
  testPage,
  apiClient,
  backend,
  seedData,
  prCapture,
}) => {
  test.setTimeout(240_000);
  const fixtureName = `agent-project-mobile-${Date.now()}`;
  const fixture = await setupAgentProjectFixture(apiClient, backend, seedData, fixtureName);
  let projectId = "";
  let projectName = "Agent Project phone";
  try {
    await testPage.setViewportSize({ width: 393, height: 851 });
    await testPage.goto("/stats");
    let menu = await openMobileProjects(testPage);
    await menu.getByTestId("agent-project-create-open").tap();
    const form = testPage.getByTestId("agent-project-form-mobile");
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
    const formHeight = await form.evaluate((element) => element.getBoundingClientRect().height);
    expect(formHeight).toBeGreaterThan(600);
    await expect(form.getByTestId("agent-project-submit")).toBeEnabled();
    await form.getByTestId("agent-project-submit").tap();

    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects[0]?.id ?? "")
      .not.toBe("");
    let project = (await listProjects(apiClient, seedData.workspaceId)).projects[0]!;
    projectId = project.id;
    menu = await openMobileProjects(testPage);
    const projectRow = menu.getByTestId(`agent-project-row-${projectId}`);
    await expect(projectRow).toBeVisible();
    await projectRow.getByRole("button", { name: `Actions for ${projectName}` }).tap();
    await testPage.getByRole("menuitem", { name: "Edit project" }).tap();
    const editForm = testPage.getByTestId("agent-project-form-mobile");
    await editForm.getByTestId("agent-project-name").fill(`${projectName} edited`);
    await editForm.getByTestId("agent-project-submit").tap();
    projectName = `${projectName} edited`;
    await expect(menu.getByTestId(`agent-project-open-${projectId}`)).toContainText(projectName);

    project = await readAgentProject(apiClient, seedData.workspaceId, projectId);
    await menu.getByTestId(`agent-project-open-${projectId}`).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${project.main_task_id}$`));
    const coordinator = new SessionPage(testPage);
    await coordinator.waitForLoad();
    await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
    await testPage.getByTestId("mobile-sessions-pill").tap();
    await testPage.getByTestId("mobile-launch-session").tap();
    await coordinator.newSessionPromptInput().fill("/e2e:agent-project-create-worker");
    await coordinator.newSessionStartButton().tap();
    await waitForLatestSessionDone(
      apiClient,
      project.main_task_id,
      1,
      "Waiting for the phone coordinator to create its worker through MCP",
      120_000,
    );
    const coordinatorSessions = await apiClient.listTaskSessions(project.main_task_id);
    const coordinatorMessages = await apiClient.listSessionMessages(
      coordinatorSessions.sessions[0]!.id,
    );
    expect(coordinatorMessages.messages.map((message) => message.content).join("\n")).toContain(
      "The project worker was created.",
    );
    await testPage.getByTestId(`mobile-session-row-${coordinatorSessions.sessions[0]!.id}`).tap();
    await expect(testPage.getByTestId("mobile-launch-session")).toBeHidden();

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
      "Waiting for the phone project worker",
      120_000,
    );
    const completedProject = await readAgentProject(apiClient, seedData.workspaceId, projectId);
    const completedWorker = completedProject.tasks.find((task) => task.id === worker!.id);
    expect(completedWorker).toBeTruthy();
    await testPage.getByTestId("app-nav-trigger").tap();
    menu = await openMobileProjects(testPage);
    await menu.getByTestId(`agent-project-expand-${projectId}`).tap();
    const workerRow = menu.getByTestId(`agent-project-worker-${worker!.id}`);
    await expect(workerRow).toBeVisible();
    await expect(workerRow.getByText(completedWorker!.state, { exact: true })).toBeVisible();
    if (prCapture.capturing) {
      await testPage.evaluate(async () => {
        const animations = document
          .getAnimations()
          .filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity);
        await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
      });
    }
    await prCapture.screenshot("agent-project-mobile-worker-row", {
      caption: "Phone project navigation with a coordinator-created worker",
    });
    await testPage.reload();
    await coordinator.waitForLoad();

    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    const fileRoots = testPage.getByTestId("agent-project-file-roots");
    await expect(fileRoots.getByRole("tab", { name: "Context" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await fileRoots.getByRole("button", { name: "notes.md", exact: true }).tap();
    await fileRoots
      .getByTestId("agent-project-context-editor")
      .fill("Shared project context from the phone E2E.\n");
    await fileRoots.getByRole("button", { name: "Save context" }).tap();
    await expect
      .poll(async () => {
        const response = await apiClient.rawRequest(
          "GET",
          `/api/v1/workspaces/${seedData.workspaceId}/agent-projects/${projectId}/context/content?path=notes.md`,
        );
        if (!response.ok) return "";
        return ((await response.json()) as { content: string }).content;
      })
      .toContain("Shared project context from the phone E2E.");
    await fileRoots.getByRole("tab", { name: "Workspace" }).tap();
    await expect(fileRoots.getByRole("tab", { name: "Workspace" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await testPage.getByRole("button", { name: "Changes", exact: true }).tap();
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
    await expect(testPage.getByTestId("mobile-changes-panel")).not.toContainText("notes.md");

    await testPage.getByTestId("app-nav-trigger").tap();
    menu = await openMobileProjects(testPage);
    await menu.getByTestId(`agent-project-expand-${projectId}`).tap();
    await menu.getByTestId(`agent-project-worker-${worker!.id}`).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${worker!.id}$`));
    const workerSession = new SessionPage(testPage);
    await workerSession.waitForLoad();
    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    const workerRoots = testPage.getByTestId("agent-project-file-roots");
    await workerRoots.getByRole("tab", { name: "Context" }).tap();
    await workerRoots.getByRole("button", { name: "notes.md", exact: true }).tap();
    await expect(workerRoots.getByTestId("agent-project-context-editor")).toHaveValue(
      "Shared project context from the phone E2E.\n",
    );

    await testPage.getByTestId("app-nav-trigger").tap();
    menu = await openMobileProjects(testPage);
    const activeRow = menu.getByTestId(`agent-project-row-${projectId}`);
    await activeRow.getByRole("button", { name: `Actions for ${projectName}` }).tap();
    await testPage.getByRole("menuitem", { name: "Archive project" }).tap();
    await testPage.getByTestId("agent-project-archive-confirm").tap();
    await expect(testPage.getByTestId("agent-project-archive-confirmation")).toBeHidden({
      timeout: 30_000,
    });
    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects.length, {
        timeout: 30_000,
      })
      .toBe(0);
    menu = await openMobileProjects(testPage);
    await expect(menu.getByText("No projects yet")).toBeVisible();
    await menu.getByTestId("agent-project-archived-toggle").tap();
    const archivedRow = menu.getByTestId(`agent-project-row-${projectId}`);
    await expect(archivedRow).toBeVisible();
    await archivedRow.getByRole("button", { name: `Restore ${projectName}` }).tap();
    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects.length, {
        timeout: 30_000,
      })
      .toBe(1);
    await menu.getByTestId("agent-project-archived-toggle").tap();

    const restoredRow = menu.getByTestId(`agent-project-row-${projectId}`);
    await expect(restoredRow).toBeVisible({ timeout: 30_000 });
    await restoredRow.getByRole("button", { name: `Actions for ${projectName}` }).tap();
    await testPage.getByRole("menuitem", { name: "Delete project" }).tap();
    const confirmation = testPage.getByTestId("agent-project-delete-confirmation");
    await expect(confirmation).toBeVisible();
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
    await confirmation.getByTestId("agent-project-delete-confirm").tap();
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
    const viewport = await testPage.evaluate(() => ({
      documentWidth: document.documentElement.scrollWidth,
      viewportWidth: window.innerWidth,
    }));
    expect(viewport.documentWidth).toBeLessThanOrEqual(viewport.viewportWidth + 1);
    projectId = "";
  } finally {
    await fixture.cleanup(projectId || undefined);
  }
});

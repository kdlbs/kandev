import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import {
  assertFullHeightProjectSurface,
  assertProjectFormGeometry,
  assertProjectTouchTarget,
  assertProjectWorkerProfilesGeometry,
  settleProjectSurface,
} from "./agent-projects-geometry";
import {
  readAgentProject,
  restoreArchivedAgentProject,
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

async function openMobileProjects(testPage: import("@playwright/test").Page, expand = true) {
  const trigger = testPage.getByTestId("app-nav-trigger");
  if (!(await testPage.getByTestId("app-nav-sheet").isVisible())) await trigger.tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  const section = menu.getByRole("button", { name: "Projects", exact: true });
  await expect(section).toBeVisible();
  if (expand && (await section.getAttribute("aria-expanded")) !== "true") await section.tap();
  return menu;
}

test("phone project drafts survive portrait and landscape resize", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await setupAgentProjectFixture(
    apiClient,
    backend,
    seedData,
    `project-rotation-${Date.now()}`,
  );
  try {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/stats");
    const menu = await openMobileProjects(testPage);
    await menu.getByTestId("agent-project-create-open").tap();
    let form = testPage.getByTestId("agent-project-form-mobile");
    await form.getByTestId("agent-project-name").fill("Rotation keeps this draft");
    await form.getByTestId("agent-project-initial-prompt").fill("Keep the exact prompt text.");

    for (const viewport of [
      { width: 844, height: 390 },
      { width: 390, height: 844 },
    ]) {
      await testPage.setViewportSize(viewport);
      form = testPage.getByTestId(
        viewport.width < 768 ? "agent-project-form-mobile" : "agent-project-form-desktop",
      );
      await expect(form).toBeVisible();
      await expect(form.getByTestId("agent-project-name")).toHaveValue("Rotation keeps this draft");
      await expect(form.getByTestId("agent-project-initial-prompt")).toHaveValue(
        "Keep the exact prompt text.",
      );
    }

    await testPage.keyboard.press("Escape");
    await expect(testPage.getByTestId("agent-project-form-mobile")).toBeHidden();
  } finally {
    await fixture.cleanup();
  }
});

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
    let menu = await openMobileProjects(testPage, false);
    await expect(menu.locator('[aria-controls="sidebar-section-projects"]')).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expect(menu.getByText("No projects yet")).not.toBeVisible();
    const createButton = menu.getByTestId("agent-project-create-open");
    await assertProjectTouchTarget(createButton);
    await createButton.tap();
    const form = testPage.getByTestId("agent-project-form-mobile");
    await expect(form).toBeVisible();
    await assertProjectFormGeometry(form, true);
    await testPage.screenshot({ path: test.info().outputPath("phone-project-form.png") });
    await assertProjectWorkerProfilesGeometry(form, true);
    await settleProjectSurface(form);
    await testPage.screenshot({ path: test.info().outputPath("phone-project-form-advanced.png") });
    const projectHelp = form.getByTestId("agent-project-project-help");
    await projectHelp.tap();
    const projectHelpDrawer = testPage.getByTestId("agent-project-project-help-drawer");
    await expect(projectHelpDrawer).toBeVisible();
    await settleProjectSurface(projectHelpDrawer);
    await testPage.screenshot({ path: test.info().outputPath("phone-project-help.png") });
    await testPage.keyboard.press("Escape");
    await expect(projectHelp).toBeFocused();
    await form.getByTestId("agent-project-name").fill(projectName);
    await form.getByTestId("agent-project-add-repository").tap();
    await testPage
      .getByTestId("agent-project-existing-repository-option")
      .filter({ hasText: `fixture/${fixtureName}` })
      .tap();
    await form
      .getByRole("combobox", { name: "Primary repository" })
      .selectOption(fixture.repositoryId);
    await form
      .getByRole("combobox", { name: "Coordinator profile" })
      .selectOption(fixture.profileId);
    const formHeight = await form.evaluate((element) => element.getBoundingClientRect().height);
    expect(formHeight).toBeGreaterThan(600);
    await expect(form.getByTestId("agent-project-submit")).toBeEnabled();
    await form.getByTestId("agent-project-submit").tap();

    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects[0]?.id ?? "")
      .not.toBe("");
    let project = (await listProjects(apiClient, seedData.workspaceId)).projects[0]!;
    projectId = project.id;
    expect((await apiClient.listTaskSessions(project.main_task_id)).sessions).toHaveLength(0);
    const contextRoot = path.join(
      backend.tmpDir,
      ".kandev",
      "agent-projects",
      projectId,
      "context",
    );
    await mkdir(path.join(contextRoot, "docs"), { recursive: true });
    await writeFile(path.join(contextRoot, "docs", "guide.md"), "# Guide\n", "utf8");
    menu = await openMobileProjects(testPage);
    const projectRow = menu.getByTestId(`agent-project-row-${projectId}`);
    await expect(projectRow).toBeVisible();
    await settleProjectSurface(menu);
    await testPage.screenshot({ path: test.info().outputPath("phone-project-sidebar.png") });
    await expect(menu.locator(`[data-task-row-id="${project.main_task_id}"]`)).toHaveCount(0);
    await assertProjectTouchTarget(menu.getByTestId(`agent-project-open-${projectId}`));
    await assertProjectTouchTarget(menu.getByRole("button", { name: "Projects", exact: true }));
    await projectRow.getByRole("button", { name: `Actions for ${projectName}` }).tap();
    const editProjectMenuItem = testPage.getByRole("menuitem", { name: "Edit project" });
    await expect(editProjectMenuItem).toBeVisible();
    await editProjectMenuItem.tap();
    const editForm = testPage.getByTestId("agent-project-form-mobile");
    await editForm.getByTestId("agent-project-name").fill(`${projectName} edited`);
    await testPage.setViewportSize({ width: 844, height: 390 });
    const landscapeEditForm = testPage.getByTestId("agent-project-form-desktop");
    await expect(landscapeEditForm).toBeVisible();
    await expect(landscapeEditForm.getByTestId("agent-project-name")).toHaveValue(
      `${projectName} edited`,
    );
    await testPage.setViewportSize({ width: 390, height: 844 });
    await expect(editForm).toBeVisible();
    await expect(editForm.getByTestId("agent-project-name")).toHaveValue(`${projectName} edited`);
    await editForm.getByTestId("agent-project-submit").tap();
    projectName = `${projectName} edited`;
    await expect(menu.getByTestId(`agent-project-open-${projectId}`)).toContainText(projectName);

    project = await readAgentProject(apiClient, seedData.workspaceId, projectId);
    await menu.getByTestId(`agent-project-open-${projectId}`).tap({ position: { x: 16, y: 40 } });
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
    const workerSessions = await apiClient.listTaskSessions(worker!.id);
    const workerSessionRecord = workerSessions.sessions[0];
    expect(workerSessionRecord).toBeTruthy();
    const workerMessages = await apiClient.listSessionMessages(workerSessionRecord!.id);
    const firstAgentMessage = workerMessages.messages.find(
      (message) => message.author_type === "agent",
    );
    expect(firstAgentMessage).toBeTruthy();
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
    await fileRoots.getByRole("button", { name: "docs", exact: true }).tap();
    const directoryBack = fileRoots.getByRole("button", {
      name: "Go to parent context folder",
    });
    await assertProjectTouchTarget(directoryBack);
    const directoryBackBox = await directoryBack.boundingBox();
    expect(directoryBackBox).not.toBeNull();
    expect(directoryBackBox!.width).toBeGreaterThanOrEqual(44);
    await directoryBack.tap();
    await fileRoots.getByRole("button", { name: "notes.md", exact: true }).tap();
    const contextEditor = fileRoots.getByTestId("agent-project-context-editor");
    const contextDraft =
      "---\ntitle: Phone draft\n---\nShared project context from the phone E2E.\n";
    await contextEditor.fill(contextDraft);
    const formatFeedback = fileRoots.getByTestId("agent-project-context-format");
    await expect(formatFeedback).toContainText("Concept type is missing");
    await expect(formatFeedback).toContainText("You can still save this draft.");
    await expect(contextEditor).toBeFocused();
    const fileBack = fileRoots.getByRole("button", { name: "Back to project context" });
    await assertProjectTouchTarget(fileBack);
    const fileBackBox = await fileBack.boundingBox();
    expect(fileBackBox).not.toBeNull();
    expect(fileBackBox!.width).toBeGreaterThanOrEqual(44);
    await assertProjectTouchTarget(fileRoots.getByRole("button", { name: "Save context" }));
    const mobileContextBounds = await fileRoots.evaluate((element) => {
      const surface = element.getBoundingClientRect();
      const editor = element
        .querySelector("[data-testid='agent-project-context-editor']")!
        .getBoundingClientRect();
      return {
        surfaceRight: surface.right,
        editorRight: editor.right,
        viewportWidth: window.innerWidth,
        documentWidth: document.documentElement.scrollWidth,
      };
    });
    expect(mobileContextBounds.surfaceRight).toBeLessThanOrEqual(mobileContextBounds.viewportWidth);
    expect(mobileContextBounds.editorRight).toBeLessThanOrEqual(mobileContextBounds.viewportWidth);
    expect(mobileContextBounds.documentWidth).toBeLessThanOrEqual(
      mobileContextBounds.viewportWidth,
    );
    await testPage.screenshot({ path: test.info().outputPath("phone-context-advice.png") });
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
      .toBe(contextDraft);
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
    await expect(workerRoots.getByTestId("agent-project-context-editor")).toHaveValue(contextDraft);

    await testPage.getByTestId("app-nav-trigger").tap();
    menu = await openMobileProjects(testPage);
    const activeRow = menu.getByTestId(`agent-project-row-${projectId}`);
    await activeRow.getByRole("button", { name: `Actions for ${projectName}` }).tap();
    await testPage.getByRole("menuitem", { name: "Archive project" }).tap();
    await assertFullHeightProjectSurface(
      testPage.getByTestId("agent-project-archive-confirmation"),
    );
    const coordinatorSnapshot = await apiClient.listTaskSessions(project.main_task_id);
    const workerSnapshot = await apiClient.listTaskSessions(worker!.id);
    const archiveSnapshotPath = test.info().outputPath("archive-session-snapshot.json");
    await writeFile(
      archiveSnapshotPath,
      JSON.stringify(
        {
          coordinator: coordinatorSnapshot.sessions.map(({ id, state }) => ({ id, state })),
          worker: workerSnapshot.sessions.map(({ id, state }) => ({ id, state })),
        },
        null,
        2,
      ),
      "utf8",
    );
    await test.info().attach("archive-session-snapshot", {
      path: archiveSnapshotPath,
      contentType: "application/json",
    });
    const archiveResponsePromise = testPage.waitForResponse(
      (response) =>
        response.url().includes(`/agent-projects/${projectId}/archive`) &&
        response.request().method() === "POST",
      { timeout: 30_000 },
    );
    const archiveStartedAt = Date.now();
    await testPage.getByTestId("agent-project-archive-confirm").tap();
    const archiveResponse = await archiveResponsePromise;
    expect(archiveResponse.status()).toBe(200);
    await test.info().attach("archive-response-timing", {
      body: JSON.stringify({
        status: archiveResponse.status(),
        elapsedMs: Date.now() - archiveStartedAt,
      }),
      contentType: "application/json",
    });
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
    await expect(menu.getByTestId("agent-project-archived-toggle")).toHaveCount(0);
    await restoreArchivedAgentProject(apiClient, seedData.workspaceId, projectId, contextDraft);
    await expect
      .poll(async () => (await listProjects(apiClient, seedData.workspaceId)).projects.length, {
        timeout: 30_000,
      })
      .toBe(1);

    const restoredRow = menu.getByTestId(`agent-project-row-${projectId}`);
    await expect(restoredRow).toBeVisible({ timeout: 30_000 });
    await restoredRow.getByRole("button", { name: `Actions for ${projectName}` }).tap();
    await testPage.getByRole("menuitem", { name: "Delete project" }).tap();
    const confirmation = testPage.getByTestId("agent-project-delete-confirmation");
    await expect(confirmation).toBeVisible();
    await assertFullHeightProjectSurface(confirmation);
    await testPage.screenshot({ path: test.info().outputPath("phone-project-delete.png") });
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

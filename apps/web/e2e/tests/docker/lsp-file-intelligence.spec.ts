import { test, expect } from "../../fixtures/docker-test-base";
import {
  attachLspDidOpenCapture,
  createKotlinTask,
  expectedMonacoModelUri,
  expectFakeLspMarkerCount,
  openDesktopFile,
  readSessionModelSnapshots,
  removeFakeKotlinLsp,
} from "../lsp/lsp-e2e-helpers";

test.describe("Docker task-host LSP", () => {
  test.describe.configure({ timeout: 180_000 });

  test("runs kotlin-lsp from inside the task container", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    removeFakeKotlinLsp(backend);
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Docker Kotlin LSP",
      executorProfileId: seedData.dockerExecutorProfileId,
      repositoryDirectory: "e2e-docker-repo",
      push: true,
    });
    await openDesktopFile(testPage, task.session, task.filePaths[0]);

    const statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await expect(statusButton).toHaveAttribute("data-lsp-language", "kotlin");
    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 30_000 });
    await expectFakeLspMarkerCount(testPage, 1, 30_000);

    const editor = testPage.locator(".monaco-editor:visible");
    await editor.click();
    await testPage.keyboard.press("Control+Space");
    await expect(testPage.locator(".suggest-widget")).toContainText("fakeGreeting", {
      timeout: 15_000,
    });
    await testPage.keyboard.press("Escape");
    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "disabled");
  });

  test("isolates Monaco models for two container sessions sharing /workspace", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const didOpenFrames = attachLspDidOpenCapture(testPage);
    const filePath = "SharedSessionModel.kt";
    const firstContent = [
      "package collision",
      "",
      'const val SESSION_CONTENT = "FIRST_CONTAINER_CONTENT"',
      "",
    ].join("\n");
    const secondContent = [
      "package collision",
      "",
      'const val SESSION_CONTENT = "SECOND_CONTAINER_CONTENT"',
      "",
    ].join("\n");

    const first = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Docker LSP Model Collision A",
      executorProfileId: seedData.dockerExecutorProfileId,
      repositoryDirectory: "e2e-docker-repo",
      filePaths: [filePath],
      fileContents: [firstContent],
      push: true,
    });
    await openDesktopFile(testPage, first.session, filePath);
    let statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 30_000 });
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).toContainText(
      "FIRST_CONTAINER_CONTENT",
    );
    await expect
      .poll(() => didOpenFrames.find((frame) => frame.sessionId === first.sessionId)?.uri ?? null)
      .not.toBeNull();
    const firstDidOpen = didOpenFrames.find((frame) => frame.sessionId === first.sessionId)!;
    expect(firstDidOpen.uri).toBe(`file:///workspace/${filePath}`);
    expect(new URL(firstDidOpen.uri).search).toBe("");
    expect(firstDidOpen.text).toContain("FIRST_CONTAINER_CONTENT");

    const secondTitle = "Docker LSP Model Collision B";
    const second = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: secondTitle,
      executorProfileId: seedData.dockerExecutorProfileId,
      repositoryDirectory: "e2e-docker-repo",
      filePaths: [filePath],
      fileContents: [secondContent],
      push: true,
      navigate: false,
    });
    await expect(first.session.taskInSidebar(secondTitle)).toBeVisible({ timeout: 15_000 });
    await first.session.clickTaskInSidebar(secondTitle);
    await expect(testPage).toHaveURL((url) => url.pathname.includes(`/t/${second.taskId}`), {
      timeout: 15_000,
    });
    await second.session.waitForLoad(45_000);
    await second.session.waitForChatIdle({ timeout: 45_000 });
    await openDesktopFile(testPage, second.session, filePath);
    statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 30_000 });
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).toContainText(
      "SECOND_CONTAINER_CONTENT",
    );
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).not.toContainText(
      "FIRST_CONTAINER_CONTENT",
    );

    await expect
      .poll(() => didOpenFrames.find((frame) => frame.sessionId === second.sessionId)?.uri ?? null)
      .toBe(firstDidOpen.uri);
    const secondDidOpen = didOpenFrames.find((frame) => frame.sessionId === second.sessionId)!;
    expect(new URL(secondDidOpen.uri).search).toBe("");
    expect(secondDidOpen.text).toContain("SECOND_CONTAINER_CONTENT");

    const sharedDocumentUri = firstDidOpen.uri;
    const firstModelUri = expectedMonacoModelUri(sharedDocumentUri, first.sessionId);
    const secondModelUri = expectedMonacoModelUri(sharedDocumentUri, second.sessionId);
    await expect
      .poll(async () => {
        const models = await readSessionModelSnapshots(testPage, filePath);
        return models.filter((model) => [firstModelUri, secondModelUri].includes(model.uri)).length;
      })
      .toBe(2);
    const models = await readSessionModelSnapshots(testPage, filePath);
    expect(models).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          uri: firstModelUri,
          search: "",
          hash: "",
          content: firstContent,
        }),
        expect.objectContaining({
          uri: secondModelUri,
          search: "",
          hash: "",
          content: secondContent,
        }),
      ]),
    );
    expect(new URL(firstModelUri).pathname).toMatch(
      /^\/__kandev_session_model__\/s-[0-9a-f]+\/l\/workspace\/.*\.kt$/,
    );
    expect(new URL(secondModelUri).pathname).toMatch(
      /^\/__kandev_session_model__\/s-[0-9a-f]+\/l\/workspace\/.*\.kt$/,
    );
  });

  test("keeps edited content when LSP stops and persists it after reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const filePath = "WorkspaceIdentityStop.kt";
    const initialContent = [
      "package lifecycle",
      "",
      'const val INITIAL_CONTENT = "BEFORE_LSP_STOP"',
      "",
    ].join("\n");
    const editMarker = `STICKY_WORKSPACE_EDIT_${Date.now()}`;
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Docker LSP Sticky Workspace Identity",
      executorProfileId: seedData.dockerExecutorProfileId,
      repositoryDirectory: "e2e-docker-repo",
      filePaths: [filePath],
      fileContents: [initialContent],
      push: true,
    });
    await openDesktopFile(testPage, task.session, filePath);

    const activeModelUri = () =>
      testPage.evaluate(() => {
        const monaco = (
          window as typeof window & {
            monaco?: {
              editor: {
                getEditors: () => Array<{
                  getModel: () => { uri: { toString: () => string } } | null;
                }>;
              };
            };
          }
        ).monaco;
        return monaco?.editor
          .getEditors()
          .find((editor) => editor.getModel())
          ?.getModel()
          ?.uri.toString();
      });
    const statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 30_000 });

    const authoritativeModelUri = expectedMonacoModelUri(
      `file:///workspace/${filePath}`,
      task.sessionId,
    );
    await expect.poll(activeModelUri).toBe(authoritativeModelUri);

    const editorContent = testPage.locator(".monaco-editor:visible .view-lines");
    await editorContent.click();
    await testPage.keyboard.press("Control+End");
    await testPage.keyboard.insertText(`\n// ${editMarker}`);
    await expect(editorContent).toContainText(editMarker);

    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "disabled");
    await expect(editorContent).toContainText(editMarker);
    await expect.poll(activeModelUri).toBe(authoritativeModelUri);

    const saveButton = testPage.getByRole("button", { name: /^Save\b/ }).last();
    await expect(saveButton).toBeEnabled();
    await saveButton.click();
    await expect(saveButton).toBeDisabled({ timeout: 15_000 });

    await testPage.reload();
    await task.session.showSessionContext(45_000);
    await task.session.waitForChatIdle({ timeout: 45_000 });
    await openDesktopFile(testPage, task.session, filePath);
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).toContainText(editMarker, {
      timeout: 15_000,
    });
  });
});

import { test, expect } from "../../fixtures/test-base";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import { pathToFileURL } from "node:url";
import { makeGitEnv } from "../../helpers/git-helper";
import {
  assertLocatorWithinViewportX,
  assertNoElementHorizontalOverflow,
} from "../../helpers/layout-assertions";
import {
  createKotlinTask,
  expectTaskLanguageDetected,
  expectedMonacoModelUri,
  expectFakeLspEvent,
  expectFakeLspEventCount,
  expectFakeLspProcessStopped,
  expectFakeLspMarkerCount,
  expectFakeLspMarkerMessages,
  installAdditionalFakeLspBinary,
  installFakeKotlinLsp,
  LONG_LSP_PROGRESS_MESSAGE,
  openDesktopFile,
  openLspStatus,
  performLspAction,
  readFakeLspEvents,
  releaseFakeLspInitialization,
  removeFakeKotlinLsp,
} from "./lsp-e2e-helpers";

const EDITORS_SETTINGS_PATH = "/settings/preferences/terminal-editors";
const RESERVED_SOURCE_PATH = "Main # query? 100%.kt";
const DEFINITION_PARENT_PATH = "nested/references";
const DEFINITION_TARGET_PATH = `${DEFINITION_PARENT_PATH}/Definition Target # query? 100%.kt`;

function isProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ESRCH") return false;
    throw error;
  }
}

test.describe("LSP file intelligence", () => {
  test.describe.configure({ timeout: 90_000 });

  test("persists Kotlin auto-start from Editors settings", async ({ testPage, apiClient }) => {
    const initial = await apiClient.getUserSettings();
    const initialAutoStart = Array.isArray(initial.settings.lsp_auto_start_languages)
      ? (initial.settings.lsp_auto_start_languages as string[])
      : [];
    const initialAutoInstall = Array.isArray(initial.settings.lsp_auto_install_languages)
      ? (initial.settings.lsp_auto_install_languages as string[])
      : [];
    const initialConfigs =
      typeof initial.settings.lsp_server_configs === "object" &&
      initial.settings.lsp_server_configs !== null
        ? initial.settings.lsp_server_configs
        : {};

    try {
      await testPage.goto(EDITORS_SETTINGS_PATH);
      await expect(testPage.getByRole("heading", { name: "Editors", exact: true })).toBeVisible();

      const kotlinCard = testPage.getByTestId("lsp-language-card-kotlin");
      await expect(kotlinCard).toBeVisible({ timeout: 20_000 });
      await expect(kotlinCard).toContainText("Kotlin (experimental)");
      await expect(kotlinCard).toContainText("Manual install required");
      await expect(kotlinCard.getByTestId("lsp-auto-install-kotlin")).toHaveCount(0);
      await expect(
        testPage.getByTestId("lsp-language-card-rust").getByTestId("lsp-auto-install-rust"),
      ).toBeVisible();

      const autoStart = kotlinCard.getByTestId("lsp-auto-start-kotlin");
      const shouldEnable = !initialAutoStart.includes("kotlin");
      if ((await autoStart.isChecked()) !== shouldEnable) await autoStart.click();

      await expect(kotlinCard).toHaveAttribute("data-settings-dirty", "true");
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await expect(floatingSave).toBeVisible();
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });

      const persisted = await apiClient.getUserSettings();
      const persistedAutoStart = persisted.settings.lsp_auto_start_languages as string[];
      expect(persistedAutoStart.includes("kotlin")).toBe(shouldEnable);

      await testPage.reload();
      await expect(testPage.getByTestId("lsp-auto-start-kotlin")).toBeChecked({
        checked: shouldEnable,
      });
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_auto_start_languages: initialAutoStart,
        lsp_auto_install_languages: initialAutoInstall,
        lsp_server_configs: initialConfigs,
      });
    }
  });

  test("surfaces task-host installation guidance when kotlin-lsp is missing", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    removeFakeKotlinLsp(backend);
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Kotlin LSP Missing Binary",
    });
    await openDesktopFile(testPage, task.session, task.filePaths[0]);

    const statusButton = testPage.getByTestId("lsp-status-button");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "detected");
    await performLspAction(testPage, "start");

    await expect(statusButton).toHaveAttribute("data-lsp-state", "error", {
      timeout: 15_000,
    });
    const errorRow = await openLspStatus(testPage);
    await expect(errorRow).toContainText("Error");
    await expect(errorRow).toContainText("Install kotlin-lsp on the task host");
    await expect(testPage.getByText(/Enable auto-install/)).toHaveCount(0);
  });

  test("shows server-reported project work without claiming full indexing", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend, {
      progress: {
        title: "Importing Kotlin project",
        beginPercentage: 5,
        message: LONG_LSP_PROGRESS_MESSAGE,
        percentage: 42,
        endMessage: "Project model loaded",
      },
    });
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Kotlin LSP Project Progress",
    });
    await openDesktopFile(testPage, task.session, task.filePaths[0]);

    const statusButton = testPage.getByTestId("lsp-status-button");
    await performLspAction(testPage, "start");
    const surface = await openLspStatus(testPage);
    const projectProgress = surface.getByTestId("task-lsp-progress");
    await expect(projectProgress).toHaveAttribute("data-lsp-progress-kind", "active", {
      timeout: 15_000,
    });
    await expect(statusButton).toHaveAttribute("data-lsp-state", "server_work");
    await expect(projectProgress).toContainText("Importing Kotlin project");
    const progressMessage = projectProgress.getByText(LONG_LSP_PROGRESS_MESSAGE, { exact: true });
    await expect(progressMessage).toBeVisible();
    await expect(projectProgress).toContainText("Cross-file definitions and references");
    await expect(projectProgress.getByTestId("lsp-work-progress-bar")).toHaveAttribute(
      "aria-valuenow",
      "42",
    );
    await expect(projectProgress).toContainText(/Elapsed \d+ sec/);
    await expect(
      surface.locator('[data-testid="lsp-lifecycle-action"][data-lsp-action="stop"]'),
    ).toBeEnabled();
    await assertNoElementHorizontalOverflow(progressMessage, "desktop toolbar LSP progress text");
    await assertNoElementHorizontalOverflow(surface, "desktop toolbar LSP progress popover");
    await assertLocatorWithinViewportX(surface, "desktop toolbar LSP progress popover");

    releaseFakeLspInitialization(backend);
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    const completed = surface.getByTestId("task-lsp-completed-work");
    await expect(completed).toHaveAttribute("data-lsp-progress-kind", "completed");
    await expect(completed).toContainText("Server-reported work finished");
    await expect(completed).toContainText("Project model loaded");
    await expect(completed).toContainText("not full project indexing");
    await expect(completed).not.toContainText(/fully indexed/i);

    await performLspAction(testPage, "stop");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "stopped");
  });

  test("shows initialization timing and an honest no-report fallback", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend, { holdInitialize: true });
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Kotlin LSP Initialization Progress",
    });
    await openDesktopFile(testPage, task.session, task.filePaths[0]);

    const statusButton = testPage.getByTestId("lsp-status-button");
    await performLspAction(testPage, "start");
    const surface = await openLspStatus(testPage);
    const projectProgress = surface.getByTestId("task-lsp-initialization");
    await expect(projectProgress).toHaveAttribute(
      "data-lsp-initialization-stage",
      "initialize_pending",
    );
    await expect(projectProgress).toContainText("Server process started");
    await expect(projectProgress).toContainText(
      "Waiting for the language server to respond to the LSP initialize request.",
    );
    await expect(projectProgress).toContainText(/Elapsed \d+ sec/);
    await expect(projectProgress.getByTestId("lsp-work-progress-bar")).toHaveCount(0);
    await expect(projectProgress).not.toContainText(/ETA|time remaining/i);
    await expect(
      surface.locator('[data-testid="lsp-lifecycle-action"][data-lsp-action="stop"]'),
    ).toBeEnabled();

    await testPage.clock.setFixedTime(Date.now() + 61_000);
    await expect(projectProgress).toHaveAttribute("data-lsp-initialization-stage", "long_running", {
      timeout: 5_000,
    });
    await expect(projectProgress).toContainText("Initialization is taking longer than usual");
    await expect(projectProgress).toContainText("Kotlin LSP may be importing the Gradle project");
    await expect(projectProgress).toContainText(
      "Cross-file features remain unavailable until initialization completes.",
    );
    await expect(projectProgress.getByTestId("lsp-work-progress-bar")).toHaveCount(0);
    await expect(projectProgress).not.toContainText(/ETA|time remaining|\d+%/i);
    await expect(statusButton).toHaveAttribute("data-lsp-state", "initializing");
    await expect(
      surface.locator('[data-testid="lsp-lifecycle-action"][data-lsp-action="stop"]'),
    ).toBeEnabled();

    releaseFakeLspInitialization(backend);
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    const idle = surface.getByTestId("task-lsp-idle");
    await expect(idle).toHaveAttribute("data-lsp-progress-kind", "idle");
    await expect(idle).toContainText("No background work reported");
    await expect(idle).toContainText("Cross-file results may still warm up");
    await expect(idle.getByTestId("lsp-work-progress-bar")).toHaveCount(0);

    await performLspAction(testPage, "stop");
  });

  test("ignores legacy placement and keeps task status independent of the active editor", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialLocation =
      initial.settings.lsp_status_location === "status_bar" ? "status_bar" : "toolbar";
    const initialStatusBarEnabled = initial.settings.app_status_bar_enabled === true;
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      app_status_bar_enabled: true,
      lsp_status_location: "toolbar",
    });

    try {
      await testPage.goto(EDITORS_SETTINGS_PATH);
      await expect(testPage.getByRole("heading", { name: "Editors", exact: true })).toBeVisible();

      await expect(testPage.getByRole("radiogroup", { name: "LSP status location" })).toHaveCount(
        0,
      );
      expect((await apiClient.getUserSettings()).settings.lsp_status_location).toBe("toolbar");

      const repositoryName = `lsp-status-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const repositoryDirectory = path.join(backend.tmpDir, "repos", repositoryName);
      const gitEnv = makeGitEnv(backend.tmpDir);
      fs.mkdirSync(repositoryDirectory, { recursive: true });
      execSync("git init -b main", { cwd: repositoryDirectory, env: gitEnv });
      fs.writeFileSync(path.join(repositoryDirectory, "fixture.txt"), "isolated LSP status repo\n");
      execSync("git add fixture.txt", { cwd: repositoryDirectory, env: gitEnv });
      execSync('git commit -m "init isolated LSP status fixture"', {
        cwd: repositoryDirectory,
        env: gitEnv,
      });
      const repository = await apiClient.createRepository(
        seedData.workspaceId,
        repositoryDirectory,
        "main",
        { name: repositoryName },
      );

      installFakeKotlinLsp(backend, {
        progress: {
          title: "Importing Kotlin project",
          message: LONG_LSP_PROGRESS_MESSAGE,
          percentage: 42,
          endMessage: "Status-bar project model loaded",
        },
      });
      const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
        title: "Kotlin LSP Status Bar Placement",
        filePaths: ["Main.kt", "README.md", "Binary.kt"],
        fileContents: [
          'package e2e\n\nfun main() = println("status")\n',
          "# Unsupported editor\n",
          Buffer.from([0x00, 0x01, 0x02, 0xff]),
        ],
        repositoryDirectory: repositoryName,
        repositoryIds: [repository.id],
      });
      await expectTaskLanguageDetected(apiClient, task.taskId, "kotlin");
      await openDesktopFile(testPage, task.session, task.filePaths[0]);

      await expect(testPage.getByTestId("lsp-status-button")).toBeVisible();
      const statusItem = testPage.getByTestId("app-status-lsp");
      await expect(statusItem).toBeVisible();
      await expect(statusItem).toContainText("Kotlin");
      await expect(testPage.locator('[data-status-item-id="builtin:lsp"]')).toHaveAttribute(
        "data-status-side",
        "right",
      );

      await performLspAction(testPage, "start");
      await expect(statusItem).toContainText("Importing Kotlin project", { timeout: 15_000 });
      const statusSurface = await openLspStatus(testPage);
      const statusProgress = statusSurface.getByTestId("task-lsp-progress");
      await expect(statusProgress).toHaveAttribute("data-lsp-progress-kind", "active");
      const statusMessage = statusProgress.getByText(LONG_LSP_PROGRESS_MESSAGE, { exact: true });
      await expect(statusMessage).toBeVisible();
      await assertNoElementHorizontalOverflow(statusMessage, "status-bar LSP progress text");
      const taskSurface = testPage.getByTestId("task-lsp-surface");
      await assertNoElementHorizontalOverflow(taskSurface, "status-bar LSP progress popover");
      await assertLocatorWithinViewportX(taskSurface, "status-bar LSP progress popover");

      releaseFakeLspInitialization(backend);
      await expect(statusSurface).toHaveAttribute("data-lsp-state", "ready", {
        timeout: 15_000,
      });
      await testPage.keyboard.press("Escape");
      await expect(taskSurface).toBeHidden();
      const kotlinPreview = testPage.getByTestId("preview-tab-file-editor");
      await kotlinPreview.dblclick();
      await expect(kotlinPreview).not.toHaveAttribute(
        "title",
        "Double-click to keep this tab open",
      );

      await task.session.clickTab("Files");
      const binaryNode = task.session.fileTreeNode(task.filePaths[2]);
      await expect(binaryNode).toBeVisible({ timeout: 15_000 });
      await binaryNode.click();
      await expect(testPage.getByText("Binary file", { exact: true })).toBeVisible();
      await expect(statusItem).toBeVisible();
      await expect(statusItem).toContainText("Kotlin");

      await openDesktopFile(testPage, task.session, task.filePaths[1]);
      await expect(statusItem).toBeVisible();
      await task.session.clickSessionChatTab();
      await expect(statusItem).toBeVisible();

      await testPage
        .locator(".dv-default-tab", { hasText: path.basename(task.filePaths[0]) })
        .click();
      await performLspAction(testPage, "stop");
      await expect(await openLspStatus(testPage)).toHaveAttribute("data-lsp-state", "stopped");
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        app_status_bar_enabled: initialStatusBarEnabled,
        lsp_status_location: initialLocation,
      });
    }
  });

  test("runs Kotlin intelligence through the task host", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Kotlin LSP Full Protocol",
      filePaths: [RESERVED_SOURCE_PATH, DEFINITION_TARGET_PATH],
    });
    const lspSockets: string[] = [];
    testPage.on("websocket", (socket) => {
      if (socket.url().includes("/lsp/")) lspSockets.push(socket.url());
    });

    await openDesktopFile(testPage, task.session, task.filePaths[0]);
    const statusButton = testPage.getByTestId("lsp-status-button");
    await expect(statusButton).toHaveAttribute("data-lsp-language", "kotlin");
    await performLspAction(testPage, "start");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });

    const started = await expectFakeLspEvent(
      backend,
      (event) => event.event === "started",
      "task-host process start",
    );
    expect(started.argv).toEqual(["--stdio"]);
    expect(started.cwd).toMatch(/\/repos\/e2e-repo$/);
    const workspaceUri = pathToFileURL(started.cwd!).href;
    const sourceUri = pathToFileURL(path.join(started.cwd!, task.filePaths[0])).href;
    const definitionUri = pathToFileURL(path.join(started.cwd!, task.filePaths[1])).href;
    const definitionModelUri = expectedMonacoModelUri(definitionUri, task.sessionId);
    await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "initialize" &&
        event.params?.rootUri === workspaceUri,
      "initialize with task workspace",
    );
    const didOpen = await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/didOpen" &&
        (event.params?.textDocument as { languageId?: string } | undefined)?.languageId ===
          "kotlin",
      "didOpen for the reserved-character Kotlin file",
    );
    expect(didOpen.params?.textDocument).toMatchObject({
      uri: sourceUri,
      languageId: "kotlin",
      version: 1,
    });
    expect(lspSockets).toHaveLength(1);
    await expectFakeLspMarkerCount(testPage, 1);

    const editor = testPage.locator(".monaco-editor:visible");
    await editor.click();
    await testPage.keyboard.press("Control+Space");
    const manualCompletion = await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/completion",
      "completion request",
    );
    expect(manualCompletion.params?.context).toEqual({ triggerKind: 1 });
    await expect(testPage.locator(".suggest-widget")).toContainText("fakeGreeting");
    await testPage.keyboard.insertText("f");
    const incompleteRetrigger = await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/completion" &&
        (event.params?.context as { triggerKind?: number } | undefined)?.triggerKind === 3,
      "incomplete completion retrigger",
    );
    expect(incompleteRetrigger.params?.context).toEqual({ triggerKind: 3 });
    await testPage.keyboard.press("Enter");
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).toContainText(
      "fakeGreeting",
    );
    await testPage.keyboard.press("Control+Z");
    await testPage.keyboard.press("Control+Z");

    await testPage
      .locator(".monaco-editor:visible .view-line")
      .nth(2)
      .hover({
        position: { x: 80, y: 8 },
      });
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/hover",
      "hover request",
    );
    await expect(testPage.getByText("Fake Kotlin hover", { exact: true })).toBeVisible();
    await testPage.keyboard.press("Escape");

    const nestedFolder = task.session.fileTreeNode("nested");
    const definitionFolder = task.session.fileTreeNode(DEFINITION_PARENT_PATH);
    const definitionTarget = task.session.fileTreeNode(DEFINITION_TARGET_PATH);
    await expect(nestedFolder.locator(".tabler-icon-chevron-right")).toBeVisible();
    await expect(definitionFolder).toHaveCount(0);
    await expect(definitionTarget).toHaveCount(0);

    await testPage.keyboard.press("F12");
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/definition",
      "definition request",
    );
    await expect(
      testPage.locator(".dv-default-tab", { hasText: path.basename(task.filePaths[1]) }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).toContainText(
      "fun greeting1(name: String): String",
    );
    await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/didOpen" &&
        (event.params?.textDocument as { uri?: string } | undefined)?.uri === definitionUri,
      "canonical didOpen for the definition target",
    );
    await expect
      .poll(() =>
        testPage.evaluate((expectedUri) => {
          const monaco = (
            window as typeof window & {
              monaco?: {
                editor: {
                  getEditors: () => Array<{
                    getModel: () => { uri: { toString: () => string } } | null;
                    getPosition: () => { lineNumber: number; column: number } | null;
                    hasTextFocus: () => boolean;
                  }>;
                  getModels: () => Array<{
                    getValue: () => string;
                    uri: { toString: () => string };
                  }>;
                };
              };
            }
          ).monaco;
          const targetModel = monaco?.editor
            .getModels()
            .find((model) => model.uri.toString() === expectedUri);
          const activeEditor = monaco?.editor
            .getEditors()
            .find((candidate) => candidate.hasTextFocus());
          return {
            modelUri: targetModel?.uri.toString() ?? null,
            modelContent: targetModel?.getValue() ?? null,
            activeUri: activeEditor?.getModel()?.uri.toString() ?? null,
            position: activeEditor?.getPosition() ?? null,
          };
        }, definitionModelUri),
      )
      .toEqual({
        modelUri: definitionModelUri,
        modelContent: expect.stringContaining("fun greeting1(name: String): String"),
        activeUri: definitionModelUri,
        position: { lineNumber: 3, column: 5 },
      });

    await expect(nestedFolder.locator(".tabler-icon-chevron-down")).toBeVisible();
    await expect(definitionFolder.locator(".tabler-icon-chevron-down")).toBeVisible();
    await expect(definitionTarget).toBeVisible();
    await expect(definitionTarget).toHaveAttribute("data-active", "true");

    await testPage.keyboard.press("Shift+F12");
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/references",
      "references request",
    );
    await testPage.keyboard.press("Escape");

    await editor.click();
    await testPage.keyboard.press("Control+Shift+Space");
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/signatureHelp",
      "signature-help request",
    );
    await testPage.keyboard.press("Escape");
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/semanticTokens/full",
      "semantic-tokens request",
    );

    await editor.click();
    await testPage.keyboard.press("Control+End");
    await testPage.keyboard.insertText("\n// e2e change");
    await testPage.getByRole("button", { name: "Save (Ctrl+S)", exact: true }).click();
    const didSave = await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/didSave" &&
        (event.params?.textDocument as { uri?: string } | undefined)?.uri === definitionUri,
      "document save",
    );
    expect(didSave.params?.text).toContain("// e2e change");
    await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/didChange" &&
        (event.params?.textDocument as { uri?: string } | undefined)?.uri === definitionUri,
      "document change before save",
    );
    const synchronizationEvents = readFakeLspEvents(backend);
    const didChangeIndex = synchronizationEvents.findIndex(
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/didChange" &&
        (event.params?.textDocument as { uri?: string } | undefined)?.uri === definitionUri,
    );
    const didSaveIndex = synchronizationEvents.findIndex(
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/didSave" &&
        (event.params?.textDocument as { uri?: string } | undefined)?.uri === definitionUri,
    );
    expect(didChangeIndex).toBeGreaterThanOrEqual(0);
    expect(didSaveIndex).toBeGreaterThan(didChangeIndex);
    await expectFakeLspMarkerCount(testPage, 2);

    await editor.click();
    await testPage.keyboard.press("Control+End");
    await testPage.keyboard.type(".");
    const triggeredCompletion = await expectFakeLspEvent(
      backend,
      (event) => {
        const context = event.params?.context as
          | { triggerKind?: number; triggerCharacter?: string }
          | undefined;
        return (
          event.event === "message" &&
          event.method === "textDocument/completion" &&
          context?.triggerKind === 2 &&
          context.triggerCharacter === "."
        );
      },
      "trigger-character completion request",
    );
    expect(triggeredCompletion.params?.context).toEqual({
      triggerKind: 2,
      triggerCharacter: ".",
    });
    await testPage.keyboard.press("Escape");
    await testPage.keyboard.press("Control+Z");

    await performLspAction(testPage, "stop");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "stopped");
    await expectFakeLspMarkerCount(testPage, 0);
    await expectFakeLspEvent(
      backend,
      (event) =>
        (event.event === "message" && event.method === "shutdown") || event.event === "signal",
      "graceful server stop",
    );
    await expect
      .poll(() => readFakeLspEvents(backend).filter((event) => event.event === "started").length)
      .toBe(1);
  });

  test("auto-starts one shared server, forwards configuration, and honors Stop", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialAutoStart = Array.isArray(initial.settings.lsp_auto_start_languages)
      ? (initial.settings.lsp_auto_start_languages as string[])
      : [];
    const initialConfigs =
      typeof initial.settings.lsp_server_configs === "object" &&
      initial.settings.lsp_server_configs !== null
        ? (initial.settings.lsp_server_configs as Record<string, Record<string, unknown>>)
        : {};

    try {
      installFakeKotlinLsp(backend);
      await apiClient.saveUserSettings({
        lsp_auto_start_languages: ["kotlin"],
        lsp_server_configs: {
          kotlin: { e2e: { enabled: true }, compiler: { jvmTarget: "21" } },
        },
      });
      const lspSockets: string[] = [];
      testPage.on("websocket", (socket) => {
        if (socket.url().includes("/lsp/")) lspSockets.push(socket.url());
      });

      const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
        title: "Kotlin LSP Shared Connection",
        fileCount: 2,
      });
      await openDesktopFile(testPage, task.session, task.filePaths[0]);
      await expect(testPage.locator('[data-testid="lsp-status-button"]:visible')).toHaveAttribute(
        "data-lsp-state",
        "ready",
        { timeout: 15_000 },
      );
      await expectFakeLspEvent(
        backend,
        (event) =>
          event.event === "response" &&
          Array.isArray(event.result) &&
          JSON.stringify(event.result).includes('"jvmTarget":"21"'),
        "custom workspace configuration response",
      );

      await apiClient.saveUserSettings({
        lsp_server_configs: {
          kotlin: { e2e: { enabled: true }, compiler: { jvmTarget: "17" } },
        },
      });
      await expectFakeLspEvent(
        backend,
        (event) =>
          event.event === "message" &&
          event.method === "workspace/didChangeConfiguration" &&
          JSON.stringify(event.params).includes('"jvmTarget":"17"'),
        "live task-host configuration update",
      );
      expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(
        1,
      );

      const started = await expectFakeLspEvent(
        backend,
        (event) => event.event === "started",
        "shared task-host process start",
      );
      const firstDocumentUri = pathToFileURL(path.join(started.cwd!, task.filePaths[0])).href;
      const secondDocumentUri = pathToFileURL(path.join(started.cwd!, task.filePaths[1])).href;
      const firstModelUri = expectedMonacoModelUri(firstDocumentUri, task.sessionId);
      const secondModelUri = expectedMonacoModelUri(secondDocumentUri, task.sessionId);
      await expectFakeLspMarkerMessages(testPage, firstModelUri, ["Fake Kotlin diagnostic"]);
      const firstPreview = testPage.getByTestId("preview-tab-file-editor");
      await firstPreview.dblclick();
      await expect(firstPreview).not.toHaveAttribute("title", "Double-click to keep this tab open");

      await openDesktopFile(testPage, task.session, task.filePaths[1]);
      await expectFakeLspEvent(
        backend,
        (event) =>
          event.event === "message" &&
          event.method === "textDocument/didOpen" &&
          JSON.stringify(event.params).includes(task.filePaths[1]),
        "didOpen for the second file",
      );
      await expectFakeLspMarkerMessages(testPage, secondModelUri, ["Fake Kotlin diagnostic"]);
      expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(
        1,
      );
      expect(lspSockets).toHaveLength(1);

      await expectFakeLspMarkerMessages(testPage, firstModelUri, ["Fake Kotlin diagnostic"]);
      await expectFakeLspMarkerMessages(testPage, secondModelUri, ["Fake Kotlin diagnostic"]);
      await testPage.locator(".monaco-editor:visible").click();
      await testPage.keyboard.press("Control+End");
      await testPage.keyboard.insertText("\n// second document edit");
      const didChange = await expectFakeLspEvent(
        backend,
        (event) => event.event === "message" && event.method === "textDocument/didChange",
        "document change",
      );
      expect(didChange.params?.textDocument).toMatchObject({ uri: secondDocumentUri });
      const contentChanges = didChange.params?.contentChanges as
        | Array<{
            range?: {
              start: { line: number; character: number };
              end: { line: number; character: number };
            };
            text?: string;
          }>
        | undefined;
      expect(contentChanges).toHaveLength(1);
      expect(contentChanges?.[0]).toMatchObject({
        text: expect.stringContaining("\n// second document edit"),
      });
      expect(contentChanges?.[0].range).toBeUndefined();
      await expectFakeLspMarkerMessages(testPage, secondModelUri, [
        "Fake Kotlin diagnostic after edit",
      ]);
      await expectFakeLspMarkerMessages(testPage, firstModelUri, ["Fake Kotlin diagnostic"]);
      const firstTab = testPage.locator(".dv-default-tab", {
        hasText: path.basename(task.filePaths[0]),
      });
      await expect(firstTab).toHaveCount(1);

      const closeCountBefore = readFakeLspEvents(backend).filter(
        (event) =>
          event.event === "message" &&
          event.method === "textDocument/didClose" &&
          (event.params?.textDocument as { uri?: string } | undefined)?.uri === secondDocumentUri,
      ).length;
      const secondTab = testPage.locator(".dv-default-tab", {
        hasText: path.basename(task.filePaths[1]),
      });
      await secondTab.hover();
      await secondTab.locator(".dv-default-tab-action").click();
      await expect(secondTab).toHaveCount(0);
      await expect(firstTab).toHaveCount(1);
      await expect
        .poll(
          () =>
            readFakeLspEvents(backend).filter(
              (event) =>
                event.event === "message" &&
                event.method === "textDocument/didClose" &&
                (event.params?.textDocument as { uri?: string } | undefined)?.uri ===
                  secondDocumentUri,
            ).length,
          { message: "waiting for didClose for the second file" },
        )
        .toBe(closeCountBefore + 1);
      await expectFakeLspMarkerMessages(testPage, secondModelUri, []);
      await expectFakeLspMarkerMessages(testPage, firstModelUri, ["Fake Kotlin diagnostic"]);

      const activeStatus = testPage.locator('[data-testid="lsp-status-button"]:visible');
      await performLspAction(testPage, "stop");
      await expect(activeStatus).toHaveAttribute("data-lsp-state", "stopped");

      await openDesktopFile(testPage, task.session, task.filePaths[1]);
      await expect(activeStatus).toHaveAttribute("data-lsp-state", "stopped");
      expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(
        1,
      );

      await performLspAction(testPage, "start");
      await expect(activeStatus).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
      await expect
        .poll(() => readFakeLspEvents(backend).filter((event) => event.event === "started").length)
        .toBe(2);
      await performLspAction(testPage, "stop");
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_auto_start_languages: initialAutoStart,
        lsp_server_configs: initialConfigs,
      });
    }
  });

  test("uses the task-host root for a secondary repository document URI", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const repositoryName = `lsp-secondary-${suffix}`;
    const secondaryFilePath = "src/Multi Repo # query? 100%.kt";
    const secondaryDefinitionPath = path.posix.join(
      path.posix.dirname(secondaryFilePath),
      DEFINITION_TARGET_PATH,
    );
    const repositoryDirectory = path.join(backend.tmpDir, "repos", repositoryName);
    const gitEnv = makeGitEnv(backend.tmpDir);
    fs.mkdirSync(path.join(repositoryDirectory, path.dirname(secondaryDefinitionPath)), {
      recursive: true,
    });
    execSync("git init -b main", { cwd: repositoryDirectory, env: gitEnv });
    fs.writeFileSync(
      path.join(repositoryDirectory, secondaryFilePath),
      [
        "package secondary",
        "",
        "fun secondaryGreeting(name: String): String {",
        '    return "Hello, $name"',
        "}",
        "",
      ].join("\n"),
    );
    fs.writeFileSync(
      path.join(repositoryDirectory, secondaryDefinitionPath),
      [
        "package secondary.definition",
        "",
        "fun secondaryDefinition(name: String): String {",
        '    return "Hello, $name"',
        "}",
        "",
      ].join("\n"),
    );
    execSync("git add -A", { cwd: repositoryDirectory, env: gitEnv });
    execSync('git commit -m "add secondary Kotlin fixture"', {
      cwd: repositoryDirectory,
      env: gitEnv,
    });
    const secondaryRepository = await apiClient.createRepository(
      seedData.workspaceId,
      repositoryDirectory,
      "main",
      { name: repositoryName, pull_before_worktree: false },
    );

    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Kotlin LSP Multi-Repo URI",
      executorProfileId: seedData.worktreeExecutorProfileId,
      repositoryIds: [seedData.repositoryId, secondaryRepository.id],
    });
    const taskRelativePath = `${repositoryName}/${secondaryFilePath}`;
    const definitionTaskRelativePath = `${repositoryName}/${secondaryDefinitionPath}`;
    await openDesktopFile(testPage, task.session, taskRelativePath);
    const statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await performLspAction(testPage, "start");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });

    const started = await expectFakeLspEvent(
      backend,
      (event) => event.event === "started",
      "multi-repo task-host process start",
    );
    expect(started.cwd).toBeTruthy();
    const workspaceUri = pathToFileURL(started.cwd!).href;
    const documentUri = pathToFileURL(path.join(started.cwd!, taskRelativePath)).href;
    await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "initialize" &&
        event.params?.rootUri === workspaceUri,
      "multi-repo initialize with task root",
    );
    const didOpen = await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/didOpen",
      "secondary repository didOpen",
    );
    expect(didOpen.params?.textDocument).toMatchObject({
      uri: documentUri,
      languageId: "kotlin",
      version: 1,
    });
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).toContainText(
      "fun secondaryGreeting(name: String): String",
    );

    // Pin the source, then open and pin the definition through the Files tree.
    // That tree path is task-root-relative (`repo/path`) while LSP resolves the
    // same target as `{ repo, path }`; navigation must reuse this tab identity.
    const sourceTab = testPage.locator(".dv-default-tab", {
      hasText: path.basename(secondaryFilePath),
    });
    await sourceTab.dblclick();

    const nestedPath = `${repositoryName}/src/nested`;
    const referencesPath = `${repositoryName}/src/${DEFINITION_PARENT_PATH}`;
    const targetPath = `${repositoryName}/${secondaryDefinitionPath}`;
    const nestedFolder = task.session.fileTreeNode(nestedPath);
    const referencesFolder = task.session.fileTreeNode(referencesPath);
    const definitionTarget = task.session.fileTreeNode(targetPath);
    await expect(nestedFolder.locator(".tabler-icon-chevron-right")).toBeVisible();
    await expect(referencesFolder).toHaveCount(0);
    await expect(definitionTarget).toHaveCount(0);

    await openDesktopFile(testPage, task.session, definitionTaskRelativePath);
    const definitionTabs = testPage.locator(".dv-default-tab", {
      hasText: path.basename(secondaryDefinitionPath),
    });
    await expect(definitionTabs).toHaveCount(1);
    await definitionTabs.dblclick();
    await sourceTab.click();
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).toContainText(
      "fun secondaryGreeting(name: String): String",
    );

    await testPage.locator(".monaco-editor:visible").click();
    await testPage.keyboard.press("F12");
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/definition",
      "secondary repository definition request",
    );
    const definitionUri = pathToFileURL(path.join(started.cwd!, definitionTaskRelativePath)).href;
    await expect(definitionTabs).toHaveCount(1);
    await expect(definitionTabs).toBeVisible({ timeout: 15_000 });
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).toContainText(
      "fun secondaryDefinition(name: String): String",
    );
    await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/didOpen" &&
        (event.params?.textDocument as { uri?: string } | undefined)?.uri === definitionUri,
      "secondary repository definition didOpen",
    );
    await expect(nestedFolder.locator(".tabler-icon-chevron-down")).toBeVisible();
    await expect(referencesFolder.locator(".tabler-icon-chevron-down")).toBeVisible();
    await expect(definitionTarget).toBeVisible();
    await expect(definitionTarget).toHaveAttribute("data-active", "true");

    await performLspAction(testPage, "stop");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "stopped");
  });

  test("keeps task policy and one generation across reload without localStorage ownership", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Kotlin LSP Manual Persistence",
    });
    await openDesktopFile(testPage, task.session, task.filePaths[0]);
    let statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await performLspAction(testPage, "start");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    const storageKey = `kandev-lsp:${task.sessionId}:kotlin`;
    expect(await testPage.evaluate((key) => localStorage.getItem(key), storageKey)).toBeNull();
    expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(1);
    expect(readFakeLspEvents(backend).filter((event) => event.event === "initialize")).toHaveLength(
      1,
    );

    await testPage.reload();
    await openDesktopFile(testPage, task.session, task.filePaths[0]);
    statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    await expectFakeLspMarkerCount(testPage, 1);
    expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(1);
    expect(readFakeLspEvents(backend).filter((event) => event.event === "initialize")).toHaveLength(
      1,
    );

    await performLspAction(testPage, "stop");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "stopped");
    expect(await testPage.evaluate((key) => localStorage.getItem(key), storageKey)).toBeNull();
  });

  test("recovers a crashed task server and browser attachment automatically", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend, { crashOnOpenOnce: true });
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Kotlin LSP Crash Recovery",
    });
    await openDesktopFile(testPage, task.session, task.filePaths[0]);
    const statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    const crashStatus = await openLspStatus(testPage);
    const startButton = crashStatus.locator(
      '[data-testid="lsp-lifecycle-action"][data-lsp-action="start"]',
    );
    await expect(startButton).toBeEnabled();
    const errorEvidence = Promise.all([
      expect(statusButton).toHaveAttribute("data-lsp-state", "error", {
        timeout: 15_000,
      }),
      expect(crashStatus).toContainText("Error", { timeout: 15_000 }),
      expect(crashStatus).toContainText("language server exited", { timeout: 15_000 }),
    ]);
    await startButton.click();
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "crashing" && event.reason === "didOpen",
      "intentional server crash",
    );
    await errorEvidence;

    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    await expectFakeLspMarkerCount(testPage, 1);
    expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(2);
    await performLspAction(testPage, "stop");
  });

  test("keeps TypeScript intelligence active when the Kotlin connection crashes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    installAdditionalFakeLspBinary(backend, "typescript-language-server");
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "LSP Cross Connection Isolation",
      extensions: ["kt", "ts"],
    });

    await openDesktopFile(testPage, task.session, task.filePaths[0]);
    let statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await performLspAction(testPage, "start");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    const kotlinOpen = await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/didOpen" &&
        JSON.stringify(event.params).includes('"languageId":"kotlin"'),
      "Kotlin didOpen",
    );
    const kotlinPreviewTab = testPage.getByTestId("preview-tab-file-editor");
    await expect(kotlinPreviewTab).toBeVisible();
    await kotlinPreviewTab.dblclick();

    await openDesktopFile(testPage, task.session, task.filePaths[1]);
    statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await expect(statusButton).toHaveAttribute("data-lsp-language", "typescript");
    await performLspAction(testPage, "start");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    const typescriptOpen = await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/didOpen" &&
        JSON.stringify(event.params).includes('"languageId":"typescript"'),
      "TypeScript didOpen",
    );
    await expectFakeLspMarkerCount(testPage, 2);

    process.kill(kotlinOpen.pid, "SIGKILL");
    await testPage.locator(".dv-default-tab", { hasText: task.filePaths[0] }).click();
    await expect(testPage.locator('[data-testid="lsp-status-button"]:visible')).toHaveAttribute(
      "data-lsp-state",
      "error",
      { timeout: 15_000 },
    );

    await performLspAction(testPage, "stop");
    await expect(testPage.locator('[data-testid="lsp-status-button"]:visible')).toHaveAttribute(
      "data-lsp-state",
      "stopped",
    );

    await testPage.locator(".dv-default-tab", { hasText: task.filePaths[1] }).click();
    const typescriptStatus = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await expect(typescriptStatus).toHaveAttribute("data-lsp-state", "ready");
    await expectFakeLspMarkerCount(testPage, 1);
    await testPage.locator(".monaco-editor:visible").click();
    await testPage.keyboard.press("Control+Space");
    await expectFakeLspEvent(
      backend,
      (event) =>
        event.pid === typescriptOpen.pid &&
        event.event === "message" &&
        event.method === "textDocument/completion",
      "TypeScript completion after Kotlin cleanup",
    );
    await expect(testPage.locator(".suggest-widget")).toContainText("fakeGreeting");
    await testPage.keyboard.press("Escape");
    await performLspAction(testPage, "stop");
    await expectFakeLspMarkerCount(testPage, 0);
  });

  test("stops the task-host process when its task is archived", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Kotlin LSP Archive Cleanup",
    });
    await openDesktopFile(testPage, task.session, task.filePaths[0]);
    const statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await performLspAction(testPage, "start");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    const started = await expectFakeLspEvent(
      backend,
      (event) => event.event === "started",
      "task-host process start",
    );

    await apiClient.archiveTask(task.taskId);
    await expectFakeLspEventCount(
      backend,
      (event) => event.pid === started.pid && (event.event === "exit" || event.event === "signal"),
      1,
      "task teardown event for the task-owned process",
    );
    await expectFakeLspProcessStopped(started.pid);
  });

  test("queues excess task servers and starts after real capacity is released", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(150_000);
    await backend.restart({ KANDEV_LSP_MAX_SERVERS: "1" });
    installFakeKotlinLsp(backend);
    let firstLspSocketObserved = false;
    let firstLspSocketClosed = false;
    testPage.on("websocket", (socket) => {
      if (firstLspSocketObserved || !socket.url().includes("/lsp/")) return;
      firstLspSocketObserved = true;
      socket.on("close", () => {
        firstLspSocketClosed = true;
      });
    });
    const secondPage = await testPage.context().newPage();
    try {
      const first = await createKotlinTask(testPage, apiClient, seedData, backend, {
        title: "Kotlin LSP Capacity One",
      });
      await openDesktopFile(testPage, first.session, first.filePaths[0]);
      const firstStatus = testPage.locator('[data-testid="lsp-status-button"]:visible');
      await performLspAction(testPage, "start");
      await expect(firstStatus).toHaveAttribute("data-lsp-state", "ready", {
        timeout: 15_000,
      });
      const firstStarted = await expectFakeLspEvent(
        backend,
        (event) => event.event === "started",
        "first capacity-limited task-host process",
      );

      const second = await createKotlinTask(secondPage, apiClient, seedData, backend, {
        title: "Kotlin LSP Capacity Two",
      });
      await openDesktopFile(secondPage, second.session, second.filePaths[0]);
      const secondStatus = secondPage.locator('[data-testid="lsp-status-button"]:visible');
      await performLspAction(secondPage, "start");
      await expect(secondStatus).toHaveAttribute("data-lsp-state", "queued", {
        timeout: 15_000,
      });
      const queuedRow = await openLspStatus(secondPage);
      await expect(queuedRow).toContainText("Waiting for capacity");
      expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(
        1,
      );
      await expect(secondPage.getByText(/Enable auto-install/)).toHaveCount(0);

      await performLspAction(testPage, "stop");
      await expect(firstStatus).toHaveAttribute("data-lsp-state", "stopped");
      await expect.poll(() => firstLspSocketObserved && firstLspSocketClosed).toBe(true);
      await expectFakeLspEvent(
        backend,
        (event) =>
          event.pid === firstStarted.pid &&
          (event.event === "exit" || event.event === "signal" || event.event === "stdin ended"),
        "first capacity-limited task-host process stop",
      );
      await expect.poll(() => isProcessAlive(firstStarted.pid)).toBe(false);

      await expect(secondStatus).toHaveAttribute("data-lsp-state", "ready", {
        timeout: 15_000,
      });
      await expect
        .poll(() => readFakeLspEvents(backend).filter((event) => event.event === "started").length)
        .toBe(2);
      await performLspAction(secondPage, "stop");
    } finally {
      await secondPage.close();
      await backend.restart();
    }
  });
});

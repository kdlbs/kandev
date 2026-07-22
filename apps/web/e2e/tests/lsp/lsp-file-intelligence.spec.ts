import { test, expect } from "../../fixtures/test-base";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import { pathToFileURL } from "node:url";
import { makeGitEnv } from "../../helpers/git-helper";
import {
  clearFakeKotlinLspModes,
  createKotlinTask,
  expectedMonacoModelUri,
  expectFakeLspEvent,
  expectFakeLspMarkerCount,
  installAdditionalFakeLspBinary,
  installFakeKotlinLsp,
  openDesktopFile,
  readFakeLspEvents,
  removeFakeKotlinLsp,
} from "./lsp-e2e-helpers";

const EDITORS_SETTINGS_PATH = "/settings/general/editors";
const RESERVED_SOURCE_PATH = "Main # query? 100%.kt";
const DEFINITION_TARGET_PATH = "Definition Target # query? 100%.kt";

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
    await expect(statusButton).toHaveAttribute("data-lsp-state", "disabled");
    await statusButton.click();

    await expect(statusButton).toHaveAttribute("data-lsp-state", "unavailable", {
      timeout: 15_000,
    });
    await expect(testPage.getByText("Language server unavailable", { exact: true })).toBeVisible();
    await expect(testPage.getByText(/Install kotlin-lsp on the task host/)).toBeVisible();
    await expect(testPage.getByText(/Enable auto-install/)).toHaveCount(0);
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
    await statusButton.click();
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
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/completion",
      "completion request",
    );
    await expect(testPage.locator(".suggest-widget")).toContainText("fakeGreeting");
    await testPage.keyboard.press("Escape");

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

    await testPage.keyboard.press("F12");
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/definition",
      "definition request",
    );
    await expect(testPage.locator(".dv-default-tab", { hasText: task.filePaths[1] })).toBeVisible({
      timeout: 15_000,
    });
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
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "message" && event.method === "textDocument/didChange",
      "document change",
    );
    await expectFakeLspMarkerCount(testPage, 1);

    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "disabled");
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

  test("auto-starts one shared server for two files and forwards configuration", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
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

    await openDesktopFile(testPage, task.session, task.filePaths[1]);
    await expectFakeLspEvent(
      backend,
      (event) =>
        event.event === "message" &&
        event.method === "textDocument/didOpen" &&
        JSON.stringify(event.params).includes(task.filePaths[1]),
      "didOpen for the second file",
    );
    expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(1);
    expect(lspSockets).toHaveLength(1);

    const activeStatus = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await activeStatus.click();
    await expect(activeStatus).toHaveAttribute("data-lsp-state", "disabled");
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
    const repositoryDirectory = path.join(backend.tmpDir, "repos", repositoryName);
    const gitEnv = makeGitEnv(backend.tmpDir);
    fs.mkdirSync(path.join(repositoryDirectory, "src"), { recursive: true });
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
    execSync("git add -A", { cwd: repositoryDirectory, env: gitEnv });
    execSync('git commit -m "add secondary Kotlin fixture"', {
      cwd: repositoryDirectory,
      env: gitEnv,
    });
    const secondaryRepository = await apiClient.createRepository(
      seedData.workspaceId,
      repositoryDirectory,
      "main",
      { name: repositoryName },
    );

    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Kotlin LSP Multi-Repo URI",
      executorProfileId: seedData.worktreeExecutorProfileId,
      repositoryIds: [seedData.repositoryId, secondaryRepository.id],
    });
    const taskRelativePath = `${repositoryName}/${secondaryFilePath}`;
    await openDesktopFile(testPage, task.session, taskRelativePath);
    const statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await statusButton.click();
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
    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "disabled");
  });

  test("restores a manual connection after reload and forgets it after stop", async ({
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
    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    const storageKey = `kandev-lsp:${task.sessionId}:kotlin`;
    expect(await testPage.evaluate((key) => localStorage.getItem(key), storageKey)).toBe("1");

    await testPage.reload();
    await openDesktopFile(testPage, task.session, task.filePaths[0]);
    statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    await expect
      .poll(() => readFakeLspEvents(backend).filter((event) => event.event === "started").length)
      .toBeGreaterThanOrEqual(2);

    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "disabled");
    expect(await testPage.evaluate((key) => localStorage.getItem(key), storageKey)).toBeNull();
  });

  test("cleans up a crashed server and reconnects", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend, { crashOnOpen: true });
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Kotlin LSP Crash Recovery",
    });
    await openDesktopFile(testPage, task.session, task.filePaths[0]);
    const statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await statusButton.click();
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "crashing" && event.reason === "didOpen",
      "intentional server crash",
    );
    await expect(statusButton).toHaveAttribute("data-lsp-state", "disabled", {
      timeout: 15_000,
    });
    await expectFakeLspMarkerCount(testPage, 0);

    clearFakeKotlinLspModes(backend);
    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    await expectFakeLspMarkerCount(testPage, 1);
    expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(2);
    await statusButton.click();
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
    await statusButton.click();
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
    await statusButton.click();
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
      "disabled",
      { timeout: 15_000 },
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
    await typescriptStatus.click();
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
    await statusButton.click();
    await expect(statusButton).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
    const started = await expectFakeLspEvent(
      backend,
      (event) => event.event === "started",
      "task-host process start",
    );

    await apiClient.archiveTask(task.taskId);
    await expectFakeLspEvent(
      backend,
      (event) => event.event === "signal" || event.event === "stdin ended",
      "task teardown signal",
    );
    await expect.poll(() => isProcessAlive(started.pid)).toBe(false);
  });

  test("rejects excess connections and succeeds after capacity is released", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(150_000);
    await backend.restart({ KANDEV_LSP_MAX_CONNECTIONS: "1" });
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
      await firstStatus.click();
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
      await secondStatus.click();
      await expect(secondStatus).toHaveAttribute("data-lsp-state", "unavailable", {
        timeout: 15_000,
      });
      await expect(secondPage.getByText(/active LSP connection cap exceeded/)).toBeVisible();
      await expect(secondPage.getByText(/Enable auto-install/)).toHaveCount(0);

      await firstStatus.click();
      await expect(firstStatus).toHaveAttribute("data-lsp-state", "disabled");
      await expect.poll(() => firstLspSocketObserved && firstLspSocketClosed).toBe(true);
      await expectFakeLspEvent(
        backend,
        (event) =>
          event.pid === firstStarted.pid &&
          (event.event === "exit" || event.event === "signal" || event.event === "stdin ended"),
        "first capacity-limited task-host process stop",
      );
      await expect.poll(() => isProcessAlive(firstStarted.pid)).toBe(false);

      await expect
        .poll(
          async () => {
            const state = await secondStatus.getAttribute("data-lsp-state");
            if (state === "disabled" || state === "unavailable") await secondStatus.click();
            return secondStatus.getAttribute("data-lsp-state");
          },
          { timeout: 15_000, message: "waiting for the released LSP slot to become available" },
        )
        .toBe("ready");
      await expect
        .poll(() => readFakeLspEvents(backend).filter((event) => event.event === "started").length)
        .toBe(2);
    } finally {
      await secondPage.close();
      await backend.restart();
    }
  });
});

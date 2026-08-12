import { expect, test } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import {
  captureAppStatusBarSettings,
  restoreAppStatusBarSettings,
  setAppStatusBarEnabled,
} from "../../helpers/app-status-bar-settings";
import { SessionPage } from "../../pages/session-page";
import {
  createKotlinTask,
  expandTaskLspLanguage,
  expectFakeLspEventCount,
  expectFakeLspGeneration,
  expectFakeLspProcessStopped,
  installFakeKotlinLsp,
  LONG_LSP_PROGRESS_MESSAGE,
  openDesktopFile,
  openTaskLspControl,
  performTaskLspAction,
  readFakeLspEvents,
  releaseFakeLspInitialization,
} from "./lsp-e2e-helpers";

async function expectKotlinState(page: Page, state: string) {
  const surface = await openTaskLspControl(page);
  await expect(surface.getByTestId("task-lsp-language-kotlin")).toHaveAttribute(
    "data-lsp-state",
    state,
  );
}

test.describe("task-scoped LSP lifecycle", () => {
  test.describe.configure({ timeout: 120_000 });

  test("starts Kotlin before opening a file", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Task LSP Start Without Editor",
      filePaths: ["build.gradle.kts", "src/Main.kt"],
    });

    await expect(testPage.locator(".monaco-editor:visible")).toHaveCount(0);
    await expectKotlinState(testPage, "detected");
    await performTaskLspAction(testPage, "kotlin", "start");

    const process = await expectFakeLspGeneration(backend, 1);
    await expectFakeLspEventCount(
      backend,
      (event) => event.event === "initialize",
      1,
      "one initialize request",
    );
    await expectFakeLspEventCount(
      backend,
      (event) => event.event === "project import",
      1,
      "one Kotlin project import",
    );
    await expectKotlinState(testPage, "ready");

    await performTaskLspAction(testPage, "kotlin", "stop");
    await expectFakeLspProcessStopped(process.pid);
  });

  test("reconciles an inherited task when the global default changes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
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
        ? (initial.settings.lsp_server_configs as Record<string, Record<string, unknown>>)
        : {};
    let started = false;
    try {
      installFakeKotlinLsp(backend);
      await apiClient.saveUserSettings({
        lsp_auto_start_languages: initialAutoStart.filter((language) => language !== "kotlin"),
      });
      await createKotlinTask(testPage, apiClient, seedData, backend, {
        title: "Task LSP Live Inherited Default",
      });
      await expectKotlinState(testPage, "detected");
      expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(
        0,
      );

      await apiClient.saveUserSettings({
        lsp_auto_start_languages: [...new Set([...initialAutoStart, "kotlin"])],
      });
      await expectFakeLspGeneration(backend, 1);
      started = true;
      await expectKotlinState(testPage, "ready");
    } finally {
      if (started) await performTaskLspAction(testPage, "kotlin", "stop");
      await apiClient.saveUserSettings({
        lsp_auto_start_languages: initialAutoStart,
        lsp_auto_install_languages: initialAutoInstall,
        lsp_server_configs: initialConfigs,
      });
    }
  });

  test("stays warm across panels and the former editor idle boundary", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await testPage.clock.install({ time: new Date("2026-08-05T12:00:00Z") });
    installFakeKotlinLsp(backend);
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Task LSP Warm Across Panels",
      filePaths: ["Main.kt", "README.md"],
      fileContents: ['fun main() = println("warm")\n', "# Unsupported file\n"],
    });

    await performTaskLspAction(testPage, "kotlin", "start");
    const process = await expectFakeLspGeneration(backend, 1);
    await openDesktopFile(testPage, task.session, "README.md");
    await task.session.clickTab("Changes");
    await testPage.clock.fastForward(121_000);

    await expectFakeLspEventCount(
      backend,
      (event) => event.event === "started",
      1,
      "one process after the former two-minute idle timeout",
    );
    await expectKotlinState(testPage, "ready");
    await performTaskLspAction(testPage, "kotlin", "stop");
    await expectFakeLspProcessStopped(process.pid);
  });

  test("deduplicates concurrent access from sessions and browser surfaces", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Task LSP Multi Session Dedup",
    });
    await apiClient.launchSession({
      task_id: task.taskId,
      agent_profile_id: seedData.agentProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: "/e2e:simple-message",
    });

    const secondPage = await testPage.context().newPage();
    try {
      await secondPage.goto(`/t/${task.taskId}`);
      const secondSession = new SessionPage(secondPage);
      await secondSession.waitForLoad(45_000);

      const [firstSurface, secondSurface] = await Promise.all([
        openTaskLspControl(testPage),
        openTaskLspControl(secondPage),
      ]);
      const start = (surface: Locator) =>
        surface.locator(
          '[data-testid="task-lsp-language-kotlin"] [data-testid="lsp-lifecycle-action"][data-lsp-action="start"]',
        );
      await Promise.all([
        expandTaskLspLanguage(firstSurface, "kotlin"),
        expandTaskLspLanguage(secondSurface, "kotlin"),
      ]);
      await expect(start(firstSurface)).toBeEnabled();
      await expect(start(secondSurface)).toBeEnabled();
      await Promise.all([start(firstSurface).click(), start(secondSurface).click()]);

      await expectFakeLspGeneration(backend, 1);
      await expectFakeLspEventCount(
        backend,
        (event) => event.event === "project import",
        1,
        "one shared import for two sessions",
      );
      // Returning focus to the first browser surface may dismiss the other
      // surface's popover. Reopen it and verify the shared disclosure state.
      await expectKotlinState(secondPage, "ready");
      const updatedSecondSurface = await openTaskLspControl(secondPage);
      await expect(updatedSecondSurface.getByTestId("task-lsp-language-kotlin")).toHaveAttribute(
        "data-lsp-state",
        "ready",
      );
      await performTaskLspAction(secondPage, "kotlin", "stop");
    } finally {
      await secondPage.close();
    }
  });

  test("reloads and reattaches without importing again", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Task LSP Reload Reattach",
    });
    await performTaskLspAction(testPage, "kotlin", "start");
    await expectFakeLspGeneration(backend, 1);
    await expectFakeLspEventCount(
      backend,
      (event) => event.event === "project import",
      1,
      "initial import",
    );

    await testPage.reload();
    await expectKotlinState(testPage, "ready");
    await expectFakeLspEventCount(
      backend,
      (event) => event.event === "started",
      1,
      "same process after reload",
    );
    await expectFakeLspEventCount(
      backend,
      (event) => event.event === "project import",
      1,
      "same import after reload",
    );
    await performTaskLspAction(testPage, "kotlin", "stop");
  });

  test("restart creates one generation and stop suppresses reacquisition", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Task LSP Explicit Restart Stop",
    });
    await performTaskLspAction(testPage, "kotlin", "start");
    const first = await expectFakeLspGeneration(backend, 1);

    await performTaskLspAction(testPage, "kotlin", "restart");
    const second = await expectFakeLspGeneration(backend, 2);
    await expectFakeLspProcessStopped(first.pid);
    await expectFakeLspEventCount(
      backend,
      (event) => event.event === "project import",
      2,
      "one import per explicit generation",
    );

    await performTaskLspAction(testPage, "kotlin", "stop");
    await expectFakeLspProcessStopped(second.pid);
    await openDesktopFile(testPage, task.session, task.filePaths[0]);
    // Observe beyond the first one-second recovery epoch so this proves Stop
    // canceled reacquisition rather than merely winning a short timing window.
    await testPage.waitForTimeout(1_500);
    expect(readFakeLspEvents(backend).filter((event) => event.event === "started")).toHaveLength(2);
    await expectKotlinState(testPage, "stopped");
  });

  test("task archive reaps the task-owned process", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "Task LSP Cleanup Owner",
    });
    await performTaskLspAction(testPage, "kotlin", "start");
    const process = await expectFakeLspGeneration(backend, 1);

    await apiClient.archiveTask(task.taskId);
    await expectFakeLspEventCount(
      backend,
      (event) => event.pid === process.pid && (event.event === "exit" || event.event === "signal"),
      1,
      "task cleanup signal for the task-owned process",
    );
    await expectFakeLspProcessStopped(process.pid);
  });

  test("keeps importing status visible away from the active file", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const statusBarBaseline = await captureAppStatusBarSettings(apiClient);
    try {
      await setAppStatusBarEnabled(apiClient, true);
      await expect
        .poll(async () => (await apiClient.getUserSettings()).settings.app_status_bar_enabled)
        .toBe(true);
      installFakeKotlinLsp(backend, {
        progress: {
          title: "Importing Kotlin project",
          message: LONG_LSP_PROGRESS_MESSAGE,
          percentage: 42,
          endMessage: "Task project model loaded",
        },
      });
      const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
        title: "Task LSP Progress Away From File",
        filePaths: ["Main.kt", "README.md"],
        fileContents: ['fun main() = println("progress")\n', "# Other panel\n"],
      });
      await performTaskLspAction(testPage, "kotlin", "start");
      await expectFakeLspGeneration(backend, 1);

      await task.session.clickTab("Changes");
      const aggregate = testPage.getByTestId("app-status-lsp");
      await expect(aggregate).toBeVisible();
      await expect(aggregate).toContainText(/Kotlin.*Importing|LSP.*running/i);
      const surface = await openTaskLspControl(testPage);
      const kotlin = await expandTaskLspLanguage(surface, "kotlin");
      await expect(kotlin).toContainText(LONG_LSP_PROGRESS_MESSAGE);

      releaseFakeLspInitialization(backend);
      await expectKotlinState(testPage, "ready");
      await performTaskLspAction(testPage, "kotlin", "stop");
    } finally {
      await restoreAppStatusBarSettings(apiClient, statusBarBaseline);
    }
  });

  test("keeps a task control discoverable when the application status bar is disabled", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const statusBarBaseline = await captureAppStatusBarSettings(apiClient);
    try {
      await setAppStatusBarEnabled(apiClient, false);
      installFakeKotlinLsp(backend);
      await createKotlinTask(testPage, apiClient, seedData, backend, {
        title: "Task LSP Status Bar Fallback",
      });

      await expect(testPage.getByTestId("app-status-bar")).toHaveCount(0);
      const trigger = testPage.getByTestId("task-lsp-control");
      await expect(trigger).toBeVisible();
      await expect(trigger).toHaveAttribute("data-lsp-placement", "task-topbar");
      await performTaskLspAction(testPage, "kotlin", "start");
      await expectFakeLspGeneration(backend, 1);
      await performTaskLspAction(testPage, "kotlin", "stop");
    } finally {
      await restoreAppStatusBarSettings(apiClient, statusBarBaseline);
    }
  });

  test("persists status visibility without changing the running task server", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialHidden = Array.isArray(initial.settings.lsp_status_hidden_languages)
      ? (initial.settings.lsp_status_hidden_languages as string[])
      : [];
    let taskId = "";
    let stopped = false;

    try {
      await apiClient.saveUserSettings({
        lsp_status_hidden_languages: initialHidden.filter((language) => language !== "kotlin"),
      });
      installFakeKotlinLsp(backend);
      const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
        title: "Task LSP Status Visibility",
      });
      taskId = task.taskId;
      await performTaskLspAction(testPage, "kotlin", "start");
      const process = await expectFakeLspGeneration(backend, 1);

      await testPage.goto("/settings/general/editors");
      await expect(testPage.getByRole("heading", { name: "Editors", exact: true })).toBeVisible();
      const visibility = testPage.getByTestId("lsp-status-visible-kotlin");
      await expect(visibility).toBeChecked();
      await visibility.click();
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).toBeHidden({ timeout: 15_000 });
      await expect
        .poll(async () => {
          const hidden = (await apiClient.getUserSettings()).settings.lsp_status_hidden_languages;
          return Array.isArray(hidden) && hidden.includes("kotlin");
        })
        .toBe(true);

      await testPage.goto(`/t/${task.taskId}`);
      await task.session.waitForLoad(45_000);
      let surface = await openTaskLspControl(testPage);
      await expect(surface.getByTestId("task-lsp-language-kotlin")).toHaveCount(0);
      await expectFakeLspEventCount(
        backend,
        (event) => event.event === "started",
        1,
        "hiding status leaves the running generation unchanged",
      );

      await testPage.reload();
      await task.session.waitForLoad(45_000);
      surface = await openTaskLspControl(testPage);
      await expect(surface.getByTestId("task-lsp-language-kotlin")).toHaveCount(0);
      await testPage.keyboard.press("Escape");

      await openDesktopFile(testPage, task.session, task.filePaths[0]);
      const editorShortcut = testPage.getByTestId("lsp-status-button");
      await expect(editorShortcut).toHaveAttribute("data-lsp-language", "kotlin");
      await editorShortcut.click();
      const editorSurface = testPage.getByTestId("task-lsp-surface");
      const kotlin = editorSurface.getByTestId("task-lsp-language-kotlin");
      await expect(kotlin).toHaveAttribute("data-lsp-generation", "1");
      await expect(kotlin.getByTestId("task-lsp-language-trigger-kotlin")).toHaveAttribute(
        "aria-expanded",
        "true",
      );

      await apiClient.rawRequest("POST", `/api/v1/tasks/${task.taskId}/lsp/kotlin/stop`);
      await expectFakeLspProcessStopped(process.pid);
      stopped = true;
    } finally {
      if (taskId && !stopped) {
        await apiClient
          .rawRequest("POST", `/api/v1/tasks/${taskId}/lsp/kotlin/stop`)
          .catch(() => undefined);
      }
      await apiClient.saveUserSettings({ lsp_status_hidden_languages: initialHidden });
    }
  });
});

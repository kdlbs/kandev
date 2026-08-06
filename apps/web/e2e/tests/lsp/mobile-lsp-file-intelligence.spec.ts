import { test, expect } from "../../fixtures/test-base";
import {
  assertNoDocumentHorizontalOverflow,
  assertNoElementHorizontalOverflow,
} from "../../helpers/layout-assertions";
import {
  createKotlinTask,
  expandTaskLspLanguage,
  installFakeKotlinLsp,
  LONG_LSP_PROGRESS_MESSAGE,
  readFakeLspEvents,
  releaseFakeLspInitialization,
} from "./lsp-e2e-helpers";
import { dwell } from "../../helpers/causal-waits";

test.describe("Mobile LSP boundaries", () => {
  test.describe.configure({ timeout: 90_000 });

  test("keeps task LSP controls available without letting the mobile viewer start a server", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialAutoStart = Array.isArray(initial.settings.lsp_auto_start_languages)
      ? (initial.settings.lsp_auto_start_languages as string[])
      : [];
    const initialLocation =
      initial.settings.lsp_status_location === "status_bar" ? "status_bar" : "toolbar";
    const initialHidden = Array.isArray(initial.settings.lsp_status_hidden_languages)
      ? (initial.settings.lsp_status_hidden_languages as string[])
      : [];

    try {
      installFakeKotlinLsp(backend);
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_auto_start_languages: [],
        lsp_status_hidden_languages: initialHidden.filter((language) => language !== "kotlin"),
        lsp_status_location: "status_bar",
      });

      const lspSockets: string[] = [];
      testPage.on("websocket", (socket) => {
        if (socket.url().includes("/lsp/")) lspSockets.push(socket.url());
      });
      const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
        title: "Mobile Kotlin LSP Boundary",
      });

      await testPage.getByRole("button", { name: "Files" }).tap();
      const fileNode = testPage.locator(
        `[data-testid="file-tree-node"][data-path="${task.filePaths[0]}"]`,
      );
      await expect(fileNode).toBeVisible({ timeout: 15_000 });
      await fileNode.tap();

      const viewer = testPage.getByTestId("mobile-file-viewer-panel");
      await expect(viewer).toBeVisible();
      await expect(viewer.locator(".cm-editor")).toBeVisible();
      await expect(viewer.getByTestId("lsp-status-button")).toHaveCount(0);

      const statusTrigger = testPage.getByRole("button", { name: "Status", exact: true });
      await expect(statusTrigger).toBeVisible();
      const triggerBox = await statusTrigger.boundingBox();
      expect(triggerBox?.height).toBeGreaterThanOrEqual(44);
      await statusTrigger.tap();

      const drawer = testPage.getByTestId("app-status-drawer");
      await expect(drawer).toBeVisible();
      const kotlin = drawer.getByTestId("task-lsp-language-kotlin");
      await expect(kotlin).toBeVisible();
      await expect(kotlin).toHaveAttribute("data-lsp-policy", "inherit");
      const languageTrigger = kotlin.getByTestId("task-lsp-language-trigger-kotlin");
      await expect(languageTrigger).toHaveAttribute("aria-expanded", "false");
      expect((await languageTrigger.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await languageTrigger.tap();
      await expect(languageTrigger).toHaveAttribute("aria-expanded", "true");
      const start = kotlin.locator('[data-testid="lsp-lifecycle-action"][data-lsp-action="start"]');
      await expect(start).toBeVisible();
      const startBox = await start.boundingBox();
      expect(startBox?.height).toBeGreaterThanOrEqual(44);
      await expect(testPage.getByTestId("lsp-status-drawer")).toHaveCount(0);
      await expect(testPage.getByTestId("app-status-drawer-scroll-region")).toHaveCount(1);
      await assertNoElementHorizontalOverflow(drawer, "phone task LSP status drawer");
      await assertNoDocumentHorizontalOverflow(testPage, "phone task LSP status drawer");

      await dwell(
        testPage,
        1_000,
        "negative-assertion",
        "asserts that no LSP socket is ever opened on mobile; a connection that must not happen publishes nothing, so the check needs the window in which it would have opened to elapse",
      );
      expect(lspSockets).toEqual([]);
      expect(readFakeLspEvents(backend)).toEqual([]);
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_auto_start_languages: initialAutoStart,
        lsp_status_hidden_languages: initialHidden,
        lsp_status_location: initialLocation,
      });
    }
  });

  test("shows the same project progress in a contained tablet drawer", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialLocation =
      initial.settings.lsp_status_location === "status_bar" ? "status_bar" : "toolbar";
    const initialHidden = Array.isArray(initial.settings.lsp_status_hidden_languages)
      ? (initial.settings.lsp_status_hidden_languages as string[])
      : [];

    try {
      await testPage.setViewportSize({ width: 820, height: 900 });
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_status_hidden_languages: initialHidden.filter((language) => language !== "kotlin"),
        lsp_status_location: "status_bar",
      });
      installFakeKotlinLsp(backend, {
        progress: {
          title: "Importing Kotlin project",
          message: LONG_LSP_PROGRESS_MESSAGE,
          percentage: 42,
          endMessage: "Tablet project model loaded",
        },
      });
      const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
        title: "Tablet Kotlin LSP Progress",
      });
      const environment = await apiClient.getTaskEnvironment(task.taskId);
      expect(["local", "worktree", "local_pc", "local_docker"]).toContain(
        environment?.executor_type,
      );
      await expect(testPage.getByTestId("tablet-task-layout")).toBeVisible();
      const fileNode = testPage.locator(
        `[data-testid="file-tree-node"][data-path="${task.filePaths[0]}"]`,
      );
      await expect(fileNode).toBeVisible({ timeout: 15_000 });
      await fileNode.tap();
      await expect(testPage.locator(".monaco-editor:visible")).toBeVisible({ timeout: 15_000 });

      const statusButton = testPage.getByTestId("task-lsp-control");
      await expect(statusButton).toBeVisible();
      await expect(testPage.getByTestId("app-status-lsp")).toHaveCount(0);
      expect((await apiClient.getUserSettings()).settings.lsp_status_location).toBe("status_bar");
      const triggerBox = await statusButton.boundingBox();
      expect(triggerBox?.width).toBeGreaterThanOrEqual(44);
      expect(triggerBox?.height).toBeGreaterThanOrEqual(44);
      await statusButton.tap();

      const drawer = testPage.getByTestId("app-status-drawer");
      await expect(drawer).toBeVisible();
      const kotlin = drawer.getByTestId("task-lsp-language-kotlin");
      await expect(kotlin).toHaveAttribute("data-lsp-policy", "inherit");
      await expandTaskLspLanguage(drawer.getByTestId("task-lsp-disclosure"), "kotlin");
      await kotlin.locator('[data-testid="lsp-lifecycle-action"][data-lsp-action="start"]').tap();
      await expect(kotlin).toHaveAttribute("data-lsp-generation", "1", { timeout: 15_000 });
      await expect
        .poll(() =>
          readFakeLspEvents(backend).some(
            (event) => event.event === "initialize progress reported",
          ),
        )
        .toBe(true);
      await expect(testPage.getByTestId("lsp-status-drawer")).toHaveCount(0);
      await expect(testPage.getByTestId("lsp-status-popover")).toHaveCount(0);
      const projectProgress = kotlin.getByTestId("task-lsp-progress");
      await expect(projectProgress).toContainText("Importing Kotlin project");
      const progressMessage = projectProgress.getByText(LONG_LSP_PROGRESS_MESSAGE, {
        exact: true,
      });
      await expect(progressMessage).toBeVisible();
      await expect(projectProgress).toContainText("42%");

      const drawerBox = await drawer.boundingBox();
      expect(drawerBox).not.toBeNull();
      expect(drawerBox!.x).toBeGreaterThanOrEqual(0);
      expect(drawerBox!.y).toBeGreaterThanOrEqual(0);
      expect(drawerBox!.x + drawerBox!.width).toBeLessThanOrEqual(820);
      expect(drawerBox!.y + drawerBox!.height).toBeLessThanOrEqual(900);
      const actionBox = await kotlin
        .locator('[data-testid="lsp-lifecycle-action"][data-lsp-action="stop"]')
        .boundingBox();
      expect(actionBox?.height).toBeGreaterThanOrEqual(44);
      const verticalScrollOwner = drawer.locator("[data-vaul-no-drag]");
      await expect(verticalScrollOwner).toHaveCount(1);
      await expect(verticalScrollOwner).toHaveCSS("overflow-y", "auto");
      await assertNoElementHorizontalOverflow(progressMessage, "tablet LSP progress text");
      await assertNoElementHorizontalOverflow(drawer, "tablet LSP progress drawer");
      await assertNoDocumentHorizontalOverflow(testPage, "tablet LSP progress drawer");

      releaseFakeLspInitialization(backend);
      await expect(kotlin).toHaveAttribute("data-lsp-state", "ready", { timeout: 15_000 });
      await expect(kotlin).toContainText("Tablet project model loaded");
      await kotlin.locator('[data-testid="lsp-lifecycle-action"][data-lsp-action="stop"]').tap();
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_status_hidden_languages: initialHidden,
        lsp_status_location: initialLocation,
      });
    }
  });

  test("persists Kotlin defaults with usable mobile guidance", async ({ testPage, apiClient }) => {
    const initial = await apiClient.getUserSettings();
    const initialAutoStart = Array.isArray(initial.settings.lsp_auto_start_languages)
      ? (initial.settings.lsp_auto_start_languages as string[])
      : [];
    const initialLocation =
      initial.settings.lsp_status_location === "status_bar" ? "status_bar" : "toolbar";
    const initialHidden = Array.isArray(initial.settings.lsp_status_hidden_languages)
      ? (initial.settings.lsp_status_hidden_languages as string[])
      : [];

    try {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_auto_start_languages: [],
        lsp_status_hidden_languages: initialHidden.filter((language) => language !== "kotlin"),
        lsp_status_location: "toolbar",
      });
      await testPage.goto("/settings/preferences/terminal-editors");
      await expect(testPage.getByRole("heading", { name: "Editors", exact: true })).toBeVisible();

      const kotlinCard = testPage.getByTestId("lsp-language-card-kotlin");
      await expect(kotlinCard).toBeVisible();
      await expect(kotlinCard.getByTestId("lsp-install-guidance-kotlin")).toContainText(
        "inside the task container",
      );
      await expect(
        testPage.getByText("the mobile file viewer does not start them", { exact: false }),
      ).toBeVisible();
      await expect(testPage.getByRole("radio", { name: /Application status bar/ })).toHaveCount(0);

      const autoStart = kotlinCard.getByTestId("lsp-auto-start-kotlin");
      await expect(autoStart).not.toBeChecked();
      await autoStart.tap();
      const statusVisibility = kotlinCard.getByTestId("lsp-status-visible-kotlin");
      await expect(statusVisibility).toBeChecked();
      await statusVisibility.tap();
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).tap();
      await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });

      expect(
        (
          (await apiClient.getUserSettings()).settings.lsp_auto_start_languages as string[]
        ).includes("kotlin"),
      ).toBe(true);
      expect(
        (
          (await apiClient.getUserSettings()).settings.lsp_status_hidden_languages as string[]
        ).includes("kotlin"),
      ).toBe(true);
      await testPage.reload();
      await expect(testPage.getByTestId("lsp-auto-start-kotlin")).toBeChecked();
      await expect(testPage.getByTestId("lsp-status-visible-kotlin")).not.toBeChecked();
      await assertNoDocumentHorizontalOverflow(testPage, "mobile Editors settings");
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_auto_start_languages: initialAutoStart,
        lsp_status_hidden_languages: initialHidden,
        lsp_status_location: initialLocation,
      });
    }
  });
});

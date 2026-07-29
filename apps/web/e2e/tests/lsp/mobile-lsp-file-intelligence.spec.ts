import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  createKotlinTask,
  installFakeKotlinLsp,
  openLspStatus,
  performLspAction,
  readFakeLspEvents,
  releaseFakeLspInitialization,
} from "./lsp-e2e-helpers";

test.describe("Mobile LSP boundaries", () => {
  test.describe.configure({ timeout: 90_000 });

  test("opens Kotlin in the mobile viewer without starting a language server", async ({
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

    try {
      installFakeKotlinLsp(backend);
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_auto_start_languages: ["kotlin"],
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
      await expect(testPage.getByTestId("app-status-lsp")).toHaveCount(0);
      await testPage.waitForTimeout(1_000);
      expect(lspSockets).toEqual([]);
      expect(readFakeLspEvents(backend)).toEqual([]);
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_auto_start_languages: initialAutoStart,
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

    try {
      await testPage.setViewportSize({ width: 820, height: 900 });
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_status_location: "status_bar",
      });
      installFakeKotlinLsp(backend, {
        progress: {
          title: "Importing Kotlin project",
          message: "Resolving tablet modules",
          percentage: 42,
          endMessage: "Tablet project model loaded",
        },
      });
      const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
        title: "Tablet Kotlin LSP Progress",
      });
      await expect(testPage.getByTestId("tablet-task-layout")).toBeVisible();
      const fileNode = testPage.locator(
        `[data-testid="file-tree-node"][data-path="${task.filePaths[0]}"]`,
      );
      await expect(fileNode).toBeVisible({ timeout: 15_000 });
      await fileNode.tap();
      await expect(testPage.locator(".monaco-editor:visible")).toBeVisible({ timeout: 15_000 });

      const statusButton = testPage.getByTestId("lsp-status-button");
      await expect(statusButton).toBeVisible();
      await expect(testPage.getByTestId("app-status-lsp")).toHaveCount(0);
      expect((await apiClient.getUserSettings()).settings.lsp_status_location).toBe("status_bar");
      const triggerBox = await statusButton.boundingBox();
      expect(triggerBox?.width).toBeGreaterThanOrEqual(44);
      expect(triggerBox?.height).toBeGreaterThanOrEqual(44);
      await performLspAction(testPage, "start");

      const drawer = await openLspStatus(testPage);
      await expect(drawer).toHaveAttribute("data-testid", "lsp-status-drawer");
      await expect(testPage.getByTestId("lsp-status-popover")).toHaveCount(0);
      const projectProgress = drawer.getByTestId("lsp-project-progress");
      await expect(projectProgress).toHaveAttribute("data-lsp-progress-kind", "active", {
        timeout: 15_000,
      });
      await expect(projectProgress).toContainText("Importing Kotlin project");
      await expect(projectProgress).toContainText("Resolving tablet modules");
      await expect(projectProgress).toContainText("42%");

      const drawerBox = await drawer.boundingBox();
      expect(drawerBox).not.toBeNull();
      expect(drawerBox!.x).toBeGreaterThanOrEqual(0);
      expect(drawerBox!.y).toBeGreaterThanOrEqual(0);
      expect(drawerBox!.x + drawerBox!.width).toBeLessThanOrEqual(820);
      expect(drawerBox!.y + drawerBox!.height).toBeLessThanOrEqual(900);
      const actionBox = await drawer.getByTestId("lsp-lifecycle-action").boundingBox();
      expect(actionBox?.height).toBeGreaterThanOrEqual(44);
      await assertNoDocumentHorizontalOverflow(testPage, "tablet LSP progress drawer");

      releaseFakeLspInitialization(backend);
      await expect(projectProgress).toHaveAttribute("data-lsp-progress-kind", "completed", {
        timeout: 15_000,
      });
      await expect(projectProgress).toContainText("Tablet project model loaded");
      await performLspAction(testPage, "stop");
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_status_location: initialLocation,
      });
    }
  });

  test("persists Kotlin settings with usable mobile guidance", async ({ testPage, apiClient }) => {
    const initial = await apiClient.getUserSettings();
    const initialAutoStart = Array.isArray(initial.settings.lsp_auto_start_languages)
      ? (initial.settings.lsp_auto_start_languages as string[])
      : [];
    const initialLocation =
      initial.settings.lsp_status_location === "status_bar" ? "status_bar" : "toolbar";

    try {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_auto_start_languages: [],
        lsp_status_location: "toolbar",
      });
      await testPage.goto("/settings/general/editors");
      await expect(testPage.getByRole("heading", { name: "Editors", exact: true })).toBeVisible();

      const kotlinCard = testPage.getByTestId("lsp-language-card-kotlin");
      await expect(kotlinCard).toBeVisible();
      await expect(kotlinCard.getByTestId("lsp-install-guidance-kotlin")).toContainText(
        "inside the task container",
      );
      await expect(
        testPage.getByText("the mobile file viewer does not start them", { exact: false }),
      ).toBeVisible();
      await expect(testPage.getByRole("radio", { name: /Application status bar/ })).toBeVisible();
      await expect(testPage.getByText(/touch-oriented layout is active/)).toBeVisible();

      const autoStart = kotlinCard.getByTestId("lsp-auto-start-kotlin");
      await expect(autoStart).not.toBeChecked();
      await autoStart.tap();
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).tap();
      await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });

      expect(
        (
          (await apiClient.getUserSettings()).settings.lsp_auto_start_languages as string[]
        ).includes("kotlin"),
      ).toBe(true);
      await testPage.reload();
      await expect(testPage.getByTestId("lsp-auto-start-kotlin")).toBeChecked();
      await assertNoDocumentHorizontalOverflow(testPage, "mobile Editors settings");
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        lsp_auto_start_languages: initialAutoStart,
        lsp_status_location: initialLocation,
      });
    }
  });
});

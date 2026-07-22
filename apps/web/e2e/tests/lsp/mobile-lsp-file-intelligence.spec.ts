import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { createKotlinTask, installFakeKotlinLsp, readFakeLspEvents } from "./lsp-e2e-helpers";

test.describe("Mobile LSP boundaries", () => {
  test.describe.configure({ timeout: 90_000 });

  test("opens Kotlin in the mobile viewer without starting a language server", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    installFakeKotlinLsp(backend);
    await apiClient.saveUserSettings({ lsp_auto_start_languages: ["kotlin"] });

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
    await testPage.waitForTimeout(1_000);
    expect(lspSockets).toEqual([]);
    expect(readFakeLspEvents(backend)).toEqual([]);
  });

  test("persists Kotlin settings with usable mobile guidance", async ({ testPage, apiClient }) => {
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

    const autoStart = kotlinCard.getByTestId("lsp-auto-start-kotlin");
    await expect(autoStart).not.toBeChecked();
    await autoStart.tap();
    const floatingSave = testPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).tap();
    await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });

    expect(
      ((await apiClient.getUserSettings()).settings.lsp_auto_start_languages as string[]).includes(
        "kotlin",
      ),
    ).toBe(true);
    await testPage.reload();
    await expect(testPage.getByTestId("lsp-auto-start-kotlin")).toBeChecked();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Editors settings");
  });
});

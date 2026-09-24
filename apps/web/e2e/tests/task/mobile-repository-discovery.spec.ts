import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import {
  DISCOVERY_FAILURE_ROOT,
  installRepositoryDiscoveryFailureRoute,
} from "./repository-discovery-failure-helpers";
import fs from "node:fs";

useRegularMode();

test.describe("Mobile repository discovery consent", () => {
  test("shows discovery actions only inside the repository selector with matched touch targets", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(120_000);
    await backend.restart({ KANDEV_DESKTOP_RUNTIME: "true" });
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("discovery-root-controls")).toHaveCount(0);

    await dialog.getByTestId("mobile-repository-manager").tap();
    const management = testPage.getByTestId("mobile-repository-management");
    const repositoryChip = management.getByTestId("repo-chip-trigger").first();
    await expect(repositoryChip).toBeVisible();
    await repositoryChip.tap();
    const controls = testPage.getByTestId("discovery-root-controls");
    const chooseFolders = controls.getByTestId("folder-picker-trigger");
    const refreshRepositories = controls.getByRole("button", {
      name: "Refresh repositories",
    });
    await expect(controls).toBeVisible();
    await waitForFiniteAnimations(controls);

    const [chooseBox, refreshBox] = await Promise.all([
      chooseFolders.boundingBox(),
      refreshRepositories.boundingBox(),
    ]);
    const [chooseCssHeight, refreshCssHeight] = await Promise.all([
      chooseFolders.evaluate((element) => Number.parseFloat(getComputedStyle(element).height)),
      refreshRepositories.evaluate((element) =>
        Number.parseFloat(getComputedStyle(element).height),
      ),
    ]);
    expect(chooseBox).not.toBeNull();
    expect(refreshBox).not.toBeNull();
    expect(refreshBox!.height).toBeCloseTo(chooseBox!.height, 1);
    expect(chooseCssHeight).toBe(44);
    expect(refreshCssHeight).toBe(44);
  });

  test("keeps failed-root diagnostics out of the phone selector", async ({ testPage, backend }) => {
    test.setTimeout(120_000);
    await installRepositoryDiscoveryFailureRoute(testPage);
    await backend.restart({ KANDEV_DESKTOP_RUNTIME: "false" });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("mobile-repository-manager").tap();
    const repositorySheet = testPage.getByTestId("mobile-repository-sheet-content").locator("..");
    await expect(repositorySheet).toBeVisible();
    await waitForFiniteAnimations(repositorySheet);
    const management = testPage.getByTestId("mobile-repository-management");
    const repositoryChip = management.getByTestId("repo-chip-trigger").first();
    await expect(repositoryChip).toBeVisible();
    await repositoryChip.tap();
    const repositoryPicker = testPage.getByRole("dialog", { name: "Repository" });
    await expect(repositoryPicker).toBeVisible();
    await waitForFiniteAnimations(repositoryPicker);

    await expect(testPage.getByTestId("discovery-root-controls")).toHaveCount(0);
    await expect(testPage.getByTestId("discovery-failure")).toHaveCount(0);
    await expect(testPage.getByText(DISCOVERY_FAILURE_ROOT, { exact: true })).toHaveCount(0);
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    const refresh = testPage.getByTestId("repo-refresh-button");
    const refreshBox = await refresh.boundingBox();
    expect(refreshBox).not.toBeNull();
    expect(refreshBox!.y).toBeGreaterThanOrEqual(0);
    expect(refreshBox!.y + refreshBox!.height).toBeLessThanOrEqual(viewport!.height);
    const availableRepository = testPage.getByRole("option", { name: /healthy-project/ });
    await expect(availableRepository).toBeEnabled();
    const availableRepositoryBox = await availableRepository.boundingBox();
    expect(availableRepositoryBox).not.toBeNull();
    expect(availableRepositoryBox!.y).toBeGreaterThanOrEqual(0);
    expect(availableRepositoryBox!.y + availableRepositoryBox!.height).toBeLessThanOrEqual(
      viewport!.height,
    );
    await availableRepository.tap();
    await expect(repositoryChip).toContainText("healthy-project");

    await repositoryChip.tap();
    await expect(repositoryPicker).toBeVisible();
    await waitForFiniteAnimations(repositoryPicker);
    const refreshResponse = testPage.waitForResponse(
      (response) =>
        /\/api\/v1\/workspaces\/[^/]+\/repositories$/.test(new URL(response.url()).pathname) &&
        response.request().method() === "GET" &&
        response.ok(),
    );
    expect(refreshBox!.height).toBeGreaterThanOrEqual(44);
    await refresh.tap();
    await refreshResponse;
    await expect(testPage.getByTestId("discovery-failure")).toHaveCount(0);
    await expect(testPage.getByRole("option", { name: /healthy-project/ })).toBeVisible();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });

  test("uses the HTTP picker on a mobile browser connected to a desktop backend", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const homePath = fs.realpathSync(backend.tmpDir);
    let rootSaved = false;

    try {
      await backend.restart({ KANDEV_DESKTOP_RUNTIME: "true" });
      await testPage.setViewportSize({ width: 390, height: 844 });
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
      await testPage.getByRole("button", { name: "Add Local Repository" }).click();

      const dialog = testPage.getByRole("dialog", { name: "Add Local Repository" });
      const controls = dialog.getByTestId("discovery-root-controls");
      await expect(controls).toBeVisible();

      const listingResponse = testPage.waitForResponse(
        (response) =>
          response.url().includes("/api/v1/fs/list-dir") &&
          response.request().method() === "GET" &&
          response.ok(),
      );
      await controls.getByTestId("folder-picker-trigger").tap();
      await listingResponse;
      const picker = testPage.getByTestId("folder-picker-popover");
      await expect(picker).toBeVisible();
      await expect(picker.getByTestId("folder-picker-choose")).toBeEnabled();

      const addResponse = testPage.waitForResponse(
        (response) =>
          response.url().endsWith("/api/v1/repositories/discovery/roots") &&
          response.request().method() === "POST" &&
          response.ok(),
      );
      await picker.getByTestId("folder-picker-choose").tap();
      expect((await addResponse).status()).toBe(201);
      rootSaved = true;

      await expect(controls.getByRole("button", { name: "Refresh repositories" })).toBeVisible();
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    } finally {
      if (rootSaved) {
        await apiClient
          .rawRequest(
            "DELETE",
            `/api/v1/repositories/discovery/roots?path=${encodeURIComponent(homePath)}`,
          )
          .catch(() => undefined);
      }
      await backend.restart();
    }
  });
});

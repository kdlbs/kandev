import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import {
  DISCOVERY_FAILURE_ROOT,
  DISCOVERY_FAILURE_ROOTS,
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

    await dialog.getByTestId("repo-chip-trigger").first().tap();
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

  test("keeps available repositories usable and recovers a failed root on a phone", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(120_000);
    await installRepositoryDiscoveryFailureRoute(testPage);
    await backend.restart({ KANDEV_DESKTOP_RUNTIME: "false" });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("repo-chip-trigger").first().tap();

    const controls = testPage.getByTestId("discovery-root-controls");
    await expect(controls.getByTestId("discovery-failure")).toContainText(DISCOVERY_FAILURE_ROOT);
    await expect(controls.getByRole("listitem")).toHaveCount(DISCOVERY_FAILURE_ROOTS.length);
    const failedPath = controls.getByTitle(DISCOVERY_FAILURE_ROOT);
    await expect(failedPath).toBeVisible();
    await expect(failedPath).toHaveCSS("word-break", "break-all");
    const picker = controls.locator("..");
    const pickerBox = await picker.boundingBox();
    const viewport = testPage.viewportSize();
    expect(pickerBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(pickerBox!.y).toBeGreaterThanOrEqual(0);
    expect(pickerBox!.y + pickerBox!.height).toBeLessThanOrEqual(viewport!.height);
    const failedRootList = controls.locator("ul");
    await expect(failedRootList).toHaveCSS("overflow-y", "auto");
    const failedRootListMetrics = await failedRootList.evaluate((element) => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
    }));
    expect(failedRootListMetrics.scrollHeight).toBeGreaterThan(failedRootListMetrics.clientHeight);
    const refresh = controls.getByTestId("discovery-failure-refresh");
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
    await expect(dialog.getByTestId("repo-chip-trigger").first()).toContainText("healthy-project");

    await dialog.getByTestId("repo-chip-trigger").first().tap();
    const refreshResponse = testPage.waitForResponse(
      (response) =>
        response.url().includes("/api/v1/workspaces/") &&
        response.url().includes("/repositories/discovery/refresh") &&
        response.request().method() === "POST" &&
        response.ok(),
    );
    await expect(refresh).toHaveCSS("height", "44px");
    await refresh.tap();
    await refreshResponse;
    await expect(testPage.getByTestId("discovery-failure")).toHaveCount(0);
    await expect(testPage.getByRole("option", { name: /recovered-project/ })).toBeVisible();
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

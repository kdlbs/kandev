import { test, expect } from "../../fixtures/test-base";
import {
  expectStorageActionHitTarget,
  expectStorageActionsFit,
  expectStorageActionStatusNodesRetained,
  markStorageActionStatusNodes,
  mockStorageActionJobs,
} from "../../helpers/storage-actions-layout";
import { setStoreRole } from "../../helpers/session-store";

// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.1
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.2
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.6
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.8
test("fits actions inside a constrained Storage section", async ({ testPage, prCapture }) => {
  await testPage.emulateMedia({ colorScheme: "dark" });
  await mockStorageActionJobs(testPage);
  await testPage.setViewportSize({ width: 1024, height: 900 });
  await testPage.goto("/settings/system/storage");
  const analyze = testPage.getByTestId("storage-analyze");
  await expect(analyze).toBeVisible();
  const cell = await testPage.getByTestId("storage-analyze-control").boundingBox();
  const button = await analyze.boundingBox();
  expect(cell).not.toBeNull();
  expect(button).not.toBeNull();
  expect(button!.x + button!.width).toBeLessThanOrEqual(cell!.x + cell!.width + 1);
  await expectStorageActionsFit(testPage);
  await prCapture.screenshot("constrained-desktop", {
    caption: "Storage actions move below their explanation in a constrained desktop section",
  });

  for (const width of [639, 640, 641, 767, 768, 769, 1400]) {
    await testPage.setViewportSize({ width, height: 900 });
    await expectStorageActionsFit(testPage);
    await expectStorageActionHitTarget(testPage, "storage-analyze");
    await expectStorageActionHitTarget(testPage, "storage-run-now");
  }
  await testPage.getByTestId("system-page-title").scrollIntoViewIfNeeded();
  await prCapture.screenshot("wide-desktop", {
    caption: "Wide Storage sections keep the explanation and action buttons beside each other",
  });

  const section = testPage.getByTestId("storage-settings-page");
  for (const width of [383, 384, 385, 831, 832, 833]) {
    await section.evaluate((element, value) => (element.style.maxWidth = `${value}px`), width);
    await expectStorageActionsFit(testPage);
  }
  await section.evaluate((element) => element.style.removeProperty("max-width"));
});

// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.3
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.6
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.7
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.12
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.13
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.14
test("contains job labels and disabled wrappers", async ({ testPage, prCapture }) => {
  const jobs = await mockStorageActionJobs(testPage);
  await testPage.setViewportSize({ width: 1024, height: 900 });
  await testPage.goto("/settings/preferences/appearance");
  await testPage.getByLabel("Display language").click();
  await testPage
    .getByRole("listbox")
    .getByRole("option", { name: /Português/ })
    .click();
  await expect(testPage.locator("html")).toHaveAttribute("lang", "pt-pt");
  try {
    await testPage.goto("/settings/system/storage");
    await markStorageActionStatusNodes(testPage);
    jobs.setState("queued");
    await testPage.getByTestId("storage-analyze").click();
    await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
      "data-job-state",
      "queued",
    );
    await testPage.getByTestId("storage-run-now").click();
    const actions = [
      {
        id: "storage-analyze",
        control: "storage-analyze-control",
        name: "Analisar",
        status: "A analisar...",
        failed: "A análise falhou",
      },
      {
        id: "storage-run-now",
        control: "storage-cleanup-control",
        name: "Executar agora",
        status: "A limpar...",
        failed: "A limpeza falhou",
      },
    ];
    for (const state of ["queued", "running", "failed"] as const) {
      jobs.setState(state);
      for (const action of actions) {
        const button = testPage.getByTestId(action.id);
        await expect(button).toHaveAttribute("data-job-state", state);
        const label = state === "failed" ? action.failed : action.status;
        const status = testPage.getByTestId(action.control).getByRole("status");
        await expect(button).toHaveText(label);
        await expect(status).toHaveText(label);
        await expect(status).toHaveCount(1);
        await expect(button.getByRole("status")).toHaveCount(0);
        await expect(button).toHaveAccessibleName(action.name);
      }
      await expectStorageActionsFit(testPage);
      await expectStorageActionStatusNodesRetained(testPage);
    }
    jobs.setState("succeeded");
    await testPage.getByTestId("storage-analyze").click();
    await expect(testPage.getByTestId("storage-analyze")).toHaveText("Análise concluída");
    await testPage.getByTestId("storage-run-now").click();
    await expect(testPage.getByTestId("storage-run-now")).toHaveText("Limpeza concluída");
    await expect(testPage.getByTestId("storage-analyze-control").getByRole("status")).toHaveText(
      "Análise concluída",
    );
    await expect(testPage.getByTestId("storage-cleanup-control").getByRole("status")).toHaveText(
      "Limpeza concluída",
    );
    await expectStorageActionsFit(testPage);
    await expectStorageActionStatusNodesRetained(testPage);
    await prCapture.screenshot("translated-completed", {
      caption: "Portuguese completion labels fit the same Storage action cells",
    });
    await setStoreRole(testPage, "member");
    await expect(testPage.getByTestId("storage-analyze")).toBeDisabled();
    await expect(testPage.getByTestId("storage-run-now")).toBeDisabled();
    await expectStorageActionsFit(testPage);
    await expectStorageActionStatusNodesRetained(testPage);
  } finally {
    await setStoreRole(testPage, "admin");
    await testPage.context().clearCookies({ name: "kandev_locale" });
  }
});

// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.9
test("retains keyboard actions after resizing", async ({ testPage }) => {
  const jobs = await mockStorageActionJobs(testPage);
  await testPage.setViewportSize({ width: 1400, height: 900 });
  await testPage.goto("/settings/system/storage");
  const hostTab = testPage.getByRole("tab", { name: "Host", exact: true });
  await hostTab.click();
  await testPage.keyboard.press("Tab");
  await expect(testPage.getByTestId("settings-storage-host")).toBeFocused();
  await testPage.keyboard.press("Tab");
  await expect(testPage.getByTestId("storage-analyze")).toBeFocused();
  await testPage.setViewportSize({ width: 1024, height: 900 });
  await expectStorageActionsFit(testPage);
  await expect(testPage.getByTestId("storage-analyze")).toBeFocused();
  const analyzeRequest = testPage.waitForRequest("**/api/v1/system/storage/analyze");
  await testPage.keyboard.press("Enter");
  expect((await analyzeRequest).method()).toBe("POST");
  await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
    "data-job-state",
    "succeeded",
  );
  await hostTab.click();
  await testPage.keyboard.press("Tab");
  await testPage.keyboard.press("Tab");
  await testPage.keyboard.press("Tab");
  await expect(testPage.getByTestId("storage-run-now")).toBeFocused();
  const cleanupRequest = testPage.waitForRequest("**/api/v1/system/storage/run");
  await testPage.keyboard.press("Space");
  expect((await cleanupRequest).postDataJSON()).toEqual({});
  await expect(testPage.getByTestId("storage-run-now")).toHaveAttribute(
    "data-job-state",
    "succeeded",
  );
  expect(jobs.requests).toEqual({ analyze: 1, run: 1 });
});

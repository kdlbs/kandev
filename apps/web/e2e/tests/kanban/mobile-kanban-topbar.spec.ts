import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
import { waitForHttp } from "../../helpers/causal-waits";
import { createStandardProfile, openTaskSession } from "../../helpers/git-helper";
import { waitForLatestSessionDone } from "../../helpers/session";

test.describe("Shared phone listing topbar", () => {
  test("returns focus to the menu button when search is hidden", async ({ testPage }) => {
    await testPage.goto("/tasks");
    const opener = testPage.getByTestId("mobile-topbar-menu");
    const menu = testPage.getByRole("dialog", { name: "Menu", exact: true });
    const search = testPage.getByPlaceholder("Search tasks...");
    await opener.tap();
    await menu.getByTestId("mobile-search-toggle").tap();
    await expect(menu).toBeHidden();
    await expect(search).toBeFocused();
    await search.fill("checkout");
    await opener.tap();
    await menu.getByTestId("mobile-search-toggle").tap();
    await expect(menu).toBeHidden();
    await expect(search).toBeHidden();
    await expect(opener).toBeFocused();
    await opener.tap();
    await menu.getByTestId("mobile-search-toggle").tap();
    await expect(search).toHaveValue("");
    await expect(search).toBeFocused();
  });

  test("keeps Home but does not duplicate Threads in the tablet menu", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 820, height: 1180 });
    await testPage.goto("/threads");
    await testPage
      .locator("header")
      .first()
      .getByRole("button", { name: "Open menu", exact: true })
      .tap();
    const menu = testPage.getByRole("dialog", { name: "Menu", exact: true });
    await expect(menu.getByRole("link", { name: "Home", exact: true })).toBeVisible();
    await expect(menu.getByRole("radio", { name: "Threads", exact: true })).toHaveAttribute(
      "data-state",
      "on",
    );
    await expect(menu.getByRole("link", { name: "Threads", exact: true })).toHaveCount(0);
  });

  test("keeps saved-view sync recovery inside the drawer without crowding the topbar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profile = await createStandardProfile(apiClient, "mobile-topbar-recovery");
    for (const title of ["First recovery thread", "Second recovery thread"]) {
      const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
      await openTaskSession(testPage, title);
      await waitForLatestSessionDone(apiClient, task.id, 1, `agent turn for ${title}`);
    }
    await testPage.setViewportSize({ width: 360, height: 851 });
    await testPage.goto("/threads");
    let rejectWrites = true;
    await testPage.route("**/api/v1/user/settings", async (route) => {
      const request = route.request();
      if (rejectWrites && request.method() === "PATCH" && request.postDataJSON()?.thread_views) {
        await route.fulfill({ status: 500, json: { error: "Saved views unavailable" } });
      } else {
        await route.continue();
      }
    });

    const header = testPage.getByTestId("threads-mobile-topbar");
    const trigger = header.getByTestId("threads-mobile-view-trigger");
    const drawer = testPage.getByTestId("threads-mobile-view-drawer");
    await trigger.tap();
    const failedWrite = waitForHttp(testPage, "PATCH", /\/api\/v1\/user\/settings$/, {
      predicate: (response) => response.status() === 500,
    });
    await drawer.getByTestId("threads-mobile-new-view").tap();
    await failedWrite;
    const recovery = drawer.getByTestId("threads-view-sync-error");
    await expect(recovery).toBeVisible();
    await expect(header.getByTestId("threads-view-sync-error")).toHaveCount(0);
    await testPage.keyboard.press("Escape");
    await expect(drawer).toBeHidden();
    await expect(header.getByTestId("threads-mobile-view-sync-status")).toBeVisible();
    expect((await requireBox(header, "header during sync failure")).height).toBe(56);
    const cue = header.getByTestId("thread-swipe-cue");
    await expect(cue).toHaveText("1/2");
    for (const target of [trigger, header.getByTestId("mobile-topbar-menu")]) {
      expect(
        await target.evaluate((element) => {
          const box = element.getBoundingClientRect();
          return element.contains(
            document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
          );
        }),
      ).toBe(true);
    }
    const cueBox = await requireBox(cue, "pagination during sync failure");
    const menuBox = await requireBox(header.getByTestId("mobile-topbar-menu"), "menu");
    expect(cueBox.x + cueBox.width).toBeLessThanOrEqual(menuBox.x);
    await assertNoDocumentHorizontalOverflow(testPage, "saved-view sync failure");

    await trigger.tap();
    for (const id of ["threads-view-sync-retry", "threads-view-sync-dismiss"]) {
      expect(
        (await requireBox(recovery.getByTestId(id), "recovery action")).height,
      ).toBeGreaterThanOrEqual(44);
    }
    await recovery.getByTestId("threads-view-sync-dismiss").tap();
    await expect(recovery).toHaveCount(0);
    await expect(header.getByTestId("threads-mobile-view-sync-status")).toHaveCount(0);
    const failedAgain = waitForHttp(testPage, "PATCH", /\/api\/v1\/user\/settings$/, {
      predicate: (response) => response.status() === 500,
    });
    await drawer.getByTestId("threads-mobile-new-view").tap();
    await failedAgain;
    await expect(recovery).toBeVisible();
    rejectWrites = false;
    const retried = waitForHttp(testPage, "PATCH", /\/api\/v1\/user\/settings$/, {
      predicate: (response) => response.ok(),
    });
    await recovery.getByTestId("threads-view-sync-retry").tap();
    await retried;
    await expect(recovery).toHaveCount(0);
    await testPage.reload();
    await expect(trigger).toContainText("New view");
    await expect(header.getByTestId("threads-mobile-view-sync-status")).toHaveCount(0);
  });

  test("keeps all three modes compact with native context and menu targets", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);
    for (const width of [360, 393]) {
      await testPage.setViewportSize({ width, height: 851 });
      for (const [route, label] of [
        ["/?home=overview", "Kanban"],
        ["/tasks", "List"],
        ["/threads", "All threads"],
      ]) {
        if (label === "Kanban") {
          await testPage.goto("/tasks");
          await testPage.getByTestId("mobile-topbar-menu").tap();
          await testPage.getByRole("radio", { name: "Kanban", exact: true }).tap();
          await expect(testPage.getByRole("dialog")).toHaveCount(0);
        } else {
          await testPage.goto(route);
        }
        const header = testPage.locator("header").first();
        const context = header.getByTestId(
          label === "All threads" ? "threads-mobile-view-trigger" : "mobile-topbar-page-context",
        );
        const menu = header.getByTestId("mobile-topbar-menu");
        await expect(context).toContainText(label);
        await expect(header.getByTestId("mobile-topbar-brand")).toHaveCount(0);
        await expect(header.getByTestId("mobile-topbar-action-strip")).toHaveCount(0);
        const headerBox = await requireBox(header, "shared phone header");
        expect(headerBox.height).toBeCloseTo(56, 0);
        for (const target of [context, menu]) {
          const box = await requireBox(target, "phone header control");
          expect(box.height).toBeGreaterThanOrEqual(44);
          expect(box.width).toBeGreaterThanOrEqual(44);
          expect(box.x + box.width).toBeLessThanOrEqual(width);
        }
        await assertNoDocumentHorizontalOverflow(testPage, `${label} at ${width}px`);
        await context.tap();
        await expect(testPage.getByRole("dialog")).toBeVisible();
        if (label === "All threads") {
          await expect(testPage.getByTestId("threads-mobile-view-drawer")).toBeVisible();
        } else {
          await expect(testPage.getByRole("radio", { name: label, exact: true })).toHaveAttribute(
            "data-state",
            "on",
          );
        }
        await testPage.keyboard.press("Escape");
        await expect(testPage.getByRole("dialog")).toHaveCount(0);
        await expect(context).toBeFocused();
        await menu.tap();
        await expect(testPage.getByRole("dialog", { name: "Menu", exact: true })).toBeVisible();
        await testPage.keyboard.press("Escape");
        await expect(testPage.getByRole("dialog")).toHaveCount(0);
        await expect(menu).toBeFocused();
      }
    }
  });

  test("keeps workspace-aware Home in the menu and restores the remembered list", async ({
    testPage,
  }) => {
    await testPage.goto("/tasks");
    await testPage.getByTestId("mobile-topbar-menu").tap();
    const dialog = testPage.getByRole("dialog", { name: "Menu", exact: true });
    await dialog.getByRole("link", { name: "Home", exact: true }).tap();
    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/tasks" && url.searchParams.has("workspace"),
    );
    await expect(testPage.getByTestId("mobile-topbar-page-context")).toContainText("List");
    await expect(dialog).toHaveCount(0);
  });

  test("leaves the coarse-pointer tablet header composition intact", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 820, height: 1180 });
    await testPage.goto("/?home=overview");
    const header = testPage.locator("header").first();
    await expect(header.getByTestId("mobile-topbar-page-context")).toHaveCount(0);
    await expect(header.getByTestId("mobile-topbar-menu")).toHaveCount(0);
    await expect(header).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "tablet listing header");
    const menu = header.getByRole("button", { name: "Open menu", exact: true });
    await expect(menu).toBeVisible();
    // Tablet retains its existing composition and direct 44px tool launchers.
    for (const id of ["tablet-quick-chat-button", "tablet-quick-terminal-button"]) {
      const box = await requireBox(header.getByTestId(id), "tablet launcher");
      expect(box.height).toBeGreaterThanOrEqual(44);
      expect(box.width).toBeGreaterThanOrEqual(44);
    }
    await menu.tap();
    await expect(testPage.getByRole("dialog", { name: "Menu", exact: true })).toBeVisible();
    await expect(testPage.getByRole("radio", { name: "Pipeline", exact: true })).toBeVisible();
  });

  test("contains a long workspace name without squeezing the phone menu", async ({
    testPage,
    apiClient,
  }) => {
    const name = "Harbor checkout accessibility and international payment reconciliation";
    const workspace = await apiClient.createWorkspace(name);
    try {
      await testPage.setViewportSize({ width: 360, height: 851 });
      await testPage.goto(`/tasks?workspace=${workspace.id}`);
      const context = testPage.getByTestId("mobile-topbar-page-context");
      await expect(context).toContainText(name);
      const label = context.locator("span.truncate").first();
      expect(await label.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
        true,
      );
      const menu = testPage.getByTestId("mobile-topbar-menu");
      const box = await requireBox(menu, "menu beside long workspace");
      expect(box.width).toBe(44);
      expect(box.x + box.width).toBeLessThanOrEqual(360);
      await assertNoDocumentHorizontalOverflow(testPage, "long workspace context");
      await context.tap();
      await expect(testPage.getByRole("dialog", { name: "Menu", exact: true })).toBeVisible();
    } finally {
      await apiClient.deleteWorkspace(workspace.id, name);
    }
  });
});

import { test, expect } from "../../fixtures/office-fixture";
import { SessionPage } from "../../pages/session-page";
import { visitOtherBrowserTab } from "../../helpers/external-focus";

// Full Chromium supports native foreground changes in headless mode.
test.use({ channel: "chromium" });

// @covers AC-UI-EDIT-FOCUS-001.1, AC-UI-EDIT-FOCUS-001.2, AC-UI-EDIT-FOCUS-001.3
for (const surface of ["topbar", "office", "dialog"] as const) {
  test(`${surface} task rename retains the draft and caret across a browser tab switch`, async ({
    testPage,
    apiClient,
    seedData,
    officeSeed,
  }) => {
    const workspaceId = surface === "office" ? officeSeed.workspaceId : seedData.workspaceId;
    const workflowId = surface === "office" ? officeSeed.workflowId : seedData.workflowId;
    const task = await apiClient.createTask(workspaceId, `Focus ${surface}`, {
      workflow_id: workflowId,
    });
    await testPage.goto(surface === "office" ? `/office/tasks/${task.id}` : `/t/${task.id}`);
    if (surface === "office") {
      await testPage.getByTestId("office-task-title").dblclick();
    } else {
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      if (surface === "topbar") await testPage.getByTestId("task-topbar-title").dblclick();
      else {
        await session.sidebarTaskItem(`Focus ${surface}`).click({ button: "right" });
        await testPage.getByRole("menuitem", { name: "Rename", exact: true }).click();
      }
    }
    const input =
      surface === "dialog"
        ? testPage.getByRole("dialog", { name: "Rename task" }).getByRole("textbox")
        : testPage.getByTestId(
            surface === "office" ? "office-task-title-input" : "task-title-rename-input",
          );
    await input.fill("Release notes");
    await input.evaluate((el) => (el as HTMLInputElement).setSelectionRange(7, 7));
    await visitOtherBrowserTab(testPage, async () => {
      await expect(input).toHaveValue("Release notes");
      expect((await apiClient.getTask(task.id)).title).toBe(`Focus ${surface}`);
    });
    await expect(input).toBeFocused();
    await testPage.keyboard.type(" draft");
    await expect(input).toHaveValue("Release draft notes");
    if (surface === "dialog")
      await testPage.getByRole("button", { name: "Save", exact: true }).click();
    else await input.press("Enter");
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).title)
      .toBe("Release draft notes");
  });
}

import { expect, test } from "../../fixtures/test-base";

const PATH = "/settings/preferences/task-behavior";
test("concise descriptions retain hover and keyboard technical help", async ({ testPage }) => {
  await testPage.goto(`${PATH}?tab=unknown`);
  await expect(testPage.getByRole("tab", { name: "Tasks", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(testPage.getByText("create_task_kandev", { exact: true })).not.toBeVisible();
  const info = testPage.getByRole("button", { name: "About Profile for Tasks Created by Agents" });
  await info.hover();
  await expect(testPage.getByRole("tooltip")).toContainText("Workflow-selected profiles win first");
  await testPage.keyboard.press("Escape");
  await expect(testPage.getByRole("tooltip")).toBeHidden();

  // Start the keyboard check after the pointer tooltip is fully closed. Radix
  // keeps the hover and focus state in the same provider, so switching while
  // the first portal is closing can otherwise lose the focus-open event under
  // a loaded browser.
  await info.focus();
  await expect(info).toBeFocused();
  await expect(testPage.getByRole("tooltip")).toContainText("agent_profile_id");
  await testPage.keyboard.press("Escape");
  await testPage.getByRole("tab", { name: "Runtime", exact: true }).click();
  await expect(testPage.getByTestId("message-queue-max-per-session")).toBeVisible();
  await expect(testPage.getByTestId("task-behavior-settings").locator("details")).toHaveCount(0);
});

test("drafts and Reset span tabs and a failed inactive save reveals its tab once", async ({
  testPage,
}) => {
  await testPage.goto(PATH);
  const creation = testPage.locator("#creation-auto-focus");
  const initial = await creation.getAttribute("aria-checked");
  await creation.click();
  await testPage.getByRole("tab", { name: "Conversation", exact: true }).click();
  await testPage.locator("#show-anchored-prompt-bar").click();
  await expect(testPage.getByRole("tab", { name: "Tasks", exact: true })).toContainText(
    "Unsaved changes",
  );
  await expect(testPage.getByRole("tab", { name: "Conversation", exact: true })).toContainText(
    "Unsaved changes",
  );
  await testPage.getByRole("tab", { name: "Runtime", exact: true }).click();
  const queue = testPage.getByTestId("message-queue-max-per-session");
  const baseline = await queue.inputValue();
  await queue.fill(String(Number(baseline) + 1));
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Reset", exact: true })
    .click();
  await expect(queue).toHaveValue(baseline);
  await testPage.getByRole("tab", { name: "Tasks", exact: true }).click();
  await expect(creation).toHaveAttribute("aria-checked", initial!);
  await creation.click();
  await testPage.getByRole("tab", { name: "Conversation", exact: true }).click();
  await testPage.route("**/api/v1/user/settings", async (route) => {
    if (route.request().method() === "PATCH") await route.fulfill({ status: 500, body: "{}" });
    else await route.continue();
  });
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" })
    .click();
  await expect(testPage.getByRole("tab", { name: "Tasks", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await testPage.getByRole("tab", { name: "Conversation", exact: true }).click();
  await expect(testPage.getByRole("tab", { name: "Conversation", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Reset", exact: true })
    .click();
});

for (const width of [767, 768]) {
  test(`tabs and info stay contained at ${width}px`, async ({ testPage }) => {
    await testPage.setViewportSize({ width, height: 1000 });
    await testPage.goto(PATH);
    const info = testPage.getByRole("button", { name: "About Open new tasks automatically" });
    const box = await info.boundingBox();
    expect(box!.height).toBe(width < 768 ? 44 : 24);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });
}

test("Save changes persists drafts from all three tabs", async ({ testPage, apiClient }) => {
  const baseline = (await apiClient.getUserSettings()).settings;
  const queueResponse = await apiClient.rawRequest("GET", "/api/v1/system/message-queue/settings");
  const queueBaseline = (await queueResponse.json()) as { settings: { max_per_session: number } };
  try {
    await testPage.goto(PATH);
    await testPage.locator("#creation-auto-focus").click();
    await testPage.getByRole("tab", { name: "Conversation", exact: true }).click();
    await testPage.locator("#show-anchored-prompt-bar").click();
    await testPage.getByRole("tab", { name: "Runtime", exact: true }).click();
    const nextQueue = queueBaseline.settings.max_per_session + 1;
    await testPage.getByTestId("message-queue-max-per-session").fill(String(nextQueue));
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();
    await testPage.reload();
    await expect(testPage.getByTestId("message-queue-max-per-session")).toHaveValue(
      String(nextQueue),
    );
    await testPage.getByRole("tab", { name: "Conversation", exact: true }).click();
    await expect(testPage.locator("#show-anchored-prompt-bar")).toHaveAttribute(
      "aria-checked",
      String(!baseline.show_anchored_prompt_bar),
    );
    await testPage.getByRole("tab", { name: "Tasks", exact: true }).click();
    await expect(testPage.locator("#creation-auto-focus")).toHaveAttribute(
      "aria-checked",
      String(!baseline.auto_focus_new_tasks),
    );
  } finally {
    await apiClient.saveUserSettings({
      auto_focus_new_tasks: baseline.auto_focus_new_tasks,
      show_anchored_prompt_bar: baseline.show_anchored_prompt_bar,
    });
    await apiClient.rawRequest("PATCH", "/api/v1/system/message-queue/settings", {
      max_per_session: queueBaseline.settings.max_per_session,
    });
  }
});

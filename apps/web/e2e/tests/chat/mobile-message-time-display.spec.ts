import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

const CREATED_AT = "2026-06-20T10:15:00Z";
const MESSAGE = "Mobile message time display fixture";
const OPTIONS = [
  { value: "relative", label: "Relative" },
  { value: "absolute_short", label: "Absolute (short)" },
  { value: "absolute_long", label: "Absolute (long)" },
] as const;

async function expectedCounterpart(page: Page, display: (typeof OPTIONS)[number]["value"]) {
  return page.evaluate(
    ({ createdAt, display }) => {
      const date = new Date(createdAt);
      if (display === "relative") {
        return new Intl.DateTimeFormat("en-US", {
          dateStyle: "short",
          timeStyle: "short",
        }).format(date);
      }
      const formatter = new Intl.RelativeTimeFormat("en-US", { numeric: "auto" });
      const diffMs = date.getTime() - Date.now();
      if (Math.abs(diffMs) < 10_000) return formatter.format(0, "second");
      const diffSec = Math.round(diffMs / 1000);
      const magnitude = Math.abs(diffSec);
      const units = [
        ["year", 365 * 24 * 60 * 60],
        ["month", 30 * 24 * 60 * 60],
        ["week", 7 * 24 * 60 * 60],
        ["day", 24 * 60 * 60],
        ["hour", 60 * 60],
        ["minute", 60],
        ["second", 1],
      ] as const;
      for (const [unit, seconds] of units) {
        if (magnitude >= seconds) return formatter.format(Math.trunc(diffSec / seconds), unit);
      }
      return formatter.format(0, "second");
    },
    { createdAt: CREATED_AT, display },
  );
}

test.use({ locale: "en-US", timezoneId: "UTC" });

test("mobile message timestamps remain contained and open their counterpart", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 390, height: 844 });
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile message time display", {
    description: MESSAGE,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, { state: "IDLE" });
  const { messageId } = await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: MESSAGE,
    authorType: "user",
    createdAt: CREATED_AT,
    newTurn: true,
    turnStartedAt: CREATED_AT,
    turnCompletedAt: "2026-06-20T10:15:07Z",
  });

  for (const option of OPTIONS) {
    await apiClient.saveUserSettings({ message_time_display: option.value });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const row = session.activeChat().locator(`#msg-${messageId}`);
    await expect(row).toHaveCount(1);
    const duration = row.getByTestId("message-turn-duration");
    await expect(duration).toBeVisible();
    const timestamp = row.locator("time[datetime]");
    await expect(timestamp).toBeVisible();

    const geometry = await testPage.evaluate(
      ({ wrapperSelector, timestampSelector, durationSelector }) => {
        const wrapper = document.querySelector(wrapperSelector)?.getBoundingClientRect();
        const label = document.querySelector(timestampSelector)?.getBoundingClientRect();
        const duration = document.querySelector(durationSelector);
        const footer = duration?.parentElement as HTMLElement | null | undefined;
        return {
          contained: !!wrapper && !!label && label.x >= wrapper.x && label.right <= wrapper.right,
          footerFits: !!footer && footer.scrollWidth <= footer.clientWidth,
          documentFits: document.documentElement.scrollWidth <= window.innerWidth,
        };
      },
      {
        wrapperSelector: `#msg-${messageId}`,
        timestampSelector: `#msg-${messageId} time[datetime]`,
        durationSelector: `#msg-${messageId} [data-testid="message-turn-duration"]`,
      },
    );
    expect(geometry).toEqual({ contained: true, footerFits: true, documentFits: true });

    const counterpart = await expectedCounterpart(testPage, option.value);
    await expect(timestamp).toHaveAttribute("title", counterpart);
    const trigger = row.getByTestId("message-timestamp-trigger");
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox?.width).toBeGreaterThanOrEqual(44);
    expect(triggerBox?.height).toBeGreaterThanOrEqual(44);
    await trigger.tap();
    const drawer = testPage.getByTestId("message-timestamp-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByText(counterpart, { exact: true })).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(drawer).toBeHidden();
  }
});

test("mobile message-time settings reset, save, and persist through the UI", async ({
  testPage,
  apiClient,
}) => {
  await testPage.setViewportSize({ width: 390, height: 844 });
  const initial = await apiClient.getUserSettings();
  const initialDisplay =
    OPTIONS.find(({ value }) => value === initial.settings.message_time_display)?.value ??
    "relative";

  try {
    await apiClient.saveUserSettings({ message_time_display: "absolute_long" });
    await testPage.goto("/settings/preferences/task-behavior?tab=conversation");

    const select = testPage.getByRole("combobox", { name: "Message time" });
    await expect(select).toBeVisible();
    await expect(select).toContainText("Absolute (long)");
    const selectBox = await select.boundingBox();
    expect(selectBox).not.toBeNull();
    expect(selectBox!.width).toBeGreaterThanOrEqual(44);
    expect(selectBox!.height).toBeGreaterThanOrEqual(44);

    await select.tap();
    await testPage.getByRole("option", { name: "Relative", exact: true }).tap();
    const saveBar = testPage.getByTestId("settings-floating-save");
    await expect(saveBar).toBeVisible();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.message_time_display)
      .toBe("absolute_long");

    await saveBar.getByRole("button", { name: "Reset" }).tap();
    await expect(select).toContainText("Absolute (long)");
    await expect(saveBar).toBeHidden();

    await select.tap();
    await testPage.getByRole("option", { name: "Absolute (short)", exact: true }).tap();
    const patchResponse = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        new URL(response.url()).pathname === "/api/v1/user/settings",
    );
    await saveBar.getByRole("button", { name: "Save changes" }).tap();
    const response = await patchResponse;
    expect(response.ok()).toBe(true);
    expect(response.request().postDataJSON()).toEqual({ message_time_display: "absolute_short" });
    await expect(saveBar).toBeHidden();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.message_time_display)
      .toBe("absolute_short");

    await testPage.reload();
    await expect(testPage.getByRole("combobox", { name: "Message time" })).toContainText(
      "Absolute (short)",
    );
  } finally {
    await apiClient.saveUserSettings({ message_time_display: initialDisplay });
  }
});

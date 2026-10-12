import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const SETTINGS_PATH = "/settings/preferences/task-behavior";
const CREATED_AT = "2026-06-20T10:15:00Z";
const MESSAGE = "Message time display seeded transcript message";
const RECENT_MESSAGE = "Recent compact message time display fixture";
const OPTIONS = [
  { value: "relative", label: "Relative" },
  { value: "absolute_short", label: "Absolute (short)" },
  { value: "absolute_long", label: "Absolute (long)" },
] as const;

test.use({ locale: "en-US", timezoneId: "UTC" });

test("message time preference saves, persists, and appears in the transcript", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Message time display", {
    description: "message time display fixture",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, { state: "IDLE" });
  const { messageId } = await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: MESSAGE,
    authorType: "user",
    createdAt: CREATED_AT,
  });
  const recentCreatedAt = new Date(Date.now() - 60_000).toISOString();
  const { messageId: recentMessageId } = await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: RECENT_MESSAGE,
    authorType: "user",
    createdAt: recentCreatedAt,
  });
  await apiClient.saveUserSettings({ message_time_display: "absolute_long" });

  await testPage.goto(`${SETTINGS_PATH}#setting-message-time-display`);
  const setting = testPage.getByRole("combobox", { name: "Message time" });
  await expect(setting).toBeVisible();
  await testPage.getByRole("tab", { name: "Tasks", exact: true }).click();
  const search = testPage
    .getByTestId("app-sidebar-settings-mode")
    .getByTestId("settings-search")
    .getByRole("searchbox");
  await search.fill("message time");
  await testPage
    .getByTestId("settings-search-results")
    .locator('[data-settings-search-motion-key="item:task-actions-message-time-display"]')
    .click();
  await expect(setting).toBeVisible();
  await expect(setting).toBeFocused();

  for (const option of OPTIONS) {
    await testPage.goto(`${SETTINGS_PATH}#setting-message-time-display`);
    const select = testPage.getByRole("combobox", { name: "Message time" });
    await select.click();
    await testPage.getByRole("option", { name: option.label, exact: true }).click();
    const floatingSave = testPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).toBeHidden();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.message_time_display)
      .toBe(option.value);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const messageRow = session.activeChat().locator(`#msg-${messageId}`);
    await expect(messageRow.getByText(MESSAGE, { exact: true })).toBeVisible();
    const timestamp = messageRow.locator("time[datetime]");
    const expected = await testPage.evaluate(
      ({ createdAt, display }) => {
        const date = new Date(createdAt);
        const short = new Intl.DateTimeFormat("en-US", {
          dateStyle: "short",
          timeStyle: "short",
        }).format(date);
        const relative = () => {
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
        };
        let label: string;
        if (display === "absolute_short") {
          label = short;
        } else if (display === "absolute_long") {
          label = new Intl.DateTimeFormat("en-US", {
            dateStyle: "long",
            timeStyle: "medium",
          }).format(date);
        } else if (Date.now() - date.getTime() >= 7 * 86_400_000) {
          label = new Intl.DateTimeFormat("en-US", {
            year: "numeric",
            month: "numeric",
            day: "numeric",
          }).format(date);
        } else {
          label = relative();
        }
        return {
          label,
          counterpart: display === "relative" ? short : relative(),
        };
      },
      { createdAt: CREATED_AT, display: option.value },
    );
    await expect(timestamp).toHaveText(expected.label);
    await expect(timestamp).toHaveAttribute("title", expected.counterpart);
    if (option.value === "relative") {
      const recentTimestamp = session
        .activeChat()
        .locator(`#msg-${recentMessageId}`)
        .locator("time[datetime]");
      await expect(recentTimestamp).toHaveText(/^\d+[smhd] ago$/);
    }
  }
});

import type { Page } from "@playwright/test";
import { captureWorkbenchInteraction } from "../../helpers/interaction-capture";
import { expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { planScript } from "../../helpers/seed-session-messages";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

export async function assertPlanLinkEditing(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  capture: PrAssetCapture,
  mobile: boolean,
) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Plan link editing",
    seed.agentProfileId,
    {
      description: planScript(
        "## Review plan\n\n[Plan reference](https://example.com/plan) and <u>underlined text</u>",
      ),
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect.poll(() => api.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
  await session.waitForChatIdle({ timeout: 45_000 });
  await session.togglePlanMode();
  if (mobile)
    await page.getByRole("navigation").getByRole("button", { name: "Plan", exact: true }).tap();
  await expect(session.planPanel).toBeVisible();
  const editor = session.planPanel.locator(".tiptap");
  await expect(editor).toHaveCount(1);
  const link = editor.getByRole("link", { name: "Plan reference" });
  await expect(link).toHaveAttribute("href", "https://example.com/plan");
  await expect(editor.locator("u")).toHaveText("underlined text");
  const popups: Page[] = [];
  const recordPopup = (popup: Page) => popups.push(popup);
  page.on("popup", recordPopup);
  try {
    if (mobile) await link.tap();
    else await link.click();
    await dwell(page, 300, "negative-assertion", "Observe a link click opening an unwanted tab");
    expect(popups).toHaveLength(0);
    await expect(editor).toBeFocused();
    await page.keyboard.press("End");
    await page.keyboard.insertText(" Reviewed");
    await expect(editor).toContainText("Reviewed");
    await captureWorkbenchInteraction(
      page,
      capture,
      mobile ? "plan-link-phone" : "plan-link-desktop",
      "Plan link remains editable without opening another tab. Underline formatting is retained.",
    );
  } finally {
    page.off("popup", recordPopup);
    for (const popup of popups) await popup.close();
  }
}

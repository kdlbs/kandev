import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";
import {
  expectAgentMessageHighlight,
  selectAgentReplyText,
} from "../chat/agent-message-comments-helpers";

const BRIEF = "The brief proposes shipping the importer in ten days.";
const REPORT = "The report flags a risk in the billing migration.";
const BRIEF_EXTRACT = "shipping the importer";
const REPORT_EXTRACT = "risk in the billing migration";
const BRIEF_REMARK = "Why ten days and not five?";
const REPORT_REMARK = "Quantify this risk.";

function commentBody(page: Page, text: string): Locator {
  return page.locator("[data-agent-message-body]").filter({ hasText: text });
}

async function addRemark(page: Page, body: Locator, extract: string, remark: string) {
  await selectAgentReplyText(body, extract);
  await page.getByTestId("agent-message-comment-trigger").click();
  await page.getByTestId("agent-message-comment-input").fill(remark);
  await page.getByTestId("agent-message-comment-add").click();
  await expect(page.getByTestId("agent-message-comment-popover")).toBeHidden();
}

test.describe("Office thread extract comments", () => {
  test("attaches remarks to extracts of agent comments and posts them", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Thread extract comments", {
      workflow_id: officeSeed.workflowId,
    });
    for (const [body, offset] of [
      [BRIEF, 60_000],
      [REPORT, 30_000],
    ] as const) {
      await apiClient.seedComment({
        taskId: task.id,
        authorType: "agent",
        authorId: officeSeed.agentId,
        body,
        source: "session",
        createdAt: new Date(Date.now() - offset).toISOString(),
      });
    }

    await testPage.goto(`/office/tasks/${task.id}`);
    const brief = commentBody(testPage, BRIEF);
    const report = commentBody(testPage, REPORT);
    await expect(brief).toBeVisible();

    await addRemark(testPage, brief, BRIEF_EXTRACT, BRIEF_REMARK);
    await addRemark(testPage, report, REPORT_EXTRACT, REPORT_REMARK);

    const chips = testPage.getByTestId("composer-context-row");
    await expect(chips).toContainText(BRIEF_EXTRACT);
    await expect(chips).toContainText(REPORT_EXTRACT);
    await expectAgentMessageHighlight(brief, 1);
    await expectAgentMessageHighlight(report, 1);

    // Highlights and chips survive a reload.
    await testPage.reload();
    await expect(chips).toContainText(BRIEF_EXTRACT);
    await expect(chips).toContainText(REPORT_EXTRACT);
    await expectAgentMessageHighlight(commentBody(testPage, BRIEF), 1);
    await expectAgentMessageHighlight(commentBody(testPage, REPORT), 1);

    await testPage.getByPlaceholder("Add a comment...").fill("Please revise both.");
    await testPage.getByPlaceholder("Add a comment...").press("Enter");

    const postedBody = async () => {
      const { comments } = (await officeApi.listTaskComments(task.id)) as {
        comments: Array<{ body: string }>;
      };
      return comments.find((c) => c.body.includes("Please revise both."))?.body ?? "";
    };
    await expect.poll(postedBody).not.toBe("");
    const posted = await postedBody();
    for (const expected of [BRIEF_EXTRACT, BRIEF_REMARK, REPORT_EXTRACT, REPORT_REMARK]) {
      expect(posted).toContain(expected);
    }
    await expect(chips).toBeHidden();
    await expectAgentMessageHighlight(commentBody(testPage, BRIEF), 0);
  });
});

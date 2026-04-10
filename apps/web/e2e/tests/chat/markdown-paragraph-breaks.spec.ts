import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Markdown paragraph breaks", () => {
  test("consecutive paragraphs have visible spacing between them", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Create a task whose mock-agent response contains three paragraphs
    // separated by blank lines (\n\n).
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Paragraph Break Test",
      seedData.agentProfileId,
      {
        description: `e2e:message("Paragraph one.\\n\\nParagraph two.\\n\\nParagraph three.")`,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Paragraph Break Test");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    // Wait for the agent to finish processing the prompt and become idle.
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Wait for the agent response containing our paragraphs to appear.
    await expect(session.chat.getByText("Paragraph three.").last()).toBeVisible({
      timeout: 10_000,
    });

    // Verify that consecutive <p> elements inside .markdown-body have spacing.
    // The CSS rule `.markdown-body p + p { margin-top: 0.5em }` should produce
    // a positive computed margin-top on the second and third paragraphs.
    const hasSpacing = await testPage.evaluate(() => {
      const containers = document.querySelectorAll(".markdown-body");
      for (const container of containers) {
        const paragraphs = container.querySelectorAll("p");
        if (paragraphs.length < 2) continue;

        for (let i = 1; i < paragraphs.length; i++) {
          const prev = paragraphs[i - 1] as HTMLElement;
          const curr = paragraphs[i] as HTMLElement;

          // Check that the previous paragraph's sibling is the current one
          // (adjacent sibling, matching p + p selector)
          if (prev.nextElementSibling !== curr) continue;

          const margin = parseFloat(getComputedStyle(curr).marginTop);
          if (margin > 0) return true;
        }
      }
      return false;
    });

    expect(hasSpacing).toBe(true);
  });
});

import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const MARKER_AFTER_INNER_FENCE = "After nested prompt sentinel.";

const NESTED_MARKDOWN_FENCE_MESSAGE = [
  "```markdown",
  "Act as a Kandev PR coordinator/watchdog.",
  "",
  "Suggested task message:",
  "",
  "```text",
  "Please run the pr-fixup skill for PR #123.",
  "```",
  "",
  MARKER_AFTER_INNER_FENCE,
  "```",
].join("\n");

type NestedFenceRenderState = {
  foundBody: boolean;
  codeBlockCount: number;
  codeBlockHasMarker: boolean;
  paragraphHasMarker: boolean;
};

async function nestedFenceRenderState(session: SessionPage): Promise<NestedFenceRenderState> {
  return session.activeChat().evaluate((chatRoot, marker) => {
    const bodies = Array.from(chatRoot.querySelectorAll(".markdown-body"));
    const body = bodies.find((candidate) => candidate.textContent?.includes(marker));
    if (!body) {
      return {
        foundBody: false,
        codeBlockCount: 0,
        codeBlockHasMarker: false,
        paragraphHasMarker: false,
      };
    }

    const codeBlocks = Array.from(body.querySelectorAll('[class*="group/code-block"]'));
    const paragraphs = Array.from(body.querySelectorAll("p"));

    return {
      foundBody: true,
      codeBlockCount: codeBlocks.length,
      codeBlockHasMarker: codeBlocks.some((block) => block.textContent?.includes(marker)),
      paragraphHasMarker: paragraphs.some((paragraph) => paragraph.textContent?.includes(marker)),
    };
  }, MARKER_AFTER_INNER_FENCE);
}

test.describe("Markdown code fences", () => {
  test("keeps a whole-message markdown fence open when nested fences appear", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Nested Markdown Fence", {
      description: "seeded nested markdown fence",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: NESTED_MARKDOWN_FENCE_MESSAGE,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect
      .poll(() => nestedFenceRenderState(session), {
        timeout: 15_000,
        message: "Expected the text after the inner fence to remain inside the outer code block",
      })
      .toEqual({
        foundBody: true,
        codeBlockCount: 1,
        codeBlockHasMarker: true,
        paragraphHasMarker: false,
      });
  });
});

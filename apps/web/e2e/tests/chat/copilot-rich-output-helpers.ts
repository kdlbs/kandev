import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { watchWs, type WsWatcher } from "../../helpers/causal-waits";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

export const COPILOT_RICH_OUTPUT_TITLE = "kandev-show_rich_output_kandev";
export const COPILOT_RICH_OUTPUT_ARGS = {
  version: 2,
  catalog_version: 1,
  title: "Synthetic report",
  spec: {
    root: "report",
    elements: {
      report: {
        type: "Callout",
        props: {
          kind: "info",
          title: "Synthetic report",
          text: "Synthetic check completed.",
        },
        children: [],
      },
    },
  },
};

export type CopilotMessageIds = {
  beforeGroup: string;
  report: string;
  afterGroup: string;
};

export async function openCopilotRichOutputSession({
  page,
  apiClient,
  seedData,
  title,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  title: string;
}) {
  const ws = watchWs(page);
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: "Copilot rich output transcript fixture",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
  });
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  return { session, sessionId, ws };
}

export function waitForCopilotRichOutputMessage(ws: WsWatcher, sessionId: string) {
  return ws.waitForEvent("session.message.added", {
    where: (payload) =>
      payload.session_id === sessionId &&
      (payload.message_type === "tool_call" || payload.type === "tool_call") &&
      payload.content === COPILOT_RICH_OUTPUT_TITLE,
  });
}

function shellActivityMetadata(toolCallId: string, command: string) {
  return {
    status: "complete",
    tool_call_id: toolCallId,
    normalized: {
      shell_exec: { command, output: { exit_code: 0 } },
    },
  };
}

export async function seedCopilotRichOutputMessages(
  apiClient: ApiClient,
  sessionId: string,
): Promise<CopilotMessageIds> {
  const firstBefore = await apiClient.seedSessionMessage(sessionId, {
    type: "tool_execute",
    content: "copilot-before-one",
    authorType: "agent",
    newTurn: true,
    turnStartedAt: new Date().toISOString(),
    metadata: shellActivityMetadata("copilot-before-one", "copilot-before-one"),
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "tool_execute",
    content: "copilot-before-two",
    authorType: "agent",
    metadata: shellActivityMetadata("copilot-before-two", "copilot-before-two"),
  });
  const report = await apiClient.seedSessionMessage(sessionId, {
    type: "tool_call",
    content: COPILOT_RICH_OUTPUT_TITLE,
    authorType: "agent",
    metadata: {
      tool_call_id: "copilot-rich-output-call",
      title: COPILOT_RICH_OUTPUT_TITLE,
      status: "complete",
      normalized: {
        kind: "generic",
        generic: {
          name: "other",
          input: { kind: "other", raw_input: COPILOT_RICH_OUTPUT_ARGS },
          output: { content: "Rich output accepted." },
        },
      },
    },
  });
  const firstAfter = await apiClient.seedSessionMessage(sessionId, {
    type: "tool_execute",
    content: "copilot-after-one",
    authorType: "agent",
    metadata: shellActivityMetadata("copilot-after-one", "copilot-after-one"),
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "tool_execute",
    content: "copilot-after-two",
    authorType: "agent",
    metadata: shellActivityMetadata("copilot-after-two", "copilot-after-two"),
  });
  if (firstBefore.turnId) await apiClient.completeSessionTurn(firstBefore.turnId);
  return {
    beforeGroup: firstBefore.messageId,
    report: report.messageId,
    afterGroup: firstAfter.messageId,
  };
}

export function copilotRichOutputRows(chat: Locator, ids: CopilotMessageIds) {
  return {
    before: chat.locator(`[id="msg-turn-group-${ids.beforeGroup}"]`),
    report: chat.locator(`[id="msg-${ids.report}"]`),
    after: chat.locator(`[id="msg-turn-group-${ids.afterGroup}"]`),
  };
}

export async function expectCopilotRichOutput(
  session: SessionPage,
  ids: CopilotMessageIds,
): Promise<Locator> {
  const chat = session.activeChat();
  const report = chat.getByTestId("rich-output");
  const rows = copilotRichOutputRows(chat, ids);

  await expect(report).toHaveCount(1);
  await expect(report).toContainText("Synthetic report");
  await expect(report.getByText("Synthetic check completed.", { exact: true })).toBeVisible();
  await expect(report).not.toContainText("This presentation is unavailable.");
  await expect(rows.before).toHaveCount(1);
  await expect(rows.report).toHaveCount(1);
  await expect(rows.after).toHaveCount(1);

  const chronological = await chat.evaluate(
    (root, selectors) => {
      const before = root.querySelector(selectors.before);
      const reportRow = root.querySelector(selectors.report);
      const after = root.querySelector(selectors.after);
      if (!before || !reportRow || !after) return false;
      const follows = Node.DOCUMENT_POSITION_FOLLOWING;
      return (
        Boolean(before.compareDocumentPosition(reportRow) & follows) &&
        Boolean(reportRow.compareDocumentPosition(after) & follows)
      );
    },
    {
      before: `[id="msg-turn-group-${ids.beforeGroup}"]`,
      report: `[id="msg-${ids.report}"]`,
      after: `[id="msg-turn-group-${ids.afterGroup}"]`,
    },
  );
  expect(chronological).toBe(true);
  return report;
}

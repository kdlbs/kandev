import type { Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForAgentMessage } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

export const REFERENCE_TITLE = "Investigate **terminal** reconnect and preserve history";
export const REFERENCE_DESCRIPTION = "Keep terminal history when the connection returns.";
export const REFERENCE_PARENT = "Terminal reliability";
export async function openAgentReferences(page: Page, api: ApiClient, seed: SeedData) {
  const options = { workflow_id: seed.workflowId, workflow_step_id: seed.startStepId };
  const parent = await api.createTask(seed.workspaceId, REFERENCE_PARENT, options);
  const target = await api.createTask(seed.workspaceId, REFERENCE_TITLE, {
    ...options,
    description: REFERENCE_DESCRIPTION,
    parent_id: parent.id,
  });
  await api.seedTaskSession(target.id, {
    sessionId: randomUUID(),
    state: "COMPLETED",
    agentProfileId: seed.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  await api.seedTaskSession(target.id, {
    sessionId: randomUUID(),
    state: "COMPLETED",
    agentProfileId: seed.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  const sibling = await api.createTask(seed.workspaceId, REFERENCE_TITLE, options);
  const escapedTitle = REFERENCE_TITLE.replaceAll("*", "\\*");
  const content = `Follow-up work: [${escapedTitle}](/t/${target.id}) and [${escapedTitle}](/t/${sibling.id}).\n\nCode: \`[example](/t/example)\`. Plain @ordinary.`;
  const source = await api.createTaskWithAgent(
    seed.workspaceId,
    "Agent reference source",
    seed.agentProfileId,
    {
      ...options,
      repository_ids: [seed.repositoryId],
      description: `e2e:message(${JSON.stringify(content)})`,
    },
  );
  if (!source.session_id) throw new Error("Missing task session");
  await waitForAgentMessage(api, source.session_id, "Follow-up work:");
  await page.goto(`/t/${source.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  const message = session
    .activeChat()
    .getByTestId("agent-message-highlight")
    .filter({ hasText: "Follow-up work:" });
  return { target, sibling, source, session, message, content };
}

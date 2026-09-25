import type { Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const TURN_ID = "e2e-usage-turn";
const USAGE_EVENT_ID = "e2e-usage-response";

const breakdown = {
  input_tokens: 400,
  cached_read_tokens: 600,
  cached_write_tokens: 0,
  output_tokens: 200,
  thought_tokens: 0,
  total_tokens: 1200,
  output_complete: true,
  event_count: 1,
  unpriced_count: 0,
  cost_subcents: "12345",
  overflow: false,
};

const response = {
  usage_event_id: USAGE_EVENT_ID,
  provider_response_id: "e2e-provider-response",
  provider_thread_id: "e2e-provider-thread",
  provider_turn_id: "e2e-provider-turn",
  scope: "direct",
  source: "response",
  completeness: "exact",
  model: "gpt-5.6-sol",
  provider: "openai",
  input_tokens: 400,
  cached_read_tokens: 600,
  cached_write_tokens: 0,
  output_tokens: 200,
  thought_tokens: 0,
  reasoning_output_tokens: 80,
  reported_total_tokens: 1200,
  total_tokens: 1200,
  cost_subcents: "12345",
  cost_source: "models_dev_list",
  estimated: true,
  occurred_at: "2026-09-24T10:00:00Z",
};

const turn = {
  turn_id: TURN_ID,
  completeness: "exact",
  direct: breakdown,
  child: {
    input_tokens: 0,
    cached_read_tokens: 0,
    cached_write_tokens: 0,
    output_tokens: 0,
    thought_tokens: 0,
    total_tokens: 0,
    output_complete: true,
    event_count: 0,
    unpriced_count: 0,
    overflow: false,
  },
  total: breakdown,
  cost_sources: ["models_dev_list"],
  last_response: response,
};

const responseDetails = {
  ...turn,
  responses: [response],
};

function totals(sessionId: string) {
  return {
    scope: "session",
    scope_id: sessionId,
    tokens_in: 400,
    tokens_cached_read: 600,
    tokens_cached_write: 0,
    tokens_out: 200,
    tokens_thought: 0,
    tokens_total: 1200,
    cost_subcents: 12345,
    cost_subcents_decimal: "12345",
    event_count: 1,
    estimated_event_count: 1,
    unpriced_event_count: 0,
    output_tokens_complete: true,
    first_event_at: response.occurred_at,
    last_event_at: response.occurred_at,
  };
}

async function installUsageRoutes(page: Page, taskId: string, sessionId: string) {
  const root = `/api/v1/tasks/${taskId}/sessions/${sessionId}/usage`;
  await page.route(
    (url) => url.pathname.startsWith(root),
    async (route) => {
      const pathname = new URL(route.request().url()).pathname;
      let payload: unknown = null;
      switch (pathname) {
        case root:
          payload = totals(sessionId);
          break;
        case `${root}/turns`:
          payload = { session_id: sessionId, turns: [turn] };
          break;
        case `${root}/turns/${TURN_ID}`:
          payload = responseDetails;
          break;
      }
      if (!payload) {
        await route.fulfill({ status: 404, contentType: "application/json", body: "{}" });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(payload),
      });
    },
  );
}

export async function openConversationUsageTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const sessionId = task.session_id ?? task.primary_session_id;
  if (!sessionId) throw new Error("The usage scenario task has no session");
  await installUsageRoutes(page, task.id, sessionId);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return { sessionId, session };
}

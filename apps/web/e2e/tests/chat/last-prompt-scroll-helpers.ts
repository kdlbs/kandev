import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

/** Deliberately wraps beyond two lines in a desktop transcript. */
export const FIRST_PROMPT_MARKER =
  "FIRST-PROMPT-MARKER-3K7L this is the very first prompt sent in the session and should stay reachable via the scroll-to-start affordance";
export const LAST_PROMPT_MARKER =
  "LAST-PROMPT-MARKER-9F2Q please handle this scrolled-past regression carefully and thoroughly across the whole module, including the exact visual boundary where any portion of the prompt becomes clipped above the transcript viewport and the responsive behavior of every navigation control.";

const MIDDLE_FILLER_COUNT = 30;
const TRAILING_FILLER_COUNT = 50;

/**
 * Boots an idle session, sends `FIRST_PROMPT_MARKER` as the first user
 * prompt, buries it under filler, sends `LAST_PROMPT_MARKER` as a second,
 * later prompt (the "last prompt"), then buries that under more trailing
 * filler so the transcript auto-scrolls both prompts out of view above the
 * fold — exactly the "scrolled way down" scenario the scroll-to-last-prompt
 * and scroll-to-start affordances exist for. Keeping the two prompts
 * distinct lets tests assert each button jumps to its own target.
 */
export async function seedScrolledPastLastPrompt(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  opts: {
    sendViaButton?: boolean;
    trailingFillerCount?: number;
    lastPromptText?: string;
    onSessionId?: (sessionId: string) => void;
  } = {},
): Promise<SessionPage> {
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
  const sessionId = task.session_id!;
  opts.onSessionId?.(sessionId);

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  const send = async (text: string) => {
    if (opts.sendViaButton) {
      await session.sendMessageViaButton(text);
    } else {
      await session.sendMessage(text);
    }
    await session.waitForChatIdle({ timeout: 30_000 });
  };

  await send(FIRST_PROMPT_MARKER);
  await apiClient.seedAgentMessages(sessionId, MIDDLE_FILLER_COUNT, "middle filler message");
  await expect(
    session.activeChat().getByText("middle filler message " + MIDDLE_FILLER_COUNT, {
      exact: false,
    }),
  ).toBeVisible({ timeout: 15_000 });

  await send(opts.lastPromptText ?? LAST_PROMPT_MARKER);
  const trailingFillerCount = opts.trailingFillerCount ?? TRAILING_FILLER_COUNT;
  await apiClient.seedAgentMessages(sessionId, trailingFillerCount);
  await expect(
    session.activeChat().getByText(`filler message ${trailingFillerCount}`, { exact: false }),
  ).toBeVisible({ timeout: 15_000 });

  return session;
}

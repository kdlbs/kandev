import { expect, type Locator, type Page } from "@playwright/test";
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

export async function persistedLastPromptId(
  apiClient: ApiClient,
  sessionId: string,
): Promise<string> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  const prompt = messages.find(
    (message) => message.author_type === "user" && message.content === LAST_PROMPT_MARKER,
  );
  if (!prompt) throw new Error("Last prompt was not persisted");
  return prompt.id;
}

export async function expectPromptAlignedAtStart(row: Locator): Promise<void> {
  await expect(row).toBeAttached();
  await expect
    .poll(
      async () => {
        const metrics = await row.evaluate((element) => {
          const scrollport = element.closest<HTMLElement>(".chat-message-list");
          if (!scrollport) return { aligned: false, reason: "missing-scrollport" };
          const rowRect = element.getBoundingClientRect();
          const listRect = scrollport.getBoundingClientRect();
          const margin = parseFloat(getComputedStyle(element).scrollMarginTop) || 0;
          const delta = rowRect.top - listRect.top - margin;
          const aligned = Math.abs(delta) <= 2;
          // Around-window loads can make the target the first row. At scrollTop 0,
          // positive scroll-margin cannot be satisfied; accept that nearest
          // position only when the row is not clipped above.
          const atTopBoundary =
            scrollport.scrollTop <= 2 && delta < -2 && rowRect.top >= listRect.top - 2;
          return {
            aligned: aligned || atTopBoundary,
            delta,
            rowTop: rowRect.top,
            listTop: listRect.top,
            scrollTop: scrollport.scrollTop,
            scrollHeight: scrollport.scrollHeight,
            clientHeight: scrollport.clientHeight,
            margin,
          };
        });
        return metrics.aligned ? "aligned" : `misaligned: ${JSON.stringify(metrics)}`;
      },
      { timeout: 5_000 },
    )
    .toBe("aligned");
}

/**
 * Boots an idle session, sends `FIRST_PROMPT_MARKER` as the first user
 * prompt, buries it under filler, sends `LAST_PROMPT_MARKER` as a second,
 * later prompt (the "last prompt"), then seeds trailing filler and scrolls to
 * its final message. This places both prompts above the fold for the "scrolled
 * way down" scenario the scroll-to-last-prompt and scroll-to-start affordances
 * exist for. Keeping the two prompts
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
    showScrollControls?: boolean;
  } = {},
): Promise<SessionPage> {
  if (opts.showScrollControls ?? true) {
    await apiClient.saveUserSettings({
      show_scroll_to_last_prompt: true,
      show_scroll_to_start: true,
    });
  }
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
  const lastFiller = session
    .activeChat()
    .getByText(`filler message ${trailingFillerCount}`, { exact: false });
  await expect(lastFiller).toBeVisible({ timeout: 15_000 });
  await lastFiller.scrollIntoViewIfNeeded();

  return session;
}

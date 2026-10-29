import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { Locator } from "@playwright/test";
import type { SessionPage } from "../../pages/session-page";
import { seedTaskWithScenario } from "./diff-update-helpers";
import { dwell } from "../../helpers/causal-waits";

type TurnChangeHistoryResponse = {
  total: number;
  change_sets?: Array<{
    id: string;
    turn_ordinal: number;
    revision: number;
    availability: string;
    reason?: string;
    terminal_at?: string | null;
    terminal_outcome?: string | null;
    file_count?: number;
    comparison_basis?: string;
    overlap_intervals?: Array<{
      change_set_id: string;
      checkout_id: string;
      started_at: string;
      ended_at?: string;
    }>;
    repositories?: Array<{
      id: string;
      availability: string;
      comparison_basis?: string;
      export_state?: string;
      content_complete?: boolean;
    }>;
  }>;
};

export async function enableTurnChangedFiles(apiClient: ApiClient): Promise<() => Promise<void>> {
  const response = await apiClient.rawRequest("GET", "/api/v1/user/settings");
  expect(response.ok).toBe(true);
  const original = ((await response.json()) as { settings: { show_turn_changed_files?: boolean } })
    .settings.show_turn_changed_files;
  const enabled = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
    show_turn_changed_files: true,
  });
  expect(enabled.ok).toBe(true);
  await expect
    .poll(
      async () => {
        const current = await apiClient.rawRequest("GET", "/api/v1/user/settings");
        if (!current.ok) return false;
        return ((await current.json()) as { settings: { show_turn_changed_files?: boolean } })
          .settings.show_turn_changed_files;
      },
      { timeout: 10_000 },
    )
    .toBe(true);
  return async () => {
    const restore = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      show_turn_changed_files: original ?? false,
    });
    expect(restore.ok).toBe(true);
  };
}

export async function waitForTurnChangeCount(
  apiClient: ApiClient,
  sessionId: string,
  count: number,
) {
  await expect
    .poll(
      async () => {
        const response = await apiClient.rawRequest(
          "GET",
          `/api/v1/task-sessions/${sessionId}/turn-changes?offset=0&limit=50`,
        );
        if (!response.ok) return { status: response.status, total: -1, finalized: -1 };
        const history = (await response.json()) as TurnChangeHistoryResponse;
        const finalized =
          history.change_sets?.filter((changeSet) =>
            Boolean(changeSet.terminal_at || changeSet.terminal_outcome),
          ).length ?? 0;
        return {
          status: response.status,
          total: history.total,
          finalized,
          states: (history.change_sets ?? []).map(
            ({ id, availability, reason, terminal_at, terminal_outcome }) => ({
              id,
              availability,
              reason,
              terminal_at,
              terminal_outcome,
            }),
          ),
        };
      },
      {
        message: `Session history should finalize and retain ${count} turn changes`,
        timeout: 30_000,
      },
    )
    .toMatchObject({ status: 200, total: count, finalized: count });
}

export async function readTurnChangeHistory(apiClient: ApiClient, sessionId: string) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/task-sessions/${sessionId}/turn-changes?offset=0&limit=50`,
  );
  expect(response.ok).toBe(true);
  return (await response.json()) as TurnChangeHistoryResponse;
}

export async function openFirstTurnChangesDiff(session: SessionPage) {
  const cards = session.activeChat().getByTestId("turn-changed-files-card");
  const firstCard = cards.first();
  await expect(firstCard).toContainText("untracked_test.txt");
  await firstCard.getByRole("button", { name: "Open diff" }).click();
  return firstCard;
}

export async function assertHistoricalSelectorStyle(viewer: Locator, touch = false) {
  const selectors = viewer.getByRole("combobox");
  await expect(selectors).toHaveCount(2);
  for (const selector of await selectors.all()) {
    const geometry = await selector.evaluate((element) => {
      const arrow = element.parentElement!.querySelector("svg")!;
      const bounds = element.getBoundingClientRect();
      const arrowBounds = arrow?.getBoundingClientRect();
      return {
        font: getComputedStyle(element).fontSize,
        height: bounds.height,
        touchTarget: matchMedia("(max-width: 767px), (pointer: coarse)").matches,
        outerFont: getComputedStyle(
          element
            .closest("[data-testid=historical-turn-diff]")!
            .querySelector("input[type=checkbox]")!.parentElement!,
        ).fontSize,
        arrowInset: arrowBounds ? bounds.right - arrowBounds.right : 0,
        paddingRight: parseFloat(getComputedStyle(element).paddingRight),
      };
    });
    expect(geometry.height).toBeCloseTo(geometry.touchTarget ? 44 : 28, 0);
    expect(geometry.arrowInset).toBeGreaterThanOrEqual(8);
    expect(geometry.paddingRight).toBeGreaterThanOrEqual(28);
    expect(geometry.font).toBe(touch ? "16px" : geometry.outerFont);
  }
}

export async function assertLatestTurnCardInView(session: SessionPage) {
  const card = session.activeChat().getByTestId("turn-changed-files-card").last();
  await expect(card).toContainText("untracked_test.txt");
  await expect
    .poll(
      () =>
        card.evaluate((element) => {
          const scroll = element.closest(".chat-message-list")!;
          return Math.round(
            element.getBoundingClientRect().bottom - scroll.getBoundingClientRect().bottom,
          );
        }),
      { message: "latest changed-files card should be inside the transcript viewport" },
    )
    .toBeLessThanOrEqual(0);
}

export async function assertTurnCardMessageGap(card: Locator) {
  const gap = await card.evaluate((element) => {
    const list = element.closest<HTMLElement>(".chat-message-list");
    if (!list) return null;
    const nextMessage = Array.from(
      list.querySelectorAll<HTMLElement>(
        "[data-testid='user-message-bubble'], [data-agent-message-body][data-message-id]",
      ),
    ).find((message) =>
      Boolean(element.compareDocumentPosition(message) & Node.DOCUMENT_POSITION_FOLLOWING),
    );
    if (!nextMessage) return null;
    return nextMessage.getBoundingClientRect().top - element.getBoundingClientRect().bottom;
  });

  expect(gap).not.toBeNull();
  expect(gap!).toBeGreaterThanOrEqual(16);
}

export function seedTurnHistoryConflictTask(
  testPage: Parameters<typeof seedUntrackedFileTask>[0],
  apiClient: Parameters<typeof seedUntrackedFileTask>[1],
  seedData: Parameters<typeof seedUntrackedFileTask>[2],
) {
  return seedTaskWithScenario(testPage, apiClient, seedData, {
    title: "Turn History Conflict E2E",
    scenarioCommand: "/e2e:turn-history-conflict-setup",
    completionText: "turn-history-conflict-setup complete",
  });
}

export async function reloadUntilTurnCard(
  testPage: import("@playwright/test").Page,
  session: SessionPage,
  timeout = 20_000,
) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle();
    const card = session.activeChat().getByTestId("turn-changed-files-card").first();
    if (await card.isVisible().catch(() => false)) return card;
    await dwell(250, "unverified", "reload retry before the turn-change card is projected");
  }
  return session.activeChat().getByTestId("turn-changed-files-card").first();
}

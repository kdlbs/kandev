import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { Locator } from "@playwright/test";
import type { SessionPage } from "../../pages/session-page";

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
  return async () => {
    const restore = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      show_turn_changed_files: original ?? true,
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

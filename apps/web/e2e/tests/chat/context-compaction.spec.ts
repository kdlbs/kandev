import { test, expect } from "../../fixtures/test-base";
import { openContextCompactionScenario, persistedCompactions } from "./context-compaction-helpers";

type ContextWindowStore = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      contextWindow: {
        bySessionId: Record<string, { compactionCount: number } | undefined>;
      };
    };
  };
};

test.describe("ACP context compaction", () => {
  test.describe.configure({ retries: 0, timeout: 90_000 });

  test("persists and updates one timeline row per compaction identity", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { sessionId, session } = await openContextCompactionScenario(
      testPage,
      apiClient,
      seedData,
      "Context compaction desktop",
    );
    const cards = session.chat.getByTestId("context-compaction-message");
    await expect(cards).toHaveCount(2);
    const completed = cards.nth(0);
    const failed = cards.nth(1);

    await expect(completed.getByTestId("context-compaction-status")).toHaveText(
      "Context compacted",
    );
    await expect(completed.getByTestId("context-compaction-token-counts")).toContainText("31,300");
    await expect(completed.getByTestId("context-compaction-trigger")).toHaveText("manual");
    await expect(completed.getByTestId("context-compaction-duration")).toHaveText("26.8s");
    await completed.getByRole("button", { name: "Show compaction summary" }).click();
    await expect(completed.getByTestId("context-compaction-summary")).toContainText(
      "Keep the delivery cursor stable.",
    );

    await expect(failed.getByTestId("context-compaction-status")).toHaveText(
      "Context compaction failed",
    );
    await failed.getByRole("button", { name: "Show compaction error" }).click();
    await expect(failed.getByTestId("context-compaction-error")).toHaveText(
      "provider could not summarize one optional section",
    );

    await expect
      .poll(() => persistedCompactions(apiClient, sessionId), {
        message: "each provider identity should persist one normalized compaction payload",
      })
      .toEqual([
        expect.objectContaining({ compaction_id: "compaction-completed", status: "completed" }),
        expect.objectContaining({ compaction_id: "compaction-failed", status: "failed" }),
      ]);

    const compactionCount = await testPage.evaluate(
      (id) =>
        (window as ContextWindowStore).__KANDEV_E2E_STORE__?.getState().contextWindow.bySessionId[
          id
        ]?.compactionCount ?? 0,
      sessionId,
    );
    expect(compactionCount).toBe(0);

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.chat.getByTestId("context-compaction-message")).toHaveCount(2);
    await expect(session.chat.getByTestId("context-compaction-summary")).toHaveCount(0);
  });
});

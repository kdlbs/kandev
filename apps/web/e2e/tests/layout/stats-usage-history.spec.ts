import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { retainedReport, retainedRow } from "../../../app/stats/token-usage.test-helpers";
import {
  exerciseCollectorRetention,
  exerciseProjectionRecovery,
} from "../../helpers/usage-history-browser";
import { exerciseNativeRetention } from "../../helpers/usage-history-native";

test.describe("Retained usage HTTP presentation", () => {
  test.describe.configure({ retries: 0 });

  // DTO presentation cases complement the real collector/projector flows below.
  test("keeps deleted rows exact and exposes an archived live task link and independent attribution", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const archived = await apiClient.createTask(seedData.workspaceId, "Retained archived task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.archiveTask(archived.id);
    const deleted = retainedRow({
      key: "deleted-incarnation",
      total_tokens: null,
      input_tokens: null,
      amounts_decimal: { total_tokens: "9007199254740993", input_tokens: "9007199254740961" },
      provider: "",
    });
    const live = retainedRow({
      key: "live-incarnation",
      task_id: archived.id,
      task_available: true,
      task_archived: true,
    });
    const totals = retainedRow({
      total_tokens: null,
      amounts_decimal: { total_tokens: "9007199254741125" },
    });
    await testPage.route("**/api/v1/workspaces/*/stats/token-usage**", async (route) => {
      const group = new URL(route.request().url()).searchParams.get("group");
      await route.fulfill({
        json: retainedReport({
          rows: group === "day" ? [deleted] : [deleted, live],
          total_rows: group === "day" ? 1 : 2,
          totals,
        }),
      });
    });
    await testPage.goto(`/stats/token-usage?workspaceId=${seedData.workspaceId}`);
    const breakdown = testPage.getByTestId("token-usage-breakdown");
    const taskRead = waitForHttp(testPage, "GET", /\/stats\/token-usage$/, {
      predicate: (response) => new URL(response.url()).searchParams.get("group") === "task",
    });
    await breakdown.getByRole("tab", { name: "Tasks", exact: true }).click();
    await taskRead;
    const deletedRow = breakdown.getByTestId("token-usage-row").filter({ hasText: "Deleted task" });
    await expect(deletedRow).toHaveCount(1);
    await expect(deletedRow.locator("a")).toHaveCount(0);
    await expect(
      deletedRow.locator("td").filter({ hasText: /^9,007,199,254,740,993$/ }),
    ).toBeVisible();
    await expect(breakdown.getByRole("link", { name: archived.id, exact: true })).toHaveAttribute(
      "href",
      `/t/${archived.id}`,
    );
    const archivedRow = breakdown
      .getByTestId("token-usage-table")
      .getByTestId("token-usage-row")
      .filter({ hasText: archived.id });
    await expect(archivedRow).toHaveCount(1);
    await expect(archivedRow.getByText("Archived", { exact: true })).toBeVisible();
    const disclosure = deletedRow.locator("summary");
    await disclosure.focus();
    await testPage.keyboard.press("Enter");
    const details = deletedRow.getByTestId("token-usage-details");
    for (const text of [
      "Mixed users",
      "Unknown",
      "agent-client-a",
      "exact-model-id",
      "native",
      "collector-a",
      "Asia/Kathmandu",
    ]) {
      await expect(details.getByText(text, { exact: true }).first()).toBeVisible();
    }
    await testPage.keyboard.press("Enter");
    await expect(disclosure).toBeFocused();
    await expect(deletedRow.locator("details")).not.toHaveAttribute("open");
    await testPage.evaluate(() =>
      Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        value: {
          writeText: async (text: string) => {
            (window as Window & { usageCopy?: string }).usageCopy = text;
          },
        },
      }),
    );
    await testPage.getByRole("button", { name: "Copy Stats", exact: true }).click();
    await expect
      .poll(() => testPage.evaluate(() => (window as Window & { usageCopy?: string }).usageCopy))
      .toContain("Total tokens: 9,007,199,254,741,125");
    await expect(testPage.getByTestId("token-usage-summary")).toContainText(
      "9,007,199,254,741,125",
    );
  });

  test("prepares the first generation and retains saved values through a busy retry and recovery", async ({
    testPage,
    seedData,
  }) => {
    let phase: "preparing" | "pending" | "busy" | "ready" = "preparing";
    await testPage.route("**/api/v1/workspaces/*/stats/token-usage**", async (route) => {
      if (phase === "busy") {
        await route.fulfill({
          status: 503,
          json: { error: "History fixture busy", error_code: "analytics_busy" },
        });
        return;
      }
      const preparing = phase === "preparing";
      await route.fulfill({
        json: retainedReport({
          projection_status: preparing ? "preparing" : "ready",
          projection_generation: preparing ? "" : "opaque/z",
          projection_pending: phase !== "ready",
          has_history: !preparing,
          rows: preparing ? [] : [retainedRow()],
        }),
      });
    });
    await testPage.goto(`/stats/token-usage?workspaceId=${seedData.workspaceId}`);
    await expect(testPage.getByTestId("token-usage-preparing")).toContainText(
      "Preparing usage history.",
    );
    await expect(testPage.getByTestId("token-usage-summary")).toHaveCount(0);
    await expect(testPage.getByTestId("token-usage-empty-history")).toHaveCount(0);
    phase = "pending";
    await testPage.getByRole("button", { name: "Retry status", exact: true }).click();
    await expect(testPage.getByTestId("token-usage-summary")).toContainText("132");
    await expect(testPage.getByTestId("token-usage-projection-notice")).toContainText(
      "Saved values remain visible. Updating history...",
    );
    phase = "busy";
    await testPage.getByRole("button", { name: "Retry status", exact: true }).click();
    await expect(testPage.getByTestId("token-usage-error").first()).toContainText(
      "History fixture busy",
    );
    await expect(testPage.getByTestId("token-usage-summary")).toContainText("132");
    phase = "ready";
    await testPage
      .getByTestId("token-usage-error")
      .first()
      .getByRole("button", { name: "Retry", exact: true })
      .click();
    await expect(testPage.getByTestId("token-usage-error")).toHaveCount(0);
    await expect(testPage.getByTestId("token-usage-preparing")).toHaveCount(0);
    await expect(testPage.getByTestId("token-usage-projection-notice")).toHaveCount(0);
    await expect(testPage.getByTestId("token-usage-summary")).toContainText("132");
  });
});

test("real collector accounting survives archive, deletion and an exact correction", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await exerciseCollectorRetention(testPage, apiClient, seedData, false);
});

test("real first publication and rebuild preserve saved values through a database deadline", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await exerciseProjectionRecovery(testPage, apiClient, seedData, true);
});

test("native accounting survives delivery pruning, tool payload retention and session deletion", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(180_000);
  await exerciseNativeRetention(testPage, apiClient, seedData, backend, false);
});

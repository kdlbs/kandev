/**
 * E2E: the `sidebar-workspace-actions` slot (docs/plans/plugins/PLUGIN-API.md),
 * rendered in the sidebar's New Task row action cluster after the built-in
 * Quick Terminal and Quick Chat icons.
 *
 * Uses the same real `plugin-fixture` gRPC plugin package as
 * `plugin-task-panel.spec.ts` — see that file's header for how
 * apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz is built
 * (`make -C apps/backend e2e-plugin-package`). The fixture's `ui/bundle.js`
 * registers a `sidebar-workspace-actions` slot component — see
 * apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js.
 */
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import type { ApiClient } from "../../helpers/api-client";

const SLOT_TEST_ID = "e2e-sidebar-workspace-actions";
const QUICK_TERMINAL_TEST_ID = "sidebar-quick-terminal-shortcut";
const QUICK_CHAT_TEST_ID = "sidebar-quick-chat-shortcut";

async function uninstallViaApi(apiClient: ApiClient): Promise<void> {
  await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
}

test.describe("Plugins — sidebar-workspace-actions slot", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallViaApi(apiClient);
  });

  test("renders the plugin's component after Quick Terminal and Quick Chat, with the active workspace id (A1/A2)", async ({
    testPage,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await installFixturePlugin(testPage);
    await testPage.goto("/");

    const slot = testPage.getByTestId(SLOT_TEST_ID);
    await expect(slot).toBeVisible({ timeout: 15_000 });
    await expect(slot).toHaveAttribute("data-workspace-id", seedData.workspaceId);
    await expect(slot).toHaveAttribute("data-presentation", "desktop");

    // A1: after both built-in icons, not before either of them.
    const terminal = testPage.getByTestId(QUICK_TERMINAL_TEST_ID);
    const quickChat = testPage.getByTestId(QUICK_CHAT_TEST_ID);
    await expect(terminal).toBeVisible();
    await expect(quickChat).toBeVisible();
    const order = await testPage.evaluate(
      ([terminalId, chatId, slotId]) => {
        const byTestId = (id: string) => document.querySelector(`[data-testid="${id}"]`);
        const terminalEl = byTestId(terminalId);
        const chatEl = byTestId(chatId);
        const slotEl = byTestId(slotId);
        if (!terminalEl || !chatEl || !slotEl) return null;
        const position = terminalEl.compareDocumentPosition(chatEl);
        const chatAfterTerminal = Boolean(position & Node.DOCUMENT_POSITION_FOLLOWING);
        const slotPosition = chatEl.compareDocumentPosition(slotEl);
        const slotAfterChat = Boolean(slotPosition & Node.DOCUMENT_POSITION_FOLLOWING);
        return { chatAfterTerminal, slotAfterChat };
      },
      [QUICK_TERMINAL_TEST_ID, QUICK_CHAT_TEST_ID, SLOT_TEST_ID] as const,
    );
    expect(order).toEqual({ chatAfterTerminal: true, slotAfterChat: true });
  });

  test("no sidebar-workspace-actions markup renders when the plugin isn't installed (A1)", async ({
    testPage,
  }) => {
    await testPage.goto("/");
    await expect(testPage.getByTestId(QUICK_CHAT_TEST_ID)).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId(SLOT_TEST_ID)).toHaveCount(0);
  });

  test("disabling the plugin removes the slot markup without a reload (A1)", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await installFixturePlugin(testPage);
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);

    await testPage.goto("/");
    await expect(testPage.getByTestId(SLOT_TEST_ID)).toBeVisible({ timeout: 15_000 });

    await testPage.goto("/settings/plugins");
    await pluginRow.getByRole("button", { name: "Disable" }).click();
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible({ timeout: 10_000 });

    await testPage.goto("/");
    await expect(testPage.getByTestId(QUICK_CHAT_TEST_ID)).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId(SLOT_TEST_ID)).toHaveCount(0);
  });
});

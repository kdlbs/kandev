// Routing: /t/{taskId} (task-keyed). The mobile- prefix scopes this spec to
// the mobile-chrome project, where interactions use touch-sized controls.
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { switchToTerminalPanel, waitForShellReady } from "./mobile-terminal-helpers";
import { pauseNextTerminalDestroy } from "./terminal-close-pause";
import { readTerminalHostBuffer } from "./terminal-test-helpers";

async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Close UI",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle();
  return session;
}

function terminalSlot(testPage: Page, terminalId: string): Locator {
  return testPage.getByTestId(`mobile-terminal-slot-${terminalId}`);
}

test.describe("Mobile terminal close", () => {
  test.describe.configure({ retries: 1 });

  test("confirmed close disappears before teardown and keeps a sibling usable", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const destroyPause = await pauseNextTerminalDestroy(testPage);
    await seedTaskWithSession(testPage, apiClient, seedData);
    const reactRenderErrors: string[] = [];
    testPage.on("console", (message) => {
      if (message.type() === "error" && message.text().includes("Cannot update a component")) {
        reactRenderErrors.push(message.text());
      }
    });

    await switchToTerminalPanel(testPage);
    await waitForShellReady(testPage);
    await testPage.getByTestId("mobile-terminals-pill").tap();
    await testPage.getByTestId("mobile-add-terminal").tap();

    const rows = testPage.locator('[data-testid^="mobile-terminal-row-"]');
    await expect(rows).toHaveCount(2, { timeout: 10_000 });
    const siblingTestId = await rows.first().getAttribute("data-testid");
    const targetTestId = await rows.last().getAttribute("data-testid");
    expect(siblingTestId).toMatch(/^mobile-terminal-row-shell-/);
    expect(targetTestId).toMatch(/^mobile-terminal-row-shell-/);
    const siblingId = siblingTestId!.replace("mobile-terminal-row-", "");
    const targetId = targetTestId!.replace("mobile-terminal-row-", "");
    const siblingRow = testPage.getByTestId(siblingTestId!);
    const targetRow = testPage.getByTestId(targetTestId!);

    const closeTarget = targetRow.getByRole("button", { name: /^Close / });
    const closeBox = await closeTarget.boundingBox();
    expect(closeBox).not.toBeNull();
    expect(closeBox!.width).toBeGreaterThanOrEqual(44);
    expect(closeBox!.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await targetRow.tap();
    const targetHost = terminalSlot(testPage, targetId).getByTestId("terminal-xterm-host");
    await expect(terminalSlot(testPage, targetId)).toHaveAttribute("data-state", "active");
    await expect
      .poll(() => readTerminalHostBuffer(targetHost), {
        timeout: 45_000,
        message: "second mobile terminal shell should connect",
      })
      .not.toBe("");
    await testPage.getByTestId("mobile-terminals-pill").tap();
    destroyPause.arm();
    await targetRow.getByRole("button", { name: /^Close / }).tap();
    const confirmation = targetRow.getByTestId("mobile-terminal-close-confirmation");
    await expect(confirmation).toBeVisible({ timeout: 5_000 });
    await expect(confirmation).toHaveRole("group");
    await expect(confirmation).toHaveAccessibleName("Close terminal?");
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await expect(targetRow).toBeVisible();
    const cancelConfirmation = confirmation.getByRole("button", { name: "Cancel", exact: true });
    const confirmClose = confirmation.getByRole("button", {
      name: "Close terminal",
      exact: true,
    });
    for (const action of [cancelConfirmation, confirmClose]) {
      const actionBox = await action.boundingBox();
      expect(actionBox).not.toBeNull();
      expect(actionBox!.width).toBeGreaterThanOrEqual(44);
      expect(actionBox!.height).toBeGreaterThanOrEqual(44);
    }
    await confirmClose.tap();
    await destroyPause.waitForRequest();

    await expect(confirmation).toBeHidden();
    await expect(targetRow).toHaveCount(0);
    await expect(siblingRow).toBeVisible();
    await siblingRow.tap();
    const siblingSlot = terminalSlot(testPage, siblingId);
    const siblingHost = siblingSlot.getByTestId("terminal-xterm-host");
    await expect(siblingSlot).toHaveAttribute("data-state", "active");
    await expect(siblingHost).toBeVisible({ timeout: 10_000 });
    await siblingHost.tap();
    await testPage.keyboard.type("printf '%s\\n' $((86420 + 7))");
    await testPage.keyboard.press("Enter");
    await expect
      .poll(() => readTerminalHostBuffer(siblingHost), {
        timeout: 10_000,
        message: "sibling mobile terminal should remain interactive after confirmed close",
      })
      .toContain("86427");
    destroyPause.release();

    expect(reactRenderErrors).toEqual([]);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});

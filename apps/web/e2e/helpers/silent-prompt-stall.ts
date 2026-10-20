import { test, expect } from "../fixtures/test-base";
import { seedRunningGeneratingSession } from "./generating-session";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import { waitForActiveSessionCancellationPending } from "./session-store";

/** Exercise the production watchdog against an accepted, silent ACP prompt. */
export function registerSilentPromptStallTests() {
  test.describe("accepted silent prompt", () => {
    let releaseEnvironment: (() => Promise<void>) | undefined;
    let recoveryProfileId: string | undefined;
    test.afterEach(async ({ apiClient }) => {
      await releaseEnvironment?.();
      releaseEnvironment = undefined;
      if (recoveryProfileId) await apiClient.deleteAgentProfile(recoveryProfileId, true);
      recoveryProfileId = undefined;
    });

    test("output clears the real watchdog notice without ending the turn", async ({
      backend,
      testPage,
      apiClient,
      seedData,
      isMobile,
    }) => {
      test.setTimeout(120_000);
      releaseEnvironment = await backend.useEnv({ KANDEV_E2E_PROMPT_STALL_TIMEOUT: "1s" });
      if (!isMobile) await testPage.setViewportSize({ width: 600, height: 850 });
      const { session, sessionId } = await seedRunningGeneratingSession(
        testPage,
        apiClient,
        seedData,
        "Silent turn resumes output",
        {
          predecessorPrompt:
            'e2e:delay(10000)\ne2e:message("silent-turn-progress")\ne2e:delay(30000)',
          startWithGeneratingTurn: true,
        },
      );
      const notice = session.activeChat().getByTestId("running-action-notice");
      await expect(notice).toHaveCount(1);
      const cancel = notice.getByTestId("stall-cancel-turn-button");
      const box = await cancel.boundingBox();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      await expect(
        session.activeChat().getByText("silent-turn-progress", { exact: true }),
      ).toBeVisible({ timeout: 20_000 });
      await expect(notice).toHaveCount(0);
      expect((await apiClient.getTaskSession(sessionId)).session.state).toBe("RUNNING");
      await testPage.reload();
      await session.waitForLoad();
      await expect(notice).toHaveCount(0);
      await assertNoDocumentHorizontalOverflow(testPage);
      await apiClient.stopSession({ session_id: sessionId, force: true });
    });

    for (const uncertain of [false, true]) {
      // @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.12
      // @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.14
      test(`watchdog preserves the turn and cancellation ${uncertain ? "stays blocked when uncertain" : "settles after acknowledgement"}`, async ({
        backend,
        testPage,
        apiClient,
        seedData,
        isMobile,
        prCapture,
      }) => {
        test.setTimeout(120_000);
        releaseEnvironment = await backend.useEnv({
          KANDEV_E2E_PROMPT_STALL_TIMEOUT: "1s",
          KANDEV_E2E_PROMPT_CANCEL_JOIN_TIMEOUT: "12s",
        });
        if (uncertain) {
          const { agents } = await apiClient.listAgents();
          const mock = agents.find((agent) => agent.name === "mock-agent");
          if (!mock) throw new Error("The mock agent is unavailable");
          const profile = await apiClient.createAgentProfile(
            mock.id,
            "Silent cancel recovery failure",
            {
              model: "mock-fast",
              cli_flags: [
                {
                  description: "Keep recovery blocked after runtime loss",
                  flag: "--fail-on-resume",
                  enabled: true,
                },
              ],
            },
          );
          recoveryProfileId = profile.id;
        }
        const { session, sessionId } = await seedRunningGeneratingSession(
          testPage,
          apiClient,
          recoveryProfileId ? { ...seedData, agentProfileId: recoveryProfileId } : seedData,
          "Accepted silent prompt",
          { predecessorPrompt: "/e2e:cancel-hold", startWithGeneratingTurn: true },
        );
        const before = (await apiClient.getTaskSession(sessionId)).session;
        const notice = session.activeChat().getByTestId("running-action-notice");
        await expect(notice).toHaveCount(1);
        await expect(notice).toContainText("Still waiting for the agent.");
        expect(before.state).toBe("RUNNING");
        await expect(testPage.getByTestId("recovery-resume-button")).toHaveCount(0);
        await testPage.reload();
        await session.waitForLoad();
        await expect(notice).toHaveCount(1);
        const cancel = notice.getByTestId("stall-cancel-turn-button");
        const box = await cancel.boundingBox();
        expect(box).not.toBeNull();
        if (isMobile || testPage.viewportSize()!.width < 768) {
          expect(box!.height).toBeGreaterThanOrEqual(44);
          expect(box!.width).toBeGreaterThanOrEqual(44);
        } else {
          expect(box!.height).toBeCloseTo(24, 0);
        }
        await assertNoDocumentHorizontalOverflow(testPage);
        if (prCapture.capturing) {
          await testPage.addStyleTag({
            content: '[data-testid="terminal-panel"] .xterm { visibility: hidden; }',
          });
        }
        await prCapture.screenshot(`${isMobile ? "phone" : "desktop"}-accepted-silent-turn`, {
          caption: "The real watchdog keeps the silent turn active and offers Cancel turn.",
        });
        if (isMobile) await cancel.tap();
        else await cancel.click();
        if (!uncertain) {
          await waitForActiveSessionCancellationPending(testPage, true);
          await expect(cancel).toBeDisabled();
          await expect(cancel.getByRole("status", { name: "Cancelling..." })).toBeVisible();
          await testPage.reload();
          await session.waitForLoad();
          await expect(cancel).toBeDisabled();
          if (prCapture.capturing) {
            await testPage.addStyleTag({
              content: '[data-testid="terminal-panel"] .xterm { visibility: hidden; }',
            });
          }
          await prCapture.screenshot(`${isMobile ? "phone" : "desktop"}-cancelling-silent-turn`, {
            caption: "Cancel turn stays disabled while cancellation is pending after reload.",
          });
          await session.waitForChatIdle({ timeout: 30_000 });
          const after = (await apiClient.getTaskSession(sessionId)).session;
          expect(after.metadata?.acp).toEqual(before.metadata?.acp);
        } else {
          await waitForActiveSessionCancellationPending(testPage, true);
          expect((await apiClient.killLocalAgentRuntimeChild()).killed).toBe(true);
          await expect(testPage.getByTestId("recovery-retry-connection-button")).toBeVisible({
            timeout: 30_000,
          });
          await testPage.reload();
          await session.waitForLoad();
          await expect(testPage.getByTestId("recovery-retry-connection-button")).toBeVisible();
          const after = (await apiClient.getTaskSession(sessionId)).session;
          expect(after.metadata?.agent_delivery_recovery).toMatchObject({ phase: "uncertain" });
        }
        await expect(notice).toHaveCount(0);
        await assertNoDocumentHorizontalOverflow(testPage);
      });
    }
  });
}

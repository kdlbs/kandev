import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Regression for: changing an agent profile's default ACP `mode` was not
 * applied to newly created tasks.
 *
 * Background: the bulk agent-edit save helper (agent-save-helpers.ts) silently
 * dropped the `mode` field, so PATCH /agent-profiles/{id} never carried it.
 * The backend then correctly read the (stale) mode from the DB on session
 * start. Frontend fix puts mode back in the dirty check + payload; this e2e
 * proves mode now propagates from profile.mode -> session.currentModeId for a
 * fresh task.
 *
 * Mock-agent advertises modes ["default", "plan-mock"] in NewSession so the
 * capability cache accepts "plan-mock" without the reconciler healing it
 * back to the current default.
 */
test.describe("Agent profile mode propagates to new task", () => {
  test("setting profile mode to a non-default applies on a new task session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const targetMode = "plan-mock";

    // Read the original profile so we can restore it after the test.
    const before = await apiClient.listAgents();
    const originalProfile = before.agents
      .flatMap((a) => a.profiles)
      .find((p) => p.id === seedData.agentProfileId);
    if (!originalProfile) throw new Error("seed agent profile not found");
    const originalMode = originalProfile.mode ?? "";

    try {
      // 1. Set the profile's default mode to a non-default advertised mode.
      await apiClient.updateAgentProfile(seedData.agentProfileId, { mode: targetMode });

      // 2. Confirm the mode actually persisted (catches reconciler healing it
      //    away if the mock-agent ever stops advertising "plan-mock").
      const after = await apiClient.listAgents();
      const persisted = after.agents
        .flatMap((a) => a.profiles)
        .find((p) => p.id === seedData.agentProfileId);
      expect(persisted?.mode).toBe(targetMode);

      // 3. Create a new task with this profile.
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Profile mode propagation test",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      const sessionId = task.session_id;
      if (!sessionId) throw new Error("created task has no session_id");

      // 4. Open the task page so the WS subscriptions populate the store.
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      // 5. Wait for the session_mode WS event to land in the Zustand store
      //    with the profile's mode as currentModeId. Poll because the mode
      //    is set asynchronously after session/new (SetMode -> WS round-trip).
      await expect
        .poll(
          () =>
            testPage.evaluate((sid) => {
              type SessionModeShape = {
                currentModeId: string;
                availableModes: { id: string }[];
              };
              type StoreShape = {
                getState: () => {
                  sessionMode: { bySessionId: Record<string, SessionModeShape | undefined> };
                };
              };
              const win = window as unknown as { __KANDEV_STORE?: StoreShape };
              const entry = win.__KANDEV_STORE?.getState().sessionMode.bySessionId[sid];
              return entry?.currentModeId ?? null;
            }, sessionId),
          { timeout: 30_000, intervals: [250, 500, 1_000] },
        )
        .toBe(targetMode);
    } finally {
      // Restore the seed profile so the worker-scoped fixture stays valid.
      await apiClient.updateAgentProfile(seedData.agentProfileId, { mode: originalMode });
    }
  });
});

import { test as base, expect } from "../../fixtures/office-fixture";

export const test = base.extend<{ quorumWorkflowId: string }>({
  quorumWorkflowId: async ({ apiClient, officeSeed }, use) => {
    const { steps } = await apiClient.listWorkflowSteps(officeSeed.workflowId);
    const work = steps.find((step) => step.stage_type === "work");
    expect(work).toBeDefined();
    const events = work!.events ?? {};
    expect(events.on_enter?.some((action) => action.type === "auto_start_agent")).toBe(true);

    // Quorum decisions use a seeded idle session. Returning to Work must not
    // launch that session while the test verifies the next manual transition.
    await apiClient.updateWorkflowStep(work!.id, {
      events: {
        ...events,
        on_enter: events.on_enter?.filter((action) => action.type !== "auto_start_agent"),
      },
    });
    try {
      await use(officeSeed.workflowId);
    } finally {
      await apiClient.updateWorkflowStep(work!.id, { events });
    }
  },
});

export { expect };

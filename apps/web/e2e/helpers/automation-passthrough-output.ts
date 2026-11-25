import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";

export const PASSTHROUGH_GUIDANCE_TITLE =
  "CLI passthrough output is not captured in this run's transcript.";
export const PASSTHROUGH_GUIDANCE_DESCRIPTION =
  "Disable CLI passthrough in the automation's agent profile to record output for future sessions. Previous terminal output cannot be recovered here.";

type RunFixture = {
  id: string;
  sessionId: string;
  status: "task_created" | "succeeded";
  prompt: string;
  error: string;
};

/** Persist run origin and session mode before loading the hidden run reader. */
export async function seedAutomationPassthroughOutput(
  apiClient: ApiClient,
  seedData: SeedData,
  name: string,
) {
  const automation = await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name,
    workflowId: seedData.workflowId,
    workflowStepId: seedData.startStepId,
    prompt: "Read the run transcript and reply with any useful follow-up.",
  });

  const fixtures: Array<{
    key: "runningPassthrough" | "parkedPassthrough" | "structured";
    label: string;
    runStatus: RunFixture["status"];
    sessionState: "RUNNING" | "WAITING_FOR_INPUT";
    isPassthrough: boolean;
  }> = [
    {
      key: "runningPassthrough",
      label: "running passthrough",
      runStatus: "task_created",
      sessionState: "RUNNING",
      isPassthrough: true,
    },
    {
      key: "parkedPassthrough",
      label: "parked passthrough",
      runStatus: "succeeded",
      sessionState: "WAITING_FOR_INPUT",
      isPassthrough: true,
    },
    {
      key: "structured",
      label: "structured",
      runStatus: "succeeded",
      sessionState: "WAITING_FOR_INPUT",
      isPassthrough: false,
    },
  ];

  const runs = {} as Record<(typeof fixtures)[number]["key"], RunFixture>;
  for (const fixture of fixtures) {
    const task = await apiClient.createTask(seedData.workspaceId, `${name} ${fixture.label}`, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.setTaskOrigin(task.id, "automation_run");
    const session = await apiClient.seedAutomationTaskSession(task.id, fixture.sessionState, true, {
      isPassthrough: fixture.isPassthrough,
    });
    const prompt = `Stored user prompt for ${fixture.label} run.`;
    const error = `Stored provider error for ${fixture.label} run.`;
    const seededPrompt = await apiClient.seedSessionMessage(session.id, {
      type: "message",
      authorType: "user",
      content: prompt,
    });
    await apiClient.seedSessionMessage(session.id, {
      type: "error",
      authorType: "agent",
      content: error,
    });
    const run = await apiClient.seedAutomationRun(automation.id, fixture.runStatus, task.id, {
      sessionId: session.id,
      turnId: seededPrompt.turnId ?? undefined,
    });

    runs[fixture.key] = {
      id: run.id,
      sessionId: session.id,
      status: fixture.runStatus,
      prompt,
      error,
    };
  }

  return { automation, runs };
}

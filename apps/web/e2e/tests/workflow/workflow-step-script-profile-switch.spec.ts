import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForAgentMessage } from "../../helpers/session";
import { pollWorkflowTaskSessions } from "./workflow-agent-switch-helpers";

type WorkflowScriptMessage = Awaited<
  ReturnType<ApiClient["listSessionMessages"]>
>["messages"][number];

type ScriptAction = {
  type: "run_script";
  config: { command: string; timeout_seconds: number; failure_policy: "block" };
};

function runScript(command: string): ScriptAction {
  return {
    type: "run_script",
    config: { command, timeout_seconds: 5, failure_policy: "block" },
  };
}

function workflowScripts(messages: WorkflowScriptMessage[]): WorkflowScriptMessage[] {
  return messages.filter(
    (message) =>
      message.type === "script_execution" && message.metadata?.script_type === "workflow_step",
  );
}

async function waitForScripts(
  apiClient: ApiClient,
  sessionId: string,
  count: number,
  timeout = 60_000,
): Promise<WorkflowScriptMessage[]> {
  let latest: WorkflowScriptMessage[] = [];
  await expect
    .poll(
      async () => {
        latest = workflowScripts((await apiClient.listSessionMessages(sessionId)).messages);
        return latest.length;
      },
      { timeout, message: `session ${sessionId} did not receive ${count} workflow scripts` },
    )
    .toBeGreaterThanOrEqual(count);
  return latest;
}

async function createProfileSwitchWorkflow(apiClient: ApiClient, seedData: SeedData) {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((candidate) => candidate.id !== "dynamic");
  if (!agent) throw new Error("no concrete E2E agent is available");
  const profileA = await apiClient.createAgentProfile(agent.id, "Script source profile", {
    model: "mock-fast",
  });
  const profileB = await apiClient.createAgentProfile(agent.id, "Script destination profile", {
    model: "mock-slow",
  });

  const workflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    "Workflow script profile switch",
  );
  const source = await apiClient.createWorkflowStep(workflow.id, "Source profile", 0, {
    is_start_step: true,
    agent_profile_id: profileA.id,
    profile_session_end_policy: "park",
  });
  const destination = await apiClient.createWorkflowStep(workflow.id, "Destination profile", 1, {
    agent_profile_id: profileB.id,
    profile_session_start_policy: "new",
  });
  await apiClient.updateWorkflowStep(source.id, {
    events: {
      on_turn_complete: [
        runScript("printf 'source-completion-marker\\n'"),
        { type: "move_to_next" },
      ],
      on_exit: [runScript("printf 'source-exit-marker\\n'")],
    },
  });
  await apiClient.updateWorkflowStep(destination.id, {
    events: { on_enter: [runScript("printf 'destination-entry-marker\\n'")] },
  });

  return { workflow, source, destination, profileA, profileB };
}

test("binds completion and exit to the source session and entry to the destination session", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const setup = await createProfileSwitchWorkflow(apiClient, seedData);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Workflow script profile switch task",
    setup.profileA.id,
    {
      workflow_id: setup.workflow.id,
      workflow_step_id: setup.source.id,
      description: 'e2e:delay(500)\ne2e:message("source agent turn")',
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  await waitForAgentMessage(apiClient, task.session_id ?? "", "source agent turn", 60_000);
  const sessions = await pollWorkflowTaskSessions(apiClient, task.id, 2, 60_000);
  const sourceSession = sessions.find((session) => session.agent_profile_id === setup.profileA.id);
  const destinationSession = sessions.find(
    (session) => session.agent_profile_id === setup.profileB.id,
  );
  expect(sourceSession, "expected the source profile session").toBeTruthy();
  expect(destinationSession, "expected the destination profile session").toBeTruthy();

  const sourceMessages = await waitForScripts(apiClient, sourceSession!.id, 2);
  const destinationMessages = await waitForScripts(apiClient, destinationSession!.id, 1);

  expect(sourceMessages.map((message) => message.metadata?.trigger).sort()).toEqual([
    "on_exit",
    "on_turn_complete",
  ]);
  expect(sourceMessages.map((message) => message.content).join("\n")).toContain(
    "source-completion-marker",
  );
  expect(sourceMessages.map((message) => message.content).join("\n")).toContain(
    "source-exit-marker",
  );
  expect(destinationMessages[0].metadata?.trigger).toBe("on_enter");
  expect(destinationMessages[0].content).toContain("destination-entry-marker");

  const allSourceMessages = workflowScripts(
    (await apiClient.listSessionMessages(sourceSession!.id)).messages,
  );
  const allDestinationMessages = workflowScripts(
    (await apiClient.listSessionMessages(destinationSession!.id)).messages,
  );
  expect(allSourceMessages.every((message) => message.metadata?.trigger !== "on_enter")).toBe(true);
  expect(allDestinationMessages.every((message) => message.metadata?.trigger === "on_enter")).toBe(
    true,
  );
  expect((await apiClient.getTask(task.id)).workflow_step_id).toBe(setup.destination.id);
});

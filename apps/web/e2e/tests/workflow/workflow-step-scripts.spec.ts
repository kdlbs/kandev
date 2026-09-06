import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  pollWorkflowTaskSessions,
  waitForWorkflowProfileSession,
} from "./workflow-agent-switch-helpers";
import { SessionPage } from "../../pages/session-page";

type WorkflowScriptAction = {
  type: "run_script";
  config: {
    command: string;
    timeout_seconds?: number;
    failure_policy?: "block" | "continue";
  };
};

type ScriptMessage = Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"][number];

const terminalScriptStatuses = new Set(["succeeded", "failed", "timed_out", "interrupted"]);

type ScriptScenarioRuntime = {
  workspaceId: string;
  repositoryId: string;
  agentProfileId: string;
};

function isWorkflowScriptMessage(message: ScriptMessage): boolean {
  return message.type === "script_execution" && message.metadata?.script_type === "workflow_step";
}

function scriptMetadata(message: ScriptMessage, key: string): unknown {
  return message.metadata?.[key];
}

async function waitForWorkflowScripts(
  apiClient: ApiClient,
  sessionId: string,
  count: number,
  timeout = 45_000,
): Promise<ScriptMessage[]> {
  let latest: ScriptMessage[] = [];
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        latest = messages.filter(isWorkflowScriptMessage);
        return latest.filter((message) =>
          terminalScriptStatuses.has(String(scriptMetadata(message, "status"))),
        ).length;
      },
      {
        timeout,
        message: `session ${sessionId} did not receive ${count} workflow script message(s)`,
      },
    )
    .toBeGreaterThanOrEqual(count);
  return latest.sort(
    (left, right) =>
      Number(scriptMetadata(left, "action_position") ?? 0) -
      Number(scriptMetadata(right, "action_position") ?? 0),
  );
}

async function waitForWorkflowScriptStatus(
  apiClient: ApiClient,
  sessionId: string,
  status: string,
  timeout = 45_000,
): Promise<ScriptMessage> {
  let latest: ScriptMessage | undefined;
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        latest = messages.find(
          (message) =>
            isWorkflowScriptMessage(message) && scriptMetadata(message, "status") === status,
        );
        return Boolean(latest);
      },
      { timeout, message: `session ${sessionId} did not reach workflow script status ${status}` },
    )
    .toBe(true);
  if (!latest) throw new Error(`workflow script status ${status} was not observed`);
  return latest;
}

async function waitForTaskStep(apiClient: ApiClient, taskId: string, stepId: string) {
  await expect
    .poll(async () => (await apiClient.getTask(taskId)).workflow_step_id, {
      timeout: 30_000,
      message: `task ${taskId} did not reach workflow step ${stepId}`,
    })
    .toBe(stepId);
}

async function createScriptScenario(
  apiClient: ApiClient,
  runtime: ScriptScenarioRuntime,
  name: string,
  actions: WorkflowScriptAction[],
) {
  const workflow = await apiClient.createWorkflow(runtime.workspaceId, name);
  const source = await apiClient.createWorkflowStep(workflow.id, "Source", 0, {
    is_start_step: true,
  });
  const target = await apiClient.createWorkflowStep(workflow.id, "Script target", 1);
  await apiClient.updateWorkflowStep(target.id, { events: { on_enter: actions } });

  const task = await apiClient.createTaskWithAgent(
    runtime.workspaceId,
    `${name} task`,
    runtime.agentProfileId,
    {
      workflow_id: workflow.id,
      workflow_step_id: source.id,
      repository_ids: [runtime.repositoryId],
    },
  );
  await waitForWorkflowProfileSession(apiClient, task.id, runtime.agentProfileId);
  await apiClient.moveTask(task.id, workflow.id, target.id);
  const sessions = await pollWorkflowTaskSessions(apiClient, task.id, 1, 60_000);
  const session = sessions.find(
    (candidate) => candidate.agent_profile_id === runtime.agentProfileId,
  );
  if (!session) throw new Error(`script scenario ${name} did not create its agent session`);

  return { workflow, source, target, task, session };
}

function runScript(
  command: string,
  options: Pick<WorkflowScriptAction["config"], "timeout_seconds" | "failure_policy"> = {},
): WorkflowScriptAction {
  return { type: "run_script", config: { command, ...options } };
}

test.describe("Workflow step scripts", () => {
  test("streams a successful entry script and keeps it after chat reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const marker = "workflow-script-success-marker";
    const scenario = await createScriptScenario(apiClient, seedData, "Workflow script success", [
      runScript(`printf '${marker}\\n'`),
    ]);

    const [message] = await waitForWorkflowScripts(apiClient, scenario.session.id, 1);
    expect(scriptMetadata(message, "status")).toBe("succeeded");
    expect(message.content).toContain(marker);
    expect(scriptMetadata(message, "command")).toContain(marker);

    const sessionPage = new SessionPage(testPage);
    await testPage.goto(`/t/${scenario.task.id}`);
    await sessionPage.waitForLoad();
    await expect(sessionPage.activeChat().getByText(marker, { exact: false })).toBeVisible();
    await testPage.reload();
    await sessionPage.waitForLoad();
    await expect(sessionPage.activeChat().getByText(marker, { exact: false })).toBeVisible();
  });

  test("records a non-zero exit and blocks the step operation", async ({ apiClient, seedData }) => {
    const scenario = await createScriptScenario(apiClient, seedData, "Workflow script block", [
      runScript("printf 'blocked-marker\\n'; exit 7", { failure_policy: "block" }),
    ]);

    const message = await waitForWorkflowScriptStatus(apiClient, scenario.session.id, "failed");
    expect(message.content).toContain("blocked-marker");
    expect(scriptMetadata(message, "exit_code")).toBe(7);
    await waitForTaskStep(apiClient, scenario.task.id, scenario.target.id);
    expect((await apiClient.listTaskSessions(scenario.task.id)).sessions).toHaveLength(1);
  });

  test("continues after a failed action and runs the next action in order", async ({
    apiClient,
    seedData,
  }) => {
    const scenario = await createScriptScenario(apiClient, seedData, "Workflow script continue", [
      runScript("printf 'continue-failure\\n'; exit 9", { failure_policy: "continue" }),
      runScript("printf 'continue-success\\n'"),
    ]);

    const messages = await waitForWorkflowScripts(apiClient, scenario.session.id, 2);
    expect(messages.map((message) => scriptMetadata(message, "status"))).toEqual([
      "failed",
      "succeeded",
    ]);
    expect(messages[0].content).toContain("continue-failure");
    expect(messages[1].content).toContain("continue-success");
    await waitForTaskStep(apiClient, scenario.task.id, scenario.target.id);
  });

  test("records a timeout as a terminal blocked script result", async ({ apiClient, seedData }) => {
    test.setTimeout(90_000);
    const scenario = await createScriptScenario(apiClient, seedData, "Workflow script timeout", [
      runScript("sleep 3", { timeout_seconds: 1, failure_policy: "block" }),
    ]);

    const message = await waitForWorkflowScriptStatus(
      apiClient,
      scenario.session.id,
      "timed_out",
      60_000,
    );
    expect(scriptMetadata(message, "error")).toContain("timed out");
    await waitForTaskStep(apiClient, scenario.task.id, scenario.target.id);
  });

  test("gives each repeated entry a new occurrence while reload remains idempotent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const marker = "workflow-script-repeated-entry";
    const scenario = await createScriptScenario(
      apiClient,
      seedData,
      "Workflow script repeated entry",
      [runScript(`printf '${marker}\\n'`)],
    );

    await waitForWorkflowScripts(apiClient, scenario.session.id, 1);
    await apiClient.moveTask(scenario.task.id, scenario.workflow.id, scenario.source.id);
    await waitForTaskStep(apiClient, scenario.task.id, scenario.source.id);
    await apiClient.moveTask(scenario.task.id, scenario.workflow.id, scenario.target.id);
    const messages = await waitForWorkflowScripts(apiClient, scenario.session.id, 2);
    expect(messages).toHaveLength(2);
    expect(messages.every((message) => message.content.includes(marker))).toBe(true);

    const sessionPage = new SessionPage(testPage);
    await testPage.goto(`/t/${scenario.task.id}`);
    await sessionPage.waitForLoad();
    await testPage.reload();
    await sessionPage.waitForLoad();
    const afterReload = await apiClient.listSessionMessages(scenario.session.id);
    expect(afterReload.messages.filter(isWorkflowScriptMessage)).toHaveLength(2);
  });

  test("marks an admitted script interrupted after backend recovery without rerunning it", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const scenario = await createScriptScenario(apiClient, seedData, "Workflow script recovery", [
      runScript("sleep 10", { timeout_seconds: 30, failure_policy: "block" }),
    ]);

    await waitForWorkflowScriptStatus(apiClient, scenario.session.id, "running");
    await backend.restart();
    await backend.ensureReady();
    const interrupted = await waitForWorkflowScriptStatus(
      apiClient,
      scenario.session.id,
      "interrupted",
      60_000,
    );
    expect(["workflow service stopped", "workflow script interrupted"]).toContain(
      scriptMetadata(interrupted, "error"),
    );

    const sessionPage = new SessionPage(testPage);
    await testPage.goto(`/t/${scenario.task.id}`);
    await sessionPage.waitForLoad();
    const { messages } = await apiClient.listSessionMessages(scenario.session.id);
    expect(messages.filter(isWorkflowScriptMessage)).toHaveLength(1);
  });
});

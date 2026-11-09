import { test as base, expect } from "@playwright/test";

import {
  startContainerizedBackend,
  type ContainerizedBackend,
} from "../../fixtures/containerized-backend";

const test = base.extend<{ containerized: ContainerizedBackend }>({
  containerized: [
    // eslint-disable-next-line no-empty-pattern
    async ({}, use) => {
      const backend = await startContainerizedBackend();
      try {
        await use(backend);
      } finally {
        await backend.stop();
      }
    },
    { scope: "worker" },
  ],
});

type Seed = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  agentProfileId: string;
  worktreeProfileId: string;
};

async function seedWorkspace(containerized: ContainerizedBackend): Promise<Seed> {
  const api = containerized.api;
  const workspace = await api.createWorkspace(`Containerized E2E ${Date.now()}`);
  const workflow = await api.createWorkflow(workspace.id, "Containerized E2E workflow", "simple");
  const { steps } = await api.listWorkflowSteps(workflow.id);
  const sorted = steps.sort((left, right) => left.position - right.position);
  const startStep = sorted.find((step) => step.is_start_step) ?? sorted[0];

  const { agents } = await api.listAgents();
  const agentProfileId = agents
    .filter((agent) => agent.id !== "dynamic")
    .flatMap((agent) => agent.profiles ?? [])[0]?.id;
  if (!agentProfileId) {
    throw new Error("containerized backend exposed no launchable mock agent profile");
  }

  const { executors } = await api.listExecutors();
  const worktreeProfileId = executors.find((executor) => executor.type === "worktree")
    ?.profiles?.[0]?.id;
  if (!worktreeProfileId) {
    throw new Error("containerized backend exposed no worktree profile");
  }

  return {
    workspaceId: workspace.id,
    workflowId: workflow.id,
    startStepId: startStep.id,
    agentProfileId,
    worktreeProfileId,
  };
}

async function waitForSession(
  containerized: ContainerizedBackend,
  taskId: string,
  expected: RegExp,
) {
  await expect
    .poll(
      async () => {
        const { sessions } = await containerized.api.listTaskSessions(taskId);
        if (sessions.length === 0) return "none";
        const { session } = await containerized.api.getTaskSession(sessions[0].id);
        return session.state;
      },
      { timeout: 180_000, message: `waiting for session state ${expected}` },
    )
    .toMatch(expected);
  const { sessions } = await containerized.api.listTaskSessions(taskId);
  const { session } = await containerized.api.getTaskSession(sessions[0].id);
  return session;
}

async function wsRequest(
  baseUrl: string,
  action: string,
  payload: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const socketURL = `${baseUrl.replace(/^http/, "ws")}/ws`;
  const socket = new WebSocket(socketURL);
  await new Promise<void>((resolve, reject) => {
    socket.addEventListener("open", () => resolve(), { once: true });
    socket.addEventListener("error", () => reject(new Error(`WebSocket ${socketURL} failed`)), {
      once: true,
    });
  });
  const id = `containerized-${action}-${Date.now()}`;
  return new Promise<Record<string, unknown>>((resolve, reject) => {
    const timeout = setTimeout(() => {
      socket.close();
      reject(new Error(`timed out waiting for ${action}`));
    }, 60_000);
    socket.addEventListener("message", (event) => {
      const text = typeof event.data === "string" ? event.data : String(event.data);
      for (const line of text.split("\n")) {
        if (!line.trim()) continue;
        try {
          const message = JSON.parse(line) as { id?: string; type?: string; payload?: unknown };
          if (message.id !== id) continue;
          clearTimeout(timeout);
          socket.close();
          if (message.type === "error") {
            reject(new Error(`${action} failed: ${JSON.stringify(message.payload)}`));
            return;
          }
          resolve((message.payload ?? {}) as Record<string, unknown>);
          return;
        } catch {
          // Ignore non-JSON frames.
        }
      }
    });
    socket.send(
      JSON.stringify({
        id,
        type: "request",
        action,
        payload,
        timestamp: new Date().toISOString(),
      }),
    );
  });
}

test.describe("containerized backend executor boundary", () => {
  test("worker and backend containers have distinct daemon identities", async ({
    containerized,
  }) => {
    test.setTimeout(300_000);
    const seed = await seedWorkspace(containerized);
    const { executors } = await containerized.api.listExecutors();
    const docker = executors.find((executor) => executor.type === "local_docker");
    expect(docker, "containerized backend must expose the Docker executor").toBeTruthy();
    const profile = await containerized.api.createExecutorProfile(docker!.id, {
      name: `Containerized Docker ${Date.now()}`,
      config: { image_tag: "alpine:3.20" },
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });

    const task = await containerized.api.createTaskWithAgent(
      seed.workspaceId,
      `Containerized Docker ${Date.now()}`,
      seed.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        executor_profile_id: profile.id,
      },
    );

    const session = await waitForSession(
      containerized,
      task.id,
      /WAITING_FOR_INPUT|COMPLETED|IDLE/,
    );
    expect(session.execution_runtime).toBe("docker");
    expect(session.container_id, "worker must expose a full daemon container ID").toBeTruthy();
    expect(session.agent_execution_id).toBeTruthy();

    const workerID = session.container_id!;
    expect(containerized.docker(["inspect", "-f", "{{.Id}}", workerID])).toBe(workerID);
    const backendID = containerized.docker([
      "inspect",
      "-f",
      "{{.Id}}",
      containerized.backendContainerID,
    ]);
    expect(workerID).not.toBe(backendID);

    const labels = JSON.parse(
      containerized.docker(["inspect", "-f", "{{json .Config.Labels}}", workerID]),
    ) as Record<string, string>;
    expect(labels["kandev.task_id"]).toBe(task.id);
    expect(labels["kandev.session_id"]).toBe(session.id);
    expect(labels["kandev.managed"]).toBe("true");

    const processes = containerized.docker(["top", workerID, "-eo", "pid,args"]);
    expect(processes).toContain("mock-agent");

    const backendPID = containerized.dockerInspectPID(containerized.backendContainerID);
    const workerPID = containerized.dockerInspectPID(workerID);
    expect(backendPID).toBeGreaterThan(0);
    expect(workerPID).toBeGreaterThan(0);
    const backendNamespace = containerized.pidNamespace(backendPID);
    const workerNamespace = containerized.pidNamespace(workerPID);
    expect(workerNamespace).not.toBe(backendNamespace);
    expect(processes).not.toContain(String(backendPID));
  });

  test("a retained standalone worker is refused as Docker and still stops exactly", async ({
    containerized,
  }) => {
    test.setTimeout(300_000);
    const seed = await seedWorkspace(containerized);

    const task = await containerized.api.createTaskWithAgent(
      seed.workspaceId,
      `Containerized retained mismatch ${Date.now()}`,
      seed.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        executor_profile_id: seed.worktreeProfileId,
      },
    );
    const session = await waitForSession(
      containerized,
      task.id,
      /WAITING_FOR_INPUT|COMPLETED|IDLE/,
    );
    expect(session.execution_runtime).toBe("standalone");
    expect(session.task_environment_id).toBeTruthy();
    const sessionID = session.id;
    const environmentID = session.task_environment_id!;
    const executionID = session.agent_execution_id!;
    expect(executionID).toBeTruthy();

    const retainedRow = containerized.sqlite(
      `select runtime || '|' || agent_execution_id || '|' || status || '|' || coalesce(local_pid,0) from executors_running where session_id='${sessionID}';`,
    );
    const [retainedRuntime, retainedExecution, retainedStatus, localPIDText] =
      retainedRow.split("|");
    expect(retainedRuntime).toBe("standalone");
    expect(retainedExecution).toBe(executionID);
    expect(retainedStatus).toBe("ready");
    const localPID = Number(localPIDText);
    expect(localPID).toBeGreaterThan(0);
    expect(
      containerized.dockerStatus([
        "exec",
        containerized.backendContainerID,
        "sh",
        "-c",
        `kill -0 ${localPID}`,
      ]),
    ).toBe(0);

    // Select Docker on the same binding while the retained worker is
    // standalone. This is the defect, not an authorized environment
    // transition.
    containerized.sqlite(
      `update task_environments set executor_type='local_docker' where id='${environmentID}'; update task_sessions set executor_id='exec-local-docker' where id='${sessionID}';`,
    );

    const refused = await containerized.api.rawRequest(
      "POST",
      `/api/v1/task-sessions/${sessionID}/workspace/files/preflight`,
      { dir: "", repo: "", paths: ["refused.txt"] },
    );
    expect(refused.status).toBe(503);

    const unchanged = containerized.sqlite(
      `select runtime || '|' || agent_execution_id || '|' || status from executors_running where session_id='${sessionID}';`,
    );
    expect(unchanged).toBe(`standalone|${executionID}|ready`);
    expect(containerized.docker(["ps", "-aq", "--filter", `label=kandev.task_id=${task.id}`])).toBe(
      "",
    );
    const afterRefusal = await containerized.api.getTaskSession(sessionID);
    expect(afterRefusal.session.execution_runtime).toBe("standalone");
    expect(afterRefusal.session.state).toMatch(/WAITING_FOR_INPUT|COMPLETED|IDLE/);

    const stopped = await wsRequest(containerized.baseUrl, "session.stop", {
      session_id: sessionID,
    });
    expect(stopped.success).toBe(true);

    await expect
      .poll(
        () =>
          containerized.sqlite(
            `select status from executors_running where session_id='${sessionID}';`,
          ),
        { timeout: 30_000 },
      )
      .toMatch(/stopped|complete|^$/);
    const finalState = containerized.sqlite(
      `select state from task_sessions where id='${sessionID}';`,
    );
    expect(finalState).toMatch(/CANCELLED|COMPLETED|WAITING_FOR_INPUT/);
    const retired = containerized.sqlite(
      `select agent_execution_id || '|' || runtime from executors_running where session_id='${sessionID}';`,
    );
    if (retired) {
      expect(retired).toBe(`${executionID}|standalone`);
    }
  });
});

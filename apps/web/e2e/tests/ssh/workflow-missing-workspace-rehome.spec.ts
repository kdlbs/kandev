import { expect, test } from "../../fixtures/ssh-test-base";
import { execInContainer } from "../../helpers/ssh";
import { waitForLatestSessionDone, waitForSessionState } from "../../helpers/session";

test("phase transition rehomes one missing SSH workspace for the same task", async ({
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "SSH rehome workflow");
  const phaseA = await apiClient.createWorkflowStep(workflow.id, "Phase A", 0, {
    is_start_step: true,
  });
  const phaseB = await apiClient.createWorkflowStep(workflow.id, "Phase B", 1);
  await apiClient.updateWorkflowStep(phaseA.id, {
    prompt: 'e2e:message("phase A complete")',
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  await apiClient.updateWorkflowStep(phaseB.id, {
    prompt: 'e2e:delay(30000)\ne2e:message("phase B running")',
    events: { on_enter: [{ type: "reset_agent_context" }, { type: "auto_start_agent" }] },
  });

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "SSH phase rehome",
    seedData.agentProfileId,
    {
      workflow_id: workflow.id,
      workflow_step_id: phaseA.id,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.sshExecutorProfileId,
    },
  );
  await waitForLatestSessionDone(apiClient, task.id, 1, "phase A should complete");
  const before = await apiClient.listTaskSessions(task.id);
  expect(before.sessions).toHaveLength(1);
  const sessionId = before.sessions[0].id;
  const sshRow = (await apiClient.listSSHSessions(seedData.sshExecutorId)).find(
    (session) => session.task_id === task.id,
  );
  expect(sshRow?.remote_task_dir).toContain("/tasks/");
  const remoteTaskDir = sshRow!.remote_task_dir!;
  execInContainer(seedData.sshTarget, ["rm", "-rf", "--", remoteTaskDir]);

  await apiClient.moveTask(task.id, workflow.id, phaseB.id);
  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId,
    expectedState: "RUNNING",
    message: "phase B should reach RUNNING after automatic rehome",
  });
  const afterTask = await apiClient.getTask(task.id);
  const afterSessions = await apiClient.listTaskSessions(task.id);
  expect(afterTask.id).toBe(task.id);
  expect(afterTask.workflow_step_id).toBe(phaseB.id);
  expect(afterSessions.sessions).toHaveLength(1);
  expect(afterSessions.sessions[0].id).toBe(sessionId);
  const replacement = (await apiClient.listSSHSessions(seedData.sshExecutorId)).filter(
    (session) => session.task_id === task.id,
  );
  expect(replacement).toHaveLength(1);
  expect(replacement[0].remote_task_dir).toBe(remoteTaskDir);
});

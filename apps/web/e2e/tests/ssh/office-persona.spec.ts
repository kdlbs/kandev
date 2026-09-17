import { test, expect } from "../../fixtures/ssh-test-base";
import { OfficeApiClient } from "../../helpers/office-api-client";
import { execInContainer } from "../../helpers/ssh";

test("Office persona uses its SSH profile and calls back with its run identity", async ({
  apiClient,
  seedData,
  backend,
  request,
}) => {
  test.setTimeout(180_000);
  const office = new OfficeApiClient(backend.baseUrl);
  execInContainer(seedData.sshTarget, [
    "sh",
    "-c",
    `mv /usr/local/bin/mock-agent /usr/local/bin/mock-agent-real
cat > /usr/local/bin/mock-agent <<'WRAPPER'
#!/bin/bash
set -e
"$KANDEV_CLI" kandev tasks message --prompt "Verified agent process callback" >/dev/null
exec /usr/local/bin/mock-agent-real "$@"
WRAPPER
chmod +x /usr/local/bin/mock-agent`,
  ]);
  const workspace = await office.completeOnboarding({
    workspaceName: "Remote office",
    taskPrefix: "REMOTE",
    agentName: "Coordinator",
    agentProfileId: seedData.agentProfileId,
    executorPreference: "local_pc",
  });
  try {
    const profile = await apiClient.createExecutorProfile(seedData.sshExecutorId, {
      name: "Office remote worker",
      config: {},
      env_vars: [],
      prepare_script:
        '#!/bin/bash\nset -eu\n"$KANDEV_CLI" kandev tasks message --prompt "Verified remote Office callback"\n',
    });
    const agent = await office.createAgent(workspace.workspaceId, {
      name: "Remote worker",
      role: "assistant",
      agent_profile_id: seedData.agentProfileId,
    });
    await office.updateAgent(agent.id as string, {
      executor_preference: JSON.stringify({ type: "ssh", executor_profile_id: profile.id }),
    });
    const opened = await request.post(
      `${backend.baseUrl}/api/v1/office/workspaces/${workspace.workspaceId}/agents/${agent.id}/conversation`,
    );
    expect(opened.ok()).toBeTruthy();
    const { channel } = await opened.json();
    const posted = await request.post(
      `${backend.baseUrl}/api/v1/office/tasks/${channel.task_id}/comments`,
      {
        data: { body: "Confirm that you can respond from the remote account environment." },
      },
    );
    expect(posted.ok()).toBeTruthy();
    await expect
      .poll(
        async () => {
          const res = await request.get(
            `${backend.baseUrl}/api/v1/office/tasks/${channel.task_id}/comments`,
          );
          const { comments } = await res.json();
          return comments.some(
            (c: { body: string; authorId: string }) =>
              c.body === "Verified agent process callback" && c.authorId === agent.id,
          );
        },
        { timeout: 90_000 },
      )
      .toBeTruthy();
    const environment = await apiClient.getTaskEnvironment(channel.task_id);
    expect(environment?.executor_type).toBe("ssh");
    await expect
      .poll(
        async () => {
          const res = await request.get(
            `${backend.baseUrl}/api/v1/office/tasks/${channel.task_id}/comments`,
          );
          const { comments } = await res.json();
          return comments.some((c: { source: string }) => c.source === "session");
        },
        { timeout: 45_000 },
      )
      .toBeTruthy();
    await request.post(`${backend.baseUrl}/api/v1/office/tasks/${channel.task_id}/comments`, {
      data: { body: "Respond again with a fresh run." },
    });
    await expect
      .poll(
        async () => {
          const res = await request.get(
            `${backend.baseUrl}/api/v1/office/tasks/${channel.task_id}/comments`,
          );
          const { comments } = await res.json();
          return comments.filter(
            (c: { body: string }) => c.body === "Verified agent process callback",
          ).length;
        },
        { timeout: 45_000 },
      )
      .toBeGreaterThanOrEqual(2);
  } finally {
    await apiClient.e2eReset(workspace.workspaceId, []);
  }
});

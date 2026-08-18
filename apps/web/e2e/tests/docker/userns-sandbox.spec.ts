import { test, expect } from "../../fixtures/docker-test-base";
import { E2E_IMAGE_TAG } from "../../fixtures/docker-probe";
import { dockerExec, dockerSecurityOpt } from "../../helpers/docker";
import { waitForLatestSessionDone } from "../../helpers/session";

test.describe("Docker executor user namespace sandbox", () => {
  test("enables bwrap user namespaces only for opted-in profiles", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const { executors } = await apiClient.listExecutors();
    const executor = executors.find((item) => item.type === "local_docker");
    expect(executor?.id).toBeTruthy();
    const [enabledProfile, disabledProfile] = await Promise.all([
      apiClient.createExecutorProfile(executor!.id, {
        name: "User namespaces enabled",
        config: { image_tag: E2E_IMAGE_TAG, allow_user_namespaces: "true" },
        prepare_script: "",
        cleanup_script: "",
        env_vars: [],
      }),
      apiClient.createExecutorProfile(executor!.id, {
        name: "User namespaces disabled",
        config: { image_tag: E2E_IMAGE_TAG },
        prepare_script: "",
        cleanup_script: "",
        env_vars: [],
      }),
    ]);
    try {
      const launch = async (title: string, profileID: string) => {
        const task = await apiClient.createTaskWithAgent(
          seedData.workspaceId,
          title,
          seedData.agentProfileId,
          {
            description: "/e2e:simple-message",
            workflow_id: seedData.workflowId,
            workflow_step_id: seedData.startStepId,
            repository_ids: [seedData.repositoryId],
            executor_profile_id: profileID,
          },
        );
        await waitForLatestSessionDone(apiClient, task.id, 1, `Waiting for ${title}`);
        const environment = await apiClient.getTaskEnvironment(task.id);
        expect(environment?.container_id).toBeTruthy();
        return environment!.container_id!;
      };
      const enabledContainer = await launch("Userns enabled", enabledProfile.id);
      const enabledSecurityOpt = dockerSecurityOpt(enabledContainer);
      expect(enabledSecurityOpt).toHaveLength(2);
      expect(enabledSecurityOpt).toContain("apparmor=unconfined");
      expect(enabledSecurityOpt![0]).toMatch(/^seccomp=\{/);
      const enabledBwrap = dockerExec(
        enabledContainer,
        "bwrap",
        "--unshare-user",
        "--dev-bind",
        "/",
        "/",
        "true",
      );
      expect(enabledBwrap.status, `${enabledBwrap.stdout}${enabledBwrap.stderr}`).toBe(0);
      const disabledContainer = await launch("Userns disabled", disabledProfile.id);
      expect(dockerSecurityOpt(disabledContainer)).toBeNull();
      const disabledUnshare = dockerExec(disabledContainer, "unshare", "--user", "true");
      expect(disabledUnshare.status).not.toBe(0);
      expect(``).toContain("Operation not permitted");
      const disabledBwrap = dockerExec(
        disabledContainer,
        "bwrap",
        "--unshare-user",
        "--dev-bind",
        "/",
        "/",
        "true",
      );
      expect(disabledBwrap.status).not.toBe(0);
    } finally {
      await Promise.all([
        apiClient.deleteExecutorProfile(enabledProfile.id).catch(() => {}),
        apiClient.deleteExecutorProfile(disabledProfile.id).catch(() => {}),
      ]);
    }
  });
});

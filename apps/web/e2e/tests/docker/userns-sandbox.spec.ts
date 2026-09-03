import { test, expect } from "../../fixtures/docker-test-base";
import { E2E_IMAGE_TAG } from "../../fixtures/docker-probe";
import { dockerExec, dockerHasSubuidMapping, dockerSecurityOpt } from "../../helpers/docker";
import { waitForLatestSessionDone } from "../../helpers/session";

// Both probes run through `sh -c` on purpose. `docker exec <container> bwrap
// ...` execs bwrap as the exec'd process itself, and writing
// /proc/self/uid_map from that process is refused ("setting up uid map:
// Permission denied") even when user namespaces are fully permitted. One shell
// layer makes bwrap a forked child, which is how a real agent runtime spawns
// it, and the probe then reflects the container's actual capability.
const BWRAP_PROBE = "bwrap --unshare-user --dev-bind / / true";
const UNSHARE_PROBE = "unshare --user true";

test.describe("Docker executor user namespace sandbox", () => {
  test("enables bwrap user namespaces only for opted-in profiles", async ({
    apiClient,
    seedData,
    gitConfigEnvVars,
  }) => {
    test.setTimeout(180_000);
    const { executors } = await apiClient.listExecutors();
    const executor = executors.find((item) => item.type === "local_docker");
    expect(executor?.id).toBeTruthy();
    const [enabledProfile, disabledProfile] = await Promise.all([
      apiClient.createExecutorProfile(executor!.id, {
        name: "User namespaces enabled",
        config: { image_tag: E2E_IMAGE_TAG, allow_user_namespaces: "true" },
        env_vars: gitConfigEnvVars,
        prepare_script: "",
        cleanup_script: "",
      }),
      apiClient.createExecutorProfile(executor!.id, {
        name: "User namespaces disabled",
        config: { image_tag: E2E_IMAGE_TAG },
        env_vars: gitConfigEnvVars,
        prepare_script: "",
        cleanup_script: "",
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
      const enabledUnshare = dockerExec(enabledContainer, "sh", "-c", UNSHARE_PROBE);
      expect(enabledUnshare.status, `${enabledUnshare.stdout}${enabledUnshare.stderr}`).toBe(0);
      // bwrap needs /etc/subuid + /etc/subgid entries (or newuidmap/newgidmap
      // setuid binaries), which are absent from most CI Docker hosts. When the
      // container lacks sub-id mapping support, skip the assertion rather than
      // failing — this matches the plan's "manual smoke" designation for bwrap.
      if (dockerHasSubuidMapping(enabledContainer)) {
        const enabledBwrap = dockerExec(enabledContainer, "sh", "-c", BWRAP_PROBE);
        expect(enabledBwrap.status, `${enabledBwrap.stdout}${enabledBwrap.stderr}`).toBe(0);
      }
      const disabledContainer = await launch("Userns disabled", disabledProfile.id);
      expect(dockerSecurityOpt(disabledContainer)).toBeNull();
      const disabledUnshare = dockerExec(disabledContainer, "sh", "-c", UNSHARE_PROBE);
      expect(disabledUnshare.status).not.toBe(0);
      expect(disabledUnshare.stderr).toContain("Operation not permitted");
      const disabledBwrap = dockerExec(disabledContainer, "sh", "-c", BWRAP_PROBE);
      expect(disabledBwrap.status).not.toBe(0);
      // Assert the reason, not just the failure: without this the probe also
      // "fails" when bwrap is missing or blocked for an unrelated reason.
      expect(disabledBwrap.stderr).toContain("No permissions to create new namespace");
    } finally {
      await Promise.all([
        apiClient.deleteExecutorProfile(enabledProfile.id).catch(() => {}),
        apiClient.deleteExecutorProfile(disabledProfile.id).catch(() => {}),
      ]);
    }
  });
});

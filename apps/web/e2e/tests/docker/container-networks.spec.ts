import { test, expect } from "../../fixtures/docker-test-base";
import { E2E_IMAGE_TAG } from "../../fixtures/docker-probe";
import {
  dockerContainerNetworks,
  dockerRemove,
  dockerNetworkCreate,
  dockerNetworkCreateWithDriver,
  dockerNetworkRemove,
  dockerPublishedPort,
} from "../../helpers/docker";
import { waitForLatestSessionDone } from "../../helpers/session";

/** The container port Kandev reaches agentctl through. */
const AGENTCTL_PORT = 39429;

test.describe("Docker executor container networks", () => {
  /**
   * The capability's whole point is that a container can hold more than one
   * attachment without losing the one Kandev talks to it through. Asserting
   * both attachments proves the placement; asserting the published agentctl
   * port on top of that proves the placement did not cost us the launch.
   *
   * @covers AC-EXECUTORS-DOCKER-NETWORKS-001.1
   * @covers AC-EXECUTORS-DOCKER-NETWORKS-001.7
   * @covers AC-EXECUTORS-DOCKER-NETWORKS-002.2
   */
  test("launches a task container on its configured networks", async ({ apiClient, seedData }) => {
    test.setTimeout(180_000);
    const suffix = `${Date.now()}-${Math.floor(Math.random() * 1_000)}`;
    const primary = `kandev-e2e-primary-${suffix}`;
    const secondary = `kandev-e2e-secondary-${suffix}`;

    const { executors } = await apiClient.listExecutors();
    const executor = executors.find((item) => item.type === "local_docker");
    expect(executor?.id).toBeTruthy();

    dockerNetworkCreate(primary);
    dockerNetworkCreate(secondary);
    let profileId: string | undefined;
    // Kandev preserves a task container on an ordinary stop, and a container
    // holding an endpoint blocks the network removal below.
    let containerID: string | undefined;
    try {
      const profile = await apiClient.createExecutorProfile(executor!.id, {
        name: `Container networks ${suffix}`,
        config: {
          image_tag: E2E_IMAGE_TAG,
          docker_network: primary,
          docker_network_gw_priority: "10",
          docker_additional_networks: JSON.stringify([{ name: secondary, gw_priority: -10 }]),
        },
        env_vars: seedData.gitConfigEnvVars,
        prepare_script: "",
        cleanup_script: "",
      });
      profileId = profile.id;

      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Container networks",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: profile.id,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for the networked task");

      const environment = await apiClient.getTaskEnvironment(task.id);
      expect(environment?.container_id).toBeTruthy();
      containerID = environment!.container_id!;

      expect(dockerContainerNetworks(containerID)).toEqual([primary, secondary].sort());
      // The agent reached done above, which it cannot do unless the backend
      // dialled it; this pins the mechanism that made it reachable.
      expect(dockerPublishedPort(containerID, AGENTCTL_PORT)).not.toBeNull();
    } finally {
      if (profileId) await apiClient.deleteExecutorProfile(profileId).catch(() => {});
      if (containerID) dockerRemove(containerID);
      dockerNetworkRemove(secondary);
      dockerNetworkRemove(primary);
    }
  });

  /**
   * A macvlan primary would place the container on the LAN and simultaneously
   * make agentctl unreachable. The launch has to fail saying so, rather than
   * producing a container that starts and never answers.
   *
   * @covers AC-EXECUTORS-DOCKER-NETWORKS-001.6
   */
  test("refuses a primary network that cannot publish ports", async ({ apiClient, seedData }) => {
    test.setTimeout(180_000);
    const suffix = `${Date.now()}-${Math.floor(Math.random() * 1_000)}`;
    const network = `kandev-e2e-ipvlan-${suffix}`;

    const { executors } = await apiClient.listExecutors();
    const executor = executors.find((item) => item.type === "local_docker");
    expect(executor?.id).toBeTruthy();

    // An ipvlan network needs no parent interface to exist, unlike macvlan,
    // so the driver rejection is exercised on any host.
    const created = dockerNetworkCreateWithDriver(network, "ipvlan");
    test.skip(!created, "host Docker does not support creating an ipvlan network");

    let profileId: string | undefined;
    try {
      const profile = await apiClient.createExecutorProfile(executor!.id, {
        name: `Unpublishable network ${suffix}`,
        config: { image_tag: E2E_IMAGE_TAG, docker_network: network },
        env_vars: seedData.gitConfigEnvVars,
        prepare_script: "",
        cleanup_script: "",
      });
      profileId = profile.id;

      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Unpublishable network",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: profile.id,
        },
      );

      await expect
        .poll(
          async () => (await apiClient.getTaskEnvironment(task.id))?.container_id ?? "",
          { message: "the launch must not produce a container", timeout: 60_000 },
        )
        .toBe("");
    } finally {
      if (profileId) await apiClient.deleteExecutorProfile(profileId).catch(() => {});
      dockerNetworkRemove(network);
    }
  });
});

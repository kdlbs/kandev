import { describe, expect, it } from "vitest";
import {
  buildSaveConfig,
  getExecutorProfileRuntimeFlags,
  type ExecutorProfileConfigForm,
} from "./serialize-executor-config";
import {
  createDefaultKubernetesProfileConfig,
  replaceKubernetesProfileConfig,
} from "../kubernetes-config";

function form(overrides: Partial<ExecutorProfileConfigForm> = {}): ExecutorProfileConfigForm {
  return {
    isSprites: false,
    networkPolicyRules: [],
    isRemote: true,
    remoteCredentials: [],
    configBundleIds: [],
    agentEnvVars: {},
    gitIdentityMode: "override",
    localGitIdentity: { userName: "", userEmail: "" },
    gitUserName: "",
    gitUserEmail: "",
    isDocker: false,
    isLocalDocker: false,
    dockerfile: "",
    imageTag: "",
    allowUserNamespaces: false,
    isSSH: false,
    sshShell: "",
    sshReclaimTaskDir: false,
    primaryNetwork: "",
    primaryGwPriority: "",
    additionalNetworks: [],
    ...overrides,
  };
}

describe("buildSaveConfig", () => {
  it("classifies Kubernetes profiles as remote without Docker-only behavior", () => {
    expect(getExecutorProfileRuntimeFlags("k8s")).toEqual({
      isRemote: true,
      isDocker: false,
      isSprites: false,
      isKubernetes: true,
    });
  });

  it("preserves unrelated profile keys while saving Kubernetes and remote fields", () => {
    const shared = buildSaveConfig(
      form({
        remoteCredentials: ["git-auth"],
        configBundleIds: ["codex.settings"],
        gitUserName: "Kandev Agent",
        gitUserEmail: "agent@kandev.ai",
      }),
      { custom_key: "keep", "workspace.mode": "empty_dir" },
    );
    const config = replaceKubernetesProfileConfig(shared, {
      ...createDefaultKubernetesProfileConfig(),
      platform: "linux/arm64",
      workspaceMode: "existing_claim",
      claimName: "shared-workspace",
    });

    expect(config).toMatchObject({
      custom_key: "keep",
      remote_credentials: '["git-auth"]',
      agent_config_bundles: '["codex.settings"]',
      git_user_name: "Kandev Agent",
      git_user_email: "agent@kandev.ai",
      platform: "linux/arm64",
      "workspace.mode": "existing_claim",
      "workspace.claim_name": "shared-workspace",
    });
  });

  it("persists selected configuration without requiring authentication", () => {
    const config = buildSaveConfig(form({ configBundleIds: ["mock.settings"] }), {
      remote_credentials: "stale",
      keep: "yes",
    });

    expect(config).toEqual({
      agent_config_bundles: '["mock.settings"]',
      keep: "yes",
    });
  });

  it("persists authentication without requiring configuration", () => {
    const config = buildSaveConfig(form({ remoteCredentials: ["codex-auth"] }));

    expect(config).toEqual({ remote_credentials: '["codex-auth"]' });
  });

  it("persists allowUserNamespaces when enabled on a Docker profile", () => {
    const config = buildSaveConfig(
      form({ isDocker: true, isLocalDocker: true, allowUserNamespaces: true }),
    );
    expect(config.allow_user_namespaces).toBe("true");
  });

  it("removes allowUserNamespaces key when disabled", () => {
    const config = buildSaveConfig(
      form({ isDocker: true, isLocalDocker: true, allowUserNamespaces: false }),
    );
    expect(config.allow_user_namespaces).toBeUndefined();
  });

  it("removes allowUserNamespaces when not a Docker profile", () => {
    const config = buildSaveConfig(form({ isDocker: false, allowUserNamespaces: true }));
    expect(config.allow_user_namespaces).toBeUndefined();
  });

  it("removes allowUserNamespaces from a remote Docker profile", () => {
    const config = buildSaveConfig(
      form({ isDocker: true, isLocalDocker: false, allowUserNamespaces: true }),
      { allow_user_namespaces: "true" },
    );
    expect(config.allow_user_namespaces).toBeUndefined();
  });
});

describe("buildSaveConfig ssh_reclaim_task_dir", () => {
  it("writes an explicit false for an SSH profile that leaves reclamation off", () => {
    const config = buildSaveConfig(form({ isSSH: true, sshReclaimTaskDir: false }));

    expect(config.ssh_reclaim_task_dir).toBe("false");
  });

  it("writes the exact string the backend compares when reclamation is on", () => {
    const config = buildSaveConfig(form({ isSSH: true, sshReclaimTaskDir: true }));

    expect(config.ssh_reclaim_task_dir).toBe("true");
  });

  it("never arms reclamation on a non-SSH profile, even from a stale stored value", () => {
    const config = buildSaveConfig(form({ isSSH: false, sshReclaimTaskDir: true }), {
      ssh_reclaim_task_dir: "true",
    });

    expect(config.ssh_reclaim_task_dir).toBeUndefined();
  });
});

const DOCKER_PRIMARY_NETWORK = "lab-bridge";

describe("buildSaveConfig docker networks", () => {
  it("persists the primary network and its gateway priority", () => {
    const config = buildSaveConfig(
      form({ isDocker: true, primaryNetwork: ` ${DOCKER_PRIMARY_NETWORK} `, primaryGwPriority: " 0 " }),
    );

    expect(config.docker_network).toBe(DOCKER_PRIMARY_NETWORK);
    expect(config.docker_network_gw_priority).toBe("0");
  });

  it("persists additional networks in order, omitting an unset priority", () => {
    const config = buildSaveConfig(
      form({
        isDocker: true,
        additionalNetworks: [
          { name: " lan-macvlan ", gwPriority: "-10" },
          { name: "metrics-internal", gwPriority: "" },
        ],
      }),
    );

    expect(JSON.parse(config.docker_additional_networks)).toEqual([
      { name: "lan-macvlan", gw_priority: -10 },
      { name: "metrics-internal" },
    ]);
  });

  it("drops a row the operator added but never named", () => {
    const config = buildSaveConfig(
      form({ isDocker: true, additionalNetworks: [{ name: "  ", gwPriority: "3" }] }),
    );

    expect(config.docker_additional_networks).toBeUndefined();
  });

  it("persists nothing for a profile that configures no network", () => {
    const config = buildSaveConfig(form({ isDocker: true }));

    expect(config.docker_network).toBeUndefined();
    expect(config.docker_network_gw_priority).toBeUndefined();
    expect(config.docker_additional_networks).toBeUndefined();
  });

  it("clears stored network keys when the profile is not a Docker profile", () => {
    const config = buildSaveConfig(
      form({ isDocker: false, primaryNetwork: DOCKER_PRIMARY_NETWORK }),
      {
        docker_network: DOCKER_PRIMARY_NETWORK,
        docker_network_gw_priority: "0",
        docker_additional_networks: '[{"name":"lan-macvlan"}]',
      },
    );

    expect(config.docker_network).toBeUndefined();
    expect(config.docker_network_gw_priority).toBeUndefined();
    expect(config.docker_additional_networks).toBeUndefined();
  });

  // A priority with no network is refused by the backend, so the editor must
  // not save one: the operator would get a launch failure for a field the
  // editor let them leave in an impossible state.
  it("drops a gateway priority left behind without a primary network", () => {
    const config = buildSaveConfig(form({ isDocker: true, primaryGwPriority: "5" }));

    expect(config.docker_network_gw_priority).toBeUndefined();
  });
});

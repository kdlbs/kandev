import { describe, expect, it } from "vitest";
import {
  DEFAULT_KUBERNETES_IMAGE,
  createDefaultKubernetesProfileConfig,
  getKubernetesProfileValidationError,
  isKubernetesExecutorDirty,
  isKubernetesProfileDirty,
  parseKubernetesExecutorConfig,
  parseKubernetesProfileConfig,
  replaceKubernetesProfileConfig,
  serializeKubernetesExecutorConfig,
  serializeKubernetesProfileConfig,
} from "./kubernetes-config";

describe("Kubernetes executor settings config", () => {
  it("uses the multi-architecture Kandev image as the starter workload", () => {
    expect(DEFAULT_KUBERNETES_IMAGE).toBe("ghcr.io/kdlbs/kandev:latest");
    expect(createDefaultKubernetesProfileConfig().podTemplateYaml).toContain(
      `image: ${DEFAULT_KUBERNETES_IMAGE}`,
    );
  });

  it("provides serialization and dirty-baseline helpers", async () => {
    const config = (await import("./kubernetes-config")) as Record<string, unknown>;

    expect(
      [
        config.serializeKubernetesExecutorConfig,
        config.parseKubernetesProfileConfig,
        config.serializeKubernetesProfileConfig,
        config.replaceKubernetesProfileConfig,
        config.isKubernetesExecutorDirty,
        config.isKubernetesProfileDirty,
      ].every((value) => typeof value === "function"),
    ).toBe(true);
  });

  it("parses a persisted kubeconfig executor", () => {
    expect(
      parseKubernetesExecutorConfig("Production cluster", {
        auth_mode: "kubeconfig",
        kubeconfig_path: " /etc/kandev/cluster.yaml ",
        kube_context: "production",
        namespace: "agents",
        request_timeout_seconds: "45",
      }),
    ).toEqual({
      name: "Production cluster",
      authMode: "kubeconfig",
      kubeconfigPath: "/etc/kandev/cluster.yaml",
      kubeContext: "production",
      namespace: "agents",
      requestTimeoutSeconds: "45",
    });
  });

  it("clears kubeconfig-only fields for in-cluster auth", () => {
    expect(
      parseKubernetesExecutorConfig("Cluster", {
        auth_mode: "in_cluster",
        kubeconfig_path: "/stale/config",
        kube_context: "stale",
        namespace: "kandev",
      }),
    ).toMatchObject({
      authMode: "in_cluster",
      kubeconfigPath: "",
      kubeContext: "",
      namespace: "kandev",
      requestTimeoutSeconds: "30",
    });
  });

  it("serializes only fields accepted by kubeconfig auth", () => {
    expect(
      serializeKubernetesExecutorConfig({
        name: " Cluster ",
        authMode: "kubeconfig",
        kubeconfigPath: " /etc/kandev/config ",
        kubeContext: " production ",
        namespace: " agents ",
        requestTimeoutSeconds: " 45 ",
      }),
    ).toEqual({
      auth_mode: "kubeconfig",
      kubeconfig_path: "/etc/kandev/config",
      kube_context: "production",
      namespace: "agents",
      request_timeout_seconds: "45",
    });
  });

  it("omits stale kubeconfig fields from in-cluster serialization", () => {
    expect(
      serializeKubernetesExecutorConfig({
        name: "Cluster",
        authMode: "in_cluster",
        kubeconfigPath: "/stale/config",
        kubeContext: "stale",
        namespace: "kandev",
        requestTimeoutSeconds: "30",
      }),
    ).toEqual({
      auth_mode: "in_cluster",
      namespace: "kandev",
      request_timeout_seconds: "30",
    });
  });
});

describe("Kubernetes profile config parsing", () => {
  it("parses dotted managed-PVC profile config and defaults access mode", () => {
    expect(
      parseKubernetesProfileConfig({
        platform: "linux/arm64",
        main_container: "worker",
        pod_template_yaml: "template-yaml\n",
        "workspace.mode": "managed_pvc",
        "workspace.size": "20Gi",
        "workspace.storage_class": "fast",
      }),
    ).toEqual({
      platform: "linux/arm64",
      mainContainer: "worker",
      podTemplateYaml: "template-yaml\n",
      workspaceMode: "managed_pvc",
      workspaceSize: "20Gi",
      storageClass: "fast",
      accessModes: ["ReadWriteOnce"],
      claimName: "",
    });
  });

  it("parses legacy workspace aliases without retaining irrelevant fields", () => {
    expect(
      parseKubernetesProfileConfig({
        platform: "linux/amd64",
        main_container: "kandev-agent",
        pod_template_yaml: "template-yaml",
        workspace_mode: "existing_claim",
        workspace_claim_name: "shared-workspace",
        workspace_size: "stale",
        workspace_access_modes: '["ReadWriteMany"]',
      }),
    ).toMatchObject({
      workspaceMode: "existing_claim",
      workspaceSize: "",
      storageClass: "",
      accessModes: [],
      claimName: "shared-workspace",
    });
  });

  it("preserves unsupported persisted access modes and reports them as invalid", () => {
    const parsed = parseKubernetesProfileConfig({
      "workspace.mode": "managed_pvc",
      "workspace.access_modes": '["UnsupportedMode"]',
    });

    expect(parsed.accessModes).toEqual(["UnsupportedMode"]);
    expect(getKubernetesProfileValidationError(parsed)).toBe("access_mode_invalid");
  });
});

describe("Kubernetes profile config serialization", () => {
  it("serializes managed PVC fields and JSON access modes", () => {
    expect(
      serializeKubernetesProfileConfig({
        ...createDefaultKubernetesProfileConfig(),
        workspaceSize: " 25Gi ",
        storageClass: " fast ",
        accessModes: ["ReadWriteOnce", "ReadOnlyMany"],
        claimName: "stale-claim",
      }),
    ).toMatchObject({
      "workspace.mode": "managed_pvc",
      "workspace.size": "25Gi",
      "workspace.storage_class": "fast",
      "workspace.access_modes": '["ReadWriteOnce","ReadOnlyMany"]',
    });
  });

  it.each([
    {
      mode: "empty_dir" as const,
      expected: { "workspace.mode": "empty_dir" },
    },
    {
      mode: "existing_claim" as const,
      expected: { "workspace.mode": "existing_claim", "workspace.claim_name": "shared" },
    },
  ])("serializes only conditional $mode workspace fields", ({ mode, expected }) => {
    const serialized = serializeKubernetesProfileConfig({
      ...createDefaultKubernetesProfileConfig(),
      workspaceMode: mode,
      workspaceSize: "10Gi",
      storageClass: "fast",
      accessModes: ["ReadWriteMany"],
      claimName: " shared ",
    });

    expect(
      Object.fromEntries(Object.entries(serialized).filter(([key]) => key.startsWith("workspace"))),
    ).toEqual(expected);
  });

  it("replaces Kubernetes keys while preserving shared profile config", () => {
    expect(
      replaceKubernetesProfileConfig(
        {
          remote_credentials: '["git"]',
          workspace_mode: "empty_dir",
          workspace_size: "stale",
          "workspace.claim_name": "stale",
        },
        { ...createDefaultKubernetesProfileConfig(), workspaceMode: "empty_dir" },
      ),
    ).toMatchObject({
      remote_credentials: '["git"]',
      "workspace.mode": "empty_dir",
    });
  });
});

describe("Kubernetes settings baselines and validation", () => {
  it("normalizes executor fields before dirty comparison", () => {
    const baseline = parseKubernetesExecutorConfig("Cluster", {
      auth_mode: "in_cluster",
      namespace: "agents",
      request_timeout_seconds: "30",
    });

    expect(isKubernetesExecutorDirty({ ...baseline, namespace: " agents " }, baseline)).toBe(false);
    expect(isKubernetesExecutorDirty({ ...baseline, namespace: "other" }, baseline)).toBe(true);
  });

  it("detects raw template and workspace dirty state", () => {
    const baseline = createDefaultKubernetesProfileConfig();

    expect(isKubernetesProfileDirty({ ...baseline }, baseline)).toBe(false);
    expect(
      isKubernetesProfileDirty(
        { ...baseline, podTemplateYaml: `${baseline.podTemplateYaml}\n` },
        baseline,
      ),
    ).toBe(true);
    expect(isKubernetesProfileDirty({ ...baseline, workspaceMode: "empty_dir" }, baseline)).toBe(
      true,
    );
  });

  it("validates required and conditional profile fields", () => {
    const valid = createDefaultKubernetesProfileConfig();

    expect(getKubernetesProfileValidationError(valid)).toBeNull();
    expect(getKubernetesProfileValidationError({ ...valid, mainContainer: " " })).toBe(
      "main_container_required",
    );
    expect(getKubernetesProfileValidationError({ ...valid, mainContainer: "NOT_VALID" })).toBe(
      "main_container_invalid",
    );
    expect(getKubernetesProfileValidationError({ ...valid, podTemplateYaml: "" })).toBe(
      "pod_template_required",
    );
    expect(
      getKubernetesProfileValidationError({
        ...valid,
        workspaceMode: "managed_pvc",
        workspaceSize: " ",
      }),
    ).toBe("workspace_size_required");
    expect(
      getKubernetesProfileValidationError({
        ...valid,
        workspaceMode: "managed_pvc",
        accessModes: [],
      }),
    ).toBe("access_mode_required");
    expect(
      getKubernetesProfileValidationError({
        ...valid,
        workspaceMode: "existing_claim",
        claimName: " ",
      }),
    ).toBe("claim_name_required");
  });
});

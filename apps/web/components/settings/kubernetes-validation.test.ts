import { describe, expect, it } from "vitest";
import { createDefaultKubernetesProfileConfig } from "./kubernetes-config";
import {
  getKubernetesCreateContributorState,
  kubernetesExecutorInvalidReason,
  kubernetesProfileInvalidReason,
} from "./kubernetes-validation";

describe("kubernetesProfileInvalidReason", () => {
  const translate = (key: string) => `translated:${key}`;

  it("maps pure validation errors to localized explanations", () => {
    const valid = createDefaultKubernetesProfileConfig();

    expect(kubernetesProfileInvalidReason(valid, translate)).toBeUndefined();
    expect(kubernetesProfileInvalidReason({ ...valid, mainContainer: "" }, translate)).toBe(
      "translated:executors:kubernetesMainContainerRequired",
    );
    expect(
      kubernetesProfileInvalidReason(
        { ...valid, podTemplateYaml: "x".repeat(256 * 1024 + 1) },
        translate,
      ),
    ).toBe("translated:executors:kubernetesPodTemplateTooLarge");
  });
});

describe("kubernetesExecutorInvalidReason", () => {
  const translate = (key: string) => `translated:${key}`;
  const valid = {
    name: "Cluster",
    authMode: "kubeconfig" as const,
    kubeconfigPath: "/etc/kandev/config",
    kubeContext: "",
    namespace: "agents",
    requestTimeoutSeconds: "30",
  };

  it.each(["1e2", "1.0", "+30", " 30 "])("rejects non-canonical timeout %s", (timeout) => {
    expect(
      kubernetesExecutorInvalidReason(
        { ...valid, requestTimeoutSeconds: timeout },
        true,
        translate,
      ),
    ).toBe("translated:executors:kubernetesTimeoutInvalid");
  });

  it("rejects a relative kubeconfig path before save", () => {
    expect(
      kubernetesExecutorInvalidReason(
        { ...valid, kubeconfigPath: "configs/cluster.yaml" },
        true,
        translate,
      ),
    ).toBe("translated:executors:kubernetesKubeconfigPathRequired");
  });
});

describe("Kubernetes create save participation", () => {
  it("never marks a read-only member route dirty or saveable", () => {
    expect(getKubernetesCreateContributorState(false, "Administrator required")).toEqual({
      isDirty: false,
      canSave: false,
    });
  });

  it("lets an administrator participate when the form is valid", () => {
    expect(getKubernetesCreateContributorState(true, undefined)).toEqual({
      isDirty: true,
      canSave: true,
    });
    expect(getKubernetesCreateContributorState(true, "Invalid form").canSave).toBe(false);
  });
});

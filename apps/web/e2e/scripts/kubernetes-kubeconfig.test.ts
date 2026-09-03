import { describe, expect, it } from "vitest";
import { buildServiceAccountKubeconfig } from "../fixtures/kubernetes-kubeconfig";

describe("Kubernetes E2E kubeconfig generation", () => {
  it("uses the Kubernetes current-context wire key", () => {
    const document = buildServiceAccountKubeconfig(
      { server: "https://127.0.0.1:6443", "certificate-authority-data": "test-ca" },
      "kandev-host",
      "test-token",
    );

    expect(document).toMatchObject({
      apiVersion: "v1",
      kind: "Config",
      "current-context": "kind-kandev-host",
      contexts: [
        {
          name: "kind-kandev-host",
          context: { cluster: "kind-kandev-host", user: "kandev-host" },
        },
      ],
    });
    expect(document).not.toHaveProperty("currentContext");
  });
});

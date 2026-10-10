// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import { waitForKubernetesStorageReady } from "../fixtures/kubernetes-storage-readiness";

function fixture(failWait = false) {
  const commands: string[][] = [];
  const kubectl = vi.fn((args: string[], options?: { input?: string }) => {
    commands.push(args);
    if (args[0] === "create") {
      const manifest = JSON.parse(options!.input!);
      expect(manifest.items[0].kind).toBe("PersistentVolumeClaim");
      expect(manifest.items[1].spec.automountServiceAccountToken).toBe(false);
      expect(manifest.items[1].spec.containers[0].imagePullPolicy).toBe("Never");
      expect(manifest.items[1].spec.securityContext.fsGroup).toBe(1000);
    }
    if (args[0] === "wait" && failWait) throw new Error("storage unavailable");
    return "";
  });
  return { cluster: { kubectl, namespace: "fixture", image: "cached:image" }, commands };
}

describe("Kubernetes storage readiness", () => {
  it("requires writable storage before returning and removes both exact probe resources", () => {
    const { cluster, commands } = fixture();
    waitForKubernetesStorageReady(cluster);
    expect(commands.map((args) => args[0])).toEqual(["create", "wait", "exec", "delete", "delete"]);
    expect(commands[1]).toContain("--for=condition=Ready");
    expect(commands[2]).toContain("/workspace/.fixture-ready");
    expect(commands.slice(-2).every((args) => !args.includes("--all"))).toBe(true);
  });

  it("cleans up the claimed pod and PVC after a failed readiness wait", () => {
    const { cluster, commands } = fixture(true);
    expect(() => waitForKubernetesStorageReady(cluster)).toThrow("storage unavailable");
    expect(commands.map((args) => args[0])).toEqual(["create", "wait", "delete", "delete"]);
  });
});

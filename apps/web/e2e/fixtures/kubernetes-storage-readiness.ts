import { randomUUID } from "node:crypto";
import type { KubernetesCluster } from "./kubernetes-tools";
import { cleanupFailedFixture } from "./kubernetes-fixture-policy";

type StorageCluster = Pick<KubernetesCluster, "kubectl" | "namespace" | "image">;

function storageProbeManifest(cluster: StorageCluster, name: string) {
  const metadata = { name, namespace: cluster.namespace };
  return {
    apiVersion: "v1",
    kind: "List",
    items: [
      {
        apiVersion: "v1",
        kind: "PersistentVolumeClaim",
        metadata,
        spec: { accessModes: ["ReadWriteOnce"], resources: { requests: { storage: "1Gi" } } },
      },
      {
        apiVersion: "v1",
        kind: "Pod",
        metadata,
        spec: {
          restartPolicy: "Never",
          automountServiceAccountToken: false,
          securityContext: { runAsUser: 1000, runAsGroup: 1000, fsGroup: 1000 },
          containers: [
            {
              name: "storage-probe",
              image: cluster.image,
              imagePullPolicy: "Never",
              command: ["sh", "-ceu", "touch /workspace/.fixture-ready; exec sleep 3600"],
              readinessProbe: {
                exec: { command: ["test", "-f", "/workspace/.fixture-ready"] },
                periodSeconds: 1,
              },
              resources: {
                requests: { cpu: "50m", memory: "32Mi" },
                limits: { cpu: "250m", memory: "128Mi" },
              },
              volumeMounts: [{ name: "workspace", mountPath: "/workspace" }],
            },
          ],
          volumes: [{ name: "workspace", persistentVolumeClaim: { claimName: name } }],
        },
      },
    ],
  };
}

// Prove cold local-path provisioning before a task enters its normal request deadline.
export function waitForKubernetesStorageReady(cluster: StorageCluster): void {
  const name = `kandev-fixture-storage-${randomUUID()}`;
  const manifest = storageProbeManifest(cluster, name);
  const remove = (resource: string) =>
    cluster.kubectl([
      "delete",
      "-n",
      cluster.namespace,
      resource,
      name,
      "--ignore-not-found",
      "--wait=true",
      "--timeout=60s",
    ]);
  const cleanups = [() => remove("pod"), () => remove("persistentvolumeclaim")];
  try {
    cluster.kubectl(["create", "-f", "-"], { input: JSON.stringify(manifest) });
    cluster.kubectl(
      ["wait", "-n", cluster.namespace, "--for=condition=Ready", `pod/${name}`, "--timeout=120s"],
      { timeoutMs: 150_000 },
    );
    cluster.kubectl([
      "exec",
      "-n",
      cluster.namespace,
      name,
      "--",
      "test",
      "-f",
      "/workspace/.fixture-ready",
    ]);
  } catch (error) {
    cleanupFailedFixture(error, cleanups);
  }
  // Run both cleanups even if the first fails, preserving exact probe ownership.
  const failures: unknown[] = [];
  for (const cleanup of cleanups) {
    try {
      cleanup();
    } catch (error) {
      failures.push(error);
    }
  }
  if (failures.length) throw new AggregateError(failures, "Storage readiness probe cleanup failed");
}

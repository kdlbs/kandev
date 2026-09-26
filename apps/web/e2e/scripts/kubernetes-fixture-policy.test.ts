import fs from "node:fs";
import path from "node:path";
import { describe, expect, it, vi } from "vitest";
import {
  assertRuntimeImageTagAvailable,
  FixtureResourceOwnership,
  redactKubernetesDiagnosticText,
  WORKLOAD_RBAC_PROBES,
  WORKLOAD_RBAC_RULES,
} from "../fixtures/kubernetes-fixture-policy";
import { selectExactKubernetesResource } from "../helpers/kubernetes";

const REPOSITORY_ROOT = path.resolve(__dirname, "../../../..");

describe("Kubernetes E2E fixture safety policy", () => {
  it("grants only the published workload permissions", () => {
    expect(WORKLOAD_RBAC_RULES).toEqual([
      { apiGroups: [""], resources: ["pods"], verbs: ["get", "create", "delete", "watch"] },
      {
        apiGroups: [""],
        resources: ["pods/exec", "pods/portforward"],
        verbs: ["get", "create"],
      },
      {
        apiGroups: [""],
        resources: ["persistentvolumeclaims"],
        verbs: ["get", "create", "delete"],
      },
    ]);

    const serialized = JSON.stringify(WORKLOAD_RBAC_RULES);
    for (const forbidden of ["events", "list", "patch"]) {
      expect(serialized).not.toContain(`"${forbidden}"`);
    }
  });

  it("probes every allowed verb and rejects collection, mutation, and event extras", () => {
    const answer = (verb: string, resource: string) =>
      WORKLOAD_RBAC_PROBES.find((probe) => probe.verb === verb && probe.resource === resource)
        ?.allowed;

    for (const [verb, resource] of [
      ["get", "pods"],
      ["create", "pods"],
      ["delete", "pods"],
      ["watch", "pods"],
      ["get", "pods/exec"],
      ["create", "pods/exec"],
      ["get", "pods/portforward"],
      ["create", "pods/portforward"],
      ["get", "persistentvolumeclaims"],
      ["create", "persistentvolumeclaims"],
      ["delete", "persistentvolumeclaims"],
    ]) {
      expect(answer(verb!, resource!)).toBe(true);
    }
    for (const [verb, resource] of [
      ["list", "pods"],
      ["patch", "pods"],
      ["list", "persistentvolumeclaims"],
      ["watch", "persistentvolumeclaims"],
      ["patch", "persistentvolumeclaims"],
      ["get", "events"],
      ["list", "events"],
      ["create", "events"],
    ]) {
      expect(answer(verb!, resource!)).toBe(false);
    }
  });

  it("refuses a pre-existing exact runtime-image tag", () => {
    expect(() => assertRuntimeImageTagAvailable("fixture:image", false)).not.toThrow();
    expect(() => assertRuntimeImageTagAvailable("fixture:image", true)).toThrow(
      /refusing to overwrite existing Docker image fixture:image/,
    );
  });

  it("retains ownership when a creation callback partially creates then throws", () => {
    const ownership = new FixtureResourceOwnership();
    const failedCreate = vi.fn(() => {
      throw new Error("create failed");
    });

    expect(() => ownership.acquire("cluster", failedCreate)).toThrow("create failed");
    expect(ownership.owns("cluster")).toBe(true);

    ownership.release("cluster");
    expect(ownership.owns("cluster")).toBe(false);

    ownership.acquire("cluster", vi.fn());
    expect(ownership.owns("cluster")).toBe(true);
    ownership.release("cluster");
    expect(ownership.owns("cluster")).toBe(false);
  });

  it("marks the exact Kind name before creation and removes the marker after deletion", () => {
    const source = fs.readFileSync(
      path.join(REPOSITORY_ROOT, "apps/web/e2e/fixtures/kubernetes-tools.ts"),
      "utf8",
    );
    const preflight = source.indexOf("if (existingClusters.includes(name))");
    const writeMarker = source.indexOf("writeClusterOwnershipMarker(marker, name)", preflight);
    const reserveAndCreate = source.indexOf('ownership.acquire("cluster"', preflight);
    const deleteCluster = source.indexOf(
      'execFileSync(tools.kind, ["delete", "cluster", "--name", name]',
    );
    const removeMarker = source.indexOf(
      "removeClusterOwnershipMarker(marker, name)",
      deleteCluster,
    );
    const release = source.indexOf('ownership.release("cluster")', removeMarker);
    const cleanupAfterCreateFailure = source.indexOf(
      "try {\n      deleteCluster();",
      reserveAndCreate,
    );

    expect(preflight).toBeGreaterThan(-1);
    expect(writeMarker).toBeGreaterThan(preflight);
    expect(reserveAndCreate).toBeGreaterThan(writeMarker);
    expect(deleteCluster).toBeGreaterThan(-1);
    expect(removeMarker).toBeGreaterThan(deleteCluster);
    expect(release).toBeGreaterThan(removeMarker);
    expect(cleanupAfterCreateFailure).toBeGreaterThan(reserveAndCreate);
  });

  it("redacts every exact bearer-token occurrence from Kubernetes diagnostics", () => {
    const token = "eyJhbGciOiJSUzI1NiJ9.fixture.signature";
    const diagnostic = [
      `pod log token=${token}`,
      `previous log repeated ${token}`,
      `describe output value: ${token}`,
      "safe status text",
    ].join("\n");

    const redacted = redactKubernetesDiagnosticText(diagnostic, token);

    expect(redacted.includes(token)).toBe(false);
    expect(redacted.match(/\[REDACTED KUBERNETES CREDENTIAL\]/g)).toHaveLength(3);
    expect(redacted).toContain("safe status text");
  });

  it("rejects duplicate resources for one task and session identity", () => {
    const identity = { taskId: "task-1", sessionId: "session-1" };
    const resource = {
      metadata: {
        name: "pod-1",
        uid: "uid-1",
        labels: {
          "kandev.ai/task-id": identity.taskId,
          "kandev.ai/session-id": identity.sessionId,
        },
      },
    };

    expect(selectExactKubernetesResource([], identity, "Pod")).toBeUndefined();
    expect(selectExactKubernetesResource([resource], identity, "Pod")).toBe(resource);
    expect(() =>
      selectExactKubernetesResource(
        [resource, { ...resource, metadata: { ...resource.metadata, uid: "uid-2" } }],
        identity,
        "Pod",
      ),
    ).toThrow(/multiple Pods.*task-1.*session-1/i);
  });

  it("rejects a resource whose labels do not match the requested identity", () => {
    expect(() =>
      selectExactKubernetesResource(
        [
          {
            metadata: {
              name: "pod-1",
              uid: "uid-1",
              labels: {
                "kandev.ai/task-id": "task-1",
                "kandev.ai/session-id": "different-session",
              },
            },
          },
        ],
        { taskId: "task-1", sessionId: "session-1" },
        "Pod",
      ),
    ).toThrow(/unexpected identity/i);
  });
});

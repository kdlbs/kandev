import fs from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  KIND_NODE_IMAGE,
  KIND_SHA256_AMD64,
  KIND_VERSION,
  KUBECTL_SHA256_AMD64,
  KUBERNETES_FIXTURE_PINS,
  KUBERNETES_E2E_BASE_IMAGE,
  KUBERNETES_VERSION,
  resolveKubernetesFixturePin,
} from "../fixtures/kubernetes-pins";

const REPOSITORY_ROOT = path.resolve(__dirname, "../../../..");
const OLDEST_KUBERNETES_VERSION = "v1.34.8";
const OLDEST_KUBECTL_SHA256_AMD64 =
  "f6249132865c13abe3c9dd5038f5da65849cb86eee1608c001831504e481aa8c";
const OLDEST_KIND_NODE_IMAGE =
  "kindest/node:v1.34.8@sha256:02722c2dedddcfc00febf5d27fbeb9b7b2c14294c82109ff4a85d89ac9ba3256";
const CURRENT_KUBERNETES_VERSION = "v1.36.1";
const CURRENT_KUBECTL_SHA256_AMD64 =
  "629d3f410e09bf49b64ae7079f7f0bda1191efed311f7d37fdbab0ad5b0ec2b7";
const CURRENT_KIND_NODE_IMAGE =
  "kindest/node:v1.36.1@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5";
const KUBERNETES_E2E_RUNTIME_IMAGE =
  "ghcr.io/kdlbs/kandev-ci:runtime-sha-6f288a23c526@sha256:b9636e1c20adb0fcce1c65858a767fb1b0fe48a15ac5688b9c623163575efb8d";

describe("Kubernetes E2E version pins", () => {
  it("uses an immutable prebuilt runtime image for compatibility jobs", () => {
    expect(KUBERNETES_E2E_BASE_IMAGE).toMatch(
      /^ghcr\.io\/kdlbs\/kandev-ci:runtime-sha-[0-9a-f]{12}@sha256:[0-9a-f]{64}$/,
    );
  });

  it("uses the current supported Kubernetes release for the full lifecycle fixture", () => {
    expect(KUBERNETES_VERSION).toBe(CURRENT_KUBERNETES_VERSION);
    expect(KUBECTL_SHA256_AMD64).toBe(CURRENT_KUBECTL_SHA256_AMD64);
    expect(KIND_NODE_IMAGE).toBe(CURRENT_KIND_NODE_IMAGE);
  });

  it("pins the oldest and current compatibility releases as one supported matrix", () => {
    expect(KUBERNETES_FIXTURE_PINS).toEqual({
      [OLDEST_KUBERNETES_VERSION]: {
        kubectlSha256Amd64: OLDEST_KUBECTL_SHA256_AMD64,
        nodeImage: OLDEST_KIND_NODE_IMAGE,
      },
      [CURRENT_KUBERNETES_VERSION]: {
        kubectlSha256Amd64: CURRENT_KUBECTL_SHA256_AMD64,
        nodeImage: CURRENT_KIND_NODE_IMAGE,
      },
    });
    expect(resolveKubernetesFixturePin(OLDEST_KUBERNETES_VERSION)).toMatchObject({
      version: OLDEST_KUBERNETES_VERSION,
      kubectlSha256Amd64: OLDEST_KUBECTL_SHA256_AMD64,
      nodeImage: OLDEST_KIND_NODE_IMAGE,
    });
    expect(() => resolveKubernetesFixturePin("v1.33.0")).toThrow(/unsupported/);
  });

  it("uses a prebuilt immutable runtime image for Kubernetes fixture containers", () => {
    const tools = fs.readFileSync(
      path.join(REPOSITORY_ROOT, "apps/web/e2e/fixtures/kubernetes-tools.ts"),
      "utf8",
    );

    expect(KUBERNETES_E2E_BASE_IMAGE).toBe(KUBERNETES_E2E_RUNTIME_IMAGE);
    expect(tools).toContain("FROM ${KUBERNETES_E2E_BASE_IMAGE}");
    expect(tools).not.toContain("RUN apt-get update");
  });

  it("keeps CI tool installation aligned with the fixture", () => {
    const workflow = fs.readFileSync(
      path.join(REPOSITORY_ROOT, ".github/workflows/e2e-tests.yml"),
      "utf8",
    );

    for (const pin of [
      KIND_VERSION,
      KIND_SHA256_AMD64,
      KUBERNETES_VERSION,
      KUBECTL_SHA256_AMD64,
      KIND_NODE_IMAGE,
      KUBERNETES_E2E_BASE_IMAGE,
    ]) {
      expect(workflow).toContain(pin);
    }
  });

  it("documents the fixture's public version and node-image pins", () => {
    const readme = fs.readFileSync(path.join(REPOSITORY_ROOT, "apps/web/e2e/README.md"), "utf8");

    expect(readme).toContain(KIND_VERSION);
    expect(readme).toContain(KUBERNETES_VERSION);
    expect(readme).toContain(KIND_NODE_IMAGE);
    expect(readme).toContain(OLDEST_KUBERNETES_VERSION);
    expect(readme).toContain(OLDEST_KIND_NODE_IMAGE);
  });

  it("wires a separate two-version API compatibility project without changing containers", () => {
    const config = fs.readFileSync(
      path.join(REPOSITORY_ROOT, "apps/web/e2e/playwright.config.ts"),
      "utf8",
    );
    const workflow = fs.readFileSync(
      path.join(REPOSITORY_ROOT, ".github/workflows/e2e-tests.yml"),
      "utf8",
    );

    expect(config).toContain('name: "kubernetes-compat"');
    expect(config).toMatch(/name: "kubernetes-compat"[\s\S]*kubernetes-compat/);
    expect(config).toMatch(/name: "chromium"[\s\S]*kubernetes-compat/);
    expect(workflow).toMatch(/^ {2}e2e-kubernetes-compatibility:/m);
    expect(workflow).toContain("--project=kubernetes-compat");
    expect(workflow).toContain("KANDEV_E2E_KUBERNETES_VERSION");
    expect(workflow).toContain("KANDEV_E2E_BUILD_IDENTITY");
    expect(workflow).toContain("KANDEV_E2E_EXPECTED_SOURCE_REVISION");
    expect(workflow).toContain("Pull pinned Kubernetes fixture base image");
    expect(workflow).toMatch(/for attempt in 1 2 3; do[\s\S]*docker pull/);
    expect(workflow).not.toContain("KANDEV_E2E_SKIP_FRESHNESS");
    for (const pin of [
      OLDEST_KUBERNETES_VERSION,
      OLDEST_KUBECTL_SHA256_AMD64,
      OLDEST_KIND_NODE_IMAGE,
      CURRENT_KUBERNETES_VERSION,
      CURRENT_KUBECTL_SHA256_AMD64,
      CURRENT_KIND_NODE_IMAGE,
    ]) {
      expect(workflow).toContain(pin);
    }
  });

  it("guards CI teardown with an exact-name ownership marker", () => {
    const workflow = fs.readFileSync(
      path.join(REPOSITORY_ROOT, ".github/workflows/e2e-tests.yml"),
      "utf8",
    );

    expect(workflow).toContain("KANDEV_E2E_KIND_OWNERSHIP_MARKER");
    expect(workflow).toContain("${KANDEV_E2E_KIND_BIN:-}");
    expect(workflow).toContain("${KANDEV_E2E_KIND_CLUSTER_NAME:-}");
    expect(workflow).toContain("${KANDEV_E2E_KIND_OWNERSHIP_MARKER:-}");
    expect(workflow).toMatch(/if \[\[ ! -f "\$ownership_marker" \]\]; then/);
    expect(workflow).toMatch(/owned_name=.*ownership_marker/);
    expect(workflow).toContain('[[ "$owned_name" == "$cluster_name" ]]');
  });
});

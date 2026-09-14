import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import type { KubernetesCluster } from "./kubernetes-tools";

const REPOSITORY_ROOT = path.resolve(__dirname, "../../../..");
const IMAGE_DIR = path.join(REPOSITORY_ROOT, "k8s", "worker-images");
const DOCKERFILE = path.join(IMAGE_DIR, "Dockerfile");
const PIN_FILE = path.join(IMAGE_DIR, "pins.env");
const MOCK_AGENT = path.join(REPOSITORY_ROOT, "apps/backend/bin/mock-agent-linux-amd64");
const TARGETS = ["minimal", "node-pnpm", "python"] as const;
const PRESET_FILES: Record<KubernetesWorkerTarget, string> = {
  minimal: "minimal.yaml",
  "node-pnpm": "node-pnpm.yaml",
  python: "python.yaml",
};

export type KubernetesWorkerTarget = (typeof TARGETS)[number];

export type KubernetesWorkerImages = {
  minimal: string;
  nodePnpm: string;
  python: string;
  dispose: () => Promise<void>;
};

export function kubernetesWorkerPreset(
  target: KubernetesWorkerTarget,
  image: string,
  imagePullPolicy: "Always" | "IfNotPresent" | "Never" = "Never",
): string {
  const presetPath = path.join(REPOSITORY_ROOT, "k8s", "presets", PRESET_FILES[target]);
  let template = fs.readFileSync(presetPath, "utf8");
  template = replacePresetField(template, "image", image, presetPath);
  return replacePresetField(template, "imagePullPolicy", imagePullPolicy, presetPath);
}

function replacePresetField(
  template: string,
  field: string,
  value: string,
  presetPath: string,
): string {
  const pattern = new RegExp(`^(\\s+${field}:\\s+).+$`, "gm");
  const matches = template.match(pattern) ?? [];
  if (matches.length !== 1) {
    throw new Error(`worker preset must contain exactly one ${field} field: ${presetPath}`);
  }
  return template.replace(pattern, (_match: string, prefix: string) => `${prefix}${value}`);
}

type BuiltImage = {
  tag: string;
  target: KubernetesWorkerTarget;
  kind: "recipe" | "harness";
};

export async function buildKubernetesWorkerImages(
  cluster: KubernetesCluster,
): Promise<KubernetesWorkerImages> {
  const pins = readPins();
  const suffix = `${cluster.name}-${process.pid}-${randomUUID().slice(0, 8)}`;
  const built: BuiltImage[] = [];

  try {
    for (const target of TARGETS) {
      const recipeTag = `kandev-worker-e2e:${suffix}-${target}`;
      const harnessTag = `kandev-worker-e2e:${suffix}-${target}-harness`;
      built.push(
        { tag: recipeTag, target, kind: "recipe" },
        { tag: harnessTag, target, kind: "harness" },
      );
      buildImage(target, recipeTag, pins.baseImage, pins.pnpmVersion);
      loadImage(cluster, recipeTag);
      buildHarnessImage(recipeTag, harnessTag);
      loadImage(cluster, harnessTag);
    }
  } catch (error) {
    await disposeImages(built);
    throw error;
  }

  let disposed = false;
  return {
    minimal: imageFor(built, "minimal"),
    nodePnpm: imageFor(built, "node-pnpm"),
    python: imageFor(built, "python"),
    dispose: async () => {
      if (disposed) return;
      disposed = true;
      await disposeImages(built);
    },
  };
}

function readPins(): { baseImage: string; pnpmVersion: string } {
  const values = new Map<string, string>();
  for (const line of fs.readFileSync(PIN_FILE, "utf8").split(/\r?\n/)) {
    const match = line.match(/^([A-Z_]+)=(.+)$/);
    if (match) values.set(match[1]!, match[2]!);
  }
  const baseImage = values.get("BASE_IMAGE");
  const pnpmVersion = values.get("PNPM_VERSION");
  if (!baseImage || !/@sha256:[0-9a-f]{64}$/.test(baseImage)) {
    throw new Error(`worker image pins must contain an immutable BASE_IMAGE: ${PIN_FILE}`);
  }
  if (!pnpmVersion || !/^\d+\.\d+\.\d+$/.test(pnpmVersion)) {
    throw new Error(`worker image pins must contain a semantic PNPM_VERSION: ${PIN_FILE}`);
  }
  return { baseImage, pnpmVersion };
}

function buildImage(
  target: KubernetesWorkerTarget,
  tag: string,
  baseImage: string,
  pnpmVersion: string,
): void {
  execFileSync(
    "docker",
    [
      "build",
      "--platform",
      "linux/amd64",
      "--build-arg",
      `BASE_IMAGE=${baseImage}`,
      "--build-arg",
      `PNPM_VERSION=${pnpmVersion}`,
      "--target",
      target,
      "--tag",
      tag,
      "--file",
      DOCKERFILE,
      REPOSITORY_ROOT,
    ],
    {
      timeout: 600_000,
      stdio: process.env.E2E_DEBUG ? "inherit" : ["ignore", "ignore", "inherit"],
    },
  );
}

function buildHarnessImage(baseTag: string, tag: string): void {
  if (!fs.existsSync(MOCK_AGENT)) {
    throw new Error(`Kubernetes E2E mock-agent binary is missing: ${MOCK_AGENT}`);
  }
  const context = fs.mkdtempSync(path.join(path.dirname(MOCK_AGENT), "kubernetes-worker-harness-"));
  const dockerfile = path.join(context, "Dockerfile");
  fs.copyFileSync(MOCK_AGENT, path.join(context, "mock-agent-linux-amd64"));
  fs.writeFileSync(
    dockerfile,
    `FROM ${baseTag}
USER root
COPY mock-agent-linux-amd64 /usr/local/bin/mock-agent
RUN chmod 0755 /usr/local/bin/mock-agent
USER 1000
`,
  );
  try {
    execFileSync(
      "docker",
      ["build", "--platform", "linux/amd64", "--tag", tag, "--file", dockerfile, context],
      {
        timeout: 600_000,
        stdio: process.env.E2E_DEBUG ? "inherit" : ["ignore", "ignore", "inherit"],
      },
    );
  } finally {
    fs.rmSync(context, { recursive: true, force: true });
  }
}

function loadImage(cluster: KubernetesCluster, tag: string): void {
  execFileSync(cluster.kindBin, ["load", "docker-image", tag, "--name", cluster.name], {
    timeout: 300_000,
    stdio: process.env.E2E_DEBUG ? "inherit" : ["ignore", "ignore", "inherit"],
  });
}

function imageFor(built: BuiltImage[], target: KubernetesWorkerTarget): string {
  const image = built.find((entry) => entry.target === target && entry.kind === "harness")?.tag;
  if (!image) throw new Error(`worker image target was not built: ${target}`);
  return image;
}

async function disposeImages(images: BuiltImage[]): Promise<void> {
  for (const { tag } of images) {
    try {
      execFileSync("docker", ["image", "rm", "--force", tag], {
        timeout: 60_000,
        stdio: "ignore",
      });
    } catch {
      // The exact tag may already be gone after a failed build or Kind load.
    }
  }
}

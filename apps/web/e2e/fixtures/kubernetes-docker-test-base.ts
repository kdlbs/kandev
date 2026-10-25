import { execFileSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { kubernetesTest } from "./kubernetes-test-base";

const ROOT = path.resolve(__dirname, "../../../..");
const RECIPE = path.join(ROOT, "k8s/worker-images/full");

export function fullWorkerPrepare(): string {
  return fs.readFileSync(path.join(RECIPE, "prepare.sh"), "utf8");
}

export function fullWorkerTemplate(image: string, unavailable = false): string {
  let template = fs
    .readFileSync(path.join(RECIPE, "pod-template.yaml"), "utf8")
    .replace("FULL_WORKER_IMAGE_REQUIRED", image)
    .replace("imagePullPolicy: IfNotPresent", "imagePullPolicy: Never");
  if (unavailable) template = template.replace(/exec dockerd[^\n]+/, "exec sleep 3600");
  return template;
}

export const test = kubernetesTest.extend<object, { fullWorkerImage: string }>({
  fullWorkerImage: [
    async ({ cluster }, use) => {
      const image = process.env.KANDEV_E2E_FULL_WORKER_IMAGE ?? "";
      if (!/^sha256:[a-f0-9]{64}$/.test(image))
        throw new Error("KANDEV_E2E_FULL_WORKER_IMAGE must be the verified local image ID");
      const node = `${cluster.name}-control-plane`;
      execFileSync("docker", ["update", "--cpus=2", "--memory=8g", "--memory-swap=8g", node], {
        timeout: 30_000,
      });
      const limits = execFileSync(
        "docker",
        ["inspect", "--format", "{{.HostConfig.NanoCpus}} {{.HostConfig.Memory}}", node],
        { encoding: "utf8", timeout: 30_000 },
      ).trim();
      if (limits !== "2000000000 8589934592")
        throw new Error("Kind workload resource limits were not applied");
      const suffix = randomUUID();
      const container = `kandev-full-harness-${suffix}`;
      const tag = `kandev-full-harness:${suffix}`;
      let created = false;
      try {
        // Copy the mock transport into a never-started container; no tool rebuild
        // or mutable image input is needed for this fixture-only layer.
        execFileSync(
          "docker",
          ["create", "--name", container, "--network=none", "--memory=128m", "--cpus=1", image],
          { timeout: 30_000 },
        );
        created = true;
        execFileSync(
          "docker",
          [
            "cp",
            path.join(ROOT, "apps/backend/bin/mock-agent-linux-amd64"),
            `${container}:/usr/local/bin/mock-agent`,
          ],
          { timeout: 30_000 },
        );
        execFileSync("docker", ["commit", container, tag], { timeout: 60_000 });
        execFileSync(cluster.kindBin, ["load", "docker-image", tag, "--name", cluster.name], {
          timeout: 300_000,
        });
        await use(tag);
      } finally {
        if (created) execFileSync("docker", ["rm", container], { timeout: 30_000 });
        const found = execFileSync("docker", ["image", "ls", "--quiet", tag], {
          encoding: "utf8",
          timeout: 30_000,
        }).trim();
        if (found) execFileSync("docker", ["image", "rm", tag], { timeout: 60_000 });
      }
    },
    { scope: "worker", timeout: 480_000 },
  ],
});
export { expect } from "@playwright/test";

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { createRequire } from "node:module";
import { afterEach, beforeEach, expect, it } from "vitest";

let root: string;
let binDir: string;
let commandLog: string;
const cli = createRequire(import.meta.url).resolve("tsx/cli");

beforeEach(() => {
  root = fs.mkdtempSync(path.join(os.tmpdir(), "worker-image-contract-"));
  binDir = path.join(root, "apps/backend/bin");
  commandLog = path.join(root, "commands.jsonl");
  const fixtureDir = path.join(root, "apps/web/e2e/fixtures");
  const imageDir = path.join(root, "k8s/worker-images");
  for (const directory of [binDir, fixtureDir, imageDir, path.join(root, "tools")]) {
    fs.mkdirSync(directory, { recursive: true });
  }
  fs.copyFileSync(
    path.resolve("e2e/fixtures/kubernetes-worker-images.ts"),
    path.join(fixtureDir, "kubernetes-worker-images.ts"),
  );
  for (const file of ["pins.env", "Dockerfile"]) {
    fs.copyFileSync(path.resolve("../../k8s/worker-images", file), path.join(imageDir, file));
  }
  fs.writeFileSync(path.join(binDir, "mock-agent-linux-amd64"), "current mock-agent fixture");
  const command = `#!${process.execPath}
const fs = require("node:fs"), path = require("node:path");
const command = path.basename(process.argv[1]), args = process.argv.slice(2);
fs.appendFileSync(process.env.IMAGE_COMMAND_LOG, JSON.stringify({ command, args }) + "\\n");
if (process.env.FAIL_IMAGE_TRANSFER === "1" && command === "kind" && args.some(arg => arg.endsWith("node-pnpm-harness"))) process.exit(1);
`;
  for (const name of ["docker", "kind"]) {
    fs.writeFileSync(path.join(root, "tools", name), command, { mode: 0o755 });
  }
  fs.writeFileSync(
    path.join(root, "run.ts"),
    `
import { buildKubernetesWorkerImages } from "./apps/web/e2e/fixtures/kubernetes-worker-images";
async function run() {
  const images = await buildKubernetesWorkerImages({ name: "owned-cluster", kindBin: ${JSON.stringify(path.join(root, "tools/kind"))} } as any);
  await images.dispose();
  await images.dispose();
  console.log(JSON.stringify(images));
}
run().catch(error => { console.error(error.message); process.exitCode = 1; });
`,
  );
});

afterEach(() => fs.rmSync(root, { recursive: true, force: true }));

type Command = { command: string; args: string[] };
function commands(): Command[] {
  return fs
    .readFileSync(commandLog, "utf8")
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line) as Command);
}
function run(fail = false): string {
  return execFileSync(process.execPath, [cli, path.join(root, "run.ts")], {
    encoding: "utf8",
    stdio: "pipe",
    env: {
      ...process.env,
      PATH: `${path.join(root, "tools")}${path.delimiter}${process.env.PATH}`,
      IMAGE_COMMAND_LOG: commandLog,
      FAIL_IMAGE_TRANSFER: fail ? "1" : "0",
    },
  });
}

it("builds all recipes and loads only final images referenced by worker presets", () => {
  const images = JSON.parse(run()) as { minimal: string; nodePnpm: string; python: string };
  const all = commands();
  const built = all.filter(({ command, args }) => command === "docker" && args[0] === "build");
  expect(built).toHaveLength(6);
  expect(
    built
      .filter(({ args }) => args.includes("--target"))
      .map(({ args }) => args[args.indexOf("--target") + 1]),
  ).toEqual(["minimal", "node-pnpm", "python"]);
  const loaded = all.filter(({ command }) => command === "kind");
  expect(loaded).toHaveLength(1);
  expect(loaded[0]!.args).toEqual([
    "load",
    "docker-image",
    images.minimal,
    images.nodePnpm,
    images.python,
    "--name",
    "owned-cluster",
  ]);
  expect(all.indexOf(loaded[0]!)).toBeGreaterThan(all.indexOf(built.at(-1)!));
  const removed = all.filter(({ args }) => args[0] === "image" && args[1] === "rm");
  expect(removed).toHaveLength(6);
  expect(new Set(removed.map(({ args }) => args[3]))).toEqual(
    new Set(built.map(({ args }) => args[args.indexOf("--tag") + 1])),
  );
  expect(fs.readdirSync(binDir)).toEqual(["mock-agent-linux-amd64"]);
});

it("cleans owned images and propagates a failed final transfer", () => {
  expect(() => run(true)).toThrow();
  const all = commands();
  const built = all.filter(({ command, args }) => command === "docker" && args[0] === "build");
  const removed = all.filter(({ args }) => args[0] === "image" && args[1] === "rm");
  expect(built).toHaveLength(6);
  expect(removed).toHaveLength(6);
  expect(new Set(removed.map(({ args }) => args[3]))).toEqual(
    new Set(built.map(({ args }) => args[args.indexOf("--tag") + 1])),
  );
  expect(fs.readdirSync(binDir)).toEqual(["mock-agent-linux-amd64"]);
});

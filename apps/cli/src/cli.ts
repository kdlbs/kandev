import path from "node:path";
import fs from "node:fs";

import pkg from "../package.json";
import { runDev } from "./dev";
import { runRelease } from "./run";
import { ensureValidPort } from "./ports";
import { maybePromptForUpdate } from "./update";

type Command = "run" | "dev";

type CliOptions = {
  command: Command;
  version?: string;
  backendPort?: number;
  webPort?: number;
};

function printHelp() {
  console.log(`kandev launcher

Usage:
  kandev run [--version <tag>] [--backend-port <port>] [--web-port <port>]
  kandev dev [--backend-port <port>] [--web-port <port>]
  kandev [--version <tag>] [--backend-port <port>] [--web-port <port>]
  kandev --dev [--backend-port <port>] [--web-port <port>]

Examples:
  kandev
  kandev run
  kandev --dev
  kandev dev
  kandev --version v0.1.0
  kandev --backend-port 18080 --web-port 13000

Options:
  dev             Use local repo for dev (make dev + next dev) if available.
  run             Use release bundles (default).
  --dev            Alias for "dev".
  --version        Release tag to install (default: latest).
  --backend-port   Override backend port.
  --web-port       Override web port.
  --help, -h       Show help.
`);
}

function parseArgs(argv: string[]): CliOptions {
  const opts: CliOptions = { command: "run" };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--help" || arg === "-h") {
      printHelp();
      process.exit(0);
    }
    if (arg === "dev" || arg === "run") {
      opts.command = arg;
      continue;
    }
    if (arg === "--version") {
      opts.version = argv[i + 1];
      i += 1;
      continue;
    }
    if (arg.startsWith("--version=")) {
      opts.version = arg.split("=")[1];
      continue;
    }
    if (arg === "--dev") {
      opts.command = "dev";
      continue;
    }
    if (arg === "--backend-port") {
      opts.backendPort = Number(argv[i + 1]);
      i += 1;
      continue;
    }
    if (arg.startsWith("--backend-port=")) {
      opts.backendPort = Number(arg.split("=")[1]);
      continue;
    }
    if (arg === "--web-port") {
      opts.webPort = Number(argv[i + 1]);
      i += 1;
      continue;
    }
    if (arg.startsWith("--web-port=")) {
      opts.webPort = Number(arg.split("=")[1]);
      continue;
    }
  }
  return opts;
}

function findRepoRoot(startDir: string): string | null {
  let current = path.resolve(startDir);
  while (true) {
    const backendDir = path.join(current, "apps", "backend");
    const webDir = path.join(current, "apps", "web");
    if (fs.existsSync(backendDir) && fs.existsSync(webDir)) {
      return current;
    }
    const parent = path.dirname(current);
    if (parent === current) {
      return null;
    }
    current = parent;
  }
}

async function main(): Promise<void> {
  const raw = parseArgs(process.argv.slice(2));
  const backendPort = ensureValidPort(raw.backendPort, "backend port");
  const webPort = ensureValidPort(raw.webPort, "web port");

  if (raw.command === "dev") {
    const repoRoot = findRepoRoot(process.cwd());
    if (!repoRoot) {
      throw new Error("Unable to locate repo root for dev. Run from the repo.");
    }
    await runDev({ repoRoot, backendPort, webPort });
    return;
  }

  await maybePromptForUpdate(pkg.version, process.argv.slice(2));
  await runRelease({ version: raw.version, backendPort, webPort });
}

main().catch((err) => {
  console.error(`[kandev] ${err instanceof Error ? err.message : String(err)}`);
  process.exit(1);
});

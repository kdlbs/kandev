import path from "node:path";
import fs from "node:fs";

import pkg from "../package.json";
import { parseArgs, resolvePorts } from "./args";
import { runDev } from "./dev";
import { runRelease } from "./run";
import { runStart } from "./start";
import { ensureValidPort } from "./ports";
import { maybePromptForUpdate } from "./update";

function printHelp() {
  console.log(`kandev launcher

Usage:
  kandev run [--version <tag>] [--port <port>] [--verbose] [--debug]
  kandev dev [--port <port>]
  kandev start [--port <port>] [--verbose] [--debug]
  kandev [--version <tag>] [--port <port>] [--verbose] [--debug]
  kandev --dev [--port <port>]

Examples:
  kandev
  kandev run
  kandev --dev
  kandev dev
  kandev start
  kandev --version v0.1.0
  kandev --port 3000
  kandev --debug

Options:
  dev              Use local repo for dev (make dev + next dev) if available.
  start            Use local production build (make build + next start).
  run              Use release bundles (default).
  --dev            Alias for "dev".
  --version        Release tag to install (default: latest).
  --port           Port to open kandev on (or KANDEV_PORT env var). In start/run
                   this is the backend port (front door); in dev it is the Next
                   dev server port.
  --verbose, -v    Show info logs from backend + web.
  --debug          Show debug logs + agent message dumps.
  --help, -h       Show help.

Advanced:
  --backend-port       Deprecated alias for --port (start/run). Sets the Go
                       backend port directly. Also reads KANDEV_BACKEND_PORT.
  --web-internal-port  Override the internal Next.js port (start/run only). The
                       Go backend reverse-proxies to it; users never hit it
                       directly. Also reads KANDEV_WEB_PORT.
  --web-port           Deprecated alias for --web-internal-port. Misleading name
                       because the public URL is on the backend port.
`);
}

function findRepoRoot(startDir: string): string | null {
  let current = path.resolve(startDir);
  while (true) {
    if (current.endsWith(`${path.sep}apps`)) {
      const backendInApps = path.join(current, "backend");
      const webInApps = path.join(current, "web");
      if (fs.existsSync(backendInApps) && fs.existsSync(webInApps)) {
        return path.dirname(current);
      }
    }
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
  const { options, showHelp } = parseArgs(process.argv.slice(2));
  if (showHelp) {
    printHelp();
    return;
  }

  const resolved = resolvePorts(options, process.env);
  const backendPort = ensureValidPort(resolved.backendPort, "backend port");
  const webPort = ensureValidPort(resolved.webPort, "web port");

  if (options.command === "dev") {
    const repoRoot = findRepoRoot(process.cwd());
    if (!repoRoot) {
      throw new Error("Unable to locate repo root for dev. Run from the repo.");
    }
    await runDev({ repoRoot, backendPort, webPort });
    return;
  }

  if (options.command === "start") {
    const repoRoot = findRepoRoot(process.cwd());
    if (!repoRoot) {
      throw new Error("Unable to locate repo root for start. Run from the repo.");
    }
    await runStart({
      repoRoot,
      backendPort,
      webPort,
      verbose: options.verbose,
      debug: options.debug,
    });
    return;
  }

  await maybePromptForUpdate(pkg.version, process.argv.slice(2));
  await runRelease({
    version: options.version,
    backendPort,
    webPort,
    verbose: options.verbose,
    debug: options.debug,
  });
}

main().catch((err) => {
  console.error(`[kandev] ${err instanceof Error ? err.message : String(err)}`);
  process.exit(1);
});

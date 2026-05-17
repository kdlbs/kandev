import path from "node:path";
import fs from "node:fs";

import pkg from "../package.json";
import { parseArgs, ParseError, resolvePorts } from "./args";
import { runDev } from "./dev";
import { runRelease } from "./run";
import { runServiceCommand } from "./service";
import { warnIfStaleServiceUnit } from "./service/stale_check";
import { runStart } from "./start";
import { ensureValidPort } from "./ports";

function printHelp() {
  console.log(`kandev launcher

Usage:
  kandev run [--port <port>] [--verbose] [--debug]
  kandev dev [--port <port>]
  kandev start [--port <port>] [--verbose] [--debug]
  kandev [--port <port>] [--verbose] [--debug]
  kandev --dev [--port <port>]
  kandev service install|uninstall|start|stop|restart|status|logs [--system]

Examples:
  kandev
  kandev run
  kandev --dev
  kandev dev
  kandev start
  kandev --version
  kandev --port 3000
  kandev --debug

Options:
  dev              Use local repo for dev (make dev + next dev) if available.
  start            Use local production build (make build + next start).
  run              Use installed runtime bundle (default).
  service          Manage kandev as an OS service (systemd / launchd).
                   Run 'kandev service --help' for details.
  --dev            Alias for "dev".
  --version, -V    Print CLI version and exit.
  --port           Port for the Go backend (the URL kandev opens on in
                   start/run). Alias for --backend-port. Also reads
                   KANDEV_PORT or KANDEV_BACKEND_PORT.
  --verbose, -v    Show info logs from backend + web.
  --debug          Show debug logs + agent message dumps.
  --headless       Skip opening the browser. Used by service units.
  --help, -h       Show help.

Advanced:
  --backend-port         Same as --port.
  --web-internal-port    Override the internal Next.js port. The Go backend
                         reverse-proxies to it; users hit the backend port.
                         Also reads KANDEV_WEB_PORT.
  --web-port             Deprecated alias for --web-internal-port.
  --runtime-version <tag>  Download and use a specific release tag instead of
                           the installed runtime. For debugging only.
                           Example: kandev --runtime-version v0.16.0
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
  // The `service` subcommand has its own argv parser (own subcommands, flags).
  // Handle it before the top-level flag parser to keep the two parsers isolated.
  if (process.argv[2] === "service") {
    await runServiceCommand(process.argv.slice(3));
    return;
  }

  const { options, showHelp, deprecatedFlags } = parseArgs(process.argv.slice(2));

  if (options.showVersion) {
    console.log(pkg.version);
    return;
  }

  if (showHelp) {
    printHelp();
    return;
  }

  for (const flag of deprecatedFlags) {
    process.stderr.write(`[kandev] ${flag} is deprecated; use --web-internal-port\n`);
  }

  const resolved = resolvePorts(options, process.env);
  const backendPort = ensureValidPort(resolved.backendPort, "backend port");
  const webPort = ensureValidPort(resolved.webPort, "web port");

  // After an npm/brew upgrade, paths baked into an installed service unit may
  // be stale. Warn once before launch so the user knows to re-run install.
  // Skip when invoked from a service unit (--headless) — the warning would
  // land in journalctl/launchd logs and pile up on every restart.
  if (!options.headless) {
    warnIfStaleServiceUnit();
  }

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

  await runRelease({
    runtimeVersion: options.runtimeVersion,
    backendPort,
    webPort,
    verbose: options.verbose,
    debug: options.debug,
    headless: options.headless,
  });
}

main().catch((err) => {
  if (err instanceof ParseError) {
    console.error(`[kandev] ${err.message}`);
    console.error("[kandev] run --help for usage");
    process.exit(2);
  }
  console.error(`[kandev] ${err instanceof Error ? err.message : String(err)}`);
  process.exit(1);
});

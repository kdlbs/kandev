/**
 * Pure CLI argument parsing + port resolution.
 *
 * Kept free of side effects (other than a one-shot stderr deprecation note)
 * so it can be unit-tested without spawning the launcher.
 */

export type Command = "run" | "dev" | "start";

export type CliOptions = {
  command: Command;
  version?: string;
  port?: number;
  backendPort?: number;
  webPort?: number;
  verbose?: boolean;
  debug?: boolean;
};

export type ParseResult = {
  options: CliOptions;
  showHelp: boolean;
};

export function parseArgs(argv: string[]): ParseResult {
  const opts: CliOptions = { command: "run" };
  let showHelp = false;
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--help" || arg === "-h") {
      showHelp = true;
      continue;
    }
    if (arg === "dev" || arg === "run" || arg === "start") {
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
    if (arg === "--port") {
      opts.port = Number(argv[i + 1]);
      i += 1;
      continue;
    }
    if (arg.startsWith("--port=")) {
      opts.port = Number(arg.split("=")[1]);
      continue;
    }
    if (arg === "--backend-port") {
      opts.backendPort = Number(argv[i + 1]);
      warnDeprecatedFlag("--backend-port", "--port");
      i += 1;
      continue;
    }
    if (arg.startsWith("--backend-port=")) {
      opts.backendPort = Number(arg.split("=")[1]);
      warnDeprecatedFlag("--backend-port", "--port");
      continue;
    }
    if (arg === "--web-internal-port") {
      opts.webPort = Number(argv[i + 1]);
      i += 1;
      continue;
    }
    if (arg.startsWith("--web-internal-port=")) {
      opts.webPort = Number(arg.split("=")[1]);
      continue;
    }
    if (arg === "--web-port") {
      opts.webPort = Number(argv[i + 1]);
      warnDeprecatedFlag("--web-port", "--web-internal-port");
      i += 1;
      continue;
    }
    if (arg.startsWith("--web-port=")) {
      opts.webPort = Number(arg.split("=")[1]);
      warnDeprecatedFlag("--web-port", "--web-internal-port");
      continue;
    }
    if (arg === "--verbose" || arg === "-v") {
      opts.verbose = true;
      continue;
    }
    if (arg === "--debug") {
      opts.debug = true;
      continue;
    }
  }
  return { options: opts, showHelp };
}

export type ResolvedPorts = {
  backendPort?: number;
  webPort?: number;
};

/**
 * Resolves backend/web ports from CLI options + env, applying the rule that
 * `--port` (and KANDEV_PORT) maps to whichever process is the public front
 * door for the chosen command:
 *
 * - start / run → backend port (the Go server reverse-proxies Next.js)
 * - dev         → web port (browser hits Next dev directly for HMR)
 *
 * Explicit `--backend-port` / `--web-port` (and their *_PORT env vars)
 * always take precedence over the generic `--port`.
 */
export function resolvePorts(options: CliOptions, env: NodeJS.ProcessEnv): ResolvedPorts {
  const publicPort = options.port ?? envPort(env, "KANDEV_PORT");
  let backendPort = options.backendPort ?? envPort(env, "KANDEV_BACKEND_PORT");
  let webPort = options.webPort ?? envPort(env, "KANDEV_WEB_PORT");
  if (publicPort !== undefined) {
    if (options.command === "dev") {
      webPort = webPort ?? publicPort;
    } else {
      backendPort = backendPort ?? publicPort;
    }
  }
  return { backendPort, webPort };
}

function envPort(env: NodeJS.ProcessEnv, name: string): number | undefined {
  const val = env[name];
  return val ? Number(val) : undefined;
}

const warnedFlags = new Set<string>();
function warnDeprecatedFlag(oldFlag: string, newFlag: string): void {
  if (warnedFlags.has(oldFlag)) return;
  warnedFlags.add(oldFlag);
  process.stderr.write(`[kandev] ${oldFlag} is deprecated; use ${newFlag}\n`);
}

// Test-only: resets the dedup set between cases.
export function _resetWarnedFlagsForTest(): void {
  warnedFlags.clear();
}

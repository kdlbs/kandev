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
  /** Deprecated flags seen on the command line. cli.ts emits warnings after parsing so they can be command-aware. */
  deprecatedFlags: string[];
};

export class ParseError extends Error {}

export function parseArgs(argv: string[]): ParseResult {
  const opts: CliOptions = { command: "run" };
  let showHelp = false;
  const deprecatedFlags: string[] = [];
  const noteDeprecated = (flag: string) => {
    if (!deprecatedFlags.includes(flag)) deprecatedFlags.push(flag);
  };
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
      opts.version = takeValue(argv, i, "--version");
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
      opts.port = parsePort(takeValue(argv, i, "--port"), "--port");
      i += 1;
      continue;
    }
    if (arg.startsWith("--port=")) {
      opts.port = parsePort(arg.split("=")[1], "--port");
      continue;
    }
    if (arg === "--backend-port") {
      opts.backendPort = parsePort(takeValue(argv, i, "--backend-port"), "--backend-port");
      noteDeprecated("--backend-port");
      i += 1;
      continue;
    }
    if (arg.startsWith("--backend-port=")) {
      opts.backendPort = parsePort(arg.split("=")[1], "--backend-port");
      noteDeprecated("--backend-port");
      continue;
    }
    if (arg === "--web-internal-port") {
      opts.webPort = parsePort(takeValue(argv, i, "--web-internal-port"), "--web-internal-port");
      i += 1;
      continue;
    }
    if (arg.startsWith("--web-internal-port=")) {
      opts.webPort = parsePort(arg.split("=")[1], "--web-internal-port");
      continue;
    }
    if (arg === "--web-port") {
      opts.webPort = parsePort(takeValue(argv, i, "--web-port"), "--web-port");
      noteDeprecated("--web-port");
      i += 1;
      continue;
    }
    if (arg.startsWith("--web-port=")) {
      opts.webPort = parsePort(arg.split("=")[1], "--web-port");
      noteDeprecated("--web-port");
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
  return { options: opts, showHelp, deprecatedFlags };
}

function takeValue(argv: string[], i: number, flag: string): string {
  const v = argv[i + 1];
  if (v === undefined || v.startsWith("-")) {
    throw new ParseError(`${flag} requires a value`);
  }
  return v;
}

function parsePort(raw: string, flag: string): number {
  const n = Number(raw);
  if (!Number.isFinite(n)) {
    throw new ParseError(`${flag} value must be a number, got "${raw}"`);
  }
  return n;
}

export type ResolvedPorts = {
  backendPort?: number;
  webPort?: number;
};

// `--port` (and KANDEV_PORT) maps to the public front door: backend in run/start (proxied), web in dev (HMR direct). Explicit per-process flags still win.
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
  if (!val) return undefined;
  const n = Number(val);
  if (!Number.isFinite(n)) throw new ParseError(`${name} must be a number, got "${val}"`);
  return n;
}

// In dev, --port maps to web, so --backend-port → KANDEV_BACKEND_PORT (not --port).
export function deprecationReplacement(flag: string, command: Command): string {
  if (flag === "--backend-port") {
    return command === "dev" ? "KANDEV_BACKEND_PORT" : "--port";
  }
  if (flag === "--web-port") {
    return "--web-internal-port";
  }
  return "--port";
}

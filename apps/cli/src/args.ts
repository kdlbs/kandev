export type Command = "run" | "dev" | "start";

export type CliOptions = {
  command: Command;
  runtimeVersion?: string;
  backendPort?: number;
  webPort?: number;
  verbose?: boolean;
  debug?: boolean;
  showVersion?: boolean;
  /** Skip browser open + interactive prompts. Set by systemd/launchd units. */
  headless?: boolean;
};

export type ParseResult = {
  options: CliOptions;
  showHelp: boolean;
};

export class ParseError extends Error {}

export function parseArgs(argv: string[]): ParseResult {
  const opts: CliOptions = { command: "run" };
  let showHelp = false;
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--help" || arg === "-h") {
      showHelp = true;
      continue;
    }
    if (arg === "--version" || arg === "-V") {
      opts.showVersion = true;
      continue;
    }
    if (arg === "dev" || arg === "run" || arg === "start") {
      opts.command = arg;
      continue;
    }
    if (arg === "--runtime-version") {
      opts.runtimeVersion = takeValue(argv, i, "--runtime-version");
      i += 1;
      continue;
    }
    if (arg.startsWith("--runtime-version=")) {
      const value = arg.slice("--runtime-version=".length);
      if (value.length === 0) throw new ParseError("--runtime-version requires a value");
      opts.runtimeVersion = value;
      continue;
    }
    if (arg === "--dev") {
      opts.command = "dev";
      continue;
    }
    // --port is an alias for --backend-port (the user-facing port in run/start).
    if (arg === "--port" || arg === "--backend-port") {
      opts.backendPort = parsePort(takeValue(argv, i, arg), arg);
      i += 1;
      continue;
    }
    if (arg.startsWith("--port=") || arg.startsWith("--backend-port=")) {
      const flag = arg.startsWith("--port=") ? "--port" : "--backend-port";
      opts.backendPort = parsePort(arg.slice(flag.length + 1), flag);
      continue;
    }
    if (arg === "--web-internal-port") {
      opts.webPort = parsePort(takeValue(argv, i, "--web-internal-port"), "--web-internal-port");
      i += 1;
      continue;
    }
    if (arg.startsWith("--web-internal-port=")) {
      opts.webPort = parsePort(arg.slice("--web-internal-port=".length), "--web-internal-port");
      continue;
    }
    if (arg === "--web-port" || arg.startsWith("--web-port=")) {
      throw new ParseError("--web-port has been removed; use --web-internal-port for dev mode");
    }
    if (arg === "--verbose" || arg === "-v") {
      opts.verbose = true;
      continue;
    }
    if (arg === "--debug") {
      opts.debug = true;
      continue;
    }
    if (arg === "--headless" || arg === "--no-browser") {
      opts.headless = true;
      continue;
    }
  }
  if (opts.webPort !== undefined && opts.command !== "dev") {
    throw new ParseError("--web-internal-port only applies to dev mode");
  }
  return { options: opts, showHelp };
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
  if (raw === "" || !Number.isInteger(n) || n < 1 || n > 65535) {
    throw new ParseError(`${flag} value must be an integer between 1 and 65535, got "${raw}"`);
  }
  return n;
}

export type ResolvedPorts = {
  backendPort?: number;
  webPort?: number;
};

// CLI flags beat env vars; KANDEV_PORT is an alias for KANDEV_BACKEND_PORT.
export function resolvePorts(options: CliOptions, env: NodeJS.ProcessEnv): ResolvedPorts {
  if (options.webPort !== undefined && options.command !== "dev") {
    throw new ParseError("--web-internal-port only applies to dev mode");
  }
  return {
    backendPort:
      options.backendPort ?? envPort(env, "KANDEV_BACKEND_PORT") ?? envPort(env, "KANDEV_PORT"),
    webPort:
      options.command === "dev" ? (options.webPort ?? envPort(env, "KANDEV_WEB_PORT")) : undefined,
  };
}

function envPort(env: NodeJS.ProcessEnv, name: string): number | undefined {
  const val = env[name];
  if (val === undefined) return undefined;
  const n = Number(val);
  if (val === "" || !Number.isInteger(n) || n < 1 || n > 65535) {
    throw new ParseError(`${name} must be an integer between 1 and 65535, got "${val}"`);
  }
  return n;
}

export type Command = "run" | "dev" | "start";

export type CliOptions = {
  command: Command;
  runtimeVersion?: string;
  backendPort?: number;
  webPort?: number;
  verbose?: boolean;
  debug?: boolean;
  showVersion?: boolean;
};

export type ParseResult = {
  options: CliOptions;
  showHelp: boolean;
  /** Deprecated flags seen on the command line. cli.ts emits warnings after parsing. */
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
    if (arg === "--web-port") {
      opts.webPort = parsePort(takeValue(argv, i, "--web-port"), "--web-port");
      noteDeprecated("--web-port");
      i += 1;
      continue;
    }
    if (arg.startsWith("--web-port=")) {
      opts.webPort = parsePort(arg.slice("--web-port=".length), "--web-port");
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
  return {
    backendPort:
      options.backendPort ?? envPort(env, "KANDEV_BACKEND_PORT") ?? envPort(env, "KANDEV_PORT"),
    webPort: options.webPort ?? envPort(env, "KANDEV_WEB_PORT"),
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

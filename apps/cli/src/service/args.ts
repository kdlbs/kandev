import { ParseError } from "../args";

export type ServiceAction =
  | "install"
  | "uninstall"
  | "start"
  | "stop"
  | "restart"
  | "status"
  | "logs"
  | "config";

export type ServiceArgs = {
  action: ServiceAction;
  /** Install as a system unit/daemon instead of a per-user one. */
  system?: boolean;
  /** Backend port to bake into the unit (passed via KANDEV_SERVER_PORT). */
  port?: number;
  /** Override KANDEV_HOME_DIR baked into the unit. */
  homeDir?: string;
  /** Skip the Linux enable-linger prompt during install. */
  noBootStart?: boolean;
  /** Tail logs (only valid with `logs`). */
  follow?: boolean;
  /** Print help and exit. */
  showHelp?: boolean;
};

const VALID_ACTIONS = new Set<ServiceAction>([
  "install",
  "uninstall",
  "start",
  "stop",
  "restart",
  "status",
  "logs",
  "config",
]);

export function parseServiceArgs(argv: string[]): ServiceArgs {
  if (argv.length === 0 || argv[0] === "--help" || argv[0] === "-h") {
    return { action: "install", showHelp: true };
  }
  const head = argv[0];
  if (!VALID_ACTIONS.has(head as ServiceAction)) {
    throw new ParseError(
      `unknown service action "${head}". expected one of: ${[...VALID_ACTIONS].join(", ")}`,
    );
  }
  const out: ServiceArgs = { action: head as ServiceAction };
  for (let i = 1; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--help" || arg === "-h") {
      out.showHelp = true;
      continue;
    }
    if (arg === "--system") {
      out.system = true;
      continue;
    }
    if (arg === "--no-boot-start") {
      out.noBootStart = true;
      continue;
    }
    if (arg === "-f" || arg === "--follow") {
      out.follow = true;
      continue;
    }
    if (arg === "--port") {
      out.port = parsePort(takeValue(argv, i, "--port"), "--port");
      i += 1;
      continue;
    }
    if (arg.startsWith("--port=")) {
      out.port = parsePort(arg.slice("--port=".length), "--port");
      continue;
    }
    if (arg === "--home-dir") {
      out.homeDir = takeValue(argv, i, "--home-dir");
      i += 1;
      continue;
    }
    if (arg.startsWith("--home-dir=")) {
      const value = arg.slice("--home-dir=".length);
      if (value.length === 0) throw new ParseError("--home-dir requires a value");
      out.homeDir = value;
      continue;
    }
    throw new ParseError(`unknown flag "${arg}" for kandev service ${head}`);
  }
  return out;
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

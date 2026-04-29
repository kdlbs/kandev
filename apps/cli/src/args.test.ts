import { describe, expect, it } from "vitest";

import { deprecationReplacement, parseArgs, ParseError, resolvePorts } from "./args";

describe("parseArgs", () => {
  it("defaults to the run command with no args", () => {
    const { options, showHelp, deprecatedFlags } = parseArgs([]);
    expect(options.command).toBe("run");
    expect(showHelp).toBe(false);
    expect(deprecatedFlags).toEqual([]);
  });

  it("parses --port and --port=<n>", () => {
    expect(parseArgs(["--port", "3000"]).options.port).toBe(3000);
    expect(parseArgs(["--port=3000"]).options.port).toBe(3000);
  });

  it("parses --web-internal-port (the renamed advanced flag) without a deprecation note", () => {
    const r = parseArgs(["--web-internal-port", "12345"]);
    expect(r.options.webPort).toBe(12345);
    expect(r.deprecatedFlags).toEqual([]);
  });

  it("records --backend-port as deprecated but still parses it", () => {
    const r = parseArgs(["--backend-port", "3447"]);
    expect(r.options.backendPort).toBe(3447);
    expect(r.deprecatedFlags).toEqual(["--backend-port"]);
  });

  it("records --web-port as deprecated", () => {
    const r = parseArgs(["--web-port=8080"]);
    expect(r.options.webPort).toBe(8080);
    expect(r.deprecatedFlags).toEqual(["--web-port"]);
  });

  it("dedupes deprecated flags across repeats", () => {
    const r = parseArgs(["--backend-port=1", "--backend-port=2"]);
    expect(r.deprecatedFlags).toEqual(["--backend-port"]);
  });

  it("reports --help via showHelp without exiting", () => {
    expect(parseArgs(["--help"]).showHelp).toBe(true);
  });

  it("throws ParseError when a value-taking flag has no value", () => {
    expect(() => parseArgs(["--port"])).toThrow(ParseError);
    expect(() => parseArgs(["--port"])).toThrow(/--port requires a value/);
  });

  it("throws ParseError when the next token is another flag", () => {
    expect(() => parseArgs(["--port", "--debug"])).toThrow(/--port requires a value/);
  });

  it("throws ParseError on a non-numeric port value", () => {
    expect(() => parseArgs(["--port=abc"])).toThrow(/--port value must be an integer/);
  });

  it("throws ParseError on a non-integer port value", () => {
    expect(() => parseArgs(["--port=3000.5"])).toThrow(/--port value must be an integer/);
  });

  it.each(["0", "-1", "65536"])("throws ParseError on out-of-range port %s", (val) => {
    expect(() => parseArgs([`--port=${val}`])).toThrow(/--port value must be an integer between/);
  });
});

describe("resolvePorts", () => {
  it("maps --port to backend port for run/start", () => {
    const r = resolvePorts({ command: "start", port: 3447 }, {} as NodeJS.ProcessEnv);
    expect(r).toEqual({ backendPort: 3447, webPort: undefined });
  });

  it("maps --port to web port for dev", () => {
    const r = resolvePorts({ command: "dev", port: 3000 }, {} as NodeJS.ProcessEnv);
    expect(r).toEqual({ backendPort: undefined, webPort: 3000 });
  });

  it("explicit --backend-port wins over --port in start mode", () => {
    const r = resolvePorts(
      { command: "start", port: 3000, backendPort: 4000 },
      {} as NodeJS.ProcessEnv,
    );
    expect(r.backendPort).toBe(4000);
  });

  it("falls back to KANDEV_PORT when no flag is given", () => {
    const r = resolvePorts({ command: "run" }, { KANDEV_PORT: "5555" } as NodeJS.ProcessEnv);
    expect(r.backendPort).toBe(5555);
  });

  it("KANDEV_BACKEND_PORT wins over KANDEV_PORT", () => {
    const r = resolvePorts({ command: "run" }, {
      KANDEV_PORT: "5555",
      KANDEV_BACKEND_PORT: "6666",
    } as NodeJS.ProcessEnv);
    expect(r.backendPort).toBe(6666);
  });

  it("--port wins over KANDEV_BACKEND_PORT (explicit CLI > env var)", () => {
    const r = resolvePorts({ command: "start", port: 3000 }, {
      KANDEV_BACKEND_PORT: "4000",
    } as NodeJS.ProcessEnv);
    expect(r.backendPort).toBe(3000);
  });

  it("--port wins over KANDEV_WEB_PORT in dev (explicit CLI > env var)", () => {
    const r = resolvePorts({ command: "dev", port: 3000 }, {
      KANDEV_WEB_PORT: "4000",
    } as NodeJS.ProcessEnv);
    expect(r.webPort).toBe(3000);
  });

  it("KANDEV_PORT routes to web port for dev", () => {
    const r = resolvePorts({ command: "dev" }, { KANDEV_PORT: "9000" } as NodeJS.ProcessEnv);
    expect(r).toEqual({ backendPort: undefined, webPort: 9000 });
  });

  it("returns undefined for both ports when nothing is set", () => {
    const r = resolvePorts({ command: "run" }, {} as NodeJS.ProcessEnv);
    expect(r).toEqual({ backendPort: undefined, webPort: undefined });
  });

  it("throws ParseError when KANDEV_PORT is not a number", () => {
    expect(() =>
      resolvePorts({ command: "run" }, { KANDEV_PORT: "abc" } as NodeJS.ProcessEnv),
    ).toThrow(ParseError);
  });

  it("throws ParseError when KANDEV_BACKEND_PORT is not a number", () => {
    expect(() =>
      resolvePorts({ command: "run" }, { KANDEV_BACKEND_PORT: "nope" } as NodeJS.ProcessEnv),
    ).toThrow(/KANDEV_BACKEND_PORT must be an integer/);
  });

  it("throws ParseError when KANDEV_PORT is a float", () => {
    expect(() =>
      resolvePorts({ command: "run" }, { KANDEV_PORT: "3000.5" } as NodeJS.ProcessEnv),
    ).toThrow(/KANDEV_PORT must be an integer/);
  });

  it.each(["0", "-1", "65536"])("throws ParseError when KANDEV_PORT is out-of-range %s", (val) => {
    expect(() =>
      resolvePorts({ command: "run" }, { KANDEV_PORT: val } as NodeJS.ProcessEnv),
    ).toThrow(/KANDEV_PORT must be an integer between/);
  });

  it("KANDEV_WEB_PORT sets the web port in dev", () => {
    const r = resolvePorts({ command: "dev" }, { KANDEV_WEB_PORT: "8080" } as NodeJS.ProcessEnv);
    expect(r.webPort).toBe(8080);
  });

  it("KANDEV_WEB_PORT sets the internal web port in run/start (backend stays undefined)", () => {
    const r = resolvePorts({ command: "run" }, { KANDEV_WEB_PORT: "8080" } as NodeJS.ProcessEnv);
    expect(r).toEqual({ backendPort: undefined, webPort: 8080 });
  });

  it("throws ParseError when KANDEV_WEB_PORT is out of range", () => {
    expect(() =>
      resolvePorts({ command: "dev" }, { KANDEV_WEB_PORT: "0" } as NodeJS.ProcessEnv),
    ).toThrow(/KANDEV_WEB_PORT must be an integer between/);
  });
});

describe("deprecationReplacement", () => {
  it("points --backend-port at --port for run/start", () => {
    expect(deprecationReplacement("--backend-port", "run")).toBe("--port");
    expect(deprecationReplacement("--backend-port", "start")).toBe("--port");
  });

  it("points --backend-port at the env var in dev (since --port maps to web there)", () => {
    expect(deprecationReplacement("--backend-port", "dev")).toBe("KANDEV_BACKEND_PORT");
  });

  it("points --web-port at --web-internal-port", () => {
    expect(deprecationReplacement("--web-port", "run")).toBe("--web-internal-port");
    expect(deprecationReplacement("--web-port", "dev")).toBe("--web-internal-port");
  });
});

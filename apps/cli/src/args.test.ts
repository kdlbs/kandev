import { describe, expect, it } from "vitest";

import { parseArgs, ParseError, resolvePorts } from "./args";

describe("parseArgs", () => {
  it("defaults to the run command with no args", () => {
    const { options, showHelp, deprecatedFlags } = parseArgs([]);
    expect(options.command).toBe("run");
    expect(showHelp).toBe(false);
    expect(deprecatedFlags).toEqual([]);
  });

  it("parses --port and --port=<n> as the backend port", () => {
    expect(parseArgs(["--port", "3000"]).options.backendPort).toBe(3000);
    expect(parseArgs(["--port=3000"]).options.backendPort).toBe(3000);
  });

  it("parses --backend-port the same as --port (no deprecation note)", () => {
    expect(parseArgs(["--backend-port=3447"]).options.backendPort).toBe(3447);
    expect(parseArgs(["--backend-port=3447"]).deprecatedFlags).toEqual([]);
  });

  it("parses --web-internal-port without a deprecation note", () => {
    const r = parseArgs(["--web-internal-port", "12345"]);
    expect(r.options.webPort).toBe(12345);
    expect(r.deprecatedFlags).toEqual([]);
  });

  it("records --web-port as deprecated", () => {
    const r = parseArgs(["--web-port=8080"]);
    expect(r.options.webPort).toBe(8080);
    expect(r.deprecatedFlags).toEqual(["--web-port"]);
  });

  it("dedupes deprecated flags across repeats", () => {
    const r = parseArgs(["--web-port=1", "--web-port=2"]);
    expect(r.deprecatedFlags).toEqual(["--web-port"]);
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

  it("throws ParseError on empty --port=", () => {
    expect(() => parseArgs(["--port="])).toThrow(/--port value must be an integer/);
  });

  it("sets showVersion for --version and -V flags", () => {
    expect(parseArgs(["--version"]).options.showVersion).toBe(true);
    expect(parseArgs(["-V"]).options.showVersion).toBe(true);
  });

  it("parses --runtime-version as runtimeVersion", () => {
    expect(parseArgs(["--runtime-version", "v0.16.0"]).options.runtimeVersion).toBe("v0.16.0");
    expect(parseArgs(["--runtime-version=v0.16.0"]).options.runtimeVersion).toBe("v0.16.0");
  });

  it("throws ParseError on empty --runtime-version=", () => {
    expect(() => parseArgs(["--runtime-version="])).toThrow(/--runtime-version requires a value/);
  });

  it("throws ParseError when --runtime-version has no value", () => {
    expect(() => parseArgs(["--runtime-version"])).toThrow(ParseError);
    expect(() => parseArgs(["--runtime-version"])).toThrow(/--runtime-version requires a value/);
  });

  it("throws ParseError when the next token after --runtime-version is another flag", () => {
    expect(() => parseArgs(["--runtime-version", "--debug"])).toThrow(
      /--runtime-version requires a value/,
    );
  });
});

describe("resolvePorts", () => {
  it("returns undefined for both ports when nothing is set", () => {
    const r = resolvePorts({ command: "run" }, {} as NodeJS.ProcessEnv);
    expect(r).toEqual({ backendPort: undefined, webPort: undefined });
  });

  it("CLI backendPort wins over env vars", () => {
    const r = resolvePorts({ command: "start", backendPort: 3000 }, {
      KANDEV_BACKEND_PORT: "4000",
      KANDEV_PORT: "5000",
    } as NodeJS.ProcessEnv);
    expect(r.backendPort).toBe(3000);
  });

  it("KANDEV_BACKEND_PORT wins over KANDEV_PORT (more specific env wins)", () => {
    const r = resolvePorts({ command: "run" }, {
      KANDEV_PORT: "5555",
      KANDEV_BACKEND_PORT: "6666",
    } as NodeJS.ProcessEnv);
    expect(r.backendPort).toBe(6666);
  });

  it("falls back to KANDEV_PORT when KANDEV_BACKEND_PORT is not set", () => {
    const r = resolvePorts({ command: "run" }, { KANDEV_PORT: "5555" } as NodeJS.ProcessEnv);
    expect(r.backendPort).toBe(5555);
  });

  it("KANDEV_WEB_PORT sets the internal web port in dev", () => {
    const r = resolvePorts({ command: "dev" }, { KANDEV_WEB_PORT: "8080" } as NodeJS.ProcessEnv);
    expect(r.webPort).toBe(8080);
  });

  it("KANDEV_WEB_PORT sets the internal web port in run/start (backend stays undefined)", () => {
    const r = resolvePorts({ command: "run" }, { KANDEV_WEB_PORT: "8080" } as NodeJS.ProcessEnv);
    expect(r).toEqual({ backendPort: undefined, webPort: 8080 });
  });

  it("--port maps to backend in every command (including dev)", () => {
    const r = resolvePorts({ command: "dev", backendPort: 3447 }, {} as NodeJS.ProcessEnv);
    expect(r).toEqual({ backendPort: 3447, webPort: undefined });
  });

  it("throws ParseError when KANDEV_PORT is not a number", () => {
    expect(() =>
      resolvePorts({ command: "run" }, { KANDEV_PORT: "abc" } as NodeJS.ProcessEnv),
    ).toThrow(ParseError);
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

  it.each(["KANDEV_PORT", "KANDEV_BACKEND_PORT", "KANDEV_WEB_PORT"])(
    "throws ParseError when %s is set to empty string",
    (name) => {
      expect(() => resolvePorts({ command: "run" }, { [name]: "" } as NodeJS.ProcessEnv)).toThrow(
        new RegExp(`${name} must be an integer`),
      );
    },
  );
});

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { _resetWarnedFlagsForTest, parseArgs, resolvePorts } from "./args";

describe("parseArgs", () => {
  let stderrSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    _resetWarnedFlagsForTest();
    stderrSpy = vi.spyOn(process.stderr, "write").mockImplementation(() => true);
  });

  afterEach(() => {
    stderrSpy.mockRestore();
  });

  it("defaults to the run command with no args", () => {
    const { options, showHelp } = parseArgs([]);
    expect(options.command).toBe("run");
    expect(showHelp).toBe(false);
  });

  it("parses --port and --port=<n>", () => {
    expect(parseArgs(["--port", "3000"]).options.port).toBe(3000);
    expect(parseArgs(["--port=3000"]).options.port).toBe(3000);
  });

  it("parses --web-internal-port (the renamed advanced flag) without warning", () => {
    expect(parseArgs(["--web-internal-port", "12345"]).options.webPort).toBe(12345);
    expect(parseArgs(["--web-internal-port=12345"]).options.webPort).toBe(12345);
    expect(stderrSpy).not.toHaveBeenCalled();
  });

  it("emits a deprecation warning for --backend-port but still parses it", () => {
    const { options } = parseArgs(["--backend-port", "3447"]);
    expect(options.backendPort).toBe(3447);
    expect(stderrSpy).toHaveBeenCalledWith(
      expect.stringContaining("--backend-port is deprecated; use --port"),
    );
  });

  it("emits a deprecation warning for --web-port pointing at --web-internal-port", () => {
    const { options } = parseArgs(["--web-port=8080"]);
    expect(options.webPort).toBe(8080);
    expect(stderrSpy).toHaveBeenCalledWith(
      expect.stringContaining("--web-port is deprecated; use --web-internal-port"),
    );
  });

  it("warns about each deprecated flag at most once per process", () => {
    parseArgs(["--backend-port=1", "--backend-port=2"]);
    expect(stderrSpy).toHaveBeenCalledTimes(1);
  });

  it("reports --help via showHelp without exiting", () => {
    const { showHelp } = parseArgs(["--help"]);
    expect(showHelp).toBe(true);
  });
});

describe("resolvePorts", () => {
  it("maps --port to backend port for run/start", () => {
    const r = resolvePorts(
      { command: "start", port: 3447 },
      {} as NodeJS.ProcessEnv,
    );
    expect(r).toEqual({ backendPort: 3447, webPort: undefined });
  });

  it("maps --port to web port for dev", () => {
    const r = resolvePorts(
      { command: "dev", port: 3000 },
      {} as NodeJS.ProcessEnv,
    );
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
    const r = resolvePorts(
      { command: "run" },
      { KANDEV_PORT: "5555", KANDEV_BACKEND_PORT: "6666" } as NodeJS.ProcessEnv,
    );
    expect(r.backendPort).toBe(6666);
  });

  it("KANDEV_PORT routes to web port for dev", () => {
    const r = resolvePorts({ command: "dev" }, { KANDEV_PORT: "9000" } as NodeJS.ProcessEnv);
    expect(r).toEqual({ backendPort: undefined, webPort: 9000 });
  });

  it("returns undefined for both ports when nothing is set", () => {
    const r = resolvePorts({ command: "run" }, {} as NodeJS.ProcessEnv);
    expect(r).toEqual({ backendPort: undefined, webPort: undefined });
  });
});

import { describe, expect, it } from "vitest";

import { ParseError } from "../args";
import { parseServiceArgs } from "./args";

describe("parseServiceArgs", () => {
  it("returns showHelp when invoked with no args", () => {
    const r = parseServiceArgs([]);
    expect(r.showHelp).toBe(true);
  });

  it("returns showHelp on --help with no action", () => {
    expect(parseServiceArgs(["--help"]).showHelp).toBe(true);
    expect(parseServiceArgs(["-h"]).showHelp).toBe(true);
  });

  it("parses each valid action", () => {
    for (const action of [
      "install",
      "uninstall",
      "start",
      "stop",
      "restart",
      "status",
      "logs",
      "config",
    ] as const) {
      expect(parseServiceArgs([action]).action).toBe(action);
    }
  });

  it("throws ParseError on unknown action", () => {
    expect(() => parseServiceArgs(["nope"])).toThrow(ParseError);
    expect(() => parseServiceArgs(["nope"])).toThrow(/unknown service action/);
  });

  it("parses --system on install", () => {
    expect(parseServiceArgs(["install", "--system"]).system).toBe(true);
    expect(parseServiceArgs(["install"]).system).toBeUndefined();
  });

  it("parses --port", () => {
    expect(parseServiceArgs(["install", "--port", "9000"]).port).toBe(9000);
    expect(parseServiceArgs(["install", "--port=9000"]).port).toBe(9000);
  });

  it("rejects invalid --port values", () => {
    expect(() => parseServiceArgs(["install", "--port=abc"])).toThrow(/must be an integer/);
    expect(() => parseServiceArgs(["install", "--port=0"])).toThrow(/between 1 and 65535/);
    expect(() => parseServiceArgs(["install", "--port=65536"])).toThrow(/between 1 and 65535/);
  });

  it("parses --home-dir", () => {
    expect(parseServiceArgs(["install", "--home-dir", "/srv/kandev"]).homeDir).toBe("/srv/kandev");
    expect(parseServiceArgs(["install", "--home-dir=/srv/kandev"]).homeDir).toBe("/srv/kandev");
  });

  it("rejects empty --home-dir", () => {
    expect(() => parseServiceArgs(["install", "--home-dir="])).toThrow(/--home-dir requires/);
  });

  it("parses --no-boot-start", () => {
    expect(parseServiceArgs(["install", "--no-boot-start"]).noBootStart).toBe(true);
  });

  it("parses --follow on logs", () => {
    expect(parseServiceArgs(["logs", "-f"]).follow).toBe(true);
    expect(parseServiceArgs(["logs", "--follow"]).follow).toBe(true);
  });

  it("throws ParseError on unknown flag", () => {
    expect(() => parseServiceArgs(["install", "--nope"])).toThrow(/unknown flag/);
  });

  it("rejects --follow for actions other than logs", () => {
    expect(() => parseServiceArgs(["start", "--follow"])).toThrow(
      /--follow only applies to 'kandev service logs'/,
    );
    expect(() => parseServiceArgs(["status", "-f"])).toThrow(
      /--follow only applies to 'kandev service logs'/,
    );
    // happy path
    expect(parseServiceArgs(["logs", "-f"]).follow).toBe(true);
  });

  it("rejects install-only flags on non-install actions", () => {
    expect(() => parseServiceArgs(["stop", "--port=9000"])).toThrow(
      /--port only applies to 'kandev service install'/,
    );
    expect(() => parseServiceArgs(["restart", "--home-dir=/x"])).toThrow(
      /--home-dir only applies to 'kandev service install'/,
    );
    expect(() => parseServiceArgs(["status", "--no-boot-start"])).toThrow(
      /--no-boot-start only applies to 'kandev service install'/,
    );
    // happy path
    expect(parseServiceArgs(["install", "--port=9000"]).port).toBe(9000);
  });
});

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { createDebugLogger, registerSessionTaskResolver } from "./log";

describe("createDebugLogger", () => {
  beforeEach(() => {
    vi.spyOn(console, "debug").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns a function", () => {
    expect(typeof createDebugLogger("test")).toBe("function");
  });

  it("prefixes output with [namespace]", () => {
    const log = createDebugLogger("my-ns");
    log("hello");
    expect(console.debug).toHaveBeenCalledWith("[my-ns] hello");
  });

  it("logs plain strings as-is", () => {
    const log = createDebugLogger("ns");
    log("simple message");
    expect(console.debug).toHaveBeenCalledWith("[ns] simple message");
  });

  it("flattens plain objects to key=value pairs", () => {
    const log = createDebugLogger("ns");
    log("msg", { a: 1, b: "hello world" });
    expect(console.debug).toHaveBeenCalledWith('[ns] msg a=1 b="hello world"');
  });

  it("quotes values containing spaces", () => {
    const log = createDebugLogger("ns");
    log({ key: "value with space" });
    expect(console.debug).toHaveBeenCalledWith('[ns] key="value with space"');
  });

  it("leaves bare values unquoted", () => {
    const log = createDebugLogger("ns");
    log({ key: "simple_value" });
    expect(console.debug).toHaveBeenCalledWith("[ns] key=simple_value");
  });

  it("formats null and undefined", () => {
    const log = createDebugLogger("ns");
    log({ a: null, b: undefined });
    expect(console.debug).toHaveBeenCalledWith("[ns] a=null b=undefined");
  });

  it("formats numbers and booleans", () => {
    const log = createDebugLogger("ns");
    log({ count: 42, flag: true });
    expect(console.debug).toHaveBeenCalledWith("[ns] count=42 flag=true");
  });

  it("formats Error objects with name and message", () => {
    const log = createDebugLogger("ns");
    log({ err: new Error("boom") });
    expect(console.debug).toHaveBeenCalledWith('[ns] err={"name":"Error","message":"boom"}');
  });

  it("serializes nested objects as JSON", () => {
    const log = createDebugLogger("ns");
    log({ nested: { x: 1 } });
    expect(console.debug).toHaveBeenCalledWith('[ns] nested={"x":1}');
  });

  it("mixes strings and objects in a single call", () => {
    const log = createDebugLogger("ns");
    log("prefix", { a: 1 }, "suffix");
    expect(console.debug).toHaveBeenCalledWith("[ns] prefix a=1 suffix");
  });
});

describe("registerSessionTaskResolver", () => {
  beforeEach(() => {
    vi.spyOn(console, "debug").mockImplementation(() => {});
  });

  afterEach(() => {
    registerSessionTaskResolver(null);
    vi.restoreAllMocks();
  });

  it("annotates lines carrying a sessionId with task_id", () => {
    registerSessionTaskResolver((sid) => (sid === "s_1" ? "t_42" : undefined));
    const log = createDebugLogger("ns");
    log("msg", { sessionId: "s_1", fileCount: 3 });
    expect(console.debug).toHaveBeenCalledWith("[ns] msg sessionId=s_1 fileCount=3 task_id=t_42");
  });

  it("resolves from a snake_case session_id field too", () => {
    registerSessionTaskResolver(() => "t_99");
    const log = createDebugLogger("ns");
    log("msg", { session_id: "s_1" });
    expect(console.debug).toHaveBeenCalledWith("[ns] msg session_id=s_1 task_id=t_99");
  });

  it("does not annotate when the line already names a task", () => {
    registerSessionTaskResolver(() => "t_resolved");
    const log = createDebugLogger("ns");
    log("msg", { sessionId: "s_1", taskId: "t_explicit" });
    expect(console.debug).toHaveBeenCalledWith("[ns] msg sessionId=s_1 taskId=t_explicit");
  });

  it("does not annotate when no session is present", () => {
    registerSessionTaskResolver(() => "t_42");
    const log = createDebugLogger("ns");
    log("msg", { foo: "bar" });
    expect(console.debug).toHaveBeenCalledWith("[ns] msg foo=bar");
  });

  it("omits task_id when the session cannot be resolved", () => {
    registerSessionTaskResolver(() => undefined);
    const log = createDebugLogger("ns");
    log("msg", { sessionId: "s_unknown" });
    expect(console.debug).toHaveBeenCalledWith("[ns] msg sessionId=s_unknown");
  });

  it("never throws when the resolver throws", () => {
    registerSessionTaskResolver(() => {
      throw new Error("store gone");
    });
    const log = createDebugLogger("ns");
    expect(() => log("msg", { sessionId: "s_1" })).not.toThrow();
    expect(console.debug).toHaveBeenCalledWith("[ns] msg sessionId=s_1");
  });

  it("stops annotating once cleared with null", () => {
    registerSessionTaskResolver(() => "t_42");
    registerSessionTaskResolver(null);
    const log = createDebugLogger("ns");
    log("msg", { sessionId: "s_1" });
    expect(console.debug).toHaveBeenCalledWith("[ns] msg sessionId=s_1");
  });
});

describe("IS_DEBUG", () => {
  // Module re-imports are required because IS_DEBUG is a module-level constant
  // evaluated at import time. resetModules forces re-evaluation with the
  // current env / globals each time.

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("is true in a production build when window.__KANDEV_DEBUG is set", async () => {
    // Simulates `make start-debug`: production bundle, runtime window flag.
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("NEXT_PUBLIC_KANDEV_DEBUG", "");
    vi.stubGlobal("window", { __KANDEV_DEBUG: true });
    const { IS_DEBUG } = await import("./log");
    expect(IS_DEBUG).toBe(true);
  });

  it("is false in a production build with no flag set", async () => {
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("NEXT_PUBLIC_KANDEV_DEBUG", "");
    vi.stubGlobal("window", {});
    const { IS_DEBUG } = await import("./log");
    expect(IS_DEBUG).toBe(false);
  });

  it("is true when NEXT_PUBLIC_KANDEV_DEBUG=true at build time", async () => {
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("NEXT_PUBLIC_KANDEV_DEBUG", "true");
    vi.stubGlobal("window", {});
    const { IS_DEBUG } = await import("./log");
    expect(IS_DEBUG).toBe(true);
  });
});

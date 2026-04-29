import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

import { _resetForTesting, snapshotLogs } from "./buffer";
import { _resetInstalledForTesting, installConsoleInterceptor } from "./intercept";

describe("console interceptor", () => {
  // Hold originals so each test starts with a clean console.
  const originals = {
    debug: console.debug,
    info: console.info,
    warn: console.warn,
    error: console.error,
  };

  let infoSpy: ReturnType<typeof vi.fn>;
  let warnSpy: ReturnType<typeof vi.fn>;
  let errorSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    _resetForTesting();
    _resetInstalledForTesting();
    infoSpy = vi.fn();
    warnSpy = vi.fn();
    errorSpy = vi.fn();
    console.debug = vi.fn();
    console.info = infoSpy;
    console.warn = warnSpy;
    console.error = errorSpy;
  });

  afterEach(() => {
    Object.assign(console, originals);
  });

  it("captures console.* into the ring buffer and still calls the original", () => {
    installConsoleInterceptor();

    console.info("hello", { user: "x" });
    console.warn("careful");
    console.error(new Error("boom"));

    const snap = snapshotLogs();
    expect(snap).toHaveLength(3);
    expect(snap[0].level).toBe("info");
    expect(snap[0].message).toBe("hello");
    expect(snap[0].source).toBe("console");
    expect(snap[1].level).toBe("warn");
    expect(snap[2].level).toBe("error");
    expect(snap[2].message).toBe("boom");
    // Error stack must survive on the entry so the bundle preserves it.
    expect(typeof snap[2].stack).toBe("string");
    expect(snap[2].stack).toContain("Error");

    expect(infoSpy).toHaveBeenCalledWith("hello", { user: "x" });
    expect(warnSpy).toHaveBeenCalledWith("careful");
    expect(errorSpy).toHaveBeenCalled();
  });

  it("is idempotent — second install does not double-wrap", () => {
    installConsoleInterceptor();
    installConsoleInterceptor();

    console.info("once");
    expect(snapshotLogs()).toHaveLength(1);
  });

  it("captures window error events", () => {
    installConsoleInterceptor();

    const err = new Error("explode");
    window.dispatchEvent(
      new ErrorEvent("error", {
        message: err.message,
        error: err,
        filename: "x.ts",
        lineno: 1,
        colno: 2,
      }),
    );

    const snap = snapshotLogs();
    expect(snap.some((e) => e.source === "window.onerror" && e.message === "explode")).toBe(true);
  });

  it("captures unhandled promise rejections", () => {
    installConsoleInterceptor();

    const reason = new Error("rejected");
    const event = new Event("unhandledrejection") as Event & {
      reason: unknown;
      promise: Promise<unknown>;
    };
    Object.defineProperty(event, "reason", { value: reason });
    Object.defineProperty(event, "promise", {
      value: Promise.reject(reason).catch(() => undefined),
    });
    window.dispatchEvent(event);

    const snap = snapshotLogs();
    expect(snap.some((e) => e.source === "unhandledrejection" && e.message === "rejected")).toBe(
      true,
    );
  });
});

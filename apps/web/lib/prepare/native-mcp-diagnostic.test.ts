import { describe, expect, it } from "vitest";
import { normalizeNativeMcpDiagnostic } from "./native-mcp-diagnostic";

const diagnostic = {
  operation: "enable",
  stage: "wait",
  kind: "output_wait_timeout",
  message: "exec: WaitDelay expired before I/O complete",
  exit_code: 0,
  cleanup_message: "",
};

describe("normalizeNativeMcpDiagnostic", () => {
  it("accepts the closed diagnostic contract and returns only supported fields", () => {
    expect(normalizeNativeMcpDiagnostic({ ...diagnostic, unexpected: "must be dropped" })).toEqual({
      operation: "enable",
      stage: "wait",
      kind: "output_wait_timeout",
      message: "exec: WaitDelay expired before I/O complete",
      exitCode: 0,
    });
  });

  it("rejects unknown enum values, empty messages, invalid exit codes and oversized UTF-8 text", () => {
    expect(normalizeNativeMcpDiagnostic({ ...diagnostic, operation: "run" })).toBeUndefined();
    expect(normalizeNativeMcpDiagnostic({ ...diagnostic, stage: "spawn" })).toBeUndefined();
    expect(normalizeNativeMcpDiagnostic({ ...diagnostic, kind: "unknown" })).toBeUndefined();
    expect(normalizeNativeMcpDiagnostic({ ...diagnostic, message: "" })).toBeUndefined();
    expect(normalizeNativeMcpDiagnostic({ ...diagnostic, exit_code: -1 })).toBeUndefined();
    expect(
      normalizeNativeMcpDiagnostic({ ...diagnostic, message: "é".repeat(513) }),
    ).toBeUndefined();
    expect(
      normalizeNativeMcpDiagnostic({
        ...diagnostic,
        message: "failed: https://private.test/token",
      }),
    ).toBeUndefined();
    expect(
      normalizeNativeMcpDiagnostic({ ...diagnostic, message: "failed\u001b[31m" }),
    ).toBeUndefined();
    expect(normalizeNativeMcpDiagnostic({ ...diagnostic, cleanup_message: 5 })).toBeUndefined();
    expect(
      normalizeNativeMcpDiagnostic({ ...diagnostic, cleanup_message: "x".repeat(1025) }),
    ).toBeUndefined();
  });
});

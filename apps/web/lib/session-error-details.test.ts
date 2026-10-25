import { describe, expect, it } from "vitest";
import { formatSessionErrorDetails, sanitizeSessionErrorDetails } from "./session-error-details";

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.13
// These are synthetic diagnostics, never real credentials.
describe("session error diagnostics", () => {
  it.each([
    ["Authorization: Bearer private-credential\nconnection refused", "private-credential"],
    [
      'failed: {"secret":"private credential", "nested": {"token":"private-tail"}}\nconnection refused',
      "private-tail",
    ],
    ["secret: abc'def'ghi\nconnection refused", "def"],
    ['token="unterminated private-value\nconnection refused', "private-value"],
    [
      "resolve secret runtime-agentctl-auth-secret: missing\nconnection refused",
      "runtime-agentctl-auth-secret",
    ],
    ["open /home/alice/private/repo/file: connection refused", "/home/alice"],
    ["open C:\\Users\\Alice\\private\\file: connection refused", "Alice"],
    [
      "request https://user:private@host.test/private/account?token=hidden: connection refused",
      "private",
    ],
    ["session ses_private_id: connection refused", "ses_private_id"],
    ["key sk-abcdefghijklmnopqrstuvwxyz: connection refused", "abcdefghijklmnopqrstuvwxyz"],
  ])("redacts diagnostic %s", (input, secret) => {
    const result = sanitizeSessionErrorDetails(input);
    expect(result).not.toContain(secret);
    if (!input.includes("unterminated")) expect(result).toContain("connection refused");
    expect(sanitizeSessionErrorDetails(result)).toBe(result);
  });

  it("does not stringify arbitrary payloads", () => {
    expect(sanitizeSessionErrorDetails({ toString: () => "private-object" })).toBe("");
  });

  it("redacts before truncation can sever the sensitive assignment", () => {
    const result = sanitizeSessionErrorDetails('token="' + "private ".repeat(1000));
    expect(result).not.toContain("private");
  });
});

it.each(["secret:\nprivate-value", "credential=private-value", "secret://private-value/path"])(
  "hides multiline and referenced credentials: %s",
  (input) => {
    expect(sanitizeSessionErrorDetails(input)).not.toContain("private-value");
  },
);

it("sanitizes prose before adding separately validated host references", () => {
  const attemptId = "resume-1";
  const executionId = "650e8400-e29b-41d4-a716-446655440000";
  const text = formatSessionErrorDetails("token=private-token", [
    { label: "Phase", value: "bootstrap" },
    { label: "Occurred", value: "2026-09-30T10:00:00Z" },
    { label: "Attempt", value: attemptId, kind: "host-attempt-reference" },
    { label: "Execution", value: executionId, kind: "host-execution-reference" },
    {
      label: "Invalid attempt",
      value: "resume-999999999999999999999999 token=malicious-private-value",
      kind: "host-attempt-reference",
    },
    { label: "Invalid execution", value: "execution started", kind: "host-execution-reference" },
    { label: "Model", value: "vendor/opus-5" },
  ]);

  expect(text).toContain("Phase: bootstrap");
  expect(text).toContain("Occurred: 2026-09-30T10:00:00Z");
  expect(text).toContain(`Attempt: ${attemptId}`);
  expect(text).toContain(`Execution: ${executionId}`);
  expect(text).toContain("Model: vendor/opus-5");
  expect(text).not.toContain("private-token");
  expect(text).not.toContain("malicious-private-value");
  expect(text).not.toContain("execution started");
});

import { describe, expect, it } from "vitest";
import { summarizePermissionAction } from "./permission-action-summary";

describe("summarizePermissionAction", () => {
  it("returns null when details are missing", () => {
    expect(summarizePermissionAction(undefined, "x")).toBeNull();
  });

  it("prefers explicit description", () => {
    expect(
      summarizePermissionAction(
        { description: "Run bash command 'ls -la'", raw_input: { command: "ls -la" } },
        "other",
      ),
    ).toBe("Run bash command 'ls -la'");
  });

  it("falls back to raw_input.command", () => {
    expect(summarizePermissionAction({ raw_input: { command: "rm -rf /tmp/x" } }, "execute")).toBe(
      "rm -rf /tmp/x",
    );
  });

  it("picks file_path from raw_input", () => {
    expect(
      summarizePermissionAction({ raw_input: { file_path: "/etc/hosts", limit: 5 } }, "read"),
    ).toBe("/etc/hosts");
  });

  it("falls back to JSON when no preferred key matches", () => {
    expect(summarizePermissionAction({ raw_input: { foo: "bar", n: 1 } }, "other")).toBe(
      `{"foo":"bar","n":1}`,
    );
  });

  it("skips when summary equals the title", () => {
    expect(summarizePermissionAction({ description: "same" }, "same")).toBeNull();
  });

  it("uses legacy top-level command/path when raw_input absent", () => {
    expect(summarizePermissionAction({ command: 'grep -r "x"' }, "other")).toBe('grep -r "x"');
    expect(summarizePermissionAction({ path: "/foo/bar" }, "read")).toBe("/foo/bar");
  });

  it("returns null when raw_input is empty object", () => {
    expect(summarizePermissionAction({ raw_input: {} }, "other")).toBeNull();
  });

  it("truncates very long values", () => {
    const long = "a".repeat(500);
    const result = summarizePermissionAction({ description: long }, "other");
    expect(result).not.toBeNull();
    expect(result!.length).toBeLessThanOrEqual(200);
    expect(result!.endsWith("…")).toBe(true);
  });
});

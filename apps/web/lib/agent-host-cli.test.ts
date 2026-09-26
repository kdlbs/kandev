import { describe, expect, it } from "vitest";
import { presentHostCLIVersion, presentModelDiscovery } from "@/lib/agent-host-cli";
import type { ModelDiscovery } from "@/lib/types/http";

describe("presentHostCLIVersion", () => {
  it("returns known with the detected version", () => {
    expect(presentHostCLIVersion({ cli_version: "2.1.220", cli_version_error: undefined })).toEqual(
      { kind: "known", version: "2.1.220" },
    );
  });

  it("returns unknown with the reason when only the error is present", () => {
    expect(
      presentHostCLIVersion({ cli_version: undefined, cli_version_error: "command timed out" }),
    ).toEqual({ kind: "unknown", reason: "command timed out" });
  });

  it("returns null for an agent type with no host CLI", () => {
    expect(
      presentHostCLIVersion({ cli_version: undefined, cli_version_error: undefined }),
    ).toBeNull();
  });

  it("prefers the version over a stale error when both are set", () => {
    expect(
      presentHostCLIVersion({ cli_version: "0.155.1", cli_version_error: "previous failure" }),
    ).toEqual({ kind: "known", version: "0.155.1" });
  });
});

function makeDiscovery(overrides: Partial<ModelDiscovery>): ModelDiscovery {
  return {
    source: "acp_probe",
    status: "ok",
    allows_custom_model: true,
    ...overrides,
  };
}

describe("presentModelDiscovery", () => {
  it("returns null when no discovery block is present", () => {
    expect(presentModelDiscovery(undefined)).toBeNull();
  });

  it("returns null while discovery is pending, to avoid a flicker", () => {
    expect(presentModelDiscovery(makeDiscovery({ status: "pending" }))).toBeNull();
  });

  it("returns cli with the executable and version on a successful CLI listing", () => {
    expect(
      presentModelDiscovery(
        makeDiscovery({
          status: "ok",
          source: "cli_command",
          executable: "codex",
          cli_version: "0.155.1",
        }),
      ),
    ).toEqual({ kind: "cli", executable: "codex", version: "0.155.1" });
  });

  it("returns no_listing when the CLI declares no model source (skipped)", () => {
    expect(
      presentModelDiscovery(
        makeDiscovery({ status: "skipped", source: "acp_probe", executable: "claude" }),
      ),
    ).toEqual({ kind: "no_listing", executable: "claude" });
  });

  it("returns no_listing when the CLI succeeded but the bridge stayed the source", () => {
    expect(
      presentModelDiscovery(
        makeDiscovery({ status: "ok", source: "acp_probe", executable: "codex" }),
      ),
    ).toEqual({ kind: "no_listing", executable: "codex" });
  });

  it("returns failed with the reported error for a failure status", () => {
    expect(
      presentModelDiscovery(
        makeDiscovery({ status: "not_logged_in", error: "the command-line tool is not signed in" }),
      ),
    ).toEqual({ kind: "failed", reason: "the command-line tool is not signed in" });
  });

  it("falls back to the status string when a failure carries no error message", () => {
    expect(presentModelDiscovery(makeDiscovery({ status: "failed", error: undefined }))).toEqual({
      kind: "failed",
      reason: "failed",
    });
  });
});

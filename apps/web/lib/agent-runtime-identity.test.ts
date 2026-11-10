import { describe, expect, it } from "vitest";
import type { AgentUpdateStatus } from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";
import { presentAgentRuntimeIdentity } from "./agent-runtime-identity";

const COPILOT_PACKAGE = "@github/copilot";
const COPILOT_AGENT = "copilot-acp";

const runtime: RuntimeUpdate = {
  supported: true,
  update_mode: "pinned",
  package: COPILOT_PACKAGE,
  effective_version: "1.0.88",
  current_version: "1.0.70",
  default_version: "1.0.80",
};
const status: AgentUpdateStatus = {
  agent_name: COPILOT_AGENT,
  display_name: "Copilot",
  runtime_id: "npm:@github/copilot",
  owner: "kandev",
  mechanism: "npm_candidate",
  management: "managed",
  package: COPILOT_PACKAGE,
  effective_version: "1.0.89",
  current_version: "1.0.70",
  default_version: "1.0.80",
  update_mode: "pinned",
  available: true,
  enabled: true,
  auto_update: false,
  auto_update_supported: true,
  check_state: "unknown",
};

// @covers AC-AGENTS-RUNTIME-IDENTITY-001.1, AC-AGENTS-RUNTIME-IDENTITY-001.4
it("uses the selected managed package and version rather than an observed version", () => {
  expect(presentAgentRuntimeIdentity(COPILOT_AGENT, runtime, status)).toEqual({
    kind: "managed",
    package: COPILOT_PACKAGE,
    version: "1.0.89",
  });
});

// @covers AC-AGENTS-RUNTIME-IDENTITY-001.2, AC-AGENTS-RUNTIME-IDENTITY-001.4
it("keeps a native host separate from its managed fallback", () => {
  expect(
    presentAgentRuntimeIdentity("opencode-acp", runtime, {
      ...status,
      source: "native",
      owner: "external",
      mechanism: "native",
      management: "manual",
      managed_fallback: true,
    }),
  ).toEqual({ kind: "native" });
});
it("distinguishes harness-owned updating from managed npm", () => {
  expect(
    presentAgentRuntimeIdentity("omp-acp", { ...runtime, update_mode: "self_update" }),
  ).toEqual({ kind: "harness" });
});
it("identifies custom commands without treating them as managed packages", () => {
  expect(
    presentAgentRuntimeIdentity("custom", runtime, {
      ...status,
      mechanism: "custom",
      management: "unsupported",
    }),
  ).toEqual({ kind: "custom" });
});
it("uses unambiguous managed metadata before the status arrives", () => {
  expect(presentAgentRuntimeIdentity(COPILOT_AGENT, runtime)).toEqual({
    kind: "managed",
    package: COPILOT_PACKAGE,
    version: "1.0.88",
  });
});
it("leaves a missing managed version unknown rather than using the observed version", () => {
  expect(
    presentAgentRuntimeIdentity(COPILOT_AGENT, runtime, { ...status, effective_version: "" }),
  ).toEqual({ kind: "managed", package: COPILOT_PACKAGE, version: undefined });
});
describe("ambiguous or unsupported identity", () => {
  it.each([
    ["missing metadata", COPILOT_AGENT, undefined, undefined],
    [
      "native fallback without status",
      "claude-acp",
      { ...runtime, managed_fallback: true },
      undefined,
    ],
    ["OpenCode without selected source", "opencode-acp", runtime, undefined],
    ["disabled metadata", COPILOT_AGENT, { ...runtime, supported: false }, undefined],
    ["unknown mechanism", COPILOT_AGENT, runtime, { ...status, mechanism: "future" }],
    [
      "virtual runtime",
      "router",
      runtime,
      { ...status, mechanism: "virtual", owner: "none", management: "unsupported" },
    ],
    ["missing package", COPILOT_AGENT, runtime, { ...status, package: "" }],
  ] as const)("shows unknown for %s", (_label, name, metadata, runtimeStatus) => {
    expect(presentAgentRuntimeIdentity(name, metadata, runtimeStatus)).toEqual({ kind: "unknown" });
  });
});

it("recognizes harness-owned status even when its executable is native", () => {
  expect(
    presentAgentRuntimeIdentity("omp-acp", runtime, {
      ...status,
      update_mode: "self_update",
      owner: "external",
      mechanism: "native",
      management: "manual",
    }),
  ).toEqual({ kind: "harness" });
});

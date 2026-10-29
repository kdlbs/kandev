import { describe, expect, it } from "vitest";
import { buildSandboxPolicy, emptySandboxPolicyDraft } from "./sandbox-policy-draft";
import type { SandboxProvider } from "./types/sandbox";

const provider: SandboxProvider = {
  installation_id: "installation",
  provider_key: "generic",
  display_name: "Provider",
  version: "1",
  available: true,
};

describe("operator policy drafts", () => {
  it("starts with no implicit filesystem, network or credential grants", () => {
    const draft = { ...emptySandboxPolicyDraft(), name: " Restricted " };
    expect(buildSandboxPolicy("policy", provider, draft)).toEqual({
      id: "policy",
      workspace_id: "",
      name: "Restricted",
      installation_id: "installation",
      provider_key: "generic",
      sources: [],
      grants: { filesystem: [], network: [], environment_keys: [], kandev_mcp: false },
    });
  });
  it("records exact explicit grants without resolved runtime paths or environment values", () => {
    const draft = {
      ...emptySandboxPolicyDraft(),
      name: "Work",
      workDir: "write" as const,
      agentState: "write" as const,
      readPaths: "/opt/runtime\n\n/opt/tools",
      environmentKeys: "API_KEY\nAPI_KEY\nOTHER_KEY",
      mcp: true,
      network: [{ id: "row", host: "api.example.test", port: "443", protocol: "https" as const }],
    };
    expect(buildSandboxPolicy("policy", provider, draft)?.grants).toEqual({
      filesystem: [
        { reference: "work_dir", writable: true },
        { reference: "agent_state", writable: true },
        { path: "/opt/runtime", writable: false },
        { path: "/opt/tools", writable: false },
      ],
      environment_keys: ["API_KEY", "OTHER_KEY"],
      network: [{ host: "api.example.test", port: 443, protocol: "https" }],
      kandev_mcp: true,
    });
  });
  it("refuses secret assignments and ambiguous or unrestricted network input", () => {
    const draft = { ...emptySandboxPolicyDraft(), name: "Work" };
    expect(
      buildSandboxPolicy("policy", provider, { ...draft, environmentKeys: "API_KEY=private" }),
    ).toBeNull();
    for (const host of [
      "*",
      "*.example.test",
      "user:private@example.test",
      "https://api.example.test",
    ]) {
      expect(
        buildSandboxPolicy("policy", provider, {
          ...draft,
          network: [{ id: "row", host, port: "443", protocol: "https" }],
        }),
      ).toBeNull();
    }
    for (const port of ["", "0", "65536", "1e2", "443/path"]) {
      expect(
        buildSandboxPolicy("policy", provider, {
          ...draft,
          network: [{ id: "row", host: "api.example.test", port, protocol: "https" }],
        }),
      ).toBeNull();
    }
  });
});

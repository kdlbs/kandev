import type { SandboxGrants, SandboxPolicy, SandboxProvider } from "./types/sandbox";

export type SandboxAccessMode = "none" | "read" | "write";
export type SandboxPolicyDraft = {
  name: string;
  workDir: SandboxAccessMode;
  agentState: SandboxAccessMode;
  readPaths: string;
  writePaths: string;
  environmentKeys: string;
  mcp: boolean;
  network: { id: string; host: string; port: string; protocol: "http" | "https" | "tcp" }[];
};

export function emptySandboxPolicyDraft(): SandboxPolicyDraft {
  return {
    name: "",
    workDir: "none",
    agentState: "none",
    readPaths: "",
    writePaths: "",
    environmentKeys: "",
    mcp: false,
    network: [],
  };
}

function lines(value: string) {
  return [
    ...new Set(
      value
        .split("\n")
        .map((line) => line.trim())
        .filter(Boolean),
    ),
  ];
}

export function buildSandboxPolicy(
  id: string,
  provider: SandboxProvider,
  draft: SandboxPolicyDraft,
): SandboxPolicy | null {
  const name = draft.name.trim();
  const environment_keys = lines(draft.environmentKeys);
  if (
    !name ||
    name.length > 128 ||
    !provider.installation_id ||
    !provider.provider_key ||
    environment_keys.some((key) => !/^[A-Z_][A-Z0-9_]*$/.test(key))
  )
    return null;
  const network = draft.network.map((row) => ({
    host: row.host.trim(),
    port: Number(row.port),
    protocol: row.protocol,
  }));
  if (
    draft.network.some((row) => !/^\d+$/.test(row.port)) ||
    network.some((row) => !row.host || /[\s/*@]/.test(row.host) || row.port < 1 || row.port > 65535)
  )
    return null;
  const filesystem: SandboxGrants["filesystem"] = [];
  for (const [reference, access] of [
    ["work_dir", draft.workDir],
    ["agent_state", draft.agentState],
  ] as const) {
    if (access !== "none") filesystem.push({ reference, writable: access === "write" });
  }
  filesystem.push(
    ...lines(draft.readPaths).map((path) => ({ path, writable: false })),
    ...lines(draft.writePaths).map((path) => ({ path, writable: true })),
  );
  if (filesystem.length > 256 || network.length > 256) return null;
  return {
    id,
    name,
    workspace_id: "",
    installation_id: provider.installation_id,
    provider_key: provider.provider_key,
    sources: [],
    grants: { filesystem, network, environment_keys, kandev_mcp: draft.mcp },
  };
}

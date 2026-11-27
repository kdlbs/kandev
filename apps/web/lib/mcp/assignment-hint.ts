import type { MCPServerDefinition } from "@/lib/types/http-mcp";

const HINT_KEY = "mcpServerId";

export function readMCPAssignmentHint(search: string) {
  const values = new URLSearchParams(search).getAll(HINT_KEY);
  const definitionId = values.length === 1 && /^[a-zA-Z0-9_-]+$/.test(values[0]) ? values[0] : null;
  return { present: values.length > 0, definitionId };
}

export function appendMCPAssignmentHint(ownerRoute: string, definitionId: string) {
  const url = new URL(ownerRoute, "https://kandev.invalid");
  url.searchParams.set(HINT_KEY, definitionId);
  return `${url.pathname}${url.search}${url.hash}`;
}

export function removeMCPAssignmentHint(ownerRoute: string) {
  const url = new URL(ownerRoute, "https://kandev.invalid");
  url.searchParams.delete(HINT_KEY);
  return `${url.pathname}${url.search}${url.hash}`;
}

type AssignmentHintInput = {
  search: string;
  workspaceId: string;
  definitions: ReadonlyArray<Pick<MCPServerDefinition, "id" | "workspace_id" | "enabled">>;
  visibleDefinitionIds: readonly string[];
  ready: boolean;
  active: boolean;
};

type AssignmentHintResolution =
  | { status: "absent" | "pending" }
  | { status: "consume"; definitionId: string | null };

export function resolveMCPAssignmentHint({
  search,
  workspaceId,
  definitions,
  visibleDefinitionIds,
  ready,
  active,
}: AssignmentHintInput): AssignmentHintResolution {
  const hint = readMCPAssignmentHint(search);
  if (!hint.present) return { status: "absent" };
  if (!active || !ready) return { status: "pending" };
  const definition = definitions.find(
    (item) => item.id === hint.definitionId && item.workspace_id === workspaceId && item.enabled,
  );
  return {
    status: "consume",
    definitionId: definition && visibleDefinitionIds.includes(definition.id) ? definition.id : null,
  };
}

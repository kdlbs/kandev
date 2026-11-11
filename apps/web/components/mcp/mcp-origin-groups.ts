import type { MCPInheritedSelection, MCPSelectionOrigin } from "@/lib/types/http-mcp";

export type MCPOriginGroupScope = "task" | "repository" | "profile" | "unknown";
export type MCPOriginGroup = { scope: MCPOriginGroupScope; items: MCPInheritedSelection[] };

const SCOPES = ["task", "repository", "profile"] as const;

function uniqueOrigins(origins: MCPSelectionOrigin[]): MCPSelectionOrigin[] {
  const seen = new Set<string>();
  return origins.filter((origin) => {
    const key = JSON.stringify([origin.workspace_id, origin.scope, origin.owner_id]);
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export function groupMCPOrigins(inherited: MCPInheritedSelection[]): MCPOriginGroup[] {
  const definitions = new Map<string, MCPInheritedSelection>();
  for (const item of inherited) {
    const previous = definitions.get(item.definition.id);
    definitions.set(item.definition.id, {
      definition: previous?.definition ?? item.definition,
      origins: uniqueOrigins([...(previous?.origins ?? []), ...item.origins]),
    });
  }
  const items: Record<MCPOriginGroupScope, MCPInheritedSelection[]> = {
    task: [],
    repository: [],
    profile: [],
    unknown: [],
  };
  for (const item of definitions.values()) {
    const scope = SCOPES.find((candidate) =>
      item.origins.some((origin) => origin.scope === candidate),
    );
    items[scope ?? "unknown"].push(item);
  }
  return ([...SCOPES, "unknown"] as const)
    .map((scope) => ({ scope, items: items[scope] }))
    .filter((group) => group.items.length > 0);
}

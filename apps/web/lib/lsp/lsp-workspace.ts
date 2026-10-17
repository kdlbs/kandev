import { canonicalFileUri, filePathToUri } from "./file-uri";
import type { LspReadyWorkspace, ManagedLspConnection } from "./lsp-client-types";

export type WorkspaceMetadata = {
  uri: string;
  repositorySubpaths: Set<string>;
  sessionIds: Set<string>;
};

export function configureLspWorkspace(
  connection: ManagedLspConnection,
  workspace: LspReadyWorkspace,
): WorkspaceMetadata | null {
  connection.repositorySubpaths = new Set(workspace.repositorySubpaths.filter(Boolean));
  connection.workspaceUri = canonicalWorkspaceUri(workspace);
  if (!connection.workspaceUri) return null;
  return {
    uri: connection.workspaceUri,
    repositorySubpaths: new Set(connection.repositorySubpaths),
    sessionIds: new Set(connection.sessionRefCounts.keys()),
  };
}

function canonicalWorkspaceUri(workspace: LspReadyWorkspace): string | null {
  if (workspace.uri) {
    const canonicalUri = canonicalFileUri(workspace.uri);
    if (canonicalUri) return canonicalUri;
  }
  try {
    return workspace.path ? filePathToUri(workspace.path) : null;
  } catch {
    return null;
  }
}

export function lspWorkspaceFolders(
  workspaceUri: string | null,
  workspacePath: string | null,
): Array<{ uri: string; name: string }> | null {
  if (!workspaceUri) return null;
  return [
    {
      uri: workspaceUri,
      name: workspacePath?.split(/[\\/]/).filter(Boolean).at(-1) ?? "workspace",
    },
  ];
}

export function workspaceUriForSession(
  connections: Iterable<ManagedLspConnection>,
  workspaceMetadata: ReadonlyMap<string, WorkspaceMetadata>,
  sessionId: string,
): string | null {
  for (const connection of connections) {
    if (connection.sessionRefCounts.has(sessionId) && connection.workspaceUri) {
      return connection.workspaceUri;
    }
  }
  for (const workspace of workspaceMetadata.values()) {
    if (workspace.sessionIds.has(sessionId)) return workspace.uri;
  }
  return null;
}

export function repositorySubpathsForSession(
  connections: Iterable<ManagedLspConnection>,
  workspaceMetadata: ReadonlyMap<string, WorkspaceMetadata>,
  sessionId: string,
): string[] {
  const repositories = new Set<string>();
  for (const connection of connections) {
    if (!connection.sessionRefCounts.has(sessionId)) continue;
    for (const repository of connection.repositorySubpaths) repositories.add(repository);
  }
  for (const workspace of workspaceMetadata.values()) {
    if (!workspace.sessionIds.has(sessionId)) continue;
    for (const repository of workspace.repositorySubpaths) repositories.add(repository);
  }
  return [...repositories];
}

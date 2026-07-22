import { canonicalFileUri, filePathToUri } from "./file-uri";
import type { LspReadyWorkspace, ManagedLspConnection } from "./lsp-client-types";

export type WorkspaceMetadata = {
  uri: string;
  repositorySubpaths: Set<string>;
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

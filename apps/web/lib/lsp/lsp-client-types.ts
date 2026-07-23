import type { LSPConnection, LspRange } from "./lsp-json-rpc";

export type PublishDiagnosticsParams = {
  uri: string;
  diagnostics: Array<{
    range: LspRange;
    message: string;
    severity?: number;
    source?: string;
    code?: unknown;
  }>;
};

export type ManagedLspConnection = LSPConnection & {
  key: string;
  sessionId: string;
  ownerId: string;
  diagnosticsByUri: Map<string, PublishDiagnosticsParams>;
};

export type OpenDocumentParams = {
  uri: string;
  languageId: string;
  text: string;
  repo?: string;
};

export type LspReadyWorkspace = {
  path: string | null;
  uri: string | null;
  repositorySubpaths: string[];
};

export function createManagedLspConnection(
  key: string,
  sessionId: string,
  generation: number,
  ws: WebSocket,
): ManagedLspConnection {
  return {
    key,
    sessionId,
    ownerId: `${key}:${generation}`,
    ws,
    rpc: null,
    initialized: false,
    refCount: 1,
    idleTimer: null,
    openDocuments: new Map(),
    diagnosticsByUri: new Map(),
    providerDisposables: [],
    serverCapabilities: null,
    workspaceUri: null,
    repositorySubpaths: new Set(),
  };
}

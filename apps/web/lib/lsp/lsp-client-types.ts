import type { LSPConnection, LspRange } from "./lsp-json-rpc";
import {
  EMPTY_LSP_PROGRESS,
  type LspProgressSnapshot,
  type LspProgressToken,
} from "./lsp-progress";

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
  taskId: string;
  sessionId: string;
  sessionRefCounts: Map<string, number>;
  ownerId: string;
  configuration: Record<string, unknown>;
  protocolInitialized: boolean;
  diagnosticsByUri: Map<string, PublishDiagnosticsParams>;
  progress: LspProgressSnapshot;
  registeredProgressTokens: Set<LspProgressToken>;
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

type ManagedLspConnectionOptions = {
  key: string;
  taskId: string;
  sessionId: string;
  generation: number;
  ws: WebSocket;
  configuration: Record<string, unknown>;
};

export function createManagedLspConnection({
  key,
  taskId,
  sessionId,
  generation,
  ws,
  configuration,
}: ManagedLspConnectionOptions): ManagedLspConnection {
  return {
    key,
    taskId,
    sessionId,
    sessionRefCounts: new Map([[sessionId, 1]]),
    ownerId: `${key}:${generation}`,
    configuration,
    protocolInitialized: false,
    ws,
    rpc: null,
    initialized: false,
    refCount: 1,
    openDocuments: new Map(),
    diagnosticsByUri: new Map(),
    progress: EMPTY_LSP_PROGRESS,
    registeredProgressTokens: new Set(),
    providerDisposables: [],
    serverCapabilities: null,
    workspaceUri: null,
    repositorySubpaths: new Set(),
  };
}

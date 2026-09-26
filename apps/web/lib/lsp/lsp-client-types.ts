import type { LSPConnection, LspRange } from "./lsp-json-rpc";
import type { LspDynamicRegistration } from "./lsp-dynamic-capabilities";
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
  sessionId: string;
  ownerId: string;
  configuration: Record<string, unknown>;
  protocolInitialized: boolean;
  diagnosticsByUri: Map<string, PublishDiagnosticsParams>;
  closedDocuments: Set<string>;
  progress: LspProgressSnapshot;
  registeredProgressTokens: Set<LspProgressToken>;
  continuityEnabled: boolean;
  leaseId: string | null;
  transportGeneration: number;
  reconnectAttempts: number;
  reconnectTimer: ReturnType<typeof setTimeout> | null;
  reconnecting: boolean;
  explicitlyStopped: boolean;
  releaseAfterConnect: "stop" | "editor_idle" | null;
  diagnosticsReady: boolean;
  documentsSynced: boolean;
  providersReady: boolean;
  dynamicRegistrations: Map<string, LspDynamicRegistration>;
  semanticRefreshCallbacks: (() => void)[];
  lspLanguage: string;
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

export type ManagedLspConnectionOptions = {
  key: string;
  sessionId: string;
  generation: number;
  ws: WebSocket;
  configuration: Record<string, unknown>;
  continuityEnabled?: boolean;
  leaseId?: string | null;
  lspLanguage?: string;
};

export function createManagedLspConnection(
  options: ManagedLspConnectionOptions,
): ManagedLspConnection {
  const {
    key,
    sessionId,
    generation,
    ws,
    configuration,
    continuityEnabled = false,
    leaseId = null,
    lspLanguage = "",
  } = options;
  return {
    key,
    sessionId,
    ownerId: `${key}:${generation}`,
    configuration,
    protocolInitialized: false,
    ws,
    rpc: null,
    initialized: false,
    refCount: 1,
    idleTimer: null,
    openDocuments: new Map(),
    diagnosticsByUri: new Map(),
    closedDocuments: new Set(),
    progress: EMPTY_LSP_PROGRESS,
    registeredProgressTokens: new Set(),
    continuityEnabled,
    leaseId,
    transportGeneration: 0,
    reconnectAttempts: 0,
    reconnectTimer: null,
    reconnecting: false,
    explicitlyStopped: false,
    releaseAfterConnect: null,
    diagnosticsReady: false,
    documentsSynced: false,
    providersReady: false,
    dynamicRegistrations: new Map(),
    semanticRefreshCallbacks: [],
    lspLanguage,
    providerDisposables: [],
    serverCapabilities: null,
    workspaceUri: null,
    repositorySubpaths: new Set(),
  };
}

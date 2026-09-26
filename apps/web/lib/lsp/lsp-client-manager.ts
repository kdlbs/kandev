import { canonicalFileUri, joinFileUri } from "./file-uri";
import type { LspStatus } from "./lsp-json-rpc";
import {
  createManagedLspConnection,
  type ManagedLspConnection,
  type OpenDocumentParams,
} from "./lsp-client-types";
import { LspClientEditorState } from "./lsp-client-editor-state";
import {
  repositorySubpathsForSession,
  workspaceUriForSession,
  type WorkspaceMetadata,
} from "./lsp-workspace";
import { EMPTY_LSP_PROGRESS, type LspProgressSnapshot } from "./lsp-progress";
import {
  clearLspLeaseHint,
  clearLspEnabledState,
  getLspLeaseHint,
  isLspEnabledInStorage,
  saveLspEnabledState,
} from "./lsp-client-storage";
import { DISABLED_LSP_STATUS, LSP_DEFAULT_CONFIGS, LSP_IDLE_TIMEOUT } from "./lsp-client-config";
import { buildDocumentContentChanges, buildDocumentSaveParams } from "./lsp-document-sync";
import { LspClientProtocol } from "./lsp-client-protocol";
import { LspClientTransport } from "./lsp-client-transport";

export type { LspStatus } from "./lsp-json-rpc";
export { toLspLanguage } from "./lsp-json-rpc";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type ChangeListener = (key: string) => void;
type FileOpener = (uri: string, line?: number, column?: number) => boolean | Promise<boolean>;

function hasActiveLspWork(progress: LspProgressSnapshot): boolean {
  return progress.initializingSince !== null || progress.active.length > 0;
}

function configurationForLanguage(
  lspLanguage: string,
  userConfigs?: Record<string, Record<string, unknown>>,
): Record<string, unknown> {
  return {
    ...(LSP_DEFAULT_CONFIGS[lspLanguage] ?? {}),
    ...(userConfigs?.[lspLanguage] ?? {}),
  };
}

function configurationsMatch(
  current: Record<string, unknown>,
  next: Record<string, unknown>,
): boolean {
  return JSON.stringify(current) === JSON.stringify(next);
}

class LSPClientManager {
  private connections = new Map<string, ManagedLspConnection>();
  private connectionGeneration = 0;
  private statuses = new Map<string, LspStatus>();
  /** Keeps Monaco model identity stable after an LSP connection stops or crashes. */
  private workspaceMetadata = new Map<string, WorkspaceMetadata>();
  private listeners = new Set<ChangeListener>();
  private fileOpener: FileOpener | null = null;
  private editorState = new LspClientEditorState((connection) =>
    this.isCurrentConnection(connection),
  );
  private protocol = new LspClientProtocol({
    editorState: this.editorState,
    workspaceMetadata: this.workspaceMetadata,
    isCurrentConnection: (conn) => this.isCurrentConnection(conn),
    isCurrentTransportConnection: (conn, generation) =>
      this.isCurrentTransportConnection(conn, generation),
    setStatus: (key, status) => this.setStatus(key, status),
    handleProgressChange: (conn) => this.handleProgressChange(conn),
    cleanupConnection: (conn) => this.cleanupConnection(conn),
  });
  private transport = new LspClientTransport({
    protocol: this.protocol,
    editorState: this.editorState,
    isCurrentConnection: (conn) => this.isCurrentConnection(conn),
    getStatus: (key) => this.statuses.get(key) ?? DISABLED_LSP_STATUS,
    setStatus: (key, status) => this.setStatus(key, status),
    removeStatus: (key) => this.statuses.delete(key),
    notifyChange: (key) => this.notifyChange(key),
    cleanupConnection: (conn) => this.cleanupConnection(conn),
  });
  setFileOpener(opener: FileOpener | null): void {
    this.fileOpener = opener;
  }

  getFileOpener(): FileOpener | null {
    return this.fileOpener;
  }

  // ---- localStorage persistence for manual LSP toggle ----

  /** Save that LSP was manually enabled for this session+language. */
  saveEnabledState(sessionId: string, language: string): void {
    saveLspEnabledState(sessionId, language);
    this.notifyChange(`${sessionId}:${language}`);
  }

  /** Clear the saved LSP state (manual stop). */
  clearEnabledState(sessionId: string, language: string): void {
    clearLspEnabledState(sessionId, language);
    this.notifyChange(`${sessionId}:${language}`);
  }

  /** Check if LSP was previously enabled for this session+language. */
  isEnabledInStorage(sessionId: string, language: string): boolean {
    return isLspEnabledInStorage(sessionId, language);
  }

  hasLeaseHint(sessionId: string, language: string): boolean {
    return getLspLeaseHint(sessionId, language) !== null;
  }

  getStatus(sessionId: string, lspLanguage: string): LspStatus {
    const key = `${sessionId}:${lspLanguage}`;
    return this.statuses.get(key) ?? DISABLED_LSP_STATUS;
  }

  getProgress(sessionId: string, lspLanguage: string): LspProgressSnapshot {
    const key = `${sessionId}:${lspLanguage}`;
    return this.connections.get(key)?.progress ?? EMPTY_LSP_PROGRESS;
  }

  getWorkspaceUriForSession(sessionId: string): string | null {
    return workspaceUriForSession(this.connections.values(), this.workspaceMetadata, sessionId);
  }

  getRepositorySubpaths(sessionId: string): string[] {
    return repositorySubpathsForSession(
      this.connections.values(),
      this.workspaceMetadata,
      sessionId,
    );
  }

  onChange(listener: ChangeListener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private notifyChange(key: string): void {
    for (const listener of this.listeners) listener(key);
  }

  private setStatus(key: string, status: LspStatus) {
    this.statuses.set(key, status);
    this.notifyChange(key);
  }

  // ------- Connection lifecycle -------

  connect(
    sessionId: string,
    lspLanguage: string,
    userConfigs?: Record<string, Record<string, unknown>>,
    continuityEnabled = false,
  ): () => void {
    const key = `${sessionId}:${lspLanguage}`;
    const configuration = configurationForLanguage(lspLanguage, userConfigs);

    const existing = this.connections.get(key);
    if (
      existing &&
      existing.continuityEnabled === continuityEnabled &&
      (existing.ws.readyState <= WebSocket.OPEN || existing.reconnecting)
    ) {
      return this.acquireConnection(existing, configuration);
    }
    if (existing) this.cleanupConnection(existing);

    if (!continuityEnabled) clearLspLeaseHint(sessionId, lspLanguage);
    const leaseId = continuityEnabled ? getLspLeaseHint(sessionId, lspLanguage) : null;
    const ws = this.transport.createSocket(sessionId, lspLanguage, continuityEnabled, leaseId);
    const conn = createManagedLspConnection({
      key,
      sessionId,
      generation: ++this.connectionGeneration,
      ws,
      configuration,
      continuityEnabled,
      leaseId,
      lspLanguage,
    });
    this.connections.set(key, conn);
    this.setStatus(key, { state: "connecting" });

    this.transport.bind(conn, ws);
    return () => this.decrementRef(conn);
  }

  private acquireConnection(
    conn: ManagedLspConnection,
    configuration: Record<string, unknown>,
  ): () => void {
    this.updateConfiguration(conn, configuration);
    conn.refCount++;
    this.clearIdleTimer(conn);
    return () => this.decrementRef(conn);
  }

  private updateConfiguration(
    conn: ManagedLspConnection,
    configuration: Record<string, unknown>,
  ): void {
    if (configurationsMatch(conn.configuration, configuration)) return;
    conn.configuration = configuration;
    if (!conn.protocolInitialized || !conn.rpc) return;
    conn.rpc.sendNotification("workspace/didChangeConfiguration", {
      settings: configuration,
    });
  }

  /** Dispose a placeholder model (e.g. when the file is opened in a real tab). */
  disposePlaceholderModel(modelUri: string): void {
    this.editorState.disposePlaceholderModel(modelUri);
  }

  // ------- Document synchronization -------

  openDocument(sessionId: string, lspLanguage: string, document: OpenDocumentParams): void {
    const key = `${sessionId}:${lspLanguage}`;
    const conn = this.connections.get(key);
    if (!conn || (!conn.initialized && !conn.reconnecting)) return;
    const documentUri = canonicalFileUri(document.uri);
    if (!documentUri) return;
    conn.closedDocuments.delete(documentUri);
    this.promoteDocumentModel(sessionId, documentUri, document.text);
    const existing = conn.openDocuments.get(documentUri);
    if (existing) {
      if (existing.pendingClose) {
        existing.pendingClose = false;
        existing.refCount = 1;
        if (existing.text !== document.text) {
          existing.text = document.text;
          existing.version++;
        }
        if (conn.rpc && conn.documentsSynced) {
          conn.rpc.sendNotification("textDocument/didOpen", {
            textDocument: {
              uri: documentUri,
              languageId: existing.languageId,
              version: existing.version,
              text: existing.text,
            },
          });
        }
        if (document.repo) conn.repositorySubpaths.add(document.repo);
        return;
      }
      existing.refCount++;
      if (document.repo) conn.repositorySubpaths.add(document.repo);
      return;
    }

    if (document.repo) conn.repositorySubpaths.add(document.repo);
    conn.openDocuments.set(documentUri, {
      version: 1,
      languageId: document.languageId,
      refCount: 1,
      text: document.text,
      pendingClose: false,
    });
    if (!conn.documentsSynced || !conn.rpc) return;
    conn.rpc.sendNotification("textDocument/didOpen", {
      textDocument: {
        uri: documentUri,
        languageId: document.languageId,
        version: 1,
        text: document.text,
      },
    });
  }

  /** Transfer a placeholder model to a real file editor, regardless of LSP language/status. */
  promoteDocumentModel(sessionId: string, documentUri: string, text: string): void {
    this.editorState.promoteDocumentModel(sessionId, documentUri, text);
  }

  changeDocument(sessionId: string, lspLanguage: string, documentUri: string, text: string): void {
    const key = `${sessionId}:${lspLanguage}`;
    const conn = this.connections.get(key);
    if (!conn || (!conn.initialized && !conn.reconnecting)) return;
    const canonicalUri = canonicalFileUri(documentUri);
    if (!canonicalUri) return;
    if (!conn.documentsSynced || !conn.rpc) {
      const document = conn.openDocuments.get(canonicalUri);
      if (document && document.text !== text) {
        document.text = text;
        document.version++;
      }
      return;
    }
    this.synchronizeOpenDocument(conn, canonicalUri, text);
  }

  saveDocument(
    sessionId: string,
    documentPath: string,
    repo: string | undefined,
    persistedText: string,
    liveText = persistedText,
  ): void {
    for (const conn of this.connections.values()) {
      if (
        conn.sessionId !== sessionId ||
        !conn.initialized ||
        !conn.documentsSynced ||
        !conn.rpc ||
        !conn.workspaceUri
      ) {
        continue;
      }

      let documentUri: string;
      try {
        documentUri = joinFileUri(conn.workspaceUri, repo, documentPath);
      } catch {
        continue;
      }
      if (!conn.openDocuments.has(documentUri)) continue;

      this.synchronizeOpenDocument(conn, documentUri, liveText);
      // If editing continued while persistence was in flight, including the
      // older saved snapshot could rewind servers that treat didSave.text as
      // their current document. The text field is optional, so omit it for
      // this raced save while keeping the open document on the newest buffer.
      const savedText = liveText === persistedText ? persistedText : undefined;
      const params = buildDocumentSaveParams(conn.serverCapabilities, documentUri, savedText);
      if (params) conn.rpc.sendNotification("textDocument/didSave", params);
    }
  }

  private synchronizeOpenDocument(
    conn: ManagedLspConnection,
    documentUri: string,
    text: string,
  ): void {
    if (!conn.rpc) return;
    const document = conn.openDocuments.get(documentUri);
    if (!document || document.text === text) return;

    const contentChanges = buildDocumentContentChanges(
      conn.serverCapabilities,
      document.text,
      text,
    );
    document.text = text;
    if (contentChanges.length === 0) return;
    document.version++;
    conn.rpc.sendNotification("textDocument/didChange", {
      textDocument: { uri: documentUri, version: document.version },
      contentChanges,
    });
  }

  closeDocument(sessionId: string, lspLanguage: string, documentUri: string): void {
    const key = `${sessionId}:${lspLanguage}`;
    const conn = this.connections.get(key);
    if (!conn || (!conn.initialized && !conn.reconnecting)) return;
    const canonicalUri = canonicalFileUri(documentUri);
    if (!canonicalUri) return;
    const document = conn.openDocuments.get(canonicalUri);
    if (!document) return;
    document.refCount = Math.max(0, document.refCount - 1);
    if (document.refCount > 0) return;
    this.editorState.clearDocumentDiagnostics(conn, canonicalUri);
    conn.closedDocuments.add(canonicalUri);

    if (conn.continuityEnabled && conn.reconnecting && !conn.documentsSynced) {
      document.pendingClose = true;
      return;
    }

    conn.openDocuments.delete(canonicalUri);
    if (!conn.documentsSynced || !conn.rpc) return;
    conn.rpc.sendNotification("textDocument/didClose", {
      textDocument: { uri: canonicalUri },
    });
  }

  // ------- Stop / cleanup -------

  stop(sessionId: string, lspLanguage: string): void {
    const key = `${sessionId}:${lspLanguage}`;
    const conn = this.connections.get(key);
    if (!conn) {
      this.statuses.delete(key);
      this.setStatus(key, { state: "disabled" });
      return;
    }

    this.setStatus(key, { state: "stopping" });
    if (conn.idleTimer) clearTimeout(conn.idleTimer);

    if (conn.continuityEnabled) {
      conn.explicitlyStopped = true;
      conn.releaseAfterConnect = "stop";
      if (conn.reconnectTimer) {
        clearTimeout(conn.reconnectTimer);
        conn.reconnectTimer = null;
      }
      clearLspLeaseHint(sessionId, lspLanguage);
      if (conn.ws.readyState === WebSocket.OPEN) {
        void this.transport.release(conn, "stop");
      } else if (conn.ws.readyState === WebSocket.CLOSED) {
        conn.reconnecting = true;
        this.transport.open(conn);
      }
      return;
    }

    // Send shutdown/exit before closing
    if (conn.rpc && conn.initialized) {
      try {
        conn.rpc
          .sendRequest("shutdown", null)
          .then(() => {
            conn.rpc?.sendNotification("exit", null);
          })
          .catch(() => {});
      } catch {
        // ignore
      }
    }

    this.cleanupConnection(conn);
    this.statuses.delete(key);
    this.notifyChange(key);
  }

  disconnectAll(): void {
    for (const conn of this.connections.values()) {
      if (conn.idleTimer) clearTimeout(conn.idleTimer);
      this.cleanupConnection(conn);
    }
    this.statuses.clear();
    this.workspaceMetadata.clear();
  }

  private decrementRef(conn: ManagedLspConnection) {
    if (!this.isCurrentConnection(conn)) return;
    conn.refCount--;
    if (conn.refCount <= 0) this.scheduleIdleCleanup(conn);
  }

  private handleProgressChange(conn: ManagedLspConnection): void {
    if (!this.isCurrentConnection(conn)) return;
    this.notifyChange(conn.key);
    if (conn.refCount > 0) return;
    if (hasActiveLspWork(conn.progress)) {
      this.clearIdleTimer(conn);
      return;
    }
    this.scheduleIdleCleanup(conn);
  }

  private scheduleIdleCleanup(conn: ManagedLspConnection): void {
    if (
      !this.isCurrentConnection(conn) ||
      conn.refCount > 0 ||
      hasActiveLspWork(conn.progress) ||
      conn.idleTimer
    ) {
      return;
    }
    conn.idleTimer = setTimeout(() => {
      conn.idleTimer = null;
      if (!this.isCurrentConnection(conn) || conn.refCount > 0 || hasActiveLspWork(conn.progress)) {
        return;
      }
      if (conn.continuityEnabled) {
        conn.releaseAfterConnect = "editor_idle";
        if (conn.ws.readyState === WebSocket.OPEN) {
          void this.transport.release(conn, "editor_idle");
        } else if (conn.ws.readyState === WebSocket.CLOSED) {
          if (conn.reconnectTimer) {
            clearTimeout(conn.reconnectTimer);
            conn.reconnectTimer = null;
          }
          conn.reconnecting = true;
          this.transport.open(conn);
        }
        return;
      }
      this.cleanupConnection(conn);
      this.statuses.delete(conn.key);
      this.notifyChange(conn.key);
    }, LSP_IDLE_TIMEOUT);
  }

  private clearIdleTimer(conn: ManagedLspConnection): void {
    if (!conn.idleTimer) return;
    clearTimeout(conn.idleTimer);
    conn.idleTimer = null;
  }

  private isCurrentConnection(conn: ManagedLspConnection): boolean {
    return this.connections.get(conn.key) === conn;
  }

  private isCurrentTransportConnection(conn: ManagedLspConnection, generation: number): boolean {
    return this.isCurrentConnection(conn) && conn.transportGeneration === generation;
  }

  private cleanupConnection(conn: ManagedLspConnection) {
    this.clearIdleTimer(conn);
    if (conn.reconnectTimer) {
      clearTimeout(conn.reconnectTimer);
      conn.reconnectTimer = null;
    }
    for (const d of conn.providerDisposables) d.dispose();
    conn.providerDisposables = [];
    conn.rpc?.dispose();
    conn.rpc = null;
    conn.initialized = false;
    conn.protocolInitialized = false;
    conn.documentsSynced = false;
    conn.openDocuments.clear();
    conn.diagnosticsByUri.clear();
    conn.providersReady = false;
    conn.diagnosticsReady = false;
    conn.reconnecting = false;
    conn.progress = EMPTY_LSP_PROGRESS;
    conn.registeredProgressTokens.clear();
    try {
      if (conn.ws.readyState <= WebSocket.OPEN) {
        conn.ws.close();
      }
    } catch {
      // ignore
    }
    if (this.isCurrentConnection(conn)) this.connections.delete(conn.key);
    this.editorState.disposeConnection(conn.ownerId);
  }
}

export const lspClientManager = new LSPClientManager();

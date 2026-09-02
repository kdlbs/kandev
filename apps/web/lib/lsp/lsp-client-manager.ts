import type { editor as monacoEditor, IDisposable } from "monaco-editor";
import { getMonacoInstance, waitForMonacoInstance } from "@/components/editors/monaco/monaco-init";
import {
  registerBuiltinTsSuppression,
  withLspProviderRegistration,
} from "@/components/editors/monaco/builtin-providers";
import { t } from "@/lib/i18n";
import { registerLspProviders } from "./lsp-providers";
import { canonicalFileUri, joinFileUri } from "./file-uri";
import { JsonRpcConnection, getWsBaseUrl, CLOSE_CODE_STATUS } from "./lsp-json-rpc";
import type { LspStatus } from "./lsp-json-rpc";
import {
  createManagedLspConnection,
  type LspReadyWorkspace,
  type ManagedLspConnection,
  type OpenDocumentParams,
  type PublishDiagnosticsParams,
} from "./lsp-client-types";
import { connectionDocumentUri, connectionModelUri } from "./lsp-editor-models";
import { LspClientEditorState } from "./lsp-client-editor-state";
import {
  configureLspWorkspace,
  repositorySubpathsForSession,
  workspaceUriForSession,
  type WorkspaceMetadata,
} from "./lsp-workspace";
import { EMPTY_LSP_PROGRESS, type LspProgressSnapshot } from "./lsp-progress";
import { DISABLED_LSP_STATUS } from "./lsp-client-config";
import { LSP_DEFAULT_CONFIGS } from "./lsp-client-config";
import { buildDocumentContentChanges, buildDocumentSaveParams } from "./lsp-document-sync";
import { getLspMonacoProviderMethods } from "./lsp-provider-capabilities";

export type { LspStatus } from "./lsp-json-rpc";
export { toLspLanguage } from "./lsp-json-rpc";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type ChangeListener = (key: string) => void;
type FileOpener = (uri: string, line?: number, column?: number) => boolean | Promise<boolean>;

const ATTACHMENT_RETRY_DELAYS_MS = [1_000, 2_000, 5_000] as const;

type LspLeaseSet = {
  key: string;
  taskId: string;
  lspLanguage: string;
  sessionRefCounts: Map<string, number>;
  configuration: Record<string, unknown>;
  retryIndex: number;
  retryTimer: ReturnType<typeof setTimeout> | null;
};

function lspErrorMessage(error: unknown): string {
  if (error instanceof Error) return error.message || String(error);
  if (typeof error === "object" && error !== null) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message) return message;
  }
  return String(error);
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

function registerTypeScriptModelSuppression(
  connection: ManagedLspConnection,
  lspLanguage: string,
  serverCapabilities: Record<string, unknown> | null,
): void {
  if (lspLanguage !== "typescript") return;
  connection.providerDisposables.push(
    registerBuiltinTsSuppression(
      connection.ownerId,
      (model) => connectionDocumentUri(model as monacoEditor.ITextModel, connection) !== null,
      getLspMonacoProviderMethods(serverCapabilities),
    ),
  );
}

class LSPClientManager {
  private connections = new Map<string, ManagedLspConnection>();
  /** Editor demand survives disposable browser transport generations. */
  private leaseSets = new Map<string, LspLeaseSet>();
  private connectionGeneration = 0;
  private statuses = new Map<string, LspStatus>();
  /** Keeps Monaco model identity stable after an LSP connection stops or crashes. */
  private workspaceMetadata = new Map<string, WorkspaceMetadata>();
  private listeners = new Set<ChangeListener>();
  private fileOpener: FileOpener | null = null;
  private editorState = new LspClientEditorState((connection) =>
    this.isCurrentConnection(connection),
  );
  setFileOpener(opener: FileOpener | null): void {
    this.fileOpener = opener;
  }

  getFileOpener(): FileOpener | null {
    return this.fileOpener;
  }

  getStatus(taskId: string, lspLanguage: string): LspStatus {
    const key = `${taskId}:${lspLanguage}`;
    return this.statuses.get(key) ?? DISABLED_LSP_STATUS;
  }

  getProgress(taskId: string, lspLanguage: string): LspProgressSnapshot {
    const key = `${taskId}:${lspLanguage}`;
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
    taskId: string,
    sessionId: string,
    lspLanguage: string,
    userConfigs?: Record<string, Record<string, unknown>>,
  ): () => void {
    const key = `${taskId}:${lspLanguage}`;
    const configuration = configurationForLanguage(lspLanguage, userConfigs);

    let leases = this.leaseSets.get(key);
    if (!leases) {
      leases = {
        key,
        taskId,
        lspLanguage,
        sessionRefCounts: new Map(),
        configuration,
        retryIndex: 0,
        retryTimer: null,
      };
      this.leaseSets.set(key, leases);
    }
    leases.configuration = configuration;
    leases.sessionRefCounts.set(sessionId, (leases.sessionRefCounts.get(sessionId) ?? 0) + 1);
    this.workspaceMetadata.get(key)?.sessionIds.add(sessionId);
    const existing = this.connections.get(key);
    if (existing && existing.ws.readyState <= WebSocket.OPEN) {
      this.updateConfiguration(existing, configuration);
    } else {
      if (existing) this.cleanupConnection(existing);
      this.clearReconnectTimer(leases);
      this.createConnection(leases, sessionId);
    }
    let released = false;
    return () => {
      if (released) return;
      released = true;
      this.releaseLease(leases, sessionId);
    };
  }

  private createConnection(leases: LspLeaseSet, preferredSessionId?: string): void {
    if (this.leaseSets.get(leases.key) !== leases || leases.sessionRefCounts.size === 0) return;
    const existing = this.connections.get(leases.key);
    if (existing && existing.ws.readyState <= WebSocket.OPEN) return;
    if (existing) this.cleanupConnection(existing);

    const { key, taskId, lspLanguage } = leases;
    const sessionId =
      (preferredSessionId && leases.sessionRefCounts.has(preferredSessionId)
        ? preferredSessionId
        : leases.sessionRefCounts.keys().next().value) ?? "";

    const wsUrl = `${getWsBaseUrl()}/lsp/tasks/${encodeURIComponent(taskId)}/${encodeURIComponent(lspLanguage)}/attach`;
    const ws = new WebSocket(wsUrl);

    const conn = createManagedLspConnection({
      key,
      taskId,
      sessionId,
      generation: ++this.connectionGeneration,
      ws,
      configuration: leases.configuration,
      sessionRefCounts: leases.sessionRefCounts,
    });
    this.connections.set(key, conn);
    this.setStatus(key, { state: "connecting" });

    let attachmentReady = false;

    ws.onopen = () => {
      if (!this.isCurrentConnection(conn)) return;
      this.setStatus(key, { state: "connecting" });
    };

    // The task host already owns initialize and process lifecycle. The first
    // attachment frame is a capability/workspace handshake, not JSON-RPC.
    const statusHandler = (event: MessageEvent) => {
      if (attachmentReady || !this.isCurrentConnection(conn)) return;

      let data: {
        status?: string;
        error?: string;
        language?: string;
        generation?: number;
        workspaceUri?: string;
        workspaceFolders?: Array<{ uri: string; name: string }>;
        serverCapabilities?: Record<string, unknown>;
      };
      try {
        data = JSON.parse(event.data as string);
      } catch {
        return;
      }

      if (data.status !== "attached") return;
      if (data.language !== lspLanguage) {
        this.setStatus(key, { state: "error", reason: t("lsp:connectionClosed") });
        ws.close();
        return;
      }
      ws.removeEventListener("message", statusHandler);
      attachmentReady = true;
      this.setStatus(key, { state: "starting" });
      void this.activateAttachment(conn, lspLanguage, data);
    };
    ws.addEventListener("message", statusHandler);

    ws.onclose = (event) => {
      ws.removeEventListener("message", statusHandler);
      const wasCurrent = this.isCurrentConnection(conn);
      this.cleanupConnection(conn);
      if (!wasCurrent) return;

      const statusFactory = CLOSE_CODE_STATUS[event.code];
      if (statusFactory) {
        const status = statusFactory(event.reason);
        this.setStatus(key, status);
        if (status.state === "error") this.scheduleReconnect(leases);
        else this.clearReconnectTimer(leases);
      } else {
        this.setStatus(key, { state: "error", reason: event.reason || t("lsp:connectionClosed") });
        this.scheduleReconnect(leases);
      }
    };

    ws.onerror = () => {
      if (!this.isCurrentConnection(conn)) return;
      const current = this.statuses.get(key);
      if (current?.state !== "error" && current?.state !== "unavailable") {
        this.setStatus(key, { state: "error", reason: t("lsp:webSocketError") });
        this.scheduleReconnect(leases);
      }
    };
  }

  private updateConfiguration(
    conn: ManagedLspConnection,
    configuration: Record<string, unknown>,
  ): void {
    if (configurationsMatch(conn.configuration, configuration)) return;
    conn.configuration = configuration;
    // Effective server configuration belongs to the task controller. Keep the
    // latest browser value only for provider-side compatibility; never mutate
    // the shared server from an editor lease.
  }

  private async activateAttachment(
    conn: ManagedLspConnection,
    lspLanguage: string,
    handshake: {
      workspaceUri?: string;
      workspaceFolders?: Array<{ uri: string; name: string }>;
      serverCapabilities?: Record<string, unknown>;
    },
  ) {
    if (!this.isCurrentConnection(conn)) return;
    const { key, ws } = conn;

    const workspace: LspReadyWorkspace = {
      path: null,
      uri: handshake.workspaceUri ?? null,
      repositorySubpaths: (handshake.workspaceFolders ?? []).map((folder) => folder.name),
    };
    const workspaceMetadata = configureLspWorkspace(conn, workspace);
    if (workspaceMetadata) this.workspaceMetadata.set(conn.key, workspaceMetadata);

    try {
      const rpc = new JsonRpcConnection(ws);
      rpc.listen();
      conn.rpc = rpc;
      conn.serverCapabilities = handshake.serverCapabilities ?? null;
      conn.protocolInitialized = true;

      // Register diagnostics handler
      rpc.onNotification("textDocument/publishDiagnostics", (params) => {
        if (!this.isCurrentConnection(conn)) return;
        this.editorState.handleDiagnostics(conn, params as PublishDiagnosticsParams);
      });

      // Collect callbacks for semantic token refresh
      const semanticRefreshCallbacks: (() => void)[] = [];
      rpc.onRequest("workspace/semanticTokens/refresh", () => {
        for (const cb of semanticRefreshCallbacks) cb();
        return null;
      });

      // Monaco loads asynchronously. Do not expose a ready connection until its
      // providers can be registered; otherwise early diagnostics are dropped.
      const monaco = await waitForMonacoInstance();
      if (!this.isCurrentConnection(conn)) {
        this.cleanupConnection(conn);
        return;
      }

      conn.providerDisposables.push(
        monaco.editor.onDidCreateModel((model: monacoEditor.ITextModel) => {
          if (this.isCurrentConnection(conn)) {
            this.editorState.applyCachedDiagnostics(conn, model);
          }
        }),
      );
      for (const model of monaco.editor.getModels()) {
        this.editorState.applyCachedDiagnostics(conn, model);
      }

      registerTypeScriptModelSuppression(conn, lspLanguage, conn.serverCapabilities);

      // Register Monaco providers for this language.
      conn.providerDisposables.push(
        ...withLspProviderRegistration(() =>
          this.registerProviders(
            rpc,
            lspLanguage,
            conn,
            conn.serverCapabilities,
            semanticRefreshCallbacks,
          ),
        ),
      );
      conn.initialized = true;

      const leases = this.leaseSets.get(key);
      if (leases) leases.retryIndex = 0;
      this.setStatus(key, { state: "ready" });
    } catch (err) {
      const wasCurrent = this.isCurrentConnection(conn);
      this.cleanupConnection(conn);
      if (!wasCurrent) return;
      console.error(`[LSP] activateAttachment error:`, err);
      this.setStatus(key, { state: "error", reason: lspErrorMessage(err) });
      const leases = this.leaseSets.get(key);
      if (leases) this.scheduleReconnect(leases);
    }
  }

  // ------- Monaco provider registration (delegated to lsp-providers.ts) -------

  private registerProviders(
    rpc: JsonRpcConnection,
    lspLanguage: string,
    conn: ManagedLspConnection,
    serverCapabilities: Record<string, unknown> | null,
    semanticRefreshCallbacks: (() => void)[],
  ): IDisposable[] {
    return registerLspProviders({
      rpc,
      lspLanguage,
      serverCapabilities,
      semanticRefreshCallbacks,
      getDocumentUri: (model) => connectionDocumentUri(model, conn),
      getModelUri: (uri) =>
        connectionModelUri(uri, conn, getMonacoInstance()?.editor.getModels() ?? []),
      ensureModelsExist: (uris) => this.editorState.ensureModelsExist(uris, conn),
    });
  }

  /** Dispose a placeholder model (e.g. when the file is opened in a real tab). */
  disposePlaceholderModel(modelUri: string): void {
    this.editorState.disposePlaceholderModel(modelUri);
  }

  // ------- Document synchronization -------

  private connectionForSession(
    sessionId: string,
    lspLanguage: string,
  ): ManagedLspConnection | undefined {
    for (const connection of this.connections.values()) {
      if (
        connection.key.endsWith(`:${lspLanguage}`) &&
        connection.sessionRefCounts.has(sessionId)
      ) {
        return connection;
      }
    }
    return undefined;
  }

  openDocument(sessionId: string, lspLanguage: string, document: OpenDocumentParams): void {
    const conn = this.connectionForSession(sessionId, lspLanguage);
    if (!conn?.initialized || !conn.rpc) return;
    const documentUri = canonicalFileUri(document.uri);
    if (!documentUri) return;
    this.promoteDocumentModel(sessionId, documentUri, document.text);
    const existing = conn.openDocuments.get(documentUri);
    if (existing) {
      existing.refCount++;
      existing.sessionRefCounts.set(sessionId, (existing.sessionRefCounts.get(sessionId) ?? 0) + 1);
      if (document.repo) this.addRepositorySubpath(conn, document.repo);
      return;
    }

    if (document.repo) this.addRepositorySubpath(conn, document.repo);
    conn.openDocuments.set(documentUri, {
      version: 1,
      languageId: document.languageId,
      refCount: 1,
      sessionRefCounts: new Map([[sessionId, 1]]),
      text: document.text,
    });
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
    const conn = this.connectionForSession(sessionId, lspLanguage);
    if (!conn?.initialized || !conn.rpc) return;
    const canonicalUri = canonicalFileUri(documentUri);
    if (!canonicalUri) return;
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
        !conn.sessionRefCounts.has(sessionId) ||
        !conn.initialized ||
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
    const conn = this.connectionForSession(sessionId, lspLanguage);
    if (!conn?.initialized || !conn.rpc) return;
    const canonicalUri = canonicalFileUri(documentUri);
    if (!canonicalUri) return;
    this.releaseDocumentReferences(conn, sessionId, canonicalUri, 1);
  }

  private releaseDocumentReferences(
    conn: ManagedLspConnection,
    sessionId: string,
    documentUri: string,
    requestedCount: number,
  ): void {
    if (!conn.rpc) return;
    const document = conn.openDocuments.get(documentUri);
    if (!document) return;
    const sessionRefCount = document.sessionRefCounts.get(sessionId) ?? 0;
    if (sessionRefCount === 0) return;
    const releasedCount = Math.min(requestedCount, sessionRefCount);
    if (releasedCount === sessionRefCount) document.sessionRefCounts.delete(sessionId);
    else document.sessionRefCounts.set(sessionId, sessionRefCount - releasedCount);
    document.refCount -= releasedCount;
    if (document.refCount > 0) return;

    conn.openDocuments.delete(documentUri);
    conn.rpc.sendNotification("textDocument/didClose", {
      textDocument: { uri: documentUri },
    });
  }

  private releaseSessionDocuments(conn: ManagedLspConnection, sessionId: string): void {
    if (!conn.initialized || !conn.rpc) return;
    for (const [documentUri, document] of conn.openDocuments) {
      const sessionRefCount = document.sessionRefCounts.get(sessionId) ?? 0;
      if (sessionRefCount > 0) {
        this.releaseDocumentReferences(conn, sessionId, documentUri, sessionRefCount);
      }
    }
  }

  private addRepositorySubpath(conn: ManagedLspConnection, repository: string): void {
    conn.repositorySubpaths.add(repository);
    this.workspaceMetadata.get(conn.key)?.repositorySubpaths.add(repository);
  }

  // ------- Attachment cleanup -------

  detach(taskId: string, lspLanguage: string): void {
    const key = `${taskId}:${lspLanguage}`;
    const leases = this.leaseSets.get(key);
    if (leases) {
      this.clearReconnectTimer(leases);
      this.leaseSets.delete(key);
    }
    const conn = this.connections.get(key);
    if (conn) this.cleanupConnection(conn);
    this.statuses.delete(key);
    this.notifyChange(key);
  }

  disconnectAll(): void {
    for (const leases of this.leaseSets.values()) this.clearReconnectTimer(leases);
    this.leaseSets.clear();
    for (const conn of this.connections.values()) {
      this.cleanupConnection(conn);
    }
    this.statuses.clear();
    this.workspaceMetadata.clear();
  }

  private releaseLease(leases: LspLeaseSet, sessionId: string): void {
    if (this.leaseSets.get(leases.key) !== leases) return;
    const sessionRefs = (leases.sessionRefCounts.get(sessionId) ?? 0) - 1;
    const conn = this.connections.get(leases.key);
    if (sessionRefs <= 0) {
      if (conn) this.releaseSessionDocuments(conn, sessionId);
      leases.sessionRefCounts.delete(sessionId);
    } else leases.sessionRefCounts.set(sessionId, sessionRefs);
    if (conn?.sessionId === sessionId && !leases.sessionRefCounts.has(sessionId)) {
      conn.sessionId = leases.sessionRefCounts.keys().next().value ?? sessionId;
    }
    if (leases.sessionRefCounts.size !== 0) return;
    this.clearReconnectTimer(leases);
    this.leaseSets.delete(leases.key);
    if (conn) this.cleanupConnection(conn);
    this.statuses.delete(leases.key);
    this.notifyChange(leases.key);
  }

  private scheduleReconnect(leases: LspLeaseSet): void {
    if (
      this.leaseSets.get(leases.key) !== leases ||
      leases.sessionRefCounts.size === 0 ||
      leases.retryTimer !== null
    ) {
      return;
    }
    const delayIndex = Math.min(leases.retryIndex, ATTACHMENT_RETRY_DELAYS_MS.length - 1);
    const delay = ATTACHMENT_RETRY_DELAYS_MS[delayIndex] ?? 5_000;
    leases.retryIndex = Math.min(delayIndex + 1, ATTACHMENT_RETRY_DELAYS_MS.length - 1);
    leases.retryTimer = setTimeout(() => {
      leases.retryTimer = null;
      if (this.leaseSets.get(leases.key) !== leases || leases.sessionRefCounts.size === 0) return;
      const current = this.connections.get(leases.key);
      if (current) this.cleanupConnection(current);
      this.createConnection(leases);
    }, delay);
  }

  private clearReconnectTimer(leases: LspLeaseSet): void {
    if (leases.retryTimer === null) return;
    clearTimeout(leases.retryTimer);
    leases.retryTimer = null;
  }

  private isCurrentConnection(conn: ManagedLspConnection): boolean {
    return this.connections.get(conn.key) === conn;
  }

  private cleanupConnection(conn: ManagedLspConnection) {
    for (const d of conn.providerDisposables) d.dispose();
    conn.providerDisposables = [];
    conn.rpc?.dispose();
    conn.rpc = null;
    conn.initialized = false;
    conn.protocolInitialized = false;
    conn.openDocuments.clear();
    conn.diagnosticsByUri.clear();
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

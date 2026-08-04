import type { editor as monacoEditor, IDisposable } from "monaco-editor";
import { getMonacoInstance, waitForMonacoInstance } from "@/components/editors/monaco/monaco-init";
import {
  setBuiltinTsSuppressed,
  withLspProviderRegistration,
} from "@/components/editors/monaco/builtin-providers";
import { registerLspProviders } from "./lsp-providers";
import { canonicalFileUri } from "./file-uri";
import {
  JsonRpcConnection,
  getWsBaseUrl,
  CLOSE_CODE_STATUS,
  LSP_CLIENT_CAPABILITIES,
} from "./lsp-json-rpc";
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
  lspWorkspaceFolders,
  repositorySubpathsForSession,
  workspaceUriForSession,
  type WorkspaceMetadata,
} from "./lsp-workspace";
import {
  EMPTY_LSP_PROGRESS,
  finishLspInitialization,
  type LspProgressSnapshot,
} from "./lsp-progress";
import { beginLspProgressTracking } from "./lsp-client-progress";
import {
  clearLspEnabledState,
  isLspEnabledInStorage,
  saveLspEnabledState,
} from "./lsp-client-storage";
import { DISABLED_LSP_STATUS, LSP_IDLE_TIMEOUT } from "./lsp-client-config";
import { LSP_DEFAULT_CONFIGS } from "./lsp-client-config";
import { buildDocumentContentChanges } from "./lsp-document-sync";

export type { LspStatus } from "./lsp-json-rpc";
export { toLspLanguage } from "./lsp-json-rpc";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type ChangeListener = (key: string) => void;

function hasActiveLspWork(progress: LspProgressSnapshot): boolean {
  return progress.initializingSince !== null || progress.active.length > 0;
}

class LSPClientManager {
  private connections = new Map<string, ManagedLspConnection>();
  private connectionGeneration = 0;
  private statuses = new Map<string, LspStatus>();
  /** Keeps Monaco model identity stable after an LSP connection stops or crashes. */
  private workspaceMetadata = new Map<string, WorkspaceMetadata>();
  private listeners = new Set<ChangeListener>();
  private fileOpener: ((uri: string, line?: number, column?: number) => void) | null = null;
  private editorState = new LspClientEditorState((connection) =>
    this.isCurrentConnection(connection),
  );
  /** Tracks ready TypeScript connections that require Monaco's built-in providers to stay off. */
  private typescriptConnections = new Set<string>();

  setFileOpener(opener: ((uri: string, line?: number, column?: number) => void) | null): void {
    this.fileOpener = opener;
  }

  getFileOpener(): ((uri: string, line?: number, column?: number) => void) | null {
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
  ): () => void {
    const key = `${sessionId}:${lspLanguage}`;

    const existing = this.connections.get(key);
    if (existing && existing.ws.readyState <= WebSocket.OPEN) {
      existing.refCount++;
      if (existing.idleTimer) {
        clearTimeout(existing.idleTimer);
        existing.idleTimer = null;
      }
      return () => this.decrementRef(existing);
    }
    if (existing) this.cleanupConnection(existing);

    const wsUrl = `${getWsBaseUrl()}/lsp/${sessionId}?language=${lspLanguage}`;
    const ws = new WebSocket(wsUrl);

    const conn = createManagedLspConnection(key, sessionId, ++this.connectionGeneration, ws);
    this.connections.set(key, conn);
    this.setStatus(key, { state: "connecting" });

    let bridgeStarted = false;
    let terminalStatusReceived = false;

    ws.onopen = () => {
      if (!this.isCurrentConnection(conn)) return;
      this.setStatus(key, { state: "starting" });
    };

    // Listen for backend status messages before the LSP bridge starts.
    const statusHandler = (event: MessageEvent) => {
      if (bridgeStarted || !this.isCurrentConnection(conn)) return;

      let data: {
        status?: string;
        error?: string;
        workspacePath?: string;
        workspaceUri?: string;
        repoSubpaths?: string[];
      };
      try {
        data = JSON.parse(event.data as string);
      } catch {
        return;
      }

      if (data.status === "installing") {
        this.setStatus(key, { state: "installing" });
      } else if (data.status === "installed") {
        this.setStatus(key, { state: "starting" });
      } else if (data.status === "ready") {
        // Language server is running — start the LSP JSON-RPC bridge
        ws.removeEventListener("message", statusHandler);
        bridgeStarted = true;
        this.initializeLsp(
          conn,
          lspLanguage,
          {
            path: data.workspacePath ?? null,
            uri: data.workspaceUri ?? null,
            repositorySubpaths: data.repoSubpaths ?? [],
          },
          userConfigs,
        );
      } else if (data.status === "install_failed") {
        ws.removeEventListener("message", statusHandler);
        terminalStatusReceived = true;
        this.setStatus(key, { state: "error", reason: data.error || "Install failed" });
      }
    };
    ws.addEventListener("message", statusHandler);

    ws.onclose = (event) => {
      ws.removeEventListener("message", statusHandler);
      const wasCurrent = this.isCurrentConnection(conn);
      this.cleanupConnection(conn);
      if (!wasCurrent) return;

      const current = this.statuses.get(key);
      if (current?.state === "stopping") {
        this.setStatus(key, { state: "disabled" });
        this.statuses.delete(key);
        return;
      }
      if (terminalStatusReceived) return;

      const statusFactory = CLOSE_CODE_STATUS[event.code];
      if (statusFactory) {
        this.setStatus(key, statusFactory(event.reason));
      } else {
        const fallbackReason = bridgeStarted ? "language server exited" : "Connection closed";
        this.setStatus(key, { state: "error", reason: event.reason || fallbackReason });
      }
    };

    ws.onerror = () => {
      if (!this.isCurrentConnection(conn)) return;
      const current = this.statuses.get(key);
      if (current?.state !== "error" && current?.state !== "unavailable") {
        this.setStatus(key, { state: "error", reason: "WebSocket error" });
      }
    };

    return () => this.decrementRef(conn);
  }

  private async initializeLsp(
    conn: ManagedLspConnection,
    lspLanguage: string,
    workspace: LspReadyWorkspace,
    userConfigs?: Record<string, Record<string, unknown>>,
  ) {
    if (!this.isCurrentConnection(conn)) return;
    const { key, ws } = conn;

    const workspaceMetadata = configureLspWorkspace(conn, workspace);
    if (workspaceMetadata) this.workspaceMetadata.set(conn.key, workspaceMetadata);

    // Merge default configs with user overrides for this language
    const mergedConfig: Record<string, unknown> = {
      ...(LSP_DEFAULT_CONFIGS[lspLanguage] ?? {}),
      ...(userConfigs?.[lspLanguage] ?? {}),
    };

    try {
      const rpc = new JsonRpcConnection(ws);
      rpc.listen();
      conn.rpc = rpc;
      beginLspProgressTracking(
        conn,
        rpc,
        () => this.isCurrentConnection(conn),
        () => this.handleProgressChange(conn),
      );

      // Handle server requests
      rpc.onRequest("workspace/configuration", (params: unknown) => {
        const items = (params as { items?: { section?: string }[] })?.items;
        if (!Array.isArray(items)) return [mergedConfig];
        return items.map(() => mergedConfig);
      });
      rpc.onRequest("client/registerCapability", () => null);

      const initResult = (await rpc.sendRequest("initialize", {
        processId: null,
        capabilities: LSP_CLIENT_CAPABILITIES,
        workDoneToken: conn.ownerId,
        rootUri: conn.workspaceUri,
        workspaceFolders: lspWorkspaceFolders(conn.workspaceUri, workspace.path),
        initializationOptions: {},
      })) as { capabilities?: Record<string, unknown> } | null;

      if (!this.isCurrentConnection(conn)) {
        this.cleanupConnection(conn);
        return;
      }

      const progress = finishLspInitialization(conn.progress);
      if (progress !== conn.progress) {
        conn.progress = progress;
        this.handleProgressChange(conn);
      }
      conn.serverCapabilities = initResult?.capabilities ?? null;
      rpc.sendNotification("initialized", {});

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

      // Monaco's provider wrappers are installed once its module is ready.
      // Only then suppress built-ins and explicitly mark our registration so
      // lazy built-ins that arrive later are still wrapped.
      if (lspLanguage === "typescript") {
        this.addTypeScriptConnection(conn.ownerId);
      }

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

      this.setStatus(key, { state: "ready" });
    } catch (err) {
      const wasCurrent = this.isCurrentConnection(conn);
      this.cleanupConnection(conn);
      if (!wasCurrent) return;
      console.error(`[LSP] initializeLsp error:`, err);
      this.setStatus(key, { state: "error", reason: String(err) });
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

  openDocument(sessionId: string, lspLanguage: string, document: OpenDocumentParams): void {
    const key = `${sessionId}:${lspLanguage}`;
    const conn = this.connections.get(key);
    if (!conn?.initialized || !conn.rpc) return;
    const documentUri = canonicalFileUri(document.uri);
    if (!documentUri) return;
    this.promoteDocumentModel(sessionId, documentUri, document.text);
    const existing = conn.openDocuments.get(documentUri);
    if (existing) {
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
    const key = `${sessionId}:${lspLanguage}`;
    const conn = this.connections.get(key);
    if (!conn?.initialized || !conn.rpc) return;
    const canonicalUri = canonicalFileUri(documentUri);
    if (!canonicalUri) return;
    const doc = conn.openDocuments.get(canonicalUri);
    if (!doc) return;
    if (doc.text === text) return;

    const contentChanges = buildDocumentContentChanges(conn.serverCapabilities, doc.text, text);
    doc.text = text;
    if (contentChanges.length === 0) return;
    doc.version++;
    conn.rpc.sendNotification("textDocument/didChange", {
      textDocument: { uri: canonicalUri, version: doc.version },
      contentChanges,
    });
  }

  closeDocument(sessionId: string, lspLanguage: string, documentUri: string): void {
    const key = `${sessionId}:${lspLanguage}`;
    const conn = this.connections.get(key);
    if (!conn?.initialized || !conn.rpc) return;
    const canonicalUri = canonicalFileUri(documentUri);
    if (!canonicalUri) return;
    const document = conn.openDocuments.get(canonicalUri);
    if (!document) return;
    document.refCount--;
    if (document.refCount > 0) return;

    conn.openDocuments.delete(canonicalUri);
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

  private cleanupConnection(conn: ManagedLspConnection) {
    this.clearIdleTimer(conn);
    for (const d of conn.providerDisposables) d.dispose();
    conn.providerDisposables = [];
    conn.rpc?.dispose();
    conn.rpc = null;
    conn.initialized = false;
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
    this.removeTypeScriptConnection(conn.ownerId);
    this.editorState.disposeConnection(conn.ownerId);
  }

  private addTypeScriptConnection(ownerId: string): void {
    const shouldSuppress = this.typescriptConnections.size === 0;
    this.typescriptConnections.add(ownerId);
    if (shouldSuppress) setBuiltinTsSuppressed(true);
  }

  private removeTypeScriptConnection(ownerId: string): void {
    if (!this.typescriptConnections.delete(ownerId)) return;
    if (this.typescriptConnections.size === 0) setBuiltinTsSuppressed(false);
  }
}

export const lspClientManager = new LSPClientManager();

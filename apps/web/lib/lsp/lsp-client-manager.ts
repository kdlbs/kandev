import type { editor as monacoEditor, IDisposable } from "monaco-editor";
import { getMonacoInstance, waitForMonacoInstance } from "@/components/editors/monaco/monaco-init";
import { setBuiltinTsSuppressed } from "@/components/editors/monaco/builtin-providers";
import { registerLspProviders } from "./lsp-providers";
import {
  canonicalFileUri,
  documentUriForModel,
  fileUrisEqual,
  modelUriForDocument,
  resolveFileUriInWorkspace,
} from "./file-uri";
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
import {
  connectionDocumentUri,
  connectionModelMatchesUri,
  connectionModelUri,
  diagnosticMarkers,
} from "./lsp-editor-models";
import {
  configureLspWorkspace,
  lspWorkspaceFolders,
  type WorkspaceMetadata,
} from "./lsp-workspace";

export type { LspStatus } from "./lsp-json-rpc";
export { toLspLanguage } from "./lsp-json-rpc";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export const LSP_DEFAULT_CONFIGS: Record<string, Record<string, unknown>> = {
  go: { "ui.semanticTokens": true },
};

const DISABLED_STATUS = { state: "disabled" } as const;
const LSP_IDLE_TIMEOUT = 2 * 60 * 1000; // 2 minutes

type StatusListener = (key: string, status: LspStatus) => void;
class LSPClientManager {
  private connections = new Map<string, ManagedLspConnection>();
  private connectionGeneration = 0;
  private statuses = new Map<string, LspStatus>();
  /** Keeps Monaco model identity stable after an LSP connection stops or crashes. */
  private workspaceMetadata = new Map<string, WorkspaceMetadata>();
  private listeners = new Set<StatusListener>();
  private fileOpener: ((uri: string, line?: number, column?: number) => void) | null = null;
  /** Tracks which connections own placeholder Monaco models created for references/definitions. */
  private placeholderModelOwners = new Map<string, Set<string>>();
  /** Tracks ready TypeScript connections that require Monaco's built-in providers to stay off. */
  private typescriptConnections = new Set<string>();

  setFileOpener(opener: ((uri: string, line?: number, column?: number) => void) | null): void {
    this.fileOpener = opener;
  }

  getFileOpener(): ((uri: string, line?: number, column?: number) => void) | null {
    return this.fileOpener;
  }

  // ---- localStorage persistence for manual LSP toggle ----

  private lspStorageKey(sessionId: string, language: string): string {
    return `kandev-lsp:${sessionId}:${language}`;
  }

  /** Save that LSP was manually enabled for this session+language. */
  saveEnabledState(sessionId: string, language: string): void {
    try {
      localStorage.setItem(this.lspStorageKey(sessionId, language), "1");
    } catch {}
  }

  /** Clear the saved LSP state (manual stop). */
  clearEnabledState(sessionId: string, language: string): void {
    try {
      localStorage.removeItem(this.lspStorageKey(sessionId, language));
    } catch {}
  }

  /** Check if LSP was previously enabled for this session+language. */
  isEnabledInStorage(sessionId: string, language: string): boolean {
    try {
      return localStorage.getItem(this.lspStorageKey(sessionId, language)) === "1";
    } catch {
      return false;
    }
  }

  getStatus(sessionId: string, lspLanguage: string): LspStatus {
    const key = `${sessionId}:${lspLanguage}`;
    return this.statuses.get(key) ?? DISABLED_STATUS;
  }

  getWorkspaceUriForSession(sessionId: string): string | null {
    for (const conn of this.connections.values()) {
      if (conn.key.startsWith(`${sessionId}:`) && conn.workspaceUri) return conn.workspaceUri;
    }
    for (const [key, workspace] of this.workspaceMetadata) {
      if (key.startsWith(`${sessionId}:`)) return workspace.uri;
    }
    return null;
  }

  getRepositorySubpaths(sessionId: string): string[] {
    const repositories = new Set<string>();
    for (const conn of this.connections.values()) {
      if (!conn.key.startsWith(`${sessionId}:`)) continue;
      for (const repo of conn.repositorySubpaths) repositories.add(repo);
    }
    for (const [key, workspace] of this.workspaceMetadata) {
      if (!key.startsWith(`${sessionId}:`)) continue;
      for (const repo of workspace.repositorySubpaths) repositories.add(repo);
    }
    return [...repositories];
  }

  onStatusChange(listener: StatusListener): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  private setStatus(key: string, status: LspStatus) {
    this.statuses.set(key, status);
    for (const listener of this.listeners) {
      listener(key, status);
    }
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
      if (current?.state === "ready" || current?.state === "stopping") {
        this.setStatus(key, { state: "disabled" });
        this.statuses.delete(key);
        return;
      }

      if (!bridgeStarted) {
        const statusFactory = CLOSE_CODE_STATUS[event.code];
        if (statusFactory) {
          this.setStatus(key, statusFactory(event.reason));
        } else if (current?.state !== "error" && current?.state !== "unavailable") {
          this.setStatus(key, { state: "error", reason: event.reason || "Connection closed" });
        }
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

      // Handle server requests
      rpc.onRequest("workspace/configuration", (params: unknown) => {
        const items = (params as { items?: { section?: string }[] })?.items;
        if (!Array.isArray(items)) return [mergedConfig];
        return items.map(() => mergedConfig);
      });
      rpc.onRequest("client/registerCapability", () => null);
      rpc.onRequest("window/workDoneProgress/create", () => null);

      const initResult = (await rpc.sendRequest("initialize", {
        processId: null,
        capabilities: LSP_CLIENT_CAPABILITIES,
        rootUri: conn.workspaceUri,
        workspaceFolders: lspWorkspaceFolders(conn.workspaceUri, workspace.path),
        initializationOptions: {},
      })) as { capabilities?: Record<string, unknown> } | null;

      if (!this.isCurrentConnection(conn)) {
        this.cleanupConnection(conn);
        return;
      }

      conn.serverCapabilities = initResult?.capabilities ?? null;
      rpc.sendNotification("initialized", {});

      // Register diagnostics handler
      rpc.onNotification("textDocument/publishDiagnostics", (params) => {
        if (!this.isCurrentConnection(conn)) return;
        this.handleDiagnostics(conn, params as PublishDiagnosticsParams);
      });

      // Suppress Monaco's built-in TS/JS providers BEFORE registering our LSP providers.
      if (lspLanguage === "typescript") {
        this.addTypeScriptConnection(conn.ownerId);
      }

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
          if (this.isCurrentConnection(conn)) this.applyCachedDiagnostics(conn, model);
        }),
      );
      for (const model of monaco.editor.getModels()) this.applyCachedDiagnostics(conn, model);

      // Register Monaco providers for this language
      conn.providerDisposables.push(
        ...this.registerProviders(
          rpc,
          lspLanguage,
          conn,
          conn.serverCapabilities,
          semanticRefreshCallbacks,
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
      ensureModelsExist: (uris) => this.ensureModelsExist(uris, conn),
    });
  }

  // ------- Placeholder models for Go-to-Definition / References -------

  private ensureModelsExist(uris: string[], conn: ManagedLspConnection): void {
    if (!this.isCurrentConnection(conn)) return;
    const monaco = getMonacoInstance();
    if (!monaco) return;

    for (const fileUri of uris) {
      const canonicalUri = canonicalFileUri(fileUri);
      if (!canonicalUri) continue;
      if (
        !conn.workspaceUri ||
        !resolveFileUriInWorkspace(canonicalUri, conn.workspaceUri, conn.repositorySubpaths)
      ) {
        continue;
      }
      const modelUri = modelUriForDocument(canonicalUri, conn.sessionId);
      const parsed = monaco.Uri.parse(modelUri);
      const existingModel = monaco.editor
        .getModels()
        .find((model: monacoEditor.ITextModel) =>
          connectionModelMatchesUri(model, canonicalUri, conn),
        );

      if (existingModel) {
        const owners = this.placeholderModelOwners.get(modelUri);
        if (!owners || owners.has(conn.ownerId)) continue;
        owners.add(conn.ownerId);
        this.loadPlaceholderContent(canonicalUri, modelUri, existingModel, conn);
        continue;
      }

      const placeholderModel = monaco.editor.createModel("", undefined, parsed);
      this.placeholderModelOwners.set(modelUri, new Set([conn.ownerId]));
      this.loadPlaceholderContent(canonicalUri, modelUri, placeholderModel, conn);
    }
  }

  private loadPlaceholderContent(
    documentUri: string,
    modelUri: string,
    placeholderModel: monacoEditor.ITextModel,
    conn: ManagedLspConnection,
  ): void {
    if (!conn.workspaceUri) return;
    const location = resolveFileUriInWorkspace(
      documentUri,
      conn.workspaceUri,
      conn.repositorySubpaths,
    );
    if (!location) return;

    // Dynamic import to avoid circular dependency
    Promise.all([import("@/lib/ws/connection"), import("@/lib/ws/workspace-files")])
      .then(([{ getWebSocketClient }, { requestFileContent }]) => {
        if (!this.isPlaceholderOwner(modelUri, conn)) return;
        const client = getWebSocketClient();
        if (!client) return;
        return requestFileContent(client, conn.sessionId, location.path, location.repo);
      })
      .then((response) => {
        if (!response || !this.isPlaceholderOwner(modelUri, conn)) return;
        const monaco = getMonacoInstance();
        const currentModel = monaco?.editor.getModel(placeholderModel.uri);
        if (currentModel === placeholderModel) placeholderModel.setValue(response.content);
      })
      .catch(() => {
        // Best effort — placeholder stays empty
      });
  }

  private isPlaceholderOwner(modelUri: string, conn: ManagedLspConnection): boolean {
    return (
      this.isCurrentConnection(conn) &&
      this.placeholderModelOwners.get(modelUri)?.has(conn.ownerId) === true
    );
  }

  /** Dispose a placeholder model (e.g. when the file is opened in a real tab). */
  disposePlaceholderModel(modelUri: string): void {
    if (!this.placeholderModelOwners.delete(modelUri)) return;
    const monaco = getMonacoInstance();
    if (!monaco) return;
    const model = monaco.editor.getModel(monaco.Uri.parse(modelUri));
    if (model) model.dispose();
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
    const canonicalUri = canonicalFileUri(documentUri);
    if (!canonicalUri) return;
    const monaco = getMonacoInstance();
    const realModelUri = modelUriForDocument(canonicalUri, sessionId);
    let promoted = false;
    for (const placeholderUri of this.placeholderModelOwners.keys()) {
      const placeholderDocumentUri = documentUriForModel(placeholderUri, sessionId);
      if (!placeholderDocumentUri || !fileUrisEqual(placeholderDocumentUri, canonicalUri)) continue;
      this.placeholderModelOwners.delete(placeholderUri);
      promoted = true;
      if (monaco && placeholderUri !== realModelUri) {
        monaco.editor.getModel(monaco.Uri.parse(placeholderUri))?.dispose();
      }
    }
    if (!promoted || !monaco) return;
    monaco.editor.getModel(monaco.Uri.parse(realModelUri))?.setValue(text);
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

    doc.version++;
    doc.text = text;
    conn.rpc.sendNotification("textDocument/didChange", {
      textDocument: { uri: canonicalUri, version: doc.version },
      contentChanges: [{ text }],
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

  // ------- Helpers -------

  private handleDiagnostics(conn: ManagedLspConnection, params: PublishDiagnosticsParams) {
    const uri = canonicalFileUri(params.uri);
    if (!uri) return;
    const canonicalParams = { ...params, uri };
    conn.diagnosticsByUri.set(uri, canonicalParams);
    const monaco = getMonacoInstance();
    if (!monaco) return;

    const models = monaco.editor.getModels();
    const targetModel = models.find((model: monacoEditor.ITextModel) =>
      connectionModelMatchesUri(model, uri, conn),
    );
    if (!targetModel) return;

    this.applyDiagnostics(conn.ownerId, targetModel, canonicalParams);
  }

  private applyCachedDiagnostics(conn: ManagedLspConnection, model: monacoEditor.ITextModel): void {
    for (const params of conn.diagnosticsByUri.values()) {
      if (connectionModelMatchesUri(model, params.uri, conn)) {
        this.applyDiagnostics(conn.ownerId, model, params);
      }
    }
  }

  private applyDiagnostics(
    ownerId: string,
    targetModel: monacoEditor.ITextModel,
    params: PublishDiagnosticsParams,
  ): void {
    const monaco = getMonacoInstance();
    if (!monaco) return;

    monaco.editor.setModelMarkers(
      targetModel,
      this.markerOwner(ownerId),
      diagnosticMarkers(params),
    );
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
    for (const listener of this.listeners) {
      listener(key, DISABLED_STATUS);
    }
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
    const { key } = conn;
    conn.refCount--;
    if (conn.refCount <= 0) {
      conn.idleTimer = setTimeout(() => {
        const wasCurrent = this.isCurrentConnection(conn);
        this.cleanupConnection(conn);
        if (!wasCurrent) return;
        this.statuses.delete(key);
        for (const listener of this.listeners) {
          listener(key, DISABLED_STATUS);
        }
      }, LSP_IDLE_TIMEOUT);
    }
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
    conn.openDocuments.clear();
    conn.diagnosticsByUri.clear();
    try {
      if (conn.ws.readyState <= WebSocket.OPEN) {
        conn.ws.close();
      }
    } catch {
      // ignore
    }
    if (this.isCurrentConnection(conn)) this.connections.delete(conn.key);
    this.removeTypeScriptConnection(conn.ownerId);
    this.disposeConnectionEditorState(conn.ownerId);
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

  private markerOwner(ownerId: string): string {
    return `lsp:${ownerId}`;
  }

  private disposeConnectionEditorState(ownerId: string): void {
    const monaco = getMonacoInstance();
    for (const [uri, owners] of this.placeholderModelOwners) {
      owners.delete(ownerId);
      if (owners.size === 0) {
        if (monaco) {
          const parsed = monaco.Uri.parse(uri);
          const model = monaco.editor.getModel(parsed);
          if (model) model.dispose();
        }
        this.placeholderModelOwners.delete(uri);
      }
    }

    if (monaco) {
      const markerOwner = this.markerOwner(ownerId);
      for (const model of monaco.editor.getModels()) {
        monaco.editor.setModelMarkers(model, markerOwner, []);
      }
    }
  }
}

export const lspClientManager = new LSPClientManager();

import type { editor as monacoEditor, IDisposable } from "monaco-editor";
import { getMonacoInstance } from "@/components/editors/monaco/monaco-init";
import { setBuiltinTsSuppressed } from "@/components/editors/monaco/builtin-providers";
import { registerLspProviders } from "./lsp-providers";
import {
  JsonRpcConnection,
  toMonacoRange,
  toMonacoSeverity,
  getWsBaseUrl,
  CLOSE_CODE_STATUS,
  LSP_CLIENT_CAPABILITIES,
} from "./lsp-json-rpc";
import type { LspRange, LSPConnection, LspStatus } from "./lsp-json-rpc";

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
  private connections = new Map<string, LSPConnection>();
  private statuses = new Map<string, LspStatus>();
  private listeners = new Set<StatusListener>();
  private fileOpener: ((uri: string, line?: number, column?: number) => void) | null = null;
  /** Tracks placeholder Monaco models created for LSP references/definitions. */
  private placeholderModels = new Set<string>();

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
      return () => this.decrementRef(key);
    }

    const wsUrl = `${getWsBaseUrl()}/lsp/${sessionId}?language=${lspLanguage}`;
    const ws = new WebSocket(wsUrl);

    const conn: LSPConnection = {
      ws,
      rpc: null,
      initialized: false,
      refCount: 1,
      idleTimer: null,
      openDocuments: new Map(),
      providerDisposables: [],
      serverCapabilities: null,
      workspacePath: null,
    };
    this.connections.set(key, conn);
    this.setStatus(key, { state: "connecting" });

    let bridgeStarted = false;

    ws.onopen = () => {
      this.setStatus(key, { state: "starting" });
    };

    // Listen for backend status messages before the LSP bridge starts.
    const statusHandler = (event: MessageEvent) => {
      if (bridgeStarted) return;

      let data: { status?: string; error?: string; workspacePath?: string };
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
        this.initializeLsp(key, ws, lspLanguage, data.workspacePath ?? null, userConfigs);
      } else if (data.status === "install_failed") {
        ws.removeEventListener("message", statusHandler);
        this.setStatus(key, { state: "error", reason: data.error || "Install failed" });
      }
    };
    ws.addEventListener("message", statusHandler);

    ws.onclose = (event) => {
      ws.removeEventListener("message", statusHandler);
      this.disposeConnection(key);

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
      const current = this.statuses.get(key);
      if (current?.state !== "error" && current?.state !== "unavailable") {
        this.setStatus(key, { state: "error", reason: "WebSocket error" });
      }
    };

    return () => this.decrementRef(key);
  }

  private async initializeLsp(
    key: string,
    ws: WebSocket,
    lspLanguage: string,
    workspacePath: string | null,
    userConfigs?: Record<string, Record<string, unknown>>,
  ) {
    const conn = this.connections.get(key);
    if (!conn) return;

    conn.workspacePath = workspacePath;

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
        rootUri: workspacePath ? `file://${workspacePath}` : null,
        workspaceFolders: workspacePath
          ? [
              {
                uri: `file://${workspacePath}`,
                name: workspacePath.split("/").pop() ?? "workspace",
              },
            ]
          : null,
        initializationOptions: {},
      })) as { capabilities?: Record<string, unknown> } | null;

      conn.serverCapabilities = initResult?.capabilities ?? null;
      rpc.sendNotification("initialized", {});

      // Register diagnostics handler
      rpc.onNotification("textDocument/publishDiagnostics", (params) => {
        this.handleDiagnostics(
          params as {
            uri: string;
            diagnostics: Array<{
              range: LspRange;
              message: string;
              severity?: number;
              source?: string;
              code?: unknown;
            }>;
          },
        );
      });

      // Suppress Monaco's built-in TS/JS providers BEFORE registering our LSP providers.
      if (lspLanguage === "typescript") {
        setBuiltinTsSuppressed(true);
      }

      // Collect callbacks for semantic token refresh
      const semanticRefreshCallbacks: (() => void)[] = [];
      rpc.onRequest("workspace/semanticTokens/refresh", () => {
        for (const cb of semanticRefreshCallbacks) cb();
        return null;
      });

      // Register Monaco providers for this language
      conn.providerDisposables = this.registerProviders(
        rpc,
        lspLanguage,
        key,
        conn.serverCapabilities,
        semanticRefreshCallbacks,
      );
      conn.initialized = true;

      this.setStatus(key, { state: "ready" });
    } catch (err) {
      console.error(`[LSP] initializeLsp error:`, err);
      this.setStatus(key, { state: "error", reason: String(err) });
    }
  }

  // ------- Monaco provider registration (delegated to lsp-providers.ts) -------

  private registerProviders(
    rpc: JsonRpcConnection,
    lspLanguage: string,
    connectionKey: string,
    serverCapabilities: Record<string, unknown> | null,
    semanticRefreshCallbacks: (() => void)[],
  ): IDisposable[] {
    return registerLspProviders({
      rpc,
      lspLanguage,
      connectionKey,
      serverCapabilities,
      semanticRefreshCallbacks,
      getDocumentUri: (model) => this.getDocumentUri(model),
      ensureModelsExist: (uris, key) => this.ensureModelsExist(uris, key),
    });
  }

  // ------- Placeholder models for Go-to-Definition / References -------

  private ensureModelsExist(uris: string[], connectionKey: string): void {
    const monaco = getMonacoInstance();
    if (!monaco) return;

    const conn = this.connections.get(connectionKey);

    for (const fileUri of uris) {
      if (!fileUri.startsWith("file://")) continue;
      const parsed = monaco.Uri.parse(fileUri);

      if (monaco.editor.getModel(parsed)) continue;

      monaco.editor.createModel("", undefined, parsed);
      this.placeholderModels.add(fileUri);

      if (conn?.workspacePath) {
        const absolutePath = parsed.path;
        if (!absolutePath.startsWith(conn.workspacePath)) continue;
        const relativePath = absolutePath.slice(conn.workspacePath.length + 1);

        // Extract sessionId from connection key (format: "sessionId:lspLanguage")
        const sessionId = connectionKey.split(":")[0];

        // Dynamic import to avoid circular dependency
        Promise.all([import("@/lib/ws/connection"), import("@/lib/ws/workspace-files")])
          .then(([{ getWebSocketClient }, { requestFileContent }]) => {
            const client = getWebSocketClient();
            if (!client) return;
            return requestFileContent(client, sessionId, relativePath);
          })
          .then((response) => {
            if (!response) return;
            const model = monaco.editor.getModel(parsed);
            if (model && this.placeholderModels.has(fileUri)) {
              model.setValue(response.content);
            }
          })
          .catch(() => {
            // Best effort — placeholder stays empty
          });
      }
    }
  }

  /** Dispose a placeholder model (e.g. when the file is opened in a real tab). */
  disposePlaceholderModel(fileUri: string): void {
    const monaco = getMonacoInstance();
    if (!monaco || !this.placeholderModels.has(fileUri)) return;
    const parsed = monaco.Uri.parse(fileUri);
    const model = monaco.editor.getModel(parsed);
    if (model) model.dispose();
    this.placeholderModels.delete(fileUri);
  }

  // ------- Document synchronization -------

  openDocument(
    sessionId: string,
    lspLanguage: string,
    documentUri: string,
    languageId: string,
    text: string,
  ): void {
    const key = `${sessionId}:${lspLanguage}`;
    const conn = this.connections.get(key);
    if (!conn?.initialized || !conn.rpc) return;
    if (conn.openDocuments.has(documentUri)) return;

    conn.openDocuments.set(documentUri, { version: 1, languageId });
    conn.rpc.sendNotification("textDocument/didOpen", {
      textDocument: { uri: documentUri, languageId, version: 1, text },
    });
  }

  changeDocument(sessionId: string, lspLanguage: string, documentUri: string, text: string): void {
    const key = `${sessionId}:${lspLanguage}`;
    const conn = this.connections.get(key);
    if (!conn?.initialized || !conn.rpc) return;
    const doc = conn.openDocuments.get(documentUri);
    if (!doc) return;

    doc.version++;
    conn.rpc.sendNotification("textDocument/didChange", {
      textDocument: { uri: documentUri, version: doc.version },
      contentChanges: [{ text }],
    });
  }

  closeDocument(sessionId: string, lspLanguage: string, documentUri: string): void {
    const key = `${sessionId}:${lspLanguage}`;
    const conn = this.connections.get(key);
    if (!conn?.initialized || !conn.rpc) return;
    if (!conn.openDocuments.has(documentUri)) return;

    conn.openDocuments.delete(documentUri);
    conn.rpc.sendNotification("textDocument/didClose", {
      textDocument: { uri: documentUri },
    });
  }

  // ------- Helpers -------

  /** Build a file:// URI from a Monaco model, or null if it can't be determined. */
  private getDocumentUri(model: monacoEditor.ITextModel): string | null {
    const uri = model.uri.toString();
    if (uri.startsWith("file://")) return uri;
    const path = model.uri.path;
    if (path && path.startsWith("/")) return `file://${path}`;
    return null;
  }

  private handleDiagnostics(params: {
    uri: string;
    diagnostics: Array<{
      range: LspRange;
      message: string;
      severity?: number;
      source?: string;
      code?: unknown;
    }>;
  }) {
    const monaco = getMonacoInstance();
    if (!monaco) return;

    const models = monaco.editor.getModels();
    const targetModel = models.find((m: monacoEditor.ITextModel) => {
      const modelUri = m.uri.toString();
      if (modelUri === params.uri) return true;
      if (params.uri.startsWith("file://")) {
        const filePath = params.uri.replace("file://", "");
        if (m.uri.path === filePath) return true;
      }
      return false;
    });
    if (!targetModel) return;

    const markers = params.diagnostics.map((d) => ({
      message: d.message,
      severity: toMonacoSeverity(d.severity),
      ...toMonacoRange(d.range),
      source: d.source,
      code: (() => {
        if (typeof d.code === "object" && d.code !== null)
          return String((d.code as { value: unknown }).value);
        if (d.code !== undefined) return String(d.code);
        return undefined;
      })(),
    }));

    monaco.editor.setModelMarkers(targetModel, "lsp", markers);
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

    this.cleanupConnection(key, conn);
    this.statuses.delete(key);
    for (const listener of this.listeners) {
      listener(key, DISABLED_STATUS);
    }
  }

  disconnectAll(): void {
    for (const [key, conn] of this.connections) {
      if (conn.idleTimer) clearTimeout(conn.idleTimer);
      this.cleanupConnection(key, conn);
    }
    this.statuses.clear();
  }

  private decrementRef(key: string) {
    const conn = this.connections.get(key);
    if (!conn) return;
    conn.refCount--;
    if (conn.refCount <= 0) {
      conn.idleTimer = setTimeout(() => {
        this.cleanupConnection(key, conn);
        this.statuses.delete(key);
        for (const listener of this.listeners) {
          listener(key, DISABLED_STATUS);
        }
      }, LSP_IDLE_TIMEOUT);
    }
  }

  private disposeConnection(key: string) {
    const conn = this.connections.get(key);
    if (!conn) return;
    for (const d of conn.providerDisposables) d.dispose();
    conn.providerDisposables = [];
    conn.rpc?.dispose();
    conn.rpc = null;
    conn.initialized = false;
    conn.openDocuments.clear();
    this.connections.delete(key);
  }

  private cleanupConnection(key: string, conn: LSPConnection) {
    const lspLanguage = key.split(":")[1];
    if (lspLanguage === "typescript") {
      setBuiltinTsSuppressed(false);
    }

    for (const d of conn.providerDisposables) d.dispose();
    conn.providerDisposables = [];
    conn.rpc?.dispose();
    conn.rpc = null;
    conn.initialized = false;
    conn.openDocuments.clear();
    try {
      if (conn.ws.readyState <= WebSocket.OPEN) {
        conn.ws.close();
      }
    } catch {
      // ignore
    }
    this.connections.delete(key);

    // Dispose placeholder models created for this connection
    const monaco = getMonacoInstance();
    if (monaco) {
      for (const uri of this.placeholderModels) {
        const parsed = monaco.Uri.parse(uri);
        const model = monaco.editor.getModel(parsed);
        if (model) model.dispose();
      }
      this.placeholderModels.clear();

      // Clear any LSP markers from Monaco models
      for (const model of monaco.editor.getModels()) {
        monaco.editor.setModelMarkers(model, "lsp", []);
      }
    }
  }
}

export const lspClientManager = new LSPClientManager();

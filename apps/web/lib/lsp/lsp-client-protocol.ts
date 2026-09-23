import type { editor as monacoEditor } from "monaco-editor";
import { getMonacoInstance, waitForMonacoInstance } from "@/components/editors/monaco/monaco-init";
import {
  registerBuiltinTsSuppression,
  withLspProviderRegistration,
} from "@/components/editors/monaco/builtin-providers";
import { t } from "@/lib/i18n";
import { registerLspProviders } from "./lsp-providers";
import { JsonRpcConnection, LSP_CLIENT_CAPABILITIES } from "./lsp-json-rpc";
import type { LspStatus } from "./lsp-json-rpc";
import type {
  LspReadyWorkspace,
  ManagedLspConnection,
  PublishDiagnosticsParams,
} from "./lsp-client-types";
import { connectionDocumentUri, connectionModelUri } from "./lsp-editor-models";
import type { LspClientEditorState } from "./lsp-client-editor-state";
import {
  configureLspWorkspace,
  lspWorkspaceFolders,
  type WorkspaceMetadata,
} from "./lsp-workspace";
import { finishLspInitialization } from "./lsp-progress";
import { beginLspProgressTracking } from "./lsp-client-progress";
import { LSP_RELEASE_ACK_TIMEOUT_MS } from "./lsp-client-config";
import {
  applyLspRegistrations,
  effectiveLspCapabilities,
  normalizeLspRegistrations,
} from "./lsp-dynamic-capabilities";
import { nextLspControlRequestId, waitForLspControlAck } from "./lsp-client-control";
import { getLspMonacoProviderMethods } from "./lsp-provider-capabilities";

export type LspClientProtocolHost = {
  editorState: LspClientEditorState;
  workspaceMetadata: Map<string, WorkspaceMetadata>;
  isCurrentConnection: (conn: ManagedLspConnection) => boolean;
  isCurrentTransportConnection: (conn: ManagedLspConnection, generation: number) => boolean;
  setStatus: (key: string, status: LspStatus) => void;
  handleProgressChange: (conn: ManagedLspConnection) => void;
  cleanupConnection: (conn: ManagedLspConnection) => void;
};

export class LspClientProtocol {
  constructor(private readonly host: LspClientProtocolHost) {}

  async initialize(
    conn: ManagedLspConnection,
    lspLanguage: string,
    workspace: LspReadyWorkspace,
  ): Promise<void> {
    if (!this.host.isCurrentConnection(conn)) return;
    const { key, ws } = conn;
    const transportGeneration = conn.transportGeneration;
    this.host.setStatus(key, { state: "starting" });

    const workspaceMetadata = configureLspWorkspace(conn, workspace);
    if (workspaceMetadata) this.host.workspaceMetadata.set(conn.key, workspaceMetadata);

    const rpc = new JsonRpcConnection(ws);
    rpc.listen();
    conn.rpc = rpc;
    conn.diagnosticsReady = true;
    beginLspProgressTracking(
      conn,
      rpc,
      () => this.host.isCurrentTransportConnection(conn, transportGeneration),
      () => this.host.handleProgressChange(conn),
    );
    this.installRpcHandlers(conn, rpc, lspLanguage);

    try {
      const initResult = (await rpc.sendRequest("initialize", {
        processId: null,
        capabilities: LSP_CLIENT_CAPABILITIES,
        workDoneToken: conn.ownerId,
        rootUri: conn.workspaceUri,
        workspaceFolders: lspWorkspaceFolders(conn.workspaceUri, workspace.path),
        initializationOptions: {},
      })) as { capabilities?: Record<string, unknown> } | null;

      if (!this.host.isCurrentTransportConnection(conn, transportGeneration) || conn.rpc !== rpc)
        return;

      const progress = finishLspInitialization(conn.progress);
      if (progress !== conn.progress) {
        conn.progress = progress;
        this.host.handleProgressChange(conn);
      }
      conn.serverCapabilities = initResult?.capabilities ?? null;
      rpc.sendNotification("initialized", {});
      conn.protocolInitialized = true;
      rpc.sendNotification("workspace/didChangeConfiguration", {
        settings: conn.configuration,
      });

      await waitForMonacoInstance();
      if (!this.host.isCurrentTransportConnection(conn, transportGeneration) || conn.rpc !== rpc)
        return;

      this.rebuildProviders(conn);
      conn.initialized = true;
      this.host.setStatus(key, { state: "ready" });
    } catch (error) {
      if (!this.host.isCurrentTransportConnection(conn, transportGeneration) || conn.rpc !== rpc)
        return;
      this.host.cleanupConnection(conn);
      console.error("[LSP] initializeLsp error:", error);
      this.host.setStatus(key, { state: "error", reason: lspErrorMessage(error) });
    }
  }

  async resume(
    conn: ManagedLspConnection,
    handshake: {
      capabilities?: Record<string, unknown>;
      registrations?: unknown[];
      initialized?: boolean;
      progressTokens?: unknown[];
      progress?: unknown[];
    },
    workspace: LspReadyWorkspace,
  ): Promise<void> {
    if (!this.host.isCurrentConnection(conn)) return;
    const transportGeneration = conn.transportGeneration;
    const { key, ws } = conn;
    const workspaceMetadata = configureLspWorkspace(conn, workspace);
    if (workspaceMetadata) this.host.workspaceMetadata.set(key, workspaceMetadata);
    this.host.setStatus(key, { state: "reconnecting" });
    conn.serverCapabilities = handshake.capabilities ?? null;
    conn.dynamicRegistrations.clear();
    for (const registration of normalizeLspRegistrations(handshake.registrations)) {
      conn.dynamicRegistrations.set(registration.id, registration);
    }

    const rpc = new JsonRpcConnection(ws);
    rpc.listen();
    conn.rpc = rpc;
    conn.diagnosticsReady = false;
    beginLspProgressTracking(
      conn,
      rpc,
      () => this.host.isCurrentTransportConnection(conn, transportGeneration),
      () => this.host.handleProgressChange(conn),
      { progressTokens: handshake.progressTokens, progress: handshake.progress },
    );
    this.installRpcHandlers(conn, rpc, conn.lspLanguage);

    try {
      await waitForMonacoInstance();
      if (!this.host.isCurrentTransportConnection(conn, transportGeneration) || conn.rpc !== rpc)
        return;
      this.rebuildProviders(conn);

      if (handshake.initialized === false) rpc.sendNotification("initialized", {});

      for (const [uri, document] of conn.openDocuments) {
        rpc.sendNotification("textDocument/didOpen", {
          textDocument: {
            uri,
            languageId: document.languageId,
            version: document.version,
            text: document.text,
          },
        });
      }
      const requestId = nextLspControlRequestId();
      const acknowledgement = await waitForLspControlAck(
        ws,
        requestId,
        "attachmentReady",
        LSP_RELEASE_ACK_TIMEOUT_MS,
        () =>
          ws.send(
            JSON.stringify({
              kandev: "lsp",
              action: "attachmentReady",
              requestId,
            }),
          ),
      );
      if (!this.host.isCurrentTransportConnection(conn, transportGeneration) || conn.rpc !== rpc)
        return;
      if (!acknowledgement) throw new Error(t("lsp:syncFailed"));

      conn.diagnosticsReady = true;
      conn.protocolInitialized = true;
      conn.initialized = true;
      conn.reconnecting = false;
      conn.reconnectAttempts = 0;
      this.host.setStatus(key, { state: "ready" });
    } catch (error) {
      if (!this.host.isCurrentTransportConnection(conn, transportGeneration) || conn.rpc !== rpc)
        return;
      this.host.cleanupConnection(conn);
      console.error("[LSP] resumeLsp error:", error);
      this.host.setStatus(key, { state: "error", reason: lspErrorMessage(error) });
    }
  }

  private installRpcHandlers(
    conn: ManagedLspConnection,
    rpc: JsonRpcConnection,
    lspLanguage: string,
  ): void {
    const transportGeneration = conn.transportGeneration;
    const isCurrent = () => this.host.isCurrentTransportConnection(conn, transportGeneration);
    rpc.onRequest("workspace/configuration", (params: unknown) => {
      const items = (params as { items?: { section?: string }[] } | null)?.items;
      if (!Array.isArray(items)) return [conn.configuration];
      return items.map((item) =>
        configurationSection(conn.configuration, item.section, lspLanguage),
      );
    });
    rpc.onRequest("client/registerCapability", (params: unknown) => {
      if (isCurrent()) {
        conn.dynamicRegistrations = applyLspRegistrations(
          conn.dynamicRegistrations,
          "client/registerCapability",
          params,
        );
        if (conn.providersReady) this.rebuildProviders(conn);
      }
      return null;
    });
    rpc.onRequest("client/unregisterCapability", (params: unknown) => {
      if (isCurrent()) {
        conn.dynamicRegistrations = applyLspRegistrations(
          conn.dynamicRegistrations,
          "client/unregisterCapability",
          params,
        );
        if (conn.providersReady) this.rebuildProviders(conn);
      }
      return null;
    });
    rpc.onRequest("workspace/semanticTokens/refresh", () => {
      if (isCurrent()) {
        for (const callback of conn.semanticRefreshCallbacks) callback();
      }
      return null;
    });
    rpc.onNotification("textDocument/publishDiagnostics", (params) => {
      if (!isCurrent() || !conn.diagnosticsReady) return;
      this.host.editorState.handleDiagnostics(conn, params as PublishDiagnosticsParams);
    });
    conn.lspLanguage = lspLanguage;
  }

  private rebuildProviders(conn: ManagedLspConnection): void {
    if (!conn.rpc || !this.host.isCurrentConnection(conn)) return;
    const monaco = getMonacoInstance();
    if (!monaco) return;
    for (const disposable of conn.providerDisposables) disposable.dispose();
    conn.providerDisposables = [];
    conn.semanticRefreshCallbacks = [];
    conn.providerDisposables.push(
      monaco.editor.onDidCreateModel((model: monacoEditor.ITextModel) => {
        if (this.host.isCurrentConnection(conn))
          this.host.editorState.applyCachedDiagnostics(conn, model);
      }),
    );
    for (const model of monaco.editor.getModels())
      this.host.editorState.applyCachedDiagnostics(conn, model);

    const capabilities = effectiveLspCapabilities(
      conn.serverCapabilities,
      conn.dynamicRegistrations,
    );
    this.registerTypeScriptSuppression(conn, capabilities);
    conn.providerDisposables.push(
      ...withLspProviderRegistration(() =>
        registerLspProviders({
          rpc: conn.rpc as JsonRpcConnection,
          lspLanguage: conn.lspLanguage,
          serverCapabilities: capabilities,
          semanticRefreshCallbacks: conn.semanticRefreshCallbacks,
          getDocumentUri: (model) => connectionDocumentUri(model, conn),
          getModelUri: (uri) =>
            connectionModelUri(uri, conn, getMonacoInstance()?.editor.getModels() ?? []),
          ensureModelsExist: (uris) => this.host.editorState.ensureModelsExist(uris, conn),
        }),
      ),
    );
    conn.providersReady = true;
  }

  private registerTypeScriptSuppression(
    conn: ManagedLspConnection,
    capabilities: Record<string, unknown> | null,
  ): void {
    if (conn.lspLanguage !== "typescript") return;
    conn.providerDisposables.push(
      registerBuiltinTsSuppression(
        conn.ownerId,
        (model) => connectionDocumentUri(model as monacoEditor.ITextModel, conn) !== null,
        getLspMonacoProviderMethods(capabilities),
      ),
    );
  }
}

function configurationSection(
  configuration: Record<string, unknown>,
  section: string | undefined,
  lspLanguage: string,
): unknown {
  if (!section) return configuration;
  if (section === lspLanguage) return configuration;
  const languagePrefix = `${lspLanguage}.`;
  const normalizedSection = section.startsWith(languagePrefix)
    ? section.slice(languagePrefix.length)
    : section;
  let value: unknown = configuration;
  for (const part of normalizedSection.split(".")) {
    if (typeof value !== "object" || value === null || !(part in value)) return null;
    value = (value as Record<string, unknown>)[part];
  }
  return value;
}

function lspErrorMessage(error: unknown): string {
  if (error instanceof Error) return error.message || String(error);
  if (typeof error === "object" && error !== null) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message) return message;
  }
  return String(error);
}

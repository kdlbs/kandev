import { t } from "@/lib/i18n";
import { getWsBaseUrl, CLOSE_CODE_STATUS } from "./lsp-json-rpc";
import type { LspStatus } from "./lsp-json-rpc";
import type { LspReadyWorkspace, ManagedLspConnection } from "./lsp-client-types";
import type { LspClientEditorState } from "./lsp-client-editor-state";
import { LSP_RECONNECT_DELAYS_MS, LSP_RELEASE_ACK_TIMEOUT_MS } from "./lsp-client-config";
import { EMPTY_LSP_PROGRESS } from "./lsp-progress";
import { clearLspLeaseHint, saveLspLeaseHint } from "./lsp-client-storage";
import { nextLspControlRequestId, waitForLspControlAck } from "./lsp-client-control";
import type { LspClientProtocol } from "./lsp-client-protocol";

export type LspReadyMessage = {
  status?: string;
  error?: string;
  leaseId?: string;
  resumed?: boolean;
  initialized?: boolean;
  workspacePath?: string | null;
  workspaceUri?: string | null;
  repoSubpaths?: string[];
  capabilities?: Record<string, unknown>;
  registrations?: unknown[];
  progressTokens?: unknown[];
  progress?: unknown[];
};

type LspTransportState = {
  transportGeneration: number;
  bridgeStarted: boolean;
  terminalStatusReceived: boolean;
  statusHandler?: (event: MessageEvent) => void;
};

type LspTransportHost = {
  protocol: LspClientProtocol;
  editorState: LspClientEditorState;
  isCurrentConnection: (conn: ManagedLspConnection) => boolean;
  getStatus: (key: string) => LspStatus;
  setStatus: (key: string, status: LspStatus) => void;
  removeStatus: (key: string) => void;
  notifyChange: (key: string) => void;
  cleanupConnection: (conn: ManagedLspConnection) => void;
};

export class LspClientTransport {
  constructor(private readonly host: LspTransportHost) {}

  createSocket(
    sessionId: string,
    lspLanguage: string,
    continuityEnabled: boolean,
    leaseId: string | null,
  ): WebSocket {
    const url = `${getWsBaseUrl()}/lsp/${encodeURIComponent(sessionId)}`;
    const query = new URLSearchParams({ language: lspLanguage });
    if (continuityEnabled && leaseId) query.set("leaseId", leaseId);
    return new WebSocket(`${url}?${query.toString()}`);
  }

  open(conn: ManagedLspConnection): void {
    if (
      !this.host.isCurrentConnection(conn) ||
      (conn.explicitlyStopped && conn.releaseAfterConnect !== "stop")
    ) {
      return;
    }
    const ws = this.createSocket(
      conn.sessionId,
      conn.lspLanguage,
      conn.continuityEnabled,
      conn.leaseId,
    );
    conn.ws = ws;
    this.bind(conn, ws);
  }

  bind(conn: ManagedLspConnection, ws: WebSocket): void {
    const state: LspTransportState = {
      transportGeneration: ++conn.transportGeneration,
      bridgeStarted: false,
      terminalStatusReceived: false,
    };
    const statusHandler = (event: MessageEvent) => this.handleStatus(conn, ws, event, state);
    state.statusHandler = statusHandler;
    ws.onopen = () => {
      if (!this.isCurrentTransport(conn, ws, state) || conn.reconnecting) return;
      this.host.setStatus(conn.key, { state: "starting" });
    };
    ws.addEventListener("message", statusHandler);
    ws.onclose = (event) => this.handleClose(conn, ws, event, statusHandler, state);
    ws.onerror = () => this.handleError(conn, ws, state);
  }

  private handleStatus(
    conn: ManagedLspConnection,
    ws: WebSocket,
    event: MessageEvent,
    state: LspTransportState,
  ): void {
    if (state.bridgeStarted || !this.isCurrentTransport(conn, ws, state)) return;
    const data = this.parseReadyMessage(event.data);
    if (!data) return;
    if (data.status === "installing") {
      this.host.setStatus(conn.key, { state: "installing" });
      return;
    }
    if (data.status === "installed") {
      this.host.setStatus(conn.key, { state: "starting" });
      return;
    }
    if (data.status === "install_failed") {
      if (state.statusHandler) ws.removeEventListener("message", state.statusHandler);
      state.bridgeStarted = true;
      state.terminalStatusReceived = true;
      this.host.setStatus(conn.key, {
        state: "error",
        reason: data.error || t("lsp:installFailed"),
      });
      return;
    }
    if (data.status !== "ready") return;
    if (state.statusHandler) ws.removeEventListener("message", state.statusHandler);
    state.bridgeStarted = true;
    if (conn.continuityEnabled && data.leaseId) {
      conn.leaseId = data.leaseId;
      saveLspLeaseHint(conn.sessionId, conn.lspLanguage, data.leaseId);
      this.host.notifyChange(conn.key);
    }
    if (conn.releaseAfterConnect) {
      void this.release(conn, conn.releaseAfterConnect);
      return;
    }
    this.initializeOrResume(conn, data);
  }

  private parseReadyMessage(data: unknown): LspReadyMessage | null {
    try {
      return JSON.parse(data as string) as LspReadyMessage;
    } catch {
      return null;
    }
  }

  private initializeOrResume(conn: ManagedLspConnection, data: LspReadyMessage): void {
    const workspace: LspReadyWorkspace = {
      path: data.workspacePath ?? null,
      uri: data.workspaceUri ?? null,
      repositorySubpaths: data.repoSubpaths ?? [],
    };
    if (conn.continuityEnabled && data.resumed === true) {
      conn.reconnecting = true;
      void this.host.protocol.resume(conn, data, workspace);
      return;
    }
    conn.reconnecting = false;
    conn.reconnectAttempts = 0;
    conn.dynamicRegistrations.clear();
    void this.host.protocol.initialize(conn, conn.lspLanguage, workspace);
  }

  private isCurrentTransport(
    conn: ManagedLspConnection,
    ws: WebSocket,
    state: LspTransportState,
  ): boolean {
    return (
      this.host.isCurrentConnection(conn) &&
      conn.transportGeneration === state.transportGeneration &&
      conn.ws === ws
    );
  }

  private handleClose(
    conn: ManagedLspConnection,
    ws: WebSocket,
    event: CloseEvent,
    statusHandler: (event: MessageEvent) => void,
    state: LspTransportState,
  ): void {
    ws.removeEventListener("message", statusHandler);
    if (!this.isCurrentTransport(conn, ws, state)) return;
    if (this.handleStoppingClose(conn, event)) return;
    if (state.terminalStatusReceived) {
      this.host.cleanupConnection(conn);
      return;
    }
    if (this.shouldReconnect(conn, event)) {
      this.prepareForReconnect(conn);
      this.scheduleReconnect(conn);
      return;
    }
    this.handleTerminalClose(conn, event, state.bridgeStarted);
  }

  private handleStoppingClose(conn: ManagedLspConnection, event: CloseEvent): boolean {
    if (this.host.getStatus(conn.key).state !== "stopping") return false;
    if (conn.continuityEnabled && conn.releaseAfterConnect === "stop") {
      if (isAbnormalClose(event.code)) {
        conn.reconnecting = true;
        this.host.setStatus(conn.key, { state: "reconnecting" });
        this.scheduleReconnect(conn);
        return true;
      }
      conn.releaseAfterConnect = null;
      this.handleTerminalClose(conn, event, true);
      return true;
    }
    this.host.cleanupConnection(conn);
    this.host.setStatus(conn.key, { state: "disabled" });
    this.host.removeStatus(conn.key);
    return true;
  }

  private shouldReconnect(conn: ManagedLspConnection, event: CloseEvent): boolean {
    return (
      conn.continuityEnabled &&
      (!conn.explicitlyStopped || conn.releaseAfterConnect === "stop") &&
      isAbnormalClose(event.code)
    );
  }

  private handleTerminalClose(
    conn: ManagedLspConnection,
    event: CloseEvent,
    bridgeStarted: boolean,
  ): void {
    const statusFactory = CLOSE_CODE_STATUS[event.code];
    if (conn.continuityEnabled && (event.code === 4006 || event.code === 4010)) {
      clearLspLeaseHint(conn.sessionId, conn.lspLanguage);
      conn.leaseId = null;
    }
    this.host.cleanupConnection(conn);
    if (statusFactory) {
      this.host.setStatus(conn.key, statusFactory(event.reason));
      return;
    }
    const fallbackReason =
      bridgeStarted && !conn.continuityEnabled
        ? t("lsp:languageServerExited")
        : t("lsp:connectionClosed");
    this.host.setStatus(conn.key, { state: "error", reason: event.reason || fallbackReason });
  }

  private handleError(conn: ManagedLspConnection, ws: WebSocket, state: LspTransportState): void {
    if (!this.isCurrentTransport(conn, ws, state) || conn.continuityEnabled) return;
    const current = this.host.getStatus(conn.key);
    if (current.state !== "error" && current.state !== "unavailable") {
      this.host.setStatus(conn.key, { state: "error", reason: t("lsp:webSocketError") });
    }
  }

  prepareForReconnect(conn: ManagedLspConnection): void {
    if (!this.host.isCurrentConnection(conn)) return;
    for (const disposable of conn.providerDisposables) disposable.dispose();
    conn.providerDisposables = [];
    conn.providersReady = false;
    conn.semanticRefreshCallbacks = [];
    conn.rpc?.dispose();
    conn.rpc = null;
    conn.initialized = false;
    conn.protocolInitialized = false;
    conn.documentsSynced = false;
    conn.diagnosticsReady = false;
    this.host.editorState.clearConnectionDiagnostics(conn);
    conn.progress = EMPTY_LSP_PROGRESS;
    conn.registeredProgressTokens.clear();
    conn.reconnecting = true;
    this.host.setStatus(conn.key, { state: "reconnecting" });
  }

  scheduleReconnect(conn: ManagedLspConnection): void {
    if (!this.host.isCurrentConnection(conn) || conn.reconnectTimer) return;
    if (conn.reconnectAttempts >= LSP_RECONNECT_DELAYS_MS.length) {
      conn.reconnecting = false;
      if (conn.releaseAfterConnect) {
        conn.releaseAfterConnect = null;
        this.host.cleanupConnection(conn);
        this.host.setStatus(conn.key, { state: "error", reason: t("lsp:releaseFailed") });
      } else {
        this.host.setStatus(conn.key, { state: "error", reason: t("lsp:connectionClosed") });
      }
      return;
    }
    const delay = LSP_RECONNECT_DELAYS_MS[conn.reconnectAttempts];
    conn.reconnectAttempts++;
    conn.reconnectTimer = setTimeout(() => {
      conn.reconnectTimer = null;
      if (this.host.isCurrentConnection(conn)) this.open(conn);
    }, delay);
  }

  async release(conn: ManagedLspConnection, reason: "stop" | "editor_idle"): Promise<void> {
    if (!this.host.isCurrentConnection(conn) || conn.ws.readyState !== WebSocket.OPEN) return;
    const ws = conn.ws;
    const transportGeneration = conn.transportGeneration;
    const requestId = nextLspControlRequestId();
    const acknowledgement = await waitForLspControlAck(ws, requestId, "released", {
      timeoutMs: LSP_RELEASE_ACK_TIMEOUT_MS,
      send: () => ws.send(JSON.stringify({ kandev: "lsp", action: "release", reason, requestId })),
      cancelOnClose: true,
    });
    if (
      !this.host.isCurrentConnection(conn) ||
      conn.ws !== ws ||
      conn.transportGeneration !== transportGeneration
    ) {
      return;
    }
    if (!acknowledgement) {
      if (ws.readyState !== WebSocket.OPEN) return;
      conn.releaseAfterConnect = null;
      conn.reconnecting = false;
      this.host.cleanupConnection(conn);
      this.host.setStatus(conn.key, { state: "error", reason: t("lsp:releaseFailed") });
      return;
    }
    clearLspLeaseHint(conn.sessionId, conn.lspLanguage);
    conn.releaseAfterConnect = null;
    conn.reconnecting = false;
    this.host.cleanupConnection(conn);
    this.host.removeStatus(conn.key);
    this.host.notifyChange(conn.key);
  }
}

function isAbnormalClose(code: number): boolean {
  return code === 4009 || code === 1001 || code === 1005 || code === 1006;
}

import type { MarkerSeverity as MarkerSeverityType, IDisposable } from "monaco-editor";
import { getBackendConfig } from "@/lib/config";
import { getMonacoInstance } from "@/components/editors/monaco/monaco-init";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type LspStatus =
  | { state: "disabled" }
  | { state: "connecting" }
  | { state: "installing" }
  | { state: "starting" }
  | { state: "ready" }
  | { state: "stopping" }
  | { state: "unavailable"; reason: string }
  | { state: "error"; reason: string };

export type OpenDocument = { version: number; languageId: string };

export type LSPConnection = {
  ws: WebSocket;
  rpc: JsonRpcConnection | null;
  initialized: boolean;
  refCount: number;
  idleTimer: ReturnType<typeof setTimeout> | null;
  openDocuments: Map<string, OpenDocument>;
  providerDisposables: IDisposable[];
  serverCapabilities: Record<string, unknown> | null;
  workspacePath: string | null;
};

// ---------------------------------------------------------------------------
// Minimal JSON-RPC 2.0 client over WebSocket
// ---------------------------------------------------------------------------

type PendingRequest = { resolve: (value: unknown) => void; reject: (reason: unknown) => void };

export class JsonRpcConnection {
  private nextId = 1;
  private pending = new Map<number, PendingRequest>();
  private notificationHandlers = new Map<string, (params: unknown) => void>();
  private requestHandlers = new Map<string, (params: unknown) => unknown>();
  private messageHandler: ((event: MessageEvent) => void) | null = null;

  constructor(private ws: WebSocket) {}

  listen() {
    this.messageHandler = (event: MessageEvent) => {
      let msg: {
        jsonrpc?: string;
        id?: number;
        method?: string;
        params?: unknown;
        result?: unknown;
        error?: unknown;
      };
      try {
        msg = JSON.parse(event.data as string);
      } catch {
        return;
      }

      if (msg.id !== undefined && msg.method !== undefined) {
        // Server → client request
        const handler = this.requestHandlers.get(msg.method);
        if (handler) {
          try {
            const result = handler(msg.params);
            this.ws.send(JSON.stringify({ jsonrpc: "2.0", id: msg.id, result: result ?? null }));
          } catch (err) {
            this.ws.send(
              JSON.stringify({
                jsonrpc: "2.0",
                id: msg.id,
                error: { code: -32603, message: String(err) },
              }),
            );
          }
        } else {
          // Respond with empty result for unhandled server requests (e.g. workspace/configuration)
          this.ws.send(JSON.stringify({ jsonrpc: "2.0", id: msg.id, result: null }));
        }
      } else if (msg.id !== undefined) {
        // Response to our request
        const p = this.pending.get(msg.id);
        if (p) {
          this.pending.delete(msg.id);
          if (msg.error) p.reject(msg.error);
          else p.resolve(msg.result);
        }
      } else if (msg.method !== undefined) {
        // Notification from server
        this.notificationHandlers.get(msg.method)?.(msg.params);
      }
    };
    this.ws.addEventListener("message", this.messageHandler);
  }

  sendRequest(method: string, params: unknown): Promise<unknown> {
    const id = this.nextId++;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.ws.send(JSON.stringify({ jsonrpc: "2.0", id, method, params }));
    });
  }

  sendNotification(method: string, params: unknown): void {
    this.ws.send(JSON.stringify({ jsonrpc: "2.0", method, params }));
  }

  onNotification(method: string, handler: (params: unknown) => void): void {
    this.notificationHandlers.set(method, handler);
  }

  onRequest(method: string, handler: (params: unknown) => unknown): void {
    this.requestHandlers.set(method, handler);
  }

  dispose() {
    if (this.messageHandler) {
      this.ws.removeEventListener("message", this.messageHandler);
      this.messageHandler = null;
    }
    for (const p of this.pending.values()) {
      p.reject(new Error("Connection disposed"));
    }
    this.pending.clear();
    this.notificationHandlers.clear();
    this.requestHandlers.clear();
  }
}

// ---------------------------------------------------------------------------
// LSP ↔ Monaco type conversions
// ---------------------------------------------------------------------------

export type LspPosition = { line: number; character: number };
export type LspRange = { start: LspPosition; end: LspPosition };

export function toMonacoRange(r: LspRange): {
  startLineNumber: number;
  startColumn: number;
  endLineNumber: number;
  endColumn: number;
} {
  return {
    startLineNumber: r.start.line + 1,
    startColumn: r.start.character + 1,
    endLineNumber: r.end.line + 1,
    endColumn: r.end.character + 1,
  };
}

// LSP DiagnosticSeverity → Monaco MarkerSeverity
export function toMonacoSeverity(lspSeverity: number | undefined): MarkerSeverityType {
  const monaco = getMonacoInstance();
  if (!monaco) return 8 as MarkerSeverityType; // Error fallback
  switch (lspSeverity) {
    case 1:
      return monaco.MarkerSeverity.Error;
    case 2:
      return monaco.MarkerSeverity.Warning;
    case 3:
      return monaco.MarkerSeverity.Info;
    case 4:
      return monaco.MarkerSeverity.Hint;
    default:
      return monaco.MarkerSeverity.Info;
  }
}

// ---------------------------------------------------------------------------
// Connection helpers
// ---------------------------------------------------------------------------

export function getWsBaseUrl(): string {
  try {
    const backendUrl = getBackendConfig().apiBaseUrl;
    const url = new URL(backendUrl);
    const protocol = url.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${url.host}`;
  } catch {
    return "ws://localhost:8080";
  }
}

/** Map WebSocket close codes to LSP status for pre-bridge failures. */
export const CLOSE_CODE_STATUS: Record<number, (reason: string) => LspStatus> = {
  4001: (reason) => ({ state: "unavailable", reason: reason || "Language server not found" }),
  4002: () => ({ state: "unavailable", reason: "No active workspace" }),
  4003: (reason) => ({ state: "error", reason: reason || "Install failed" }),
};

/** LSP client capabilities sent during initialization. */
export const LSP_CLIENT_CAPABILITIES = {
  textDocument: {
    synchronization: {
      dynamicRegistration: false,
      willSave: false,
      didSave: true,
      willSaveWaitUntil: false,
    },
    completion: {
      dynamicRegistration: false,
      completionItem: {
        snippetSupport: true,
        commitCharactersSupport: true,
        documentationFormat: ["markdown", "plaintext"],
        deprecatedSupport: true,
        preselectSupport: true,
      },
      contextSupport: true,
    },
    hover: { dynamicRegistration: false, contentFormat: ["markdown", "plaintext"] },
    definition: { dynamicRegistration: false },
    references: { dynamicRegistration: false },
    signatureHelp: {
      dynamicRegistration: false,
      signatureInformation: {
        documentationFormat: ["markdown", "plaintext"],
        parameterInformation: { labelOffsetSupport: true },
      },
    },
    publishDiagnostics: { relatedInformation: true },
    semanticTokens: {
      dynamicRegistration: false,
      requests: { full: true },
      tokenTypes: [
        "namespace",
        "type",
        "class",
        "enum",
        "interface",
        "struct",
        "typeParameter",
        "parameter",
        "variable",
        "property",
        "enumMember",
        "event",
        "function",
        "method",
        "macro",
        "keyword",
        "modifier",
        "comment",
        "string",
        "number",
        "regexp",
        "operator",
        "decorator",
      ],
      tokenModifiers: [
        "declaration",
        "definition",
        "readonly",
        "static",
        "deprecated",
        "abstract",
        "async",
        "modification",
        "documentation",
        "defaultLibrary",
      ],
      formats: ["relative"],
      overlappingTokenSupport: false,
      multilineTokenSupport: false,
    },
  },
  workspace: {
    configuration: true,
    didChangeConfiguration: { dynamicRegistration: false },
    semanticTokens: { refreshSupport: true },
  },
} as const;

// ---------------------------------------------------------------------------
// Language mapping helpers
// ---------------------------------------------------------------------------

export function toLspLanguage(monacoLanguage: string): string | null {
  const map: Record<string, string> = {
    typescript: "typescript",
    javascript: "typescript",
    typescriptreact: "typescript",
    javascriptreact: "typescript",
    go: "go",
    rust: "rust",
    python: "python",
  };
  return map[monacoLanguage] ?? null;
}

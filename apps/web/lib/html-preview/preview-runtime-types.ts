export const PREVIEW_RUNTIME_PROTOCOL_VERSION = 1;

export type PreviewEventType = "click" | "input" | "change" | "submit" | "keydown";

export type PreviewEvent = {
  type: PreviewEventType;
  nodeId: string;
  value?: string;
  checked?: boolean;
  key?: string;
};

export type PreviewSnapshotNode = {
  id: string;
  tagName: string;
  attributes: Record<string, string>;
  styles: Record<string, string>;
  text?: string;
  children: PreviewSnapshotNode[];
  eventTypes: PreviewEventType[];
};

export type PreviewResource = {
  token: string;
  content: string;
  mediaType: string;
};

export type PreviewDiagnostic = {
  code: PreviewRuntimeFailureCode;
  level: "warning" | "error";
};

export type PreviewSnapshot = {
  protocolVersion: typeof PREVIEW_RUNTIME_PROTOCOL_VERSION;
  root: PreviewSnapshotNode;
  resources: PreviewResource[];
  diagnostics: PreviewDiagnostic[];
};

export type PreviewRuntimeFailureCode =
  | "runtime-error"
  | "unsupported-capability"
  | "budget-exceeded"
  | "malformed-message"
  | "initialization-failed"
  | "disposed";

export class PreviewRuntimeError extends Error {
  readonly code: PreviewRuntimeFailureCode;

  constructor(code: PreviewRuntimeFailureCode) {
    super(code);
    this.name = "PreviewRuntimeError";
    this.code = code;
  }
}

export type PreviewRuntimeOptions = {
  instructionBudget: number;
  wallClockBudgetMs: number;
  memoryLimitBytes: number;
  maxStackSizeBytes: number;
  maxTimers: number;
  maxEventQueue: number;
  maxSnapshotBytes: number;
};

export type PreviewRuntimeLoadRequest = {
  protocolVersion: typeof PREVIEW_RUNTIME_PROTOCOL_VERSION;
  type: "load";
  generation: number;
  source: string;
};

export type PreviewRuntimeDispatchRequest = {
  protocolVersion: typeof PREVIEW_RUNTIME_PROTOCOL_VERSION;
  type: "dispatch";
  generation: number;
  event: PreviewEvent;
};

export type PreviewRuntimeDisposeRequest = {
  protocolVersion: typeof PREVIEW_RUNTIME_PROTOCOL_VERSION;
  type: "dispose";
  generation: number;
};

export type PreviewRuntimeRequest =
  | PreviewRuntimeLoadRequest
  | PreviewRuntimeDispatchRequest
  | PreviewRuntimeDisposeRequest;

export type PreviewRuntimeReadyResponse = {
  protocolVersion: typeof PREVIEW_RUNTIME_PROTOCOL_VERSION;
  type: "ready" | "snapshot";
  generation: number;
  snapshot: PreviewSnapshot;
};

export type PreviewRuntimeFailedResponse = {
  protocolVersion: typeof PREVIEW_RUNTIME_PROTOCOL_VERSION;
  type: "failed";
  generation: number;
  failure: PreviewDiagnostic;
};

export type PreviewRuntimeResponse = PreviewRuntimeReadyResponse | PreviewRuntimeFailedResponse;

export type PreviewRuntimeSession = {
  load(source: string): Promise<PreviewSnapshot>;
  dispatch(event: PreviewEvent): Promise<PreviewSnapshot>;
  dispose(): Promise<void>;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

export function isPreviewRuntimeRequest(value: unknown): value is PreviewRuntimeRequest {
  if (!isRecord(value) || value.protocolVersion !== PREVIEW_RUNTIME_PROTOCOL_VERSION) return false;
  if (value.type === "load")
    return typeof value.generation === "number" && typeof value.source === "string";
  if (value.type === "dispose") return typeof value.generation === "number";
  if (value.type !== "dispatch" || typeof value.generation !== "number" || !isRecord(value.event)) {
    return false;
  }

  return (
    typeof value.event.nodeId === "string" &&
    typeof value.event.type === "string" &&
    ["click", "input", "change", "submit", "keydown"].includes(value.event.type)
  );
}

export function isPreviewRuntimeResponse(value: unknown): value is PreviewRuntimeResponse {
  if (!isRecord(value) || value.protocolVersion !== PREVIEW_RUNTIME_PROTOCOL_VERSION) return false;
  if (typeof value.generation !== "number") return false;
  if (value.type === "failed") {
    return isRecord(value.failure) && typeof value.failure.code === "string";
  }
  return (value.type === "ready" || value.type === "snapshot") && isRecord(value.snapshot);
}

export function findPreviewSnapshotNode(
  node: PreviewSnapshotNode,
  id: string,
): PreviewSnapshotNode | undefined {
  if (node.attributes.id === id) return node;
  for (const child of node.children) {
    const match = findPreviewSnapshotNode(child, id);
    if (match) return match;
  }
  return undefined;
}

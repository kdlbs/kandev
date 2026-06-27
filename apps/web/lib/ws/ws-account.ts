export const WS_ACCOUNT_MAX_ENTRIES = 5000;

export interface WsAccountEnvelope {
  connection_id?: string;
  connection_seq?: number;
  session_seq?: number;
  type?: string;
  action?: string;
  payload?: unknown;
}

export interface WsAccountEntry {
  connectionSeq: number;
  sessionSeq?: number;
  type: string;
  action: string;
  sessionId: string | null;
  receivedAt: number;
}

export interface WsAccountReceivedEvent {
  connectionSeq: number;
  sessionSeq?: number;
  action: string;
  sessionId: string | null;
  type: string;
}

export interface WsAccountSessionSnapshot {
  processedSeqs: number[];
  gaps: number[];
  minSeq: number | null;
  maxSeq: number | null;
}

export interface WsAccountSnapshot extends WsAccountSessionSnapshot {
  connectionId: string | null;
  receivedEvents: WsAccountReceivedEvent[];
  bySession: Record<string, WsAccountSessionSnapshot>;
}

type WsAccountWindow = Window & {
  __KANDEV_E2E_EXPOSE_STORE__?: boolean;
  __kandev_ws_account__?: () => WsAccountSnapshot;
  __kandev_ws_account_clear__?: () => void;
};

export function detectGaps(processed: number[]): number[] {
  if (processed.length < 2) return [];
  const min = processed[0];
  const max = processed[processed.length - 1];
  const seen = new Set(processed);
  const gaps: number[] = [];
  for (let seq = min + 1; seq < max; seq++) {
    if (!seen.has(seq)) gaps.push(seq);
  }
  return gaps;
}

class WsAccountBucket {
  private entries = new Map<number, WsAccountEntry>();

  constructor(private readonly maxEntries: number) {}

  record(seq: number, entry: WsAccountEntry): void {
    if (this.entries.has(seq)) {
      this.entries.delete(seq);
    }
    this.entries.set(seq, entry);
    while (this.entries.size > this.maxEntries) {
      const oldest = this.entries.keys().next().value;
      if (oldest === undefined) break;
      this.entries.delete(oldest);
    }
  }

  clear(): void {
    this.entries.clear();
  }

  snapshot(): WsAccountSessionSnapshot & { receivedEvents: WsAccountReceivedEvent[] } {
    const processedSeqs = Array.from(this.entries.keys()).sort((a, b) => a - b);
    const receivedEvents = processedSeqs.map((seq) => {
      const entry = this.entries.get(seq);
      return {
        connectionSeq: entry?.connectionSeq ?? seq,
        sessionSeq: entry?.sessionSeq,
        action: entry?.action ?? "",
        sessionId: entry?.sessionId ?? null,
        type: entry?.type ?? "",
      };
    });
    return {
      processedSeqs,
      gaps: detectGaps(processedSeqs),
      minSeq: processedSeqs[0] ?? null,
      maxSeq: processedSeqs.at(-1) ?? null,
      receivedEvents,
    };
  }
}

export class WsAccount {
  private connectionId: string | null = null;
  private connection = new WsAccountBucket(this.maxEntries);
  private bySession = new Map<string, WsAccountBucket>();

  constructor(private readonly maxEntries: number = WS_ACCOUNT_MAX_ENTRIES) {}

  recordEnvelope(envelope: WsAccountEnvelope): void {
    const entry = toAccountEntry(envelope);
    if (!entry) return;
    const connectionId = envelope.connection_id ?? null;
    if (connectionId && connectionId !== this.connectionId) {
      this.connection.clear();
      this.bySession.clear();
      this.connectionId = connectionId;
    }
    this.connection.record(entry.connectionSeq, entry);
    if (entry.sessionId && entry.sessionSeq && entry.sessionSeq > 0) {
      this.sessionBucket(entry.sessionId).record(entry.sessionSeq, entry);
    }
  }

  snapshot(): WsAccountSnapshot {
    const connectionSnapshot = this.connection.snapshot();
    const bySession: Record<string, WsAccountSessionSnapshot> = {};
    for (const [sessionId, bucket] of this.bySession) {
      const snapshot = bucket.snapshot();
      bySession[sessionId] = {
        processedSeqs: snapshot.processedSeqs,
        gaps: snapshot.gaps,
        minSeq: snapshot.minSeq,
        maxSeq: snapshot.maxSeq,
      };
    }
    return {
      connectionId: this.connectionId,
      processedSeqs: connectionSnapshot.processedSeqs,
      gaps: connectionSnapshot.gaps,
      minSeq: connectionSnapshot.minSeq,
      maxSeq: connectionSnapshot.maxSeq,
      receivedEvents: connectionSnapshot.receivedEvents,
      bySession,
    };
  }

  clear(): void {
    this.connectionId = null;
    this.connection.clear();
    this.bySession.clear();
  }

  private sessionBucket(sessionId: string): WsAccountBucket {
    const existing = this.bySession.get(sessionId);
    if (existing) return existing;
    const bucket = new WsAccountBucket(this.maxEntries);
    this.bySession.set(sessionId, bucket);
    return bucket;
  }
}

const globalAccount = new WsAccount();

export function recordParsedWsEnvelope(envelope: unknown): void {
  const win = getWindow();
  if (!win || !wsAccountEnabled(win)) return;
  installWsAccountGlobals(win);
  globalAccount.recordEnvelope(envelope as WsAccountEnvelope);
}

export function installWsAccountGlobalsForE2E(): void {
  const win = getWindow();
  if (!win || !wsAccountEnabled(win)) return;
  installWsAccountGlobals(win);
}

export function installWsAccountGlobals(win: WsAccountWindow): void {
  if (win.__kandev_ws_account__) return;
  win.__kandev_ws_account__ = () => globalAccount.snapshot();
  win.__kandev_ws_account_clear__ = () => globalAccount.clear();
}

function wsAccountEnabled(win: WsAccountWindow): boolean {
  return win.__KANDEV_E2E_EXPOSE_STORE__ === true;
}

function getWindow(): WsAccountWindow | null {
  if (typeof window === "undefined") return null;
  return window as WsAccountWindow;
}

function toAccountEntry(envelope: WsAccountEnvelope): WsAccountEntry | null {
  const connectionSeq = envelope.connection_seq;
  if (typeof connectionSeq !== "number" || !Number.isFinite(connectionSeq) || connectionSeq <= 0) {
    return null;
  }
  return {
    connectionSeq,
    sessionSeq: normalizeSeq(envelope.session_seq),
    type: envelope.type ?? "",
    action: envelope.action ?? "",
    sessionId: extractSessionId(envelope.payload),
    receivedAt: Date.now(),
  };
}

function normalizeSeq(seq: number | undefined): number | undefined {
  return typeof seq === "number" && Number.isFinite(seq) && seq > 0 ? seq : undefined;
}

function extractSessionId(payload: unknown): string | null {
  if (!payload || typeof payload !== "object") return null;
  const value = (payload as { session_id?: unknown }).session_id;
  return typeof value === "string" && value !== "" ? value : null;
}

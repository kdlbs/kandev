import {
  MAX_UNCLAIMED_BYTES,
  MAX_UNCLAIMED_FACTS,
  SHELL_OUTCOMES_BEHAVIOR,
  SHELL_OUTCOMES_VERSION,
  UNCLAIMED_FACT_LIFETIME_MS,
  type BehaviorConfig,
  decodeShellClaims,
  type ObservationPayload,
  type ShellClaim,
  type ShellTerminalPayload,
  type ShellTerminalStatus,
} from "../protocol.js";
import type { Behavior, BehaviorContext } from "./types.js";

interface ShellFact {
  readonly shellId: string;
  readonly nativeSessionId: string;
  readonly startedAt?: number;
  status?: ShellTerminalStatus;
  exitCode?: number;
  completedAt?: number;
  lastSeen: number;
  readonly emitted: Set<string>;
}

/**
 * Observes native shell terminal facts for jobs that ACP already confirmed.
 * A fact is emitted only after its exact claim arrives; unrelated foreground
 * shells never gain a claim and cannot create background work.
 */
export class ShellOutcomesBehavior implements Behavior {
  readonly id = SHELL_OUTCOMES_BEHAVIOR;
  readonly version = SHELL_OUTCOMES_VERSION;
  readonly eventTypes = ["shell.created", "shell.exited", "shell.deleted"] as const;

  private readonly facts = new Map<string, ShellFact>();
  private readonly claims = new Map<string, ShellClaim>();
  private context: BehaviorContext | undefined;
  private pruneTimer: ReturnType<typeof setInterval> | undefined;

  start(context: BehaviorContext): void {
    this.context = context;
    this.pruneTimer = setInterval(() => this.prune(), 15_000);
    this.pruneTimer.unref?.();
  }

  configure(config: BehaviorConfig): void {
    const claims = decodeShellClaims(config.data);
    this.claims.clear();
    for (const claim of claims) {
      this.claims.set(claim.shellId, claim);
    }
    for (const claim of claims) {
      this.tryEmit(claim.shellId);
    }
  }

  observationKey(
    payload: ObservationPayload,
    nativeSessionId: string,
    nativeGeneration: number,
  ): string {
    if (!isShellTerminalPayload(payload)) return "";
    return [
      nativeSessionId,
      String(nativeGeneration),
      payload.shellId,
      payload.status,
      payload.exitCode === undefined ? "-" : String(payload.exitCode),
    ].join(":");
  }

  handleEvent(event: {
    readonly type: string;
    readonly data?: unknown;
    readonly created?: unknown;
  }): void {
    const data = asRecord(event.data);
    if (event.type === "shell.created") {
      this.observeCreated(data);
      return;
    }
    if (event.type === "shell.exited") {
      this.observeExited(data, numberValue(event, "created"));
      return;
    }
    if (event.type === "shell.deleted") {
      const shellId = stringValue(data?.id);
      if (shellId) this.facts.delete(shellId);
    }
  }

  dispose(): void {
    if (this.pruneTimer) clearInterval(this.pruneTimer);
    this.pruneTimer = undefined;
    this.facts.clear();
    this.claims.clear();
  }

  private observeCreated(data: Record<string, unknown> | undefined): void {
    const info = asRecord(data?.info);
    const shellId = stringValue(info?.id);
    const metadata = asRecord(info?.metadata);
    const nativeSessionId = stringValue(metadata?.sessionID);
    if (!shellId || !nativeSessionId) return;
    const now = Date.now();
    const startedAt = numberValue(info?.time, "started") ?? now;
    const existing = this.facts.get(shellId);
    if (existing) {
      existing.lastSeen = now;
      return;
    }
    this.facts.set(shellId, {
      shellId,
      nativeSessionId,
      startedAt,
      lastSeen: now,
      emitted: new Set(),
    });
    this.trimFacts();
  }

  private observeExited(data: Record<string, unknown> | undefined, eventCreated?: number): void {
    const shellId = stringValue(data?.id);
    const status = terminalStatus(data?.status);
    if (!shellId || !status) return;
    const fact = this.facts.get(shellId);
    if (!fact) return;
    fact.status = status;
    const exitCode = integerValue(data?.exit);
    if (exitCode !== undefined) fact.exitCode = exitCode;
    fact.completedAt = eventCreated ?? Date.now();
    fact.lastSeen = Date.now();
    this.tryEmit(shellId);
  }

  private tryEmit(shellId: string): void {
    const fact = this.facts.get(shellId);
    const claim = this.claims.get(shellId);
    const context = this.context;
    if (!fact || !claim || !context || !fact.status) return;
    if (fact.nativeSessionId !== claim.nativeSessionId) {
      context.log("claim_session_mismatch", { shellId });
      return;
    }
    const eventId = stableEventID(claim, fact);
    if (fact.emitted.has(eventId)) return;
    fact.emitted.add(eventId);
    const payload: ObservationPayload = {
      kind: "shell.terminal",
      shellId,
      status: fact.status,
      ...(fact.exitCode !== undefined ? { exitCode: fact.exitCode } : {}),
      ...(fact.startedAt !== undefined ? { startedAt: fact.startedAt } : {}),
      ...(fact.completedAt !== undefined ? { completedAt: fact.completedAt } : {}),
    };
    context.emit(payload, claim.nativeSessionId, claim.nativeGeneration);
  }

  private prune(): void {
    const cutoff = Date.now() - UNCLAIMED_FACT_LIFETIME_MS;
    for (const [shellId, fact] of this.facts) {
      if (fact.lastSeen >= cutoff) continue;
      if (!this.claims.has(shellId)) {
        this.facts.delete(shellId);
      } else if (fact.emitted.size > 0) {
        this.facts.delete(shellId);
      }
    }
  }

  private trimFacts(): void {
    let bytes = 0;
    for (const fact of this.facts.values()) bytes += approximateBytes(fact);
    while (this.facts.size > MAX_UNCLAIMED_FACTS || bytes > MAX_UNCLAIMED_BYTES) {
      const oldest = this.facts.keys().next().value;
      if (!oldest) break;
      const fact = this.facts.get(oldest);
      if (!fact) break;
      bytes -= approximateBytes(fact);
      this.facts.delete(oldest);
      this.context?.log("unclaimed_fact_gap", { shellId: oldest });
    }
  }
}

function stableEventID(claim: ShellClaim, fact: ShellFact): string {
  return [
    SHELL_OUTCOMES_BEHAVIOR,
    String(SHELL_OUTCOMES_VERSION),
    claim.nativeSessionId,
    String(claim.nativeGeneration),
    claim.shellId,
    fact.status ?? "unknown",
    fact.exitCode === undefined ? "-" : String(fact.exitCode),
  ].join(":");
}

function approximateBytes(fact: ShellFact): number {
  return 256 + fact.shellId.length + fact.nativeSessionId.length;
}

function isShellTerminalPayload(value: ObservationPayload): value is ShellTerminalPayload {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const record = value as Record<string, unknown>;
  return (
    record.kind === "shell.terminal" &&
    typeof record.shellId === "string" &&
    typeof record.status === "string"
  );
}

function terminalStatus(value: unknown): ShellTerminalStatus | undefined {
  if (value === "exited" || value === "timeout" || value === "killed") return value;
  return undefined;
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return undefined;
  return value as Record<string, unknown>;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

function integerValue(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) ? value : undefined;
}

function numberValue(value: unknown, key: string): number | undefined {
  const record = asRecord(value);
  const raw = record?.[key];
  return typeof raw === "number" && Number.isFinite(raw) ? raw : undefined;
}

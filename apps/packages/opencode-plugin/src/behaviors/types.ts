import type { BehaviorConfig, ObservationPayload } from "../protocol.js";

export interface BehaviorContext {
  readonly signal: AbortSignal;
  emit(payload: ObservationPayload, nativeSessionId: string, nativeGeneration: number): void;
  log: (event: string, fields?: Record<string, unknown>) => void;
}

/**
 * One compiled behavior module. The core owns transport, configuration
 * revisioning, and lifecycle; a behavior owns its payload and claim semantics.
 */
export interface Behavior {
  readonly id: string;
  readonly version: number;
  readonly eventTypes: readonly string[];
  start(context: BehaviorContext): void | Promise<void>;
  configure(config: BehaviorConfig): void;
  handleEvent?: (event: {
    readonly type: string;
    readonly data?: unknown;
    readonly created?: unknown;
  }) => void;
  /** Stable payload identity inside this behavior version. */
  observationKey(
    payload: ObservationPayload,
    nativeSessionId: string,
    nativeGeneration: number,
  ): string;
  dispose?(): void;
}

import {
  RETRY_INITIAL_MS,
  RETRY_MAX_MS,
  type BehaviorConfig,
  type ConfigResponse,
  type HelloResponse,
  type Observation,
  type ObservationPayload,
} from "../protocol.js";
import { bootstrapOption, readBootstrapFile } from "./bootstrap.js";
import { BridgeClient } from "./client.js";
import { Outbox } from "./outbox.js";
import type { Behavior } from "../behaviors/types.js";

export interface OpenCodePluginEvent {
  readonly type: string;
  readonly data?: unknown;
}

export interface OpenCodePluginContext {
  readonly app: { readonly version: string };
  readonly options: unknown;
  readonly event: {
    subscribe(options: {
      readonly types: string[];
      readonly signal: AbortSignal;
    }): AsyncIterable<OpenCodePluginEvent>;
  };
}

export type Cleanup = () => Promise<void>;

/**
 * Installs the private bridge, waits for a real native subscription barrier,
 * then negotiates behavior versions and applies revisioned behavior config. Failures
 * before readiness leave the ordinary ACP conversation untouched.
 */
export async function startCore(
  context: OpenCodePluginContext,
  behaviors: readonly Behavior[],
): Promise<Cleanup> {
  const bootstrap = await readBootstrapFile(bootstrapOption(context.options));
  const controller = new AbortController();
  const client = new BridgeClient({
    endpoint: bootstrap.endpoint,
    credential: bootstrap.credential,
    leaseId: bootstrap.leaseId,
    artifactHash: bootstrap.artifactHash,
    runtimeVersion: context.app?.version ?? "unknown",
  });
  const log = (event: string, fields?: Record<string, unknown>) => {
    const details = fields ? ` ${JSON.stringify(fields)}` : "";
    console.warn(`[kandev-opencode-plugin] ${event}${details}`);
  };
  const outbox = new Outbox({
    signal: controller.signal,
    log,
    send: (events) => client.events(events, controller.signal),
  });
  let sequence = 0;

  const emit = (
    behavior: Behavior,
    payload: ObservationPayload,
    nativeSessionId: string,
    nativeGeneration: number,
  ) => {
    if (!nativeSessionId || nativeGeneration <= 0) {
      log("observation_unattributed", { behavior: behavior.id });
      return;
    }
    sequence += 1;
    const key = behavior.observationKey(payload, nativeSessionId, nativeGeneration);
    if (!key) {
      log("observation_unkeyed", { behavior: behavior.id });
      return;
    }
    const observation: Observation = {
      eventId: eventID(bootstrap.leaseId, behavior, key),
      behaviorId: behavior.id,
      behaviorVersion: behavior.version,
      sequence,
      nativeSessionId,
      nativeGeneration,
      payload,
    };
    outbox.enqueue(observation);
  };

  const started = deferred<void>();
  const subscription = runSubscription(context, behaviors, controller.signal, log, () =>
    started.resolve(),
  );
  subscription.finished.catch(() => undefined);
  await started.promise;

  let hello: HelloResponse;
  try {
    hello = await client.hello(
      behaviors.map((behavior) => ({ id: behavior.id, version: behavior.version })),
    );
  } catch (error) {
    controller.abort();
    await subscription.finished.catch(() => undefined);
    await outbox.close();
    throw error;
  }
  if (!hello.ready) {
    controller.abort();
    await subscription.finished.catch(() => undefined);
    await outbox.close();
    log("host_not_ready");
    return async () => undefined;
  }

  const negotiated = negotiate(behaviors, hello, log);
  for (const behavior of negotiated) {
    await behavior.start({
      signal: controller.signal,
      emit: (payload, sessionID, generation) => emit(behavior, payload, sessionID, generation),
      log,
    });
  }

  const configLoop = runConfigLoop(client, negotiated, controller.signal, log);

  return async () => {
    controller.abort();
    await Promise.allSettled([subscription.finished, configLoop]);
    for (const behavior of negotiated) behavior.dispose?.();
    await outbox.close();
  };
}

function negotiate(
  behaviors: readonly Behavior[],
  hello: HelloResponse,
  log: (event: string, fields?: Record<string, unknown>) => void,
): Behavior[] {
  const enabled = new Map(hello.behaviors.map((behavior) => [behavior.id, behavior.version]));
  const result: Behavior[] = [];
  for (const behavior of behaviors) {
    if (enabled.get(behavior.id) === behavior.version) {
      result.push(behavior);
    } else if (enabled.has(behavior.id)) {
      log("behavior_version_mismatch", { behavior: behavior.id });
    }
  }
  return result;
}

interface SubscriptionRun {
  readonly finished: Promise<void>;
}

function runSubscription(
  context: OpenCodePluginContext,
  behaviors: readonly Behavior[],
  signal: AbortSignal,
  log: (event: string, fields?: Record<string, unknown>) => void,
  onStarted: () => void,
): SubscriptionRun {
  const types = [...new Set(behaviors.flatMap((behavior) => [...behavior.eventTypes]))];
  const handlers = new Map<string, Behavior[]>();
  for (const behavior of behaviors) {
    for (const type of behavior.eventTypes) {
      const list = handlers.get(type) ?? [];
      list.push(behavior);
      handlers.set(type, list);
    }
  }
  const finished = (async () => {
    try {
      const stream = context.event.subscribe({ types, signal });
      const iterator = stream[Symbol.asyncIterator]();
      // Starting next() installs the native subscription before readiness is
      // reported. This is the barrier, not a fixed sleep.
      let next = iterator.next();
      onStarted();
      for (;;) {
        const result = await next;
        if (result.done) return;
        const event = result.value;
        for (const behavior of handlers.get(event.type) ?? []) {
          try {
            behavior.handleEvent?.(event);
          } catch {
            log("behavior_event_failed", { behavior: behavior.id, type: event.type });
          }
        }
        next = iterator.next();
      }
    } catch {
      if (!signal.aborted) log("subscription_failed");
      onStarted();
    }
  })();
  return { finished };
}

async function runConfigLoop(
  client: BridgeClient,
  behaviors: readonly Behavior[],
  signal: AbortSignal,
  log: (event: string, fields?: Record<string, unknown>) => void,
): Promise<void> {
  let revision = 0;
  let attempt = 0;
  while (!signal.aborted) {
    try {
      const response = await client.config(revision, signal);
      applyConfig(response, behaviors, log);
      const unchanged = response.revision === revision;
      revision = response.revision;
      attempt = 0;
      if (unchanged) await sleep(250, signal);
    } catch {
      if (signal.aborted) return;
      attempt += 1;
      const delay = Math.min(RETRY_MAX_MS, RETRY_INITIAL_MS * 2 ** (attempt - 1));
      await sleep(delay + Math.floor(delay * 0.25 * Math.random()), signal);
    }
  }
}

function applyConfig(
  response: ConfigResponse,
  behaviors: readonly Behavior[],
  log: (event: string, fields?: Record<string, unknown>) => void,
): void {
  for (const behavior of behaviors) {
    const config = response.behaviors[behavior.id];
    if (config && config.version !== behavior.version) {
      log("config_version_mismatch", { behavior: behavior.id });
      continue;
    }
    const applied: BehaviorConfig = config ?? { version: behavior.version };
    try {
      behavior.configure(applied);
    } catch {
      log("behavior_config_failed", { behavior: behavior.id });
    }
  }
}

function eventID(leaseId: string, behavior: Behavior, key: string): string {
  return `${leaseId}:${behavior.id}:${behavior.version}:${key}`;
}

function deferred<T>(): {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (error: unknown) => void;
} {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(finish, ms);
    const onAbort = () => finish();
    function finish() {
      clearTimeout(timer);
      signal.removeEventListener("abort", onAbort);
      resolve();
    }
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

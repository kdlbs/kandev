import {
  PROTOCOL_VERSION,
  ProtocolError,
  decodeConfigResponse,
  decodeEventResponse,
  decodeHelloResponse,
  type ConfigResponse,
  type EventRequest,
  type EventResponse,
  type HelloRequest,
  type HelloResponse,
  type Observation,
} from "../protocol.js";

export interface BridgeClientOptions {
  readonly endpoint: string;
  readonly credential: string;
  readonly leaseId: string;
  readonly artifactHash: string;
  readonly runtimeVersion: string;
  readonly requestTimeoutMs?: number;
}

const CONFIG_POLL_TIMEOUT_MS = 20_000;

/**
 * Small loopback client for the launch host. Redirects are rejected and every
 * request carries the launch-scoped bearer credential.
 */
export class BridgeClient {
  private readonly endpoint: string;
  private readonly credential: string;
  private readonly leaseId: string;
  private readonly artifactHash: string;
  private readonly runtimeVersion: string;
  private readonly requestTimeoutMs: number;

  constructor(options: BridgeClientOptions) {
    this.endpoint = options.endpoint;
    this.credential = options.credential;
    this.leaseId = options.leaseId;
    this.artifactHash = options.artifactHash;
    this.runtimeVersion = options.runtimeVersion;
    this.requestTimeoutMs = options.requestTimeoutMs ?? 10_000;
  }

  async hello(behaviors: readonly { id: string; version: number }[]): Promise<HelloResponse> {
    const body: HelloRequest = {
      protocolVersion: PROTOCOL_VERSION,
      leaseId: this.leaseId,
      artifactHash: this.artifactHash,
      runtime: { family: "opencode", version: this.runtimeVersion },
      behaviors,
    };
    const value = await this.request("/v1/hello", {
      method: "POST",
      body: JSON.stringify(body),
    });
    return decodeHelloResponse(value, this.leaseId);
  }

  async config(revision: number, signal: AbortSignal): Promise<ConfigResponse> {
    const query = new URLSearchParams({ lease: this.leaseId, revision: String(revision) });
    const value = await this.request(`/v1/config?${query.toString()}`, {
      method: "GET",
      signal,
      timeoutMs: CONFIG_POLL_TIMEOUT_MS,
    });
    return decodeConfigResponse(value, this.leaseId);
  }

  async events(events: readonly Observation[], signal?: AbortSignal): Promise<EventResponse> {
    const body: EventRequest = {
      protocolVersion: PROTOCOL_VERSION,
      leaseId: this.leaseId,
      producerId: this.leaseId,
      events,
    };
    const value = await this.request("/v1/events", {
      method: "POST",
      body: JSON.stringify(body),
      signal,
    });
    return decodeEventResponse(value);
  }

  private async request(
    path: string,
    options: {
      method: string;
      body?: string | undefined;
      signal?: AbortSignal | undefined;
      timeoutMs?: number;
    },
  ): Promise<unknown> {
    const controller = new AbortController();
    const timeout = setTimeout(
      () => controller.abort(),
      options.timeoutMs ?? this.requestTimeoutMs,
    );
    const onAbort = () => controller.abort();
    if (options.signal) {
      if (options.signal.aborted) controller.abort();
      else options.signal.addEventListener("abort", onAbort, { once: true });
    }
    try {
      const response = await fetch(`${this.endpoint}${path}`, {
        method: options.method,
        headers: {
          authorization: `Bearer ${this.credential}`,
          "content-type": "application/json",
        },
        ...(options.body !== undefined ? { body: options.body } : {}),
        redirect: "error",
        signal: controller.signal,
      });
      if (!response.ok) {
        throw new ProtocolError("request_rejected", `host returned HTTP ${response.status}`);
      }
      return await response.json();
    } catch (error) {
      if (error instanceof ProtocolError) throw error;
      if (options.signal?.aborted) {
        throw new ProtocolError("request_aborted", "request was cancelled");
      }
      throw new ProtocolError("request_failed", "launch host request failed");
    } finally {
      clearTimeout(timeout);
      options.signal?.removeEventListener("abort", onAbort);
    }
  }
}

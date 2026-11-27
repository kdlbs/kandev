import { expect, type Page } from "@playwright/test";
import type { GatewayTrafficFrame } from "../../helpers/ws-traffic";

type BootSnapshot = {
  taskIds?: string[];
  tasks?: unknown[];
};

export type BootCapture = {
  version?: number;
  bytes: number;
  initialState: {
    kanban?: BootSnapshot;
    kanbanMulti?: { snapshots?: Record<string, BootSnapshot> };
  };
  routeData?: {
    taskDetail?: {
      taskId?: string;
      sessionId?: string | null;
      initialState?: { kanban?: BootSnapshot };
      sidebarTaskPage?: { entries?: Array<{ task_id?: string }> };
    };
  };
  entities?: {
    tasks?: Record<string, unknown>;
    sessions?: Record<string, unknown>;
  };
};

export async function readBootCapture(page: Page): Promise<BootCapture> {
  return page.evaluate(() => {
    const boot = (window as Window & { __KANDEV_BOOT_PAYLOAD__?: unknown }).__KANDEV_BOOT_PAYLOAD__;
    const parsed = JSON.parse(JSON.stringify(boot)) as Omit<BootCapture, "bytes">;
    return { ...parsed, bytes: new TextEncoder().encode(JSON.stringify(boot)).byteLength };
  });
}

export function bootSnapshotTaskCount(snapshot: BootSnapshot | undefined): number {
  if (!snapshot) return 0;
  return snapshot.taskIds?.length ?? snapshot.tasks?.length ?? 0;
}

type MetadataEvent = {
  id: number;
  key: string;
  phase: "start" | "end";
  cancelled?: boolean;
  status?: number;
};

/** Records pending fetches by authorization generation and projection, including synchronous aborts. */
export async function captureJourneyMetadata(page: Page) {
  const resources: Record<
    string,
    {
      requests: number;
      active: number;
      peak: number;
      cancelled: number;
      statuses: Record<number, number>;
    }
  > = {};
  await page.exposeFunction("__reportJourneyMetadata", (event: MetadataEvent) => {
    const resource = (resources[event.key] ??= {
      requests: 0,
      active: 0,
      peak: 0,
      cancelled: 0,
      statuses: {},
    });
    if (event.phase === "start") {
      resource.requests++;
      resource.active++;
      resource.peak = Math.max(resource.peak, resource.active);
    } else {
      resource.active--;
      if (event.cancelled) resource.cancelled++;
      if (event.status)
        resource.statuses[event.status] = (resource.statuses[event.status] ?? 0) + 1;
    }
  });
  await page.addInitScript(() => {
    const win = window as Window & {
      __KANDEV_E2E_EXPOSE_STORE__?: boolean;
      __KANDEV_E2E_STORE__?: {
        getState(): {
          workspaceContextGeneration?: number;
          auth?: { authenticated?: boolean; user?: { id?: string } | null };
        };
      };
      __reportJourneyMetadata(event: MetadataEvent): Promise<void>;
      __journeyReconnectGateway?: () => void;
    };
    win.__KANDEV_E2E_EXPOSE_STORE__ = true;
    const NativeWebSocket = window.WebSocket;
    let gateway: WebSocket | undefined;
    const recordGateway = (socket: WebSocket) => {
      gateway = socket;
    };
    window.WebSocket = class extends NativeWebSocket {
      constructor(url: string | URL, protocols?: string | string[]) {
        super(url, protocols);
        if (String(url).endsWith("/ws")) recordGateway(this);
      }
    };
    win.__journeyReconnectGateway = () => gateway?.close();
    const original = window.fetch.bind(window);
    let requestId = 0;
    const metadataKey = (url: URL) => {
      const state = win.__KANDEV_E2E_STORE__?.getState();
      url.searchParams.sort();
      return JSON.stringify([
        state?.auth?.user?.id ?? null,
        state?.auth?.authenticated ?? false,
        state?.workspaceContextGeneration ?? 0,
        url.pathname + url.search,
      ]);
    };
    const requestOptions = (input: RequestInfo | URL, init?: RequestInit) => ({
      url: new URL(input instanceof Request ? input.url : String(input), location.href),
      method: init?.method ?? (input instanceof Request ? input.method : "GET"),
      signal: init?.signal ?? (input instanceof Request ? input.signal : undefined),
    });
    window.fetch = (input, init) => {
      const { url, method, signal } = requestOptions(input, init);
      if (
        method !== "GET" ||
        !/\/api\/v1\/(?:workspaces(?:\/[^/]+\/repositories)?|workflows|user\/settings|agent-profiles\/[^/]+\/mcp-config|github\/tasks\/[^/]+\/ci-options)$/.test(
          url.pathname,
        )
      ) {
        return original(input, init);
      }
      const key = metadataKey(url);
      const id = ++requestId;
      const report = (event: MetadataEvent) => void win.__reportJourneyMetadata(event);
      report({ id, key, phase: "start" });
      let finished = false;
      const finish = (cancelled = false, status?: number) => {
        if (finished) return;
        finished = true;
        signal?.removeEventListener("abort", onAbort);
        report({ id, key, phase: "end", cancelled, status });
      };
      const onAbort = () => finish(true);
      if (signal?.aborted) onAbort();
      else signal?.addEventListener("abort", onAbort, { once: true });
      return original(input, init).then(
        (response) => {
          finish(false, response.status);
          return response;
        },
        (error: unknown) => {
          finish(signal?.aborted ?? false);
          throw error;
        },
      );
    };
  });
  return resources;
}

/** Exercises transport recovery while retaining the current route and draft. */
export async function reconnectJourneyGateway(page: Page, frames: GatewayTrafficFrame[]) {
  const subscriptions = () =>
    frames.filter((frame) => frame.direction === "sent" && frame.action === "session.subscribe")
      .length;
  const before = subscriptions();
  await page.evaluate(() =>
    (
      window as Window & {
        __journeyReconnectGateway?: () => void;
      }
    ).__journeyReconnectGateway?.(),
  );
  await expect.poll(subscriptions, { timeout: 30_000 }).toBeGreaterThan(before);
}

/** Moves an existing sibling chat between a visible split and the retained tab group. */
export async function setJourneyChatSplit(
  page: Page,
  firstId: string,
  siblingId: string,
  split: boolean,
) {
  await page.evaluate(
    ({ firstId, siblingId, split }) => {
      const api = (
        window as Window & {
          __dockviewApi__?: import("dockview-react").DockviewApi;
        }
      ).__dockviewApi__;
      const first = api?.getPanel(`session:${firstId}`);
      const sibling = api?.getPanel(`session:${siblingId}`);
      if (!first || !sibling) throw new Error("Journey chat panels are missing");
      sibling.api.moveTo({ group: first.group, position: split ? "right" : "center" });
      first.api.setActive();
    },
    { firstId, siblingId, split },
  );
}

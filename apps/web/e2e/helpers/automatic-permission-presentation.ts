import type { Page } from "@playwright/test";
import type { ApiClient } from "./api-client";

/**
 * Creates a disposable profile that enables backend automatic approval. The
 * returned profile is deleted by the E2E reset between tests.
 */
export async function createAutomaticPermissionProfile(
  apiClient: ApiClient,
  seededAgentProfileId: string,
) {
  const agents = await apiClient.listAgents();
  const owner = agents.agents.find((agent) =>
    agent.profiles?.some((profile) => profile.id === seededAgentProfileId),
  );
  if (!owner) throw new Error("could not resolve the seeded agent for profile creation");
  return apiClient.createAgentProfile(owner.id, "Automatic permission presentation", {
    model: "mock-model",
    auto_approve: true,
  });
}

export type PermissionObservation = {
  actionRows: number;
  approveButtons: number;
  permissionMessages: number;
  amber: number;
  mutations: number;
};

type ObservationWindow = Window & {
  __kandevPermissionObservation?: PermissionObservation;
  __kandevPermissionObserver?: MutationObserver;
};

/**
 * Installs an in-page recorder before the scenario prompt is submitted. It
 * samples every DOM mutation so a prompt that appears and disappears inside the
 * automatic resolution interval is still counted.
 */
export async function installPermissionPresentationObserver(page: Page): Promise<void> {
  await page.evaluate(() => {
    const state: PermissionObservation = {
      actionRows: 0,
      approveButtons: 0,
      permissionMessages: 0,
      amber: 0,
      mutations: 0,
    };
    const windowWithRecorder = window as ObservationWindow;
    windowWithRecorder.__kandevPermissionObserver?.disconnect();
    windowWithRecorder.__kandevPermissionObservation = state;
    const inspect = () => {
      const chat = document.querySelector('[data-testid="session-chat"]');
      if (!chat) return;
      state.actionRows = Math.max(
        state.actionRows,
        chat.querySelectorAll('[data-testid="permission-action-row"]').length,
      );
      state.approveButtons = Math.max(
        state.approveButtons,
        chat.querySelectorAll('[data-testid="permission-approve"]').length,
      );
      const permissionMessages = chat.querySelectorAll(
        '[data-testid="permission-request-message"]',
      );
      state.permissionMessages = Math.max(state.permissionMessages, permissionMessages.length);
      for (const message of permissionMessages) {
        if (message.querySelector(".text-amber-600, .text-amber-400")) state.amber += 1;
      }
    };
    const observer = new MutationObserver(() => {
      state.mutations += 1;
      inspect();
    });
    observer.observe(document.body, { subtree: true, childList: true, attributes: true });
    windowWithRecorder.__kandevPermissionObserver = observer;
    inspect();
  });
}

export async function readPermissionPresentationObservation(
  page: Page,
): Promise<PermissionObservation> {
  return page.evaluate(
    () =>
      (window as ObservationWindow).__kandevPermissionObservation ?? {
        actionRows: 0,
        approveButtons: 0,
        permissionMessages: 0,
        amber: 0,
        mutations: 0,
      },
  );
}

type PermissionFrame = {
  id?: string;
  action?: string;
  type?: string;
  event_type?: string;
  session_id?: string;
  payload?: {
    session_id?: string;
    message_id?: string;
    type?: string;
    message_type?: string;
    metadata?: { permission_presentation?: unknown };
  };
};

const LEGACY_PERMISSION_ACTIONS: Record<string, "added" | "updated"> = {
  "session.message.added": "added",
  "session.message.updated": "updated",
};
const ORDERED_PERMISSION_EVENTS: Record<string, "added" | "updated"> = {
  "message.added": "added",
  "message.updated": "updated",
};

function permissionFrameKind(frame: PermissionFrame): {
  kind: "added" | "updated";
  messageId: string;
} | null {
  const payload = frame.payload;
  if (!payload?.message_id) return null;
  if (frame.action && LEGACY_PERMISSION_ACTIONS[frame.action]) {
    if (payload.type !== "permission_request") return null;
    return { kind: LEGACY_PERMISSION_ACTIONS[frame.action], messageId: payload.message_id };
  }
  if (
    frame.type === "session.event" &&
    frame.event_type &&
    ORDERED_PERMISSION_EVENTS[frame.event_type]
  ) {
    if (payload.message_type !== "permission_request") return null;
    return { kind: ORDERED_PERMISSION_EVENTS[frame.event_type], messageId: payload.message_id };
  }
  return null;
}

/**
 * Pure frame gate used by the browser helper. It tracks permission candidate
 * rows observed through `message.added` and holds only the completion updates
 * for those rows. The first candidate frame always passes so the presentation
 * under test can render.
 */
export class PermissionFrameGate {
  private readonly candidates = new Set<string>();
  private readonly held: string[] = [];
  private readonly snapshotRequests = new Set<string>();

  constructor(private readonly sessionId?: string) {}

  observe(frame: PermissionFrame): "pass" | "hold" {
    if (!this.matchesSession(frame)) return "pass";
    const permission = permissionFrameKind(frame);
    if (!permission) return "pass";
    if (permission.kind === "added") {
      this.candidates.add(permission.messageId);
      return "pass";
    }
    return this.candidates.has(permission.messageId) ? "hold" : "pass";
  }

  /**
   * Records a target-session `message.list` request so its response can be held
   * while the automatic interval is observed. The request itself is forwarded.
   */
  observeClientFrame(frame: PermissionFrame): void {
    if (frame.type !== "request" || frame.action !== "message.list" || !frame.id) return;
    if (this.sessionId && frame.payload?.session_id !== this.sessionId) return;
    this.snapshotRequests.add(frame.id);
  }

  shouldHoldResponse(frame: PermissionFrame): boolean {
    if (frame.type !== "response" && frame.type !== "error") return false;
    return !!frame.id && this.snapshotRequests.has(frame.id);
  }

  hold(frame: string): void {
    this.held.push(frame);
  }

  heldCount(): number {
    return this.held.length;
  }

  release(): string[] {
    return this.held.splice(0);
  }

  private matchesSession(frame: PermissionFrame): boolean {
    if (!this.sessionId) return true;
    return frame.session_id === this.sessionId || frame.payload?.session_id === this.sessionId;
  }
}

export type AutomaticPermissionGate = {
  arm: () => void;
  heldCount: () => number;
  snapshotHeldCount: () => number;
  waitForHeld: () => Promise<void>;
  release: () => Promise<void>;
};

/**
 * Routes the gateway socket once, before navigation, then arms the gate after
 * the page is idle. While armed, permission completion notifications and the
 * target session's `message.list` responses wait for the same release so a
 * refresh cannot overwrite the held interval.
 */
export async function createAutomaticPermissionGate(
  page: Page,
  sessionId?: string,
): Promise<AutomaticPermissionGate> {
  const gate = new PermissionFrameGate(sessionId);
  const heldSenders: Array<() => void> = [];
  const waiters = new Set<() => void>();
  let snapshotHeld = 0;
  let armed = false;
  let released = false;
  let releaseSnapshotReads: (() => void) | undefined;
  const snapshotRelease = new Promise<void>((resolve) => {
    releaseSnapshotReads = resolve;
  });

  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message === "string" && armed) {
        for (const part of message.split("\n").filter(Boolean)) {
          const frame = decodePermissionFrame(part);
          if (frame) gate.observeClientFrame(frame);
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      if (typeof message !== "string" || !armed) {
        socket.send(message);
        return;
      }
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = decodePermissionFrame(part);
        if (!frame) {
          socket.send(part);
          continue;
        }
        if (gate.shouldHoldResponse(frame)) {
          snapshotHeld += 1;
          heldSenders.push(() => socket.send(part));
          for (const wake of waiters) wake();
          waiters.clear();
          continue;
        }
        if (gate.observe(frame) === "pass") {
          socket.send(part);
          continue;
        }
        gate.hold(part);
        heldSenders.push(() => socket.send(part));
        for (const wake of waiters) wake();
        waiters.clear();
      }
    });
  });
  await page.route(/\/api\/v1\/task-sessions\/[^/]+\/messages/, async (route) => {
    if (!armed || gate.heldCount() === 0) {
      await route.continue();
      return;
    }
    await snapshotRelease;
    await route.continue();
  });

  return {
    arm: () => {
      armed = true;
    },
    heldCount: () => gate.heldCount(),
    snapshotHeldCount: () => snapshotHeld,
    waitForHeld: () =>
      new Promise<void>((resolve) => {
        if (gate.heldCount() > 0 || snapshotHeld > 0) {
          resolve();
          return;
        }
        waiters.add(resolve);
      }),
    release: async () => {
      if (released) return;
      released = true;
      armed = false;
      releaseSnapshotReads?.();
      for (const send of heldSenders.splice(0)) send();
      gate.release();
    },
  };
}

function decodePermissionFrame(raw: string): PermissionFrame | null {
  try {
    return JSON.parse(raw) as PermissionFrame;
  } catch {
    return null;
  }
}

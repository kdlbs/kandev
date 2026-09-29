import {
  getUndoConflict,
  listActivity,
  undoActivity,
  type ActivityClass,
  type ActivityItem,
} from "@/lib/api/domains/coordinator-activity-api";
import { ApiError } from "@/lib/api/client";

export type ActivityStatus = "loading" | "loaded" | "failed";
export type LoadMoreState = "idle" | "loading" | "failed";

export type UndoMessageKey =
  | "activityConflictMoved"
  | "activityConflictAgentRunning"
  | "activityConflictStepDeleted"
  | "activityConflictStepDone"
  | "activityConflictStepFull"
  | "activityNotUndoable"
  | "activityUndoFailed"
  | "activityGone";

export type UndoMessage = {
  key: UndoMessageKey;
  /** True until the first re-read that starts after the refusal settles has settled. */
  survives: boolean;
  /** The re-read counter when the message was set. */
  setAt: number;
};

export type ActivitySnapshot = {
  status: ActivityStatus;
  rows: ActivityItem[];
  nextCursor: string | null;
  loadMore: LoadMoreState;
  rereading: boolean;
  messages: ReadonlyMap<string, UndoMessage>;
  notice: UndoMessage | null;
  busy: ReadonlySet<string>;
};

export type ActivityControllerOptions = {
  workspaceId: string;
  coordinatorId: string;
  activityClass?: ActivityClass;
  /** Called after every successful first load, re-read or Load more with the rows now shown. */
  onArrival?: (rows: ActivityItem[]) => void;
};

const CONFLICT_KEY_BY_REASON: Record<string, UndoMessageKey> = {
  agent_running: "activityConflictAgentRunning",
  step_deleted: "activityConflictStepDeleted",
  step_done: "activityConflictStepDone",
  step_full: "activityConflictStepFull",
};

export function conflictMessageKey(reason: string | undefined): UndoMessageKey {
  return (reason && CONFLICT_KEY_BY_REASON[reason]) || "activityConflictMoved";
}

export const INITIAL_ACTIVITY_SNAPSHOT: ActivitySnapshot = {
  status: "loading",
  rows: [],
  nextCursor: null,
  loadMore: "idle",
  rereading: false,
  messages: new Map(),
  notice: null,
  busy: new Set(),
};

type Waiter = (ok: boolean) => void;

/**
 * One list for one (coordinator, class filter). A controller is the request
 * generation: a filter or coordinator change, or unmount, disposes it and every
 * response that is still in flight is dropped.
 */
export class ActivityController {
  private snap: ActivitySnapshot = INITIAL_ACTIVITY_SNAPSHOT;
  private readonly listeners = new Set<() => void>();
  private disposed = false;
  private pages = 0;
  private rereadCounter = 0;
  private rereadInFlight = false;
  private trailing: Waiter[] | null = null;
  private loadMoreSeq = 0;

  constructor(private readonly opts: ActivityControllerOptions) {}

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getSnapshot = (): ActivitySnapshot => this.snap;

  start(): void {
    void this.firstLoad();
  }

  dispose(): void {
    this.disposed = true;
    this.listeners.clear();
    this.trailing?.forEach((w) => w(false));
    this.trailing = null;
  }

  retry(): void {
    if (this.snap.status === "failed") void this.firstLoad();
  }

  /** A `coordinator.updated` event or a reconnect. Ignored until the first load has succeeded. */
  refresh(): void {
    void this.requestReread();
  }

  loadMore(): void {
    const s = this.snap;
    if (s.status !== "loaded" || s.loadMore === "loading" || s.rereading || !s.nextCursor) return;
    const seq = ++this.loadMoreSeq;
    this.update({ loadMore: "loading" });
    listActivity(this.opts.workspaceId, this.opts.coordinatorId, {
      class: this.opts.activityClass,
      before: s.nextCursor,
    })
      .then((page) => {
        if (this.disposed || seq !== this.loadMoreSeq) return;
        this.pages += 1;
        const rows = [...this.snap.rows, ...page.rows];
        this.update({ rows, nextCursor: page.next_cursor, loadMore: "idle" });
        this.opts.onArrival?.(rows);
      })
      .catch(() => {
        if (this.disposed || seq !== this.loadMoreSeq) return;
        this.update({ loadMore: "failed" });
      });
  }

  /** Confirmed Undo of one row. */
  async undo(row: ActivityItem): Promise<void> {
    if (this.disposed || this.snap.status !== "loaded" || this.snap.busy.has(row.id)) return;
    this.clearMessage(row.id);
    this.setBusy(row.id, true);
    let error: unknown = null;
    try {
      await undoActivity(this.opts.workspaceId, this.opts.coordinatorId, row.id);
    } catch (e) {
      error = e;
    }
    if (this.disposed) return;
    if (error === null) {
      await this.requestReread();
      this.setBusy(row.id, false);
      return;
    }
    await this.settleUndoError(row.id, error);
  }

  private async settleUndoError(rowId: string, error: unknown): Promise<void> {
    const conflict = getUndoConflict(error);
    if (conflict?.code === "already_undone") {
      this.setBusy(rowId, false);
      void this.requestReread();
      return;
    }
    if (conflict?.code === "undo_conflict") {
      this.setMessage(rowId, conflictMessageKey(conflict.reason), false);
    } else if (conflict?.code === "not_undoable") {
      this.setMessage(rowId, "activityNotUndoable", true);
    } else if (error instanceof ApiError && error.status === 404) {
      this.setNotice(rowId, "activityGone");
    } else {
      this.setMessage(rowId, "activityUndoFailed", false);
    }
    this.setBusy(rowId, false);
    const rereads = conflict?.code === "not_undoable" || (!conflict && isNotFound(error));
    if (rereads) void this.requestReread();
  }

  private async firstLoad(): Promise<void> {
    this.pages = 0;
    this.update({ ...INITIAL_ACTIVITY_SNAPSHOT, status: "loading" });
    try {
      const page = await listActivity(this.opts.workspaceId, this.opts.coordinatorId, {
        class: this.opts.activityClass,
      });
      if (this.disposed) return;
      this.pages = 1;
      this.update({ status: "loaded", rows: page.rows, nextCursor: page.next_cursor });
      this.opts.onArrival?.(page.rows);
    } catch {
      if (this.disposed) return;
      this.update({ status: "failed" });
    }
  }

  private requestReread(): Promise<boolean> {
    if (this.disposed || this.snap.status !== "loaded") return Promise.resolve(false);
    if (this.rereadInFlight) {
      return new Promise((resolve) => {
        (this.trailing ??= []).push(resolve);
      });
    }
    return this.runReread();
  }

  private async runReread(): Promise<boolean> {
    const number = ++this.rereadCounter;
    this.rereadInFlight = true;
    this.loadMoreSeq += 1;
    this.update({ rereading: true, loadMore: "idle" });
    let ok = false;
    try {
      const result = await this.readLoadedPages();
      if (!this.disposed) {
        this.pages = result.pages;
        this.update({ rows: result.rows, nextCursor: result.cursor });
        ok = true;
        this.opts.onArrival?.(result.rows);
      }
    } catch {
      ok = false;
    }
    this.rereadInFlight = false;
    if (this.disposed) return false;
    this.settleMessages(number, ok);
    const waiters = this.trailing;
    this.trailing = null;
    if (waiters) {
      void this.runReread().then((result) => waiters.forEach((w) => w(result)));
    } else {
      this.update({ rereading: false });
    }
    return ok;
  }

  private async readLoadedPages() {
    const { workspaceId, coordinatorId, activityClass } = this.opts;
    const first = await listActivity(workspaceId, coordinatorId, { class: activityClass });
    const rows = [...first.rows];
    let cursor = first.next_cursor;
    let pages = 1;
    while (pages < this.pages && cursor) {
      const page = await listActivity(workspaceId, coordinatorId, {
        class: activityClass,
        before: cursor,
      });
      rows.push(...page.rows);
      cursor = page.next_cursor;
      pages += 1;
    }
    return { rows, cursor, pages };
  }

  private settleMessages(number: number, ok: boolean): void {
    const settle = (m: UndoMessage): UndoMessage | null => {
      if (number <= m.setAt) return m;
      if (m.survives) return { ...m, survives: false };
      return ok ? null : m;
    };
    const messages = new Map<string, UndoMessage>();
    for (const [id, m] of this.snap.messages) {
      const next = settle(m);
      if (next) messages.set(id, next);
    }
    this.update({ messages, notice: this.snap.notice ? settle(this.snap.notice) : null });
  }

  private setBusy(rowId: string, on: boolean): void {
    const busy = new Set(this.snap.busy);
    if (on) busy.add(rowId);
    else busy.delete(rowId);
    this.update({ busy });
  }

  private clearMessage(rowId: string): void {
    if (!this.snap.messages.has(rowId)) return;
    const messages = new Map(this.snap.messages);
    messages.delete(rowId);
    this.update({ messages });
  }

  private isListed(rowId: string): boolean {
    return this.snap.rows.some((r) => r.id === rowId);
  }

  private setMessage(rowId: string, key: UndoMessageKey, survives: boolean): void {
    if (!this.isListed(rowId)) return;
    const messages = new Map(this.snap.messages);
    messages.set(rowId, { key, survives, setAt: this.rereadCounter });
    this.update({ messages });
  }

  private setNotice(rowId: string, key: UndoMessageKey): void {
    if (!this.isListed(rowId)) return;
    this.update({ notice: { key, survives: true, setAt: this.rereadCounter } });
  }

  private update(patch: Partial<ActivitySnapshot>): void {
    this.snap = { ...this.snap, ...patch };
    this.listeners.forEach((l) => l());
  }
}

function isNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

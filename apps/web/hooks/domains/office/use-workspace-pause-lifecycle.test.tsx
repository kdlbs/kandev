import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  WorkspacePauseState,
  WorkspacePauseTopbarActions,
} from "@/app/office/components/workspace-pause-banner";
import { t } from "@/lib/i18n";
import { useWorkspacePause } from "./use-workspace-pause";

function deferred() {
  let resolve!: (response: Response) => void;
  const promise = new Promise<Response>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

const PAUSED_BANNER = "office-workspace-paused-banner";
const UNAVAILABLE_BANNER = "office-workspace-pause-unavailable";

const requests: (ReturnType<typeof deferred> & { url: string; method: string })[] = [];
const actions: Promise<unknown>[] = [];

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const request = { ...deferred(), url: String(url), method: init?.method ?? "GET" };
      requests.push(request);
      return request.promise;
    }),
  );
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const request of requests.splice(0)) request.resolve(response(false));
    await Promise.all(actions.splice(0));
  });
  vi.unstubAllGlobals();
});

function response(paused: boolean, workspace = "ws-1") {
  return new Response(
    JSON.stringify({
      workspace_id: workspace,
      paused,
      pause: paused
        ? {
            id: "pause-1",
            reason: "incident",
            created_by: "operator",
            created_by_kind: "user",
            created_at: "2026-10-15T00:00:00Z",
          }
        : null,
    }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  );
}

function mount() {
  let current!: {
    value: ReturnType<typeof useWorkspacePause>;
    store: ReturnType<typeof useAppStoreApi>;
  };
  function Consumer() {
    const workspace = useAppStore((state) => state.workspaces.activeId);
    const value = useWorkspacePause(workspace);
    current = { value, store: useAppStoreApi() };
    const view = { activeWorkspaceId: workspace, ...value };
    return (
      <>
        <WorkspacePauseState view={view} />
        <WorkspacePauseTopbarActions view={view} />
      </>
    );
  }
  function tree(key: number) {
    return (
      <StateProvider
        initialState={{ workspaces: { items: [], activeId: "ws-1", activeIdRevision: 0 } }}
      >
        <Consumer key={key} />
      </StateProvider>
    );
  }
  const rendered = render(tree(0));
  return {
    get current() {
      return current;
    },
    remount: () => rendered.rerender(tree(1)),
    unmount: rendered.unmount,
  };
}

type View = ReturnType<typeof mount>;

async function succeed(index: number, paused = false, workspace = "ws-1") {
  expect(requests[index]).toBeDefined();
  await act(async () => {
    requests[index].resolve(response(paused, workspace));
  });
}

async function fail(index: number) {
  expect(requests[index]).toBeDefined();
  await act(async () => {
    requests[index].resolve(
      new Response(JSON.stringify({ error: "service unavailable" }), {
        status: 503,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
}

function start(view: View, kind: "read" | "pause" | "resume") {
  let pending!: Promise<unknown>;
  act(() => {
    if (kind === "read") pending = view.current.value.refresh();
    else if (kind === "pause") pending = view.current.value.pause("incident");
    else pending = view.current.value.resume();
    actions.push(pending);
  });
  return pending;
}

function expectLoading() {
  expect(screen.getByTestId("office-workspace-pause-loading").textContent).toBe(
    t("office:pauseStateLoading"),
  );
  expect(screen.queryByTestId(UNAVAILABLE_BANNER)).toBeNull();
}

describe("workspace pause real store, hook and banner lifecycle", () => {
  // @covers AC-OFFICE-KILL-SWITCH-006.4
  it("keeps the cached pause visible while a same-workspace remount read is pending", async () => {
    const view = mount();
    expectLoading();
    await succeed(0, true);
    view.remount();
    expect(requests).toHaveLength(2);
    expect(screen.getByTestId(PAUSED_BANNER).textContent).toContain("incident");
    expect(screen.queryByTestId(UNAVAILABLE_BANNER)).toBeNull();
    expect(view.current.store.getState().office.pause.workspaceId).toBe("ws-1");
    await succeed(1, true);
  });

  it("clears another workspace's record while the selected workspace read is pending", async () => {
    const view = mount();
    await succeed(0, true);
    act(() => view.current.store.getState().setActiveWorkspace("ws-2"));
    expect(requests[1].url).toContain("/workspaces/ws-2/pause");
    expect(screen.queryByTestId(PAUSED_BANNER)).toBeNull();
    expectLoading();
    await succeed(1, false, "ws-2");
    expect(view.current.value.isPaused).toBe(false);
  });

  it("binds the cached resume control to the selected workspace during revalidation", async () => {
    const view = mount();
    await succeed(0, true);
    view.remount();
    const topbar = screen.getByTestId("office-workspace-topbar-actions");
    fireEvent.click(within(topbar).getByTestId("office-resume-workspace-button"));
    fireEvent.click(screen.getByTestId("office-resume-confirm-button"));
    expect(requests[2].method).toBe("POST");
    expect(requests[2].url).toContain("/workspaces/ws-1/resume");
    await succeed(2);
    await succeed(1, true);
    expect(screen.queryByTestId(PAUSED_BANNER)).toBeNull();
  });

  it("shows the localized warning only after a genuine cold-read failure", async () => {
    mount();
    expectLoading();
    await fail(0);
    expect(screen.queryByTestId("office-workspace-pause-loading")).toBeNull();
    expect(screen.getByTestId(UNAVAILABLE_BANNER).textContent).toBe(
      t("office:pauseStateUnavailable"),
    );
  });

  it("retains the cached record as stale after revalidation fails", async () => {
    const view = mount();
    await succeed(0, true);
    view.remount();
    await fail(1);
    expect(screen.getByTestId(PAUSED_BANNER).textContent).toContain(t("office:pauseStateStale"));
    expect(view.current.value.record?.reason).toBe("incident");
  });
});

describe("workspace pause real request ordering", () => {
  it("ignores an obsolete GET failure while a newer read remains pending", async () => {
    const view = mount();
    start(view, "read");
    await fail(0);
    expectLoading();
    await succeed(1, true);
    expect(screen.getByTestId(PAUSED_BANNER)).toBeTruthy();
  });

  it.each(["read-first", "mutation-first"])(
    "settles failed GET and POST in %s order",
    async (order) => {
      const view = mount();
      const pending = start(view, "pause");
      expect(requests[0].method).toBe("GET");
      expect(requests[1].method).toBe("POST");
      if (order === "read-first") {
        await fail(0);
        await fail(1);
      } else {
        await fail(1);
        expectLoading();
        await fail(0);
      }
      expect(await pending).toMatchObject({ ok: false });
      expect(view.current.value.isMutating).toBe(false);
      expect(screen.getByTestId(UNAVAILABLE_BANNER)).toBeTruthy();
    },
  );

  it("accepts an outstanding successful GET after a newer POST fails", async () => {
    const view = mount();
    const pending = start(view, "pause");
    await fail(1);
    await succeed(0, true);
    expect(await pending).toMatchObject({ ok: false });
    expect(screen.getByTestId(PAUSED_BANNER)).toBeTruthy();
  });

  it("preserves a successful mutation when an older GET later fails", async () => {
    const view = mount();
    const pending = start(view, "pause");
    await succeed(1, true);
    await fail(0);
    expect(await pending).toMatchObject({ ok: true });
    expect(view.current.value.status).toBe("known");
    expect(screen.getByTestId(PAUSED_BANNER).textContent).not.toContain(
      t("office:pauseStateStale"),
    );
  });

  it("keeps the newer read outcome when an older read completes afterward", async () => {
    const view = mount();
    start(view, "read");
    await succeed(1, true);
    await succeed(0, false);
    expect(view.current.value.record?.id).toBe("pause-1");
  });

  it("discards an old workspace response after selection changes", async () => {
    const view = mount();
    act(() => view.current.store.getState().setActiveWorkspace("ws-2"));
    await succeed(0, true);
    expectLoading();
    await succeed(1, false, "ws-2");
    expect(view.current.value.record).toBeNull();
    expect(view.current.store.getState().office.pause.workspaceId).toBe("ws-2");
  });
});

import { StrictMode, Suspense, startTransition, useEffect, useLayoutEffect, useState } from "react";
import { act, cleanup, render, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import { useWorkflowSync, type WorkflowSyncController } from "./use-workflow-sync";
import { config, deferred, Providers, transport } from "./workflow-sync.lifetime.test-helpers";

const { fetchJson } = vi.hoisted(() => ({ fetchJson: vi.fn() }));
vi.mock("@/lib/api/client", async (original) => ({
  ...(await original<typeof import("@/lib/api/client")>()),
  fetchJson,
}));

const SYNC_PATH = "/api/v1/workflow-sync/sync";
let wire: ReturnType<typeof transport>;
let reload: ReturnType<typeof vi.spyOn>;
beforeEach(() => {
  wire = transport();
  fetchJson.mockImplementation(wire.fetch);
  reload = vi.spyOn(window.location, "reload").mockImplementation(() => {});
});
afterEach(async () => {
  cleanup();
  await act(async () => wire.drain());
  vi.useRealTimers();
  vi.restoreAllMocks();
});

async function mount(workspace = "A") {
  const hook = renderHook(({ id }) => useWorkflowSync(id), {
    initialProps: { id: workspace },
    wrapper: Providers,
  });
  await waitFor(() => expect(hook.result.current.loading).toBe(false));
  return hook;
}

// @covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.1, AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.5
describe("actual retired hook instances", () => {
  it.each(["remove", "force"] as const)(
    "retired %s completes without reloading B",
    async (action) => {
      const old = await mount();
      const response = wire.hold(
        action === "remove" ? "DELETE" : "POST",
        action === "force" ? SYNC_PATH : undefined,
      );
      let outcome!: Promise<boolean | void>;
      act(() => {
        outcome =
          action === "remove"
            ? old.result.current.handleDelete()
            : old.result.current.handleSyncNow();
      });
      old.unmount();
      const current = await mount("B");
      await act(async () => {
        response.resolve(
          action === "remove"
            ? { deleted: true }
            : { config: config("A"), result: { unchanged: false } },
        );
        expect(await outcome).toBe(action === "remove" ? true : undefined);
      });
      expect(current.result.current.config?.workspace_id).toBe("B");
      expect(current.result.current.form.repo_name).toBe("B");
      expect(reload).not.toHaveBeenCalled();
      expect(screen.queryByText(/sync removed|sync completed/i)).toBeNull();
    },
  );

  it.each(["save", "remove", "force"] as const)(
    "retired %s rejection keeps B feedback and pending state",
    async (action) => {
      const old = await mount();
      const held = wire.hold(
        action === "remove" ? "DELETE" : "POST",
        action === "force" ? SYNC_PATH : undefined,
      );
      let outcome!: Promise<boolean | void>;
      act(() => {
        if (action === "save") outcome = old.result.current.handleSave();
        else if (action === "remove") outcome = old.result.current.handleDelete();
        else outcome = old.result.current.handleSyncNow();
      });
      old.unmount();
      const current = await mount("B");
      await act(async () => {
        held.reject(new Error("retired failure"));
        expect(await outcome).toBe(action === "force" ? undefined : false);
      });
      expect(screen.queryByText(/retired failure/)).toBeNull();
      expect(current.result.current.config?.workspace_id).toBe("B");
      expect(current.result.current.saving).toBe(false);
      expect(reload).not.toHaveBeenCalled();
    },
  );
});

// @covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.2, AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.3
describe("retained hook API callbacks (separate from route unmount evidence)", () => {
  it.each(["B", "A"])("A-B%s retains no old admission", async (last) => {
    const hook = await mount();
    const retired = hook.result.current;
    const held = wire.hold("POST");
    let saved!: Promise<boolean>;
    act(() => {
      saved = retired.handleSave();
    });
    hook.rerender({ id: "B" });
    await waitFor(() => expect(hook.result.current.config?.workspace_id).toBe("B"));
    if (last === "A") {
      hook.rerender({ id: "A" });
      await waitFor(() => expect(hook.result.current.config?.workspace_id).toBe("A"));
    }
    const calls = wire.requests.length;
    await act(async () => {
      retired.update("branch", "retired");
      retired.setUrlInput("https://github.com/retired/target");
      retired.setProvider("gitlab");
      expect(await retired.handleSave()).toBe(false);
      expect(await retired.handleDelete()).toBe(false);
      await retired.handleSyncNow();
      held.resolve(config("A", { branch: "acknowledged-old" }));
      expect(await saved).toBe(true);
    });
    expect(wire.requests).toHaveLength(calls);
    expect(wire.requests.find((r) => r.method === "POST")?.workspace).toBe("A");
    expect(hook.result.current.form.branch).toBe("main");
    expect(hook.result.current.form.repo_name).toBe(last);
    expect(screen.queryByText(/configuration saved/i)).toBeNull();
  });

  it("retires callbacks during layout before passive cleanup", async () => {
    let latest!: WorkflowSyncController;
    let passiveCleaned = false;
    let refused!: Promise<boolean>;
    function Harness({ id }: { id: string }) {
      const sync = useWorkflowSync(id);
      latest = sync;
      useEffect(
        () => () => {
          passiveCleaned = true;
        },
        [id],
      );
      useLayoutEffect(() => {
        if (id === "B") {
          expect(passiveCleaned).toBe(false);
          old.update("branch", "stale-layout-input");
          refused = old.handleDelete();
        }
      }, [id]);
      return null;
    }
    const view = render(
      <Providers>
        <Harness id="A" />
      </Providers>,
    );
    await waitFor(() => expect(latest.loading).toBe(false));
    const old = latest;
    view.rerender(
      <Providers>
        <Harness id="B" />
      </Providers>,
    );
    await act(async () => expect(await refused).toBe(false));
    expect(wire.requests.filter((r) => r.method === "DELETE")).toHaveLength(0);
    expect(latest.form.branch).toBe("main");
  });
});

describe("committed StrictMode activation", () => {
  it("first StrictMode setup callbacks never regain admission after replay", async () => {
    const held = wire.hold("DELETE");
    let first!: WorkflowSyncController;
    let live!: WorkflowSyncController;
    let firstResult!: Promise<boolean>;
    let setups = 0;
    let cleanups = 0;
    function Harness() {
      const sync = useWorkflowSync("A");
      live = sync;
      useLayoutEffect(() => {
        setups += 1;
        if (!first) {
          first = sync;
          firstResult = sync.handleDelete();
        }
        return () => {
          cleanups += 1;
        };
      }, []);
      return null;
    }
    render(
      <StrictMode>
        <Providers>
          <Harness />
        </Providers>
      </StrictMode>,
    );
    await waitFor(() => expect(live.loading).toBe(false));
    expect(setups).toBe(2);
    expect(cleanups).toBe(1);
    expect(wire.requests.filter((r) => r.method === "DELETE")).toHaveLength(1);
    await act(async () => {
      first.update("branch", "first-setup");
      expect(await first.handleDelete()).toBe(false);
      held.resolve({ deleted: true });
      expect(await firstResult).toBe(true);
    });
    expect(wire.requests.filter((r) => r.method === "DELETE")).toHaveLength(1);
    expect(live.form.branch).toBe("main");
    expect(reload).not.toHaveBeenCalled();
    await act(async () => expect(await live.handleDelete()).toBe(true));
    expect(wire.requests.filter((r) => r.method === "DELETE")).toHaveLength(2);
    expect(reload).toHaveBeenCalledTimes(1);
  });
});

describe("speculative and independent views", () => {
  it("an uncommitted suspended B render does not retire visible A", async () => {
    const gate = deferred<void>();
    let change!: (id: string) => void;
    let current!: WorkflowSyncController;
    let speculative!: WorkflowSyncController;
    let blocked = true;
    function Consumer({ id }: { id: string }) {
      const sync = useWorkflowSync(id);
      if (id === "B" && blocked) {
        speculative = sync;
        throw gate.promise;
      }
      current = sync;
      return <div>{id}</div>;
    }
    function Harness() {
      const [id, setId] = useState("A");
      change = setId;
      return (
        <Suspense fallback={<div>pending</div>}>
          <Consumer id={id} />
        </Suspense>
      );
    }
    render(
      <Providers>
        <Harness />
      </Providers>,
    );
    await waitFor(() => expect(current.loading).toBe(false));
    const visible = current;
    act(() => startTransition(() => change("B")));
    expect(screen.getByText("A")).toBeTruthy();
    await act(async () => expect(await speculative.handleDelete()).toBe(false));
    expect(wire.requests.filter((r) => r.method === "DELETE")).toHaveLength(0);
    await act(async () => expect(await visible.handleSave()).toBe(true));
    expect(wire.requests.find((r) => r.method === "POST")?.workspace).toBe("A");
    await act(async () => {
      blocked = false;
      gate.resolve();
    });
  });

  it.each(["A", "B"])("retiring one view does not retire independent %s view/store", async (id) => {
    const first = await mount();
    const second = await mount(id);
    first.unmount();
    await act(async () => expect(await second.result.current.handleSave()).toBe(true));
    expect(second.result.current.config?.workspace_id).toBe(id);
    expect(screen.getByText(/configuration saved/i)).toBeTruthy();
  });
});

it("two same-workspace consumers sharing providers keep independent admission", async () => {
  let first!: WorkflowSyncController;
  let second!: WorkflowSyncController;
  function First() {
    first = useWorkflowSync("A");
    return null;
  }
  function Second() {
    second = useWorkflowSync("A");
    return null;
  }
  function Views({ show }: { show: boolean }) {
    return (
      <Providers>
        {show && <First />}
        <Second />
      </Providers>
    );
  }
  const view = render(<Views show />);
  await waitFor(() => expect(first.loading || second.loading).toBe(false));
  const old = first;
  view.rerender(<Views show={false} />);
  await act(async () => {
    expect(await old.handleSave()).toBe(false);
    expect(await second.handleSave()).toBe(true);
  });
  expect(wire.requests.filter((r) => r.method === "POST")).toHaveLength(1);
  expect(second.config?.workspace_id).toBe("A");
});

// @covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.4, AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.6
describe("current pending and presentation", () => {
  it.each(["save", "force"] as const)(
    "older %s finally cannot clear newer pending control",
    async (action) => {
      const hook = await mount();
      const path = action === "force" ? SYNC_PATH : undefined;
      const old = wire.hold("POST", path);
      let first!: Promise<boolean | void>;
      act(() => {
        first =
          action === "save"
            ? hook.result.current.handleSave()
            : hook.result.current.handleSyncNow();
      });
      const next = wire.hold("POST", path);
      let second!: Promise<boolean | void>;
      act(() => {
        second =
          action === "save"
            ? hook.result.current.handleSave()
            : hook.result.current.handleSyncNow();
      });
      await act(async () => {
        old.reject(new Error("older current failure"));
        await first;
      });
      expect(action === "save" ? hook.result.current.saving : hook.result.current.syncing).toBe(
        true,
      );
      await act(async () => {
        next.resolve(
          action === "save" ? config() : { config: config(), result: { unchanged: true } },
        );
        await second;
      });
      expect(action === "save" ? hook.result.current.saving : hook.result.current.syncing).toBe(
        false,
      );
    },
  );

  it.each(["save", "force"] as const)(
    "newest %s may settle before older success without ordering config",
    async (action) => {
      const hook = await mount();
      const path = action === "force" ? SYNC_PATH : undefined;
      const old = wire.hold("POST", path);
      let first!: Promise<boolean | void>;
      act(() => {
        first =
          action === "save"
            ? hook.result.current.handleSave()
            : hook.result.current.handleSyncNow();
      });
      const next = wire.hold("POST", path);
      let second!: Promise<boolean | void>;
      act(() => {
        second =
          action === "save"
            ? hook.result.current.handleSave()
            : hook.result.current.handleSyncNow();
      });
      const result = (branch: string) =>
        action === "save"
          ? config("A", { branch })
          : { config: config("A", { branch }), result: { unchanged: true } };
      await act(async () => {
        next.resolve(result("newer"));
        await second;
      });
      expect(action === "save" ? hook.result.current.saving : hook.result.current.syncing).toBe(
        false,
      );
      await act(async () => {
        old.resolve(result("older"));
        await first;
      });
      expect(action === "save" ? hook.result.current.saving : hook.result.current.syncing).toBe(
        false,
      );
      expect(hook.result.current.config?.branch).toBe("older");
    },
  );
});

describe("replacement pending controls", () => {
  it.each(["save", "force"] as const)(
    "retired %s success cannot clear B's active control",
    async (action) => {
      const hook = await mount();
      const path = action === "force" ? SYNC_PATH : undefined;
      const old = wire.hold("POST", path);
      let first!: Promise<boolean | void>;
      act(() => {
        first =
          action === "save"
            ? hook.result.current.handleSave()
            : hook.result.current.handleSyncNow();
      });
      hook.rerender({ id: "B" });
      await waitFor(() => expect(hook.result.current.loading).toBe(false));
      const next = wire.hold("POST", path);
      let second!: Promise<boolean | void>;
      act(() => {
        second =
          action === "save"
            ? hook.result.current.handleSave()
            : hook.result.current.handleSyncNow();
      });
      await act(async () => {
        old.resolve(
          action === "save" ? config() : { config: config(), result: { unchanged: false } },
        );
        expect(await first).toBe(action === "save" ? true : undefined);
      });
      expect(action === "save" ? hook.result.current.saving : hook.result.current.syncing).toBe(
        true,
      );
      expect(hook.result.current.config?.workspace_id).toBe("B");
      expect(reload).not.toHaveBeenCalled();
      await act(async () => {
        next.reject(new Error("new-current-failure"));
        await second;
      });
      expect(action === "save" ? hook.result.current.saving : hook.result.current.syncing).toBe(
        false,
      );
      expect(screen.getByText(/new-current-failure/)).toBeTruthy();
    },
  );

  it.each(["resolve", "reject"] as const)(
    "retired initial GET %s preserves newer loading",
    async (finish) => {
      const old = wire.hold("GET");
      const hook = renderHook(({ id }) => useWorkflowSync(id), {
        initialProps: { id: "A" },
        wrapper: Providers,
      });
      const current = wire.hold("GET");
      hook.rerender({ id: "B" });
      await act(async () => {
        if (finish === "resolve") old.resolve(config());
        else old.reject(new Error("old initial failure"));
      });
      expect(hook.result.current.loading).toBe(true);
      expect(hook.result.current.config).toBeNull();
      expect(screen.queryByText(/old initial failure/)).toBeNull();
      await act(async () => current.resolve(config("B")));
      expect(hook.result.current.loading).toBe(false);
      expect(hook.result.current.form.repo_name).toBe("B");
    },
  );
});

describe("current and retired reads", () => {
  it("background GET is status-only and its retired response cannot publish", async () => {
    vi.useFakeTimers();
    const hook = renderHook(({ id }) => useWorkflowSync(id), {
      initialProps: { id: "A" },
      wrapper: Providers,
    });
    await act(async () => {});
    expect(hook.result.current.loading).toBe(false);
    act(() => hook.result.current.update("branch", "draft"));
    const refresh = wire.hold("GET");
    act(() => vi.advanceTimersByTime(INTEGRATION_STATUS_REFRESH_MS));
    await act(async () => refresh.resolve(config("A", { last_error: "status", branch: "server" })));
    expect(hook.result.current.config?.last_error).toBe("status");
    expect(hook.result.current.form.branch).toBe("draft");
    const retired = wire.hold("GET");
    act(() => vi.advanceTimersByTime(INTEGRATION_STATUS_REFRESH_MS));
    hook.rerender({ id: "B" });
    await act(async () => retired.resolve(config("A", { last_error: "retired" })));
    expect(hook.result.current.config?.workspace_id).toBe("B");
    expect(hook.result.current.form.repo_name).toBe("B");
  });

  it("current initial GET errors settle loading while background errors stay silent", async () => {
    vi.useFakeTimers();
    const initial = wire.hold("GET");
    const hook = renderHook(() => useWorkflowSync("A"), { wrapper: Providers });
    await act(async () => initial.reject(new Error("current-initial-error")));
    expect(hook.result.current.loading).toBe(false);
    expect(screen.getByText(/current-initial-error/)).toBeTruthy();
    const background = wire.hold("GET");
    act(() => vi.advanceTimersByTime(INTEGRATION_STATUS_REFRESH_MS));
    await act(async () => background.reject(new Error("silent-background-error")));
    expect(screen.queryByText(/silent-background-error/)).toBeNull();
    const count = wire.requests.length;
    cleanup();
    act(() => vi.advanceTimersByTime(INTEGRATION_STATUS_REFRESH_MS));
    expect(wire.requests).toHaveLength(count);
  });
});

describe("current mutation outcomes", () => {
  it.each([
    { result: { unchanged: false }, message: /sync completed/i, refresh: 1 },
    { result: { unchanged: true }, message: /sync completed/i, refresh: 0 },
    { result: { unchanged: false, warnings: ["warning"] }, message: /warnings/i, refresh: 1 },
    { error: "sync outcome error", message: /sync outcome error/, refresh: 0 },
  ])(
    "current forced-sync outcome $refresh refreshes remain intact",
    async ({ message, refresh, ...outcome }) => {
      const hook = await mount();
      const held = wire.hold("POST", SYNC_PATH);
      let pending!: Promise<void>;
      act(() => {
        pending = hook.result.current.handleSyncNow();
      });
      await act(async () => {
        held.resolve({ config: config(), ...outcome });
        expect(await pending).toBeUndefined();
      });
      expect(screen.getByText(message)).toBeTruthy();
      expect(reload).toHaveBeenCalledTimes(refresh);
      expect(hook.result.current.syncing).toBe(false);
    },
  );

  it("current removal clears configuration and refreshes", async () => {
    const hook = await mount();
    await act(async () => expect(await hook.result.current.handleDelete()).toBe(true));
    expect(hook.result.current.config).toBeNull();
    expect(hook.result.current.form.repo_name).toBe("");
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("current same-workspace save keeps existing reset and GitLab payload semantics", async () => {
    const hook = await mount();
    act(() => {
      hook.result.current.setProvider("gitlab");
      hook.result.current.setUrlInput("group/project");
    });
    const held = wire.hold("POST");
    let pending!: Promise<boolean>;
    act(() => {
      pending = hook.result.current.handleSave();
    });
    act(() => hook.result.current.update("branch", "in-flight-edit"));
    await act(async () => {
      held.resolve(
        config("A", {
          provider: "gitlab",
          project_path: "group/project",
          repo_owner: "",
          repo_name: "",
        }),
      );
      expect(await pending).toBe(true);
    });
    const payload = JSON.parse(wire.requests.find((r) => r.method === "POST")!.body!);
    expect(payload).toMatchObject({
      provider: "gitlab",
      project_path: "group/project",
      branch: "main",
    });
    expect(payload).not.toHaveProperty("repo_owner");
    expect(hook.result.current.form.branch).toBe("main");
    expect(hook.result.current.saving).toBe(false);
  });
});

import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useDevServerPreview } from "@/hooks/domains/session/use-dev-server-preview";
import { DevServerPreviewButton } from "./dev-server-preview-button";

type Store = ReturnType<typeof useAppStoreApi>;
type Command = "start" | "stop";
type Request = {
  url: string;
  resolve: (response: Response) => void;
  reject: (error: Error) => void;
};
let requests: Request[];
let transport: ReturnType<typeof vi.fn>;

function response(session: string) {
  return new Response(
    JSON.stringify({
      process: {
        id: `process-${session}`,
        session_id: session,
        kind: "dev",
        status: "running",
        command: "pnpm dev",
        working_dir: "/repo",
        started_at: "2026-10-21T12:00:00Z",
        updated_at: "2026-10-21T12:00:00Z",
      },
    }),
    { status: 200 },
  );
}

function mount(prefix = "") {
  let store!: Store;
  let dev!: ReturnType<typeof useDevServerPreview>;
  function Capture() {
    store = useAppStoreApi();
    return null;
  }
  function CaptureCommand() {
    const session = useAppStore((s) => s.tasks.activeSessionId);
    dev = useDevServerPreview(session);
    return null;
  }
  function View({ visible = true }: { visible?: boolean }) {
    return (
      <StateProvider>
        <Capture />
        {visible && (
          <>
            <CaptureCommand />
            <DevServerPreviewButton className="" iconClassName="" testId={`${prefix}first`} />
            <DevServerPreviewButton className="" iconClassName="" testId={`${prefix}second`} />
          </>
        )}
      </StateProvider>
    );
  }
  const view = render(<View />);
  const select = (session: string | null) =>
    act(() => {
      if (session) store.getState().setActiveSession(`task-${session}`, session);
      else store.getState().clearActiveSession();
    });
  select("a");
  return {
    store,
    select,
    command: () => dev,
    button: (id = "first") => screen.getByTestId(`${prefix}${id}`) as HTMLButtonElement,
    hide: () => view.rerender(<View visible={false} />),
    show: () => view.rerender(<View />),
    unmount: view.unmount,
  };
}

function running(store: Store, session: string) {
  act(() =>
    store.getState().upsertProcessStatus({
      processId: `process-${session}`,
      sessionId: session,
      kind: "dev",
      status: "running",
    }),
  );
}

async function settle(request: Request, kind: Command = "start", fail = false) {
  await act(async () => {
    if (fail) request.reject(new Error("transport failed"));
    else
      request.resolve(
        kind === "start"
          ? response(request.url.split("/").at(-3)!)
          : new Response(null, { status: 204 }),
      );
  });
}

beforeEach(() => {
  requests = [];
  transport = vi.fn(
    (url: string) =>
      new Promise<Response>((resolve, reject) => {
        requests.push({ url, resolve, reject });
      }),
  );
  vi.stubGlobal("fetch", transport);
});
afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const request of requests) request.resolve(new Response(null, { status: 204 }));
  });
  vi.unstubAllGlobals();
  localStorage.clear();
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.17
it.each(["start", "stop"] as const)(
  "isolates %s pending across navigation and null selection",
  async (kind) => {
    const ui = mount();
    if (kind === "stop") running(ui.store, "a");
    fireEvent.click(ui.button());
    expect(ui.button().disabled).toBe(true);
    expect.soft(ui.button("second").disabled).toBe(true);
    expect(transport).toHaveBeenCalledTimes(1);
    ui.select("b");
    expect.soft(ui.button().disabled).toBe(false);
    expect.soft(ui.command().isPending).toBe(false);
    ui.select(null);
    expect(ui.button().disabled).toBe(true);
    expect.soft(ui.command().isPending).toBe(false);
    ui.select("a");
    expect(ui.button().disabled).toBe(true);
    await settle(requests[0], kind);
    expect(ui.button().disabled).toBe(false);
    expect(ui.button("second").disabled).toBe(false);
  },
);

it.each([
  { kind: "start", first: "a" },
  { kind: "start", first: "b" },
  { kind: "stop", first: "a" },
  { kind: "stop", first: "b" },
] as const)(
  "settles $kind for $first first without clearing the other session command",
  async ({ kind, first }) => {
    const ui = mount();
    if (kind === "stop") {
      running(ui.store, "a");
      running(ui.store, "b");
    }
    fireEvent.click(ui.button());
    ui.select("b");
    fireEvent.click(ui.button("second"));
    expect(requests).toHaveLength(2);
    const a = requests[0],
      b = requests[1];
    await settle(first === "a" ? a : b, kind);
    expect.soft(ui.button().disabled).toBe(first !== "b");
    expect.soft(ui.button("second").disabled).toBe(first !== "b");
    ui.select("a");
    expect.soft(ui.button().disabled).toBe(first !== "a");
    await settle(first === "a" ? b : a, kind);
    expect(ui.button().disabled).toBe(false);
    expect(ui.store.getState().processes.devProcessBySessionId).toEqual({
      a: "process-a",
      b: "process-b",
    });
  },
);

it.each(["start", "stop"] as const)(
  "shares %s failures and permits a fresh retry",
  async (kind) => {
    const ui = mount();
    if (kind === "stop") running(ui.store, "a");
    act(() => {
      void ui.command()[kind]();
    });
    expect.soft(ui.button().disabled).toBe(true);
    expect.soft(ui.button("second").disabled).toBe(true);
    ui.select("b");
    act(() => {
      void ui.command().start();
    });
    await settle(requests[0], kind, true);
    expect.soft(ui.button().disabled).toBe(true);
    expect.soft(ui.button("second").disabled).toBe(true);
    ui.select("a");
    expect(ui.button().disabled).toBe(false);
    act(() => {
      void ui.command()[kind]();
    });
    expect(requests).toHaveLength(3);
    await settle(requests[2], kind);
    await settle(requests[1]);
  },
);

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.18
it.each(["start", "stop"] as const)(
  "admits one same-tick %s and joins opposite commands without queuing",
  async (kind) => {
    const ui = mount();
    if (kind === "stop") running(ui.store, "a");
    let completions = 0;
    act(() => {
      const command = ui.command();
      void command[kind]().then(() => completions++);
      void command[kind]().then(() => completions++);
      void command[kind === "start" ? "stop" : "start"]().then(() => completions++);
    });
    expect.soft(requests).toHaveLength(1);
    expect.soft(ui.button("second").disabled).toBe(true);
    await act(async () => {});
    expect(completions).toBe(0);
    await settle(requests[0], kind);
    expect(completions).toBe(3);
    expect(transport).toHaveBeenCalledTimes(1);
  },
);

it.each(["start", "stop"] as const)(
  "retains %s pending while both buttons unmount and remount",
  async (kind) => {
    const ui = mount();
    if (kind === "stop") running(ui.store, "a");
    fireEvent.click(ui.button());
    ui.hide();
    ui.show();
    expect.soft(ui.button().disabled).toBe(true);
    expect.soft(ui.button("second").disabled).toBe(true);
    await settle(requests[0], kind);
    expect(ui.button().disabled).toBe(false);
    ui.unmount();
  },
);

it("publishes to its originating store after the initiating provider unmounts", async () => {
  const ui = mount();
  fireEvent.click(ui.button());
  ui.unmount();
  await settle(requests[0]);
  expect(ui.store.getState().processes.devProcessBySessionId.a).toBe("process-a");
});

it("keeps independent providers' same-session commands and publications independent", async () => {
  const first = mount("one-"),
    second = mount("two-");
  fireEvent.click(first.button());
  expect(second.button().disabled).toBe(false);
  fireEvent.click(second.button());
  expect.soft(requests).toHaveLength(2);
  await settle(requests[0]);
  expect.soft(second.button().disabled).toBe(true);
  expect.soft(second.store.getState().processes.devProcessBySessionId.a).toBeUndefined();
  if (requests[1]) await settle(requests[1]);
  expect(second.store.getState().processes.devProcessBySessionId.a).toBe("process-a");
});

it("preserves a newer same-process terminal status before the HTTP start response", async () => {
  const ui = mount();
  fireEvent.click(ui.button());
  act(() =>
    ui.store.getState().upsertProcessStatus({
      processId: "process-a",
      sessionId: "a",
      kind: "dev",
      status: "exited",
      updatedAt: "2026-10-21T12:00:05Z",
    }),
  );
  await settle(requests[0]);
  expect(ui.store.getState().processes.processesById["process-a"].status).toBe("exited");
  expect(ui.button().getAttribute("aria-label")).toBe("Start dev server preview");
  expect(ui.button().disabled).toBe(false);
});

import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { useAutoUpdateSettings } from "./use-auto-update-settings";

vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

type HeldWrite = { value: boolean; resolve: (response: Response) => void };
const settings = (value: boolean) =>
  new Response(JSON.stringify({ auto_update_default: value }), { status: 200 });
const failure = () => new Response(JSON.stringify({ error: "write failed" }), { status: 503 });
let held: HeldWrite[] = [];
let calls: Promise<void>[] = [];

function transport(initial: boolean | "failed" = false) {
  let persisted = initial === "failed" ? undefined : initial;
  vi.stubGlobal(
    "fetch",
    vi.fn((_url: string, init: RequestInit) => {
      if (init.method !== "PUT") {
        return Promise.resolve(initial === "failed" ? failure() : settings(initial));
      }
      const value = JSON.parse(String(init.body)).auto_update_default as boolean;
      return new Promise<Response>((resolve) =>
        held.push({
          value,
          resolve: async (response) => {
            if (response.ok) persisted = (await response.clone().json()).auto_update_default;
            resolve(response);
          },
        }),
      );
    }),
  );
  return () => persisted;
}

async function mount(initial: boolean | "failed" = false) {
  const persisted = transport(initial);
  const hook = renderHook(useAutoUpdateSettings);
  expect(hook.result.current.loaded).toBe(false);
  await waitFor(() => expect(hook.result.current.loaded).toBe(true));
  return { ...hook, persisted };
}

function submit(setDefault: (value: boolean) => Promise<void>, value: boolean) {
  act(() => {
    calls.push(setDefault(value));
  });
}

async function settle(index: number, response: Response) {
  await waitFor(() => expect(held[index]).toBeDefined());
  await act(async () => {
    held[index].resolve(response);
  });
}

afterEach(async () => {
  cleanup();
  // Drain accepted writes even when a regression assertion exits early.
  await act(async () => {
    for (let index = 0; index < calls.length; index++) {
      await vi.waitFor(() => expect(held[index]).toBeDefined());
      held[index].resolve(settings(held[index].value));
      await calls[index];
    }
  });
  held = [];
  calls = [];
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

// @covers AC-PLUGINS-MARKETPLACE-004.1, AC-PLUGINS-MARKETPLACE-004.3
it("loads the default and reconciles a single normalized acknowledgement", async () => {
  const { result } = await mount(true);
  expect(result.current.autoUpdateDefault).toBe(true);
  submit(result.current.setDefault, false);
  expect(result.current.autoUpdateDefault).toBe(false);
  await settle(0, settings(true));
  expect(result.current.autoUpdateDefault).toBe(true);
});

// @covers AC-PLUGINS-MARKETPLACE-004.2, AC-PLUGINS-MARKETPLACE-004.3
it.each([
  [true, true, false],
  [false, true, false],
  [true, false, true],
  [false, false, false],
])(
  "settles rapid true/false writes (successes %s/%s) against accepted data",
  async (first, second, expected) => {
    const { result, persisted } = await mount();
    submit(result.current.setDefault, true);
    submit(result.current.setDefault, false);
    await settle(0, first ? settings(true) : failure());
    expect(result.current.autoUpdateDefault).toBe(false);
    await settle(1, second ? settings(false) : failure());
    expect(result.current.autoUpdateDefault).toBe(expected);
    expect(persisted()).toBe(expected);
    expect(toast.error).toHaveBeenCalledTimes(Number(!first) + Number(!second));
  },
);

// @covers AC-PLUGINS-MARKETPLACE-004.2
it("serializes HTTP admission and preserves three same-tick intents", async () => {
  const { result, persisted } = await mount();
  const callback = result.current.setDefault;
  act(() => {
    calls.push(callback(true), callback(false), callback(true));
  });
  await waitFor(() => expect(held).toHaveLength(1));
  expect(result.current.autoUpdateDefault).toBe(true);
  await settle(0, settings(false));
  expect(result.current.autoUpdateDefault).toBe(true);
  await settle(1, settings(false));
  expect(result.current.autoUpdateDefault).toBe(true);
  await settle(2, settings(true));
  expect(held.map((write) => write.value)).toEqual([true, false, true]);
  expect(result.current.autoUpdateDefault).toBe(true);
  expect(persisted()).toBe(true);
});

// @covers AC-PLUGINS-MARKETPLACE-004.3
it("rolls back a single failure to the successful initial read", async () => {
  const { result } = await mount(true);
  submit(result.current.setDefault, false);
  await settle(0, failure());
  expect(result.current.autoUpdateDefault).toBe(true);
  expect(toast.error).toHaveBeenCalledWith("write failed");
});

it("keeps failed-GET fallback usable and later adopts a successful write", async () => {
  const { result, persisted } = await mount("failed");
  expect(persisted()).toBeUndefined();
  expect(toast.error).not.toHaveBeenCalled();
  submit(result.current.setDefault, true);
  await settle(0, failure());
  expect(result.current.autoUpdateDefault).toBe(false);
  expect(persisted()).toBeUndefined();
  submit(result.current.setDefault, true);
  await settle(1, settings(true));
  expect(result.current.autoUpdateDefault).toBe(true);
  expect(persisted()).toBe(true);
});

it("drains accepted writes after unmount without notifying or admitting stale callbacks", async () => {
  const { result, unmount, persisted } = await mount();
  const callback = result.current.setDefault;
  submit(callback, true);
  submit(callback, false);
  unmount();
  await settle(0, failure());
  await settle(1, settings(false));
  let stale!: Promise<void>;
  await act(async () => {
    stale = callback(true);
  });
  for (const write of held.slice(2)) write.resolve(settings(write.value));
  await stale;
  expect(held).toHaveLength(2);
  expect(persisted()).toBe(false);
  expect(toast.error).not.toHaveBeenCalled();
});

it("does not let a late initial read replace an admitted edit", async () => {
  transport();
  const writeTransport = vi.mocked(fetch).getMockImplementation()!;
  let finishRead!: (response: Response) => void;
  vi.mocked(fetch).mockImplementation((url, init) =>
    init?.method === "PUT"
      ? writeTransport(url, init)
      : new Promise<Response>((resolve) => {
          finishRead = resolve;
        }),
  );
  const { result } = renderHook(useAutoUpdateSettings);
  expect(result.current.loaded).toBe(false);
  submit(result.current.setDefault, true);
  await settle(0, settings(true));
  await act(async () => finishRead(settings(false)));
  expect(result.current.loaded).toBe(true);
  expect(result.current.autoUpdateDefault).toBe(true);
});

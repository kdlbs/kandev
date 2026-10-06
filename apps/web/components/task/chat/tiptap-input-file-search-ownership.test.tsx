import { createRef, StrictMode } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import { WebSocketClient } from "@/lib/ws/client";
import type { StoreProviderProps } from "@/lib/state/store";
import { getChatDraftContent, setChatDraftContent } from "@/lib/local-storage";
import { TipTapInput, type TipTapInputHandle } from "./tiptap-input";

type WireRequest = { id: string; action: string; payload: unknown };
type InputRef = ReturnType<typeof createRef<TipTapInputHandle>>;

const MENU = "mention-menu";
const ALPHA_FILE = "unique-alpha.ts";
const BETA_FILE = "unique-beta.ts";
const RETIRED_FILE = "unique-retired.ts";
const CURRENT_FILE = "unique-current.ts";

// Only the wire transport is replaced; the component, provider, editor and API are real.
class DeferredSocket {
  static readonly OPEN = 1;
  static readonly CLOSED = 3;
  static current: DeferredSocket;
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  requests: WireRequest[] = [];
  settled = new Set<string>();

  constructor() {
    DeferredSocket.current = this;
  }

  send(data: string) {
    const request = JSON.parse(data) as WireRequest;
    this.requests.push(request);
  }

  close() {
    this.readyState = DeferredSocket.CLOSED;
  }

  searches() {
    return this.requests.filter((request) => request.action === "workspace.files.search");
  }

  settle(index: number, type: "response" | "error", payload: unknown) {
    const request = this.searches()[index];
    expect(request).toBeDefined();
    expect(this.settled.has(request.id)).toBe(false);
    this.settled.add(request.id);
    this.onmessage?.({ data: JSON.stringify({ id: request.id, type, payload }) });
  }
}

let client: WebSocketClient;
let previousClient: WebSocketClient | null;
let previousDrafts: unknown[];

beforeEach(() => {
  vi.useFakeTimers();
  previousDrafts = [getChatDraftContent("alpha"), getChatDraftContent("beta")];
  setChatDraftContent("alpha", null);
  setChatDraftContent("beta", null);
  previousClient = getWebSocketClient();
  vi.stubGlobal("WebSocket", DeferredSocket);
  client = new WebSocketClient("ws://file-search-test", undefined, { enabled: false });
  client.connect();
  DeferredSocket.current.readyState = DeferredSocket.OPEN;
  DeferredSocket.current.onopen?.();
  setWebSocketClient(client);
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    // Allow the real editor's deferred unmount destruction before discarding timers.
    await vi.advanceTimersByTimeAsync(60);
    DeferredSocket.current.searches().forEach((request, index) => {
      if (!DeferredSocket.current.settled.has(request.id)) {
        DeferredSocket.current.settle(index, "response", { files: [] });
      }
    });
    await Promise.resolve();
  });
  client.disconnect();
  setWebSocketClient(previousClient);
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  setChatDraftContent("alpha", previousDrafts[0]);
  setChatDraftContent("beta", previousDrafts[1]);
});

const initialState: StoreProviderProps["initialState"] = {
  prompts: { items: [], loaded: true, loading: false },
};

function tree(ref: InputRef, sessionId: string | null, state = initialState) {
  return (
    <StateProvider initialState={state}>
      <TipTapInput ref={ref} sessionId={sessionId} value="" onChange={() => {}} />
    </StateProvider>
  );
}

async function flush() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(60);
    await Promise.resolve();
    await Promise.resolve();
  });
}

async function query(ref: InputRef, text = "unique") {
  expect(ref.current?.getTextareaElement()).not.toBeNull();
  act(() => ref.current!.clear());
  await flush();
  act(() => ref.current!.insertText(`@${text}`, 1, 1));
  await flush();
}

async function reply(index: number, files: string[] = []) {
  await act(async () => DeferredSocket.current.settle(index, "response", { files }));
  await flush();
}

async function reject(index: number) {
  await act(async () =>
    DeferredSocket.current.settle(index, "error", { message: "File search failed" }),
  );
  await flush();
}

function wire(index: number, sessionId: string, text = "unique") {
  expect(DeferredSocket.current.searches()[index]?.payload).toEqual({
    session_id: sessionId,
    query: text,
    limit: 20,
  });
}

function menuHas(path: string) {
  expect(screen.getByTestId(MENU).textContent).toContain(path);
}

function menuExcludes(path: string) {
  expect(screen.queryByTestId(MENU)?.textContent ?? "").not.toContain(path);
}

async function mount(sessionId: string | null = "alpha") {
  if (sessionId) setChatDraftContent(sessionId, null);
  const ref = createRef<TipTapInputHandle>();
  const view = render(tree(ref, sessionId));
  await flush();
  return { ref, view };
}

async function switchSession(fixture: Awaited<ReturnType<typeof mount>>, sessionId: string | null) {
  act(() => fixture.ref.current!.clear());
  await flush();
  // Start the selected synthetic session with an empty fixture draft, not a restored query.
  if (sessionId) setChatDraftContent(sessionId, null);
  fixture.view.rerender(tree(fixture.ref, sessionId));
  await flush();
}

describe("TipTapInput completed file cache", () => {
  // @covers AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.1
  it("renders the current session files through the actual editor and wire", async () => {
    const { ref } = await mount();
    await query(ref);
    expect(DeferredSocket.current.searches()).toHaveLength(1);
    wire(0, "alpha");
    await reply(0, [ALPHA_FILE]);
    menuHas(ALPHA_FILE);
  });

  it("reads and renders beta files for the same query in the same mounted editor", async () => {
    const fixture = await mount();
    const editor = fixture.ref.current!.getTextareaElement();
    await query(fixture.ref);
    await reply(0, [ALPHA_FILE]);
    menuHas(ALPHA_FILE);
    await switchSession(fixture, "beta");
    expect(fixture.ref.current!.getTextareaElement()).toBe(editor);
    await query(fixture.ref);
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    wire(1, "beta");
    await reply(1, [BETA_FILE]);
    menuHas(BETA_FILE);
    menuExcludes(ALPHA_FILE);
    await query(fixture.ref);
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    menuHas(BETA_FILE);
  });

  // @covers AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.2
  it("reuses completed exact queries across ordinary same-session rerenders", async () => {
    const fixture = await mount();
    await query(fixture.ref, "MixedCase");
    wire(0, "alpha", "MixedCase");
    await reply(0, ["MixedCase.ts"]);
    fixture.view.rerender(tree(fixture.ref, "alpha"));
    await flush();
    await query(fixture.ref, "MixedCase");
    expect(DeferredSocket.current.searches()).toHaveLength(1);
    menuHas("MixedCase.ts");
    await query(fixture.ref, "other");
    wire(1, "alpha", "other");
    await reply(1, ["other.ts"]);
    menuHas("other.ts");
  });

  it("keeps empty text distinct from a literal __empty__ query", async () => {
    const { ref } = await mount();
    await query(ref, "");
    wire(0, "alpha", "");
    await reply(0, ["empty-result.ts"]);
    menuHas("empty-result.ts");
    await query(ref, "__empty__");
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    wire(1, "alpha", "__empty__");
    await reply(1, ["__empty__.ts"]);
    menuHas("__empty__.ts");
    menuExcludes("empty-result.ts");
  });
});

describe("TipTapInput retired session replies", () => {
  // @covers AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.3
  it.each([
    { order: "before", files: [RETIRED_FILE] },
    { order: "after", files: [RETIRED_FILE] },
    { order: "before", files: [] },
    { order: "after", files: [] },
  ])(
    "rejects old success ($files) settling $order beta success without poisoning reuse",
    async ({ order, files }) => {
      const fixture = await mount();
      await query(fixture.ref);
      await switchSession(fixture, "beta");
      await query(fixture.ref);
      wire(0, "alpha");
      wire(1, "beta");
      if (order === "before") {
        await reply(0, files);
        menuExcludes(RETIRED_FILE);
      }
      await reply(1, [BETA_FILE]);
      menuHas(BETA_FILE);
      if (order === "after") {
        await reply(0, files);
        menuExcludes(RETIRED_FILE);
      }
      await query(fixture.ref);
      expect(DeferredSocket.current.searches()).toHaveLength(2);
      menuHas(BETA_FILE);
      menuExcludes(RETIRED_FILE);
    },
  );

  it.each(["before", "after"] as const)(
    "ignores old errors settling %s beta success",
    async (order) => {
      const fixture = await mount();
      await query(fixture.ref);
      await switchSession(fixture, "beta");
      await query(fixture.ref);
      if (order === "before") await reject(0);
      await reply(1, [BETA_FILE]);
      menuHas(BETA_FILE);
      if (order === "after") await reject(0);
      await query(fixture.ref);
      expect(DeferredSocket.current.searches()).toHaveLength(2);
      menuHas(BETA_FILE);
    },
  );

  it("does not let a retired response become the cache before beta admission", async () => {
    const fixture = await mount();
    await query(fixture.ref);
    await switchSession(fixture, "beta");
    await reply(0, [RETIRED_FILE]);
    await query(fixture.ref);
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    wire(1, "beta");
    await reply(1, [BETA_FILE]);
    menuHas(BETA_FILE);
    menuExcludes(RETIRED_FILE);
  });
});

describe("TipTapInput query and null ownership", () => {
  it("keeps newer different-query results when an older same-session reply settles", async () => {
    const { ref } = await mount();
    await query(ref, "old");
    await query(ref, "new");
    wire(0, "alpha", "old");
    wire(1, "alpha", "new");
    await reply(1, ["new.ts"]);
    menuHas("new.ts");
    await reply(0, ["old.ts"]);
    menuExcludes("old.ts");
    await query(ref, "new");
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    menuHas("new.ts");
  });

  it("a completed cache hit retires an older pending cache miss", async () => {
    const { ref } = await mount();
    await query(ref, "ready");
    await reply(0, ["ready.ts"]);
    await query(ref, "old");
    await query(ref, "ready");
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    menuHas("ready.ts");
    await reply(1, ["old.ts"]);
    menuExcludes("old.ts");
    await query(ref, "ready");
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    menuHas("ready.ts");
  });

  // @covers AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.4
  it("retires pending files at null and reads fresh after returning to a session", async () => {
    const fixture = await mount();
    await query(fixture.ref);
    await switchSession(fixture, null);
    expect(DeferredSocket.current.searches()).toHaveLength(1);
    await reply(0, [RETIRED_FILE]);
    await query(fixture.ref);
    expect(DeferredSocket.current.searches()).toHaveLength(1);
    menuExcludes(RETIRED_FILE);
    await switchSession(fixture, "alpha");
    await query(fixture.ref);
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    wire(1, "alpha");
    await reply(1, [CURRENT_FILE]);
    menuHas(CURRENT_FILE);
  });

  it("cannot revive the original alpha request after an alpha-beta-alpha transition", async () => {
    const fixture = await mount();
    await query(fixture.ref);
    await switchSession(fixture, "beta");
    await switchSession(fixture, "alpha");
    expect(DeferredSocket.current.searches()).toHaveLength(1);
    await query(fixture.ref);
    wire(1, "alpha");
    await reply(1, [CURRENT_FILE]);
    await reply(0, [RETIRED_FILE]);
    menuExcludes(RETIRED_FILE);
    await query(fixture.ref);
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    menuHas(CURRENT_FILE);
  });
});

describe("TipTapInput mounted owner lifetimes", () => {
  // @covers AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.5
  it("keeps independent editors' caches and owner retirement separate", async () => {
    const first = await mount();
    await query(first.ref);
    await reply(0, ["unique-first.ts"]);
    const second = await mount();
    await query(second.ref);
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    await reply(1, ["unique-second.ts"]);
    await switchSession(second, "beta");
    await query(first.ref);
    expect(DeferredSocket.current.searches()).toHaveLength(2);
    expect(
      screen.getAllByTestId(MENU).some((menu) => menu.textContent?.includes("unique-first.ts")),
    ).toBe(true);
  });

  it.each(["response", "error"] as const)("retires %s after unmount", async (type) => {
    const fixture = await mount();
    await query(fixture.ref);
    fixture.view.unmount();
    if (type === "response") await reply(0, [RETIRED_FILE]);
    else await reject(0);
    expect(screen.queryByTestId(MENU)).toBeNull();
    const current = await mount();
    await query(current.ref);
    wire(1, "alpha");
    await reply(1, [CURRENT_FILE]);
    menuHas(CURRENT_FILE);
  });

  it("has a usable current owner after StrictMode effect replay", async () => {
    const ref = createRef<TipTapInputHandle>();
    render(<StrictMode>{tree(ref, "alpha")}</StrictMode>);
    await flush();
    await query(ref);
    wire(0, "alpha");
    await reply(0, [CURRENT_FILE]);
    menuHas(CURRENT_FILE);
    await query(ref);
    expect(DeferredSocket.current.searches()).toHaveLength(1);
    menuHas(CURRENT_FILE);
  });
});

describe("TipTapInput current file fallbacks", () => {
  // @covers AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.6
  it("reuses an empty successful response but retries a current failed query", async () => {
    const { ref } = await mount();
    await query(ref);
    await reply(0);
    await query(ref);
    expect(DeferredSocket.current.searches()).toHaveLength(1);
    await query(ref, "retry");
    await reject(1);
    await query(ref, "retry");
    expect(DeferredSocket.current.searches()).toHaveLength(3);
    wire(2, "alpha", "retry");
    await reply(2, ["retry.ts"]);
    menuHas("retry.ts");
  });

  it("keeps non-file candidates and ranking when the wire fails or client is absent", async () => {
    const ref = createRef<TipTapInputHandle>();
    render(
      tree(ref, "alpha", {
        prompts: {
          items: [
            {
              id: "prompt",
              name: "Saved prompt",
              content: "Prompt content",
              builtin: false,
              created_at: "2026-10-06T00:00:00Z",
              updated_at: "2026-10-06T00:00:00Z",
            },
          ],
          loaded: true,
          loading: false,
        },
        kanban: {
          workflowId: "workflow",
          steps: [],
          tasks: [
            {
              id: "task",
              title: "Sibling task",
              workflowId: "workflow",
              workflowStepId: "step",
              position: 0,
            },
          ],
        },
      }),
    );
    await flush();
    await query(ref, "");
    await reject(0);
    const fallback = screen.getByTestId(MENU).textContent;
    menuHas("Saved prompt");
    menuHas("Sibling task");
    menuHas("Plan");
    setWebSocketClient(null);
    await query(ref, "");
    expect(DeferredSocket.current.searches()).toHaveLength(1);
    expect(screen.getByTestId(MENU).textContent).toBe(fallback);
  });
});

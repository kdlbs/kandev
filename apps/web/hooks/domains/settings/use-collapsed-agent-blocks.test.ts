import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useCollapsedAgentBlocks } from "./use-collapsed-agent-blocks";
import { makeLocalStorageMock } from "@/hooks/local-storage-mock.test-helpers";

const STORAGE_KEY = "kandev:agents:collapsedBlocks:v1";
const SYNC_EVENT = "kandev:agents:collapsed-blocks-changed";

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);
Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
  configurable: true,
});

describe("useCollapsedAgentBlocks defaults", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("defaults every agent to expanded when no record exists", () => {
    const { result } = renderHook(() => useCollapsedAgentBlocks());
    expect(result.current.collapsed("claude")).toBe(false);
    expect(result.current.collapsed("codex")).toBe(false);
  });

  it("defaults to expanded for agents absent from a stored record", () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ claude: true }));
    const { result } = renderHook(() => useCollapsedAgentBlocks());
    expect(result.current.collapsed("claude")).toBe(true);
    expect(result.current.collapsed("codex")).toBe(false);
  });

  it("treats invalid JSON as an empty record", () => {
    window.localStorage.setItem(STORAGE_KEY, "{not-json");
    const { result } = renderHook(() => useCollapsedAgentBlocks());
    expect(result.current.collapsed("claude")).toBe(false);
  });

  it("treats a non-object record as empty", () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify([true]));
    const { result } = renderHook(() => useCollapsedAgentBlocks());
    expect(result.current.collapsed("claude")).toBe(false);
  });

  it("treats only the literal boolean true as collapsed", () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ claude: "yes", codex: 1 }));
    const { result } = renderHook(() => useCollapsedAgentBlocks());
    expect(result.current.collapsed("claude")).toBe(false);
    expect(result.current.collapsed("codex")).toBe(false);
  });
});

describe("useCollapsedAgentBlocks persistence", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("setCollapsed persists one entry and updates state", async () => {
    const { result } = renderHook(() => useCollapsedAgentBlocks());
    expect(result.current.collapsed("claude")).toBe(false);

    act(() => result.current.setCollapsed("claude", true));

    await waitFor(() => expect(result.current.collapsed("claude")).toBe(true));
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe(JSON.stringify({ claude: true }));
  });

  it("setCollapsed merges into an existing record without touching other agents", async () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ codex: true }));
    const { result } = renderHook(() => useCollapsedAgentBlocks());

    act(() => result.current.setCollapsed("claude", true));

    await waitFor(() => expect(result.current.collapsed("claude")).toBe(true));
    expect(result.current.collapsed("codex")).toBe(true);
    // Key order in the stored JSON is an object-spread artifact, not contract.
    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "{}")).toEqual({
      claude: true,
      codex: true,
    });
  });

  it("setCollapsed(false) writes an explicit expanded entry for that agent", async () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ claude: true }));
    const { result } = renderHook(() => useCollapsedAgentBlocks());

    act(() => result.current.setCollapsed("claude", false));

    await waitFor(() => expect(result.current.collapsed("claude")).toBe(false));
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe(JSON.stringify({ claude: false }));
  });
});

describe("useCollapsedAgentBlocks sync", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("propagates updates dispatched via the sync event", async () => {
    const { result } = renderHook(() => useCollapsedAgentBlocks());
    expect(result.current.collapsed("claude")).toBe(false);

    act(() => {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ claude: true }));
      window.dispatchEvent(new Event(SYNC_EVENT));
    });

    await waitFor(() => expect(result.current.collapsed("claude")).toBe(true));
  });

  it("propagates cross-tab storage events for the same key", async () => {
    const { result } = renderHook(() => useCollapsedAgentBlocks());
    expect(result.current.collapsed("claude")).toBe(false);

    act(() => {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ claude: true }));
      window.dispatchEvent(new Event("storage"));
    });

    await waitFor(() => expect(result.current.collapsed("claude")).toBe(true));
  });
});

describe("useCollapsedAgentBlocks failure modes", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("degrades to expanded when localStorage.getItem throws", () => {
    const original = localStorageMock.getItem;
    localStorageMock.getItem = () => {
      throw new Error("quota exceeded");
    };
    try {
      const { result } = renderHook(() => useCollapsedAgentBlocks());
      expect(result.current.collapsed("claude")).toBe(false);
    } finally {
      localStorageMock.getItem = original;
    }
  });

  it("throws but still applies the toggle in memory when localStorage.setItem fails", async () => {
    const original = localStorageMock.setItem;
    localStorageMock.setItem = () => {
      throw new Error("quota exceeded");
    };
    try {
      const { result } = renderHook(() => useCollapsedAgentBlocks());
      expect(() => result.current.setCollapsed("claude", true)).toThrow();
      // The write failed, but the session override keeps the toggle working.
      await waitFor(() => expect(result.current.collapsed("claude")).toBe(true));
      expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
    } finally {
      localStorageMock.setItem = original;
    }
  });

  it("lets a collapsed agent expand when the write fails, and drops the override after a successful write", async () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ claude: true }));
    const { result } = renderHook(() => useCollapsedAgentBlocks());
    expect(result.current.collapsed("claude")).toBe(true);

    // Expand while writes fail: the override applies for the session.
    const original = localStorageMock.setItem;
    localStorageMock.setItem = () => {
      throw new Error("quota exceeded");
    };
    try {
      expect(() => result.current.setCollapsed("claude", false)).toThrow();
      await waitFor(() => expect(result.current.collapsed("claude")).toBe(false));
      expect(window.localStorage.getItem(STORAGE_KEY)).toBe(JSON.stringify({ claude: true }));
    } finally {
      localStorageMock.setItem = original;
    }

    // A later successful write makes storage authoritative again.
    act(() => result.current.setCollapsed("claude", false));
    await waitFor(() => expect(result.current.collapsed("claude")).toBe(false));
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe(JSON.stringify({ claude: false }));
  });
});

describe("useCollapsedAgentBlocks storage event filtering", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("ignores storage events for other keys", () => {
    const { result } = renderHook(() => useCollapsedAgentBlocks());

    act(() => {
      window.localStorage.setItem("kandev:other:flag:v1", JSON.stringify({ claude: true }));
      window.dispatchEvent(new Event("storage"));
    });

    expect(result.current.collapsed("claude")).toBe(false);
  });
});

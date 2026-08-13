import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { makeLocalStorageMock } from "../../local-storage-mock.test-helpers";
import {
  integrationEnabledStorageKey,
  readIntegrationEnabled,
  useIntegrationEnabled,
  useIntegrationEnabledReader,
} from "./use-integration-enabled";

const STORAGE_KEY = "kandev:acme:enabled:v1";
const LEGACY_PREFIX = "kandev:acme:enabled:";
const SYNC_EVENT = "kandev:acme:enabled-changed";
const WORKSPACE_A = "ws-a";
const WORKSPACE_B = "ws-b";

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);
Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
  configurable: true,
});

function renderEnabled(workspaceId?: string | null) {
  return renderHook(() =>
    useIntegrationEnabled(STORAGE_KEY, LEGACY_PREFIX, SYNC_EVENT, workspaceId),
  );
}

describe("useIntegrationEnabled workspace scoping", () => {
  beforeEach(() => localStorageMock.clear());
  afterEach(() => localStorageMock.clear());

  it("writes each workspace's toggle to its own key", () => {
    const { result } = renderEnabled(WORKSPACE_A);

    act(() => result.current.setEnabled(false));

    expect(window.localStorage.getItem(`${STORAGE_KEY}:${WORKSPACE_A}`)).toBe("false");
    // The pre-scoping install-wide key is never written, so it keeps working as
    // the seed for workspaces that have not been toggled.
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it("leaves the other workspaces enabled when one turns the integration off", () => {
    const a = renderEnabled(WORKSPACE_A);
    act(() => a.result.current.setEnabled(false));

    const b = renderEnabled(WORKSPACE_B);

    expect(a.result.current.enabled).toBe(false);
    expect(b.result.current.enabled).toBe(true);
  });

  it("re-reads for the workspace it is given when the active workspace changes", () => {
    window.localStorage.setItem(`${STORAGE_KEY}:${WORKSPACE_B}`, "false");
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) =>
        useIntegrationEnabled(STORAGE_KEY, LEGACY_PREFIX, SYNC_EVENT, workspaceId),
      { initialProps: { workspaceId: WORKSPACE_A } },
    );
    expect(result.current.enabled).toBe(true);

    rerender({ workspaceId: WORKSPACE_B });

    expect(result.current.enabled).toBe(false);
  });

  it("seeds an untouched workspace from the pre-scoping install-wide value", () => {
    // Upgrading must not silently re-enable an integration the user had turned
    // off while the toggle was install-wide.
    window.localStorage.setItem(STORAGE_KEY, "false");

    const { result } = renderEnabled(WORKSPACE_A);

    expect(result.current.enabled).toBe(false);
  });

  it("lets a workspace override the install-wide value it was seeded from", () => {
    window.localStorage.setItem(STORAGE_KEY, "false");
    const a = renderEnabled(WORKSPACE_A);

    act(() => a.result.current.setEnabled(true));

    expect(a.result.current.enabled).toBe(true);
    expect(renderEnabled(WORKSPACE_B).result.current.enabled).toBe(false);
  });

  it("defaults to enabled with no stored value at either scope", () => {
    expect(renderEnabled(WORKSPACE_A).result.current.enabled).toBe(true);
  });

  it("reads and writes the bare key when no workspace is known", () => {
    const { result } = renderEnabled(undefined);

    act(() => result.current.setEnabled(false));

    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("false");
  });

  it("folds a pre-v1 legacy key in without touching the per-workspace keys", () => {
    // The scoped keys start with the legacy prefix too; folding them together
    // would let one workspace's toggle overwrite every other workspace's.
    window.localStorage.setItem(`${LEGACY_PREFIX}ws-legacy`, "false");
    window.localStorage.setItem(`${STORAGE_KEY}:${WORKSPACE_B}`, "true");

    const { result } = renderEnabled(WORKSPACE_A);

    expect(result.current.enabled).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("false");
    expect(window.localStorage.getItem(`${LEGACY_PREFIX}ws-legacy`)).toBeNull();
    expect(window.localStorage.getItem(`${STORAGE_KEY}:${WORKSPACE_B}`)).toBe("true");
  });

  it("propagates a same-tab toggle to the other slider on the same workspace", () => {
    const { result } = renderEnabled(WORKSPACE_A);

    act(() => {
      window.localStorage.setItem(`${STORAGE_KEY}:${WORKSPACE_A}`, "false");
      window.dispatchEvent(new Event(SYNC_EVENT));
    });

    expect(result.current.enabled).toBe(false);
  });
});

describe("readIntegrationEnabled", () => {
  beforeEach(() => localStorageMock.clear());

  it("keys the value by workspace, falling back to the install-wide value", () => {
    window.localStorage.setItem(STORAGE_KEY, "false");
    window.localStorage.setItem(`${STORAGE_KEY}:${WORKSPACE_A}`, "true");

    expect(readIntegrationEnabled(STORAGE_KEY, WORKSPACE_A)).toBe(true);
    expect(readIntegrationEnabled(STORAGE_KEY, WORKSPACE_B)).toBe(false);
    expect(integrationEnabledStorageKey(STORAGE_KEY, WORKSPACE_A)).toBe(
      `${STORAGE_KEY}:${WORKSPACE_A}`,
    );
    expect(integrationEnabledStorageKey(STORAGE_KEY, null)).toBe(STORAGE_KEY);
  });
});

describe("useIntegrationEnabledReader", () => {
  beforeEach(() => localStorageMock.clear());

  it("reads any workspace's toggle without a hook per workspace", () => {
    window.localStorage.setItem(`${STORAGE_KEY}:${WORKSPACE_A}`, "false");
    const { result } = renderHook(() => useIntegrationEnabledReader([SYNC_EVENT]));

    expect(result.current(STORAGE_KEY, WORKSPACE_A)).toBe(false);
    expect(result.current(STORAGE_KEY, WORKSPACE_B)).toBe(true);
  });

  it("takes a new identity on a toggle, so memoized consumers recompute", () => {
    const { result } = renderHook(() => useIntegrationEnabledReader([SYNC_EVENT]));
    const before = result.current;

    act(() => {
      window.localStorage.setItem(`${STORAGE_KEY}:${WORKSPACE_A}`, "false");
      window.dispatchEvent(new Event(SYNC_EVENT));
    });

    expect(result.current).not.toBe(before);
    expect(result.current(STORAGE_KEY, WORKSPACE_A)).toBe(false);
  });

  it("also reacts to another tab's write", () => {
    const { result } = renderHook(() => useIntegrationEnabledReader([SYNC_EVENT]));
    const before = result.current;

    act(() => {
      window.localStorage.setItem(`${STORAGE_KEY}:${WORKSPACE_A}`, "false");
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: `${STORAGE_KEY}:${WORKSPACE_A}`,
          newValue: "false",
        }),
      );
    });

    expect(result.current).not.toBe(before);
  });
});

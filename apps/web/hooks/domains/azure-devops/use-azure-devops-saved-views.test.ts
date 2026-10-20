import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  getAzureDevOpsSavedViews,
  setAzureDevOpsSavedViews,
} from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsSavedView } from "@/lib/types/azure-devops";
import { useAzureDevOpsSavedViews } from "./use-azure-devops-saved-views";

vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  getAzureDevOpsSavedViews: vi.fn(),
  setAzureDevOpsSavedViews: vi.fn(),
}));

const WORKSPACE_ID = "workspace-1";
const view: AzureDevOpsSavedView = {
  id: "mine",
  kind: "work_item",
  label: "My work",
  projectId: "project-1",
  wiql: "SELECT [System.Id] FROM WorkItems",
  createdAt: "2026-07-20T12:00:00Z",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    reject = rejectPromise;
    resolve = resolvePromise;
  });
  return { promise, resolve, reject };
}

describe("useAzureDevOpsSavedViews", () => {
  beforeEach(() => {
    vi.mocked(getAzureDevOpsSavedViews).mockReset();
    vi.mocked(setAzureDevOpsSavedViews).mockReset();
  });

  it("loads saved views and treats a load failure as empty", async () => {
    vi.mocked(getAzureDevOpsSavedViews)
      .mockResolvedValueOnce({ views: [view] })
      .mockRejectedValueOnce(new Error("unavailable"));

    const loaded = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(loaded.result.current.loading).toBe(false));
    expect(loaded.result.current.views).toEqual([view]);
    loaded.unmount();

    const failed = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(failed.result.current.loading).toBe(false));
    expect(failed.result.current.views).toEqual([]);
  });

  it("optimistically replaces views and commits the server-normalized list", async () => {
    const response = deferred<{ views: AzureDevOpsSavedView[] }>();
    const normalized = { ...view, label: "Normalized" };
    vi.mocked(getAzureDevOpsSavedViews).mockResolvedValue({ views: [] });
    vi.mocked(setAzureDevOpsSavedViews).mockReturnValue(response.promise);
    const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(result.current.loading).toBe(false));

    let replacePromise!: Promise<void>;
    act(() => {
      replacePromise = result.current.replace([view]);
    });
    expect(result.current.views).toEqual([view]);

    await act(async () => {
      response.resolve({ views: [normalized] });
      await replacePromise;
    });
    expect(result.current.views).toEqual([normalized]);
  });

  it("rolls back an optimistic replacement when persistence fails", async () => {
    vi.mocked(getAzureDevOpsSavedViews).mockResolvedValue({ views: [view] });
    vi.mocked(setAzureDevOpsSavedViews).mockRejectedValue(new Error("save failed"));
    const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(result.current.views).toEqual([view]));

    const replacement = { ...view, id: "replacement" };
    await expect(
      act(async () => {
        await result.current.replace([replacement]);
      }),
    ).rejects.toThrow("save failed");
    expect(result.current.views).toEqual([view]);
  });

  it("removes the target view through replace", async () => {
    const other = { ...view, id: "other" };
    vi.mocked(getAzureDevOpsSavedViews).mockResolvedValue({ views: [view, other] });
    vi.mocked(setAzureDevOpsSavedViews).mockResolvedValue({ views: [other] });
    const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(result.current.views).toEqual([view, other]));

    await act(async () => result.current.remove(view.id));
    expect(setAzureDevOpsSavedViews).toHaveBeenCalledWith(WORKSPACE_ID, [other]);
    expect(result.current.views).toEqual([other]);
  });

  it("ignores an in-flight load after unmount", async () => {
    const response = deferred<{ views: AzureDevOpsSavedView[] }>();
    vi.mocked(getAzureDevOpsSavedViews).mockReturnValue(response.promise);
    const hook = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    hook.unmount();

    await act(async () => {
      response.resolve({ views: [view] });
      await response.promise;
    });
    expect(hook.result.current.views).toEqual([]);
  });
});

// @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.15
// @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.16
const other = { ...view, id: "other", label: "Other view" };

describe("saved-view mutation ordering", () => {
  beforeEach(() => vi.resetAllMocks());

  it.each([
    [false, false],
    [true, false],
    [false, true],
    [true, true],
  ])(
    "serializes deletes and reconciles failures (first=%s, second=%s)",
    async (failFirst, failSecond) => {
      const first = deferred<{ views: AzureDevOpsSavedView[] }>();
      const second = deferred<{ views: AzureDevOpsSavedView[] }>();
      let persisted = [view, other];
      vi.mocked(getAzureDevOpsSavedViews).mockResolvedValue({ views: persisted });
      vi.mocked(setAzureDevOpsSavedViews)
        .mockImplementationOnce(async (_workspace, next) => {
          await first.promise;
          persisted = next;
          return { views: next };
        })
        .mockImplementationOnce(async (_workspace, next) => {
          await second.promise;
          persisted = next;
          return { views: next };
        });
      const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
      await waitFor(() => expect(result.current.loading).toBe(false));
      let firstResult!: Promise<unknown>;
      let secondResult!: Promise<unknown>;
      act(() => {
        firstResult = result.current.remove(view.id).catch((error: unknown) => error);
      });
      expect(result.current.views).toEqual([other]);
      act(() => {
        secondResult = result.current.remove(other.id).catch((error: unknown) => error);
      });
      expect(result.current.views).toEqual([]);
      await waitFor(() => expect(setAzureDevOpsSavedViews).toHaveBeenCalledTimes(1));
      const firstError = new Error("first failed");
      await act(async () => {
        if (failFirst) first.reject(firstError);
        else first.resolve({ views: [other] });
        expect(await firstResult).toBe(failFirst ? firstError : undefined);
      });
      const secondPayload = failFirst ? [view] : [];
      await waitFor(() => expect(setAzureDevOpsSavedViews).toHaveBeenCalledTimes(2));
      expect(setAzureDevOpsSavedViews).toHaveBeenNthCalledWith(2, WORKSPACE_ID, secondPayload);
      expect(result.current.views).toEqual(secondPayload);
      const secondError = new Error("second failed");
      await act(async () => {
        if (failSecond) second.reject(secondError);
        else second.resolve({ views: secondPayload });
        expect(await secondResult).toBe(failSecond ? secondError : undefined);
      });
      const confirmed = failFirst ? [view, other] : [other];
      const expected = failSecond ? confirmed : secondPayload;
      expect(result.current.views).toEqual(expected);
      expect(persisted).toEqual(expected);
    },
  );

  it("keeps both same-tick deletes without waiting for a React render", async () => {
    vi.mocked(getAzureDevOpsSavedViews).mockResolvedValue({ views: [view, other] });
    vi.mocked(setAzureDevOpsSavedViews).mockImplementation(async (_workspace, next) => ({
      views: next,
    }));
    const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => {
      await Promise.all([result.current.remove(view.id), result.current.remove(other.id)]);
    });
    expect(result.current.views).toEqual([]);
    expect(setAzureDevOpsSavedViews).toHaveBeenNthCalledWith(2, WORKSPACE_ID, []);
  });
});

// @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.17
describe("saved-view initial read", () => {
  beforeEach(() => vi.resetAllMocks());

  it.each([false, true])(
    "rebases an edit on a pending initial GET (failure=%s)",
    async (failWrite) => {
      const read = deferred<{ views: AzureDevOpsSavedView[] }>();
      const write = deferred<{ views: AzureDevOpsSavedView[] }>();
      vi.mocked(getAzureDevOpsSavedViews).mockReturnValue(read.promise);
      vi.mocked(setAzureDevOpsSavedViews).mockReturnValue(write.promise);
      const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
      let outcome!: Promise<unknown>;
      act(() => {
        outcome = result.current.remove(view.id).catch((error: unknown) => error);
      });
      expect(result.current.views).toEqual([]);
      expect(setAzureDevOpsSavedViews).not.toHaveBeenCalled();
      await act(async () => {
        read.resolve({ views: [view, other] });
        await read.promise;
      });
      await waitFor(() =>
        expect(setAzureDevOpsSavedViews).toHaveBeenCalledWith(WORKSPACE_ID, [other]),
      );
      expect(result.current.views).toEqual([other]);
      expect(result.current.loading).toBe(false);
      const error = new Error("write failed");
      await act(async () => {
        if (failWrite) write.reject(error);
        else write.resolve({ views: [other] });
        expect(await outcome).toBe(failWrite ? error : undefined);
      });
      expect(result.current.views).toEqual(failWrite ? [view, other] : [other]);
      expect(result.current.loading).toBe(false);
    },
  );
});

// @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.16
// Page saves append through a functional replacement, preserving independent intent.
describe("saved-view saves", () => {
  beforeEach(() => vi.resetAllMocks());

  it.each([
    [false, false],
    [true, false],
    [false, true],
    [true, true],
  ])(
    "rebases consecutive saves on normalized success or failure (first=%s, second=%s)",
    async (failFirst, failSecond) => {
      const first = deferred<{ views: AzureDevOpsSavedView[] }>();
      const second = deferred<{ views: AzureDevOpsSavedView[] }>();
      const normalized = { ...view, label: "Normalized by server" };
      let persisted: AzureDevOpsSavedView[] = [];
      vi.mocked(getAzureDevOpsSavedViews).mockResolvedValue({ views: [] });
      vi.mocked(setAzureDevOpsSavedViews)
        .mockImplementationOnce(async () => {
          const response = await first.promise;
          persisted = response.views;
          return response;
        })
        .mockImplementationOnce(async (_workspace, next) => {
          await second.promise;
          persisted = next;
          return { views: next };
        });
      const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
      await waitFor(() => expect(result.current.loading).toBe(false));
      let savedFirst!: Promise<unknown>;
      let savedSecond!: Promise<unknown>;
      act(() => {
        savedFirst = result.current
          .replace((current) => [...current, view])
          .catch((error: unknown) => error);
        savedSecond = result.current
          .replace((current) => [...current, other])
          .catch((error: unknown) => error);
      });
      expect(result.current.views).toEqual([view, other]);
      await waitFor(() => expect(setAzureDevOpsSavedViews).toHaveBeenCalledTimes(1));
      const firstError = new Error("first save failed");
      await act(async () => {
        if (failFirst) first.reject(firstError);
        else first.resolve({ views: [normalized] });
        expect(await savedFirst).toBe(failFirst ? firstError : undefined);
      });
      const baseline = failFirst ? [] : [normalized];
      await waitFor(() => expect(setAzureDevOpsSavedViews).toHaveBeenCalledTimes(2));
      expect(setAzureDevOpsSavedViews).toHaveBeenNthCalledWith(2, WORKSPACE_ID, [
        ...baseline,
        other,
      ]);
      expect(result.current.views).toEqual([...baseline, other]);
      const secondError = new Error("second save failed");
      await act(async () => {
        if (failSecond) second.reject(secondError);
        else second.resolve({ views: [...baseline, other] });
        expect(await savedSecond).toBe(failSecond ? secondError : undefined);
      });
      const expected = failSecond ? baseline : [...baseline, other];
      expect(result.current.views).toEqual(expected);
      expect(persisted).toEqual(expected);
      vi.mocked(setAzureDevOpsSavedViews).mockImplementation(async (_workspace, next) => {
        persisted = next;
        return { views: next };
      });
      await act(async () =>
        result.current.replace((current) => [...current, { ...view, id: "retry" }]),
      );
      expect(persisted).toEqual([...expected, { ...view, id: "retry" }]);
      expect(result.current.views).toEqual(persisted);
    },
  );

  it("preserves a later save when an earlier delete fails", async () => {
    const deletion = deferred<{ views: AzureDevOpsSavedView[] }>();
    vi.mocked(getAzureDevOpsSavedViews).mockResolvedValue({ views: [view] });
    vi.mocked(setAzureDevOpsSavedViews)
      .mockReturnValueOnce(deletion.promise)
      .mockImplementation(async (_workspace, next) => ({ views: next }));
    const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(result.current.loading).toBe(false));
    let removed!: Promise<unknown>;
    let saved!: Promise<void>;
    act(() => {
      removed = result.current.remove(view.id).catch((error: unknown) => error);
      saved = result.current.replace((current) => [...current, other]);
    });
    expect(result.current.views).toEqual([other]);
    const error = new Error("delete failed");
    await act(async () => {
      deletion.reject(error);
      expect(await removed).toBe(error);
      await saved;
    });
    expect(setAzureDevOpsSavedViews).toHaveBeenNthCalledWith(2, WORKSPACE_ID, [view, other]);
    expect(result.current.views).toEqual([view, other]);
  });
});

describe("saved-view page lifetime", () => {
  beforeEach(() => vi.resetAllMocks());

  it("finishes admitted writes for an unmounted page without publishing into a new keyed page", async () => {
    const read = deferred<{ views: AzureDevOpsSavedView[] }>();
    vi.mocked(getAzureDevOpsSavedViews)
      .mockReturnValueOnce(read.promise)
      .mockResolvedValueOnce({ views: [other] });
    vi.mocked(setAzureDevOpsSavedViews).mockImplementation(async (_workspace, next) => ({
      views: next,
    }));
    const previousPage = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    let removed!: Promise<void>;
    act(() => {
      removed = previousPage.result.current.remove(view.id);
    });
    previousPage.unmount();
    const nextPage = renderHook(() => useAzureDevOpsSavedViews("workspace-2"));
    await waitFor(() => expect(nextPage.result.current.loading).toBe(false));
    await act(async () => {
      read.resolve({ views: [view] });
      await removed;
    });
    expect(setAzureDevOpsSavedViews).toHaveBeenCalledWith(WORKSPACE_ID, []);
    expect(nextPage.result.current.views).toEqual([other]);
  });
});

describe("failed saved-view baselines", () => {
  beforeEach(() => vi.resetAllMocks());

  it.each(["save", "delete"])(
    "rejects %s after a failed initial read and reloads the baseline on retry",
    async (action) => {
      const read = deferred<{ views: AzureDevOpsSavedView[] }>();
      const retryRead = deferred<{ views: AzureDevOpsSavedView[] }>();
      let persisted = [other];
      vi.mocked(getAzureDevOpsSavedViews)
        .mockReturnValueOnce(read.promise)
        .mockReturnValueOnce(retryRead.promise);
      vi.mocked(setAzureDevOpsSavedViews).mockImplementation(async (_workspace, next) => {
        persisted = next;
        return { views: next };
      });
      const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
      const submit = () =>
        action === "save"
          ? result.current.replace((current) => [...current, view])
          : result.current.remove(other.id);
      let failed!: Promise<unknown>;
      act(() => {
        failed = submit().catch((error: unknown) => error);
      });
      const error = new Error("load failed");
      await act(async () => {
        read.reject(error);
        expect(await failed).toBe(error);
      });
      expect(setAzureDevOpsSavedViews).not.toHaveBeenCalled();
      expect(persisted).toEqual([other]);
      expect(result.current.views).toEqual([]);
      expect(result.current.loading).toBe(false);
      let retry!: Promise<void>;
      act(() => {
        retry = submit();
      });
      expect(getAzureDevOpsSavedViews).toHaveBeenCalledTimes(2);
      expect(setAzureDevOpsSavedViews).not.toHaveBeenCalled();
      await act(async () => {
        retryRead.resolve({ views: [other] });
        await retry;
      });
      const expected = action === "save" ? [other, view] : [];
      expect(setAzureDevOpsSavedViews).toHaveBeenCalledWith(WORKSPACE_ID, expected);
      expect(persisted).toEqual(expected);
      expect(result.current.views).toEqual(expected);
    },
  );

  it("rejects all actions admitted against a failed read even when a retry starts between settlements", async () => {
    const read = deferred<{ views: AzureDevOpsSavedView[] }>();
    const retryRead = deferred<{ views: AzureDevOpsSavedView[] }>();
    vi.mocked(getAzureDevOpsSavedViews)
      .mockReturnValueOnce(read.promise)
      .mockReturnValueOnce(retryRead.promise);
    vi.mocked(setAzureDevOpsSavedViews).mockImplementation(async (_workspace, next) => ({
      views: next,
    }));
    const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    const retryView = { ...view, id: "retry" };
    let first!: Promise<unknown>;
    let second!: Promise<unknown>;
    let retry!: Promise<void>;
    act(() => {
      first = result.current
        .replace((current) => [...current, view])
        .catch((error: unknown) => {
          retry = result.current.replace((current) => [...current, retryView]);
          return error;
        });
      second = result.current.remove(other.id).catch((error: unknown) => error);
    });
    const error = new Error("initial load failed");
    await act(async () => {
      read.reject(error);
      expect(await first).toBe(error);
      expect(await second).toBe(error);
    });
    expect(setAzureDevOpsSavedViews).not.toHaveBeenCalled();
    expect(getAzureDevOpsSavedViews).toHaveBeenCalledTimes(2);
    expect(result.current.views).toEqual([retryView]);
    await act(async () => {
      retryRead.resolve({ views: [other] });
      await retry;
    });
    expect(setAzureDevOpsSavedViews).toHaveBeenCalledExactlyOnceWith(WORKSPACE_ID, [
      other,
      retryView,
    ]);
    expect(result.current.views).toEqual([other, retryView]);
  });
});

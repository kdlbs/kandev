import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SandboxBinding } from "@/lib/types/sandbox";
import { useSandboxBinding } from "./use-sandbox-binding";
import { ApiError } from "@/lib/api/client";

const api = vi.hoisted(() => ({ fetchSandboxBinding: vi.fn(), updateSandboxBinding: vi.fn() }));
vi.mock("@/lib/api/domains/sandbox-api", () => api);
const binding: SandboxBinding = {
  scope: "agent",
  target_id: "agent",
  workspace_id: "workspace",
  version: 4,
  revision_id: "saved",
};
beforeEach(() => {
  vi.clearAllMocks();
  api.fetchSandboxBinding.mockResolvedValue(binding);
});

describe("sandbox binding drafts", () => {
  it("does not read while disabled and discards data when authority changes", async () => {
    let finish!: (value: SandboxBinding) => void;
    api.fetchSandboxBinding.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const { result, rerender } = renderHook(
      ({ enabled, identity }) => useSandboxBinding("agent", "agent", { enabled, identity }),
      { initialProps: { enabled: false, identity: "operator" } },
    );
    expect(api.fetchSandboxBinding).not.toHaveBeenCalled();
    rerender({ enabled: true, identity: "operator" });
    await waitFor(() => expect(api.fetchSandboxBinding).toHaveBeenCalledOnce());
    rerender({ enabled: false, identity: "member" });
    await act(async () => {
      finish(binding);
    });
    expect(result.current.saved).toBeNull();
    await expect(result.current.save()).rejects.toBeDefined();
    expect(api.updateSandboxBinding).not.toHaveBeenCalled();
  });
  it("rejects a late save after the editor authority changes", async () => {
    let finish!: (value: SandboxBinding) => void;
    api.updateSandboxBinding.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const { result, rerender } = renderHook(
      ({ identity }) => useSandboxBinding("agent", "agent", { identity }),
      { initialProps: { identity: "operator" } },
    );
    await waitFor(() => expect(result.current.saved).not.toBeNull());
    let outcome!: Promise<string>;
    act(() => {
      outcome = result.current.save().then(
        () => "success",
        () => "discarded",
      );
    });
    rerender({ identity: "another-user" });
    await act(async () => {
      finish({ ...binding, version: 5 });
    });
    expect(await outcome).toBe("discarded");
    expect(result.current.saved?.version).toBe(4);
  });
});

describe("sandbox binding writes", () => {
  it("submits only one write when save is called twice before rerender", async () => {
    let finish!: (value: SandboxBinding) => void;
    api.updateSandboxBinding.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const { result } = renderHook(() => useSandboxBinding("agent", "agent"));
    await waitFor(() => expect(result.current.saved).not.toBeNull());
    let first!: Promise<void>;
    let second!: Promise<unknown>;
    act(() => {
      first = result.current.save();
      second = result.current.save().catch(() => undefined);
    });
    expect(api.updateSandboxBinding).toHaveBeenCalledTimes(1);
    await act(async () => {
      finish({ ...binding, version: 5 });
      await Promise.all([first, second]);
    });
  });
  it("keeps edits made during save and advances only the saved version", async () => {
    let finish!: (value: SandboxBinding) => void;
    api.updateSandboxBinding.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const { result } = renderHook(() => useSandboxBinding("agent", "agent"));
    await waitFor(() => expect(result.current.saved?.version).toBe(4));
    act(() => result.current.update({ revision_id: "first" }));
    let save!: Promise<void>;
    act(() => {
      save = result.current.save();
    });
    act(() => result.current.update({ revision_id: "second" }));
    await act(async () => {
      finish({ ...binding, revision_id: "first", version: 5 });
      await save;
    });
    expect(result.current.saved?.revision_id).toBe("first");
    expect(result.current.draft?.revision_id).toBe("second");
    expect(result.current.draft?.version).toBe(5);
    expect(result.current.dirty).toBe(true);
    act(() => result.current.discard());
    expect(result.current.draft?.revision_id).toBe("first");
  });

  it("preserves the draft and version after a conflict", async () => {
    api.updateSandboxBinding.mockRejectedValue(
      new ApiError("conflict", 409, { error: "sandbox_revision_conflict" }),
    );
    const { result } = renderHook(() => useSandboxBinding("agent", "agent"));
    await waitFor(() => expect(result.current.saved).not.toBeNull());
    act(() => result.current.update({ revision_id: "draft" }));
    await act(async () => {
      await expect(result.current.save()).rejects.toThrow();
    });
    expect(result.current.draft?.revision_id).toBe("draft");
    expect(result.current.draft?.version).toBe(4);
    expect(result.current.saved?.revision_id).toBe("saved");
    expect(result.current.errorMessage).toBe("agents:sandboxRevisionConflict");
  });

  it("ignores a late binding from the previous target", async () => {
    let finish!: (value: SandboxBinding) => void;
    api.fetchSandboxBinding.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    api.fetchSandboxBinding.mockResolvedValueOnce({
      ...binding,
      target_id: "new",
      revision_id: "new-policy",
    });
    const { result, rerender } = renderHook(({ id }) => useSandboxBinding("agent", id), {
      initialProps: { id: "old" },
    });
    rerender({ id: "new" });
    await waitFor(() => expect(result.current.saved?.target_id).toBe("new"));
    await act(async () => {
      finish({ ...binding, target_id: "old" });
    });
    expect(result.current.saved?.revision_id).toBe("new-policy");
  });
});

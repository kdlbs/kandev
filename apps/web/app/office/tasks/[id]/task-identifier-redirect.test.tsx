import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useTaskIdentifierRedirect } from "./task-identifier-redirect";

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  resolve: vi.fn(),
  activeId: "ws-active" as string | null,
  search: "",
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace: mocks.replace }),
  useSearchParams: () => new URLSearchParams(mocks.search),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: unknown) => unknown) =>
    selector({ workspaces: { activeId: mocks.activeId } }),
}));

vi.mock("@/lib/api/domains/office-extended-api", () => ({
  resolveTaskIdentifier: (...args: unknown[]) => mocks.resolve(...args),
}));

describe("useTaskIdentifierRedirect", () => {
  beforeEach(() => {
    mocks.replace.mockReset();
    mocks.resolve.mockReset();
    mocks.activeId = "ws-active";
    mocks.search = "";
  });
  afterEach(cleanup);

  it("replaces the URL with the resolved task id", async () => {
    mocks.resolve.mockResolvedValue({ id: "task-uuid" });
    const { result } = renderHook(() => useTaskIdentifierRedirect("KAN-11"));

    expect(result.current).toBe("resolving");
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/office/tasks/task-uuid"));
    expect(mocks.resolve).toHaveBeenCalledWith("ws-active", "KAN-11");
  });

  it("prefers the workspace named in the link over the active one", async () => {
    mocks.search = "workspace=ws-link";
    mocks.resolve.mockResolvedValue({ id: "task-uuid" });
    renderHook(() => useTaskIdentifierRedirect("KAN-11"));

    await waitFor(() => expect(mocks.resolve).toHaveBeenCalledWith("ws-link", "KAN-11"));
  });

  it("reports failure when the lookup fails", async () => {
    mocks.resolve.mockRejectedValue(new Error("404"));
    const { result } = renderHook(() => useTaskIdentifierRedirect("KAN-99"));

    await waitFor(() => expect(result.current).toBe("failed"));
    expect(mocks.replace).not.toHaveBeenCalled();
  });

  it("waits for a workspace before resolving", () => {
    mocks.activeId = null;
    const { result } = renderHook(() => useTaskIdentifierRedirect("KAN-11"));

    expect(result.current).toBe("resolving");
    expect(mocks.resolve).not.toHaveBeenCalled();
  });

  it("ignores a response that arrives after unmount", async () => {
    let finish: (v: { id: string }) => void = () => {};
    mocks.resolve.mockReturnValue(new Promise((r) => (finish = r)));
    const { unmount } = renderHook(() => useTaskIdentifierRedirect("KAN-11"));

    unmount();
    await act(async () => finish({ id: "task-uuid" }));
    expect(mocks.replace).not.toHaveBeenCalled();
  });
});

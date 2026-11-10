import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { TaskIdentifierRedirect, useTaskIdentifierRedirect } from "./task-identifier-redirect";

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  resolve: vi.fn(),
  activeId: "ws-active" as string | null,
  search: "",
  push: vi.fn(),
  router: null as null | { replace: typeof vi.fn; push: typeof vi.fn },
}));

mocks.router = { replace: mocks.replace, push: mocks.push };

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => mocks.router,
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
    mocks.push.mockReset();
    mocks.resolve.mockReset();
    mocks.activeId = "ws-active";
    mocks.search = "";
  });
  afterEach(cleanup);

  it("replaces the URL with the resolved task id", async () => {
    mocks.resolve.mockResolvedValue({ id: "task-uuid" });
    const { result } = renderHook(() => useTaskIdentifierRedirect("KAN-11"));

    expect(result.current.status).toBe("resolving");
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/office/tasks/task-uuid"));
    expect(mocks.resolve).toHaveBeenCalledWith("ws-active", "KAN-11");
  });

  it("prefers the workspace named in the link over the active one", async () => {
    mocks.search = "workspaceId=ws-link";
    mocks.resolve.mockResolvedValue({ id: "task-uuid" });
    renderHook(() => useTaskIdentifierRedirect("KAN-11"));

    await waitFor(() => expect(mocks.resolve).toHaveBeenCalledWith("ws-link", "KAN-11"));
    expect(mocks.replace).toHaveBeenCalledWith("/office/tasks/task-uuid?workspaceId=ws-link");
  });

  it("upgrades the legacy workspace parameter and preserves other query values", async () => {
    mocks.search = "workspace=ws-link&tab=comments";
    mocks.resolve.mockResolvedValue({ id: "task-uuid" });
    renderHook(() => useTaskIdentifierRedirect("KAN-11"));

    await waitFor(() => expect(mocks.resolve).toHaveBeenCalledWith("ws-link", "KAN-11"));
    expect(mocks.replace).toHaveBeenCalledWith(
      "/office/tasks/task-uuid?tab=comments&workspaceId=ws-link",
    );
  });

  it("reports not found only when the lookup returns 404", async () => {
    mocks.resolve.mockRejectedValue(new ApiError("Not found", 404, null));
    const { result } = renderHook(() => useTaskIdentifierRedirect("KAN-99"));

    await waitFor(() => expect(result.current.status).toBe("not-found"));
    expect(mocks.replace).not.toHaveBeenCalled();
  });

  it("shows a retryable error for other failures", async () => {
    mocks.resolve
      .mockRejectedValueOnce(new ApiError("Server error", 500, null))
      .mockResolvedValueOnce({ id: "task-uuid" });
    const { result } = renderHook(() => useTaskIdentifierRedirect("KAN-11"));

    await waitFor(() => expect(result.current.status).toBe("error"));
    await act(async () => result.current.retry());

    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/office/tasks/task-uuid"));
    expect(mocks.resolve).toHaveBeenCalledTimes(2);
  });

  it("renders a touch-sized retry action for transient failures", async () => {
    mocks.resolve
      .mockRejectedValueOnce(new ApiError("Server error", 500, null))
      .mockResolvedValueOnce({ id: "task-uuid" });
    render(<TaskIdentifierRedirect identifier="KAN-11" />);

    await waitFor(() => expect(screen.getAllByRole("button")).toHaveLength(2));
    const retry = screen.getAllByRole("button")[0];
    expect(retry.className).toContain("[@media(pointer:coarse)]:min-h-11");
    fireEvent.click(retry);

    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/office/tasks/task-uuid"));
  });

  it("reports when no workspace is available", async () => {
    mocks.activeId = null;
    const { result } = renderHook(() => useTaskIdentifierRedirect("KAN-11"));

    await waitFor(() => expect(result.current.status).toBe("workspace-unavailable"));
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

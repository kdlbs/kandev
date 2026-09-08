import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const getDefaultCeilingMock = vi.fn<(workspaceId: string) => Promise<{ limit_subcents: number }>>();
const setDefaultCeilingMock =
  vi.fn<(workspaceId: string, limitSubcents: number) => Promise<{ limit_subcents: number }>>();

vi.mock("@/lib/api/domains/office-api", () => ({
  getDefaultCeiling: (workspaceId: string) => getDefaultCeilingMock(workspaceId),
  setDefaultCeiling: (workspaceId: string, limitSubcents: number) =>
    setDefaultCeilingMock(workspaceId, limitSubcents),
}));

const toastError = vi.fn();
const toastSuccess = vi.fn();
vi.mock("@/lib/toast/sonner", () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

import { DefaultCeilingCard } from "./default-ceiling-card";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

// deferred returns a promise plus its resolver, so a test can control exactly
// when a fetch "arrives" relative to a workspace switch.
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("DefaultCeilingCard", () => {
  it("ignores a stale workspace's response that resolves after switching workspaces", async () => {
    const wsA = deferred<{ limit_subcents: number }>();
    const wsB = deferred<{ limit_subcents: number }>();
    getDefaultCeilingMock.mockImplementationOnce(() => wsA.promise);
    getDefaultCeilingMock.mockImplementationOnce(() => wsB.promise);

    const { rerender } = render(<DefaultCeilingCard workspaceId="ws-a" />);
    await waitFor(() => expect(getDefaultCeilingMock).toHaveBeenCalledWith("ws-a"));

    rerender(<DefaultCeilingCard workspaceId="ws-b" />);
    await waitFor(() => expect(getDefaultCeilingMock).toHaveBeenCalledWith("ws-b"));

    // ws-b's response arrives first, then ws-a's stale response arrives late.
    await act(async () => {
      wsB.resolve({ limit_subcents: 200000 });
      await wsB.promise;
    });
    await screen.findByText("$20.00");

    await act(async () => {
      wsA.resolve({ limit_subcents: 500000 });
      await wsA.promise;
    });

    expect(screen.getByText("$20.00")).toBeTruthy();
    expect(screen.queryByText("$50.00")).toBeNull();
  });

  it("resets in-progress edits when the workspace changes", async () => {
    getDefaultCeilingMock.mockResolvedValue({ limit_subcents: 100000 });

    const { rerender } = render(<DefaultCeilingCard workspaceId="ws-a" />);
    await screen.findByText("$10.00");

    fireEvent.click(screen.getByRole("button", { name: "Edit default ceiling" }));
    expect(screen.getByRole("spinbutton")).toBeTruthy();

    rerender(<DefaultCeilingCard workspaceId="ws-b" />);

    expect(screen.queryByRole("spinbutton")).toBeNull();
  });

  it("rejects a non-positive draft ceiling without calling the API", async () => {
    getDefaultCeilingMock.mockResolvedValue({ limit_subcents: 100000 });
    render(<DefaultCeilingCard workspaceId="ws-a" />);
    await screen.findByText("$10.00");

    fireEvent.click(screen.getByRole("button", { name: "Edit default ceiling" }));
    const input = screen.getByRole("spinbutton");
    fireEvent.change(input, { target: { value: "0" } });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
    });

    expect(setDefaultCeilingMock).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith("Default ceiling must be greater than zero");
  });
});

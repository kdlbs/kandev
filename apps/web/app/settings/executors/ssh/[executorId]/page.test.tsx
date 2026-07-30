import { act, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { fetchExecutor } from "@/lib/api/domains/settings-api";
import SSHExecutorPage from "./page";

vi.mock("@/components/settings/ssh-connection-card", () => ({
  SSHConnectionCard: () => null,
}));

vi.mock("@/components/settings/ssh-sessions-card", () => ({
  SSHSessionsCard: () => null,
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchExecutor: vi.fn(),
  listExecutors: vi.fn(),
  updateExecutor: vi.fn(),
}));

vi.mock("@/lib/api/domains/ssh-api", () => ({
  listSSHSessions: vi.fn().mockResolvedValue([]),
}));

type LoadedExecutor = Awaited<ReturnType<typeof fetchExecutor>>;

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function executor(id: string, name: string): LoadedExecutor {
  return { id, name, type: "ssh", config: {} };
}

describe("SSHExecutorPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("keeps the current executor when the previous route request resolves last", async () => {
    const first = deferred<LoadedExecutor>();
    const second = deferred<LoadedExecutor>();
    vi.mocked(fetchExecutor).mockImplementation((executorId) => {
      if (executorId === "executor-a") return first.promise;
      if (executorId === "executor-b") return second.promise;
      throw new Error(`Unexpected executor ${executorId}`);
    });

    const { rerender } = render(
      <StateProvider>
        <SSHExecutorPage executorId="executor-a" />
      </StateProvider>,
    );
    await waitFor(() => expect(fetchExecutor).toHaveBeenCalledWith("executor-a"));

    rerender(
      <StateProvider>
        <SSHExecutorPage executorId="executor-b" />
      </StateProvider>,
    );
    await waitFor(() => expect(fetchExecutor).toHaveBeenCalledWith("executor-b"));
    expect(screen.getByText("Loading executor...")).toBeTruthy();

    second.resolve(executor("executor-b", "Executor B"));
    expect(await screen.findByRole("heading", { name: "Executor B" })).toBeTruthy();
    expect(screen.queryByText("Loading executor...")).toBeNull();

    await act(async () => {
      first.resolve(executor("executor-a", "Executor A"));
    });
    expect(screen.getByRole("heading", { name: "Executor B" })).toBeTruthy();
  });
});

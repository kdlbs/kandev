import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchSystemInfo: vi.fn(),
  requestRestart: vi.fn(),
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  fetchSystemInfo: mocks.fetchSystemInfo,
  requestRestart: mocks.requestRestart,
}));

import { useKandevRestart } from "./use-kandev-restart";

beforeEach(() => {
  mocks.fetchSystemInfo.mockReset();
  mocks.requestRestart.mockReset();
});

describe("useKandevRestart", () => {
  it("starts a restart and waits for a new boot id", async () => {
    mocks.fetchSystemInfo
      .mockResolvedValueOnce({ boot_id: "boot-1" })
      .mockResolvedValue({ boot_id: "boot-2" });
    mocks.requestRestart.mockResolvedValue({ accepted: true, message: "Restarting" });
    const onComplete = vi.fn();

    const { result } = renderHook(() => useKandevRestart({ onComplete }));

    await act(async () => {
      await result.current.start();
    });

    expect(mocks.requestRestart).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(result.current.phase).toBe("done"));
    expect(onComplete).toHaveBeenCalledTimes(1);
  });

  it("keeps waiting while the backend is unreachable", async () => {
    mocks.fetchSystemInfo
      .mockResolvedValueOnce({ boot_id: "boot-1" })
      .mockRejectedValue(new Error("connection refused"));
    mocks.requestRestart.mockResolvedValue({ accepted: true, message: "Restarting" });

    const { result } = renderHook(() => useKandevRestart());

    await act(async () => {
      await result.current.start();
    });

    await waitFor(() => expect(result.current.phase).toBe("restarting"));
    expect(result.current.isRestarting).toBe(true);
  });

  it("reports an error when the restart request fails", async () => {
    mocks.fetchSystemInfo.mockResolvedValueOnce({ boot_id: "boot-1" });
    mocks.requestRestart.mockRejectedValue(new Error("unsupported launch mode"));

    const { result } = renderHook(() => useKandevRestart());

    await act(async () => {
      await result.current.start();
    });

    expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(1);
    expect(mocks.fetchSystemInfo.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.requestRestart.mock.invocationCallOrder[0],
    );
    expect(result.current.phase).toBe("error");
    expect(result.current.errorMessage).toBe("unsupported launch mode");
  });

  it("ignores duplicate starts while a restart is already active", async () => {
    mocks.fetchSystemInfo.mockResolvedValueOnce({ boot_id: "boot-1" });
    let releaseRestart!: () => void;
    const restartStarted = new Promise<void>((resolve) => {
      mocks.requestRestart.mockImplementationOnce(
        () =>
          new Promise((release) => {
            releaseRestart = () => release({ accepted: true, message: "Restarting" });
            resolve();
          }),
      );
    });

    const { result } = renderHook(() => useKandevRestart());

    let firstStart!: Promise<void>;
    await act(async () => {
      firstStart = result.current.start();
      await restartStarted;
    });

    await act(async () => {
      await result.current.start();
    });
    expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(1);
    expect(mocks.requestRestart).toHaveBeenCalledTimes(1);

    releaseRestart();
    await act(async () => {
      await firstStart;
    });

    await waitFor(() => expect(result.current.phase).toBe("restarting"));
    expect(mocks.requestRestart).toHaveBeenCalledTimes(1);
  });
});

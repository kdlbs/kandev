import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  PortForwardingVisibilityProvider,
  useOptionalPortForwardingVisibility,
  usePortForwardingVisibility,
} from "./port-forwarding-visibility-provider";

const { updateTaskPortForwardingMock, toastMock } = vi.hoisted(() => ({
  updateTaskPortForwardingMock: vi.fn(),
  toastMock: vi.fn(),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  updateTaskPortForwarding: updateTaskPortForwardingMock,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function wrapper({ children }: PropsWithChildren) {
  return (
    <PortForwardingVisibilityProvider
      taskId="task-1"
      metadata={{ port_forwarding_enabled: true }}
      sessionId="session-1"
      isAgentctlReady
    >
      {children}
    </PortForwardingVisibilityProvider>
  );
}

beforeEach(() => {
  updateTaskPortForwardingMock.mockReset();
  toastMock.mockReset();
});

describe("PortForwardingVisibilityProvider", () => {
  it("rolls back a failed preference write and reports translated feedback", async () => {
    updateTaskPortForwardingMock.mockRejectedValueOnce(new Error("network"));
    const { result } = renderHook(() => usePortForwardingVisibility(), { wrapper });

    await act(async () => {
      await result.current.togglePortForwarding();
    });

    expect(updateTaskPortForwardingMock).toHaveBeenCalledWith("task-1", false);
    expect(result.current.enabled).toBe(true);
    expect(toastMock).toHaveBeenCalledWith({
      variant: "error",
      description: "task:portForwardingPreferenceUpdateFailed",
    });
  });

  it("opens the existing dialog after a successful enable", async () => {
    updateTaskPortForwardingMock.mockResolvedValueOnce({
      metadata: { port_forwarding_enabled: true },
    });
    const { result } = renderHook(() => usePortForwardingVisibility(), {
      wrapper: ({ children }) => (
        <PortForwardingVisibilityProvider
          taskId="task-1"
          metadata={{ port_forwarding_enabled: false }}
          sessionId="session-1"
          isAgentctlReady
        >
          {children}
        </PortForwardingVisibilityProvider>
      ),
    });

    await act(async () => {
      await result.current.togglePortForwarding({ openDialogOnEnable: true });
    });

    expect(updateTaskPortForwardingMock).toHaveBeenCalledWith("task-1", true);
    expect(result.current.enabled).toBe(true);
    expect(result.current.dialogOpen).toBe(true);
  });

  it("discards an in-flight toggle result when the task switches", async () => {
    let resolveToggle!: () => void;
    updateTaskPortForwardingMock.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        resolveToggle = resolve;
      }),
    );
    let providerProps = {
      taskId: "task-1",
      metadata: { port_forwarding_enabled: false } as Record<string, unknown>,
    };
    const dynamicWrapper = ({ children }: PropsWithChildren) => (
      <PortForwardingVisibilityProvider
        taskId={providerProps.taskId}
        metadata={providerProps.metadata}
        sessionId="session-1"
        isAgentctlReady
      >
        {children}
      </PortForwardingVisibilityProvider>
    );
    const { result, rerender } = renderHook(() => usePortForwardingVisibility(), {
      wrapper: dynamicWrapper,
    });

    act(() => {
      void result.current.togglePortForwarding({ openDialogOnEnable: true });
    });
    expect(updateTaskPortForwardingMock).toHaveBeenCalledWith("task-1", true);

    providerProps = { taskId: "task-2", metadata: {} };
    rerender();
    expect(result.current.enabled).toBe(false);
    expect(result.current.isUpdating).toBe(false);

    await act(async () => {
      resolveToggle();
    });

    expect(result.current.enabled).toBe(false);
    expect(result.current.dialogOpen).toBe(false);
    expect(result.current.isUpdating).toBe(false);
  });
});

describe("PortForwardingVisibilityProvider reconciliation", () => {
  it("keeps a successful toggle while task metadata is still stale", async () => {
    let resolveToggle!: () => void;
    updateTaskPortForwardingMock.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        resolveToggle = resolve;
      }),
    );
    const { result } = renderHook(() => usePortForwardingVisibility(), {
      wrapper: ({ children }) => (
        <PortForwardingVisibilityProvider
          taskId="task-1"
          metadata={{ port_forwarding_enabled: false }}
          sessionId="session-1"
          isAgentctlReady
        >
          {children}
        </PortForwardingVisibilityProvider>
      ),
    });

    act(() => {
      void result.current.togglePortForwarding();
    });
    expect(result.current.isUpdating).toBe(true);

    await act(async () => {
      resolveToggle();
    });

    expect(result.current.enabled).toBe(true);
    expect(result.current.isUpdating).toBe(false);
  });
});

describe("PortForwardingVisibility context boundaries", () => {
  // @covers AC-UI-PORT-FORWARDING-DISCOVERY-001.10
  it.each([false, true])(
    "keeps management open when hiding its shortcut (failure=%s)",
    async (fail) => {
      if (fail) updateTaskPortForwardingMock.mockRejectedValueOnce(new Error("network"));
      else updateTaskPortForwardingMock.mockResolvedValueOnce({});
      const { result } = renderHook(() => usePortForwardingVisibility(), { wrapper });
      act(() => result.current.setDialogOpen(true));

      await act(async () => {
        await result.current.togglePortForwarding({ preserveDialogOpen: true });
      });

      expect(result.current.dialogOpen).toBe(true);
      expect(result.current.enabled).toBe(fail);
    },
  );

  // @covers AC-UI-PORT-FORWARDING-DISCOVERY-001.13
  it.each(["session", "readiness", "archive"])(
    "does not reopen after a delayed enable and %s change",
    async (change) => {
      let resolveWrite!: () => void;
      updateTaskPortForwardingMock.mockReturnValueOnce(
        new Promise<void>((resolve) => {
          resolveWrite = resolve;
        }),
      );
      let sessionId = "session-1";
      let isAgentctlReady = true;
      let isArchived = false;
      const { result, rerender } = renderHook(() => usePortForwardingVisibility(), {
        wrapper: ({ children }) => (
          <PortForwardingVisibilityProvider
            taskId="task-1"
            metadata={{}}
            sessionId={sessionId}
            isAgentctlReady={isAgentctlReady}
            isArchived={isArchived}
          >
            {children}
          </PortForwardingVisibilityProvider>
        ),
      });
      act(() => {
        void result.current.togglePortForwarding({ openDialogOnEnable: true });
      });
      if (change === "session") sessionId = "session-2";
      if (change === "readiness") isAgentctlReady = false;
      if (change === "archive") isArchived = true;
      rerender();
      await act(async () => resolveWrite());
      expect(result.current.dialogOpen).toBe(false);
      expect(result.current.isUpdating).toBe(false);
    },
  );

  it("closes management on a ready session replacement", () => {
    let sessionId = "session-1";
    const { result, rerender } = renderHook(() => usePortForwardingVisibility(), {
      wrapper: ({ children }) => (
        <PortForwardingVisibilityProvider
          taskId="task-1"
          metadata={{}}
          sessionId={sessionId}
          isAgentctlReady
        >
          {children}
        </PortForwardingVisibilityProvider>
      ),
    });
    act(() => result.current.setDialogOpen(true));
    sessionId = "session-2";
    rerender();
    expect(result.current.dialogOpen).toBe(false);
  });

  it("returns no visibility state outside a task provider", () => {
    const { result } = renderHook(() => useOptionalPortForwardingVisibility());

    expect(result.current).toBeUndefined();
  });

  it("closes the dialog when agentctl readiness is lost", () => {
    let isAgentctlReady = true;
    const dynamicWrapper = ({ children }: PropsWithChildren) => (
      <PortForwardingVisibilityProvider
        taskId="task-1"
        metadata={{ port_forwarding_enabled: true }}
        sessionId="session-1"
        isAgentctlReady={isAgentctlReady}
      >
        {children}
      </PortForwardingVisibilityProvider>
    );
    const { result, rerender } = renderHook(() => usePortForwardingVisibility(), {
      wrapper: dynamicWrapper,
    });

    act(() => result.current.setDialogOpen(true));
    expect(result.current.dialogOpen).toBe(true);

    isAgentctlReady = false;
    rerender();

    expect(result.current.dialogOpen).toBe(false);
  });
});

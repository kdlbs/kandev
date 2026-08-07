import { type ButtonHTMLAttributes, type PropsWithChildren } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PortForwardButton } from "./port-forward-dialog";

type Tunnel = { port: number; tunnel_port: number };

const { listPortsMock, listTunnelsMock, visibilityMock, dockviewMock } = vi.hoisted(() => ({
  listPortsMock: vi.fn(),
  listTunnelsMock: vi.fn(),
  visibilityMock: {
    enabled: true,
    canToggle: true,
    dialogOpen: false,
    setDialogOpen: vi.fn(),
  },
  dockviewMock: {
    api: {} as object | null,
    openBrowserPanel: vi.fn(),
  },
}));

vi.mock("@/lib/api/domains/port-api", () => ({
  listPorts: listPortsMock,
  listTunnels: listTunnelsMock,
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: typeof dockviewMock) => unknown) => selector(dockviewMock),
}));

vi.mock("./port-forwarding-visibility-provider", () => ({
  usePortForwardingVisibility: () => visibilityMock,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({
    variant,
    children,
    ...props
  }: PropsWithChildren<ButtonHTMLAttributes<HTMLButtonElement> & { variant?: string }>) => (
    <button data-variant={variant} {...props}>
      {children}
    </button>
  ),
}));

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children }: PropsWithChildren) => <>{children}</>,
  DialogContent: ({ children, ...props }: PropsWithChildren<Record<string, unknown>>) => (
    <div {...props}>{children}</div>
  ),
  DialogHeader: () => null,
  DialogTitle: () => null,
  DialogTrigger: ({ children }: PropsWithChildren) => <>{children}</>,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: PropsWithChildren) => <>{children}</>,
  TooltipContent: () => null,
  TooltipTrigger: ({ children }: PropsWithChildren) => <>{children}</>,
}));

vi.mock("./use-tunnel-actions", () => ({
  useTunnelActions: () => ({
    pendingTunnels: new Set<number>(),
    handleTunnelStart: vi.fn(),
    handleTunnelStop: vi.fn(),
  }),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return { promise, resolve };
}

describe("PortForwardButton", () => {
  beforeEach(() => {
    listPortsMock.mockReset();
    listTunnelsMock.mockReset();
    listPortsMock.mockResolvedValue([]);
    listTunnelsMock.mockResolvedValue([]);
    visibilityMock.enabled = true;
    visibilityMock.canToggle = true;
    visibilityMock.dialogOpen = false;
    visibilityMock.setDialogOpen.mockReset();
    dockviewMock.api = {};
    dockviewMock.openBrowserPanel.mockReset();
  });

  afterEach(() => cleanup());

  it("ignores tunnel results from a previous session", async () => {
    const firstSession = deferred<Tunnel[]>();
    const secondSession = deferred<Tunnel[]>();
    listTunnelsMock.mockImplementation((sessionId: string) =>
      sessionId === "session-1" ? firstSession.promise : secondSession.promise,
    );

    const { rerender } = render(<PortForwardButton sessionId="session-1" />);
    rerender(<PortForwardButton sessionId="session-2" />);

    await act(async () => {
      secondSession.resolve([]);
      await secondSession.promise;
    });
    expect(screen.getByTestId("port-forward-button").getAttribute("data-variant")).toBe("outline");

    await act(async () => {
      firstSession.resolve([{ port: 3000, tunnel_port: 4000 }]);
      await firstSession.promise;
    });
    expect(screen.getByTestId("port-forward-button").getAttribute("data-variant")).toBe("outline");
  });

  it("opens a proxy URL in the Browser panel and closes the dialog", () => {
    render(<PortForwardButton sessionId="session-1" />);

    fireEvent.change(screen.getByTestId("port-forward-port-input"), {
      target: { value: "3000" },
    });
    fireEvent.click(screen.getByTestId("port-forward-add-button"));
    fireEvent.click(screen.getByTestId("port-forward-open-browser-3000"));

    expect(dockviewMock.openBrowserPanel).toHaveBeenCalledWith(
      expect.stringContaining("/port-proxy/session-1/3000/"),
    );
    expect(visibilityMock.setDialogOpen).toHaveBeenCalledWith(false);
  });

  it("reloads tunnels when the control becomes visible", async () => {
    visibilityMock.enabled = false;
    listTunnelsMock.mockResolvedValue([{ port: 3000, tunnel_port: 4000 }]);

    const { rerender } = render(<PortForwardButton sessionId="session-1" />);
    expect(listTunnelsMock).not.toHaveBeenCalled();

    visibilityMock.enabled = true;
    rerender(<PortForwardButton sessionId="session-1" />);

    await waitFor(() => {
      expect(listTunnelsMock).toHaveBeenCalledWith("session-1");
      expect(screen.getByTestId("port-forward-button").getAttribute("data-variant")).toBe(
        "default",
      );
    });
  });
});

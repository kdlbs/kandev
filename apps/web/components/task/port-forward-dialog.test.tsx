import { type ButtonHTMLAttributes, type PropsWithChildren } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PortForwardButton } from "./port-forward-dialog";

const SESSION_ONE = "session-1";
const SESSION_TWO = "session-2";
const VARIANT_ATTRIBUTE = "data-variant";
const BUTTON_ID = "port-forward-button";
const ROW_9000 = "port-forward-row-9000";
const ROW_3000 = "port-forward-row-3000";

type Tunnel = { port: number; tunnel_port: number };

const {
  listPortsMock,
  listTunnelsMock,
  startTunnelMock,
  stopTunnelMock,
  toastMock,
  visibilityMock,
  dockviewMock,
} = vi.hoisted(() => ({
  listPortsMock: vi.fn(),
  listTunnelsMock: vi.fn(),
  startTunnelMock: vi.fn(),
  stopTunnelMock: vi.fn(),
  toastMock: { success: vi.fn(), error: vi.fn() },
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
  startTunnel: startTunnelMock,
  stopTunnel: stopTunnelMock,
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

vi.mock("@/lib/toast/sonner", () => ({ toast: toastMock }));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://localhost:3001" }),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return { promise, resolve };
}

beforeEach(() => {
  listPortsMock.mockReset();
  listTunnelsMock.mockReset();
  startTunnelMock.mockReset();
  stopTunnelMock.mockReset();
  startTunnelMock.mockResolvedValue(49152);
  stopTunnelMock.mockResolvedValue(undefined);
  toastMock.error.mockReset();
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

describe("PortForwardButton", () => {
  it("ignores tunnel results from a previous session", async () => {
    const firstSession = deferred<Tunnel[]>();
    const secondSession = deferred<Tunnel[]>();
    listTunnelsMock.mockImplementation((sessionId: string) =>
      sessionId === SESSION_ONE ? firstSession.promise : secondSession.promise,
    );

    const { rerender } = render(<PortForwardButton sessionId={SESSION_ONE} />);
    rerender(<PortForwardButton sessionId={SESSION_TWO} />);

    await act(async () => {
      secondSession.resolve([]);
      await secondSession.promise;
    });
    expect(screen.getByTestId(BUTTON_ID).getAttribute(VARIANT_ATTRIBUTE)).toBe("outline");

    await act(async () => {
      firstSession.resolve([{ port: 3000, tunnel_port: 4000 }]);
      await firstSession.promise;
    });
    expect(screen.getByTestId(BUTTON_ID).getAttribute(VARIANT_ATTRIBUTE)).toBe("outline");
  });

  it("opens a proxy URL in the Browser panel and closes the dialog", () => {
    render(<PortForwardButton sessionId={SESSION_ONE} />);

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

    const { rerender } = render(<PortForwardButton sessionId={SESSION_ONE} />);
    expect(listTunnelsMock).not.toHaveBeenCalled();

    visibilityMock.enabled = true;
    rerender(<PortForwardButton sessionId={SESSION_ONE} />);

    await waitFor(() => {
      expect(listTunnelsMock).toHaveBeenCalledWith(SESSION_ONE);
      expect(screen.getByTestId(BUTTON_ID).getAttribute(VARIANT_ATTRIBUTE)).toBe("default");
    });
  });
});

function portOrder() {
  return screen.queryAllByTestId(/^port-forward-row-/).map((row) => row.dataset.testid);
}

function addPort(port: number) {
  fireEvent.change(screen.getByTestId("port-forward-port-input"), {
    target: { value: String(port) },
  });
  fireEvent.click(screen.getByTestId("port-forward-add-button"));
}

async function startPort(port: number) {
  const row = screen.getByTestId(`port-forward-row-${port}`);
  fireEvent.click(within(row).getByRole("button", { name: "task:startTunnel" }));
  fireEvent.click(within(row).getByRole("button", { name: "task:start2" }));
}

// @covers AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.1-.5, .7-.8
describe("active-first port state", () => {
  it("preserves untouched hydrated tunnels alongside a newer successful start", async () => {
    const tunnels = deferred<Tunnel[]>();
    listTunnelsMock.mockReturnValue(tunnels.promise);
    render(<PortForwardButton sessionId={SESSION_ONE} />);
    addPort(9500);
    await startPort(9500);
    await waitFor(() =>
      expect(screen.getByTestId(BUTTON_ID).getAttribute(VARIANT_ATTRIBUTE)).toBe("default"),
    );
    await act(async () => {
      tunnels.resolve([{ port: 9000, tunnel_port: 49152 }]);
    });
    expect(portOrder()).toEqual([ROW_9000, "port-forward-row-9500"]);
  });

  it("shows tunnel-only rows when tunnels arrive after detection, ahead of lower ports", async () => {
    const tunnels = deferred<Tunnel[]>();
    listTunnelsMock.mockReturnValue(tunnels.promise);
    listPortsMock.mockResolvedValue([{ port: 3000, address: "*" }]);
    render(<PortForwardButton sessionId={SESSION_ONE} />);
    fireEvent.click(screen.getByTestId("port-forward-refresh"));
    await waitFor(() => expect(screen.getByTestId(ROW_3000)).toBeTruthy());
    await act(async () => {
      tunnels.resolve([{ port: 9000, tunnel_port: 49152 }]);
    });
    expect(portOrder()).toEqual([ROW_9000, ROW_3000]);
    const links = within(screen.getByTestId(ROW_9000)).getAllByRole("link");
    expect(links[0].getAttribute("href")).toBe("http://localhost:49152/");
  });
});

// @covers AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.4-.5, .7-.8
describe("active-first port mutations", () => {
  it("promotes only successful starts and keeps failed or pending stops active with row focus", async () => {
    render(<PortForwardButton sessionId={SESSION_ONE} />);
    addPort(3000);
    addPort(9000);
    const start = deferred<number>();
    startTunnelMock.mockReturnValueOnce(start.promise);
    const originalRow = screen.getByTestId(ROW_9000);
    await startPort(9000);
    expect(portOrder()).toEqual([ROW_3000, ROW_9000]);
    await act(async () => {
      start.resolve(49152);
    });
    expect(portOrder()).toEqual([ROW_9000, ROW_3000]);
    expect(screen.getByTestId(ROW_9000)).toBe(originalRow);
    const stop = within(originalRow).getByRole("button", { name: "task:stopTunnel" });
    stop.focus();
    stopTunnelMock.mockRejectedValueOnce(new Error("stop failed"));
    fireEvent.click(stop);
    await waitFor(() => expect(toastMock.error).toHaveBeenCalled());
    expect(portOrder()[0]).toBe(ROW_9000);
    const stopped = deferred<void>();
    stopTunnelMock.mockReturnValueOnce(stopped.promise);
    fireEvent.click(stop);
    expect(portOrder()[0]).toBe(ROW_9000);
    await act(async () => {
      stopped.resolve();
    });
    expect(portOrder()).toEqual([ROW_3000, ROW_9000]);
    expect(document.activeElement).toBe(stop);
    expect(screen.queryByText("task:forwardedPorts")).toBeNull();
  });

  it("retains a stopped tunnel-only target for restarting", async () => {
    listTunnelsMock.mockResolvedValue([{ port: 9000, tunnel_port: 49152 }]);
    render(<PortForwardButton sessionId={SESSION_ONE} />);
    const stop = await screen.findByRole("button", { name: "task:stopTunnel" });
    fireEvent.click(stop);
    await waitFor(() =>
      expect(
        within(screen.getByTestId(ROW_9000)).getByRole("button", {
          name: "task:startTunnel",
        }),
      ).toBeTruthy(),
    );
  });

  it("keeps failed starts inactive and cannot overwrite successful starts with old hydration", async () => {
    const tunnels = deferred<Tunnel[]>();
    listTunnelsMock.mockReturnValue(tunnels.promise);
    render(<PortForwardButton sessionId={SESSION_ONE} />);
    addPort(9000);
    startTunnelMock.mockRejectedValueOnce(new Error("start failed"));
    await startPort(9000);
    await waitFor(() => expect(toastMock.error).toHaveBeenCalled());
    expect(screen.getByTestId(BUTTON_ID).getAttribute(VARIANT_ATTRIBUTE)).toBe("outline");
    await startPort(9000);
    await waitFor(() =>
      expect(screen.getByTestId(BUTTON_ID).getAttribute(VARIANT_ATTRIBUTE)).toBe("default"),
    );
    await act(async () => {
      tunnels.resolve([]);
    });
    expect(screen.getByTestId(BUTTON_ID).getAttribute(VARIANT_ATTRIBUTE)).toBe("default");
  });

  it("isolates manual rows, delayed refresh, and delayed start when the session changes", async () => {
    const ports = deferred<Array<{ port: number; address: string }>>();
    const start = deferred<number>();
    listPortsMock.mockReturnValueOnce(ports.promise);
    startTunnelMock.mockReturnValueOnce(start.promise);
    const { rerender } = render(<PortForwardButton sessionId={SESSION_ONE} />);
    addPort(9000);
    fireEvent.click(screen.getByTestId("port-forward-refresh"));
    await startPort(9000);
    rerender(<PortForwardButton sessionId={SESSION_TWO} />);
    await act(async () => {
      ports.resolve([{ port: 3000, address: "*" }]);
      start.resolve(49152);
    });
    expect(portOrder()).toEqual([]);
    expect(screen.getByTestId(BUTTON_ID).getAttribute(VARIANT_ATTRIBUTE)).toBe("outline");
  });
});

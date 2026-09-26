import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { MobileTerminalsPicker } from "./mobile-terminals-section";

const mockDestroyTerminal = vi.fn();
const mockAddTerminal = vi.fn();
const mockRemoveTerminal = vi.fn();
const mockReleaseAutoCreatedEnvironment = vi.fn();
const mockStopUserShell = vi.fn();
const CLOSE_TERMINAL_BUTTON_NAME = "Close Terminal 1";
const CLOSE_CONFIRMATION_NAME = "Close terminal?";

const terminal = {
  id: "shell-1",
  type: "shell" as const,
  label: "Terminal 1",
  closable: true,
  kind: "ordinary" as const,
};

const mobileContext = {
  terminals: [terminal],
  terminalTabValue: terminal.id,
  addTerminal: mockAddTerminal,
  removeTerminal: mockRemoveTerminal,
  destroyTerminal: mockDestroyTerminal,
  environmentId: "env-1",
};

vi.mock("./mobile-terminals-context", () => ({
  useMobileTerminalsContext: () => mobileContext,
}));

vi.mock("./mobile-pill-button", () => ({
  MobilePillButton: ({ onClick }: { onClick: () => void }) => (
    <button type="button" onClick={onClick}>
      open terminals
    </button>
  ),
}));

vi.mock("./mobile-picker-sheet", () => ({
  MobilePickerSheet: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div role="dialog">{children}</div> : null,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      tasks: { activeTaskId: "task-1" },
      setRightPanelActiveTab: vi.fn(),
    }),
}));

vi.mock("@/hooks/domains/session/use-user-shells", () => ({
  useUserShells: () => ({ shells: [] }),
}));

vi.mock("@/hooks/domains/session/use-mobile-terminals", () => ({
  releaseAutoCreatedEnvironment: (...args: unknown[]) => mockReleaseAutoCreatedEnvironment(...args),
}));

vi.mock("@/lib/api/domains/user-shell-api", () => ({
  stopUserShell: (...args: unknown[]) => mockStopUserShell(...args),
}));

function renderOpenPicker() {
  render(<MobileTerminalsPicker sessionId="session-1" />);
  fireEvent.click(screen.getByRole("button", { name: "open terminals" }));
}

describe("mobile terminal close feedback", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mobileContext.terminals = [terminal];
    mobileContext.terminalTabValue = terminal.id;
    mockStopUserShell.mockResolvedValue(undefined);
  });

  afterEach(cleanup);

  it("keeps a 44px close action without rendering teardown progress", () => {
    mockDestroyTerminal.mockImplementation(() => new Promise<boolean>(() => {}));
    renderOpenPicker();

    const close = screen.getByRole("button", { name: CLOSE_TERMINAL_BUTTON_NAME });
    expect(close.classList.contains("h-11")).toBe(true);
    expect(close.classList.contains("w-11")).toBe(true);
    expect(close.getAttribute("aria-busy")).toBeNull();
    expect(screen.queryByRole("status", { name: "Closing terminal" })).toBeNull();

    fireEvent.click(close);
    expect(mockDestroyTerminal).not.toHaveBeenCalled();
    expect(screen.getByRole("group", { name: CLOSE_CONFIRMATION_NAME })).toBeTruthy();
  });

  it("always morphs the close action into inline cancel and confirm actions", async () => {
    mockDestroyTerminal.mockImplementation(() => new Promise<boolean>(() => {}));
    renderOpenPicker();

    fireEvent.click(screen.getByRole("button", { name: CLOSE_TERMINAL_BUTTON_NAME }));

    expect(mockDestroyTerminal).not.toHaveBeenCalled();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    const confirmation = screen.getByRole("group", { name: CLOSE_CONFIRMATION_NAME });
    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("group", { name: CLOSE_CONFIRMATION_NAME })).toBeNull();
    expect(screen.getByRole("button", { name: CLOSE_TERMINAL_BUTTON_NAME })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: CLOSE_TERMINAL_BUTTON_NAME }));
    fireEvent.click(
      within(screen.getByRole("group", { name: CLOSE_CONFIRMATION_NAME })).getByRole("button", {
        name: "Close terminal",
      }),
    );

    await waitFor(() => expect(mockDestroyTerminal).toHaveBeenCalledWith(terminal.id));
    expect(screen.queryByRole("group", { name: CLOSE_CONFIRMATION_NAME })).toBeNull();
  });

  it("keeps the final-terminal auto-create guard after failed shared teardown", async () => {
    mockDestroyTerminal.mockResolvedValue(false);
    renderOpenPicker();

    fireEvent.click(screen.getByRole("button", { name: CLOSE_TERMINAL_BUTTON_NAME }));
    fireEvent.click(
      within(screen.getByRole("group", { name: CLOSE_CONFIRMATION_NAME })).getByRole("button", {
        name: "Close terminal",
      }),
    );

    await waitFor(() => expect(mockDestroyTerminal).toHaveBeenCalledWith(terminal.id));
    expect(mockReleaseAutoCreatedEnvironment).not.toHaveBeenCalled();
    expect(mockAddTerminal).not.toHaveBeenCalled();
    expect(mockRemoveTerminal).not.toHaveBeenCalled();
    expect(mockStopUserShell).not.toHaveBeenCalled();
  });

  it("releases the final-terminal auto-create guard only after shared teardown succeeds", async () => {
    mockDestroyTerminal.mockResolvedValue(true);
    renderOpenPicker();

    fireEvent.click(screen.getByRole("button", { name: CLOSE_TERMINAL_BUTTON_NAME }));
    fireEvent.click(
      within(screen.getByRole("group", { name: CLOSE_CONFIRMATION_NAME })).getByRole("button", {
        name: "Close terminal",
      }),
    );

    await waitFor(() => expect(mockReleaseAutoCreatedEnvironment).toHaveBeenCalledWith("env-1"));
    expect(mockDestroyTerminal).toHaveBeenCalledWith(terminal.id);
    expect(mockAddTerminal).toHaveBeenCalledTimes(1);
    expect(mockReleaseAutoCreatedEnvironment.mock.invocationCallOrder[0]).toBeLessThan(
      mockAddTerminal.mock.invocationCallOrder[0],
    );
    expect(mockRemoveTerminal).not.toHaveBeenCalled();
    expect(mockStopUserShell).not.toHaveBeenCalled();
  });
});

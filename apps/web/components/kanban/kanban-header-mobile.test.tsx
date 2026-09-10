import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState, type ComponentProps, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { KanbanHeaderMobile } from "./kanban-header-mobile";
import { pluginRegistry } from "@/lib/plugins/registry";

vi.mock("@/components/page-topbar", () => ({
  PageTopbar: ({
    title,
    titleSlot,
    leading,
    actions,
    freeWidth,
  }: {
    title: string;
    titleSlot?: ReactNode;
    leading?: ReactNode;
    actions?: ReactNode;
    freeWidth?: "lead" | "actions";
  }) => (
    <header data-free-width={freeWidth}>
      {leading}
      {titleSlot ?? title}
      {actions}
    </header>
  ),
}));

vi.mock("./mobile-menu-sheet", () => ({
  MobileMenuSheet: ({ open, pageActions }: { open: boolean; pageActions?: ReactNode }) =>
    open ? <div role="dialog">{pageActions}</div> : null,
}));

const launchers = vi.hoisted(() => ({
  openQuickChat: vi.fn(),
  openQuickTerminal: vi.fn(),
}));
const status = vi.hoisted(() => ({
  issueSeverity: "none" as "none" | "unstable" | "lost",
}));
const chat = vi.hoisted(() => ({
  activity: null as "running" | "finished" | null,
  label: "Quick Chat",
}));

vi.mock("@/hooks/use-quick-chat-launcher", () => ({
  useQuickChatLauncher: () => launchers.openQuickChat,
}));
vi.mock("@/hooks/use-quick-terminal-launcher", () => ({
  useQuickTerminalLauncher: () => launchers.openQuickTerminal,
}));
vi.mock("@/components/app-status-bar/app-status-surface-provider", () => ({
  useAppStatusDrawer: () => status,
}));
vi.mock("@/components/quick-chat/use-quick-chat-activity", () => ({
  useQuickChatActivity: () => chat,
}));

const MENU = "mobile-topbar-menu";
const CHAT = "mobile-quick-chat-button";
const TERMINAL = "mobile-quick-terminal-button";
const CONTEXT = "mobile-topbar-page-context";
const SEARCH = "mobile-search-toggle";
const WORKSPACE = "workspace-1";

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame"] });
});
afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin("stateful-topbar-test");
  vi.useRealTimers();
  vi.clearAllMocks();
  status.issueSeverity = "none";
  chat.activity = null;
  chat.label = "Quick Chat";
});

function renderHeader(props: Partial<ComponentProps<typeof KanbanHeaderMobile>> = {}) {
  return render(
    <StateProvider>
      <KanbanHeaderMobile
        title="Localized page title"
        workspaceId={WORKSPACE}
        workspaceLabel="Harbor"
        {...props}
      />
    </StateProvider>,
  );
}

function openMenu() {
  fireEvent.click(screen.getByTestId(MENU));
}
function frame() {
  act(() => vi.advanceTimersToNextFrame());
}

describe("stateful phone menu plugins", () => {
  // Review-requested contract coverage: plugin slots own state, not just launch callbacks.
  it("keeps a plugin's local disclosure and input mounted during interaction", () => {
    function StatefulPlugin() {
      const [open, setOpen] = useState(false);
      const [value, setValue] = useState("");
      return (
        <>
          <button onClick={() => setOpen(true)}>Plugin details</button>
          {open && (
            <input
              aria-label="Plugin query"
              value={value}
              onChange={(event) => setValue(event.target.value)}
            />
          )}
        </>
      );
    }
    pluginRegistry
      .forPlugin("stateful-topbar-test")
      .registerComponent("main-top-bar", StatefulPlugin);
    renderHeader();
    openMenu();
    fireEvent.click(screen.getByRole("button", { name: "Plugin details" }));
    frame();
    const query = screen.getByRole("textbox", { name: "Plugin query" });
    fireEvent.click(query);
    fireEvent.change(query, { target: { value: "checkout" } });
    frame();
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect((screen.getByRole("textbox", { name: "Plugin query" }) as HTMLInputElement).value).toBe(
      "checkout",
    );
  });
});

describe("shared phone listing header", () => {
  // @covers AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.5 AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.7
  it.each([
    ["kanban", "Kanban"],
    ["tasks", "List"],
  ] as const)("uses route-derived context and opens the menu on %s", (currentPage, label) => {
    renderHeader({ currentPage });
    const context = screen.getByTestId(CONTEXT);
    expect(context.textContent).toContain("Harbor");
    expect(context.textContent).toContain(label);
    expect(screen.queryByTestId("mobile-topbar-brand")).toBeNull();
    expect(screen.queryByTestId("mobile-topbar-action-strip")).toBeNull();
    expect(context.closest("header")?.getAttribute("data-free-width")).toBe("lead");
    fireEvent.click(context);
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("keeps the Threads-supplied view control in the title slot", () => {
    renderHeader({ currentPage: "threads", taskListingControls: <button>Review view</button> });
    const view = screen.getByRole("button", { name: "Review view" });
    expect(view.closest("header")).not.toBeNull();
    expect(screen.queryByTestId(CONTEXT)).toBeNull();
    expect(screen.queryByTestId("mobile-topbar-action-strip")).toBeNull();
  });

  // @covers AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.2 AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.3
  it.each([
    [CHAT, "openQuickChat"],
    [TERMINAL, "openQuickTerminal"],
  ] as const)("closes the menu before launching %s", (testId, launcher) => {
    renderHeader();
    expect(screen.queryByTestId(testId)).toBeNull();
    openMenu();
    fireEvent.click(screen.getByTestId(testId));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(launchers[launcher]).not.toHaveBeenCalled();
    frame();
    expect(launchers[launcher]).toHaveBeenCalledTimes(1);
  });

  it("omits workspace launchers without an active workspace", () => {
    renderHeader({ workspaceId: undefined });
    openMenu();
    expect(screen.queryByTestId(CHAT)).toBeNull();
    expect(screen.queryByTestId(TERMINAL)).toBeNull();
  });

  // @covers AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.10
  it("reveals search from the menu and clears its query on collapse", () => {
    const onSearchChange = vi.fn();
    renderHeader({ currentPage: "tasks", searchQuery: "Alpha", onSearchChange });
    openMenu();
    expect(screen.getByTestId(SEARCH).getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(screen.getByTestId(SEARCH));
    expect(screen.queryByRole("dialog")).toBeNull();
    frame();
    expect(onSearchChange).not.toHaveBeenCalled();
    openMenu();
    expect(screen.getByTestId(SEARCH).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(screen.getByTestId(SEARCH));
    frame();
    expect(onSearchChange).toHaveBeenCalledWith("");
  });

  it("keeps connectivity and Quick Chat feedback on the persistent menu", () => {
    status.issueSeverity = "lost";
    chat.activity = "running";
    chat.label = "Quick Chat running";
    renderHeader();
    const menu = screen.getByTestId(MENU);
    expect(menu.getAttribute("aria-label")).toContain("Connection lost");
    expect(menu.getAttribute("aria-description")).toBe(chat.label);
    expect(menu.getAttribute("data-connection-severity")).toBe("lost");
    expect(
      within(menu).getByTestId("quick-chat-activity-indicator").getAttribute("data-state"),
    ).toBe("running");
  });
});

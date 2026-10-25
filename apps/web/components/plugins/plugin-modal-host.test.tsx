import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { pluginModalManager } from "@/lib/plugins/modal-manager";
import { PluginModalHost } from "./plugin-modal-host";

function cleanupModals(pluginId: string) {
  pluginModalManager.closeAllForPlugin(pluginId);
}

const LOCKED_MODAL_TITLE = "Locked modal";
const MODAL_OPENER_TEST_ID = "modal-opener";
const MODAL_FOCUS_TARGET_TEST_ID = "modal-focus-target";
const PARENT_MODAL_TITLE = "Parent modal";
const CHILD_MODAL_TITLE = "Child modal";

describe("PluginModalHost", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
  });

  it("renders nothing when no plugin has an open modal", () => {
    const { container } = render(<PluginModalHost />);
    expect(container.innerHTML).toBe("");
  });

  it("renders an open modal's title and content", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "My Modal",
      content: () => <div data-testid="modal-content">Hello</div>,
    });

    render(<PluginModalHost />);

    expect(screen.getByText("My Modal")).not.toBeNull();
    expect(screen.getByTestId("modal-content")).not.toBeNull();
  });

  it("renders a modal description inside the host-owned header", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "Link Bitbucket pull request",
      description: "Use a Bitbucket pull request URL for this task.",
      content: () => <div data-testid="modal-content">Hello</div>,
    } as never);

    render(<PluginModalHost />);

    expect(screen.getByText("Use a Bitbucket pull request URL for this task.")).not.toBeNull();
  });

  it("keeps dialog content in a local scroll body while retaining the close control", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "Growing modal",
      content: () => <div data-testid="modal-content">Long plugin content</div>,
    });

    render(<PluginModalHost />);

    const dialog = screen.getByTestId(/^plugin-modal-dialog-/);
    const body = screen.getByTestId(/^plugin-modal-body-/);
    expect(dialog.getAttribute("data-layout")).toBe("contained");
    expect(dialog.contains(body)).toBe(true);
    expect(screen.getByRole("button", { name: "Close" })).not.toBeNull();
  });
});

describe("PluginModalHost — other presentations", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
  });

  it("keeps titleless task-link content in the bounded scroll row", () => {
    pluginModalManager.openTaskLinkDialog("plugin-a", {
      content: () => <div data-testid="modal-content">Long task-link content</div>,
    });

    render(<PluginModalHost />);

    const dialog = screen.getByTestId(/^plugin-modal-dialog-/);
    const body = screen.getByTestId(/^plugin-modal-body-/);
    expect(dialog.getAttribute("data-layout")).toBe("contained");
    expect(body.className).toContain("row-start-2");
    expect(screen.getByRole("dialog", { name: "Plugin dialog" })).toBeTruthy();
  });

  it("does not add a close control to a nondismissible dialog", () => {
    pluginModalManager.openModal("plugin-a", {
      title: LOCKED_MODAL_TITLE,
      content: () => <div>Content</div>,
      dismissible: false,
    });

    render(<PluginModalHost />);

    expect(screen.getByTestId(/^plugin-modal-dialog-/)).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
  });

  it("keeps a nondismissible dialog open on Escape and outside interaction", () => {
    pluginModalManager.openModal("plugin-a", {
      title: LOCKED_MODAL_TITLE,
      content: () => <div>Content</div>,
      dismissible: false,
    });

    render(<PluginModalHost />);

    const dialog = screen.getByRole("dialog", { name: LOCKED_MODAL_TITLE });
    fireEvent.keyDown(dialog, { key: "Escape", code: "Escape" });
    fireEvent.pointerDown(screen.getByTestId(/^plugin-modal-dialog-/).parentElement!);

    expect(screen.getByRole("dialog", { name: LOCKED_MODAL_TITLE })).toBe(dialog);
  });

  it("renders a host-owned drawer when the plugin requests mobile presentation", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "Link pull request",
      content: () => <div data-testid="drawer-content">Mobile action</div>,
      presentation: "drawer",
    });

    render(<PluginModalHost />);

    expect(document.querySelector('[data-slot="drawer-content"]')).not.toBeNull();
    expect(screen.getByTestId("drawer-content")).not.toBeNull();
  });

  it("gives title-less plugin surfaces an accessible fallback name", () => {
    pluginModalManager.openModal("plugin-a", {
      content: () => <div>Untitled content</div>,
      presentation: "drawer",
    });

    render(<PluginModalHost />);

    expect(screen.getByRole("dialog", { name: "Plugin dialog" })).not.toBeNull();
  });

  it("removes the modal from the DOM once its handle is closed", () => {
    const handle = pluginModalManager.openModal("plugin-a", {
      content: () => <div data-testid="modal-content">Hello</div>,
    });

    render(<PluginModalHost />);
    expect(screen.getByTestId("modal-content")).not.toBeNull();

    act(() => {
      handle.close();
    });

    expect(screen.queryByTestId("modal-content")).toBeNull();
  });
});

describe("PluginModalHost — keyboard and close-button focus return", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
  });

  it("returns focus to the opening action after Escape", async () => {
    function ModalContent() {
      return <button data-testid={MODAL_FOCUS_TARGET_TEST_ID}>Modal action</button>;
    }

    render(
      <>
        <PluginModalHost />
        <button
          data-testid={MODAL_OPENER_TEST_ID}
          onClick={() => pluginModalManager.openModal("plugin-a", { content: ModalContent })}
        >
          Open modal
        </button>
      </>,
    );

    const opener = screen.getByTestId(MODAL_OPENER_TEST_ID);
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByTestId(MODAL_FOCUS_TARGET_TEST_ID)),
    );

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape", code: "Escape" });

    await waitFor(() => expect(screen.queryByTestId(MODAL_FOCUS_TARGET_TEST_ID)).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("returns focus when the close control dismisses a dialog", async () => {
    render(
      <>
        <PluginModalHost />
        <button
          data-testid={MODAL_OPENER_TEST_ID}
          onClick={() =>
            pluginModalManager.openModal("plugin-a", {
              content: () => <button data-testid={MODAL_FOCUS_TARGET_TEST_ID}>Modal action</button>,
            })
          }
        >
          Open modal
        </button>
      </>,
    );

    const opener = screen.getByTestId(MODAL_OPENER_TEST_ID);
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByTestId(MODAL_FOCUS_TARGET_TEST_ID)).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Close" }));

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });
});

describe("PluginModalHost — programmatic close focus return", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
  });

  it("returns focus when a modal handle closes the dialog", async () => {
    let closeHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(
      <>
        <PluginModalHost />
        <button data-testid={MODAL_OPENER_TEST_ID}>Open modal</button>
      </>,
    );

    const opener = screen.getByTestId(MODAL_OPENER_TEST_ID);
    opener.focus();
    act(() => {
      closeHandle = pluginModalManager.openModal("plugin-a", {
        content: () => <button data-testid={MODAL_FOCUS_TARGET_TEST_ID}>Modal action</button>,
      });
    });
    await waitFor(() => expect(screen.getByTestId(MODAL_FOCUS_TARGET_TEST_ID)).toBeTruthy());
    act(() => closeHandle?.close());

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("returns focus from an explicitly requested drawer", async () => {
    let closeHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(
      <>
        <PluginModalHost />
        <button data-testid="drawer-opener">Open drawer</button>
      </>,
    );

    const opener = screen.getByTestId("drawer-opener");
    opener.focus();
    act(() => {
      closeHandle = pluginModalManager.openModal("plugin-a", {
        content: () => <button data-testid="drawer-focus-target">Drawer action</button>,
        presentation: "drawer",
      });
    });
    await waitFor(() => expect(screen.getByTestId("drawer-focus-target")).toBeTruthy());
    act(() => closeHandle?.close());

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });
});

describe("PluginModalHost — unavailable opener recovery", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
  });

  it.each([
    "removed",
    "disabled",
    "hidden",
    "inert",
    "hidden ancestor",
    "inert ancestor",
    "CSS-hidden ancestor",
  ] as const)("keeps focus in a surviving dialog when the opener is %s", async (state) => {
    let parentHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    let childHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(<PluginModalHost />);

    act(() => {
      parentHandle = pluginModalManager.openModal("plugin-a", {
        title: PARENT_MODAL_TITLE,
        content: () => (
          <div data-testid="parent-action-container">
            <button data-testid="parent-action">Parent action</button>
          </div>
        ),
      });
    });
    const parentSurface = screen.getByRole("dialog", { name: PARENT_MODAL_TITLE });
    const parentAction = screen.getByTestId("parent-action");
    const parentActionContainer = screen.getByTestId("parent-action-container");
    parentAction.focus();

    act(() => {
      childHandle = pluginModalManager.openModal("plugin-a", {
        title: CHILD_MODAL_TITLE,
        content: () => <button data-testid="child-action">Child action</button>,
      });
    });
    await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("child-action")));

    if (state === "removed") parentAction.remove();
    if (state === "disabled") (parentAction as HTMLButtonElement).disabled = true;
    if (state === "hidden") (parentAction as HTMLButtonElement).hidden = true;
    if (state === "inert") parentAction.setAttribute("inert", "");
    if (state === "hidden ancestor") (parentActionContainer as HTMLElement).hidden = true;
    if (state === "inert ancestor") parentActionContainer.setAttribute("inert", "");
    if (state === "CSS-hidden ancestor") {
      (parentActionContainer as HTMLElement).style.display = "none";
    }

    act(() => childHandle?.close());

    await waitFor(() => expect(parentSurface.contains(document.activeElement)).toBe(true));
    act(() => parentHandle?.close());
  });
});

describe("PluginModalHost — nested surface focus", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
  });

  it("returns a child modal to its available opener in the parent", async () => {
    let parentHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    let childHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(<PluginModalHost />);

    act(() => {
      parentHandle = pluginModalManager.openModal("plugin-a", {
        title: PARENT_MODAL_TITLE,
        content: () => <button data-testid="parent-action">Parent action</button>,
      });
    });
    const parentAction = screen.getByTestId("parent-action");
    parentAction.focus();
    act(() => {
      childHandle = pluginModalManager.openModal("plugin-a", {
        title: CHILD_MODAL_TITLE,
        content: () => <button data-testid="child-action">Child action</button>,
      });
    });
    await waitFor(() => expect(screen.getByTestId("child-action")).toBeTruthy());

    act(() => childHandle?.close());

    await waitFor(() => expect(document.activeElement).toBe(parentAction));
    act(() => parentHandle?.close());
  });
});

describe("PluginModalHost — overlapping modal focus", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
    cleanupModals("plugin-b");
    if (vi.isFakeTimers()) {
      act(() => vi.runOnlyPendingTimers());
      vi.useRealTimers();
    }
  });

  it("does not move focus behind a newer modal when a background modal closes", async () => {
    let backgroundHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    let foregroundHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(<PluginModalHost />);

    const pageOpener = document.createElement("button");
    document.body.append(pageOpener);
    pageOpener.focus();
    act(() => {
      backgroundHandle = pluginModalManager.openModal("plugin-a", {
        title: "Background modal",
        content: () => <button data-testid="background-action">Background action</button>,
      });
    });
    const backgroundAction = screen.getByTestId("background-action");
    backgroundAction.focus();
    act(() => {
      foregroundHandle = pluginModalManager.openModal("plugin-b", {
        title: "Foreground modal",
        content: () => <button data-testid="foreground-action">Foreground action</button>,
      });
    });
    const foregroundAction = screen.getByTestId("foreground-action");
    await waitFor(() => expect(document.activeElement).toBe(foregroundAction));

    vi.useFakeTimers();
    act(() => backgroundHandle?.close());
    act(() => vi.runOnlyPendingTimers());

    expect(document.activeElement).toBe(foregroundAction);
    expect(screen.getByRole("dialog", { name: "Foreground modal" })).toBeTruthy();
    act(() => foregroundHandle?.close());
    act(() => vi.runOnlyPendingTimers());
    vi.useRealTimers();
    pageOpener.remove();
  });

  it("preserves a foreground plugin popover when a background modal closes", async () => {
    let backgroundHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    let foregroundHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(<PluginModalHost />);

    const pageOpener = document.createElement("button");
    document.body.append(pageOpener);
    pageOpener.focus();
    act(() => {
      backgroundHandle = pluginModalManager.openModal("plugin-a", {
        title: "Background modal",
        content: () => <button data-testid="background-action">Background action</button>,
      });
      foregroundHandle = pluginModalManager.openModal("plugin-b", {
        title: "Foreground modal",
        content: ForegroundPopoverContent,
      });
    });

    const popoverTrigger = screen.getByTestId("foreground-popover-trigger");
    fireEvent.click(popoverTrigger);
    const popoverAction = await screen.findByTestId("foreground-popover-action");
    popoverAction.focus();
    expect(document.activeElement).toBe(popoverAction);

    vi.useFakeTimers();
    act(() => backgroundHandle?.close());
    act(() => vi.runOnlyPendingTimers());

    expect(screen.getByTestId("foreground-popover-content").getAttribute("data-state")).toBe(
      "open",
    );
    expect(document.activeElement).toBe(popoverAction);
    act(() => foregroundHandle?.close());
    act(() => vi.runOnlyPendingTimers());
    vi.useRealTimers();
    pageOpener.remove();
  });
});

describe("PluginModalHost — no unrelated focus fallback", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
  });

  it("does not focus an unrelated control when the opener is unavailable", async () => {
    let closeHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(
      <>
        <PluginModalHost />
        <button
          data-testid={MODAL_OPENER_TEST_ID}
          onClick={() =>
            (closeHandle = pluginModalManager.openModal("plugin-a", {
              content: () => <button data-testid={MODAL_FOCUS_TARGET_TEST_ID}>Modal action</button>,
            }))
          }
        >
          Open modal
        </button>
        <button data-testid="unrelated-control">Unrelated control</button>
      </>,
    );

    const opener = screen.getByTestId(MODAL_OPENER_TEST_ID);
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByTestId(MODAL_FOCUS_TARGET_TEST_ID)).toBeTruthy());
    (opener as HTMLButtonElement).disabled = true;

    act(() => closeHandle?.close());

    await waitFor(() => expect(screen.queryByTestId(MODAL_FOCUS_TARGET_TEST_ID)).toBeNull());
    expect(document.activeElement).not.toBe(screen.getByTestId("unrelated-control"));
  });
});

describe("PluginModalHost — owner cleanup focus", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
    cleanupModals("plugin-b");
    if (vi.isFakeTimers()) {
      act(() => vi.runOnlyPendingTimers());
      vi.useRealTimers();
    }
  });

  it("preserves foreground focus when owner cleanup removes multiple modals", async () => {
    let foregroundHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(<PluginModalHost />);

    act(() => {
      pluginModalManager.openModal("plugin-a", {
        title: "First owned modal",
        content: () => <div>First owned content</div>,
      });
      pluginModalManager.openModal("plugin-a", {
        title: "Second owned modal",
        content: () => <div>Second owned content</div>,
      });
      foregroundHandle = pluginModalManager.openModal("plugin-b", {
        title: "Other owner modal",
        content: () => <button data-testid="other-owner-action">Other owner action</button>,
      });
    });
    const foregroundAction = screen.getByTestId("other-owner-action");
    await waitFor(() => expect(document.activeElement).toBe(foregroundAction));

    vi.useFakeTimers();
    act(() => pluginModalManager.closeAllForPlugin("plugin-a"));
    act(() => vi.runOnlyPendingTimers());

    expect(document.activeElement).toBe(foregroundAction);
    expect(screen.getByRole("dialog", { name: "Other owner modal" })).toBeTruthy();
    act(() => foregroundHandle?.close());
    act(() => vi.runOnlyPendingTimers());
    vi.useRealTimers();
  });

  it("runs a background close callback after its successor opens", async () => {
    let backgroundHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    let successorHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(<PluginModalHost />);

    act(() => {
      backgroundHandle = pluginModalManager.openModal("plugin-a", {
        title: "Old modal",
        content: () => <button data-testid="old-modal-action">Old action</button>,
      });
    });
    await screen.findByTestId("old-modal-action");

    vi.useFakeTimers();
    act(() => backgroundHandle?.close());
    act(() => {
      successorHandle = pluginModalManager.openModal("plugin-b", {
        title: "Successor modal",
        content: () => <button data-testid="successor-action">Successor action</button>,
      });
    });
    const successorAction = screen.getByTestId("successor-action");
    successorAction.focus();

    act(() => vi.runOnlyPendingTimers());

    expect(document.activeElement).toBe(successorAction);
    expect(screen.getByRole("dialog", { name: "Successor modal" })).toBeTruthy();
    act(() => successorHandle?.close());
    act(() => vi.runOnlyPendingTimers());
    vi.useRealTimers();
  });
});

function ForegroundPopoverContent() {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button data-testid="foreground-popover-trigger">Open popover</button>
      </PopoverTrigger>
      <PopoverContent data-testid="foreground-popover-content">
        <button data-testid="foreground-popover-action">Popover action</button>
      </PopoverContent>
    </Popover>
  );
}

/**
 * The host mounts as a sibling of `<AppShell/>` (src/main.tsx), so it is
 * outside the app-wide `TooltipProvider` in app/layout.tsx. Without a
 * provider of its own, a `Tooltip` anywhere in plugin modal content throws on
 * render and the whole modal is lost to the error boundary.
 *
 * jsdom does not reliably open a Radix tooltip from synthetic hover (see
 * apps/web/CLAUDE.md), so the open assertion goes through focus. Pointer
 * hover is a Playwright concern.
 */
describe("PluginModalHost — tooltips in modal content", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
  });

  function TooltipContentComponent() {
    return (
      <Tooltip>
        <TooltipTrigger data-testid="tooltip-trigger">Trigger</TooltipTrigger>
        <TooltipContent>Helpful hint</TooltipContent>
      </Tooltip>
    );
  }

  it("renders modal content containing a Tooltip without throwing", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "Tooltip Modal",
      content: TooltipContentComponent,
    });

    render(<PluginModalHost />);

    expect(screen.getByTestId("tooltip-trigger")).not.toBeNull();
  });

  it("opens the tooltip on focus, proving a TooltipProvider is in scope", async () => {
    pluginModalManager.openModal("plugin-a", { content: TooltipContentComponent });

    render(<PluginModalHost />);
    fireEvent.focus(screen.getByTestId("tooltip-trigger"));

    await waitFor(() => {
      expect(screen.getAllByText("Helpful hint").length).toBeGreaterThan(0);
    });
  });
});

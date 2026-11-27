import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ModalToastOutlet } from "@kandev/ui/modal-toast-outlet";
import { Dialog, DialogContent } from "@kandev/ui/dialog";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";

const report = vi.hoisted(() => vi.fn());
const TOAST_MESSAGE_TEST_ID = "toast-message";
const TOAST_MESSAGE_SELECTOR = '[data-testid="toast-message"]';

beforeEach(() => {
  cleanup();
  report.mockReset();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

vi.mock("@/lib/api/domains/frontend-error-log-api", () => ({
  scheduleFrontendErrorReport: report,
}));

import { ToastProvider, useToast } from "./toast-provider";

const TOAST_ANNOUNCER_TEST_ID = "toast-announcer";
const REPEATED_NOTICE_TITLE = "Repeated notice";

describe("ToastProvider error reporting", () => {
  it("reports error creation without changing the rendered toast", () => {
    const api = renderProvider();
    let id = "";
    act(() => {
      id = api.toast({
        title: "Failed to save",
        description: "The backend rejected the update",
        variant: "error",
      });
    });

    expect(id).not.toBe("");
    expect(screen.getByText("Failed to save")).toBeTruthy();
    expect(screen.getByText("The backend rejected the update")).toBeTruthy();
    expect(report).toHaveBeenCalledOnce();
    expect(report).toHaveBeenCalledWith({
      source: "toast-provider",
      title: "Failed to save",
      description: "The backend rejected the update",
    });
  });

  it("reports each transition into error exactly once", () => {
    const api = renderProvider();
    let id = "";
    act(() => {
      id = api.toast({ title: "Saving", variant: "loading" });
    });
    expect(report).not.toHaveBeenCalled();

    act(() => api.updateToast(id, { title: "First failure", variant: "error" }));
    act(() => api.updateToast(id, { description: "More detail", variant: "error" }));
    expect(report).toHaveBeenCalledTimes(1);
    expect(report).toHaveBeenLastCalledWith({
      source: "toast-provider",
      title: "First failure",
      description: undefined,
    });

    act(() => api.updateToast(id, { title: "Recovered", variant: "success" }));
    act(() => api.updateToast(id, { title: "Failed again", variant: "error" }));
    expect(report).toHaveBeenCalledTimes(2);
    expect(report).toHaveBeenLastCalledWith({
      source: "toast-provider",
      title: "Failed again",
      description: "More detail",
    });
  });
});

describe("ToastProvider announcements", () => {
  // @covers AC-UI-TOAST-INTERACTION-001.4
  it("keeps one polite live region in the active modal before its first toast", async () => {
    const api = renderProviderWithAnnouncementDialog();
    const dialog = screen.getByTestId("announcement-dialog");
    const announcer = screen.getByTestId(TOAST_ANNOUNCER_TEST_ID);

    expect(announcer.getAttribute("aria-live")).toBe("polite");
    expect(dialog.contains(announcer)).toBe(true);
    expect(announcer.closest('[aria-hidden="true"]')).toBeNull();
    expect(screen.getAllByTestId(TOAST_ANNOUNCER_TEST_ID)).toHaveLength(1);
    expect(document.querySelectorAll('[aria-live="polite"]')).toHaveLength(1);

    vi.useFakeTimers();
    act(() => {
      api.toast({ title: "Update ready", description: "Version 2 is available." });
    });
    await act(async () => vi.advanceTimersToNextTimerAsync());

    expect(screen.getByTestId(TOAST_ANNOUNCER_TEST_ID)).toBe(announcer);
    expect(dialog.contains(announcer)).toBe(true);
    expect(document.querySelectorAll('[aria-live="polite"]')).toHaveLength(1);
    expect(announcer.textContent).toContain("Update ready");
    expect(announcer.textContent).toContain("Version 2 is available.");
  });

  it("queues notices and re-announces repeated text with a live-region mutation", async () => {
    const api = renderProviderWithAnnouncementDialog();
    vi.useFakeTimers();
    const announcer = screen.getByTestId(TOAST_ANNOUNCER_TEST_ID);

    act(() => {
      api.toast({ title: "First notice", variant: "loading" });
      api.toast({ title: REPEATED_NOTICE_TITLE, variant: "loading" });
    });
    expect(announcer.textContent).toBe("");

    await act(async () => vi.advanceTimersToNextTimerAsync());
    expect(announcer.textContent).toBe("First notice");
    await act(async () => vi.advanceTimersToNextTimerAsync());
    expect(announcer.textContent).toBe("");
    await act(async () => vi.advanceTimersToNextTimerAsync());
    expect(announcer.textContent).toBe(REPEATED_NOTICE_TITLE);

    act(() => api.toast({ title: REPEATED_NOTICE_TITLE, variant: "loading" }));
    await act(async () => vi.advanceTimersToNextTimerAsync());
    expect(announcer.textContent).toBe("");
    await act(async () => vi.advanceTimersToNextTimerAsync());
    expect(announcer.textContent).toBe(REPEATED_NOTICE_TITLE);
  });

  it("cancels the pending announcement timer on unmount", () => {
    vi.useFakeTimers();
    let api!: ReturnType<typeof useToast>;
    function Capture() {
      api = useToast();
      return null;
    }
    const view = render(
      <ToastProvider>
        <Capture />
      </ToastProvider>,
    );

    act(() => api.toast({ title: "Pending notice", variant: "loading" }));
    expect(vi.getTimerCount()).toBe(1);

    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("ToastProvider action", () => {
  it("runs the action once and dismisses the toast", () => {
    const api = renderProvider();
    const onClick = vi.fn();
    act(() => {
      api.toast({ title: "Rejected", action: { label: "Keep it", onClick } });
    });
    fireEvent.click(screen.getByRole("button", { name: "Keep it" }));
    expect(onClick).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("Rejected")).toBeNull();
  });

  // @covers AC-UI-TOAST-INTERACTION-001.1, AC-UI-TOAST-INTERACTION-001.2
  it("dismisses a loading toast independently while an actionable toast remains", () => {
    const api = renderProvider();
    const onClick = vi.fn();
    act(() => {
      api.toast({ title: "Waiting for the server", variant: "loading" });
      api.toast({ title: "Update failed", action: { label: "Retry", onClick } });
    });

    const loadingToast = screen.getByText("Waiting for the server").closest(TOAST_MESSAGE_SELECTOR);
    expect(loadingToast).not.toBeNull();
    fireEvent.click(
      within(loadingToast as HTMLElement).getByRole("button", { name: "Dismiss notification" }),
    );

    expect(screen.queryByText("Waiting for the server")).toBeNull();
    const actionableToast = screen.getByTestId(TOAST_MESSAGE_TEST_ID);
    expect(actionableToast.textContent).toContain("Update failed");
    fireEvent.click(within(actionableToast).getByRole("button", { name: "Retry" }));
    expect(onClick).toHaveBeenCalledOnce();
    expect(screen.queryByText("Update failed")).toBeNull();
  });
});

describe("ToastProvider modal outlets", () => {
  // @covers AC-UI-TOAST-INTERACTION-001.4, AC-UI-TOAST-INTERACTION-001.5
  it("moves a timed toast through nested modal outlets without restarting its timer", () => {
    vi.useFakeTimers();
    const api = renderProviderWithModalOutlets();
    act(() => {
      api.toast({ title: "Timed notice", duration: 5000 });
    });
    act(() => vi.advanceTimersByTime(1000));

    fireEvent.click(screen.getByRole("button", { name: "Open parent modal" }));
    const parent = screen.getByTestId("parent-modal");
    expect(parent.contains(screen.getByTestId(TOAST_MESSAGE_TEST_ID))).toBe(true);
    expect(screen.getAllByTestId(TOAST_MESSAGE_TEST_ID)).toHaveLength(1);
    act(() => vi.advanceTimersByTime(1000));

    fireEvent.click(screen.getByRole("button", { name: "Open nested modal" }));
    const nested = screen.getByTestId("nested-modal");
    expect(nested.contains(screen.getByTestId(TOAST_MESSAGE_TEST_ID))).toBe(true);
    expect(parent.contains(screen.getByTestId(TOAST_MESSAGE_TEST_ID))).toBe(false);
    expect(screen.getAllByTestId(TOAST_MESSAGE_TEST_ID)).toHaveLength(1);
    act(() => vi.advanceTimersByTime(1000));

    fireEvent.click(screen.getByRole("button", { name: "Close nested modal" }));
    expect(parent.contains(screen.getByTestId(TOAST_MESSAGE_TEST_ID))).toBe(true);
    expect(screen.getAllByTestId(TOAST_MESSAGE_TEST_ID)).toHaveLength(1);
    act(() => vi.advanceTimersByTime(1500));

    fireEvent.click(screen.getByRole("button", { name: "Close parent modal" }));
    expect(screen.getAllByTestId(TOAST_MESSAGE_TEST_ID)).toHaveLength(1);
    const message = screen.getByTestId(TOAST_MESSAGE_TEST_ID);
    const viewport = screen
      .getAllByTestId("toast-container")
      .find((container) => container.contains(message));
    expect(parent.contains(message)).toBe(false);
    expect(viewport).toBeDefined();
    act(() => vi.advanceTimersByTime(500));
    expect(screen.queryByText("Timed notice")).toBeNull();
  });

  // @covers AC-UI-TOAST-INTERACTION-001.5
  it("keeps toast placement in the viewport when an open dialog is nonmodal", () => {
    const api = renderProviderWithNonModalDialog();
    act(() => {
      api.toast({ title: "Outside the modal scope", duration: 10_000 });
    });

    expect(screen.getByTestId("non-modal-dialog")).toBeTruthy();
    expect(screen.queryByTestId("modal-toast-outlet")).toBeNull();
    const message = screen.getByTestId(TOAST_MESSAGE_TEST_ID);
    expect(
      screen.getAllByTestId("toast-container").some((container) => container.contains(message)),
    ).toBe(true);
  });

  it("renders a shared dialog without a ToastProvider", () => {
    render(
      <Dialog open>
        <DialogContent showCloseButton={false} data-testid="dialog-without-toast-provider">
          Dialog content
        </DialogContent>
      </Dialog>,
    );

    expect(screen.getByTestId("dialog-without-toast-provider")).toBeTruthy();
    expect(screen.queryByTestId("modal-toast-outlet")).toBeNull();
  });

  it("keeps AlertDialogContent asChild slotted around its modal toast outlet", () => {
    let api!: ReturnType<typeof useToast>;
    function Harness() {
      api = useToast();
      return (
        <AlertDialog open>
          <AlertDialogContent asChild data-testid="alert-dialog-as-child">
            <section data-testid="alert-dialog-section">
              <AlertDialogTitle>Confirm action</AlertDialogTitle>
              <AlertDialogDescription>This action needs confirmation.</AlertDialogDescription>
            </section>
          </AlertDialogContent>
        </AlertDialog>
      );
    }

    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );

    const section = screen.getByTestId("alert-dialog-section");
    expect(section.getAttribute("data-slot")).toBe("alert-dialog-content");
    act(() => {
      api.toast({ title: "Alert dialog toast", duration: 10_000 });
    });
    expect(section.contains(screen.getByTestId(TOAST_MESSAGE_TEST_ID))).toBe(true);
  });
});

function renderProvider() {
  let current!: ReturnType<typeof useToast>;
  function Capture() {
    current = useToast();
    return null;
  }
  render(
    <ToastProvider>
      <Capture />
    </ToastProvider>,
  );
  return current;
}

function renderProviderWithModalOutlets() {
  let current!: ReturnType<typeof useToast>;
  function Harness() {
    current = useToast();
    const [parentOpen, setParentOpen] = useState(false);
    const [nestedOpen, setNestedOpen] = useState(false);
    return (
      <>
        <button type="button" onClick={() => setParentOpen(true)}>
          Open parent modal
        </button>
        {parentOpen && (
          <section data-testid="parent-modal">
            <ModalToastOutlet />
            <button type="button" onClick={() => setNestedOpen(true)}>
              Open nested modal
            </button>
            <button type="button" onClick={() => setParentOpen(false)}>
              Close parent modal
            </button>
          </section>
        )}
        {parentOpen && nestedOpen && (
          <section data-testid="nested-modal">
            <ModalToastOutlet />
            <button type="button" onClick={() => setNestedOpen(false)}>
              Close nested modal
            </button>
          </section>
        )}
      </>
    );
  }
  render(
    <ToastProvider>
      <Harness />
    </ToastProvider>,
  );
  return current;
}

function renderProviderWithAnnouncementDialog() {
  let current!: ReturnType<typeof useToast>;
  function Harness() {
    current = useToast();
    return (
      <Dialog open>
        <DialogContent showCloseButton={false} data-testid="announcement-dialog">
          Dialog content
        </DialogContent>
      </Dialog>
    );
  }
  render(
    <ToastProvider>
      <Harness />
    </ToastProvider>,
  );
  return current;
}

function renderProviderWithNonModalDialog() {
  let current!: ReturnType<typeof useToast>;
  function Harness() {
    current = useToast();
    return (
      <Dialog open modal={false}>
        <DialogContent showCloseButton={false} data-testid="non-modal-dialog">
          Non-modal content
        </DialogContent>
      </Dialog>
    );
  }
  render(
    <ToastProvider>
      <Harness />
    </ToastProvider>,
  );
  return current;
}

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BrowserPanel } from "./browser-panel";

const usePreviewCapture = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/use-preview-capture", () => ({ usePreviewCapture }));
vi.mock("@/hooks/use-preview-console-forwarder", () => ({
  usePreviewConsoleForwarder: vi.fn(),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      tasks: { activeTaskId: "task-1", activeSessionId: "session-1" },
      processes: { devProcessBySessionId: {}, outputsByProcessId: {} },
    }),
}));

const capture = {
  items: [],
  snapshot: { task_id: "task-1", revision: 0, items: [] },
  mode: null,
  draft: null,
  draftComment: "",
  setDraftComment: vi.fn(),
  candidateLabel: null,
  captureError: null,
  isRasterizing: false,
  isUploading: false,
  isMutating: false,
  mutationError: null,
  startCapture: vi.fn(),
  cancelCapture: vi.fn(),
  discardDraft: vi.fn(),
  saveDraft: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  clear: vi.fn(),
  handleIframeLoad: vi.fn(),
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("BrowserPanel preview feedback", () => {
  it("offers task-backed annotation for a localhost page through the injected proxy", () => {
    usePreviewCapture.mockReturnValue(capture);
    render(<BrowserPanel panelId="browser-1" params={{ url: "http://localhost:3000/products" }} />);

    expect(screen.getByRole("button", { name: "Annotate (0)" })).toBeTruthy();
    expect(usePreviewCapture).toHaveBeenCalledWith(
      expect.objectContaining({
        taskId: "task-1",
        enabled: true,
        source: {
          kind: "browser",
          sessionId: "session-1",
          label: "http://localhost:3000",
        },
      }),
    );
  });

  it("keeps task-backed annotation visible when secondary actions overflow", async () => {
    class ResizeObserverMock {
      constructor(private readonly callback: ResizeObserverCallback) {}

      observe(target: Element) {
        Object.defineProperty(target, "getBoundingClientRect", {
          configurable: true,
          value: () => ({ width: 240 }),
        });
        this.callback([{ target } as ResizeObserverEntry], this as unknown as ResizeObserver);
      }

      disconnect() {}
    }

    vi.stubGlobal("ResizeObserver", ResizeObserverMock);
    usePreviewCapture.mockReturnValue(capture);
    render(<BrowserPanel panelId="browser-1" params={{ url: "http://localhost:3000/products" }} />);

    await waitFor(() => expect(screen.getByTestId("panel-header-overflow")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Annotate (0)" })).toBeTruthy();
  });

  it("allows the annotation popover to extend beyond the panel header", () => {
    usePreviewCapture.mockReturnValue(capture);
    render(<BrowserPanel panelId="browser-1" params={{ url: "http://localhost:3000/products" }} />);

    fireEvent.click(screen.getByRole("button", { name: "Annotate (0)" }));
    const actionSlot = screen.getByTestId("preview-feedback-popover").parentElement?.parentElement;
    expect(actionSlot?.className).toContain("overflow-visible");
    expect(actionSlot?.className).not.toContain("overflow-hidden");
  });
});

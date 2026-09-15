import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskPreviewFeedback } from "@/lib/types/http";

const drawerMode = vi.hoisted(() => ({ value: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => drawerMode.value,
}));

vi.mock("@kandev/ui/popover", async () => {
  const React = await import("react");
  const Context = React.createContext<{ open: boolean; setOpen: (open: boolean) => void } | null>(
    null,
  );
  return {
    Popover: ({
      open,
      onOpenChange,
      children,
    }: React.PropsWithChildren<{ open: boolean; onOpenChange: (open: boolean) => void }>) => (
      <Context.Provider value={{ open, setOpen: onOpenChange }}>{children}</Context.Provider>
    ),
    PopoverTrigger: ({ children }: React.PropsWithChildren) => {
      const value = React.useContext(Context)!;
      return React.cloneElement(children as React.ReactElement<{ onClick?: () => void }>, {
        onClick: () => value.setOpen(!value.open),
      });
    },
    PopoverContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => {
      const value = React.useContext(Context)!;
      return value.open ? <div {...props}>{children}</div> : null;
    },
  };
});

vi.mock("@kandev/ui/drawer", async () => {
  const React = await import("react");
  const Context = React.createContext<{ open: boolean; setOpen: (open: boolean) => void } | null>(
    null,
  );
  return {
    Drawer: ({
      open,
      onOpenChange,
      children,
    }: React.PropsWithChildren<{ open: boolean; onOpenChange: (open: boolean) => void }>) => (
      <Context.Provider value={{ open, setOpen: onOpenChange }}>{children}</Context.Provider>
    ),
    DrawerContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => {
      const value = React.useContext(Context)!;
      return value.open ? <div {...props}>{children}</div> : null;
    },
    DrawerHeader: (props: React.HTMLAttributes<HTMLDivElement>) => <div {...props} />,
    DrawerTitle: (props: React.HTMLAttributes<HTMLHeadingElement>) => <h2 {...props} />,
  };
});

import {
  PreviewFeedbackControls,
  type PreviewFeedbackController,
} from "./preview-feedback-controls";

const item: TaskPreviewFeedback = {
  id: "feedback-1",
  task_id: "task-1",
  kind: "element",
  comment: "Align this with the price",
  source_kind: "browser",
  source_session_id: "session-1",
  source_label: "Local app",
  page_route: "/products",
  page_title: "Products",
  element_snapshot: {
    tag: "button",
    id: "checkout",
    classes: ["primary"],
    outer_html: '<button id="checkout">Buy</button>',
  },
  version: 2,
  created_at: "2026-09-15T00:00:00Z",
  updated_at: "2026-09-15T00:00:00Z",
};

function controller(overrides: Partial<PreviewFeedbackController> = {}): PreviewFeedbackController {
  return {
    items: [item],
    snapshot: { task_id: "task-1", revision: 3, items: [item] },
    mode: null,
    draft: null,
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
    ...overrides,
  };
}

describe("PreviewFeedbackControls", () => {
  afterEach(cleanup);

  beforeEach(() => {
    drawerMode.value = false;
  });

  it("offers text and visually hinted element selection from a desktop review popover", () => {
    const capture = controller();
    render(<PreviewFeedbackControls capture={capture} enabled />);

    fireEvent.click(screen.getByRole("button", { name: /Annotate.*1/i }));
    expect(screen.getByTestId("preview-feedback-popover")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Select element" }));
    expect(capture.startCapture).toHaveBeenCalledWith("element");
  });

  it("offers screenshot-region mode and shows the actual PNG before saving", () => {
    const capture = controller({
      items: [],
      snapshot: { task_id: "task-1", revision: 0, items: [] },
      draft: {
        kind: "screenshot",
        page_route: "/checkout",
        page_title: "Checkout",
        capture_rect: { x: 20, y: 40, width: 300, height: 180 },
        screenshot: {
          blob: new Blob([], { type: "image/png" }),
          width: 600,
          height: 360,
          previewUrl: "blob:preview-screenshot",
          fileName: "preview.png",
        },
      },
    });
    render(<PreviewFeedbackControls capture={capture} enabled />);

    const preview = screen.getByRole("img", { name: "Screenshot preview" });
    expect(preview.getAttribute("src")).toBe("blob:preview-screenshot");
    expect(screen.getByText("600 × 360 · PNG")).toBeTruthy();
  });

  it("keeps the captured evidence while collecting the required comment", async () => {
    const capture = controller({
      items: [],
      snapshot: { task_id: "task-1", revision: 0, items: [] },
      draft: {
        kind: "text",
        page_route: "/checkout",
        page_title: "Checkout",
        selected_text: "$42.00",
        text_anchor: {
          start: { node_path: [0], offset: 0 },
          end: { node_path: [0], offset: 6 },
        },
      },
    });
    render(<PreviewFeedbackControls capture={capture} enabled />);

    expect(screen.getByText("$42.00")).toBeTruthy();
    const comment = screen.getByLabelText("Comment on selection");
    fireEvent.change(comment, { target: { value: "Keep the generated total visible" } });
    fireEvent.click(screen.getByRole("button", { name: "Save feedback" }));
    expect(capture.saveDraft).toHaveBeenCalledWith("Keep the generated total visible");
  });

  it("uses a touch-sized drawer entry and exposes the same capture choices", () => {
    drawerMode.value = true;
    const capture = controller();
    render(<PreviewFeedbackControls capture={capture} enabled />);

    const trigger = screen.getByRole("button", { name: /Annotate.*1/i });
    expect(trigger.className).toContain("h-11");
    fireEvent.click(trigger);
    expect(screen.getByTestId("preview-feedback-drawer")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Select text" }));
    expect(capture.startCapture).toHaveBeenCalledWith("text");
    fireEvent.click(screen.getByRole("button", { name: /Annotate.*1/i }));
    fireEvent.click(screen.getByRole("button", { name: "Select screenshot region" }));
    expect(capture.startCapture).toHaveBeenCalledWith("screenshot");
  });
});

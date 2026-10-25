import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskPreviewFeedback, TaskPreviewFeedbackSnapshot } from "@/lib/types/http";

const touchState = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchState.enabled,
}));

vi.mock("@/components/task/mobile/mobile-picker-sheet", () => ({
  MobilePickerSheet: ({
    open,
    title,
    headerAction,
    children,
  }: {
    open: boolean;
    title: string;
    headerAction?: React.ReactNode;
    children: React.ReactNode;
  }) =>
    open ? (
      <div data-testid="preview-feedback-collection-drawer">
        <h2>{title}</h2>
        {headerAction}
        {children}
      </div>
    ) : null,
}));

vi.mock("@kandev/ui/popover", async () => {
  const React = await import("react");
  const Context = React.createContext<{
    open: boolean;
    onOpenChange: (open: boolean) => void;
  } | null>(null);
  return {
    Popover: ({
      open,
      onOpenChange,
      children,
    }: React.PropsWithChildren<{
      open: boolean;
      onOpenChange: (open: boolean) => void;
    }>) => <Context.Provider value={{ open, onOpenChange }}>{children}</Context.Provider>,
    PopoverAnchor: ({ children }: React.PropsWithChildren) => <>{children}</>,
    PopoverContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => {
      const value = React.useContext(Context)!;
      return value.open ? <div {...props}>{children}</div> : null;
    },
  };
});

vi.mock("@kandev/ui/drawer", () => ({
  DrawerClose: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock("@/lib/api/domains/attachment-api", () => ({
  attachmentContentUrl: (attachmentId: string) => `/api/v1/attachments/${attachmentId}/content`,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: { count?: number }) => {
      const editCommentLabel = "Edit comment";
      const copy: Record<string, string> = {
        "task:previewPendingFeedback": "Pending feedback",
        "task:previewClearAll": "Clear all",
        "task:previewConfirmClearDescription":
          "Remove all pending preview feedback from this task?",
        "task:previewCancel": "Cancel",
        "task:previewDeleteFeedback": "Delete feedback",
        "task:previewEditFeedback": "Edit feedback",
        "task:previewEditComment": editCommentLabel,
        "task:previewSaveChanges": "Save changes",
        "task:previewPendingEmpty": "No pending preview feedback.",
        "task:previewFeedbackTitle": "Preview feedback",
        "task:previewScreenshotAlt": "Screenshot preview",
        "task:previewLoadFailed": "The pending preview feedback could not be loaded. Try again.",
        "task:previewMutationFailed": "The feedback could not be saved. Try again.",
        "common:close": "Close",
      };
      if (key === "task:previewFeedbackCount")
        return `${values?.count ?? 0} preview feedback items`;
      return copy[key] ?? key;
    },
  }),
}));

const editCommentLabel = "Edit comment";

import {
  PreviewFeedbackCollection,
  PreviewFeedbackCollectionSurface,
  type PreviewFeedbackCollectionController,
} from "./preview-feedback-collection";

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

const snapshot: TaskPreviewFeedbackSnapshot = {
  task_id: "task-1",
  revision: 3,
  items: [item],
};

function controller(
  overrides: Partial<PreviewFeedbackCollectionController> = {},
): PreviewFeedbackCollectionController {
  return {
    items: [item],
    snapshot,
    isLoading: false,
    loadError: null,
    isMutating: false,
    mutationError: null,
    update: vi.fn().mockResolvedValue(snapshot),
    remove: vi.fn().mockResolvedValue(snapshot),
    clear: vi.fn().mockResolvedValue({ task_id: "task-1", revision: 4, items: [] }),
    ...overrides,
  };
}

afterEach(cleanup);

describe("PreviewFeedbackCollection", () => {
  beforeEach(() => {
    touchState.enabled = false;
  });

  it("edits, deletes, and clears with the displayed item versions and revision", async () => {
    const collection = controller();
    render(<PreviewFeedbackCollection collection={collection} touch={false} />);

    fireEvent.click(screen.getByRole("button", { name: "Edit feedback" }));
    const editor = screen.getByRole("textbox", { name: editCommentLabel });
    fireEvent.change(editor, { target: { value: "Use a stronger contrast" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() =>
      expect(collection.update).toHaveBeenCalledWith("feedback-1", "Use a stronger contrast", 2),
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete feedback" }));
    expect(collection.remove).toHaveBeenCalledWith("feedback-1", 2);

    fireEvent.click(screen.getByRole("button", { name: "Clear all" }));
    expect(screen.getByText("Remove all pending preview feedback from this task?")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Clear all" }));
    await waitFor(() => expect(collection.clear).toHaveBeenCalledWith(3));
  });

  it("retains an edit when the server reports a conflict", async () => {
    const collection = controller({
      update: vi.fn().mockResolvedValue(null),
      mutationError: "The feedback changed.",
    });
    render(<PreviewFeedbackCollection collection={collection} touch={false} />);

    fireEvent.click(screen.getByRole("button", { name: "Edit feedback" }));
    const editor = screen.getByRole("textbox", { name: editCommentLabel });
    fireEvent.change(editor, { target: { value: "Keep my typed change" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(collection.update).toHaveBeenCalled());
    expect(
      (screen.getByRole("textbox", { name: editCommentLabel }) as HTMLTextAreaElement).value,
    ).toBe("Keep my typed change");
    expect(screen.getByRole("alert").textContent).toContain("The feedback could not be saved.");
  });

  it("renders the authorized screenshot content for a persisted item", () => {
    const screenshotItem: TaskPreviewFeedback = {
      ...item,
      id: "feedback-screenshot-1",
      kind: "screenshot",
      screenshot_attachment_id: "attachment-screenshot-1",
      screenshot_attachment: {
        attachment_id: "attachment-screenshot-1",
        name: "preview.png",
        mime_type: "image/png",
        kind: "image",
        delivery_mode: "prompt",
        size_bytes: 128,
        state: "claimed",
      },
    };
    const collection = controller({
      items: [screenshotItem],
      snapshot: { ...snapshot, items: [screenshotItem] },
    });
    render(<PreviewFeedbackCollection collection={collection} touch={false} />);

    expect(screen.getByRole("img", { name: "Screenshot preview" }).getAttribute("src")).toBe(
      "/api/v1/attachments/attachment-screenshot-1/content",
    );
  });

  it("shows loading errors and the empty collection state", () => {
    const collection = controller({
      items: [],
      snapshot: { task_id: "task-1", revision: 4, items: [] },
      loadError: "Could not load feedback.",
    });
    render(<PreviewFeedbackCollection collection={collection} touch={true} />);

    expect(screen.getByRole("alert").textContent).toContain(
      "The pending preview feedback could not be loaded.",
    );
    expect(screen.getByText("No pending preview feedback.")).toBeTruthy();
  });
});

describe("PreviewFeedbackCollectionSurface", () => {
  it("opens the desktop Popover from its task-scoped trigger", () => {
    const collection = controller();
    render(
      <PreviewFeedbackCollectionSurface
        taskId="task-1"
        collection={collection}
        open={false}
        onOpenChange={vi.fn()}
        showTrigger
      />,
    );

    expect(screen.getByRole("button", { name: "1 preview feedback items" })).toBeTruthy();
  });

  it("uses the touch Drawer and keeps the trigger at a touch-sized target", () => {
    touchState.enabled = true;
    const collection = controller();
    render(
      <PreviewFeedbackCollectionSurface
        taskId="task-1"
        collection={collection}
        open
        onOpenChange={vi.fn()}
        showTrigger
      />,
    );

    const trigger = screen.getByRole("button", { name: "1 preview feedback items" });
    expect(trigger.className).toContain("h-11");
    expect(screen.getByTestId("preview-feedback-collection-drawer")).toBeTruthy();
  });
});

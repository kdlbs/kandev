import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueuedGhostMessage } from "./queued-ghost-message";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=";

const ATTACHMENT_1_ALT = "Attachment 1";
const OPEN_ATTACHMENT_1_LABEL = "Open Attachment 1";
const FULL_SIZE_ATTACHMENT_1_ALT = "Full size Attachment 1";

function entry(overrides: Partial<QueuedMessage> = {}): QueuedMessage {
  return {
    id: "q-1",
    session_id: "sess-1",
    task_id: "task-1",
    content: "hello",
    plan_mode: false,
    queued_at: "2026-05-18T00:00:00Z",
    queued_by: "user-1",
    ...overrides,
  };
}

describe("QueuedGhostMessage workflow badge", () => {
  it("renders workflow metadata as a workflow step badge", () => {
    render(
      <QueuedGhostMessage
        entry={entry({
          queued_by: "workflow",
          metadata: {
            workflow_message: true,
            workflow_step_name: "In Progress",
            workflow_step_color: "bg-green-500",
          },
        })}
        canEdit={false}
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );

    expect(screen.getByTestId("workflow-message-badge").textContent).toContain("In Progress");
    expect(screen.getByTestId("workflow-message-dot").className).toContain("bg-green-500");
    expect(screen.queryByTestId("sender-task-badge")).toBeNull();
  });
});

describe("QueuedGhostMessage attachment thumbnails", () => {
  it("renders an image thumbnail for image attachments", () => {
    render(
      <QueuedGhostMessage
        entry={entry({
          attachments: [{ type: "image", data: PNG_BASE64, mime_type: "image/png" }],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    const trigger = screen.getByRole("button", { name: OPEN_ATTACHMENT_1_LABEL });
    const img = trigger.querySelector("img") as HTMLImageElement;
    expect(img.src).toBe(`data:image/png;base64,${PNG_BASE64}`);
    expect(trigger.className).toContain("cursor-pointer");
  });

  it("renders a file chip for non-image (resource) attachments", () => {
    render(
      <QueuedGhostMessage
        entry={entry({
          content: "",
          attachments: [{ type: "resource", data: "ZmlsZQ==", mime_type: "text/plain" }],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    expect(screen.getByText("Attachment")).toBeTruthy();
  });

  it("renders image thumbnails as accessible dialog triggers in display mode", () => {
    render(
      <QueuedGhostMessage
        entry={entry({
          attachments: [{ type: "image", data: PNG_BASE64, mime_type: "image/png" }],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    const trigger = screen.getByRole("button", { name: OPEN_ATTACHMENT_1_LABEL });
    expect(trigger.getAttribute("type")).toBe("button");
    expect(trigger.querySelector("img")).toBeTruthy();
  });

  it("opens the image in a preview dialog when clicked in display mode", () => {
    const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
    render(
      <QueuedGhostMessage
        entry={entry({
          attachments: [{ type: "image", data: PNG_BASE64, mime_type: "image/png" }],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: OPEN_ATTACHMENT_1_LABEL }));
    expect(openSpy).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByAltText(FULL_SIZE_ATTACHMENT_1_ALT).getAttribute("src")).toBe(
      `data:image/png;base64,${PNG_BASE64}`,
    );
  });

  it("renders thumbnails read-only in edit mode (no dialog trigger, no cursor-pointer)", () => {
    const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
    render(
      <QueuedGhostMessage
        entry={entry({
          attachments: [{ type: "image", data: PNG_BASE64, mime_type: "image/png" }],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    fireEvent.click(screen.getByTitle("Edit queued message"));
    const img = screen.getByAltText(ATTACHMENT_1_ALT) as HTMLImageElement;
    expect(img.className).not.toContain("cursor-pointer");
    expect(screen.queryByRole("button", { name: OPEN_ATTACHMENT_1_LABEL })).toBeNull();
    fireEvent.click(img);
    expect(openSpy).not.toHaveBeenCalled();
  });

  it("renders no thumbnail row when there are no attachments", () => {
    const { container } = render(
      <QueuedGhostMessage entry={entry()} canEdit onSave={async () => {}} onRemove={() => {}} />,
    );
    expect(container.querySelector("img")).toBeNull();
  });
});

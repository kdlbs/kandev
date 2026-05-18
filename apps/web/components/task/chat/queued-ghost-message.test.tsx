import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueuedGhostMessage } from "./queued-ghost-message";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

afterEach(() => {
  cleanup();
});

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=";

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
    const img = screen.getByAltText("Attachment 1") as HTMLImageElement;
    expect(img.src).toBe(`data:image/png;base64,${PNG_BASE64}`);
    expect(img.className).toContain("cursor-pointer");
  });

  it("renders a file chip for non-image (resource) attachments", () => {
    render(
      <QueuedGhostMessage
        entry={entry({
          content: "",
          attachments: [
            {
              type: "resource",
              data: "ZmlsZQ==",
              mime_type: "text/plain",
            } as QueuedMessage["attachments"] extends Array<infer T> | undefined ? T : never,
          ],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    expect(screen.getByText("Attachment")).toBeTruthy();
  });

  it("opens the image in a new window when clicked in display mode", () => {
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
    fireEvent.click(screen.getByAltText("Attachment 1"));
    expect(openSpy).toHaveBeenCalledOnce();
    openSpy.mockRestore();
  });

  it("renders thumbnails read-only in edit mode (no click handler, no cursor-pointer)", () => {
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
    const img = screen.getByAltText("Attachment 1") as HTMLImageElement;
    expect(img.className).not.toContain("cursor-pointer");
    fireEvent.click(img);
    expect(openSpy).not.toHaveBeenCalled();
    openSpy.mockRestore();
  });

  it("renders no thumbnail row when there are no attachments", () => {
    const { container } = render(
      <QueuedGhostMessage entry={entry()} canEdit onSave={async () => {}} onRemove={() => {}} />,
    );
    expect(container.querySelector("img")).toBeNull();
  });
});

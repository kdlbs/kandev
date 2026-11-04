import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ClarificationAnswerAttachments } from "./clarification-answer-attachments";
import type { ClarificationAnswerAttachmentController } from "./use-clarification-answer-attachments";
import type { FileAttachment } from "./file-attachment";

const TRIGGER_TESTID = "clarification-attachment-trigger";

const responsive = vi.hoisted(() => ({ isFinePointer: true, isMobile: false }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

function file(overrides: Partial<FileAttachment> = {}): FileAttachment {
  return {
    id: "file-1",
    attachmentId: "att-1",
    mimeType: "image/png",
    fileName: "screen.png",
    size: 3,
    isImage: true,
    deliveryMode: "path",
    uploadStatus: "ready",
    ...overrides,
  };
}

function controller(files: FileAttachment[]): ClarificationAnswerAttachmentController {
  return {
    filesFor: () => files,
    addFiles: vi.fn(),
    remove: vi.fn(),
    retry: vi.fn(),
    readyAttachments: () => [],
    clearAll: vi.fn(),
    totalCount: files.length,
    failedCount: files.filter((entry) => entry.uploadStatus === "failed").length,
    uploading: files.some((entry) => entry.uploadStatus === "uploading"),
    allReady: files.every((entry) => entry.uploadStatus === "ready"),
  };
}

beforeEach(() => {
  responsive.isFinePointer = true;
  responsive.isMobile = false;
});

afterEach(cleanup);

describe("ClarificationAnswerAttachments", () => {
  it("shows the file count on the paperclip and opens the manager on demand", () => {
    render(
      <ClarificationAnswerAttachments
        questionId="q1"
        controller={controller([file()])}
        variant="input"
      />,
    );

    const trigger = screen.getByTestId(TRIGGER_TESTID);
    expect(screen.getByTestId("clarification-attachment-badge").textContent).toBe("1");
    fireEvent.click(trigger);

    expect(screen.getByTestId("clarification-attachment-manager")).toBeTruthy();
    expect(screen.getByText("screen.png")).toBeTruthy();
    expect(screen.getByTestId("clarification-attachment-add")).toBeTruthy();
  });

  it("removes one file without changing its ready siblings", () => {
    const remove = vi.fn();
    const value = controller([
      file(),
      file({ id: "file-2", attachmentId: "att-2", fileName: "log.txt", isImage: false }),
    ]);
    value.remove = remove;
    render(<ClarificationAnswerAttachments questionId="q1" controller={value} variant="input" />);

    fireEvent.click(screen.getByTestId(TRIGGER_TESTID));
    const removeButtons = screen.getAllByTestId("clarification-attachment-remove");
    expect(removeButtons).toHaveLength(2);
    fireEvent.click(removeButtons[0]);
    expect(remove).toHaveBeenCalledWith("q1", "file-1");
  });

  it("exposes a failed state on the paperclip without another row", () => {
    render(
      <ClarificationAnswerAttachments
        questionId="q1"
        controller={controller([file({ uploadStatus: "failed" })])}
        variant="header"
      />,
    );
    const badge = screen.getByTestId("clarification-attachment-badge");
    expect(badge.getAttribute("data-state")).toBe("failed");
  });

  it("uses phone touch sizing and the drawer on a narrow fine-pointer viewport", () => {
    responsive.isFinePointer = true;
    responsive.isMobile = true;
    render(
      <ClarificationAnswerAttachments
        questionId="q1"
        controller={controller([file()])}
        variant="input"
      />,
    );
    const trigger = screen.getByTestId(TRIGGER_TESTID);
    expect(trigger.className).toContain("h-11");
    expect(trigger.className).toContain("w-11");
    fireEvent.click(trigger);
    expect(screen.getByTestId("clarification-attachment-manager")).toBeTruthy();
    expect(screen.getByTestId("clarification-attachment-add").className).toContain("min-h-11");
    expect(screen.getByTestId("clarification-attachment-remove").className).toContain("h-11");
  });

  it("keeps fine-pointer desktop controls compact", () => {
    render(
      <ClarificationAnswerAttachments
        questionId="q1"
        controller={controller([file()])}
        variant="input"
      />,
    );
    const trigger = screen.getByTestId(TRIGGER_TESTID);
    expect(trigger.className).toContain("h-7");
    fireEvent.click(trigger);
    expect(screen.getByTestId("clarification-attachment-add").className).toContain("px-2");
    expect(screen.getByTestId("clarification-attachment-remove").className).toContain("h-7");
  });
});

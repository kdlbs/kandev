import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderUserMessageBody } from "./user-message-body";

const { triggerFileDownload } = vi.hoisted(() => ({ triggerFileDownload: vi.fn() }));

vi.mock("@/lib/utils/file-download", () => ({ triggerFileDownload }));

afterEach(() => {
  cleanup();
  triggerFileDownload.mockReset();
});

const oversizedLog = Array.from(
  { length: 3921 },
  (_, index) =>
    `2026-09-18T06:32:25Z INFO worker=${index} operation completed; duration=125ms status=ok`,
).join("\n");

describe("renderUserMessageBody", () => {
  it("bounds oversized user text before Markdown rendering", () => {
    const { container } = render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: oversizedLog,
          taskId: "task-1",
        })}
      </>,
    );

    expect(container.querySelectorAll("br").length).toBeLessThanOrEqual(199);
    expect(container.textContent).not.toContain("worker=3920");
  });

  it("bounds raw content and downloads the complete raw source", () => {
    const rawContent = oversizedLog;
    const { container } = render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: true,
          hasAttachments: false,
          content: "formatted content",
          rawContent,
          taskId: "task-1",
        })}
      </>,
    );

    expect(container.textContent).not.toContain("worker=3920");
    fireEvent.click(screen.getByRole("button", { name: "Download full text" }));
    expect(triggerFileDownload).toHaveBeenCalledWith({
      fileName: "kandev-message.txt",
      content: rawContent,
      isBinary: false,
    });
  });

  it("bounds disclosed workflow instructions without exposing their markers", () => {
    const instructions = Array.from({ length: 240 }, (_, index) => `instruction-${index}`).join(
      "\n",
    );
    const content = [
      "## Workflow instructions",
      "",
      instructions,
      "",
      "<!-- /workflow-instructions -->",
      "",
      "After the workflow block",
    ].join("\n");

    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content,
          taskId: "task-1",
        })}
      </>,
    );

    fireEvent.click(screen.getByTestId("workflow-instructions-toggle"));

    expect(screen.queryByText("instruction-239")).toBeNull();
    expect(screen.getByText("After the workflow block")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Download full text" })).toBeTruthy();
  });

  it("shares the preview budget across text segments", () => {
    const before = Array.from({ length: 120 }, (_, index) => `before-${index}`).join("\n");
    const after = Array.from({ length: 120 }, (_, index) => `after-${index}`).join("\n");
    const content = [
      before,
      "## One-time workflow move instructions",
      "",
      "move details",
      "",
      "<!-- /one-time-workflow-move-instructions -->",
      after,
    ].join("\n");

    const { container } = render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content,
          taskId: "task-1",
        })}
      </>,
    );

    expect(container.querySelectorAll("br").length).toBeLessThanOrEqual(199);
    expect(container.textContent).not.toContain("after-119");
    expect(screen.getByTestId("bounded-message-preview-notice")).toBeTruthy();
  });
});

describe("segmented user-message downloads", () => {
  it("downloads the complete display message from a shortened text segment", () => {
    const before = Array.from({ length: 120 }, (_, index) => `before-${index}`).join("\n");
    const after = Array.from({ length: 120 }, (_, index) => `after-${index}`).join("\n");
    const content = [
      before,
      "## One-time workflow move instructions",
      "",
      "move details",
      "",
      "<!-- /one-time-workflow-move-instructions -->",
      after,
    ].join("\n");

    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content,
          taskId: "task-1",
        })}
      </>,
    );

    fireEvent.click(screen.getByTestId("bounded-message-preview-download"));

    expect(triggerFileDownload).toHaveBeenCalledWith({
      fileName: "kandev-message.txt",
      content,
      isBinary: false,
    });
  });
});

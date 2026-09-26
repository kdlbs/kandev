import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderUserMessageBody } from "./user-message-body";

const { triggerFileDownload } = vi.hoisted(() => ({
  triggerFileDownload: vi.fn(),
}));

vi.mock("@/lib/utils/file-download", () => ({ triggerFileDownload }));

afterEach(() => {
  cleanup();
  triggerFileDownload.mockReset();
});

const TAG_TESTID = "coordinator-about-tag";

describe("coordinator About-prefix tag", () => {
  it("renders the remainder as message content and the id as a tag", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About KAN-418: why is this here?",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );

    expect(screen.getByText("why is this here?")).toBeTruthy();
    const tag = screen.getByTestId(TAG_TESTID);
    expect(tag.textContent).toBe("about KAN-418");
  });

  it("renders the tag alone when the remainder is empty", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About KAN-418: ",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );

    expect(screen.getByTestId(TAG_TESTID)).toBeTruthy();
    expect(screen.queryByTestId("bounded-message-preview")).toBeNull();
  });

  it("renders verbatim when the task origin is not coordinator", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About KAN-418: why is this here?",
          taskId: "task-1",
        })}
      </>,
    );

    expect(screen.queryByTestId(TAG_TESTID)).toBeNull();
    expect(screen.getByText(/About KAN-418: why is this here\?/)).toBeTruthy();
  });

  it("splits at the id's own first ': ' when the id contains one (known limit)", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About Proposal: Rename: do the thing",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );

    expect(screen.getByTestId(TAG_TESTID).textContent).toBe("about Proposal");
    expect(screen.getByText("Rename: do the thing")).toBeTruthy();
  });

  it("downloads the full stored message, prefix included, from a shortened remainder", () => {
    const longRemainder = Array.from({ length: 240 }, (_, index) => `line-${index}`).join("\n");
    const content = `About KAN-418: ${longRemainder}`;

    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content,
          taskId: "task-1",
          taskOrigin: "coordinator",
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

describe("coordinator About-prefix tag: fallbacks", () => {
  it("renders verbatim when the content has no ': ' separator", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About the weather today",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );

    expect(screen.queryByTestId(TAG_TESTID)).toBeNull();
    expect(screen.getByText("About the weather today")).toBeTruthy();
  });

  it("renders verbatim when the id would contain a line break", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About line1\nline2: rest",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );

    expect(screen.queryByTestId(TAG_TESTID)).toBeNull();
  });
});

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

  it("renders unchanged with no tag when the matched remainder is empty (legacy form)", () => {
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

    expect(screen.queryByTestId(TAG_TESTID)).toBeNull();
    expect(screen.getByText(/About KAN-418:/)).toBeTruthy();
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

describe("coordinator About-prefix tag: referenced form", () => {
  it("renders the remainder and the id-only tag for the referenced form", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About KAN-418 [task:task-9]: why is this here?",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );

    expect(screen.getByText("why is this here?")).toBeTruthy();
    expect(screen.getByTestId(TAG_TESTID).textContent).toBe("about KAN-418");
  });

  it("matches the shortest id even when the title contains ': ', '[' and ']'", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About Fix: login [task:x]: message here",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );

    expect(screen.getByTestId(TAG_TESTID).textContent).toBe("about Fix: login");
    expect(screen.getByText("message here")).toBeTruthy();
  });

  it("is unaffected by a second bracketed reference inside the message body", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About KAN-418 [task:t-1]: message with [task:x]: embedded",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );

    expect(screen.getByTestId(TAG_TESTID).textContent).toBe("about KAN-418");
    expect(screen.getByText("message with [task:x]: embedded")).toBeTruthy();
  });

  it("renders unchanged with no tag when the referenced-form matched remainder is empty", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About KAN-418 [task:t-1]: ",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );

    expect(screen.queryByTestId(TAG_TESTID)).toBeNull();
    expect(screen.getByText(/About KAN-418 \[task:t-1\]:/)).toBeTruthy();
  });

  it("recognizes the proposal and stall kinds", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: false,
          hasAttachments: false,
          content: "About New feature [proposal:p-1]: why propose this?",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );
    expect(screen.getByTestId(TAG_TESTID).textContent).toBe("about New feature");
    expect(screen.getByText("why propose this?")).toBeTruthy();
  });
});

describe("coordinator About-prefix tag: raw view", () => {
  it("renders the raw content verbatim, without the tag, even for coordinator-origin messages", () => {
    render(
      <>
        {renderUserMessageBody({
          hasContent: true,
          showRaw: true,
          hasAttachments: false,
          content: "About KAN-418: hi",
          taskId: "task-1",
          taskOrigin: "coordinator",
        })}
      </>,
    );

    expect(screen.getByText("About KAN-418: hi")).toBeTruthy();
    expect(screen.queryByTestId(TAG_TESTID)).toBeNull();
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

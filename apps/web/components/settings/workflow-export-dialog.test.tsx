import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { i18n } from "@/lib/i18n";
import { ToastProvider } from "@/components/toast-provider";
import { WorkflowExportDialog } from "./workflow-export-dialog";

const { downloadMock, reportMock } = vi.hoisted(() => ({
  downloadMock: vi.fn(),
  reportMock: vi.fn(),
}));
vi.mock("@/lib/utils/file-download", () => ({ triggerFileDownload: downloadMock }));
vi.mock("@/lib/api/domains/frontend-error-log-api", () => ({
  scheduleFrontendErrorReport: reportMock,
}));

const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
const originalExecCommand = Object.getOwnPropertyDescriptor(document, "execCommand");
const DIALOG_TITLE = "Workflow export";
const FIRST_EXPORT = "name: First workflow\nsteps:\n  - name: Review\n";
const SECOND_EXPORT = "name: Second workflow\nsteps:\n  - name: Done\n";

function exportDialog(content: string) {
  return (
    <ToastProvider>
      <WorkflowExportDialog open onOpenChange={() => {}} title={DIALOG_TITLE} content={content} />
    </ToastProvider>
  );
}

async function activate(button: HTMLElement) {
  await act(async () => {
    button.focus();
    fireEvent.click(button);
  });
}

function advance(milliseconds: number) {
  act(() => vi.advanceTimersByTime(milliseconds));
}

describe("WorkflowExportDialog clipboard feedback", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("en");
    vi.useFakeTimers();
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: vi.fn().mockReturnValue(false),
    });
  });

  afterEach(async () => {
    cleanup();
    vi.useRealTimers();
    vi.restoreAllMocks();
    if (originalClipboard) Object.defineProperty(navigator, "clipboard", originalClipboard);
    else Reflect.deleteProperty(navigator, "clipboard");
    if (originalExecCommand) Object.defineProperty(document, "execCommand", originalExecCommand);
    else Reflect.deleteProperty(document, "execCommand");
    await i18n.changeLanguage("en");
  });

  // @covers AC-UI-CLIPBOARD-FEEDBACK-001.1, AC-UI-CLIPBOARD-FEEDBACK-001.7
  it.each([
    ["en", "Copy", "Copied"],
    ["pt-pt", "Copiar", "Copiado"],
  ])(
    "keeps real localized feedback after copying a second export (%s)",
    async (locale, copyLabel, copiedLabel) => {
      await i18n.changeLanguage(locale);
      const writeText = vi.fn().mockResolvedValue(undefined);
      Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
      const { rerender } = render(exportDialog(FIRST_EXPORT));
      const dialog = screen.getByRole("dialog", { name: DIALOG_TITLE });
      expect(within(dialog).getByRole("textbox")).toHaveProperty("value", FIRST_EXPORT);
      await activate(within(dialog).getByRole("button", { name: copyLabel }));
      expect(within(dialog).getByRole("button", { name: copiedLabel })).toBeTruthy();
      advance(1000);
      rerender(exportDialog(SECOND_EXPORT));
      expect(within(dialog).getByRole("textbox")).toHaveProperty("value", SECOND_EXPORT);
      await activate(within(dialog).getByRole("button", { name: copiedLabel }));
      expect(writeText.mock.calls).toEqual([[FIRST_EXPORT], [SECOND_EXPORT]]);
      advance(1000);
      expect(within(dialog).getByRole("button", { name: copiedLabel })).toBeTruthy();
      expect(within(dialog).getByRole("textbox")).toHaveProperty("value", SECOND_EXPORT);
      advance(999);
      expect(within(dialog).getByRole("button", { name: copiedLabel })).toBeTruthy();
      advance(1);
      expect(within(dialog).getByRole("button", { name: copyLabel })).toBeTruthy();
    },
  );

  // @covers AC-UI-CLIPBOARD-FEEDBACK-001.7
  it("copies actual selected fallback text inside Radix and restores button focus", async () => {
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: undefined });
    const copiedValues: string[] = [];
    const execCommand = vi.fn((command: string) => {
      expect(command).toBe("copy");
      const selected = document.activeElement;
      expect(selected).toBeInstanceOf(HTMLTextAreaElement);
      const textarea = selected as HTMLTextAreaElement;
      expect(textarea.closest('[role="dialog"]')).toBe(screen.getByRole("dialog"));
      expect(textarea.selectionStart).toBe(0);
      expect(textarea.selectionEnd).toBe(FIRST_EXPORT.length);
      copiedValues.push(textarea.value);
      return true;
    });
    Object.defineProperty(document, "execCommand", { configurable: true, value: execCommand });
    render(exportDialog(FIRST_EXPORT));
    const dialog = screen.getByRole("dialog", { name: DIALOG_TITLE });
    const button = within(dialog).getByRole("button", { name: "Copy" });
    await activate(button);
    expect(copiedValues).toEqual([FIRST_EXPORT]);
    expect(document.activeElement).toBe(button);
    expect(within(dialog).getAllByRole("textbox")).toHaveLength(1);
    expect(within(dialog).getByRole("textbox")).toHaveProperty("value", FIRST_EXPORT);
    expect(within(dialog).getByRole("button", { name: "Copied" })).toBeTruthy();
    advance(2000);
    expect(within(dialog).getByRole("button", { name: "Copy" })).toBeTruthy();
  });

  // @covers AC-UI-CLIPBOARD-FEEDBACK-001.4
  it("keeps the real copy action and export usable when both transports fail", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("permission denied"));
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(exportDialog(FIRST_EXPORT));
    const dialog = screen.getByRole("dialog", { name: DIALOG_TITLE });
    await activate(within(dialog).getByRole("button", { name: "Copy" }));
    expect(writeText).toHaveBeenCalledWith(FIRST_EXPORT);
    expect(document.execCommand).toHaveBeenCalledWith("copy");
    expect(errorSpy).toHaveBeenCalledWith("Failed to copy to clipboard");
    expect(within(dialog).getByRole("button", { name: "Copy" })).toBeTruthy();
    expect(within(dialog).getByRole("textbox")).toHaveProperty("value", FIRST_EXPORT);
  });
});

describe("WorkflowExportDialog download", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("en");
  });
  afterEach(() => {
    cleanup();
    downloadMock.mockReset();
    reportMock.mockReset();
  });

  // @covers AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.1
  it("offers download for the displayed workflow export", async () => {
    await i18n.changeLanguage("en");
    render(exportDialog("name: Café\nsteps:\n  - name: 审查\n"));
    const dialog = screen.getByRole("dialog", { name: DIALOG_TITLE });
    expect(within(dialog).getByRole("button", { name: "Download" })).toBeTruthy();
  });

  // @covers AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.1, AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.5
  it("downloads the current unmodified text after a rerender", async () => {
    const first = "name: Café\nsteps:\n  - name: 审查\n";
    const second = "name: 雪 🌍\n\n  ending: yes\n";
    const { rerender } = render(exportDialog(first));
    const dialog = screen.getByRole("dialog", { name: DIALOG_TITLE });
    await activate(within(dialog).getByRole("button", { name: "Download" }));
    rerender(exportDialog(second));
    await activate(within(dialog).getByRole("button", { name: "Download" }));
    expect(downloadMock.mock.calls).toEqual([
      [{ fileName: "workflows.yaml", content: first, isBinary: false }],
      [{ fileName: "workflows.yaml", content: second, isBinary: false }],
    ]);
    expect(within(dialog).getByRole("textbox")).toHaveProperty("value", second);
    expect(within(dialog).getByRole("button", { name: "Copy" })).toBeTruthy();
  });

  // @covers AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.2
  it.each(["", " \n\t "])("disables download for blank text", (content) => {
    render(exportDialog(content));
    const button = screen.getByRole("button", { name: "Download" });
    expect(button).toHaveProperty("disabled", true);
    fireEvent.click(button);
    expect(downloadMock).not.toHaveBeenCalled();
  });

  // @covers AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.3
  it("shows local feedback and allows retry after a helper failure", async () => {
    downloadMock.mockImplementationOnce(() => {
      throw new Error("download failed");
    });
    render(exportDialog(FIRST_EXPORT));
    const dialog = screen.getByRole("dialog", { name: DIALOG_TITLE });
    const button = within(dialog).getByRole("button", { name: "Download" });
    await activate(button);
    expect(screen.getByTestId("toast-message").textContent).toContain(
      i18n.t("task:failedToDownloadFile"),
    );
    expect(reportMock).not.toHaveBeenCalled();
    expect(within(dialog).getByRole("textbox")).toHaveProperty("value", FIRST_EXPORT);
    await activate(button);
    expect(downloadMock).toHaveBeenCalledTimes(2);
    expect(downloadMock).toHaveBeenLastCalledWith({
      fileName: "workflows.yaml",
      content: FIRST_EXPORT,
      isBinary: false,
    });
  });

  // @covers AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.5
  it("retains the Close action", () => {
    const onOpenChange = vi.fn();
    render(
      <ToastProvider>
        <WorkflowExportDialog
          open
          onOpenChange={onOpenChange}
          title={DIALOG_TITLE}
          content={FIRST_EXPORT}
        />
      </ToastProvider>,
    );
    fireEvent.click(
      within(screen.getByRole("dialog")).getAllByRole("button", { name: "Close" })[0],
    );
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

import { createRef, StrictMode, type ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ToastProvider } from "@/components/toast-provider";
import { MAX_FILES, MAX_TOTAL_SIZE, processFile } from "@/components/task/chat/file-attachment";
import { formatBytes } from "@/lib/utils/format-bytes";
import { TaskFormInputs } from "./task-create-dialog-selectors";
import type { TaskFormInputsHandle } from "./task-create-dialog-types";

const TOAST_MESSAGE_TEST_ID = "toast-message";

vi.mock("@/components/task/chat/file-attachment", async () => {
  const actual = await vi.importActual<typeof import("@/components/task/chat/file-attachment")>(
    "@/components/task/chat/file-attachment",
  );
  return { ...actual, processFile: vi.fn() };
});

// Capture the props (notably `onTranscript` / `onAutoSend`) that
// TaskFormInputs hands the voice button so we can drive transcripts
// without instantiating the real VoiceInputButton (which subscribes to
// the user-settings store and instantiates voice engines).
type VoiceProps = {
  onTranscript: (text: string) => void;
  onAutoSend?: () => void;
  disabled?: boolean;
};
const voiceCalls: VoiceProps[] = [];
vi.mock("@/components/task/chat/voice-input-button", () => ({
  VoiceInputButton: (props: VoiceProps) => {
    voiceCalls.push(props);
    return <button type="button" data-testid="voice-input-button" />;
  },
}));

// Inert mention popover — the real hook installs a `keydown` listener that
// drains React's event queue across re-renders and adds noise to assertions.
vi.mock("@/hooks/use-task-create-prompt-mention", () => ({
  useTaskCreatePromptMention: () => ({
    isOpen: false,
    isLoading: false,
    position: null,
    items: [],
    query: "",
    selectedIndex: 0,
    handleChange: (_: string) => {},
    handleKeyDown: (_: React.KeyboardEvent) => {},
    handleSelect: () => {},
    closeMenu: () => {},
    setSelectedIndex: () => {},
  }),
}));

afterEach(() => {
  cleanup();
  voiceCalls.length = 0;
  vi.restoreAllMocks();
  vi.mocked(processFile).mockReset();
});

function lastVoiceProps(): VoiceProps {
  const last = voiceCalls.at(-1);
  if (!last) throw new Error("VoiceInputButton was not rendered");
  return last;
}

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <ToastProvider>
      <TooltipProvider>{children}</TooltipProvider>
    </ToastProvider>
  );
}

function renderTaskFormInputs(initial: string, strict = false) {
  const ref = createRef<TaskFormInputsHandle>();
  const form = (
    <TaskFormInputs
      isSessionMode={false}
      autoFocus={false}
      initialDescription={initial}
      onDescriptionChange={() => {}}
      onKeyDown={() => {}}
      descriptionValueRef={ref}
    />
  );
  const utils = render(strict ? <StrictMode>{form}</StrictMode> : form, { wrapper: Wrapper });
  const textarea = screen.getByTestId("task-description-input") as HTMLTextAreaElement;
  return { ...utils, textarea, ref };
}

describe("TaskFormInputs voice-input wiring — rendering", () => {
  it("renders the voice button inside the prompt toolbar", () => {
    renderTaskFormInputs("");
    expect(screen.getByTestId("voice-input-button")).toBeTruthy();
  });

  it("renders the voice button in session mode too", () => {
    const ref = createRef<TaskFormInputsHandle>();
    render(
      <TaskFormInputs
        isSessionMode
        autoFocus={false}
        initialDescription=""
        onDescriptionChange={() => {}}
        onKeyDown={() => {}}
        descriptionValueRef={ref}
      />,
      { wrapper: Wrapper },
    );
    expect(screen.getByTestId("voice-input-button")).toBeTruthy();
    expect(lastVoiceProps()).toBeTruthy();
  });

  it("forwards onVoiceAutoSend to the voice button", () => {
    const onVoiceAutoSend = vi.fn();
    const ref = createRef<TaskFormInputsHandle>();
    render(
      <TaskFormInputs
        isSessionMode={false}
        autoFocus={false}
        initialDescription=""
        onDescriptionChange={() => {}}
        onKeyDown={() => {}}
        descriptionValueRef={ref}
        onVoiceAutoSend={onVoiceAutoSend}
      />,
      { wrapper: Wrapper },
    );

    const { onAutoSend } = lastVoiceProps();
    onAutoSend?.();
    expect(onVoiceAutoSend).toHaveBeenCalledTimes(1);
  });

  it("disables the voice button when the form is disabled", () => {
    const ref = createRef<TaskFormInputsHandle>();
    render(
      <TaskFormInputs
        isSessionMode={false}
        autoFocus={false}
        initialDescription=""
        onDescriptionChange={() => {}}
        onKeyDown={() => {}}
        descriptionValueRef={ref}
        disabled
      />,
      { wrapper: Wrapper },
    );

    expect(lastVoiceProps().disabled).toBe(true);
  });
});

describe("TaskFormInputs voice-input wiring — at-cursor splice", () => {
  it("splices the transcript at the caret with a leading space after a word", () => {
    const { textarea } = renderTaskFormInputs("hello world");
    textarea.focus();
    textarea.setSelectionRange(5, 5);

    act(() => lastVoiceProps().onTranscript("there"));

    expect(textarea.value).toBe("hello there world");
    expect(textarea.selectionStart).toBe(11);
    expect(textarea.selectionEnd).toBe(11);
  });

  it("inserts the transcript without a leading space when the caret follows whitespace", () => {
    const { textarea } = renderTaskFormInputs("hello ");
    textarea.focus();
    textarea.setSelectionRange(6, 6);

    act(() => lastVoiceProps().onTranscript("world"));

    expect(textarea.value).toBe("hello world");
    expect(textarea.selectionStart).toBe(11);
  });

  it("replaces selected text with the transcript", () => {
    const { textarea } = renderTaskFormInputs("hello world");
    textarea.focus();
    textarea.setSelectionRange(6, 11);

    act(() => lastVoiceProps().onTranscript("there"));

    expect(textarea.value).toBe("hello there");
  });

  it("ignores empty / whitespace-only transcripts", () => {
    const { textarea } = renderTaskFormInputs("hello");
    textarea.focus();
    textarea.setSelectionRange(5, 5);

    act(() => lastVoiceProps().onTranscript("   "));

    expect(textarea.value).toBe("hello");
  });

  it("inserts the transcript into a multi-line description at the line caret", () => {
    const { textarea } = renderTaskFormInputs("line one\nline two");
    textarea.focus();
    // Caret right after "line one" on the first line — char-before is "e",
    // non-whitespace, so a leading space is prepended.
    textarea.setSelectionRange(8, 8);

    act(() => lastVoiceProps().onTranscript("added"));

    expect(textarea.value).toBe("line one added\nline two");
    expect(textarea.selectionStart).toBe(14);
  });

  it("preserves internal newlines from the transcript", () => {
    const { textarea } = renderTaskFormInputs("");
    textarea.focus();
    textarea.setSelectionRange(0, 0);

    act(() => lastVoiceProps().onTranscript("first\nsecond"));

    expect(textarea.value).toBe("first\nsecond");
  });

  it("treats existing tabs / newlines before the caret as whitespace (no extra space)", () => {
    const { textarea } = renderTaskFormInputs("line\n");
    textarea.focus();
    textarea.setSelectionRange(5, 5);

    act(() => lastVoiceProps().onTranscript("two"));

    expect(textarea.value).toBe("line\ntwo");
  });
});

describe("TaskFormInputs attachment feedback", () => {
  it("shows one count-limit toast when Strict Mode replays an attachment-limit update", async () => {
    vi.mocked(processFile).mockImplementation(async (file) => ({
      id: file.name,
      data: "",
      mimeType: file.type,
      fileName: file.name,
      size: file.size,
      isImage: false,
      deliveryMode: "path",
    }));
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const { textarea } = renderTaskFormInputs("", true);
    const files = Array.from(
      { length: MAX_FILES + 1 },
      (_, index) => new File(["file"], `attachment-${index}.txt`, { type: "text/plain" }),
    );

    fireEvent.paste(textarea, {
      clipboardData: {
        files,
        items: files.map((file) => ({ kind: "file", type: file.type, getAsFile: () => file })),
        getData: () => "",
      },
    });

    const warning = await screen.findByTestId(TOAST_MESSAGE_TEST_ID);
    expect(warning.textContent).toContain("Attachment limit reached");
    expect(warning.textContent).toContain(`You can attach up to ${MAX_FILES} files.`);
    expect(screen.getAllByTestId(TOAST_MESSAGE_TEST_ID)).toHaveLength(1);
    expect(warn).not.toHaveBeenCalled();
  });

  it("shows a total-size toast when pasted attachments exceed the aggregate limit", async () => {
    vi.mocked(processFile).mockImplementation(async (file) => ({
      id: file.name,
      data: "",
      mimeType: file.type,
      fileName: file.name,
      size: file.size,
      isImage: false,
      deliveryMode: "path",
    }));
    const { textarea } = renderTaskFormInputs("");
    const fileSize = MAX_TOTAL_SIZE / 3 + 1;
    const files = Array.from({ length: 3 }, (_, index) => {
      const file = new File(["attachment"], `attachment-${index}.txt`, { type: "text/plain" });
      Object.defineProperty(file, "size", { value: fileSize });
      return file;
    });

    fireEvent.paste(textarea, {
      clipboardData: {
        files,
        items: files.map((file) => ({ kind: "file", type: file.type, getAsFile: () => file })),
        getData: () => "",
      },
    });

    const warning = await screen.findByTestId(TOAST_MESSAGE_TEST_ID);
    expect(warning.textContent).toContain("Attachment limit reached");
    expect(warning.textContent).toContain(
      `Attachments can total up to ${formatBytes(MAX_TOTAL_SIZE)}.`,
    );
  });

  it("warns when a pasted image exceeds the attachment limit", async () => {
    const { textarea } = renderTaskFormInputs("");
    const image = new File(["image"], "copied-image.png", { type: "image/png" });
    Object.defineProperty(image, "size", { value: 14 * 1024 * 1024 });

    fireEvent.paste(textarea, {
      clipboardData: {
        files: [image],
        items: [{ kind: "file", type: image.type, getAsFile: () => image }],
        getData: () => "",
      },
    });

    const warning = await screen.findByTestId(TOAST_MESSAGE_TEST_ID);
    expect(warning.textContent).toContain("Attachment is too large");
    expect(warning.textContent).toContain(
      "copied-image.png is 14.0 MB. The maximum file size is 10.0 MB.",
    );
  });

  it("warns when Chrome exposes a copied image without readable file data", async () => {
    const { textarea } = renderTaskFormInputs("");

    fireEvent.paste(textarea, {
      clipboardData: {
        files: [],
        items: [{ kind: "string", type: "text/html" }],
        getData: (type: string) =>
          type === "text/html"
            ? '<meta charset="utf-8"><img src="https://images.example.test/copied-image.png">'
            : "",
      },
    });

    const warning = await screen.findByTestId(TOAST_MESSAGE_TEST_ID);
    expect(warning.textContent).toContain("Pasted image couldn’t be attached");
    expect(warning.textContent).toContain(
      "The browser didn’t provide image data. Save the image, then attach the file instead.",
    );
  });
});

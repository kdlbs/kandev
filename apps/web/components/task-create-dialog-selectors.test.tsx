import { createRef, type ReactNode } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskFormInputs } from "./task-create-dialog-selectors";
import type { TaskFormInputsHandle } from "./task-create-dialog-types";

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
});

function lastVoiceProps(): VoiceProps {
  const last = voiceCalls.at(-1);
  if (!last) throw new Error("VoiceInputButton was not rendered");
  return last;
}

function Wrapper({ children }: { children: ReactNode }) {
  return <TooltipProvider>{children}</TooltipProvider>;
}

function renderTaskFormInputs(initial: string) {
  const ref = createRef<TaskFormInputsHandle>();
  const utils = render(
    <TaskFormInputs
      isSessionMode={false}
      autoFocus={false}
      initialDescription={initial}
      onDescriptionChange={() => {}}
      onKeyDown={() => {}}
      descriptionValueRef={ref}
    />,
    { wrapper: Wrapper },
  );
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

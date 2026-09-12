import { useRef, useState } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createPortal } from "react-dom";
import {
  ComposerDisclosureContext,
  ComposerDisclosureRegion,
  useComposerActivity,
  useComposerFocus,
} from "./composer-disclosure";
import { useComposerDisclosure, type ComposerActivity } from "./use-composer-disclosure";
import type { TipTapInputHandle } from "./tiptap-input";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function Activity({ activity }: { activity: ComposerActivity }) {
  useComposerActivity(activity);
  return null;
}

function Editor() {
  const element = useRef<HTMLTextAreaElement>(null);
  const input = useRef({ focus: () => element.current?.focus() } as TipTapInputHandle);
  const focus = useComposerFocus(input);
  return (
    <>
      <button onClick={focus}>Explicit focus</button>
      <ComposerDisclosureRegion>
        <textarea ref={element} aria-label="Draft" />
      </ComposerDisclosureRegion>
    </>
  );
}

function Fixture({ activity, mounted = true }: { activity?: ComposerActivity; mounted?: boolean }) {
  const disclosure = useComposerDisclosure({ enabled: true, sessionId: "a" });
  const [portal, setPortal] = useState(false);
  return (
    <ComposerDisclosureContext value={disclosure}>
      <section onFocusCapture={() => disclosure.focus()} onBlurCapture={disclosure.blur}>
        {mounted && activity && <Activity activity={activity} />}
        <output>{String(disclosure.expanded)}</output>
        <Editor />
        <button onClick={() => setPortal(true)}>Open owned portal</button>
        {portal && createPortal(<input aria-label="Owned portal" />, document.body)}
      </section>
    </ComposerDisclosureContext>
  );
}

describe("composer disclosure ownership", () => {
  // @covers AC-UI-THREADS-DECK-005.4/.10
  it("releases an owner's hold when it unmounts without replacing the editor", () => {
    vi.useFakeTimers();
    const view = render(<Fixture activity={{ busy: true }} />);
    const editor = screen.getByLabelText("Draft");
    fireEvent.change(editor, { target: { value: "preserved" } });
    expect(screen.getByRole("status").textContent).toBe("true");
    view.rerender(<Fixture mounted={false} />);
    act(() => vi.advanceTimersByTime(300));
    expect(screen.getByRole("status").textContent).toBe("false");
    expect(screen.getByLabelText("Draft")).toBe(editor);
    expect((editor as HTMLTextAreaElement).value).toBe("preserved");
  });

  // @covers AC-UI-THREADS-DECK-005.1/.3
  it("keeps hidden controls inert and reveals before an explicit native focus", () => {
    render(<Fixture />);
    const editor = screen.getByLabelText("Draft");
    expect(editor.closest("[inert]")).not.toBeNull();
    fireEvent.click(screen.getByText("Explicit focus"));
    expect(editor.closest("[inert]")).toBeNull();
    expect(document.activeElement).toBe(editor);
  });

  it("counts focus in a React-owned portal after the pointer leaves the tile", () => {
    vi.useFakeTimers();
    render(<Fixture />);
    fireEvent.click(screen.getByText("Open owned portal"));
    fireEvent.focus(screen.getByLabelText("Owned portal"));
    act(() => vi.advanceTimersByTime(1000));
    expect(screen.getByRole("status").textContent).toBe("true");
    fireEvent.blur(screen.getByLabelText("Owned portal"));
    act(() => vi.advanceTimersByTime(300));
    expect(screen.getByRole("status").textContent).toBe("false");
  });
});

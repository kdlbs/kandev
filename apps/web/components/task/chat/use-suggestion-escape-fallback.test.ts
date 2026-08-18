import { cleanup, fireEvent, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createElement, createRef, type RefObject } from "react";
import { useSuggestionEscapeFallback } from "./use-suggestion-escape-fallback";
import {
  ClarificationEscapeGuardProvider,
  type ClarificationEscapeGuardRegistry,
  type ClarificationEscapePredicate,
} from "@/hooks/use-clarification-escape-guard";

afterEach(cleanup);

function renderFallback(overrides: {
  isSuggestionMenuOpen: boolean;
  mentionMenuOpen?: boolean;
  containerRef?: RefObject<HTMLElement | null>;
}) {
  const closeMentionMenu = vi.fn();
  const closeSlashMenu = vi.fn();
  const closeEntityReferenceMenu = vi.fn();
  const containerRef = overrides.containerRef ?? createRef<HTMLElement>();
  const { rerender } = renderHook(
    (props: { isSuggestionMenuOpen: boolean; mentionMenuOpen: boolean }) =>
      useSuggestionEscapeFallback({
        isSuggestionMenuOpen: props.isSuggestionMenuOpen,
        mentionMenuOpen: props.mentionMenuOpen,
        slashMenuOpen: false,
        entityReferenceMenuOpen: false,
        closeMentionMenu,
        closeSlashMenu,
        closeEntityReferenceMenu,
        containerRef,
      }),
    {
      initialProps: {
        isSuggestionMenuOpen: overrides.isSuggestionMenuOpen,
        mentionMenuOpen: overrides.mentionMenuOpen ?? false,
      },
    },
  );
  return { closeMentionMenu, closeSlashMenu, closeEntityReferenceMenu, rerender, containerRef };
}

describe("useSuggestionEscapeFallback", () => {
  it("does nothing on Escape while no suggestion menu is open", () => {
    const { closeMentionMenu, closeSlashMenu, closeEntityReferenceMenu } = renderFallback({
      isSuggestionMenuOpen: false,
    });

    fireEvent.keyDown(document, { key: "Escape" });

    expect(closeMentionMenu).not.toHaveBeenCalled();
    expect(closeSlashMenu).not.toHaveBeenCalled();
    expect(closeEntityReferenceMenu).not.toHaveBeenCalled();
  });

  it("closes the open menu and claims the event when the keydown targets this composer's own container", () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const containerRef = { current: container };

    const { closeMentionMenu } = renderFallback({
      isSuggestionMenuOpen: true,
      mentionMenuOpen: true,
      containerRef,
    });

    const event = fireEvent.keyDown(container, {
      key: "Escape",
      cancelable: true,
      bubbles: true,
    });

    expect(closeMentionMenu).toHaveBeenCalledTimes(1);
    // fireEvent's return value is the pre-dispatch continue flag: `false` means
    // something called preventDefault() during dispatch.
    expect(event).toBe(false);

    document.body.removeChild(container);
  });

  it("ignores non-Escape keys", () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const containerRef = { current: container };

    const { closeMentionMenu } = renderFallback({
      isSuggestionMenuOpen: true,
      mentionMenuOpen: true,
      containerRef,
    });

    fireEvent.keyDown(container, { key: "Enter", bubbles: true });

    expect(closeMentionMenu).not.toHaveBeenCalled();
    document.body.removeChild(container);
  });

  /**
   * On the main task chat panel there is no Radix capture-phase interceptor,
   * so ProseMirror's Suggestion plugin runs its own onKeyDown and calls
   * event.stopPropagation() on the keydown at the editor's own bubble-phase
   * listener -- see tiptap-suggestion.tsx. That halts propagation before the
   * event reaches `document`, so this hook's document-bubble listener never
   * fires there, regardless of registration order (bubble phase always visits
   * the DOM tree from target to document). This test reproduces that halt with
   * a real DOM listener attached between the target and document, using
   * jsdom's genuine event propagation rather than a synthetic stand-in.
   */
  it("stays inert when an earlier bubble-phase listener stops propagation, as on the main task chat panel", () => {
    const editor = document.createElement("div");
    document.body.appendChild(editor);
    const stopAtEditor = (event: KeyboardEvent) => {
      if (event.key === "Escape") event.stopPropagation();
    };
    editor.addEventListener("keydown", stopAtEditor);
    const containerRef = { current: editor };

    const { closeMentionMenu } = renderFallback({
      isSuggestionMenuOpen: true,
      mentionMenuOpen: true,
      containerRef,
    });

    fireEvent.keyDown(editor, { key: "Escape", bubbles: true, cancelable: true });

    expect(closeMentionMenu).not.toHaveBeenCalled();

    editor.removeEventListener("keydown", stopAtEditor);
    document.body.removeChild(editor);
  });

  it("removes its listener when the suggestion menu closes", () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const containerRef = { current: container };

    const { closeMentionMenu, rerender } = renderFallback({
      isSuggestionMenuOpen: true,
      mentionMenuOpen: true,
      containerRef,
    });

    rerender({ isSuggestionMenuOpen: false, mentionMenuOpen: false });
    fireEvent.keyDown(container, { key: "Escape", bubbles: true, cancelable: true });

    expect(closeMentionMenu).not.toHaveBeenCalled();
    document.body.removeChild(container);
  });

  /**
   * F10 repro: two composer instances mounted at once (the main task chat
   * panel, backgrounded, plus Quick Chat's own composer in front of it), each
   * with a suggestion menu open. Escape is dispatched from inside the SECOND
   * (foreground) composer's own container. Without an ownership check the
   * first-registered hook instance (background) would see the event on
   * `document`, close its own (wrong) menu, and stopPropagation() before the
   * second instance's listener ever ran -- leaving the actually-open,
   * user-visible popup untouched. Real jsdom propagation, not a synthetic
   * stand-in for either instance.
   */
  it("only the composer instance that owns the keydown's target closes its menu (F10: two composers open at once)", () => {
    const backgroundContainer = document.createElement("div");
    const foregroundContainer = document.createElement("div");
    document.body.appendChild(backgroundContainer);
    document.body.appendChild(foregroundContainer);

    const background = renderFallback({
      isSuggestionMenuOpen: true,
      mentionMenuOpen: true,
      containerRef: { current: backgroundContainer },
    });
    const foreground = renderFallback({
      isSuggestionMenuOpen: true,
      mentionMenuOpen: true,
      containerRef: { current: foregroundContainer },
    });

    fireEvent.keyDown(foregroundContainer, { key: "Escape", bubbles: true, cancelable: true });

    expect(foreground.closeMentionMenu).toHaveBeenCalledTimes(1);
    expect(background.closeMentionMenu).not.toHaveBeenCalled();

    document.body.removeChild(backgroundContainer);
    document.body.removeChild(foregroundContainer);
  });
});

describe("useSuggestionEscapeFallback guard predicate ownership", () => {
  /**
   * The capture-phase guard predicate (registered via
   * useClarificationEscapeGuard) must claim an Escape only when this
   * composer's own container actually owns it -- exactly like the
   * bubble-phase listener in the suite above. A predicate that claimed every
   * Escape unconditionally would leave the dialog open (and the key
   * otherwise unhandled) once focus left the composer while a menu was still
   * open, since the bubble listener already refuses to act for an unowned
   * target.
   */
  it("claims Escape only when the container owns the target", () => {
    const container = document.createElement("div");
    const outsider = document.createElement("div");
    document.body.appendChild(container);
    document.body.appendChild(outsider);
    const containerRef = { current: container };

    let registeredPredicate: ClarificationEscapePredicate | null = null;
    const registry: ClarificationEscapeGuardRegistry = {
      register: (_id, predicate) => {
        registeredPredicate = predicate;
      },
      unregister: () => {
        registeredPredicate = null;
      },
    };

    renderHook(
      () =>
        useSuggestionEscapeFallback({
          isSuggestionMenuOpen: true,
          mentionMenuOpen: true,
          slashMenuOpen: false,
          entityReferenceMenuOpen: false,
          closeMentionMenu: vi.fn(),
          closeSlashMenu: vi.fn(),
          closeEntityReferenceMenu: vi.fn(),
          containerRef,
        }),
      {
        wrapper: ({ children }) =>
          createElement(ClarificationEscapeGuardProvider, { value: registry }, children),
      },
    );

    expect(registeredPredicate).not.toBeNull();
    const ownedEvent = new KeyboardEvent("keydown", { key: "Escape" });
    Object.defineProperty(ownedEvent, "target", { value: container });
    const unownedEvent = new KeyboardEvent("keydown", { key: "Escape" });
    Object.defineProperty(unownedEvent, "target", { value: outsider });

    expect(registeredPredicate!(ownedEvent)).toBe(true);
    expect(registeredPredicate!(unownedEvent)).toBe(false);

    document.body.removeChild(container);
    document.body.removeChild(outsider);
  });
});

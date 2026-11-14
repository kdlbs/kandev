import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ClarificationEscapeGuardProvider,
  type ClarificationEscapePredicate,
} from "@/hooks/use-clarification-escape-guard";
import { usePromptSuggestionEscape } from "./use-prompt-suggestion-escape";

function setup(isVisible: () => boolean) {
  const container = document.createElement("div");
  const input = document.createElement("div");
  input.tabIndex = 0;
  container.appendChild(input);
  document.body.appendChild(container);
  const predicates = new Map<string, ClarificationEscapePredicate>();
  const registry = {
    register: (id: string, p: ClarificationEscapePredicate) => predicates.set(id, p),
    unregister: (id: string) => predicates.delete(id),
  };
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(ClarificationEscapeGuardProvider, { value: registry }, children);
  const onDismiss = vi.fn();
  const hook = renderHook(
    ({ active }: { active: boolean }) =>
      usePromptSuggestionEscape({
        active,
        isVisible,
        containerRef: { current: container },
        onDismiss,
      }),
    { wrapper, initialProps: { active: true } },
  );
  return { container, input, predicates, onDismiss, hook };
}

afterEach(() => {
  document.body.innerHTML = "";
});

function escapeFrom(target: HTMLElement) {
  const event = new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true });
  target.dispatchEvent(event);
  return event;
}

// @covers AC-UI-PROMPT-SUGGEST-004.6
describe("usePromptSuggestionEscape", () => {
  it("dismisses a visible suggestion and claims the Escape from an owning composer", () => {
    const { input, predicates, onDismiss } = setup(() => true);
    input.focus();
    const event = escapeFrom(input);
    expect(onDismiss).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
    expect([...predicates.values()].some((p) => p(event))).toBe(true);
  });

  it("leaves Escape alone when the ghost text is hidden or the key comes from elsewhere", () => {
    const hidden = setup(() => false);
    hidden.input.focus();
    escapeFrom(hidden.input);
    expect(hidden.onDismiss).not.toHaveBeenCalled();
    expect([...hidden.predicates.values()].some((p) => p(new KeyboardEvent("keydown")))).toBe(
      false,
    );

    const outside = document.createElement("button");
    document.body.appendChild(outside);
    const owned = setup(() => true);
    outside.focus();
    escapeFrom(outside);
    expect(owned.onDismiss).not.toHaveBeenCalled();
  });

  it("stops listening when inactive", () => {
    const { input, onDismiss, hook, predicates } = setup(() => true);
    hook.rerender({ active: false });
    input.focus();
    escapeFrom(input);
    expect(onDismiss).not.toHaveBeenCalled();
    expect(predicates.size).toBe(0);
  });
});

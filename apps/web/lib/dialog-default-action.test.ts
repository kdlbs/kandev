import { describe, it, expect, vi } from "vitest";
import {
  resolveDialogDefaultAction,
  handleDialogDefaultActionKeyDown,
} from "@kandev/ui/lib/dialog-default-action";
import type React from "react";

/**
 * Unit coverage for the "Enter confirms the semantic action" resolver + keydown
 * handler used by the base Dialog / AlertDialog components. Builds real DOM
 * subtrees mirroring the markup the base components render, then asserts which
 * button Enter would activate.
 */
function content(html: string): HTMLElement {
  const el = document.createElement("div");
  el.innerHTML = html;
  return el;
}

describe("resolveDialogDefaultAction", () => {
  it("returns the AlertDialog action button", () => {
    const el = content(`
      <button data-slot="alert-dialog-cancel" data-variant="outline">Cancel</button>
      <button data-slot="alert-dialog-action" data-variant="destructive" id="go">Delete</button>
    `);
    expect(resolveDialogDefaultAction(el)?.id).toBe("go");
  });

  it("returns null when the AlertDialog action is disabled", () => {
    const el = content(`
      <button data-slot="alert-dialog-cancel">Cancel</button>
      <button data-slot="alert-dialog-action" disabled>Delete</button>
    `);
    expect(resolveDialogDefaultAction(el)).toBeNull();
  });

  it("returns null when the AlertDialog action is aria-disabled", () => {
    const el = content(`
      <button data-slot="alert-dialog-action" aria-disabled="true">Delete</button>
    `);
    expect(resolveDialogDefaultAction(el)).toBeNull();
  });

  it("resolves the single primary button in a generic dialog footer", () => {
    const el = content(`
      <div data-slot="dialog-footer">
        <button data-variant="outline">Cancel</button>
        <button data-variant="destructive" id="del">Delete Repository</button>
      </div>
    `);
    expect(resolveDialogDefaultAction(el)?.id).toBe("del");
  });

  it("resolves a submit button in the footer", () => {
    const el = content(`
      <div data-slot="dialog-footer">
        <button data-variant="outline">Cancel</button>
        <button type="submit" data-variant="default" id="save">Save</button>
      </div>
    `);
    expect(resolveDialogDefaultAction(el)?.id).toBe("save");
  });

  it("prefers an explicit data-dialog-default-action marker", () => {
    const el = content(`
      <div data-slot="dialog-footer">
        <button data-variant="destructive">Delete & Archive</button>
        <button data-variant="destructive" data-dialog-default-action id="pick">Migrate & Delete</button>
      </div>
    `);
    expect(resolveDialogDefaultAction(el)?.id).toBe("pick");
  });

  it("returns null when the explicit data-dialog-default-action button is disabled", () => {
    const el = content(`
      <div data-slot="dialog-footer">
        <button>Cancel</button>
        <button data-dialog-default-action disabled>Marked but disabled</button>
        <button type="submit" id="submit">Submit</button>
      </div>
    `);
    expect(resolveDialogDefaultAction(el)).toBeNull();
  });

  it("returns null when a generic footer has several primary actions (no guessing)", () => {
    const el = content(`
      <div data-slot="dialog-footer">
        <button data-variant="outline">Cancel</button>
        <button data-variant="destructive">Delete & Archive</button>
        <button data-variant="destructive">Migrate & Delete</button>
      </div>
    `);
    expect(resolveDialogDefaultAction(el)).toBeNull();
  });

  it("returns null when a second primary action is only temporarily disabled", () => {
    // "Migrate & Delete" is disabled pending a selection; the resolver must
    // treat the footer as ambiguous rather than firing the enabled destructive.
    const el = content(`
      <div data-slot="dialog-footer">
        <button data-variant="outline">Cancel</button>
        <button data-variant="destructive" id="archive">Delete & Archive</button>
        <button data-variant="destructive" disabled>Migrate & Delete</button>
      </div>
    `);
    expect(resolveDialogDefaultAction(el)).toBeNull();
  });

  it("ignores disabled primary buttons when they are the only candidate", () => {
    const el = content(`
      <div data-slot="dialog-footer">
        <button data-variant="outline">Cancel</button>
        <button data-variant="destructive" disabled>Delete</button>
      </div>
    `);
    expect(resolveDialogDefaultAction(el)).toBeNull();
  });

  it("returns null when there is no footer and no action marker", () => {
    const el = content(`<p>Just some informational content</p>`);
    expect(resolveDialogDefaultAction(el)).toBeNull();
  });
});

type KeyEventOverrides = Partial<React.KeyboardEvent<HTMLElement>>;

function keyEvent(currentTarget: HTMLElement, overrides: KeyEventOverrides = {}) {
  const preventDefault = vi.fn();
  const event = {
    key: "Enter",
    shiftKey: false,
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    repeat: false,
    defaultPrevented: false,
    currentTarget,
    target: currentTarget,
    nativeEvent: { isComposing: false } as KeyboardEvent,
    preventDefault,
    ...overrides,
  } as unknown as React.KeyboardEvent<HTMLElement>;
  return { event, preventDefault };
}

function alertContent() {
  const el = content(
    `<button data-slot="alert-dialog-cancel" data-variant="outline" id="cancel">Cancel</button>
     <button data-slot="alert-dialog-action" data-variant="destructive" id="action">Delete</button>`,
  );
  const action = el.querySelector<HTMLElement>("#action")!;
  const cancel = el.querySelector<HTMLElement>("#cancel")!;
  const click = vi.fn();
  action.addEventListener("click", click);
  return { el, action, cancel, click };
}

describe("handleDialogDefaultActionKeyDown — activation and guards", () => {
  it("clicks the semantic action and prevents default on plain Enter", () => {
    const { el, click } = alertContent();
    const { event, preventDefault } = keyEvent(el);
    handleDialogDefaultActionKeyDown(event);
    expect(click).toHaveBeenCalledTimes(1);
    expect(preventDefault).toHaveBeenCalledTimes(1);
  });

  it("fires the primary even when the Cancel button holds focus (Radix default)", () => {
    const { el, cancel, click } = alertContent();
    const { event } = keyEvent(el, { target: cancel });
    handleDialogDefaultActionKeyDown(event);
    expect(click).toHaveBeenCalledTimes(1);
  });

  it("ignores Shift+Enter so it can be used for newlines", () => {
    const { el, click } = alertContent();
    const { event, preventDefault } = keyEvent(el, { shiftKey: true });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
    expect(preventDefault).not.toHaveBeenCalled();
  });

  it.each(["metaKey", "ctrlKey", "altKey"] as const)("ignores %s+Enter", (mod) => {
    const { el, click } = alertContent();
    const { event } = keyEvent(el, { [mod]: true });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
  });

  it("ignores auto-repeat Enter (key held down)", () => {
    const { el, click } = alertContent();
    const { event } = keyEvent(el, { repeat: true });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
  });

  it("ignores Enter during IME composition", () => {
    const { el, click } = alertContent();
    const { event } = keyEvent(el, { nativeEvent: { isComposing: true } as KeyboardEvent });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
  });

  it("ignores Enter originating from a textarea", () => {
    const { el, click } = alertContent();
    const textarea = document.createElement("textarea");
    const { event } = keyEvent(el, { target: textarea });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
  });

  it("ignores Enter originating from a contenteditable element", () => {
    const { el, click } = alertContent();
    const div = document.createElement("div");
    div.contentEditable = "true";
    const { event } = keyEvent(el, { target: div });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
  });

  it("fires the primary when Enter comes from a plain single-line input", () => {
    const { el, click } = alertContent();
    const input = document.createElement("input");
    const { event } = keyEvent(el, { target: input });
    handleDialogDefaultActionKeyDown(event);
    expect(click).toHaveBeenCalledTimes(1);
  });

  it("ignores non-Enter keys", () => {
    const { el, click } = alertContent();
    const { event } = keyEvent(el, { key: "a" });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
  });

  it("does nothing when the event was already handled (defaultPrevented)", () => {
    const { el, click } = alertContent();
    const { event } = keyEvent(el, { defaultPrevented: true });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
  });

  it("does not prevent default when there is no resolvable action", () => {
    const el = content(`<p>Nothing actionable here</p>`);
    const { event, preventDefault } = keyEvent(el);
    handleDialogDefaultActionKeyDown(event);
    expect(preventDefault).not.toHaveBeenCalled();
  });
});

describe("handleDialogDefaultActionKeyDown — focused-control handling", () => {
  it("lets a focused non-cancel action button keep its own Enter behavior", () => {
    const { el, click } = alertContent();
    const validate = document.createElement("button");
    validate.setAttribute("data-variant", "default");
    const { event, preventDefault } = keyEvent(el, { target: validate });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
    expect(preventDefault).not.toHaveBeenCalled();
  });

  it("lets a focused <select> keep its own Enter behavior", () => {
    const { el, click } = alertContent();
    const select = document.createElement("select");
    const { event } = keyEvent(el, { target: select });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
  });

  it("does not double-fire when the primary action itself holds focus", () => {
    const { el, action, click } = alertContent();
    const { event, preventDefault } = keyEvent(el, { target: action });
    handleDialogDefaultActionKeyDown(event);
    // Native activation already clicks the focused button; the handler must not.
    expect(click).not.toHaveBeenCalled();
    expect(preventDefault).not.toHaveBeenCalled();
  });
});

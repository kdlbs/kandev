import { describe, it, expect, vi } from "vitest";
import {
  resolveDialogDefaultAction,
  handleDialogDefaultActionKeyDown,
} from "@kandev/ui/lib/dialog-default-action";
import type React from "react";

/**
 * Unit coverage for the "Enter confirms the semantic action" resolver used by
 * the base Dialog / AlertDialog components. Builds real DOM subtrees mirroring
 * the markup the base components render, then asserts which button Enter would
 * activate.
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

  it("prefers a submit button in the footer", () => {
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

  it("ignores disabled primary buttons in the footer", () => {
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
    defaultPrevented: false,
    currentTarget,
    target: currentTarget,
    nativeEvent: { isComposing: false } as KeyboardEvent,
    preventDefault,
    ...overrides,
  } as unknown as React.KeyboardEvent<HTMLElement>;
  return { event, preventDefault };
}

describe("handleDialogDefaultActionKeyDown", () => {
  function alertContent() {
    const el = content(
      `<button data-slot="alert-dialog-action" data-variant="destructive">Delete</button>`,
    );
    const button = el.querySelector<HTMLElement>("button");
    const click = vi.fn();
    button?.addEventListener("click", click);
    return { el, click };
  }

  it("clicks the semantic action and prevents default on plain Enter", () => {
    const { el, click } = alertContent();
    const { event, preventDefault } = keyEvent(el);
    handleDialogDefaultActionKeyDown(event);
    expect(click).toHaveBeenCalledTimes(1);
    expect(preventDefault).toHaveBeenCalledTimes(1);
  });

  it("ignores Shift+Enter so it can be used for newlines", () => {
    const { el, click } = alertContent();
    const { event, preventDefault } = keyEvent(el, { shiftKey: true });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
    expect(preventDefault).not.toHaveBeenCalled();
  });

  it("ignores Enter originating from a textarea", () => {
    const { el, click } = alertContent();
    const textarea = document.createElement("textarea");
    const { event } = keyEvent(el, { target: textarea });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
  });

  it("does nothing when the event was already handled (defaultPrevented)", () => {
    const { el, click } = alertContent();
    const { event } = keyEvent(el, { defaultPrevented: true });
    handleDialogDefaultActionKeyDown(event);
    expect(click).not.toHaveBeenCalled();
  });

  it("ignores non-Enter keys", () => {
    const { el, click } = alertContent();
    const { event } = keyEvent(el, { key: "a" });
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

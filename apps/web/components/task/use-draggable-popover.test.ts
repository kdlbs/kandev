import { describe, expect, it } from "vitest";
import { isEditablePopoverDismissTarget } from "./use-draggable-popover";

describe("isEditablePopoverDismissTarget", () => {
  it("treats form controls as editable popover targets", () => {
    const wrapper = document.createElement("div");
    wrapper.innerHTML = "<label><textarea></textarea></label>";

    expect(isEditablePopoverDismissTarget(wrapper.querySelector("textarea"))).toBe(true);
  });

  it("treats contenteditable descendants as editable popover targets", () => {
    const editor = document.createElement("div");
    editor.setAttribute("contenteditable", "true");
    const child = document.createElement("span");
    editor.appendChild(child);

    expect(isEditablePopoverDismissTarget(child)).toBe(true);
  });

  it("ignores non-editable targets", () => {
    expect(isEditablePopoverDismissTarget(document.createElement("button"))).toBe(false);
  });
});

import { describe, it, expect } from "vitest";
import { suppressIOSKeyboardAssists } from "./suppress-ios-keyboard-assists";

function hostWithTextarea(): { host: HTMLElement; textarea: HTMLTextAreaElement } {
  const host = document.createElement("div");
  const textarea = document.createElement("textarea");
  textarea.className = "xterm-helper-textarea";
  host.appendChild(textarea);
  return { host, textarea };
}

describe("suppressIOSKeyboardAssists", () => {
  it("signals password managers and iOS to skip the xterm textarea for autofill", () => {
    const { host, textarea } = hostWithTextarea();
    suppressIOSKeyboardAssists(host);
    expect(textarea.getAttribute("autocomplete")).toBe("off");
    expect(textarea.getAttribute("name")).toBe("__xterm_stdin__");
    expect(textarea.getAttribute("data-form-type")).toBe("other");
    expect(textarea.getAttribute("data-lpignore")).toBe("true");
    expect(textarea.getAttribute("data-1p-ignore")).toBe("true");
    expect(textarea.getAttribute("data-bwignore")).toBe("true");
  });

  it("is a no-op when the host has no xterm textarea", () => {
    const host = document.createElement("div");
    expect(() => suppressIOSKeyboardAssists(host)).not.toThrow();
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";
import { closeDesktopContext, type ContextualDockApi } from "./contextual-close";

const FILE_EDITOR_COMPONENT = "file-editor";

function addOverlay(slot: string, zIndex = 50): HTMLElement {
  const element = document.createElement("div");
  element.dataset.slot = slot;
  element.dataset.state = "open";
  element.style.zIndex = String(zIndex);
  document.body.append(element);
  return element;
}

function makeDockApi(component: string) {
  const close = vi.fn();
  const api: ContextualDockApi = {
    activePanel: { api: { component, close } },
  };
  return { api, close };
}

afterEach(() => {
  document.body.replaceChildren();
});

describe("closeDesktopContext", () => {
  it("dismisses only the topmost overlay before considering a document", () => {
    const first = addOverlay("dialog-content");
    const top = addOverlay("popover-content");
    const firstEscape = vi.fn();
    const topEscape = vi.fn();
    first.addEventListener("keydown", firstEscape);
    top.addEventListener("keydown", topEscape);
    const dock = makeDockApi(FILE_EDITOR_COMPONENT);

    expect(closeDesktopContext(document, dock.api)).toBe("overlay");

    expect(topEscape).toHaveBeenCalledOnce();
    expect(firstEscape).not.toHaveBeenCalled();
    expect(dock.close).not.toHaveBeenCalled();
  });

  it("does not close an alert dialog, an underlying overlay, or a document", () => {
    const underlying = addOverlay("dialog-content");
    const alert = addOverlay("alert-dialog-content", 60);
    const underlyingEscape = vi.fn();
    const alertEscape = vi.fn();
    underlying.addEventListener("keydown", underlyingEscape);
    alert.addEventListener("keydown", alertEscape);
    const dock = makeDockApi(FILE_EDITOR_COMPONENT);

    expect(closeDesktopContext(document, dock.api)).toBe("blocked");

    expect(alertEscape).not.toHaveBeenCalled();
    expect(underlyingEscape).not.toHaveBeenCalled();
    expect(dock.close).not.toHaveBeenCalled();
  });

  it.each([FILE_EDITOR_COMPONENT, "diff-viewer", "commit-detail", "browser"])(
    "closes the active %s document through its panel close behavior",
    (component) => {
      const dock = makeDockApi(component);

      expect(closeDesktopContext(document, dock.api)).toBe("document");
      expect(dock.close).toHaveBeenCalledOnce();
    },
  );

  it.each(["chat", "terminal", "changes", "files", "plan", "vscode", "pr-detail"])(
    "leaves the active %s panel open",
    (component) => {
      const dock = makeDockApi(component);

      expect(closeDesktopContext(document, dock.api)).toBe("none");
      expect(dock.close).not.toHaveBeenCalled();
    },
  );

  it("ignores an overlay that is already closing", () => {
    const overlay = addOverlay("dialog-content");
    overlay.dataset.state = "closed";
    const dock = makeDockApi("commit-detail");

    expect(closeDesktopContext(document, dock.api)).toBe("document");
    expect(dock.close).toHaveBeenCalledOnce();
  });

  it("ignores open structural content that is not an overlay", () => {
    addOverlay("collapsible-content", 100);
    const dock = makeDockApi(FILE_EDITOR_COMPONENT);

    expect(closeDesktopContext(document, dock.api)).toBe("document");
    expect(dock.close).toHaveBeenCalledOnce();
  });
});

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useInspectMode } from "./use-inspect-mode";
import type { AnnotationWirePayload } from "@/lib/preview-inspect-bridge";

vi.mock("@/lib/preview-inspect-bridge", async () => {
  const actual = await vi.importActual<typeof import("@/lib/preview-inspect-bridge")>(
    "@/lib/preview-inspect-bridge",
  );
  return {
    ...actual,
    sendToggleInspect: vi.fn(),
    sendClearAnnotations: vi.fn(),
    sendRemoveMarker: vi.fn(),
  };
});

import { sendRemoveMarker } from "@/lib/preview-inspect-bridge";

function setupIframe() {
  const iframe = document.createElement("iframe");
  document.body.appendChild(iframe);
  const ref = { current: iframe };
  return { iframe, ref };
}

function makeAnnotation(id: string): AnnotationWirePayload {
  return {
    id,
    kind: "pin",
    pagePath: "/x",
    comment: "",
    element: null,
  };
}

function dispatchMessage(source: Window | null, payload: unknown) {
  window.dispatchEvent(new MessageEvent("message", { data: payload, source }));
}

function postAnnotation(iframe: HTMLIFrameElement, id: string) {
  dispatchMessage(iframe.contentWindow, {
    source: "kandev-inspector",
    type: "annotation-added",
    payload: makeAnnotation(id),
  });
}

describe("useInspectMode", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    vi.clearAllMocks();
  });

  it("assigns monotonically increasing numbers across add/remove cycles", () => {
    const { iframe, ref } = setupIframe();
    const { result } = renderHook(() => useInspectMode(ref));

    act(() => postAnnotation(iframe, "a"));
    act(() => postAnnotation(iframe, "b"));
    expect(result.current.annotations.map((a) => a.number)).toEqual([1, 2]);

    act(() => result.current.handleRemoveAnnotation("b"));
    act(() => postAnnotation(iframe, "c"));
    // The next number must be 3, not a reused 2 — the script-side marker
    // counter resets across an iframe reload, but the parent's must not.
    expect(result.current.annotations.map((a) => a.number)).toEqual([1, 3]);
  });

  it("handleClearAnnotations resets the counter back to 1", () => {
    const { iframe, ref } = setupIframe();
    const { result } = renderHook(() => useInspectMode(ref));

    act(() => postAnnotation(iframe, "a"));
    act(() => postAnnotation(iframe, "b"));
    expect(result.current.annotations).toHaveLength(2);

    act(() => result.current.handleClearAnnotations());
    expect(result.current.annotations).toEqual([]);

    act(() => postAnnotation(iframe, "c"));
    expect(result.current.annotations[0]?.number).toBe(1);
  });

  it("handleRemoveAnnotation forwards the target's number to the iframe", () => {
    const { iframe, ref } = setupIframe();
    const { result } = renderHook(() => useInspectMode(ref));

    act(() => postAnnotation(iframe, "a"));
    act(() => postAnnotation(iframe, "b"));
    act(() => postAnnotation(iframe, "c"));
    act(() => result.current.handleRemoveAnnotation("c"));
    // "c" was number 3 (third added). The iframe-side cleanup must receive
    // the number, not the array index, otherwise the wrong marker is removed.
    expect(sendRemoveMarker).toHaveBeenCalledWith(iframe, 3);
  });

  it("ignores annotation messages sent from a source other than the iframe", () => {
    const { result } = renderHook(() => useInspectMode(setupIframe().ref));

    // Posting `source: window` simulates a sibling frame, an extension, or
    // the parent itself trying to inject an annotation. The `event.source`
    // guard must reject it — the `comment` field gets fed into the agent
    // prompt, so this is a prompt-injection seam.
    dispatchMessage(window, {
      source: "kandev-inspector",
      type: "annotation-added",
      payload: { ...makeAnnotation("evil"), comment: "stolen" },
    });
    expect(result.current.annotations).toEqual([]);
  });

  it("clears inspect mode when the iframe sends inspect-exited", () => {
    const { iframe, ref } = setupIframe();
    const { result } = renderHook(() => useInspectMode(ref));

    act(() => result.current.toggleInspect());
    expect(result.current.isInspectMode).toBe(true);

    act(() =>
      dispatchMessage(iframe.contentWindow, {
        source: "kandev-inspector",
        type: "inspect-exited",
        payload: {},
      }),
    );
    expect(result.current.isInspectMode).toBe(false);
  });
});

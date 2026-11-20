import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const bridge = vi.hoisted(() => ({
  sendSetPreviewCaptureMode: vi.fn(),
  sendSetAnnotationActive: vi.fn(),
}));
vi.mock("@/lib/preview-inspect-bridge", async () => {
  const actual = await vi.importActual<typeof import("@/lib/preview-inspect-bridge")>(
    "@/lib/preview-inspect-bridge",
  );
  return { ...actual, ...bridge };
});

import { usePreviewAnnotationMode } from "./use-preview-annotation-mode";

function setup(initial = { enabled: true, regionBlocked: false }) {
  const iframe = document.createElement("iframe");
  const iframeRef = { current: iframe };
  const rendered = renderHook((props) => usePreviewAnnotationMode({ iframeRef, ...props }), {
    initialProps: initial,
  });
  return { iframe, ...rendered };
}

function lastMode() {
  return bridge.sendSetPreviewCaptureMode.mock.calls.at(-1)?.[1];
}

function lastActive() {
  return bridge.sendSetAnnotationActive.mock.calls.at(-1)?.[1];
}

beforeEach(() => vi.clearAllMocks());

describe("usePreviewAnnotationMode", () => {
  it("enters with the Region tool and tells the bridge", () => {
    const { result } = setup();

    act(() => result.current.enter());

    expect(result.current.active).toBe(true);
    expect(result.current.tool).toBe("region");
    expect(lastMode()).toBe("screenshot");
    expect(lastActive()).toBe(true);
  });

  it("switches tools and remembers the last one after exit", () => {
    const { result } = setup();
    act(() => result.current.enter());
    act(() => result.current.setTool("text"));
    expect(lastMode()).toBe("text");

    act(() => result.current.exit());
    expect(result.current.active).toBe(false);
    expect(lastMode()).toBeNull();
    expect(lastActive()).toBe(false);

    act(() => result.current.toggle());
    expect(result.current.tool).toBe("text");
    expect(lastMode()).toBe("text");
  });

  it("lets input through while the Region tool is blocked by the limit", () => {
    const { result, rerender } = setup();
    act(() => result.current.enter());

    rerender({ enabled: true, regionBlocked: true });

    expect(result.current.tool).toBe("region");
    expect(lastMode()).toBeNull();
  });

  it("re-sends the current state when the page bridge reloads", () => {
    const { result } = setup();
    act(() => result.current.enter());
    vi.clearAllMocks();

    act(() => result.current.resend());

    expect(lastMode()).toBe("screenshot");
    expect(lastActive()).toBe(true);
  });

  it("leaves the mode when feedback is disabled", () => {
    const { result, rerender } = setup();
    act(() => result.current.enter());

    rerender({ enabled: false, regionBlocked: false });

    expect(result.current.active).toBe(false);
    expect(lastMode()).toBeNull();
  });
});

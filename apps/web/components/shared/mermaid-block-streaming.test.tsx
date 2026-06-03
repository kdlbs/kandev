/**
 * Regression tests for streaming-induced "Failed to render diagram" errors.
 *
 * When an assistant message containing a ```mermaid block streams in chunk by
 * chunk, MermaidBlock used to fire mermaid.render for every partial chunk and
 * toast on every parse error. Once it landed in the error branch, a later
 * successful render could not clear the error because the success path was
 * gated on a containerRef that had been unmounted by the error early-return.
 *
 * These tests pin down the new behaviour: render is debounced so streaming
 * chunks coalesce into a single attempt, and a later successful render clears
 * an earlier transient error.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, act } from "@testing-library/react";
import { MermaidBlock } from "./mermaid-block";

const mockToast = vi.fn();

vi.mock("next-themes", () => ({
  useTheme: () => ({ resolvedTheme: "dark" }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast, updateToast: vi.fn(), dismissToast: vi.fn() }),
}));

// Track every call to mermaid.render so tests can assert how many fired and
// with what input. Resolution / rejection is decided per-call by a queue the
// tests set up before each render-driving action.
const renderCalls: string[] = [];
type NextResult = { kind: "ok"; svg: string } | { kind: "fail"; message: string };
let nextResult: NextResult = { kind: "ok", svg: "<svg data-test='ok'></svg>" };

vi.mock("mermaid", () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn(async (_id: string, code: string) => {
      renderCalls.push(code);
      if (nextResult.kind === "fail") {
        throw new Error(nextResult.message);
      }
      return { svg: nextResult.svg };
    }),
  },
}));

beforeEach(() => {
  vi.useFakeTimers();
  renderCalls.length = 0;
  mockToast.mockClear();
  nextResult = { kind: "ok", svg: "<svg data-test='ok'></svg>" };
});

afterEach(() => {
  vi.useRealTimers();
});

// Helper: advance fake timers AND flush microtasks. mermaid.render returns a
// promise, so we need both the setTimeout to fire and the resulting promise
// chain to settle before assertions.
async function flush(ms: number) {
  await act(async () => {
    vi.advanceTimersByTime(ms);
    // Yield for the promise chain inside the component.
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("MermaidBlock streaming behaviour", () => {
  it("debounces rapid prop changes to a single render of the final code", async () => {
    const { rerender } = render(<MermaidBlock code={"sequenceDiagram"} />);
    rerender(<MermaidBlock code={"sequenceDiagram\n  participant"} />);
    rerender(<MermaidBlock code={"sequenceDiagram\n  participant A"} />);
    rerender(<MermaidBlock code={"sequenceDiagram\n  participant A\n  A->>B: hi"} />);

    // Within the debounce window (< 300ms), nothing should have rendered yet.
    await flush(50);
    expect(renderCalls).toHaveLength(0);

    // After the debounce settles, exactly one render fires with the latest code.
    await flush(400);
    expect(renderCalls).toHaveLength(1);
    expect(renderCalls[0]).toContain("A->>B: hi");
    expect(mockToast).not.toHaveBeenCalled();
  });

  it("does not toast for intermediate partial chunks during streaming", async () => {
    // Simulate streaming: many partial-and-invalid chunks arriving rapidly,
    // then a final valid chunk. The component must only attempt to render
    // once the stream has settled.
    nextResult = { kind: "ok", svg: "<svg data-test='final'></svg>" };

    const { rerender } = render(<MermaidBlock code={"flowchart LR"} />);
    for (let i = 1; i <= 6; i++) {
      rerender(<MermaidBlock code={`flowchart LR\n  subgraph wave${i}`} />);
      await flush(50);
    }
    rerender(<MermaidBlock code={"flowchart LR\n  A --> B"} />);

    await flush(500);
    expect(renderCalls).toHaveLength(1);
    expect(renderCalls[0]).toBe("flowchart LR\n  A --> B");
    expect(mockToast).not.toHaveBeenCalled();
  });

  it("clears a previous transient error when a later render succeeds", async () => {
    // First attempt: fail.
    nextResult = { kind: "fail", message: "Parse error on line 3" };
    const { rerender, container } = render(<MermaidBlock code={"flowchart LR\n  subgraph"} />);
    await flush(500);
    expect(renderCalls).toHaveLength(1);
    expect(mockToast).toHaveBeenCalledOnce();
    expect(container.textContent).toContain("Failed to render diagram");

    // Second attempt with full, valid code: succeed. The error UI must clear
    // (this is the regression — previously the success path was gated on
    // containerRef which had been unmounted by the error branch).
    nextResult = { kind: "ok", svg: "<svg data-test='recovered'></svg>" };
    rerender(<MermaidBlock code={"flowchart LR\n  A --> B"} />);
    await flush(500);

    expect(renderCalls).toHaveLength(2);
    expect(container.textContent).not.toContain("Failed to render diagram");
    expect(container.innerHTML).toContain('data-test="recovered"');
  });
});

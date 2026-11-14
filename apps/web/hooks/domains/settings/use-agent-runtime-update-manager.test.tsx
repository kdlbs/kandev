import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { useAgentRuntimeUpdateManager } from "./use-agent-runtime-update-manager";

afterEach(cleanup);

describe("useAgentRuntimeUpdateManager", () => {
  it("opens one runtime directly and returns focus to its opener", () => {
    const opener = document.createElement("button");
    document.body.appendChild(opener);
    opener.focus();
    const { result } = renderHook(() => useAgentRuntimeUpdateManager());

    act(() => result.current.openRuntime("claude-acp", opener));
    expect(result.current.view).toEqual({ kind: "runtime", agentName: "claude-acp" });

    act(() => result.current.close());
    expect(result.current.view).toEqual({ kind: "closed" });
    expect(document.activeElement).toBe(opener);
    opener.remove();
  });

  it("falls back to the header button when the opener disappeared", () => {
    const opener = document.createElement("button");
    document.body.appendChild(opener);
    const header = document.createElement("button");
    document.body.appendChild(header);
    const { result } = renderHook(() => useAgentRuntimeUpdateManager());
    act(() => {
      result.current.headerButtonRef.current = header;
      result.current.openOverview(opener);
    });
    opener.remove();
    act(() => result.current.close());
    expect(document.activeElement).toBe(header);
    header.remove();
  });

  it("returns from details to the overview without closing the surface", () => {
    const { result } = renderHook(() => useAgentRuntimeUpdateManager());
    act(() => result.current.openRuntime("codex-acp"));
    act(() => result.current.back());
    expect(result.current.view).toEqual({ kind: "overview" });
  });
});

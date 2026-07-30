import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { CommandRegistryProvider, useCommandPanelOpen } from "./command-registry";

function wrapper({ children }: { children: ReactNode }) {
  return <CommandRegistryProvider>{children}</CommandRegistryProvider>;
}

afterEach(() => cleanup());

describe("command panel mode requests", () => {
  it("opens the panel in a requested mode", () => {
    const { result } = renderHook(() => useCommandPanelOpen(), { wrapper });

    act(() => result.current.openMode("search-content"));

    expect(result.current.open).toBe(true);
    expect(result.current.mode).toBe("search-content");
  });

  it("keeps direct mode changes available to the mounted panel", () => {
    const { result } = renderHook(() => useCommandPanelOpen(), { wrapper });

    act(() => result.current.setMode("search-files"));

    expect(result.current.mode).toBe("search-files");
    expect(result.current.open).toBe(false);
  });
});

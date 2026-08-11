import { createElement, type ReactNode } from "react";
import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { useChangeRequestProviderTarget } from "./use-change-request-provider-target";

afterEach(cleanup);

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

describe("useChangeRequestProviderTarget", () => {
  it("stays render-stable when the workspace has no repositories", () => {
    let renders = 0;
    const { result, rerender } = renderHook(
      () => {
        renders += 1;
        return useChangeRequestProviderTarget(null);
      },
      { wrapper },
    );

    expect(result.current).toBeNull();
    rerender();
    expect(result.current).toBeNull();
    expect(renders).toBeLessThan(5);
  });
});

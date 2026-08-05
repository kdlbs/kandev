import type { PropsWithChildren } from "react";
import { renderHook, act } from "@testing-library/react";
import { afterAll, describe, expect, it } from "vitest";

import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { activateLocale, i18n } from "@/lib/i18n";
import { useFilterValueOptions } from "./use-filter-value-options";

function wrapper() {
  return function Wrapper({ children }: PropsWithChildren) {
    return <StateProvider>{children}</StateProvider>;
  };
}

/**
 * `executorTypeOptions` labels its options through `getExecutorLabel`, which
 * reads the catalog at call time. The hook memoizes on store data, so without
 * the active language in its dependency array the labels would keep the
 * previous locale until a task snapshot happened to change — invisible to the
 * pseudo-locale, because the text IS translated, just stale.
 *
 * The store deliberately stays untouched across the switch: that is the whole
 * point. If the only thing that changed is the locale and the label follows,
 * the dependency is doing its job.
 */
describe("useFilterValueOptions — locale", () => {
  afterAll(async () => {
    if (i18n.language !== "en") await activateLocale("en");
  });

  it("recomputes executor-type labels on a locale switch with unchanged store data", async () => {
    await activateLocale("en");

    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { options: useFilterValueOptions("executorType"), store };
      },
      { wrapper: wrapper() },
    );

    act(() => {
      result.current.store.getState().hydrate({
        kanbanMulti: {
          snapshots: {
            w1: {
              workflowId: "w1",
              workflowName: "W1",
              steps: [],
              tasks: [{ id: "t1", primaryExecutorType: "local" }],
            },
          },
        },
      } as never);
    });

    const before = result.current.options;
    expect(before.map((o) => o.value)).toEqual(["local"]);
    expect(before.map((o) => o.label)).toEqual(["Local"]);

    await act(async () => {
      await activateLocale("pseudo");
    });

    const after = result.current.options;
    // Same option VALUE — it is the persisted executor type, never translated.
    expect(after.map((o) => o.value)).toEqual(["local"]);
    expect(after[0].label).not.toBe("Local");
    expect(after[0].label).toBe(i18n.t("executors:typeLocal"));
  });
});

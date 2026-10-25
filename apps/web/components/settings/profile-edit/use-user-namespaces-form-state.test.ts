import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useUserNamespacesFormState } from "./use-user-namespaces-form-state";

describe("useUserNamespacesFormState", () => {
  it("discards a changed setting back to the profile baseline", () => {
    const { result } = renderHook(() =>
      useUserNamespacesFormState({ allow_user_namespaces: "true" }),
    );

    act(() => result.current.setAllowUserNamespaces(false));
    expect(result.current.allowUserNamespaces).toBe(false);

    act(() => result.current.resetUserNamespaces());
    expect(result.current.allowUserNamespaces).toBe(true);
  });
});

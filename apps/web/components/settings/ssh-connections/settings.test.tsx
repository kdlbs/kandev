import { cleanup, renderHook, act } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useManagerNavigation } from "./use-manager-navigation";
const router = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
let search = new URLSearchParams();
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => router,
  useSearchParams: () => search,
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.7, 004.9, 004.11
it("clears the previous record's editing and access state when selecting another", () => {
  search = new URLSearchParams(
    "tab=connections&id=old&section=access&mode=edit&adopt_executor=executor&adopt_digest=digest",
  );
  const { result } = renderHook(useManagerNavigation);
  act(() => result.current.select("next"));
  const url = new URL(router.push.mock.calls[0][0], "http://localhost");
  expect(Object.fromEntries(url.searchParams)).toEqual({
    tab: "connections",
    id: "next",
    adopt_executor: "executor",
    adopt_digest: "digest",
  });
});
it("returns a newly created identity as an explicit unsaved edit", () => {
  search = new URLSearchParams(
    "tab=identities&new=1&return_identity_connection=target&return_connection=outer",
  );
  const { result } = renderHook(useManagerNavigation);
  act(() => result.current.identityCreated("new-key"));
  const url = new URL(router.push.mock.calls[0][0], "http://localhost");
  expect(Object.fromEntries(url.searchParams)).toEqual({
    tab: "connections",
    id: "target",
    return_connection: "outer",
    select_identity: "new-key",
    mode: "edit",
  });
});
it("leaves the contextual identity selection when cancelling its edit", () => {
  search = new URLSearchParams(
    "tab=connections&id=target&select_identity=new-key&mode=edit&return_connection=outer",
  );
  const { result } = renderHook(useManagerNavigation);
  act(() => result.current.cancelEdit());
  const url = new URL(router.push.mock.calls[0][0], "http://localhost");
  expect(Object.fromEntries(url.searchParams)).toEqual({
    tab: "connections",
    id: "target",
    return_connection: "outer",
  });
});

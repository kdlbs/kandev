import { cleanup, renderHook, act } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useManagerNavigation } from "./use-manager-navigation";
const testOrigin = "http://localhost";
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
  const url = new URL(router.push.mock.calls[0][0], testOrigin);
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
  const url = new URL(router.push.mock.calls[0][0], testOrigin);
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
  const url = new URL(router.push.mock.calls[0][0], testOrigin);
  expect(Object.fromEntries(url.searchParams)).toEqual({
    tab: "connections",
    id: "target",
    return_connection: "outer",
  });
});
it("opens a bastion remedy and preserves the destination and executor context", () => {
  search = new URLSearchParams("id=target&section=diagnostics&adopt_executor=executor");
  const { result } = renderHook(useManagerNavigation);
  act(() => result.current.openRemedy("access", "bastion-id", "key-id", "target"));
  const url = new URL(router.push.mock.calls[0][0], testOrigin);
  expect(Object.fromEntries(url.searchParams)).toEqual({
    tab: "connections",
    id: "bastion-id",
    section: "access",
    return_connection: "target",
    adopt_executor: "executor",
  });
});
it("opens an identity remedy with a route back to its connection", () => {
  search = new URLSearchParams("id=target&section=diagnostics&adopt_executor=executor");
  const { result } = renderHook(useManagerNavigation);
  act(() => result.current.openRemedy("identity", "target", "key-id"));
  const url = new URL(router.push.mock.calls[0][0], testOrigin);
  expect(Object.fromEntries(url.searchParams)).toEqual({
    tab: "identities",
    id: "key-id",
    return_identity_connection: "target",
    return_section: "diagnostics",
    adopt_executor: "executor",
  });
});
it("returns from a diagnostic identity remedy to Diagnostics without opening an edit", () => {
  search = new URLSearchParams(
    "tab=identities&id=key&return_identity_connection=target&return_section=diagnostics&adopt_executor=executor",
  );
  const { result } = renderHook(useManagerNavigation);
  act(() => result.current.back());
  const url = new URL(router.push.mock.calls[0][0], testOrigin);
  expect(Object.fromEntries(url.searchParams)).toEqual({
    tab: "connections",
    id: "target",
    section: "diagnostics",
    adopt_executor: "executor",
  });
});

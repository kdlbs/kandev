// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.2, AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.3
import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import {
  GitHubConnectionDraftContext,
  useConnectionFormState,
  useConnectionRequestGuard,
} from "./github-connection-draft";

afterEach(cleanup);

it("retains an App draft across overlay remounts within the same connection", () => {
  const cache = new Map<string, unknown>();
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(GitHubConnectionDraftContext.Provider, { value: cache }, children);
  const desktop = renderHook(() => useConnectionFormState("app-name", ""), { wrapper });
  act(() => desktop.result.current[1]("Enterprise automation"));
  desktop.unmount();
  const phone = renderHook(() => useConnectionFormState("app-name", ""), { wrapper });
  expect(phone.result.current[0]).toBe("Enterprise automation");
});

it("rejects callbacks from a replaced host, workspace, or dismissed form", () => {
  const { result, rerender, unmount } = renderHook(
    ({ scope }) => useConnectionRequestGuard(scope),
    { initialProps: { scope: "workspace-a:git.company.example" } },
  );
  const hostA = result.current();
  expect(hostA()).toBe(true);
  rerender({ scope: "workspace-a:other.company.example" });
  expect(hostA()).toBe(false);
  const workspaceA = result.current();
  rerender({ scope: "workspace-b:other.company.example" });
  expect(workspaceA()).toBe(false);
  const active = result.current();
  unmount();
  expect(active()).toBe(false);
});

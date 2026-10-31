import { StrictMode } from "react";
import { renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useMCPAssignmentFocus } from "./use-mcp-assignment-focus";

const route = vi.hoisted(() => ({
  search: "mcpServerId=docs&repositoryId=repo",
  replace: vi.fn(),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useSearchParams: () => new URLSearchParams(route.search),
  useRouter: () => ({ replace: route.replace }),
  usePathname: () => "/settings/workspaces/ws/repositories",
}));
const HIGHLIGHT = "data-mcp-highlighted";
const definitions = [{ id: "docs", workspace_id: "ws", enabled: true }];
function mountRows() {
  const root = document.createElement("div");
  root.innerHTML =
    '<label data-mcp-definition-id="docs" tabindex="-1"><input type="checkbox" /></label>';
  document.body.append(root);
  const row = root.firstElementChild as HTMLElement;
  return { container: { current: root }, row };
}
beforeEach(() => {
  document.body.innerHTML = "";
  window.history.replaceState(
    null,
    "",
    "/settings/workspaces/ws/repositories?mcpServerId=docs&repositoryId=repo#mcp",
  );
  route.search = "mcpServerId=docs&repositoryId=repo";
  route.replace.mockReset();
});

it("waits for an active ready owner, focuses once, and preserves selection and other URL context", () => {
  const { container, row } = mountRows();
  const { rerender } = renderHook(
    ({ active, ready }) =>
      useMCPAssignmentFocus({
        container,
        ownerKey: "repo",
        workspaceId: "ws",
        definitions,
        active,
        ready,
      }),
    { initialProps: { active: false, ready: false } },
  );
  expect(route.replace).not.toHaveBeenCalled();
  rerender({ active: true, ready: false });
  expect(route.replace).not.toHaveBeenCalled();
  rerender({ active: true, ready: true });
  expect(row.getAttribute(HIGHLIGHT)).toBe("true");
  expect(document.activeElement).toBe(row);
  expect(row.querySelector("input")?.checked).toBe(false);
  expect(route.replace).toHaveBeenCalledWith(
    "/settings/workspaces/ws/repositories?repositoryId=repo#mcp",
    { scroll: false },
  );
  rerender({ active: true, ready: true });
  expect(route.replace).toHaveBeenCalledTimes(1);
});

it.each(["missing", "disabled", "foreign", "filtered", "ambiguous"])(
  "consumes a %s hint without focusing a fallback",
  (kind) => {
    const { container, row } = mountRows();
    const catalog =
      kind === "missing"
        ? []
        : [
            {
              ...definitions[0],
              enabled: kind !== "disabled",
              workspace_id: kind === "foreign" ? "other" : "ws",
            },
          ];
    if (kind === "filtered") row.remove();
    if (kind === "ambiguous") route.search += "&mcpServerId=other";
    renderHook(() =>
      useMCPAssignmentFocus({
        container,
        ownerKey: "repo",
        workspaceId: "ws",
        definitions: catalog,
        active: true,
        ready: true,
      }),
    );
    expect(route.replace).toHaveBeenCalledTimes(1);
    expect(row.hasAttribute(HIGHLIGHT)).toBe(false);
  },
);

it("clears the previous owner's highlight and allows a fresh hint after consumption", () => {
  const { container, row } = mountRows();
  const { rerender, unmount } = renderHook(
    ({ ownerKey }) =>
      useMCPAssignmentFocus({
        container,
        ownerKey,
        workspaceId: "ws",
        definitions,
        active: true,
        ready: true,
      }),
    { initialProps: { ownerKey: "repo" } },
  );
  route.search = "repositoryId=repo";
  rerender({ ownerKey: "repo" });
  expect(row.getAttribute(HIGHLIGHT)).toBe("true");
  rerender({ ownerKey: "other" });
  expect(row.hasAttribute(HIGHLIGHT)).toBe(false);
  route.search = "repositoryId=repo&mcpServerId=docs";
  rerender({ ownerKey: "other" });
  expect(row.getAttribute(HIGHLIGHT)).toBe("true");
  expect(route.replace).toHaveBeenCalledTimes(2);
  unmount();
  expect(row.hasAttribute(HIGHLIGHT)).toBe(false);
});

it("consumes a hint once under StrictMode effect replay", () => {
  const { container, row } = mountRows();
  renderHook(
    () =>
      useMCPAssignmentFocus({
        container,
        ownerKey: "repo",
        workspaceId: "ws",
        definitions,
        active: true,
        ready: true,
      }),
    { wrapper: StrictMode },
  );
  expect(route.replace).toHaveBeenCalledTimes(1);
  expect(row.getAttribute(HIGHLIGHT)).toBe("true");
});

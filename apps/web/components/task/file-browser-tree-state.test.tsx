import { act, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import { renderSessionRead } from "@/hooks/domains/session/session-read-test-helpers";
import { useFileTreeCacheBinding, useFileTreeState } from "./file-browser-tree-state";
import type { FileTreeNode } from "@/lib/types/backend";

afterEach(() => {
  cleanup();
  setWebSocketClient(null);
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3
it("merges concurrent folder updates into both the visible and cached tree", () => {
  setWebSocketClient({ request: vi.fn() } as unknown as WebSocketClient);
  const { result, rerender } = renderSessionRead((id: string) => {
    const binding = useFileTreeCacheBinding(id, id);
    return { ...useFileTreeState(id, binding), binding };
  }, "session" as string);
  const folder = (name: string): FileTreeNode => ({ name, path: name, is_dir: true, children: [] });
  const root: FileTreeNode = {
    name: "root",
    path: "",
    is_dir: true,
    children: [folder("a"), folder("b")],
  };
  act(() => result.current.value.setTree(root));
  act(() => {
    for (const name of ["a", "b"])
      result.current.value.setTree((previous) => ({
        ...previous!,
        children: previous!.children!.map((node) =>
          node.name === name
            ? {
                ...node,
                children: [{ name: `${name}.ts`, path: `${name}/${name}.ts`, is_dir: false }],
              }
            : node,
        ),
      }));
  });
  const tree = result.current.value.tree;
  expect(tree?.children?.map((node) => node.children?.[0]?.path)).toEqual(["a/a.ts", "b/b.ts"]);
  const binding = result.current.value.binding;
  expect(binding.cache.get(binding.key)).toEqual(tree);
  rerender("other");
  expect(result.current.value.tree).toBeNull();
  rerender("session");
  expect(result.current.value.tree).toEqual(tree);
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5 AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6
it("retires a phone session-keyed tree when its canonical environment begins restoration", () => {
  setWebSocketClient({ request: vi.fn() } as unknown as WebSocketClient);
  const { result } = renderSessionRead(() => useFileTreeCacheBinding("session", "session:1:0"), {});
  const original = result.current.value;
  expect(original.isCurrent()).toBe(true);
  act(() => {
    result.current.store.getState().beginWorkspaceRestoration("task", "session", "environment");
  });
  expect(original.isCurrent()).toBe(false);
  expect(result.current.value.isCurrent()).toBe(false);
  expect(result.current.value.key).not.toBe(original.key);
});

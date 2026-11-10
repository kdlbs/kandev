import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { useRef, useState } from "react";
import type { RenameAdapter } from "@/hooks/file-rename-continuity";
import type { OpenFileTab } from "@/lib/types/backend";
import { getFileTabKey, installFileEditorTab } from "./task-center-panel-file-tabs";
import { useTabletFileRename } from "./use-tablet-file-rename";

const dirty = (path: string, repo?: string) =>
  installFileEditorTab({
    path,
    repo,
    name: path.split("/").pop()!,
    content: "local edits",
    originalContent: "disk",
    originalHash: "hash",
    isDirty: true,
  });
afterEach(cleanup);

// @covers AC-UI-FILE-EDITOR-MUTATION-002.2, AC-UI-FILE-EDITOR-MUTATION-002.3
it("retargets a tablet folder descendant and keeps unrelated paths and dirty state", () => {
  const initial = [
    dirty("src/a.txt", "web"),
    dirty("src-other/b.txt", "web"),
    dirty("src/c.txt", "api"),
  ];
  const h = renderHook(
    ({ session }: { session: string }) => {
      const ref = useRef<RenameAdapter | null>(null);
      const [tabs, setTabs] = useState<OpenFileTab[]>(initial);
      const [selected, setSelected] = useState(`file:${getFileTabKey(initial[0])}`);
      useTabletFileRename({
        adapterRef: ref,
        session,
        tabs,
        setTabs,
        leftTab: selected,
        setLeftTab: setSelected,
      });
      return { ref, tabs, selected };
    },
    { initialProps: { session: "A" } },
  );
  const operation = h.result.current.ref.current!.capture("web/src", "web/renamed");
  expect(operation.collision).toBe(false);
  act(() => operation.publish());
  expect(h.result.current.tabs[0]).toMatchObject({
    path: "renamed/a.txt",
    repo: "web",
    content: "local edits",
    originalContent: "disk",
    isDirty: true,
    instanceId: initial[0].instanceId,
  });
  expect(h.result.current.tabs.slice(1)).toEqual(initial.slice(1));
  expect(h.result.current.selected).toBe(
    `file:${getFileTabKey({ path: "renamed/a.txt", repo: "web" })}`,
  );
  const stale = h.result.current.ref.current!.capture("web/renamed", "web/again");
  h.rerender({ session: "B" });
  act(() => stale.publish());
  expect(h.result.current.tabs[0].path).toBe("renamed/a.txt");
});

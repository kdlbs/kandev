import React from "react";
import { render, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EditorOption } from "@/lib/types/http";

// vi.mock factories are registered above module initialization, so the fixtures
// they close over are hoisted too. Mutable state lives on a property so a test
// can switch it after the factory has captured the container.
const fixtures = vi.hoisted(() => ({
  editors: [
    {
      id: "vscode",
      type: "vscode",
      name: "VS Code",
      kind: "built_in",
      installed: true,
      enabled: true,
    },
    {
      id: "embedded",
      type: "internal_vscode",
      name: "VS Code (Embedded)",
      kind: "internal_vscode",
      installed: true,
      enabled: true,
    },
  ] as EditorOption[],
  embeddedVscodeSupported: false,
}));

vi.mock("@/hooks/domains/settings/use-editors", () => ({
  useEditors: () => ({ editors: fixtures.editors, loaded: true, loading: false }),
}));

vi.mock("@/hooks/domains/session/use-session-worktrees", () => ({
  useSessionWorktrees: () => [],
}));

vi.mock("@/hooks/use-open-session-in-editor", () => ({
  useOpenSessionInEditor: () => ({ open: vi.fn(), status: "idle", isLoading: false }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      userSettings: { defaultEditorId: "vscode" },
      embeddedVscodeSupport: { bySessionId: { "session-1": fixtures.embeddedVscodeSupported } },
    }),
}));

import { FileTreeEditorProvider, useFileTreeEditorActions } from "./file-tree-editor-menu";

function Probe({ onEditors }: { onEditors: (names: string[]) => void }) {
  const actions = useFileTreeEditorActions();
  onEditors((actions?.editors ?? []).map((editor) => editor.name));
  return null;
}

function renderProvider() {
  const seen: string[][] = [];
  render(
    <FileTreeEditorProvider sessionId="session-1" treeRootName="repo">
      <Probe onEditors={(names) => seen.push(names)} />
    </FileTreeEditorProvider>,
  );
  return seen[seen.length - 1];
}

afterEach(cleanup);

describe("FileTreeEditorProvider embedded-editor availability", () => {
  it("offers the embedded editor when the session's executor supports it", () => {
    fixtures.embeddedVscodeSupported = true;

    expect(renderProvider()).toEqual(["VS Code", "VS Code (Embedded)"]);
  });

  it("omits the embedded editor when the session's executor does not support it", () => {
    fixtures.embeddedVscodeSupported = false;

    expect(renderProvider()).toEqual(["VS Code"]);
  });
});

import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";

const mocks = vi.hoisted(() => ({
  client: vi.fn(),
  create: vi.fn(),
  openEditor: vi.fn(),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: mocks.client }));
vi.mock("@/lib/ws/workspace-files", () => ({ createFile: mocks.create }));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: { getState: () => ({ addFileEditorPanel: mocks.openEditor }) },
}));

import { buildWorkspaceCommands } from "./session-commands";

const translate = ((key: string) => key) as TFunction;
const filePath = "src/example.txt";
const toast = vi.fn(() => "notice");

beforeEach(() => {
  vi.clearAllMocks();
  mocks.client.mockReturnValue({});
});

async function submit() {
  const command = buildWorkspaceCommands("session-1", translate, toast).find(
    (item) => item.id === "workspace-create-file",
  );
  expect(command?.onInputSubmit).toBeDefined();
  await command!.onInputSubmit!(filePath);
}

// @covers AC-UI-PALETTE-FILE-CREATE-001.1, AC-UI-PALETTE-FILE-CREATE-001.2
describe("palette file creation", () => {
  it("opens the created file without an error", async () => {
    mocks.create.mockResolvedValue({ success: true, path: filePath });
    await submit();
    expect(mocks.create).toHaveBeenCalledWith(expect.anything(), "session-1", filePath);
    expect(mocks.openEditor).toHaveBeenCalledExactlyOnceWith(filePath, "example.txt");
    expect(toast).not.toHaveBeenCalled();
  });

  it.each([
    [
      "a rejected request",
      () => mocks.create.mockRejectedValue(new Error("already exists")),
      "already exists",
    ],
    [
      "an unsuccessful response",
      () => mocks.create.mockResolvedValue({ success: false, error: "denied" }),
      "denied",
    ],
    [
      "an unsuccessful response without detail",
      () => mocks.create.mockResolvedValue({ success: false }),
      "common:anUnknownErrorOccurred",
    ],
    [
      "a non-Error rejection",
      () => mocks.create.mockRejectedValue(null),
      "common:anUnknownErrorOccurred",
    ],
    [
      "an unavailable client",
      () => mocks.client.mockReturnValue(null),
      "common:anUnknownErrorOccurred",
    ],
  ])("reports %s and does not open an editor", async (_case, arrange, detail) => {
    arrange();
    await submit();
    expect(toast).toHaveBeenCalledExactlyOnceWith({
      title: "task:failedToCreateFile",
      description: detail,
      variant: "error",
    });
    expect(mocks.openEditor).not.toHaveBeenCalled();
  });
});

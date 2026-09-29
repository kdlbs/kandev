import { beforeEach, describe, expect, it, vi } from "vitest";

const clipboardMocks = vi.hoisted(() => ({ copyToClipboard: vi.fn() }));

vi.mock("./copy-to-clipboard", () => clipboardMocks);

import { copyPathToClipboard } from "./copy-repository-path";

describe("copyPathToClipboard", () => {
  beforeEach(() => {
    clipboardMocks.copyToClipboard.mockReset().mockResolvedValue(true);
  });

  it("rejects every C0 and DEL control character before writing to the clipboard", async () => {
    const controlCharacters = [
      ...Array.from({ length: 0x20 }, (_, codePoint) => String.fromCharCode(codePoint)),
      String.fromCharCode(0x7f),
    ];

    for (const controlCharacter of controlCharacters) {
      await expect(copyPathToClipboard(`src/file${controlCharacter}.ts`)).resolves.toBe("unsafe");
    }

    expect(clipboardMocks.copyToClipboard).not.toHaveBeenCalled();
  });

  it("copies other repository paths exactly", async () => {
    const path = "src/current-name.ts";

    await expect(copyPathToClipboard(path)).resolves.toBe("copied");

    expect(clipboardMocks.copyToClipboard).toHaveBeenCalledWith(path);
  });
});

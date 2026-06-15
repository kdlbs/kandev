import { describe, it, expect, vi } from "vitest";
import type { FileContentResponse } from "@/lib/types/backend";

vi.mock("@/lib/utils/file-diff", () => ({
  calculateHash: async (s: string) => `h:${s.length}`,
}));

import { buildFileEditorState } from "./use-file-editors";

const RESPONSE: FileContentResponse = {
  path: "src/foo.ts",
  content: "v1",
  is_binary: false,
} as FileContentResponse;

describe("buildFileEditorState", () => {
  it("carries the repo subpath so subsequent save/sync calls scope to the right repository", async () => {
    // Multi-repo open: opening foo.ts from the "enrichment-commons" repo must
    // record `repo` on the editor state, otherwise later save/sync requests
    // drop it and the backend stats the bare task root → "file not found".
    const state = await buildFileEditorState("src/foo.ts", RESPONSE, "enrichment-commons");
    expect(state.repo).toBe("enrichment-commons");
  });

  it("leaves repo undefined for single-repo tasks", async () => {
    const state = await buildFileEditorState("src/foo.ts", RESPONSE);
    expect(state.repo).toBeUndefined();
  });
});

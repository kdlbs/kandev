import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useScopedStageOperations } from "./use-scoped-stage-operations";

const FILE_PATH = "src/app.ts";

function setup() {
  const gitOps = {
    stage: vi.fn(async (_paths?: string[], _repo?: string) => "repository-stage"),
    unstage: vi.fn(async (_paths?: string[], _repo?: string) => "repository-unstage"),
  };
  const stageAll = vi.fn(async () => "stage-all");
  const stageFile = vi.fn(async (_paths: string[], _repo?: string) => "stage-file");
  const unstageAll = vi.fn(async () => "unstage-all");
  const unstageFile = vi.fn(async (_paths: string[], _repo?: string) => "unstage-file");
  const hook = renderHook(() =>
    useScopedStageOperations(gitOps, stageAll, stageFile, unstageAll, unstageFile),
  );

  return { ...hook, gitOps, stageAll, stageFile, unstageAll, unstageFile };
}

describe("useScopedStageOperations", () => {
  it("dispatches path, repository, and workspace stage scopes", async () => {
    const { result, gitOps, stageAll, stageFile } = setup();

    await act(async () => {
      await result.current.stage([FILE_PATH], "frontend");
      await result.current.stage(undefined, "backend");
      await result.current.stage();
    });

    expect(stageFile).toHaveBeenCalledWith([FILE_PATH], "frontend");
    expect(gitOps.stage).toHaveBeenCalledWith(undefined, "backend");
    expect(stageAll).toHaveBeenCalledOnce();
  });

  it("dispatches path, repository, and workspace unstage scopes", async () => {
    const { result, gitOps, unstageAll, unstageFile } = setup();

    await act(async () => {
      await result.current.unstage([FILE_PATH], "frontend");
      await result.current.unstage(undefined, "backend");
      await result.current.unstage();
    });

    expect(unstageFile).toHaveBeenCalledWith([FILE_PATH], "frontend");
    expect(gitOps.unstage).toHaveBeenCalledWith(undefined, "backend");
    expect(unstageAll).toHaveBeenCalledOnce();
  });
});

import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createExecutor,
  createExecutorProfile,
  deleteExecutor,
} from "@/lib/api/domains/settings-api";
import { createCursorCloudProfile } from "./create-cursor-cloud-profile";
vi.mock("@/lib/api/domains/settings-api", () => ({
  createExecutor: vi.fn(),
  createExecutorProfile: vi.fn(),
  deleteExecutor: vi.fn(),
}));
const payload = { name: "Cloud", config: {}, prepare_script: "", cleanup_script: "" };
describe("createCursorCloudProfile", () => {
  beforeEach(() => vi.resetAllMocks());
  it("creates the executor before saving its configured profile", async () => {
    vi.mocked(createExecutor).mockResolvedValue({
      id: "executor",
      name: "Cloud",
      type: "cursor_cloud",
      config: {},
    });
    vi.mocked(createExecutorProfile).mockResolvedValue({
      id: "profile",
      executor_id: "executor",
      ...payload,
      created_at: "now",
      updated_at: "now",
    } as Awaited<ReturnType<typeof createExecutorProfile>>);
    const result = await createCursorCloudProfile(payload);
    expect(createExecutor).toHaveBeenCalledWith({ name: "Cloud", type: "cursor_cloud" });
    expect(createExecutorProfile).toHaveBeenCalledWith("executor", payload);
    expect(result.profiles?.[0].id).toBe("profile");
    expect(deleteExecutor).not.toHaveBeenCalled();
  });
  it("removes the incomplete executor and preserves the profile failure", async () => {
    vi.mocked(createExecutor).mockResolvedValue({
      id: "executor",
      name: "Cloud",
      type: "cursor_cloud",
      config: {},
    });
    const failure = new Error("Profile save failed");
    vi.mocked(createExecutorProfile).mockRejectedValue(failure);
    vi.mocked(deleteExecutor).mockRejectedValue(new Error("Cleanup failed"));
    await expect(createCursorCloudProfile(payload)).rejects.toBe(failure);
    expect(deleteExecutor).toHaveBeenCalledWith("executor");
  });
  it("does not save a profile when executor creation fails", async () => {
    vi.mocked(createExecutor).mockRejectedValue(new Error("Executor failed"));
    await expect(createCursorCloudProfile(payload)).rejects.toThrow("Executor failed");
    expect(createExecutorProfile).not.toHaveBeenCalled();
    expect(deleteExecutor).not.toHaveBeenCalled();
  });
});

/* eslint-disable sonarjs/no-duplicate-string */
import { describe, expect, it } from "vitest";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import {
  removeExecutorFromList,
  removeExecutorProfileFromList,
  upsertExecutorInList,
  upsertExecutorProfileInList,
} from "./use-executors-query-sync";

const profileA = {
  id: "profile-a",
  executor_id: "executor-a",
  name: "Profile A",
} as ExecutorProfile;
const profileB = {
  id: "profile-b",
  executor_id: "executor-a",
  name: "Profile B",
} as ExecutorProfile;
const executorA = {
  id: "executor-a",
  name: "Executor A",
  type: "ssh",
  status: "active",
  is_system: false,
  profiles: [profileA],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
} as Executor;

describe("executor query sync helpers", () => {
  it("upserts executors without duplicating existing rows", () => {
    expect(upsertExecutorInList([executorA], { ...executorA, name: "Renamed" })).toEqual([
      expect.objectContaining({ id: "executor-a", name: "Renamed" }),
    ]);
    expect(upsertExecutorInList([], executorA)).toEqual([executorA]);
  });

  it("upserts and removes profiles inside the owning executor", () => {
    const withProfile = upsertExecutorProfileInList([executorA], "executor-a", profileB);
    expect(withProfile[0]?.profiles).toEqual([profileA, profileB]);

    const updated = upsertExecutorProfileInList(withProfile, "executor-a", {
      ...profileB,
      name: "Renamed",
    });
    expect(updated[0]?.profiles?.find((profile) => profile.id === "profile-b")?.name).toBe(
      "Renamed",
    );

    expect(removeExecutorProfileFromList(updated, "executor-a", "profile-b")[0]?.profiles).toEqual([
      profileA,
    ]);
  });

  it("removes executors by id", () => {
    expect(removeExecutorFromList([executorA], "executor-a")).toEqual([]);
  });
});

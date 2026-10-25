import { describe, expect, it } from "vitest";
import { executorOwnerChoices } from "./executor-ownership";
const people = [
  { id: "alice", name: "Alice" },
  { id: "bob", name: "Bob" },
];
const base = {
  userId: "alice",
  currentOwner: "",
  parentOwner: "",
  manager: false,
  creating: false,
  people,
};
describe("executor ownership choices", () => {
  it("allows self-owned creation and owner clearing without claiming an existing shared resource", () => {
    expect(executorOwnerChoices(base)).toEqual([""]);
    expect(executorOwnerChoices({ ...base, creating: true })).toEqual(["", "alice"]);
    expect(executorOwnerChoices({ ...base, currentOwner: "alice" })).toEqual(["", "alice"]);
  });
  it("restricts profile owners to their private parent, including manager choices", () => {
    expect(executorOwnerChoices({ ...base, manager: true, parentOwner: "bob" })).toEqual([
      "",
      "bob",
    ]);
    expect(executorOwnerChoices({ ...base, manager: true })).toEqual(["", "alice", "bob"]);
  });
});

it("requires an explicit accessible replacement after catalogue eviction", async () => {
  const { executorProfileAvailable } = await import("./executor-ownership");
  expect(executorProfileAvailable("private", [{ profiles: [{ id: "shared" }] }])).toBe(false);
  expect(executorProfileAvailable("private", [])).toBe(false);
  expect(executorProfileAvailable("shared", [{ profiles: [{ id: "shared" }] }])).toBe(true);
});

it("keeps an in-flight ownership edit when a saved baseline arrives", async () => {
  const { reconcileExecutorOwnerDraft, acceptExecutorOwnerSave } =
    await import("./executor-ownership");
  expect(reconcileExecutorOwnerDraft({ value: "bob", baseline: "alice" }, "")).toEqual({
    value: "bob",
    baseline: "",
  });
  expect(reconcileExecutorOwnerDraft({ value: "alice", baseline: "alice" }, "")).toEqual({
    value: "",
    baseline: "",
  });
  expect(acceptExecutorOwnerSave({ value: "bob", baseline: "alice" }, "")).toEqual({
    value: "bob",
    baseline: "",
  });
});

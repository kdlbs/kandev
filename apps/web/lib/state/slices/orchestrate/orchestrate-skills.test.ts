import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOrchestrateSlice } from "./orchestrate-slice";
import type { OrchestrateSlice, Skill } from "./types";

function makeStore() {
  return create<OrchestrateSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOrchestrateSlice as any)(...a) })),
  );
}

function makeSkill(overrides?: Partial<Skill>): Skill {
  return {
    id: "skill-1",
    workspaceId: "ws-1",
    name: "Test Skill",
    slug: "test-skill",
    sourceType: "inline",
    content: "# Test",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("orchestrate skill actions", () => {
  it("setSkills replaces the list", () => {
    const store = makeStore();
    const skills = [makeSkill(), makeSkill({ id: "skill-2", name: "Second" })];
    store.getState().setSkills(skills);
    expect(store.getState().orchestrate.skills).toHaveLength(2);
  });

  it("addSkill appends to the list", () => {
    const store = makeStore();
    store.getState().setSkills([makeSkill()]);
    store.getState().addSkill(makeSkill({ id: "skill-2", name: "New Skill" }));
    expect(store.getState().orchestrate.skills).toHaveLength(2);
    expect(store.getState().orchestrate.skills[1].name).toBe("New Skill");
  });

  it("updateSkill patches an existing skill", () => {
    const store = makeStore();
    store.getState().setSkills([makeSkill()]);
    store.getState().updateSkill("skill-1", { name: "Updated Name" });
    expect(store.getState().orchestrate.skills[0].name).toBe("Updated Name");
    expect(store.getState().orchestrate.skills[0].slug).toBe("test-skill");
  });

  it("updateSkill is a no-op for unknown id", () => {
    const store = makeStore();
    store.getState().setSkills([makeSkill()]);
    store.getState().updateSkill("nonexistent", { name: "Nope" });
    expect(store.getState().orchestrate.skills[0].name).toBe("Test Skill");
  });

  it("removeSkill removes by id", () => {
    const store = makeStore();
    store.getState().setSkills([makeSkill(), makeSkill({ id: "skill-2" })]);
    store.getState().removeSkill("skill-1");
    expect(store.getState().orchestrate.skills).toHaveLength(1);
    expect(store.getState().orchestrate.skills[0].id).toBe("skill-2");
  });

  it("removeSkill is a no-op for unknown id", () => {
    const store = makeStore();
    store.getState().setSkills([makeSkill()]);
    store.getState().removeSkill("nonexistent");
    expect(store.getState().orchestrate.skills).toHaveLength(1);
  });
});

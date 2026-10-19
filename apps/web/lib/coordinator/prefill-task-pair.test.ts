import { describe, expect, it } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { prefillTaskPair, type PrefillTaskPairInput } from "./prefill-task-pair";

const profile = (id: string, cli_passthrough = false) =>
  ({ id, cli_passthrough }) as AgentProfileOption;

const base: PrefillTaskPairInput = {
  workspaceDefaultAgentProfileId: "ws",
  agentProfiles: [profile("ws"), profile("own"), profile("pass", true)],
  ownAgent: "own",
  ownExecutor: "own-exec",
  touched: { agent: false, executor: false },
  current: { agent: "", executor: "" },
};

describe("prefillTaskPair", () => {
  it("uses the workspace default agent and the own executor", () => {
    expect(prefillTaskPair(base)).toEqual({ agent: "ws", executor: "own-exec" });
  });

  it("falls back to the own agent when the default is passthrough, missing or unset", () => {
    expect(prefillTaskPair({ ...base, workspaceDefaultAgentProfileId: "pass" }).agent).toBe("own");
    expect(prefillTaskPair({ ...base, workspaceDefaultAgentProfileId: "gone" }).agent).toBe("own");
    expect(prefillTaskPair({ ...base, workspaceDefaultAgentProfileId: null }).agent).toBe("own");
  });

  it("keeps the agent empty while profiles or the default are still loading", () => {
    expect(prefillTaskPair({ ...base, agentProfiles: undefined }).agent).toBe("");
    expect(prefillTaskPair({ ...base, workspaceDefaultAgentProfileId: undefined }).agent).toBe("");
  });

  it("is empty when nothing is chosen yet", () => {
    expect(
      prefillTaskPair({
        ...base,
        workspaceDefaultAgentProfileId: null,
        ownAgent: "",
        ownExecutor: "",
      }),
    ).toEqual({ agent: "", executor: "" });
  });

  it("keeps a touched field's current value and recomputes the other", () => {
    const out = prefillTaskPair({
      ...base,
      touched: { agent: true, executor: false },
      current: { agent: "mine", executor: "ignored" },
      ownExecutor: "new-exec",
    });
    expect(out).toEqual({ agent: "mine", executor: "new-exec" });
    expect(
      prefillTaskPair({
        ...base,
        touched: { agent: false, executor: true },
        current: { agent: "", executor: "mine-exec" },
      }),
    ).toEqual({ agent: "ws", executor: "mine-exec" });
  });
});

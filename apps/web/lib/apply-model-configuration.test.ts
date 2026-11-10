import { describe, expect, it, vi } from "vitest";

const HIGH = "high";
const FAST = "fast";
const PAIR_A = "pair-a";

import {
  applyModelConfiguration,
  pendingConfigurationOptions,
  type ModelConfigurationApplyRequest,
} from "@/lib/apply-model-configuration";

function request(
  overrides: Partial<ModelConfigurationApplyRequest> = {},
): ModelConfigurationApplyRequest {
  return {
    model: PAIR_A,
    configOptions: { thinking: HIGH, speed: FAST },
    applyModel: vi.fn().mockResolvedValue(undefined),
    applyOption: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

describe("applyModelConfiguration", () => {
  it("confirms the model before applying every option in stable order", async () => {
    const calls: string[] = [];
    const result = await applyModelConfiguration(
      request({
        applyModel: async (model) => {
          calls.push(`model:${model}`);
        },
        applyOption: async (optionId, value) => {
          calls.push(`${optionId}:${value}`);
        },
      }),
    );
    expect(calls).toEqual(["model:pair-a", "speed:fast", "thinking:high"]);
    expect(result).toEqual({
      status: "applied",
      model: PAIR_A,
      appliedOptions: { speed: FAST, thinking: HIGH },
    });
  });

  it("does not apply options when the model is rejected", async () => {
    const applyOption = vi.fn().mockResolvedValue(undefined);
    const result = await applyModelConfiguration(
      request({ applyModel: vi.fn().mockRejectedValue(new Error("rejected")), applyOption }),
    );
    expect(result).toEqual({ status: "model_failed", model: PAIR_A, appliedOptions: {} });
    expect(applyOption).not.toHaveBeenCalled();
  });

  it("reports only the options the provider confirmed before a later failure", async () => {
    const applyOption = vi
      .fn()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error("option rejected"));
    const result = await applyModelConfiguration(request({ applyOption }));
    expect(result).toEqual({
      status: "option_failed",
      model: PAIR_A,
      appliedOptions: { speed: FAST },
      failedOptionId: "thinking",
      failedValue: HIGH,
    });
  });
});

describe("applyModelConfiguration confirmation and retry", () => {
  it("stops confirming after the owning session or generation is replaced", async () => {
    let current = true;
    const applyOption = vi.fn(async () => {
      current = false;
    });
    const result = await applyModelConfiguration(
      request({ applyOption, isCurrent: () => current }),
    );
    expect(result.status).toBe("superseded");
    expect(applyOption).toHaveBeenCalledTimes(1);
  });

  it("cancels before any mutation when the request is already stale", async () => {
    const applyModel = vi.fn().mockResolvedValue(undefined);
    const result = await applyModelConfiguration(request({ applyModel, isCurrent: () => false }));
    expect(result).toEqual({ status: "cancelled", model: PAIR_A, appliedOptions: {} });
    expect(applyModel).not.toHaveBeenCalled();
  });

  it("reports each accepted provider mutation to the owning surface", async () => {
    const confirmed: string[] = [];
    const result = await applyModelConfiguration(
      request({
        onModelConfirmed: (model) => confirmed.push(`model:${model}`),
        onOptionConfirmed: (optionId, value) => confirmed.push(`${optionId}:${value}`),
      }),
    );
    expect(result.status).toBe("applied");
    expect(confirmed).toEqual(["model:pair-a", "speed:fast", "thinking:high"]);
  });

  it("resumes from a confirmed model and retries only the pending options", async () => {
    const applyModel = vi.fn().mockResolvedValue(undefined);
    const first = await applyModelConfiguration(
      request({
        applyOption: vi
          .fn()
          .mockResolvedValueOnce(undefined)
          .mockRejectedValueOnce(new Error("option rejected")),
      }),
    );
    expect(first.status).toBe("option_failed");
    expect(pendingConfigurationOptions(first, { speed: FAST, thinking: HIGH })).toEqual({
      thinking: HIGH,
    });

    const retry = await applyModelConfiguration(
      request({
        modelConfirmed: true,
        configOptions: pendingConfigurationOptions(first, { speed: FAST, thinking: HIGH }),
        applyModel,
      }),
    );
    expect(applyModel).not.toHaveBeenCalled();
    expect(retry).toEqual({
      status: "applied",
      model: PAIR_A,
      appliedOptions: { thinking: HIGH },
    });
  });

  it("does not report a stale mutation as confirmed", async () => {
    let current = true;
    const onModelConfirmed = vi.fn();
    const result = await applyModelConfiguration(
      request({
        applyModel: async () => {
          current = false;
        },
        isCurrent: () => current,
        onModelConfirmed,
      }),
    );
    expect(result.status).toBe("superseded");
    expect(onModelConfirmed).not.toHaveBeenCalled();
  });
});

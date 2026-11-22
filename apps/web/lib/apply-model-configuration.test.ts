import { describe, expect, it, vi } from "vitest";

const HIGH = "high";
const FAST = "fast";
const PAIR_A = "pair-a";

function confirmed(configId: string, value: string, effective = value) {
  return {
    configuration_result: {
      status: "applied",
      config_id: configId,
      requested: value,
      effective,
      confirmed: true,
      execution_id: "execution-1",
    },
  };
}

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
    applyModel: vi.fn(async (model: string) => confirmed("model", model)),
    applyOption: vi.fn(async (optionId: string, value: string) => confirmed(optionId, value)),
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
          return confirmed("model", model);
        },
        applyOption: async (optionId, value) => {
          calls.push(`${optionId}:${value}`);
          return confirmed(optionId, value);
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
      .mockResolvedValueOnce(confirmed("speed", FAST))
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
          .mockResolvedValueOnce(confirmed("speed", FAST))
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

describe("applyModelConfiguration effective values", () => {
  it("does not apply dependent options when the agent reports a different base value", async () => {
    const applyOption = vi.fn();
    const onModelConfirmed = vi.fn();
    const result = await applyModelConfiguration(
      request({
        model: "pair-b",
        applyModel: async () => confirmed("model", "pair-b", PAIR_A),
        applyOption,
        onModelConfirmed,
      }),
    );

    expect(result.status).toBe("model_clamped");
    expect(result).toMatchObject({ requested: "pair-b", effective: PAIR_A });
    expect(applyOption).not.toHaveBeenCalled();
    expect(onModelConfirmed).toHaveBeenCalledWith(PAIR_A);
  });

  it("does not treat a fulfilled setter without correlated evidence as applied", async () => {
    const applyOption = vi.fn();
    const onModelConfirmed = vi.fn();
    const result = await applyModelConfiguration(
      request({
        model: "pair-b",
        applyModel: async () => ({
          configuration_result: {
            status: "unconfirmed",
            requested: "pair-b",
            confirmed: false,
          },
        }),
        applyOption,
        onModelConfirmed,
      }),
    );

    expect(result.status).toBe("model_unconfirmed");
    expect(applyOption).not.toHaveBeenCalled();
    expect(onModelConfirmed).not.toHaveBeenCalled();
  });

  it("commits a clamped option's effective value", async () => {
    const confirmedOptions: Array<[string, string]> = [];
    const result = await applyModelConfiguration(
      request({
        configOptions: { thinking: HIGH },
        applyOption: async () => confirmed("thinking", HIGH, "medium"),
        onOptionConfirmed: (optionId, value) => confirmedOptions.push([optionId, value]),
      }),
    );

    expect(result).toMatchObject({ status: "applied", appliedOptions: { thinking: "medium" } });
    expect(confirmedOptions).toEqual([["thinking", "medium"]]);
  });

  it("keeps saved and unconfirmed option outcomes distinct", async () => {
    const saved = await applyModelConfiguration(
      request({
        configOptions: { thinking: HIGH },
        applyOption: async () => ({
          configuration_result: { status: "saved_for_launch", requested: HIGH },
        }),
      }),
    );
    const unconfirmed = await applyModelConfiguration(
      request({
        configOptions: { thinking: HIGH },
        applyOption: async () => ({
          configuration_result: { status: "unconfirmed", requested: HIGH, confirmed: false },
        }),
      }),
    );

    expect(saved).toMatchObject({
      status: "applied",
      savedOptions: ["thinking"],
      appliedOptions: {},
    });
    expect(pendingConfigurationOptions(saved, { thinking: HIGH })).toEqual({});
    expect(unconfirmed).toMatchObject({ status: "applied", unconfirmedOptions: ["thinking"] });
  });

  it("stops dependent writes when the base is saved for the next launch", async () => {
    const applyOption = vi.fn();
    const result = await applyModelConfiguration(
      request({
        applyModel: async () => ({
          configuration_result: { status: "saved_for_launch", requested: PAIR_A },
        }),
        applyOption,
      }),
    );

    expect(result.status).toBe("model_saved");
    expect(applyOption).not.toHaveBeenCalled();
  });
});

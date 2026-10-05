import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useState } from "react";
import { useOnboardingActions } from "./use-onboarding-actions";
import type { AgentSetting } from "./agent-settings";

const actionMocks = vi.hoisted(() => ({
  updateAgentProfileAction: vi.fn(),
}));

const toastMock = vi.hoisted(() => vi.fn());

vi.mock("@/app/actions/agents", () => actionMocks);
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

const TEST_AGENT_NAME = "test-agent";
const UPDATED_MODEL = "gpt-4o";

const initialSettings: Record<string, AgentSetting> = {
  [TEST_AGENT_NAME]: {
    profileId: "profile-1",
    draft: {
      model: "gpt-4",
      cli_passthrough: false,
    },
    baseline: {
      model: "gpt-4",
      cli_passthrough: false,
    },
    savedLaunchSettings: {
      env_vars: [],
      cli_flags: [],
      command_prefix: "",
    },
    dirty: false,
  },
};

beforeEach(() => {
  vi.resetAllMocks();
  actionMocks.updateAgentProfileAction.mockResolvedValue({});
});

afterEach(cleanup);

function renderActions(settings = initialSettings) {
  return renderHook(() => {
    const [step, setStep] = useState(0);
    const [agentSettings, setAgentSettings] = useState(settings);
    const actions = useOnboardingActions({
      step,
      setStep,
      onComplete: vi.fn(),
      agentSettings,
      setAgentSettings,
    });
    return { ...actions, agentSettings, step };
  });
}

it("waits for all profile writes after a partial failure and retries only the failed profile", async () => {
  const secondSetting = { ...initialSettings[TEST_AGENT_NAME], profileId: "profile-2" };
  let finishSuccessfulSave!: () => void;
  actionMocks.updateAgentProfileAction
    .mockImplementationOnce(() => new Promise<void>((resolve) => (finishSuccessfulSave = resolve)))
    .mockRejectedValueOnce(new Error("save failed"));
  const { result } = renderActions({ ...initialSettings, "second-agent": secondSetting });
  act(() => {
    result.current.updateSetting(TEST_AGENT_NAME, { model: UPDATED_MODEL });
    result.current.updateSetting("second-agent", { model: UPDATED_MODEL });
  });
  let save!: Promise<void>;
  await act(async () => {
    save = result.current.handleNext();
  });
  expect(result.current.isSaving).toBe(true);
  await act(async () => result.current.handleNext());
  expect(actionMocks.updateAgentProfileAction).toHaveBeenCalledTimes(2);
  await act(async () => {
    finishSuccessfulSave();
    await save;
  });
  expect(result.current.step).toBe(0);
  await act(async () => result.current.handleNext());
  expect(actionMocks.updateAgentProfileAction.mock.calls).toEqual([
    ["profile-1", { model: UPDATED_MODEL }],
    ["profile-2", { model: UPDATED_MODEL }],
    ["profile-2", { model: UPDATED_MODEL }],
  ]);
});

it("saves a return to the original model after Next and Back", async () => {
  const { result } = renderActions();
  act(() => result.current.updateSetting(TEST_AGENT_NAME, { model: UPDATED_MODEL }));
  await act(async () => result.current.handleNext());
  act(() => result.current.handleBack());
  act(() => result.current.updateSetting(TEST_AGENT_NAME, { model: "gpt-4" }));
  await act(async () => result.current.handleNext());

  expect(actionMocks.updateAgentProfileAction.mock.calls).toEqual([
    ["profile-1", { model: UPDATED_MODEL }],
    ["profile-1", { model: "gpt-4" }],
  ]);
});

it("does not resave an unchanged profile on completion", async () => {
  const { result } = renderActions();
  act(() => result.current.updateSetting(TEST_AGENT_NAME, { cli_passthrough: true }));
  await act(async () => result.current.handleNext());
  await act(async () => result.current.handleGetStarted());
  expect(actionMocks.updateAgentProfileAction).toHaveBeenCalledTimes(1);
});

it("preserves edits made during a save and writes them against the newly saved baseline", async () => {
  let resolveSave!: () => void;
  actionMocks.updateAgentProfileAction.mockImplementationOnce(
    () => new Promise<void>((resolve) => (resolveSave = resolve)),
  );
  const { result } = renderActions();
  act(() => result.current.updateSetting(TEST_AGENT_NAME, { model: UPDATED_MODEL }));
  let save!: Promise<void>;
  act(() => {
    save = result.current.handleNext();
  });
  act(() => result.current.updateSetting(TEST_AGENT_NAME, { model: "gpt-4" }));
  await act(async () => {
    resolveSave();
    await save;
  });
  await act(async () => result.current.handleGetStarted());
  expect(actionMocks.updateAgentProfileAction).toHaveBeenLastCalledWith("profile-1", {
    model: "gpt-4",
  });
});

it("saves exact partial patch with only changed model", async () => {
  let currentSettings = {
    ...initialSettings,
    [TEST_AGENT_NAME]: {
      ...initialSettings[TEST_AGENT_NAME],
      draft: { model: UPDATED_MODEL, cli_passthrough: false },
      dirty: true,
    },
  };
  const setStep = vi.fn();
  const onComplete = vi.fn();
  const setAgentSettings = vi.fn((updater) => {
    if (typeof updater === "function") {
      currentSettings = updater(currentSettings);
    }
  });

  const { result } = renderHook(() =>
    useOnboardingActions({
      step: 0,
      setStep,
      onComplete,
      agentSettings: currentSettings,
      setAgentSettings,
    }),
  );

  await act(async () => {
    await result.current.handleNext();
  });

  expect(actionMocks.updateAgentProfileAction).toHaveBeenCalledWith("profile-1", {
    model: UPDATED_MODEL,
  });
  expect(setStep).toHaveBeenCalledWith(1);
});

it("saves exact partial patch with only changed cli_passthrough", async () => {
  const currentSettings = {
    ...initialSettings,
    [TEST_AGENT_NAME]: {
      ...initialSettings[TEST_AGENT_NAME],
      draft: { model: "gpt-4", cli_passthrough: true },
      dirty: true,
    },
  };
  const setStep = vi.fn();
  const onComplete = vi.fn();
  const setAgentSettings = vi.fn();

  const { result } = renderHook(() =>
    useOnboardingActions({
      step: 0,
      setStep,
      onComplete,
      agentSettings: currentSettings,
      setAgentSettings,
    }),
  );

  await act(async () => {
    await result.current.handleNext();
  });

  expect(actionMocks.updateAgentProfileAction).toHaveBeenCalledWith("profile-1", {
    cli_passthrough: true,
  });
});

it("does not call updateAgentProfileAction when draft is not dirty", async () => {
  const setStep = vi.fn();
  const onComplete = vi.fn();
  const setAgentSettings = vi.fn();

  const { result } = renderHook(() =>
    useOnboardingActions({
      step: 0,
      setStep,
      onComplete,
      agentSettings: initialSettings,
      setAgentSettings,
    }),
  );

  await act(async () => {
    await result.current.handleNext();
  });

  expect(actionMocks.updateAgentProfileAction).not.toHaveBeenCalled();
  expect(setStep).toHaveBeenCalledWith(1);
});

it("updates setting and marks dirty correctly", () => {
  let settings = initialSettings;
  const setAgentSettings = vi.fn((updater) => {
    if (typeof updater === "function") {
      settings = updater(settings);
    }
  });

  const { result } = renderHook(() =>
    useOnboardingActions({
      step: 0,
      setStep: vi.fn(),
      onComplete: vi.fn(),
      agentSettings: settings,
      setAgentSettings,
    }),
  );

  act(() => {
    result.current.updateSetting(TEST_AGENT_NAME, { model: UPDATED_MODEL });
  });

  expect(settings[TEST_AGENT_NAME].dirty).toBe(true);
  expect(settings[TEST_AGENT_NAME].draft.model).toBe(UPDATED_MODEL);

  act(() => {
    result.current.updateSetting(TEST_AGENT_NAME, { model: "gpt-4" });
  });

  expect(settings[TEST_AGENT_NAME].dirty).toBe(false);
});

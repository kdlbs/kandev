import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { TaskCreateAdvancedSettings } from "./task-create-dialog-advanced-settings";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const ADVANCED_SETTINGS_TRIGGER_TEST_ID = "task-create-advanced-settings-trigger";
const DEPENDENCY_TRIGGER_TEST_ID = "task-create-dependencies-trigger";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => (key === "task:advancedSettings" ? "Advanced settings" : key),
  }),
}));

vi.mock("@/components/task-create-dialog-dependencies", () => ({
  TaskCreateDependencies: ({
    value,
    onChange,
  }: {
    value: string[];
    onChange: (next: string[]) => void;
  }) => (
    <button
      type="button"
      data-testid={DEPENDENCY_TRIGGER_TEST_ID}
      onClick={() => onChange([...value, "task-2"])}
    >
      {value.length === 0 ? "No dependency" : `${value.length} dependencies`}
    </button>
  ),
}));

function renderAdvancedSettings(
  overrides: Partial<React.ComponentProps<typeof TaskCreateAdvancedSettings>> = {},
) {
  return render(
    <TaskCreateAdvancedSettings
      isCreateMode
      isTaskStarted={false}
      blockedBy={[]}
      onBlockedByChange={() => {}}
      {...overrides}
    />,
  );
}

describe("TaskCreateAdvancedSettings", () => {
  it("starts collapsed and keeps the dependency selector hidden", () => {
    renderAdvancedSettings();

    const trigger = screen.getByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID);
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(trigger.className).toContain("min-h-12");
    expect(screen.queryByTestId(DEPENDENCY_TRIGGER_TEST_ID)).toBeNull();
  });

  it("reveals the dependency selector when expanded", () => {
    renderAdvancedSettings();

    const trigger = screen.getByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID);
    fireEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByTestId(DEPENDENCY_TRIGGER_TEST_ID).getAttribute("hidden")).toBeNull();
  });

  it("preserves selected dependencies across collapse and reopen", () => {
    function Harness() {
      const [blockedBy, setBlockedBy] = useState<string[]>(["task-1"]);
      return (
        <TaskCreateAdvancedSettings
          isCreateMode
          isTaskStarted={false}
          blockedBy={blockedBy}
          onBlockedByChange={setBlockedBy}
        />
      );
    }

    render(<Harness />);
    const trigger = screen.getByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID);
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTestId(DEPENDENCY_TRIGGER_TEST_ID));
    expect(screen.getByTestId(DEPENDENCY_TRIGGER_TEST_ID).textContent).toContain("2 dependencies");

    fireEvent.click(trigger);
    expect(screen.queryByTestId(DEPENDENCY_TRIGGER_TEST_ID)).toBeNull();
    fireEvent.click(trigger);
    expect(screen.getByTestId(DEPENDENCY_TRIGGER_TEST_ID).textContent).toContain("2 dependencies");
  });

  it.each([
    ["edit mode", { isCreateMode: false, isTaskStarted: false }],
    ["started task", { isCreateMode: true, isTaskStarted: true }],
  ])("does not render for a %s", (_name, props) => {
    renderAdvancedSettings(props);

    expect(screen.queryByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID)).toBeNull();
  });
});

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { DynamicErrorPolicy } from "@/lib/types/agent-profile";
import { DynamicPolicyEditor } from "./dynamic-agent-policy-editor";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) => {
      if (key === "agents:dynamicPolicySchedule") {
        return `Retry up to ${options?.retries} times. First wait: ${options?.first}s.`;
      }
      return key;
    },
  }),
}));

const policy = (): DynamicErrorPolicy => ({
  retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
  waitForReset: { enabled: false, maxWaitSeconds: 0 },
  onExhausted: "skip",
});

function StatefulEditor({ errorClass }: { errorClass: "transient" | "hard" }) {
  const [current, setCurrent] = useState(policy());
  return (
    <DynamicPolicyEditor
      errorClass={errorClass}
      policy={current}
      onChange={(patch) => setCurrent((previous) => ({ ...previous, ...patch }))}
    />
  );
}

describe("DynamicPolicyEditor", () => {
  afterEach(cleanup);

  it("renders class guidance and exposes retry and reset controls", () => {
    render(
      <TooltipProvider>
        <StatefulEditor errorClass="transient" />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("dynamic-policy-transient")).toBeTruthy();
    expect(screen.getByText("agents:dynamicTransientErrors")).toBeTruthy();
    expect(screen.getByRole("switch", { name: "agents:dynamicPolicyRetry" })).toBeTruthy();
    expect(screen.getByRole("switch", { name: "agents:dynamicPolicyWaitForReset" })).toBeTruthy();
    expect(screen.getByTestId("dynamic-policy-option-help-outcome")).toBeTruthy();
  });

  it("reveals bounded retry and reset-wait fields when enabled", () => {
    render(
      <TooltipProvider>
        <StatefulEditor errorClass="hard" />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("switch", { name: "agents:dynamicPolicyRetry" }));
    expect(screen.getByRole("spinbutton", { name: "agents:dynamicPolicyMaxRetries" })).toBeTruthy();
    expect(
      screen.getByRole("spinbutton", { name: "agents:dynamicPolicyInitialInterval" }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("switch", { name: "agents:dynamicPolicyWaitForReset" }));
    expect(screen.getByRole("spinbutton", { name: "agents:dynamicPolicyMaxWait" })).toBeTruthy();
  });
});

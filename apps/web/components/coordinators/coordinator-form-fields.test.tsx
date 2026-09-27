import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { CoordinatorFormFields } from "./coordinator-form-fields";

const FIXTURE_TIMESTAMP = "2026-01-01T00:00:00Z";

function mkAgentProfile(overrides: Partial<AgentProfileOption> = {}): AgentProfileOption {
  return {
    id: "agent-1",
    label: "Claude, Sonnet",
    agent_id: "claude",
    agent_name: "Claude",
    cli_passthrough: false,
    ...overrides,
  };
}

function mkExecutorProfile(overrides: Partial<ExecutorProfile> = {}): ExecutorProfile {
  return {
    id: "profile-1",
    executor_id: "exec-1",
    name: "worktree",
    prepare_script: "",
    cleanup_script: "",
    created_at: FIXTURE_TIMESTAMP,
    updated_at: FIXTURE_TIMESTAMP,
    ...overrides,
  };
}

function mkExecutor(overrides: Partial<Executor> = {}): Executor {
  return {
    id: "exec-1",
    name: "Local",
    type: "local",
    status: "ready",
    is_system: false,
    profiles: [mkExecutorProfile()],
    created_at: FIXTURE_TIMESTAMP,
    updated_at: FIXTURE_TIMESTAMP,
    ...overrides,
  };
}

const baseForm = {
  name: "Planner",
  agentProfileId: "agent-1",
  executorProfileId: "profile-1",
  context: "Some context",
};

function renderFields(overrides: Partial<React.ComponentProps<typeof CoordinatorFormFields>> = {}) {
  return render(
    <TooltipProvider>
      <CoordinatorFormFields
        form={baseForm}
        onChange={vi.fn()}
        disabled={false}
        agentProfiles={[mkAgentProfile()]}
        executors={[mkExecutor()]}
        fieldError={null}
        {...overrides}
      />
    </TooltipProvider>,
  );
}

describe("CoordinatorFormFields", () => {
  afterEach(cleanup);

  it("renders every field with its current value (AC-004.3)", () => {
    renderFields();
    expect(screen.getByLabelText("Name")).toHaveProperty("value", "Planner");
    expect(screen.getByLabelText("Context")).toHaveProperty("value", "Some context");
  });

  it("reports name changes through onChange, capped at 60 characters", () => {
    const onChange = vi.fn();
    renderFields({ onChange });
    const input = screen.getByLabelText("Name") as HTMLInputElement;
    expect(input.maxLength).toBe(60);
    fireEvent.change(input, { target: { value: "New name" } });
    expect(onChange).toHaveBeenCalledWith("name", "New name");
  });

  it("reports context changes through onChange", () => {
    const onChange = vi.fn();
    renderFields({ onChange });
    fireEvent.change(screen.getByLabelText("Context"), { target: { value: "New context" } });
    expect(onChange).toHaveBeenCalledWith("context", "New context");
  });

  it("lists a CLI-passthrough agent profile disabled with its reason, still visible (AC-004.3)", () => {
    renderFields({
      agentProfiles: [
        mkAgentProfile({ id: "agent-1", label: "Claude, Sonnet", cli_passthrough: false }),
        mkAgentProfile({ id: "agent-2", label: "Passthrough agent", cli_passthrough: true }),
      ],
    });

    fireEvent.click(screen.getByTestId("coordinator-agent-profile-picker"));
    const passthroughOption = screen.getByRole("option", { name: /Passthrough agent/ });
    expect(passthroughOption.getAttribute("aria-disabled")).toBe("true");
  });

  it("shows the agent-missing warning under Agent profile (AC-005.1)", () => {
    renderFields({ agentProfileStatus: "missing" });
    expect(screen.getByTestId("coordinator-agent-profile-status").textContent).toContain("removed");
  });

  it("shows the agent-passthrough warning under Agent profile, distinct from missing (AC-005.1)", () => {
    renderFields({ agentProfileStatus: "passthrough" });
    expect(screen.getByTestId("coordinator-agent-profile-status").textContent).toContain(
      "CLI passthrough",
    );
  });

  it("shows the executor-missing warning under Executor (AC-005.1)", () => {
    renderFields({ executorProfileStatus: "missing" });
    expect(screen.getByTestId("coordinator-executor-status").textContent).toContain("removed");
  });

  it("shows no profile-status warnings when both are ok", () => {
    renderFields({ agentProfileStatus: "ok", executorProfileStatus: "ok" });
    expect(screen.queryByTestId("coordinator-agent-profile-status")).toBeNull();
    expect(screen.queryByTestId("coordinator-executor-status")).toBeNull();
  });

  it("shows a 400 field error under the field it names (design B11)", () => {
    renderFields({ fieldError: { field: "name", message: "Name is required" } });
    expect(screen.getByTestId("coordinator-name-error").textContent).toBe("Name is required");
  });

  it("disables every field when disabled is true (AC-004.6)", () => {
    renderFields({ disabled: true });
    expect((screen.getByLabelText("Name") as HTMLInputElement).disabled).toBe(true);
    expect((screen.getByLabelText("Context") as HTMLTextAreaElement).disabled).toBe(true);
  });

  it("shows the stored executor profile as unavailable when it no longer resolves (B11)", () => {
    renderFields({
      form: { ...baseForm, executorProfileId: "profile-missing" },
      executors: [mkExecutor()],
    });
    fireEvent.click(screen.getByTestId("executor-profile-selector"));
    expect(screen.getAllByRole("option")[0].getAttribute("aria-disabled")).toBe("true");
  });
});

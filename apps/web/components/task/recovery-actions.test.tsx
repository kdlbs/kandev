import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RecoveryActions, type RecoveryChoice } from "./recovery-actions";
import type { RecoveryActionKind } from "@/lib/session-recovery-actions";

afterEach(cleanup);

const PROGRESS_STATUS = "recovery-progress-status";
const pendingLabels: Array<[RecoveryActionKind, string]> = [
  ["resume", "Resuming session"],
  ["fresh_start", "Starting..."],
  ["runtime_retry", "Retrying..."],
  ["resume_new_branch", "Resuming..."],
  ["relocate_and_resume", "Resuming..."],
  ["continue_from_history", "Continuing from saved history..."],
  ["retry_connection", "Retrying connection..."],
  ["restore", "Restoring..."],
];

function choice(kind: RecoveryActionKind): RecoveryChoice {
  return {
    kind,
    label: kind === "resume" ? "Resume" : "Retry connection",
    testId: kind === "resume" ? "recovery-resume-button" : "recovery-retry-connection-button",
    onClick: vi.fn(),
  };
}

function renderPending(actions = [choice("resume"), choice("retry_connection")]) {
  return render(
    <RecoveryActions
      actions={actions}
      busy
      busyAction="resume"
      trailingActions={
        <button type="button" data-testid="recovery-stop-button">
          Stop
        </button>
      }
    />,
  );
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.4
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.14
describe("RecoveryActions pending status", () => {
  it.each(pendingLabels)("uses the %s operation label", (kind, expectedLabel) => {
    render(<RecoveryActions actions={[choice(kind)]} busy busyAction={kind} />);

    const status = screen.getByTestId(PROGRESS_STATUS);
    expect(status.textContent).toContain(expectedLabel);
    expect(status.getAttribute("role")).toBe("status");
    expect(status.getAttribute("aria-live")).toBe("polite");
    expect(screen.getAllByRole("status")).toHaveLength(1);
    expect(status.querySelector(".spinner-grid")).not.toBeNull();
    expect(status.querySelector('[aria-hidden="true"]')?.className).toContain("flex");
    expect(status.querySelector(".spinner-grid")?.className).toContain("text-primary");
    expect(status.className).toContain("text-sm");
    expect(status.className).toContain("font-medium");
    expect(status.className).toContain("text-foreground");
  });

  it("uses the checking label when no recovery action owns progress", () => {
    render(<RecoveryActions actions={[choice("resume")]} busy />);

    expect(screen.getByTestId(PROGRESS_STATUS).textContent).toContain("Checking session...");
  });

  it("announces progress before disabled duplicate actions and leaves Stop available", () => {
    renderPending();

    const status = screen.getByTestId(PROGRESS_STATUS);
    const resume = screen.getByRole("button", { name: "Resume" });
    const retryConnection = screen.getByRole("button", { name: "Retry connection" });
    const stop = screen.getByRole("button", { name: "Stop" });
    const order = status.compareDocumentPosition(resume);

    expect(order & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
    expect(resume.getAttribute("aria-label")).toBe("Resume");
    expect(retryConnection.getAttribute("aria-label")).toBe("Retry connection");
    expect((resume as HTMLButtonElement).disabled).toBe(true);
    expect((retryConnection as HTMLButtonElement).disabled).toBe(true);
    expect((stop as HTMLButtonElement).disabled).toBe(false);
  });

  it("keeps an empty idle status mounted without adding visible height", () => {
    render(<RecoveryActions actions={[choice("resume")]} />);

    const status = screen.getByTestId(PROGRESS_STATUS);
    expect(status.textContent).toBe("");
    expect(status.children).toHaveLength(0);
    expect(status.className).not.toContain("min-h-");
    expect(status.className).not.toContain("mt-");
  });
});

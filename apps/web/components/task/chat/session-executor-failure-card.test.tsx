import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { SessionExecutorFailureCard } from "./session-executor-failure-card";
import type { ExecutorFailureEpisode } from "@/lib/types/executor-failure";
const episode: ExecutorFailureEpisode = {
  id: "failure",
  task_id: "task",
  revision: 1,
  state: "active",
  first_observed_at: "2026-10-07T00:00:00Z",
  last_observed_at: "2026-10-07T00:00:00Z",
  observation: {
    outcome: "terminated",
    runtime: "k8s",
    observed_at: "2026-10-07T00:00:00Z",
    reason: "Evicted",
    message: "Temporary storage limit exceeded.",
    workspace: "retained",
    container_ready: false,
  },
};
afterEach(cleanup);
it("puts a named failure in the composer card and expands details inline", () => {
  render(<SessionExecutorFailureCard taskId="task" episode={episode} />);
  expect(screen.getByRole("heading").textContent).toContain("Executor evicted");
  expect(screen.getByTestId("session-executor-failure-card").textContent).toContain(
    "Temporary storage limit exceeded",
  );
  fireEvent.click(screen.getByRole("button", { name: "Show details" }));
  expect(screen.getByTestId("executor-failure-details").textContent).toContain("persistent volume");
  expect(screen.queryByRole("dialog")).toBeNull();
});

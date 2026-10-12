import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StartupSnapshot } from "@/lib/startup-progress/types";
import { RestartProgressDialog } from "./restart-progress-dialog";

const mocks = vi.hoisted(() => ({ fetchJson: vi.fn() }));

vi.mock("@/lib/api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/client")>();
  return { ...actual, fetchJson: mocks.fetchJson };
});

function snapshot(overrides: Partial<StartupSnapshot> = {}): StartupSnapshot {
  return {
    phase: "applying_migrations",
    boot: 1,
    seq: 1,
    elapsed_ms: 5000,
    phase_elapsed_ms: 5000,
    ...overrides,
  };
}

beforeEach(() => {
  mocks.fetchJson.mockReset();
  // Never resolves by default, so a test that doesn't care about the poll
  // outcome sees the pre-first-read "waiting" state deterministically.
  mocks.fetchJson.mockReturnValue(new Promise(() => {}));
});

afterEach(() => {
  cleanup();
});

describe("RestartProgressDialog", () => {
  it("renders nothing while idle", () => {
    const { container } = render(
      <RestartProgressDialog phase="idle" errorMessage={null} onDismiss={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("shows the waiting state before the first /ready read resolves", async () => {
    render(<RestartProgressDialog phase="restarting" errorMessage={null} onDismiss={vi.fn()} />);
    expect(await screen.findByText("Kandev is starting and not yet reachable.")).not.toBeNull();
  });

  it("renders phase and elapsed time alone for a snapshot with no active step", async () => {
    mocks.fetchJson.mockResolvedValue({ startup: snapshot() });

    render(<RestartProgressDialog phase="restarting" errorMessage={null} onDismiss={vi.fn()} />);

    expect(await screen.findByText("Applying migrations")).not.toBeNull();
    expect(screen.getByText("5 seconds elapsed")).not.toBeNull();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("renders step-level detail from a polled snapshot", async () => {
    mocks.fetchJson.mockResolvedValue({
      startup: snapshot({
        step: {
          id: "stores.repositories",
          label_key: "startup.step.stores_repositories",
          measure: "counted",
          unit: "turns",
          elapsed_ms: 1000,
          done: 5,
          total: 10,
          eta_ms: 30000,
          stalled: false,
        },
      }),
    });

    render(<RestartProgressDialog phase="restarting" errorMessage={null} onDismiss={vi.fn()} />);

    expect(await screen.findByText("Applying migrations")).not.toBeNull();
    expect(screen.getByText("5 seconds elapsed")).not.toBeNull();
    expect(screen.getByText("Repository stores")).not.toBeNull();
    expect(screen.getByText("5 of 10 turns")).not.toBeNull();
    expect(screen.getByText("About 30 seconds remaining")).not.toBeNull();
  });

  it("renders stalled-state detail for a step that has not advanced (AC-PLATFORM-STARTUP-PROGRESS-004.6)", async () => {
    mocks.fetchJson.mockResolvedValue({
      startup: snapshot({
        step: {
          id: "stores.repositories",
          label_key: "startup.step.stores_repositories",
          measure: "counted",
          unit: "turns",
          elapsed_ms: 1000,
          done: 5,
          total: 10,
          since_advance_ms: 125000,
          stalled: true,
        },
      }),
    });

    render(<RestartProgressDialog phase="restarting" errorMessage={null} onDismiss={vi.fn()} />);

    expect(await screen.findByText("5 of 10 turns")).not.toBeNull();
    expect(screen.getByText("Stalled for 3 minutes")).not.toBeNull();
  });

  it("renders no stalled-state text for a running step", async () => {
    mocks.fetchJson.mockResolvedValue({
      startup: snapshot({
        step: {
          id: "stores.repositories",
          label_key: "startup.step.stores_repositories",
          measure: "counted",
          unit: "turns",
          elapsed_ms: 1000,
          done: 5,
          total: 10,
          since_advance_ms: 500,
          stalled: false,
        },
      }),
    });

    render(<RestartProgressDialog phase="restarting" errorMessage={null} onDismiss={vi.fn()} />);

    expect(await screen.findByText("5 of 10 turns")).not.toBeNull();
    expect(screen.queryByText(/Stalled for/)).toBeNull();
  });

  it("shows the last-known note once a read has been lost after an earlier success", async () => {
    mocks.fetchJson
      .mockResolvedValueOnce({ startup: snapshot() })
      .mockRejectedValue(new TypeError("fetch failed"));

    render(<RestartProgressDialog phase="restarting" errorMessage={null} onDismiss={vi.fn()} />);
    await screen.findByText("Applying migrations");

    await waitFor(
      () => {
        expect(screen.getByText("Last known")).not.toBeNull();
      },
      { timeout: 2000 },
    );
  });

  it("does not render startup progress detail outside the restarting phase", () => {
    render(<RestartProgressDialog phase="done" errorMessage={null} onDismiss={vi.fn()} />);
    expect(screen.queryByText("Kandev is starting and not yet reachable.")).toBeNull();
    expect(mocks.fetchJson).not.toHaveBeenCalled();
  });
});

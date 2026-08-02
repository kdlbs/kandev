import type { ReactNode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StorageQuarantineEntry } from "@/lib/types/system";
import { StorageQuarantineCard } from "./storage-quarantine-card";

vi.mock("@/hooks/domains/system/use-system-jobs", () => ({
  useSystemJob: () => undefined,
  useSystemJobs: () => [],
}));

afterEach(cleanup);

function Providers({ children }: { children: ReactNode }) {
  return <TooltipProvider>{children}</TooltipProvider>;
}

function renderCard(
  props: { entries?: StorageQuarantineEntry[]; loading?: boolean; error?: string | null } = {},
) {
  return render(
    <StorageQuarantineCard
      entries={props.entries ?? []}
      schedulingEnabled={false}
      checkIntervalHours={24}
      onRestore={vi.fn()}
      onDelete={vi.fn()}
      onClearEligible={vi.fn()}
      onForceClearAll={vi.fn()}
      {...props}
    />,
    { wrapper: Providers },
  );
}

describe("StorageQuarantineCard", () => {
  it("renders an independent loading state", () => {
    renderCard({ loading: true });

    expect(screen.getByTestId("storage-quarantine-spinner")).toBeTruthy();
    expect(screen.getByText("Loading quarantine…")).toBeTruthy();
    expect(screen.queryByText("No restorable quarantined resources.")).toBeNull();
    expect(screen.queryByTestId("storage-quarantine-total")).toBeNull();
  });

  it("renders an isolated error state", () => {
    renderCard({ error: "quarantine unavailable" });

    expect(screen.getByTestId("storage-quarantine-error").textContent).toContain(
      "quarantine unavailable",
    );
    expect(screen.queryByTestId("storage-quarantine-spinner")).toBeNull();
    expect(screen.queryByTestId("storage-quarantine-total")).toBeNull();
  });

  it("shows the total of all listed entries", () => {
    const entries = [
      {
        id: "cache-1",
        resource_type: "go_cache",
        original_path: "/var/cache/go-build",
        quarantine_path: "/var/trash/cache-1",
        size_bytes: 2 * 1024 ** 3,
        state: "quarantined",
        quarantined_at: "2026-07-23T12:00:00Z",
        delete_after: "2026-08-23T12:00:00Z",
        last_error: "",
        metadata: {},
      },
      {
        id: "workspace-1",
        resource_type: "task_workspace",
        original_path: "/var/tasks/workspace-1",
        quarantine_path: "/var/trash/workspace-1",
        size_bytes: 1024 ** 3,
        state: "quarantined",
        quarantined_at: "2026-07-23T12:00:00Z",
        delete_after: "2026-08-23T12:00:00Z",
        last_error: "",
        metadata: {},
      },
    ] satisfies StorageQuarantineEntry[];

    renderCard({ entries });

    expect(screen.getByTestId("storage-quarantine-total").textContent).toBe(
      "Total quarantined: 3 GB",
    );
  });
});

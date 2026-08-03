import type { ReactNode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDateTime } from "@/lib/i18n/formats";
import type { StorageQuarantineEntry } from "@/lib/types/system";
import { StorageQuarantineCard } from "./storage-quarantine-card";

vi.mock("@/hooks/domains/system/use-system-jobs", () => ({
  useSystemJob: () => undefined,
  useSystemJobs: () => [],
}));

afterEach(cleanup);

const QUARANTINED_AT = "2026-07-23T12:00:00Z";
const DELETE_AFTER = "2026-08-23T12:00:00Z";

function Providers({ children }: { children: ReactNode }) {
  return <TooltipProvider>{children}</TooltipProvider>;
}

function renderCard(
  props: {
    entries?: StorageQuarantineEntry[];
    loading?: boolean;
    error?: string | null;
    schedulingEnabled?: boolean;
    checkIntervalHours?: number;
  } = {},
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
        quarantined_at: QUARANTINED_AT,
        delete_after: DELETE_AFTER,
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
        quarantined_at: QUARANTINED_AT,
        delete_after: DELETE_AFTER,
        last_error: "",
        metadata: {},
      },
    ] satisfies StorageQuarantineEntry[];

    renderCard({ entries });

    expect(screen.getByTestId("storage-quarantine-total").textContent).toBe(
      "Total quarantined: 3 GB",
    );
  });

  // The eligible/protected counts are two separate plural messages joined, and
  // the protected row is a <Trans> whose <1> addresses its <time> child
  // positionally. Both reassemble silently into fragments if they drift, so
  // each assertion is on the whole reconstructed sentence.
  it("reads the counts and the retention deadline as whole sentences", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(QUARANTINED_AT));
    const entries = [
      {
        id: "workspace-1",
        resource_type: "task_workspace",
        original_path: "/var/tasks/workspace-1",
        quarantine_path: "/var/trash/workspace-1",
        size_bytes: 1024 ** 3,
        state: "quarantined",
        quarantined_at: QUARANTINED_AT,
        delete_after: DELETE_AFTER,
        last_error: "",
        metadata: {},
      },
    ] satisfies StorageQuarantineEntry[];

    renderCard({ entries });

    const deadline = formatDateTime(DELETE_AFTER);
    expect(screen.getByTestId("storage-quarantine-workspace-1").textContent).toContain(
      `Protected until ${deadline}`,
    );
    expect(screen.getByText("Trash: /var/trash/workspace-1")).toBeTruthy();
    expect(screen.getByText(/eligible now · 1 protected$/).textContent).toContain(
      "0 eligible now · 1 protected",
    );
    vi.useRealTimers();
  });

  it("agrees the schedule copy with the interval", () => {
    renderCard({ schedulingEnabled: true, checkIntervalHours: 1 });
    expect(screen.getByTestId("storage-quarantine-schedule-copy").textContent).toContain(
      "(every 1 hour when the idle gate allows it)",
    );

    cleanup();
    renderCard({ schedulingEnabled: true, checkIntervalHours: 24 });
    expect(screen.getByTestId("storage-quarantine-schedule-copy").textContent).toContain(
      "(every 24 hours when the idle gate allows it)",
    );
  });
});

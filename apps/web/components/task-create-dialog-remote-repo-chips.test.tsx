import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import type { DialogFormState, TaskRemoteRepoRow } from "./task-create-dialog-types";
import { TooltipProvider } from "@kandev/ui/tooltip";

// Stub out the chip's heavy popover content (we test that separately). The
// chips-row's only job is to render N chips + the Add button and pipe
// branchesByUrl.ensure() for non-empty URLs — so the stub just emits a
// data-testid and exposes onRemove on the row for click assertions, plus
// two buttons that invoke onURLChange in picker- and paste-mode so we can
// assert the parent's metadata-application logic without touching the
// chip's real popover.
type ChipURLChange = (
  url: string,
  source: "picker" | "paste",
  metadata?: { provider: "github" | "gitlab"; fullName: string; defaultBranch: string },
) => void;
vi.mock("./task-create-dialog-remote-repo-chip", () => ({
  RemoteRepoChip: ({
    row,
    onRemove,
    onURLChange,
  }: {
    row: TaskRemoteRepoRow;
    onRemove: () => void;
    onURLChange: ChipURLChange;
  }) => (
    <div data-testid="remote-repo-chip" data-url={row.url}>
      <span data-testid="remote-repo-chip-url">{row.url}</span>
      <button type="button" data-testid="remote-chip-remove" onClick={onRemove}>
        x
      </button>
      <button
        type="button"
        data-testid="remote-chip-fire-picker"
        onClick={() =>
          onURLChange("https://github.com/acme/site", "picker", {
            provider: "github",
            fullName: "acme/site",
            defaultBranch: "trunk",
          })
        }
      >
        pick
      </button>
      <button
        type="button"
        data-testid="remote-chip-fire-paste"
        onClick={() => onURLChange("https://github.com/foo/bar", "paste")}
      >
        paste
      </button>
    </div>
  ),
}));

import { RemoteRepoChipsRow } from "./task-create-dialog-remote-repo-chips";

const URL_AB = "https://github.com/a/b";
const URL_CD = "https://github.com/c/d";

afterEach(cleanup);

function makeBranchesByUrl(ensure = vi.fn()) {
  return {
    branches: () => [],
    loading: () => false,
    ensure,
  };
}

function makeFs(overrides: Partial<DialogFormState>): DialogFormState {
  return {
    remoteRepos: [] as TaskRemoteRepoRow[],
    branchesByUrl: makeBranchesByUrl(),
    ...overrides,
  } as unknown as DialogFormState;
}

function renderInProvider(ui: Parameters<typeof render>[0]) {
  return render(<TooltipProvider>{ui}</TooltipProvider>);
}

describe("RemoteRepoChipsRow", () => {
  it("renders one chip per row in fs.remoteRepos", () => {
    const fs = makeFs({
      remoteRepos: [
        { key: "remote-0", url: URL_AB, branch: "", source: "paste" },
        { key: "remote-1", url: URL_CD, branch: "", source: "paste" },
      ],
    });
    renderInProvider(
      <RemoteRepoChipsRow fs={fs} onUpdateRow={vi.fn()} onAddRow={vi.fn()} onRemoveRow={vi.fn()} />,
    );
    expect(screen.getAllByTestId("remote-repo-chip")).toHaveLength(2);
  });

  it("renders a placeholder chip when remoteRepos is empty", () => {
    const fs = makeFs({ remoteRepos: [] });
    renderInProvider(
      <RemoteRepoChipsRow fs={fs} onUpdateRow={vi.fn()} onAddRow={vi.fn()} onRemoveRow={vi.fn()} />,
    );
    // Defends against the seed-effect edge case — at minimum, the add button
    // must be available so the user can add a row from nothing.
    expect(screen.getByTestId("remote-add-row")).toBeTruthy();
  });

  it("clicking + Add calls onAddRow once", () => {
    const onAddRow = vi.fn();
    renderInProvider(
      <RemoteRepoChipsRow
        fs={makeFs({
          remoteRepos: [{ key: "remote-0", url: "", branch: "", source: "paste" }],
        })}
        onUpdateRow={vi.fn()}
        onAddRow={onAddRow}
        onRemoveRow={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByTestId("remote-add-row"));
    expect(onAddRow).toHaveBeenCalledOnce();
  });

  it("clicking remove on a chip calls onRemoveRow with the row key", () => {
    const onRemoveRow = vi.fn();
    renderInProvider(
      <RemoteRepoChipsRow
        fs={makeFs({
          remoteRepos: [
            { key: "remote-0", url: URL_AB, branch: "", source: "paste" },
            { key: "remote-1", url: URL_CD, branch: "", source: "paste" },
          ],
        })}
        onUpdateRow={vi.fn()}
        onAddRow={vi.fn()}
        onRemoveRow={onRemoveRow}
      />,
    );
    fireEvent.click(screen.getAllByTestId("remote-chip-remove")[0]);
    expect(onRemoveRow).toHaveBeenCalledWith("remote-0");
  });

  it("calls branchesByUrl.ensure for every non-empty URL row", () => {
    const ensure = vi.fn();
    const fs = makeFs({
      remoteRepos: [
        { key: "remote-0", url: URL_AB, branch: "", source: "paste" },
        { key: "remote-1", url: "", branch: "", source: "paste" }, // empty — not ensured
        { key: "remote-2", url: URL_CD, branch: "", source: "paste" },
      ],
      branchesByUrl: makeBranchesByUrl(ensure),
    });
    renderInProvider(
      <RemoteRepoChipsRow fs={fs} onUpdateRow={vi.fn()} onAddRow={vi.fn()} onRemoveRow={vi.fn()} />,
    );
    expect(ensure).toHaveBeenCalledWith(URL_AB);
    expect(ensure).toHaveBeenCalledWith(URL_CD);
    expect(ensure).not.toHaveBeenCalledWith("");
  });
});

describe("RemoteRepoChipsRow — onURLChange wiring", () => {
  it("picker onURLChange writes url+metadata AND pre-fills branch with default_branch", () => {
    const onUpdateRow = vi.fn();
    renderInProvider(
      <RemoteRepoChipsRow
        fs={makeFs({
          remoteRepos: [{ key: "remote-0", url: "", branch: "", source: "paste" }],
        })}
        onUpdateRow={onUpdateRow}
        onAddRow={vi.fn()}
        onRemoveRow={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByTestId("remote-chip-fire-picker"));
    expect(onUpdateRow).toHaveBeenCalledWith("remote-0", {
      url: "https://github.com/acme/site",
      source: "picker",
      provider: "github",
      fullName: "acme/site",
      branch: "trunk",
    });
  });

  it("paste onURLChange clears picker metadata and DOES NOT pre-fill branch", () => {
    const onUpdateRow = vi.fn();
    renderInProvider(
      <RemoteRepoChipsRow
        fs={makeFs({
          remoteRepos: [{ key: "remote-0", url: "", branch: "", source: "paste" }],
        })}
        onUpdateRow={onUpdateRow}
        onAddRow={vi.fn()}
        onRemoveRow={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByTestId("remote-chip-fire-paste"));
    expect(onUpdateRow).toHaveBeenCalledWith("remote-0", {
      url: "https://github.com/foo/bar",
      source: "paste",
      provider: undefined,
      fullName: undefined,
      branch: "",
    });
  });
});

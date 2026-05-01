import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { CLIFlagsField } from "./cli-flags-field";
import type { CLIFlag, PermissionSetting } from "@/lib/types/http";

const ALLOW_INDEXING_FLAG = "--allow-indexing";
const ALLOW_INDEXING_DESC = "Enable workspace indexing without confirmation";
const CURATED_SWITCH_TESTID = "cli-flag-curated-enabled-allow_indexing";

// Curated permission whose default is ON — mirrors auggie's --allow-indexing.
// Reproduces the original bug: default-on switches couldn't be toggled off
// because no entry existed in cli_flags and the toggle short-circuited.
const allowIndexing: PermissionSetting = {
  supported: true,
  default: true,
  label: "Allow indexing",
  description: ALLOW_INDEXING_DESC,
  apply_method: "cli_flag",
  cli_flag: ALLOW_INDEXING_FLAG,
};

const onChange = vi.fn();

beforeEach(() => {
  onChange.mockClear();
});

afterEach(() => {
  cleanup();
});

describe("CLIFlagsField — curated toggles", () => {
  it("renders curated switch as checked when no entry exists and default is true", () => {
    const { getByTestId } = render(
      <CLIFlagsField
        flags={[]}
        onChange={onChange}
        permissionSettings={{ allow_indexing: allowIndexing }}
      />,
    );
    expect(getByTestId(CURATED_SWITCH_TESTID).getAttribute("data-state")).toBe("checked");
  });

  it("turning a default-on curated flag OFF persists an explicit { enabled: false } entry", () => {
    // This is the regression test for the original bug. Before the fix the
    // toggle short-circuited (`if (!enabled) return; // nothing to turn off`)
    // and onChange was never called, leaving the switch visually stuck ON.
    const { getByTestId } = render(
      <CLIFlagsField
        flags={[]}
        onChange={onChange}
        permissionSettings={{ allow_indexing: allowIndexing }}
      />,
    );
    fireEvent.click(getByTestId(CURATED_SWITCH_TESTID));
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith([
      { flag: ALLOW_INDEXING_FLAG, description: ALLOW_INDEXING_DESC, enabled: false },
    ]);
  });

  it("turning a default-off curated flag ON appends an { enabled: true } entry", () => {
    const setting: PermissionSetting = { ...allowIndexing, default: false };
    const { getByTestId } = render(
      <CLIFlagsField
        flags={[]}
        onChange={onChange}
        permissionSettings={{ allow_indexing: setting }}
      />,
    );
    const sw = getByTestId(CURATED_SWITCH_TESTID);
    expect(sw.getAttribute("data-state")).toBe("unchecked");
    fireEvent.click(sw);
    expect(onChange).toHaveBeenCalledWith([
      { flag: ALLOW_INDEXING_FLAG, description: ALLOW_INDEXING_DESC, enabled: true },
    ]);
  });

  it("toggles existing curated entry in place without duplicating it", () => {
    const existing: CLIFlag[] = [
      { flag: ALLOW_INDEXING_FLAG, description: "previous", enabled: true },
    ];
    const { getByTestId } = render(
      <CLIFlagsField
        flags={existing}
        onChange={onChange}
        permissionSettings={{ allow_indexing: allowIndexing }}
      />,
    );
    fireEvent.click(getByTestId(CURATED_SWITCH_TESTID));
    expect(onChange).toHaveBeenCalledWith([
      { flag: ALLOW_INDEXING_FLAG, description: "previous", enabled: false },
    ]);
  });

  it("reads switch state from the existing entry over setting.default", () => {
    const existing: CLIFlag[] = [
      { flag: ALLOW_INDEXING_FLAG, description: "previous", enabled: false },
    ];
    const { getByTestId } = render(
      <CLIFlagsField
        flags={existing}
        onChange={onChange}
        permissionSettings={{ allow_indexing: allowIndexing }}
      />,
    );
    expect(getByTestId(CURATED_SWITCH_TESTID).getAttribute("data-state")).toBe("unchecked");
  });
});

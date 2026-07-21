import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { LicensesList } from "./licenses-list";
import type { LicenseEntry } from "@/lib/types/system";

afterEach(() => {
  cleanup();
});

describe("LicensesList", () => {
  it("shows a stale warning when Go entries were reused from the existing manifest", () => {
    const entries: LicenseEntry[] = [
      {
        name: "github.com/example/module",
        version: "v1.0.0",
        license: "MIT",
        stale: true,
        ecosystem: "go",
      },
    ];

    const { getByTestId } = render(<LicensesList entries={entries} />);

    expect(getByTestId("system-licenses-stale-warning").textContent).toContain(
      "Go license entries were reused",
    );
  });

  it("does not show a stale warning for fresh entries", () => {
    const entries: LicenseEntry[] = [
      {
        name: "github.com/example/module",
        version: "v1.0.0",
        license: "MIT",
        ecosystem: "go",
      },
    ];

    const { queryByTestId } = render(<LicensesList entries={entries} />);

    expect(queryByTestId("system-licenses-stale-warning")).toBeNull();
  });
});

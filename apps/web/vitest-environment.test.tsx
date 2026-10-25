import { afterEach, describe, expect, it } from "vitest";
import * as React from "react";
import { cleanup, render, screen } from "@testing-library/react";

/**
 * Guards the test environment itself rather than any product code.
 *
 * `react/index.js` picks its build from `process.env.NODE_ENV`, and
 * `cjs/react.production.js` does not export `act`. When that build is loaded,
 * `@testing-library/react`'s act-compat shim silently falls back to the
 * deprecated `react-dom/test-utils`, whose production build calls `React.act`
 * and throws `TypeError: React.act is not a function` on the first `render()`
 * of every suite — 178 of 268 tests, none of them reaching an assertion.
 *
 * That is not hypothetical: the Kandev runtime image exports
 * `NODE_ENV=production` (`Dockerfile`), Vitest only *defaults* the variable
 * (`process.env.NODE_ENV ??= "test"`) rather than pinning it, and CI's image
 * sets none — so the whole frontend suite was dead inside the container while
 * CI stayed green. `vitest.config.ts` now pins the value; this file is what
 * makes a regression legible, as one named failure instead of every
 * render-based suite dying on a stack inside `node_modules`.
 *
 * The `render` case is the durable one: it keeps holding if the cause changes
 * to a duplicated `react` instance or a bad alias, which the two cheaper
 * assertions would miss.
 */

const PROBE_TEXT = "vitest-environment-probe";

afterEach(() => {
  cleanup();
});

describe("vitest environment", () => {
  it("runs with NODE_ENV=test so React loads its development build", () => {
    expect(process.env.NODE_ENV).toBe("test");
  });

  it("exposes React.act, which only the development build exports", () => {
    expect(typeof React.act).toBe("function");
  });

  it("identifies the simulated browser as Chromium", () => {
    expect(navigator.userAgent).toContain("Chrome/");
  });

  it("can render a component to the DOM", () => {
    render(<div data-testid="vitest-environment-probe">{PROBE_TEXT}</div>);

    expect(screen.getByTestId("vitest-environment-probe").textContent).toBe(PROBE_TEXT);
  });
});

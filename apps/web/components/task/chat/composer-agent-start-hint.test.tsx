import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ComposerAgentStartHint } from "./composer-agent-start-hint";

const HINT_TEST_ID = "composer-agent-start-hint";

afterEach(cleanup);

describe("ComposerAgentStartHint", () => {
  it("renders the hint line when shown and the composer accepts sends", () => {
    render(<ComposerAgentStartHint show />);
    expect(screen.getByTestId(HINT_TEST_ID)).toBeTruthy();
  });

  it("renders nothing when hidden", () => {
    render(<ComposerAgentStartHint show={false} />);
    expect(screen.queryByTestId(HINT_TEST_ID)).toBeNull();
  });

  it("renders nothing while a recovery is required (the composer is disabled)", () => {
    render(<ComposerAgentStartHint show needsRecovery />);
    expect(screen.queryByTestId(HINT_TEST_ID)).toBeNull();
  });

  it("renders nothing while the executor environment is unavailable", () => {
    render(<ComposerAgentStartHint show executorUnavailable />);
    expect(screen.queryByTestId(HINT_TEST_ID)).toBeNull();
  });

  it("renders nothing while a clarification is pending (sends only enqueue)", () => {
    render(<ComposerAgentStartHint show hasPendingClarification />);
    expect(screen.queryByTestId(HINT_TEST_ID)).toBeNull();
  });
});

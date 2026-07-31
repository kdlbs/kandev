import { describe, expect, it, vi } from "vitest";
import {
  DEFAULT_SCALE,
  calculateMermaidFitScale,
  getElementContentWidth,
  reportMermaidRenderFailure,
  sanitizeMermaidCode,
} from "./mermaid-utils";

describe("calculateMermaidFitScale", () => {
  it("keeps the default scale when the diagram already fits", () => {
    expect(calculateMermaidFitScale({ viewportWidth: 900, svgWidth: 800 })).toBe(DEFAULT_SCALE);
  });

  it("shrinks below the default scale when the diagram is wider than the viewport", () => {
    expect(calculateMermaidFitScale({ viewportWidth: 600, svgWidth: 1200 })).toBe(0.5);
  });

  it("clamps to the minimum scale for extremely wide diagrams", () => {
    expect(calculateMermaidFitScale({ viewportWidth: 100, svgWidth: 4000 })).toBe(0.1);
  });

  it("uses the default scale until valid measurements are available", () => {
    expect(calculateMermaidFitScale({ viewportWidth: 0, svgWidth: 1200 })).toBe(DEFAULT_SCALE);
    expect(calculateMermaidFitScale({ viewportWidth: 600, svgWidth: 0 })).toBe(DEFAULT_SCALE);
  });
});

describe("getElementContentWidth", () => {
  it("subtracts horizontal padding from the element client width", () => {
    const el = document.createElement("div");
    el.style.paddingLeft = "10px";
    el.style.paddingRight = "5px";
    Object.defineProperty(el, "clientWidth", { value: 100, configurable: true });

    document.body.appendChild(el);
    try {
      expect(getElementContentWidth(el)).toBe(85);
    } finally {
      el.remove();
    }
  });
});

describe("sanitizeMermaidCode", () => {
  it("escapes a literal semicolon in the reported sequence-message prose", () => {
    const input = [
      "sequenceDiagram",
      "    participant KafkaV4 as Document BusinessEvents V4",
      "    participant Subscription as DocumentMessageSubscription",
      "    participant Consumer as DocumentFlowNotificationConsumer",
      "    participant State as DocumentFlowStateMachine",
      "    participant DB as DocumentFlowDatabaseMediator / SingleStore",
      "    participant Publisher as ContextProcessEventsPublisher",
      "    participant KafkaV3 as Context Results V3",
      "",
      "    KafkaV4->>Subscription: Code=reasonCode.ToString(), Message=error.Message",
      "    Subscription->>Consumer: SingleInputMessage<DocumentBusinessResponse>",
      "    Consumer->>State: ProcessNextActionAsync(response)",
      "    State->>State: Locate numeric code string; preserve message unchanged",
      "    State->>DB: ErrorReason=reasonCode string, ErrorMessage=message",
      "    Consumer->>Publisher: PublishDocumentActionUpdatedAsync(...)",
      "    Publisher->>DB: Reload document actions",
      "    DB-->>Publisher: Persisted canonical/legacy rows",
      "    Publisher->>Publisher: Defensive rollout normalization",
      "    Publisher->>KafkaV3: ExecutionResult.Code=reasonCode string, Reason=message",
    ].join("\n");

    expect(sanitizeMermaidCode(input)).toBe(
      input.replace("string; preserve", "string#59; preserve"),
    );
  });

  it("does not double-encode an existing sequence-message semicolon escape", () => {
    const input = "sequenceDiagram\n  A->>B: Keep #59; as text";

    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("preserves a semicolon that separates inline sequence messages", () => {
    const input = "sequenceDiagram\n  A->>B: First message; B->>A: Second message";

    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("escapes prose that starts with a sequence keyword but is not a complete statement", () => {
    const input = "sequenceDiagram\n  A->>B: Retry; break if the server is unavailable";

    expect(sanitizeMermaidCode(input)).toBe(
      "sequenceDiagram\n  A->>B: Retry#59; break if the server is unavailable",
    );
  });

  it("escapes prose that begins with end", () => {
    const input = "sequenceDiagram\n  A->>B: Notify; end users receive updates";

    expect(sanitizeMermaidCode(input)).toBe(
      "sequenceDiagram\n  A->>B: Notify#59; end users receive updates",
    );
  });

  it("preserves CRLF line endings while escaping sequence-message prose", () => {
    const input = "sequenceDiagram\r\n  A->>B: First; second\r\n  B->>A: Done";

    expect(sanitizeMermaidCode(input)).toBe(
      "sequenceDiagram\r\n  A->>B: First#59; second\r\n  B->>A: Done",
    );
  });

  it("leaves semicolons in non-sequence diagrams unchanged", () => {
    const input = "flowchart TD\n  A[First; second] --> B";

    expect(sanitizeMermaidCode(input)).toBe(input);
  });
});

describe("sanitizeMermaidCode labels", () => {
  it("leaves a pre-quoted bracket label with parens inside untouched", () => {
    const input = `D --> E["router.push('/github')"]`;
    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("quotes a bracket label containing slashes alongside a pre-quoted neighbour", () => {
    const input = `A[plain] --> B[Types 'github' / 'pr' / 'dashboard']\nD --> E["router.push('/github')"]`;
    const out = sanitizeMermaidCode(input);
    expect(out).toContain(`B["Types 'github' / 'pr' / 'dashboard'"]`);
    expect(out).toContain(`E["router.push('/github')"]`);
  });

  it("leaves an init directive with single quotes untouched", () => {
    const input = `%%{init: {'theme': 'neutral'}}%%`;
    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("quotes a bracket label containing parens", () => {
    const input = `Action[reorderSidebarViews(activeId, overId)]`;
    expect(sanitizeMermaidCode(input)).toBe(`Action["reorderSidebarViews(activeId, overId)"]`);
  });

  it("quotes a bracket label containing arrow `->`", () => {
    const input = `SSR[fetchUserSettings -> mapUserSettingsResponse]`;
    expect(sanitizeMermaidCode(input)).toBe(`SSR["fetchUserSettings -> mapUserSettingsResponse"]`);
  });

  it("quotes a standalone stadium node containing `/`", () => {
    const input = `X(/api/v1)`;
    expect(sanitizeMermaidCode(input)).toBe(`X("/api/v1")`);
  });

  it("quotes an edge label containing `/`", () => {
    const input = `A -->|/path/to/x| B`;
    expect(sanitizeMermaidCode(input)).toBe(`A -->|"/path/to/x"| B`);
  });

  it("leaves a plain stadium node alone", () => {
    const input = `Y(plain text)`;
    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("does not corrupt parens inside a bracket label after the bracket pass quotes it", () => {
    // Pass 1 wraps `[fetch(/api/x)]` -> `["fetch(/api/x)"]`. Pass 3 must skip the
    // newly-quoted region rather than re-wrapping `(/api/x)` and producing nested quotes.
    const input = `Z[fetch(/api/x)]`;
    expect(sanitizeMermaidCode(input)).toBe(`Z["fetch(/api/x)"]`);
  });

  it("renders the full reported case 1 diagram without nested quotes", () => {
    const input = [
      `%%{init: {'theme': 'neutral'}}%%`,
      `flowchart TD`,
      `    A[User opens Cmd+K panel] --> B[Types 'github' / 'pr' / 'dashboard']`,
      `    D --> E["router.push('/github')"]`,
    ].join("\n");
    const out = sanitizeMermaidCode(input);
    expect(out).not.toContain(`("'`);
    expect(out).toContain(`E["router.push('/github')"]`);
    expect(out).toContain(`B["Types 'github' / 'pr' / 'dashboard'"]`);
  });

  it("quotes a stadium node next to a bracket-with-parens on the same line", () => {
    // Pass 1 quotes `[fn(x)]` (parens inside bracket). Pass 3 must still quote
    // the adjacent stadium `(/api/v1)` — the new quoted range from pass 1 must
    // not leak past its actual close and suppress the unrelated stadium node.
    const input = `A[fn(x)] --> B(/api/v1)`;
    expect(sanitizeMermaidCode(input)).toBe(`A["fn(x)"] --> B("/api/v1")`);
  });

  it("does not let an unterminated quote leak across lines", () => {
    // Line 1 has a stray `"` with no closing pair on the same line. The newline
    // guard in findQuotedRanges discards that opener instead of pairing it with
    // a `"` on a later line, so the bracket label on line 2 still gets quoted.
    const input = `%% stray " comment\nB[/api/x]`;
    const out = sanitizeMermaidCode(input);
    expect(out).toContain(`B["/api/x"]`);
  });

  it("does not match an edge label that spans a newline", () => {
    const input = `A -->|open\nB | C`;
    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("does not match a bracket label that spans a newline", () => {
    const input = `A[open\nB]`;
    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("does not match a parenthesis label that spans a newline", () => {
    const input = `A(open\n/path)`;
    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("does not quote ER diagram cardinality bars as flowchart edge labels", () => {
    const input = [
      "erDiagram",
      "workspaces ||--o{ workflows : owns",
      "workflows ||--o{ workflow_steps : contains",
    ].join("\n");

    expect(sanitizeMermaidCode(input)).toBe(input);
  });
});

describe("reportMermaidRenderFailure", () => {
  it("logs a copyable parser error with original and normalized diagram source", () => {
    const error = new Error("Parse error on line 2");
    const original = "sequenceDiagram\n  A->>B: First; second";
    const normalized = "sequenceDiagram\n  A->>B: First#59; second";
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    reportMermaidRenderFailure(error, original, normalized);

    expect(consoleError).toHaveBeenCalledOnce();
    expect(consoleError).toHaveBeenCalledWith(
      expect.stringContaining("[mermaid] Failed to render diagram"),
    );
    expect(consoleError).toHaveBeenCalledWith(expect.stringContaining(error.message));
    expect(consoleError).toHaveBeenCalledWith(
      expect.stringContaining(`Original diagram:\n${original}`),
    );
    expect(consoleError).toHaveBeenCalledWith(
      expect.stringContaining(`Normalized diagram:\n${normalized}`),
    );
  });

  it("omits the normalized source when no normalization changed it", () => {
    const source = "sequenceDiagram\n  A->>B: Valid";
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    reportMermaidRenderFailure(new Error("Parse error"), source, source);

    expect(consoleError).toHaveBeenCalledWith(expect.not.stringContaining("Normalized diagram:"));
  });
});

import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ScheduleSelector } from "./schedule-selector";
import { WebhookConfig } from "./trigger-configs/webhook-config";
import { WebhookCreatedDialog } from "./webhook-created-dialog";

vi.mock("@/lib/api/domains/automation-api", () => ({
  revealWebhookSecret: vi.fn().mockResolvedValue({ webhook_secret: "s3cret" }),
}));

afterEach(cleanup);

/**
 * The three help sentences in this directory are `<Trans>` blocks whose `<n>`
 * indices address `<code>` elements positionally, and every cron expression,
 * header name and placeholder path travels as an interpolation VALUE so the
 * pseudo-locale cannot accent it into syntax that no longer parses.
 *
 * Both halves fail silently. A drifted index renders duplicated fragments with
 * empty tags, and `check-trans-indices.mjs` only proves each `<n>` lands on an
 * element — not that the reassembled sentence is the one the surface used to
 * render. A token moved into the message body would only show up in a
 * translated build. So each test reconstructs the whole sentence and asserts
 * the syntax landed inside a `<code>`.
 *
 * `{{webhook.<path>}}` additionally checks that i18next does not re-interpolate
 * a substituted value: the placeholder's own braces must survive verbatim.
 *
 * One thing these do NOT detect, deliberately: permuting the `<n>` indices among
 * themselves. Every child here is an identical self-closing `<code />`, so any
 * permutation renders byte-identically — the drift docs/i18n.md warns about
 * needs children that differ. Probing with an index swap therefore proves
 * nothing; the mutations these were verified against are a value baked into the
 * message body, a dropped tag, and a reworded sentence.
 */
function codeTexts(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll("code")).map((el) => el.textContent ?? "");
}

function text(el: Element | null): string {
  return (el?.textContent ?? "").replace(/\s+/g, " ").trim();
}

function renderSchedule(onChange = vi.fn()) {
  return render(
    <TooltipProvider>
      <ScheduleSelector config={null} onChange={onChange} />
    </TooltipProvider>,
  );
}

describe("ScheduleSelector syntax help", () => {
  it("renders the @every help as one sentence with each expression in a code element", () => {
    const { container } = renderSchedule();

    const paragraphs = Array.from(container.querySelectorAll("p"));
    const help = paragraphs.find((p) => p.textContent?.includes("@every"));
    expect(text(help ?? null)).toBe(
      "Use @every with a duration (e.g., @every 10m, @every 2h30m) or shorthands like @hourly, @daily, @weekly.",
    );
    expect(codeTexts(container)).toEqual([
      "@every",
      "@every 10m",
      "@every 2h30m",
      "@hourly",
      "@daily",
      "@weekly",
    ]);
  });

  it("labels the interval presets from count rather than a baked-in plural", () => {
    const { container } = renderSchedule();

    const labels = Array.from(container.querySelectorAll("button")).map((b) => text(b));
    expect(labels).toEqual(["5 min", "15 min", "30 min", "1 hour", "6 hours", "Daily", "Weekly"]);
  });

  it("keeps @every verbatim in the invalid-expression error", async () => {
    const { container, findByText } = renderSchedule();

    const input = container.querySelector('[data-testid="schedule-custom-input"]')!;
    fireEvent.change(input, { target: { value: "not a cron" } });
    fireEvent.blur(input);

    // The token travels as an interpolation value, so an accented pseudo build
    // still tells the user to type the string the scheduler actually accepts.
    await findByText(
      "Invalid expression. Use @every with a duration, a shorthand, or a 5-field cron.",
    );
  });

  it("keeps the cron expression out of the preset's persisted value", () => {
    const { container } = renderSchedule();

    // The testid carries the expression, so it doubles as a check that the
    // persisted value is the untranslated shorthand and not the label.
    expect(container.querySelector('[data-testid="schedule-preset-@hourly"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="schedule-preset-@every 6h"]')).not.toBeNull();
  });
});

describe("WebhookConfig usage help", () => {
  it("renders one sentence with the header and both placeholder paths in code elements", async () => {
    const { container, findByText } = render(<WebhookConfig automationId="a1" workspaceId="w1" />);
    await findByText("Webhook secret");

    const help = Array.from(container.querySelectorAll("p")).find((p) =>
      p.textContent?.includes("POST"),
    );
    expect(text(help ?? null)).toBe(
      "Send a POST request with a JSON body and the secret in the X-Webhook-Secret header. " +
        "Reference fields from the payload with {{webhook.<path>}}, e.g. " +
        "{{webhook.pull_request.number}}.",
    );
    // The placeholder paths carry their own `{{ }}`. i18next must substitute
    // them verbatim and not treat the result as another interpolation — an
    // empty or mangled path here is the failure this asserts against.
    expect(codeTexts(container)).toEqual(
      expect.arrayContaining([
        "X-Webhook-Secret",
        "{{webhook.<path>}}",
        "{{webhook.pull_request.number}}",
      ]),
    );
  });
});

describe("WebhookCreatedDialog description", () => {
  it("renders one sentence with the secret header in a code element", () => {
    render(
      <WebhookCreatedDialog
        open
        webhookUrl="http://localhost:3000/api/v1/automations/webhook/a1"
        webhookSecret="s3cret"
        onClose={vi.fn()}
      />,
    );

    // Radix portals dialog content to document.body, so scope to that.
    const body = document.body;
    const description = body.querySelector("[data-slot='dialog-description']") ?? body;
    expect(text(description)).toContain(
      "Configure your external system to POST to this URL with the secret in the X-Webhook-Secret header.",
    );
    expect(codeTexts(body)).toContain("X-Webhook-Secret");
  });
});

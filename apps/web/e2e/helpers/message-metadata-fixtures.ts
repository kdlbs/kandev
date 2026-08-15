/**
 * Shared fixtures for the message-metadata overflow E2E specs. Keeping the
 * overflow shape in one place prevents the desktop and mobile specs from
 * silently drifting apart when the fixture data changes.
 */

/** The seeded message body both overflow specs render. */
export const SEEDED_MESSAGE = "Message metadata overflow fixture";

/**
 * Builds a `turn_metadata` payload large enough to overflow the metadata
 * dialog's height cap: a 40-option `runtime_config_snapshot` plus usage and
 * agent fields, mirroring what real turns persist.
 */
export function largeTurnMetadata(): Record<string, unknown> {
  return {
    runtime_config_snapshot: {
      config_baseline: {
        mode: "default",
        model: "anthropic/claude-sonnet-5",
        thinking: "auto",
      },
      config_options: Array.from({ length: 40 }, (_, i) => ({
        id: `opt_${i}`,
        name: `Option ${i}`,
        value: `v${i}`,
        value_name: `Value ${i}`,
      })),
    },
    prompt_usage: { input_tokens: 1234, output_tokens: 5678 },
    agent_id: "agent-123",
    agent_type: "task",
  };
}

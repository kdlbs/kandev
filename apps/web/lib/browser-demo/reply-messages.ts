import type { Message, Task, TaskSession } from "@/lib/types/http";
import { makeMessage } from "./scenario";

// i18n-exempt: browser demo fixture data is intentionally literal demo content
const REPLIES = [
  "I reviewed the current implementation and identified the next useful checks.\n\n- Cover the main user path.\n- Verify the failure and retry cases.\n- Keep the change scoped for review.",
  "The next step is a focused regression test. I would start with the expected result, reproduce the edge case, and then run the relevant suite.\n\n```bash\npnpm test\n```",
  "I checked the repository context. My review would focus on input validation, error handling, and whether existing callers retain the same behavior.\n\nThe follow-up is ready for your review.",
  "Here is the proposed follow-up:\n\n1. Trace the current behavior.\n2. Update the implementation and its tests together.\n3. Review the diff before merging.\n\nI am ready for your next instruction.",
];

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export function makeDemoReplyMessages(
  task: Task,
  session: TaskSession,
  user: Message,
  previousVariant?: number,
): Message[] {
  let variant = Math.floor(Math.random() * REPLIES.length);
  if (variant === previousVariant) variant = (variant + 1) % REPLIES.length;
  const turnId = `${user.id}-reply`;
  return [
    makeMessage(
      `${turnId}-thinking`,
      session.id,
      task.id,
      "agent",
      `Reviewing your follow-up for ${task.title}.`,
      {
        type: "thinking",
        turnId,
        metadata: {
          thinking:
            "I am checking the repository context and the earlier discussion before choosing the next step.",
        },
      },
    ),
    makeMessage(`${turnId}-read`, session.id, task.id, "agent", "Read README.md", {
      type: "tool_read",
      turnId,
      metadata: {
        status: "complete",
        normalized: {
          read_file: {
            file_path: `${session.worktrees?.[0]?.worktree_path ?? session.worktree_path}/README.md`,
            offset: 1,
            limit: 8,
            output: { line_count: 8, language: "markdown", truncated: false },
          },
        },
      },
    }),
    makeMessage(
      `${turnId}-answer`,
      session.id,
      task.id,
      "agent",
      `For **${task.title}**:\n\n${REPLIES[variant]}`,
      {
        turnId,
        metadata: { demo_reply_variant: variant },
      },
    ),
  ];
}

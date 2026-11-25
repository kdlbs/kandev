import { describe, expect, it } from "vitest";
import { splitMarkdownPromptMentionSegments } from "@/components/task/chat/messages/prompt-mention-components";
import { protectTaskMentionMarkdown } from "./task-mention-markdown";

const task = { title: "Fix **retry** loop", taskId: "task-1" };
const mention = `@${task.title}`;

function prepare(source: string) {
  return protectTaskMentionMarkdown(
    source,
    splitMarkdownPromptMentionSegments(source, [task.title]),
    [task],
  );
}

describe("render-only task mention protection", () => {
  it("protects the full title without changing surrounding Markdown or entities", () => {
    const prepared = prepare(`before &amp; ${mention} after`);
    const [token, reference] = [...prepared.tokens][0] ?? [];
    expect(reference).toEqual(task);
    expect(prepared.content).toBe(`before &amp; @${token} after`);
  });

  it("keeps authored code and links intact while protecting a separate mention", () => {
    const unchanged = `🙂 &amp; \`${mention}\` [${mention}](/docs) ![${mention}](/image.png)`;
    const prepared = prepare(`${unchanged} and ${mention}`);
    const [token] = prepared.tokens.keys();
    expect(prepared.content).toBe(`${unchanged} and @${token}`);
    expect([...prepared.tokens.values()]).toEqual([task]);
  });

  it("does not reuse a placeholder that occurs in the original message", () => {
    const first = prepare(mention);
    const [token] = first.tokens.keys();
    expect(token).toBeDefined();
    const next = prepare(`keep @${token} and ${mention}`);
    expect(next.content).toContain(`keep @${token} and `);
    expect(next.tokens.has(token)).toBe(false);
    expect([...next.tokens.values()]).toEqual([task]);
  });

  it("reuses a task token for repeated references and preserves outer formatting", () => {
    const prepared = prepare(`**${mention}** and ${mention}`);
    const [token] = prepared.tokens.keys();
    expect(prepared.content).toBe(`**@${token}** and @${token}`);
    expect(prepared.tokens.size).toBe(1);
  });

  it.each([`$x + ${mention} + y$`, `$$\n${mention}\n$$`])(
    "does not put render tokens inside mathematical source",
    (math) => {
      const prepared = prepare(`${math}\n\nand ${mention}`);
      const [token] = prepared.tokens.keys();
      expect(prepared.content).toBe(`${math}\n\nand @${token}`);
      expect([...prepared.tokens.values()]).toEqual([task]);
    },
  );
  it("preserves the renderer's literal-dollar prose semantics", () => {
    const prepared = prepare(`$ ${mention} $ and ${mention}`);
    const [token] = prepared.tokens.keys();
    expect(prepared.content).toBe(`$ @${token} $ and @${token}`);
  });
});

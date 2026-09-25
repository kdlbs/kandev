import { RuleTester } from "eslint";
import tseslint from "typescript-eslint";
import { describe, expect, it } from "vitest";

import { noTaskLinkBypass } from "./no-task-link-bypass.mjs";

RuleTester.describe = describe;
RuleTester.it = it;

const ruleTester = new RuleTester({
  languageOptions: {
    parser: tseslint.parser as never,
    parserOptions: { ecmaVersion: 2022, sourceType: "module", ecmaFeatures: { jsx: true } },
  },
});

ruleTester.run("no-task-link-bypass", noTaskLinkBypass, {
  valid: [
    { code: 'const prefix = "/t/"; pathname.startsWith(prefix);' },
    { code: 'for (const prefix of ["/t/", "/tasks/"]) pathname.startsWith(prefix);' },
    { code: "const apiPath = `/api/tasks/${taskId}`;" },
    { code: "const officePath = `/office/tasks/${taskId}`;" },
    { code: "router.push(linkToTask(taskId));" },
    {
      code: `import { linkToTask } from "@/lib/links";
        import Link from "@/components/routing/app-link";
        <Link href={linkToTask(taskId)} />;`,
    },
    {
      code: `import { linkToTask } from "@/lib/links";
        import AppLink from "@/components/routing/app-link";
        <AppLink href={linkToTask(taskId)} />;`,
    },
    { code: "<TaskLink taskId={taskId} />;" },
    { code: 'const tasksPath = "/tasks";' },
  ],
  invalid: [
    {
      code: 'const href = "/t/task-1";',
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: "const href = `/tasks/${taskId}`;",
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: 'const href = "/t/" + taskId;',
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: 'const href = condition ? `/t/${taskId}` : "/tasks/fallback";',
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: "const href = condition && `/t/${taskId}`;",
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: '<a href="/t/task-1">Task</a>;',
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: '<Link href="/t/task-1">Task</Link>;',
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: '<AppLink href={"/tasks/task-1"}>Task</AppLink>;',
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: `import { linkToTask as taskHref } from "@/lib/links";
        <a href={taskHref(taskId)}>Task</a>;`,
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: `import { linkToTask } from "@/lib/links";
        const href = linkToTask(taskId);
        <a href={href}>Task</a>;`,
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: `import * as Links from "@/lib/links";
        <a href={Links.linkToTask(taskId)}>Task</a>;`,
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: "window.location.assign(`/tasks/${taskId}`);",
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: "window.location.href = `/t/${taskId}`;",
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: `function openTask(taskId: string) {
        const prefix = "/tasks/";
        const href = prefix + taskId;
        return <a href={href}>Task</a>;
      }`,
      errors: [{ messageId: "taskLinkBypass" }],
    },
    {
      code: `function openTask(taskId: string) {
        const prefix = "/tasks/";
        window.location.assign(prefix + taskId);
      }`,
      errors: [{ messageId: "taskLinkBypass" }],
    },
  ],
});

describe("no-task-link-bypass rule", () => {
  it("keeps the diagnostic focused on first-party task workbench routes", () => {
    expect(noTaskLinkBypass.meta?.messages?.taskLinkBypass).toContain("TaskLink");
  });
});

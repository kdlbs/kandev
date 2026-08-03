"use client";

import { IconInfoCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import type { WorkflowStep } from "@/lib/types/http";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

/**
 * Monaco completion entries for the step-prompt editor. `key` is the
 * substitution token the backend expands (`{{task_prompt}}`) and is never
 * translated; only the description and example are copy, so the list is built
 * at render rather than frozen at module scope where `t()` cannot run.
 */
export function stepPromptPlaceholders(t: (key: string) => string): ScriptPlaceholder[] {
  return [
    {
      key: "task_prompt",
      description: t("workflows:stepPromptPlaceholderTaskPrompt"),
      example: t("workflows:stepPromptPlaceholderExample"),
      executor_types: [],
    },
  ];
}

export function HelpTip({
  text,
  testId,
  ariaLabel,
}: {
  text: string;
  testId?: string;
  ariaLabel?: string;
}) {
  const { t } = useTranslation();
  const label = ariaLabel ?? t("workflows:moreInformation");
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="inline-flex h-5 w-5 shrink-0 cursor-help items-center justify-center rounded-sm text-muted-foreground/50 hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            aria-label={label}
            data-testid={testId}
          >
            <IconInfoCircle className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent className="max-w-xs">{text}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

// `value` is the Tailwind class persisted as `WorkflowStep.color`; only
// `labelKey` is copy, and it resolves at render (see `StepConfigHeader`).
export const STEP_COLORS = [
  { value: "bg-slate-500", labelKey: "workflows:colorGray" },
  { value: "bg-red-500", labelKey: "workflows:colorRed" },
  { value: "bg-orange-500", labelKey: "workflows:colorOrange" },
  { value: "bg-yellow-500", labelKey: "workflows:colorYellow" },
  { value: "bg-green-500", labelKey: "workflows:colorGreen" },
  { value: "bg-cyan-500", labelKey: "workflows:colorCyan" },
  { value: "bg-blue-500", labelKey: "workflows:colorBlue" },
  { value: "bg-indigo-500", labelKey: "workflows:colorIndigo" },
  { value: "bg-purple-500", labelKey: "workflows:colorPurple" },
];

// The `prompt` bodies are deliberately NOT translated: clicking a template
// writes the text into `WorkflowStep.prompt`, which is persisted and sent to
// the agent verbatim. Only the button `labelKey` is copy.
export const PROMPT_TEMPLATES = [
  {
    labelKey: "workflows:templatePlan",
    prompt: `Analyze the task and create a detailed implementation plan.

{{task_prompt}}

INSTRUCTIONS:
1. Break the task into clear, ordered steps
2. For each step, describe what needs to be done and which files are affected
3. Identify potential risks or blockers
4. Estimate relative complexity for each step (low/medium/high)

Output the plan as a numbered list. Be specific about file paths, function names, and the approach for each step. Do NOT implement anything yet — only plan.`,
  },
  {
    labelKey: "workflows:templateCodeReview",
    prompt: `Please review the changed files in the current git worktree.

STEP 1: Determine what to review
- First, check if there are any uncommitted changes (dirty working directory)
- If there are uncommitted/staged changes: review those files
- If the working directory is clean: review the commits that have diverged from the master/main branch

STEP 2: Review the changes, then output your findings in EXACTLY 4 sections: BUG, IMPROVEMENT, NITPICK, PERFORMANCE.

Rules:
- Each section is OPTIONAL — only include it if you have findings for that category
- If a section has no findings, omit it entirely
- Format each finding as: filename:line_number - Description
- Be specific and reference exact line numbers
- Keep descriptions concise but actionable
- Sort findings by severity within each section
- Focus on logic and design issues, NOT formatting or style that automated tools handle

Section definitions:

BUG: Critical issues that will cause runtime errors, crashes, incorrect behavior, data corruption, or logic errors
- Examples: null/nil dereference, race conditions, incorrect algorithms, type mismatches, resource leaks, deadlocks

IMPROVEMENT: Code quality, architecture, security, or maintainability concerns
- Examples: missing error handling, incorrect access modifiers, SQL injection vulnerabilities, hardcoded credentials, tight coupling, missing validation

NITPICK: Significant readability or maintainability issues that impact code understanding
- Examples: misleading variable/function names, overly complex logic that should be refactored, missing critical comments for complex algorithms
- EXCLUDE: formatting, whitespace, import ordering, trivial naming preferences, style issues handled by linters/formatters

PERFORMANCE: Algorithmic or resource usage problems with measurable impact
- Examples: O(n²) where O(n) or O(1) is possible, unnecessary allocations in loops, missing indexes for database queries, blocking I/O in hot paths
- Concurrency-specific: unprotected shared state, missing synchronization, goroutine leaks, missing context cancellation

Now review the changes.`,
  },
  {
    labelKey: "workflows:templateSecurityAudit",
    prompt: `Perform a security audit on the changed files in the current git worktree.

{{task_prompt}}

Review all changes and check for the following categories:

1. **Injection Vulnerabilities**: SQL injection, command injection, XSS, template injection, path traversal
2. **Authentication & Authorization**: Missing auth checks, broken access control, privilege escalation, insecure session handling
3. **Data Exposure**: Hardcoded secrets, credentials in logs, sensitive data in error messages, missing encryption
4. **Input Validation**: Missing or insufficient validation at system boundaries, unsafe deserialization, unrestricted file uploads
5. **Dependency Risks**: Known vulnerable dependencies, unsafe use of third-party libraries
6. **Concurrency Issues**: Race conditions on shared state, TOCTOU bugs, unsafe concurrent access to resources

For each finding, output:
- **Severity**: CRITICAL / HIGH / MEDIUM / LOW
- **Location**: filename:line_number
- **Issue**: What the vulnerability is
- **Impact**: What an attacker could do
- **Fix**: Specific remediation steps

Only report real, actionable findings. Do not flag theoretical issues without evidence in the code.`,
  },
];

export function hasOnEnterAction(step: WorkflowStep, type: string): boolean {
  return step.events?.on_enter?.some((a) => a.type === type) ?? false;
}

export function getTransitionType(step: WorkflowStep): string {
  const action = step.events?.on_turn_complete?.find((a) =>
    ["move_to_next", "move_to_previous", "move_to_step"].includes(a.type),
  );
  return action?.type ?? "none";
}

export function getOnTurnStartTransitionType(step: WorkflowStep): string {
  const action = step.events?.on_turn_start?.find((a) =>
    ["move_to_next", "move_to_previous", "move_to_step"].includes(a.type),
  );
  return action?.type ?? "none";
}

export function getChildrenCompletedTransitionType(step: WorkflowStep): string {
  const action = step.events?.on_children_completed?.find((a) =>
    ["move_to_next", "move_to_previous", "move_to_step"].includes(a.type),
  );
  return action?.type ?? "none";
}

export function hasDisablePlanMode(step: WorkflowStep): boolean {
  return step.events?.on_turn_complete?.some((a) => a.type === "disable_plan_mode") ?? false;
}

export function hasOnExitAction(step: WorkflowStep, type: string): boolean {
  return step.events?.on_exit?.some((a) => a.type === type) ?? false;
}

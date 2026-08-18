"use client";

import { IconInfoCircle } from "@tabler/icons-react";
import type { ReactNode } from "react";
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
  text: ReactNode;
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
// i18n-exempt: seeded step prompts, persisted and sent verbatim to the agent.
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

Output the plan as a numbered list. Be specific about file paths, function names, and the approach for each step. Do NOT implement anything yet - only plan.`,
  },
  {
    labelKey: "workflows:templateCodeReview",
    prompt: `Please review the changed files in the current git worktree.

STEP 1: Determine what to review
- First, check if there are any uncommitted changes (dirty working directory)
- If there are uncommitted/staged changes: review those files
- If the working directory is clean: review ONLY the commits from this branch
  - Run: git remote show origin | grep 'HEAD branch' to find the default branch name
  - Set BASE_REF to origin/<default-branch> using the reported branch
  - Use: git log --oneline $(git merge-base "$BASE_REF" HEAD)..HEAD to list the branch commits
  - Use: git diff $(git merge-base "$BASE_REF" HEAD) to see the cumulative changes
  - Do NOT diff directly against BASE_REF or origin/main/master - that would include unrelated changes if the branch is outdated
- Read each changed file in full - understand surrounding code, not just the diff
- Navigate callers, interfaces, and tests to understand changes end-to-end
- Check git blame on modified sections to understand why code was written a certain way
- Only REPORT issues on code modified in this changeset, but USE the full codebase for context

If a code review skill is available (e.g. /code-review, /review), invoke it instead of using the fallback below.

STEP 2: Review the changes across these layers (skip layers that don't apply):

ARCHITECTURAL FIT (highest priority):

- Changes belong in the correct layer/module and follow the dependency direction used by the codebase
- Business/domain logic is not placed in controllers, transport handlers, repositories, data sources, or infrastructure code
- Controllers handle protocol concerns, use cases orchestrate workflows, repositories define persistence needs, and data sources handle external systems
- Domain/application code does not depend on frameworks, transport models, database models, or vendor-specific types
- Changes do not bypass existing boundaries, duplicate responsibilities, or introduce unnecessary coupling between modules or domains
- New interfaces and abstractions have clear ownership and represent a meaningful boundary, rather than wrapping a single implementation
- Compare with neighbouring features and established patterns, but flag deviations only when they create a real architectural or maintainability problem
- Treat fundamental architectural misplacement or broken dependency direction as a blocker

DATA & STATE MODELLING:

- Domain entities, value objects, DTOs, persistence models, and external API models remain separate where their responsibilities differ
- State transitions and invariants are explicit and cannot create invalid or partially updated state
- There is a single clear source of truth; state or business rules are not duplicated across layers
- Nullability, optional fields, defaults, and invalid combinations are modelled deliberately
- Persistence schemas or transport types are not leaking implementation details into domain/application contracts
- Concurrency, retries, partial failures, and duplicate requests cannot corrupt state or apply transitions more than once
- Backward compatibility, migrations, and mixed-version behaviour are considered when contracts or persisted data change

SECURITY (blockers if found):

- No secrets, tokens, or credentials in code
- Input validation at system boundaries (user input, API handlers, external data)
- No SQL injection, XSS, command injection, or path traversal
- Auth and authorization checks on new endpoints
- No insecure crypto (MD5/SHA1 for passwords, weak random)

LOGIC & CORRECTNESS:

- Edge cases handled (empty input, nil/null, zero, max values)
- Error paths covered and not silently swallowed
- Race conditions or concurrency issues (unprotected shared state, missing synchronization, goroutine leaks)
- Broken invariants - state that can become invalid

PERFORMANCE:

- No N+1 queries (loop with individual DB calls)
- No memory leaks (unclosed connections, streams, listeners)
- Algorithm complexity appropriate for data scale (O(n^2) where O(n) is possible)
- Unnecessary allocations in loops, regex compilation in hot paths, unbounded resource growth
- Prefer structured concurrency (errgroup, conc) over raw primitives

CODE QUALITY:

- No dead code, unused imports, or commented-out code
- Check for orphaned code: if the change refactored or removed callers, grep for functions/types/exports that lost their last consumer
- No speculative code (unused flags, one-off abstractions with single call site)
- No duplicated logic - extract shared helpers or constants
- Deep nesting (>3 levels) - use early returns

AI SLOP DETECTION:

- Comments that restate code or narrate obvious steps
- Unnecessary try/catch that swallow errors or return silent defaults
- as any / as unknown casts to dodge type errors instead of fixing types
- Redundant validation where inputs are already parsed/typed
- Defensive checks abnormal for the area - compare with surrounding code patterns

STEP 3: Output your findings.

Every finding needs: file:line, what's wrong, why it matters, and how to fix it.
Only report findings you're >=80% confident about.

Use these sections (omit empty ones):

## BLOCKER
Must fix before merge - architectural violations, security holes, data loss risk, broken logic, crashes.

- file:line - Description. Why it matters. How to fix.

## SUGGESTION
Recommended but doesn't block - architecture, performance, maintainability, or specific missing tests.

- file:line - Description. Why it matters. How to fix.

End with a verdict: Ready to merge / Ready with suggestions / Blocked - fix blockers first

NOT A FINDING (skip these):

- Issues on lines or files the change didn't modify - even if they are real bugs
- Pre-existing code patterns that this change didn't introduce
- Things linters, typecheckers, or CI catch (imports, types, formatting)
- Intentional functionality changes directly related to the task
- Issues explicitly suppressed in code (lint-ignore, nolint comments)
- Pedantic nitpicks a senior engineer wouldn't flag
- General "add more tests" without specifying what logic is untested

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

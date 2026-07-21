# Enhance Prompt Result Delivery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every successful sessionless Improve prompt with AI request visibly
apply its result or retain it for an explicit Apply or Copy action.

**Architecture:** Keep the sessionless host-utility API unchanged. Refactor the
shared web generator so a caller's result-delivery callback is awaited before its
loading flag clears. A focused frontend helper will compare the source snapshot
with the current editor value, apply only unchanged targets through each editor's
normal update API, and retain changed or unavailable results in local state. A
small shared recovery control gives the user explicit Apply and Copy choices.

**Tech Stack:** React, TypeScript, Vitest, React Testing Library, existing
`@kandev/ui` components and `useToast`.

## Global Constraints

- Scope only sessionless `enhance-prompt` delivery plus shared-hook regression
  compatibility; do not change the backend utility architecture, retries,
  provider authentication, or model choice.
- Never write a generated result over input changed after generation started.
- Do not expose raw prompt or generated text in diagnostics or logs.
- A completed result must be applied or retained behind an explicit Apply or
  Copy control; a toast alone is not recovery.
- Keep existing failed HTTP/provider error toasts and VCS generator behavior.
- Use native `@kandev/ui` components and keep every action button
  `cursor-pointer`.

---

## Planned file structure

| File | Responsibility |
| --- | --- |
| `apps/web/hooks/use-utility-agent-generator.ts` | Await result delivery and expose a typed generated-result payload. |
| `apps/web/hooks/use-utility-agent-generator.test.tsx` | Prove success ordering, rejected application, and API failures. |
| `apps/web/hooks/use-prompt-result-delivery.ts` | Keep pending result, prevent stale overwrite, and provide explicit apply/copy operations. |
| `apps/web/hooks/use-prompt-result-delivery.test.ts` | Prove unchanged, changed, and missing-target delivery outcomes. |
| `apps/web/components/prompt-result-recovery.tsx` | Render the generic inline Apply/Copy control without rendering raw prompt text. |
| `apps/web/components/prompt-result-recovery.test.tsx` | Prove the user can apply or copy the retained result. |
| `apps/web/components/task-create-dialog.tsx` | Deliver task description enhancements through the TipTap state API. |
| `apps/web/components/task/new-session-dialog.tsx` | Deliver new-session enhancements safely and render recovery next to its textarea. |
| `apps/web/components/task/new-session-form-prompt.tsx` | Accept the new recovery slot beside the session prompt controls. |
| `apps/web/components/task/use-subtask-submit.ts` | Deliver subtask enhancements safely through the subtask prompt state. |
| `apps/web/components/task/new-subtask-form-parts.tsx` | Render the recovery slot beside the subtask prompt controls. |
| `apps/web/components/task/simple/task-chat.tsx` | Preserve simple-task chat edits made during generation. |
| focused existing component tests | Prove each wired surface receives the same guarded-delivery contract. |
| `apps/web/e2e/tests/task/enhance-prompt.spec.ts` | Stub one successful API response and assert a visible editor result. |

### Task 1: Make shared generator delivery explicit and awaited

**Files:**

- Modify: `apps/web/hooks/use-utility-agent-generator.ts:26-181`
- Create: `apps/web/hooks/use-utility-agent-generator.test.tsx`

**Interfaces:**

- Consumes: `executeUtilityPrompt(request)` returning `success`, `response`,
  optional `call_id`, and optional `duration_ms`.
- Produces:

  ```ts
  export type UtilityGenerationResult = {
    content: string;
    callId?: string;
    durationMs?: number;
  };

  type ResultDelivery = (result: UtilityGenerationResult) => boolean | Promise<boolean>;
  enhancePrompt: (userPrompt: string, deliver: ResultDelivery) => Promise<void>;
  ```

- Preserves: `generateCommitMessage`, `generateCommitDescription`,
  `generatePRTitle`, and `generatePRDescription` accepting their existing
  string callbacks.

- [ ] **Step 1: Write the failing shared-hook tests**

  Create `apps/web/hooks/use-utility-agent-generator.test.tsx` using
  `renderHook`, mocked `executeUtilityPrompt`, `useSessionGitStatus`, and
  `useToast`. Cover the observable contract rather than the animation-frame
  implementation:

  ```tsx
  it("keeps enhance loading until the successful result is delivered", async () => {
    mockExecuteUtilityPrompt.mockResolvedValue({
      success: true,
      response: "improved prompt",
      call_id: "call-123",
      duration_ms: 80_000,
    });
    let releaseDelivery!: () => void;
    const delivered = new Promise<boolean>((resolve) => {
      releaseDelivery = () => resolve(true);
    });
    const { result } = renderHook(() => useUtilityAgentGenerator({ sessionId: null }));

    await act(async () => {
      const pending = result.current.enhancePrompt("original", async (value) => {
        expect(value).toEqual({ content: "improved prompt", callId: "call-123", durationMs: 80_000 });
        expect(result.current.isEnhancingPrompt).toBe(true);
        return delivered;
      });
      releaseDelivery();
      await pending;
    });

    expect(result.current.isEnhancingPrompt).toBe(false);
  });
  ```

  Add one test for a delivery callback returning `false` (it must not produce a
  success toast or replace input itself), plus API-rejection and
  `{ success: false, error }` tests that retain the existing error-toast
  behavior and reset loading.

- [ ] **Step 2: Run the new test and verify the expected red state**

  Run:

  ```zsh
  cd apps && pnpm --filter @kandev/web test -- --run hooks/use-utility-agent-generator.test.tsx
  ```

  Expected: failure because the current callback receives only a string and
  loading clears before that callback runs.

- [ ] **Step 3: Implement the typed awaited delivery contract**

  In `use-utility-agent-generator.ts`, replace the deferred
  `requestAnimationFrame` handoff with an awaited callback. Build the typed
  payload from the API response, await `options.onSuccess(payload)`, and move
  `clearType(type)` to the `finally` block.

  The essential branch must be equivalent to:

  ```ts
  try {
    const resp = await executeUtilityPrompt(buildRequest(type, options));
    if (!resp.success || !resp.response) {
      toast({ title: "Generation failed", description: resp.error || "Failed to generate content", variant: "error" });
      return;
    }
    await options?.onSuccess?.({
      content: resp.response,
      callId: resp.call_id,
      durationMs: resp.duration_ms,
    });
  } catch (error) {
    toast({ title: "Generation failed", description: error instanceof Error ? error.message : "Unknown error", variant: "error" });
  } finally {
    clearType(type);
  }
  ```

  Adapt the four non-enhance wrapper callbacks to forward `result.content` and
  return `true`; define `enhancePrompt` to require the outcome-bearing
  delivery callback. Do not let a `false` delivery outcome create a second,
  generic failure toast because the caller owns the recovery UI.

- [ ] **Step 4: Run the hook test and verify green**

  Run the command from Step 2.

  Expected: PASS; success application is awaited, API failure behavior remains
  visible, and no test requires `requestAnimationFrame`.

- [ ] **Step 5: Commit the focused hook change**

  ```zsh
  git add apps/web/hooks/use-utility-agent-generator.ts apps/web/hooks/use-utility-agent-generator.test.tsx
  git commit -m "fix(web): await utility prompt delivery"
  ```

### Task 2: Add reusable guarded delivery and recovery UI

**Files:**

- Create: `apps/web/hooks/use-prompt-result-delivery.ts`
- Create: `apps/web/hooks/use-prompt-result-delivery.test.ts`
- Create: `apps/web/components/prompt-result-recovery.tsx`
- Create: `apps/web/components/prompt-result-recovery.test.tsx`

**Interfaces:**

- Consumes: source text captured before generation; `getCurrent(): string | null`;
  `apply(value: string): boolean`; and `UtilityGenerationResult` from Task 1.
- Produces:

  ```ts
  type PromptResultDelivery = {
    deliver: (source: string, result: UtilityGenerationResult) => boolean;
    pendingResult: UtilityGenerationResult | null;
    applyPending: () => void;
    copyPending: () => Promise<void>;
    dismissPending: () => void;
  };
  ```

- [ ] **Step 1: Write failing helper and component tests**

  Test the helper with ordinary functions rather than a DOM ref:

  ```ts
  it.each([
    ["original", "original", true, "applies unchanged input"],
    ["original", "edited", false, "retains result after user edit"],
    ["original", null, false, "retains result after target disappears"],
  ])("%s", (source, current, expectedApplied) => {
    const apply = vi.fn(() => true);
    const { result } = renderHook(() => usePromptResultDelivery({ getCurrent: () => current, apply }));
    expect(result.current.deliver(source, GENERATED_RESULT)).toBe(expectedApplied);
    expect(apply).toHaveBeenCalledTimes(expectedApplied ? 1 : 0);
  });
  ```

  Add a component test that renders `PromptResultRecovery` with a pending
  result, clicks Apply and verifies its callback, then clicks Copy and verifies
  `navigator.clipboard.writeText` receives the exact result. Assert user-facing
  copy says only that an enhanced prompt is available; do not render the raw
  result in the alert text.

- [ ] **Step 2: Run the new tests and verify red**

  Run:

  ```zsh
  cd apps && pnpm --filter @kandev/web test -- --run hooks/use-prompt-result-delivery.test.ts components/prompt-result-recovery.test.tsx
  ```

  Expected: failure because neither guarded-delivery helper nor recovery
  component exists.

- [ ] **Step 3: Implement the helper and recovery control**

  Implement the helper as local React state. `deliver` must only call `apply`
  when `getCurrent()` exactly equals the captured source and `apply` returns
  true. Otherwise retain the typed result, show the existing error toast text
  `"Enhanced prompt was generated but could not be inserted."`, and return
  false. `applyPending` is an explicit user action, may replace the current
  editor text, and clears only after `apply` succeeds. `copyPending` writes the
  result to the clipboard, reports copy success/failure through the existing
  toast, and does not log content.

  Render the recovery control with a concise status plus outline `Copy` and
  primary `Apply` buttons; use `aria-live="polite"`, `cursor-pointer` classes,
  and `data-testid="prompt-result-recovery"`. It must not put raw generated
  text into diagnostics, data attributes, or visible prose.

- [ ] **Step 4: Run the helper and component tests and verify green**

  Run the command from Step 2.

  Expected: PASS; changed and unavailable targets retain a recoverable result,
  and Apply/Copy works only after user action.

- [ ] **Step 5: Commit the reusable recovery layer**

  ```zsh
  git add apps/web/hooks/use-prompt-result-delivery.ts apps/web/hooks/use-prompt-result-delivery.test.ts apps/web/components/prompt-result-recovery.tsx apps/web/components/prompt-result-recovery.test.tsx
  git commit -m "fix(web): retain unavailable enhanced prompts"
  ```

### Task 3: Wire every sessionless editor through the guarded delivery contract

**Files:**

- Modify: `apps/web/components/task-create-dialog.tsx:250-265`
- Modify: `apps/web/components/task/new-session-dialog.tsx:169-188,303-463`
- Modify: `apps/web/components/task/new-session-form-prompt.tsx`
- Modify: `apps/web/components/task/use-subtask-submit.ts:151-191`
- Modify: `apps/web/components/task/new-subtask-form-parts.tsx`
- Modify: `apps/web/components/task/simple/task-chat.tsx:281-377`
- Modify/Create: the smallest matching `*.test.tsx` files beside the changed
  session and subtask prompt forms.

**Interfaces:**

- Consumes: `enhancePrompt(source, deliver)` from Task 1 and
  `usePromptResultDelivery` from Task 2.
- Produces: each sessionless Improve prompt button captures its exact source,
  delegates safe application to the helper, shows success acknowledgement after
  immediate insertion, and renders `PromptResultRecovery` when needed.

- [ ] **Step 1: Write failing surface tests**

  Add focused tests that mock a successful utility response and assert each
  surface's editor state, not merely spinner state:

  ```tsx
  await user.click(screen.getByTestId("enhance-prompt-button"));
  await waitFor(() => expect(screen.getByRole("textbox")).toHaveValue("improved prompt"));
  expect(screen.queryByTestId("prompt-result-recovery")).not.toBeInTheDocument();
  ```

  For the changed-input path, resolve the mocked utility promise only after
  changing the editor value. Assert the changed value remains, the recovery
  control appears, and clicking Apply replaces it with the generated result.
  Add one ref-null test through the extracted delivery helper or the relevant
  prompt-zone hook. Retain existing API-failure coverage and assert it never
  replaces editor content.

- [ ] **Step 2: Run the surface tests and verify red**

  Run the exact focused file list selected during Step 1, for example:

  ```zsh
  cd apps && pnpm --filter @kandev/web test -- --run components/task/simple/task-chat.test.tsx components/task/new-session-form-prompt.test.tsx components/task/new-subtask-form-parts.test.tsx
  ```

  Expected: failure because the current callback is deferred and all but simple
  chat silently drop a missing ref; simple chat overwrites changed text.

- [ ] **Step 3: Wire each canonical editor update path**

  At every caller, capture the exact text before starting generation and call
  the shared helper from the awaited delivery callback. Update editor state only
  through its owning API:

  - Task create: `descriptionInputRef.current.getValue()` and `.setValue()`;
    set `hasDescription` only when the setter succeeds.
  - New session: promote the textarea value to the form's prompt state so the
    controlled setter, submit resolver, and `hasPrompt` stay consistent; pass
    the recovery control through `SessionPromptField`.
  - New subtask: use the existing task-form prompt input handle rather than a
    raw textarea assignment; pass the recovery control through the subtask
    prompt form part and make `resolvePrompt` read the same state.
  - Simple task chat: use `input` and `setInput`; a different current value
    must retain, not overwrite, the generated result.

  Immediately applied results show a concise success toast. Changed or absent
  targets render the local recovery control; no raw prompt content goes into
  toast text. Keep active-session TipTap and VCS behavior compiling against the
  shared-hook contract; do not add sessionless recovery UI to unrelated VCS
  generators.

- [ ] **Step 4: Run surface tests and verify green**

  Re-run the exact command from Step 2 after each caller is wired.

  Expected: PASS; all four sessionless surfaces safely apply unchanged text and
  expose recovery when the source no longer matches or the target is absent.

- [ ] **Step 5: Commit the editor wiring**

  ```zsh
  git add apps/web/components/task-create-dialog.tsx apps/web/components/task/new-session-dialog.tsx apps/web/components/task/new-session-form-prompt.tsx apps/web/components/task/use-subtask-submit.ts apps/web/components/task/new-subtask-form-parts.tsx apps/web/components/task/simple/task-chat.tsx apps/web/components/task/*.test.tsx apps/web/components/task/simple/task-chat.test.tsx
  git commit -m "fix(web): recover enhanced prompt results"
  ```

### Task 4: Prove the user path and complete validation

**Files:**

- Modify: `apps/web/e2e/tests/task/enhance-prompt.spec.ts`
- Modify: `docs/enhance-prompt-result-delivery.md` only if implementation
  evidence changes a stated open question or verification command.

**Interfaces:**

- Consumes: sessionless API response with `success`, `call_id`, and `response`.
- Produces: a browser guard proving successful enhancement changes a visible
  editor plus an implementation audit against the investigation requirements.

- [ ] **Step 1: Write the failing browser assertion**

  Add a single route stub to the existing enhancement spec and exercise the
  task-create prompt. Assert the utility request receives the sessionless
  utility agent id and the description field visibly changes to the stubbed
  response. Do not include real user prompt or generated content in test logs.

- [ ] **Step 2: Run the browser assertion and verify red**

  Run:

  ```zsh
  cd apps/web && pnpm e2e -- e2e/tests/task/enhance-prompt.spec.ts
  ```

  Expected before wiring: the test observes spinner completion without the
  expected editor value.

- [ ] **Step 3: Run full affected-package validation**

  Run in order:

  ```zsh
  cd apps
  pnpm --filter @kandev/web test -- --run hooks/use-utility-agent-generator.test.tsx hooks/use-prompt-result-delivery.test.ts components/prompt-result-recovery.test.tsx components/task/simple/task-chat.test.tsx
  pnpm --filter @kandev/web typecheck
  pnpm --filter @kandev/web lint
  cd web && pnpm e2e -- e2e/tests/task/enhance-prompt.spec.ts
  ```

  If `apps/node_modules` is absent, run `pnpm install --frozen-lockfile` from
  `apps/` first, then repeat the same commands. Fix every failure before
  proceeding; do not treat an unavailable test runner as proof.

- [ ] **Step 4: Run the manual smoke check**

  Start the local web app against the released/local service, submit a
  representative large prompt, and verify all of the following:

  1. An unchanged target receives the generated text and a success
     acknowledgement after loading stops.
  2. Editing the target while the request is pending preserves the edit and
     reveals Apply/Copy.
  3. Copy copies the generated text; Apply replaces only after the explicit
     click.
  4. A failed API response leaves the editor unchanged and keeps the existing
     error toast.
  5. No UI diagnostics reveal raw prompt content or utility-call history.

- [ ] **Step 5: Commit the tests and audit-aligned documentation**

  ```zsh
  git add apps/web/e2e/tests/task/enhance-prompt.spec.ts docs/enhance-prompt-result-delivery.md
  git commit -m "test(web): cover enhanced prompt delivery"
  ```

## Plan self-review

- **Spec coverage:** Tasks 1–3 cover awaited delivery, explicit result outcome,
  all four sessionless callers, success acknowledgement, changed/missing target
  recovery, and error preservation. Task 4 covers the required focused tests,
  package checks, browser guard, and manual smoke path. Backend architecture,
  retries, model choice, and raw call history remain explicitly out of scope.
- **Placeholder scan:** no TBD/TODO or unspecified test action remains; every
  task lists exact files, interfaces, red/green commands, and expected behavior.
- **Type consistency:** Task 1's `UtilityGenerationResult` and callback are the
  sole payload consumed by Task 2 and supplied by Task 3; non-enhance wrappers
  preserve their string callback API.

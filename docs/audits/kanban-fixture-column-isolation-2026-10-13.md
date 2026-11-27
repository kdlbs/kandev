# Kanban column visibility fixture isolation

Hosted run 38439998051 spends 180 seconds in `threads-view.spec.ts` while its
setup card disappears. Its snapshot shows the task in the sidebar but no
Review column on the board. The same worker previously ran
`preview-workflow-step-navigation.spec.ts`, whose first case hides the step
after the seeded In Progress step: Review in the simple workflow. That case
does not restore the preference. Shared fixture resets preserve the seeded
workflow and omit `kanban_hidden_step_ids`, so the hidden Review column survives.
Whether the setup click beats the agent's automatic move into Review determines
whether the later test can open the card.

Reset the hidden-column map in both existing shared settings-reset paths.
Each test may still select and persist its own hidden columns after setup;
no product preference or rendering behavior changes. The real Kanban click
remains in the Threads setup helper.

The regression's automatic precondition writes and verifies a hidden column
before the normal page fixture resets settings. Its test body creates a real
task and asserts that the column and task are visible through the board. Before
the repair it fails at the missing column, reproducing the isolation defect
without waiting for a three-minute click timeout. The test uses one browser
case and exercises persisted state through the real API and UI.

After the repair, all 15 cases across the regression, complete preview file and
complete consolidated Threads file pass in the runtime image with one worker
and zero retries. The preview case still successfully hides its column during
its own test. Web typecheck and changed-file ESLint/Prettier pass.

Logs and blob reports are retained in ignored `e2e-focus/column-isolation-*`;
the original hosted snapshot is `threads-open-error-context.markdown` there.
The full-suite performance target still requires a passing hosted run.

---
plan: plan.md
created: 2026-09-23
status: current
---

# Handover: agent permission control integrity

Written for a session picking this work up cold. It records what was done and in
what order, every trap that cost time, and what is still unsettled. The plan and
the work orders hold the durable contract; this file holds the operational
knowledge that is not in them.

## 1. Where the work stands

All eight work orders are `done`. The branch is
`feature/fix-agent-permission-599`, based on `8690df2f7`.

| Order | Commit | What it changed |
| --- | --- | --- |
| 01 auto-approve approves or prompts | `f19a42945` | Auto-approve selects only an allow-kind option; otherwise falls through to the prompt. Also fixed the agentctl log relay. |
| 05 create-task profile validation | `8c22037b7` | `create_task_kandev` rejects an unresolvable `agent_profile_id` before any write. |
| 07 permission mode at session start | `abd1d14c2` | The agent process is configured with its mode before it starts. |
| 04 remove unread `approval_policy` | `d29c43275` | Dead field dropped from the configure contract. |
| 02 confirm and attribute session mode | `fc6f8c352` | Reports the mode the agent is in, plus which layer supplied it. |
| 06 unattended/attended evidence | `239593d8b` | E2E in both directions, accepted against a resolved commit object. |
| 08 copy_files seed reachability | `b9f7ba0a9` | Reports a seed that cannot reach the agent in a multi-repo layout. |
| 03 CLI flag destination | `6cb16e75d` | Refuses a passthrough-only flag on an ACP profile; preview names the destination. |
| plan status | `ec5b3b19b` | Marks the package implemented. |

Requirements and designs:

- `docs/specs/agents/requirements/agent-permission-control-integrity.md`
- `docs/specs/agents/system-design/agent-permission-control-integrity.md`
- `docs/specs/tasks/requirements/mcp-create-task-agent-profile-validation.md`
- `docs/specs/tasks/system-design/mcp-create-task-agent-profile-validation.md`

The designs are still `draft`. Promote them to `current` only after confirming
the implementation still matches, per `/spec-driven-development` phase 5 step 6.

## 2. Environment setup a fresh session needs

This worktree does not inherit a working toolchain. In this order:

1. **Go is not on `PATH`.** `export PATH=/usr/local/go/bin:$PATH`. Without it
   every `go` and `gofmt` invocation fails, including the `gofmt` pre-commit
   hook.
2. **`golangci-lint` is not installed.** The `go-lint` pre-commit hook fails
   with exit 127 until it is:
   `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`,
   then `export PATH=$HOME/go/bin:$PATH`.
3. **`apps/node_modules` is missing** in a fresh worktree. Run
   `pnpm install --frozen-lockfile` from `apps/` before any web command.
4. **The Go module cache may be cold.** `go mod download all` takes several
   minutes; run it in the background before the first `go build ./...` or the
   build times out.

Put all three `export`s in every shell. Compound commands that `cd` between
`apps/backend` and `apps/web` lose them otherwise.

## 3. Traps that cost time

Each of these was hit during the work. They are not hypothetical.

### `NODE_ENV=production` leaks into `make dev` and produces a white page

The agent shell exports `NODE_ENV=production`. `make dev` passes it to the Vite
child. `@vitejs/plugin-react` computes
`skipFastRefresh = isProduction || command === "build" || server.hmr === false`
and omits the Fast Refresh preamble from the HTML, while the JSX transform still
emits `$RefreshSig$()` calls. The first React module throws
`ReferenceError: $RefreshSig$ is not defined`, `#root` stays empty, and the page
is blank with every network request returning 200.

Start the dev instance as `NODE_ENV=development make dev`.

`apps/web/CLAUDE.md` already documents the same variable breaking Vitest with
`React.act is not a function`. The Vite symptom is different and was not
documented; consider forcing `NODE_ENV=development` for the Vite child in
`internal/launcher/dev.go` so the trap cannot recur. That fix was offered and not
yet taken.

### HTTP 200 does not mean the app renders

The Go server serves the SPA shell with 200 even when the SPA dies during boot.
Three consecutive instances were reported as healthy on `curl /` and
`curl /health` while the UI was blank. Verify with a headless check that
`#root` has children:

```js
// run from apps/web so @playwright/test resolves
import { chromium } from "@playwright/test";
const b = await chromium.launch();
const p = await b.newPage();
p.on("pageerror", (e) => console.log("PAGEERROR:", e.message));
await p.goto(process.argv[2], { waitUntil: "load", timeout: 120000 });
await p.waitForTimeout(15000);
console.log(await p.evaluate(() => document.getElementById("root")?.childElementCount));
await b.close();
```

### `pkill -f <pattern>` kills the agent's own shell

The pattern appears in the shell's own command line, so `pkill -f "kandev-launcher dev"`
matches and terminates the invoking shell (exit 143/144) before killing anything
useful. Resolve the PID first with `ps -ef | awk '/[k]andev-launcher dev/ {print $2}'`
— the bracket keeps `awk`'s own line from matching — then `kill` that PID.

### `go mod download all` rewrites `go.sum`

It adds hashes for modules the build does not need. Revert it before committing:
`git checkout -- apps/backend/go.sum`. The build works without them.

### Rewriting locale JSON reorders every key

Loading a locale file, adding a key and dumping it sorted produces a ~1350-line
diff per file. Insert the new key at its alphabetical position while preserving
the original order of everything else. One line per locale is the correct diff
size.

### Size limits reject a commit late

- Go files: 800 lines (revive `file-length-limit`). Hit in
  `cmd/mock-agent/scenarios.go` and
  `internal/agent/settings/controller/profile_crud.go`; both were split into a
  new file.
- Web functions: 100 lines. Hit in `components/task/mode-selector.tsx` and in a
  test file; extract a component or split the `describe`.
- Go `goconst`: a literal repeated three times becomes an error. Reuse the
  package's existing key constants rather than introducing a new one.

Run `golangci-lint run ./... --new-from-rev=8690df2f7 --timeout=12m` from
`apps/backend` before attempting a commit; it is much faster than discovering it
in the hook.

### `golangci-lint` could not lint build-tagged packages

Touching any file under `//go:build e2e` failed the hook with
`build constraints exclude all Go files`. Fixed in `d29c43275` by adding `e2e` to
`run.build-tags` in `apps/backend/.golangci.yml`.

### Radix `asChild` needs a forwarding child

Extracting the mode selector's trigger into a plain function component silently
broke the dropdown: Radix attaches props and a ref through `asChild`, and a
component that ignores them renders an inert button. The test failure reads
`Unable to find an accessible element with the role "menuitem"`. Use
`forwardRef` and spread the incoming props.

### Changing an interface method breaks a runtime type assertion silently

`adapter.ModeSettableAdapter` is asserted at runtime (`a.(adapter.ModeSettableAdapter)`).
Changing `SetMode`'s signature on the concrete adapter compiled fine and would
have failed only at runtime with "agent does not support mode switching".
Update the interface in the same change, and put the shared result type in
`internal/agentctl/types/streams` so both packages can name it.

### The mock agent must be rebuilt before an E2E run

`make build-mock-agent` from `apps/backend`. A new scenario is otherwise absent
from the binary the fixture launches, and the test fails for a reason unrelated
to the change.

### `git commit` inside a mock-agent scenario

The worktree carries no committer identity, so a bare `git commit` exits 1. Pass
`-c user.name=... -c user.email=...`. A repeated run also stages nothing unless
the probe file's content is unique per run.

## 4. Pre-existing failures, measured

None of these were introduced by this package. Do not spend time on them
believing they are regressions; do not treat them as proof that the tree is
clean either.

| Failure | Evidence |
| --- | --- |
| `TestHandleAgentCompleted_BlocksOnTurnCompleteWhileClarificationPending` | Flaky at the base commit: 7/50 failures at `8690df2f7`, 4/50 with this branch. Its `require.Eventually` settles on session state and then asserts the active turn separately; the two observations race. |
| `TestManagerRescanKeepsFocusedWorkspaceFast` | Fails only under full-sweep parallel load. 0/3 in isolation. |
| `TestRunAgentProcessAsync_ObservesStartingSiblingsBeforeProcessStart` | Same: 0/3 in isolation. |
| `TestInstallSystemdWritesOwnerOnlyNativeMetadata`, `TestInstallLaunchdWritesNativeMetadata` | Read the developer machine's real runtime bundle directory, so they fail on any host with Kandev installed. |
| `pnpm run i18n:check` | 32 missing `ja` keys for SSH reachability and launch warnings, from `c7cc92382` landing after the Japanese catalog in `cd08c50ca`. `i18n:ratchet` (the new-code gate) is clean. |

The way to classify a suspected regression: stash with a unique tag, run the
test at base, restore. Never a bare `git stash` — the stack is shared with other
worktrees and sessions.

```bash
TAG="check-$$"
git stash push -u -m "$TAG"
SHA=$(git stash list --format='%H %gs' | grep "$TAG" | head -1 | cut -d' ' -f1)
# ... run the test ...
git stash apply "$SHA"
N=$(git stash list | grep -n "$TAG" | head -1 | cut -d: -f1)
git stash drop "stash@{$((N-1))}"
```

## 5. What the package does not settle

This is the most important section for whoever continues.

**The reporter's own case is not proven fixed.** Work order 06 proves the Kandev
side against the mock agent: an unattended profile commits with nobody
answering, the default profile holds the call until a person does. It does not
prove what a real provider enforces once the mode reaches its process.

**The repository predictor is resolved: a deny rule in the main clone.**
Across thirteen probes every failure was in one repository (`sxBackend`) and
every success in the other two (`sxAiCoop`, `dev-standards`). The cause is not
repository identity and not anything Kandev controls.

Claude Code anchors its project-scoped settings on the **main worktree**, not on
the linked worktree it runs in. For a Kandev worktree,
`git rev-parse --git-common-dir` resolves to the main clone's `.git`, and the
session's own `.claude.json` confirms the anchor: its only `projects` key is the
main clone path (`/home/stefan/SX/sxBackend`), never the worktree path. So the
file that governs an agent in a Kandev worktree is
`<main clone>/.claude/settings.local.json`.

In the failing repository that file is **uncommitted in the main clone** and
carries

```json
"deny": ["Bash(git checkout:*)", "Bash(git commit:*)", "Bash(git merge:*)",
         "Bash(git push:*)", "Bash(git rebase:*)", "Bash(git reset:*)"]
```

which is exactly the refused set, while `git add`, `git branch`, `git status`,
`git rev-parse` and `git remote -v` are absent from it and ran. The main clones
of `sxAiCoop` and `dev-standards` have no deny list, and every probe in them
passed.

A deny rule outranks `bypassPermissions`. The bundled bridge says so itself at
session start:

```
[CLAUDE_SDK_CAN_USE_TOOL_SHADOWED] permissionMode 'bypassPermissions'
auto-approves every tool call (except explicit deny rules) before the callback
is consulted.
```

and the agent transcript records the refusal as `"toolDenialKind":
"permission-rule"` — not a hook, not a prompt, and never a permission request
Kandev could have answered. Probe A is no longer sharp: the human answered a
prompt for some other call while the git writes were refused by a rule.

**Two earlier reads of this evidence were wrong.** The *tracked*
`.claude/settings.local.json` (3391 B, 63 allow entries) that `git worktree add`
materializes has `deny: []` — verified on disk in the Fall 1 worktree and in its
`HEAD`. It explains nothing. Reading `git show HEAD:.claude/settings.local.json`
in the main clone on `main` shows a deny list only because that branch carries an
older version of the file; the rules that actually fire are the uncommitted ones
next to it.

This also invalidated the experiment previously recorded here: a
`prepare_script: rm -rf .claude` in the worktree cannot falsify anything,
because it deletes the wrong file. The two directions that decide it were then
run, unattended, on the same ACP profile (`ec705664`):

| probe | tool calls | denied |
| --- | --- | --- |
| `sxBackend` with `<main clone>/.claude/settings.local.json` moved aside | 4 | 0 |
| a throwaway repository without the file | 3 | 0 |
| the same throwaway repository with the file copied in | 3 | 3 |

Both directions hold. The repository that failed thirteen probes succeeds once
the file is gone, and a repository that always succeeded fails once it is
present. The predictor is that file, not the repository.

**The product question this leaves.** Kandev sets the profile's mode correctly
and can prove it, but a repository's local, uncommitted deny rules silently
outrank that mode, apply to every worktree session of that repository, and are
invisible in the profile UI and in the Kandev logs. Surfacing the effective deny
rules at session start is the reporter's case turned into a Kandev capability;
it is not covered by any work order in this package.

**Work order 03 was narrowed.** The flag list does not render a per-row blocking
warning with a disabled save. The save is refused server-side with an actionable
message, which is what the acceptance criteria require. The inline pre-save
affordance needs the flag catalog's `passthrough_only` surfaced through the
profile editor's own state — the value never reaches the frontend today. The
work order's Scope and ASCII preview sections described the affordance as
shipped until the qualification caught it; both now state what was cut.

**The passthrough path is intended for unattended MCP tasks, and its permission
control is the CLI flag, not the mode.** `create_task_kandev` with `start_agent`
handles a passthrough profile deliberately: the prepare is marked
`DeferredStart` so the prompt-bearing start is not pre-empted
(`task/handlers/task_http_handlers.go`). The unattended control on that path is
`dangerously_skip_permissions`, declared `PassthroughOnly: true` with
`ACPEquivalent: "the profile's permission mode (Bypass permissions)"`, and
AC-001.6 requires the flag to reach the agent CLI's argv. The mode picker is
hidden there because `hasModes` is driven by the ACP session modes an agent
advertises, and a passthrough profile opens no ACP session. So the absent mode
is the declared design, not a gap.

**Measured on the passthrough path: the controls arrive, the path does not
work unattended, and the blocker is upstream of permissions.** Reading
`buildPassthroughEnv` suggests the mode and auto-approve never reach the
process. Measuring two live passthrough sessions says otherwise, and the
measurement wins:

```
argv:  npx -y @anthropic-ai/claude-code --model default \
       --dangerously-skip-permissions --mcp-config <session>.json
env:   CLAUDE_CONFIG_DIR=<data>/agent-sessions/<execution>/.claude
       AGENTCTL_AUTO_APPROVE_PERMISSIONS=true
file:  <that dir>/settings.json -> {"permissions":{"defaultMode":"bypassPermissions"}}
```

So an enabled `cli_flags` entry does reach the agent CLI's argv at launch — AC
001.6 holds beyond the save — and the mode is delivered through the same
settings-file channel as on the ACP path. `AGENTCTL_AUTO_APPROVE_PERMISSIONS`
is set too, though nothing reads it there: no agentctl sits in a passthrough
session. That one is inert but harmless.

Neither probe ran its command. Both stop at Claude Code's workspace trust
dialog, captured verbatim from the PTY:

```
Quick safety check: Is this a project you created or one you trust?
❯ No, exit
  Yes, I trust this folder
Enter to confirm · Esc to cancel
```

The session config Kandev materializes carries no `projects` entry for the new
worktree, so the dialog is unavoidable for a fresh workspace. With the flag
enabled the process exits 1 after 4.15 s and the log reads
`interactive process exited early — likely startup failure`; without it the
process stays alive at the dialog while Kandev records `passthrough turn
complete` and settles the session to `WAITING_FOR_INPUT`. The likely mechanism
for the exit is the injected prompt's newline confirming the highlighted
default, but that is inference; the dialog and the exit code are measured.

**Three gates, measured against Claude Code 2.1.278 in a PTY.** The obvious
repair — let Kandev answer the dialog — was tested rather than argued. Two of
the three gates can be answered before the process starts, by seeding the
config directory Kandev already owns; the third cannot:

| gate on a fresh Kandev session | pre-answerable without a keystroke |
| --- | --- |
| onboarding / theme picker | yes: `hasCompletedOnboarding: true` |
| workspace trust | yes: `projects["<workspace>"].hasTrustDialogAccepted: true` |
| bypass-permissions warning | **no flag found** |

The third gate appears for the mode *and* for `--dangerously-skip-permissions`,
and seeding `numStartups`, a prior `lastSessionId` or the trust entry does not
suppress it. It disappears only on a later start of a config directory where a
human accepted once. Since Kandev materializes a fresh directory per session,
every passthrough session is a first start and meets it again. Its wording is
why that matters: "you accept all responsibility for actions taken while
running in Bypass Permissions mode".

So the options are, in order of how much they ask Kandev to assert on the
operator's behalf: seed the two answerable gates and keep passthrough attended;
seed them and reuse one config directory per profile, accepted once knowingly
by a human, so later unattended sessions start clean; or send the keystroke and
have Kandev accept the responsibility clause itself. All three depend on file
layout that is observed, not published — a CLI update can rename a key, and the
keystroke variant breaks on any change to the screen.

**Therefore the passthrough path is not usable for an unattended MCP task
today, for a reason that no permission control can fix.** By the qualification's
own framing that makes a documented limitation the right answer, plus the
defect-4 treatment: refuse to auto-start an MCP-created task on a passthrough
profile rather than launch a PTY that asks a question nobody can answer. The
permission controls on that path are a second-order concern behind it.

**Auto-approve does leave a trace, under a name the old heuristic did not
look for.** "No `responding to permission request` line means Kandev was never
asked" is false when `auto_approve` is on: that path never reaches the
responder. It emits `auto-approving permission request` from agentctl and
`recorded auto-approved permission` from the orchestrator, with the selected
`option_id`, plus a permission record carrying `AutoApprovedOptionID` so the
session shows what Kandev answered. Measured on this branch: three such lines
for one probe session. Grep for `auto-approv`, not for the responder line.

**Measured on one configuration only.** The qualification exercised
`claude-acp` on the `exec-worktree` executor with a single repository per task.
Not exercised, and therefore residual risk rather than evidence: any other
agent, any other executor (container, SSH, Kubernetes), a multi-repository task,
switching the mode on a session that is already running, and `--resume`. Only
`claude-acp` declares a start-mode channel at all, so every other agent keeps
the post-creation switch and the warning
`session mode will only be applied after the session starts`.

**Ruled out, do not re-investigate.** The `.claude/` directory *inside* the
worktree, in either direction. Its `settings.local.json` is the tracked version
(`deny: []`), and `standards/scripts/maintain_tools.py --setup` rewrites it from
the repository's own `setup_script`, which is why byte-identical copies appear in
worktrees of unrelated repositories that share that submodule. `copy_files` is
empty for every repository in the instance and seeds nothing. None of it carries
a rule that can refuse a tool call.

## 6. Running the test instance

```bash
export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH
cd <worktree>
NODE_ENV=development make dev          # auto-assigned port, printed as "[kandev] url:"
```

State lives in `<worktree>/.kandev-dev/`: `data/kandev.db`, `logs/backend-logs.log`.
The instance is fully isolated from the user's primary Kandev (own process, own
database). The dev profile sets `KANDEV_MOCK_AGENT=true`, so prompts are
`/e2e:<scenario>` and a real provider needs a profile that is not the mock.

The port changes on every start. Tunnel with the literal `127.0.0.1`, not
`localhost` — OpenSSH may resolve `localhost` to `::1` on the remote side and not
fall back:

```bash
ssh -N -L 18080:127.0.0.1:<port> <user>@<host>
```

Reproducing the reported defect 3 needs no agent at all:

```bash
BASE=http://localhost:<port> scripts/verify-create-task-profile-validation.sh
```

That script initializes an MCP session over `POST /mcp`, calls
`create_task_kandev` with `current_task`, `workspace_default` and an unknown
UUID, and prints the task count before and after so the "creates nothing" half
is visible rather than assumed.

## 7. Suggested next steps

1. Decide whether Kandev should surface the effective deny rules of an agent
   session (section 5). The reporter's case is explained without a Kandev
   defect, but a rule in a main clone silently outranking the profile's mode is
   invisible today, and that invisibility is what cost thirteen probes.
2. Open a requirement for the unattended passthrough start (section 5): an
   MCP-created task on a passthrough profile launches a PTY that stops at the
   workspace trust dialog, so it either exits 1 or sits forever. Defect 4's
   treatment applies — refuse the start rather than begin one that cannot
   finish.
3. Decide on forcing `NODE_ENV=development` for the Vite child in the dev
   launcher (section 3). Small, contained, and prevents a whole class of
   "the page is white" reports from agent and container shells.
4. Promote the two system designs from `draft` to `current` once the
   implementation is confirmed to match, and synchronize the affected
   `docs/specs/INDEX.md` entries.
5. Open the PR. Per `planner-orchestration`, do not run `/simplify`, `/qa`,
   `/code-review` or a broad `/verify` first — the configured PR reviewers are
   the semantic gate. Use `/pr-fixup` only for a CI failure or an actionable
   reviewer finding.

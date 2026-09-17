# Custom Kandev candidate and live rollout runbook

Status: operational plan; no cutover has occurred. Commands containing placeholders
must be filled from the reviewed candidate and actual installation. Keep paths,
service environment, database contents and provider credentials in local private
state, not in this repository or PR media.

## 1. Establish the candidate

From a clean private integration/topic branch, record:

```bash
git status --short
git rev-parse HEAD
git merge-base HEAD upstream/release-v0.94.0
```

The initial merge base must be `bf819a0228e742d069c528293d848c985a4d1bd1`.
Complete delivery 01 and the central-view package, then pass the candidate
qualification commands. Choose an immutable version such as
`0.94.0-orchestration.YYYYMMDD.sha<12hex>` and a new directory outside live data.

```bash
make runtime-bundle RUNTIME_VERSION='<candidate-version>' RUNTIME_BUNDLE_DIR='<new-candidate-directory>'
```

Use the Makefile's web/backend/helper packaging and FTS5 build. Hash every artifact
and store a manifest with full commit, base, toolchain and checks. Do not build
into or overwrite the current live bundle. Do not use a dirty-tree version for
the pilot. Capture actual launcher help/version output before choosing its flags.

## 2. Synthetic candidate smoke

Start the exact bundle on a free local port with a fresh isolated data directory,
test configuration and scripted agent provider. A separate browser context must
contain no production cookies/history. Verify health/version, login when enabled,
Kanban access, role/assignment setup, task overview and persistent chat, multiple
assignments, callback/retry, mobile controls and feature combinations.

Verify Orchestration on / Personal assistant off through direct APIs, not only
hidden navigation. Confirm a page visit sends no prompt or delivery task. Test
Automation delivery only with a fixture-owned manually triggered schedule; remove
it afterward. This database is the only source for shareable media.

## 3. Rehearse a private copy of live data

Inventory the effective user service and running binary through `systemctl --user
show`/`cat` locally. Record executable, arguments, working directory, data/config
paths, port, bundle/version environment and every drop-in. Record any configured
update mechanism that could replace the custom bundle, and choose an explicit
update policy for the pilot before cutover. Redact diagnostic
exports. Read active/queued work through supported APIs, not an assumed table layout.

Create a consistent backup using the installation's supported backup path. If no
online consistency mechanism is established, schedule the smallest explicit
maintenance pause; do not copy a live SQLite file while ignoring its WAL. Back up
the whole required data/config set, including database sidecars and blob/attachment
storage, with restrictive permissions and a content manifest. Never upload it.

Restore into a distinct rehearsal home. Block provider and external integration
egress before starting the candidate, disable automations/watchers/schedulers and
run dispatch using supported controls, and verify effective settings. Feature
disablement alone is not a guarantee against every upstream integration job.
If isolation cannot be proved, stop before starting the copied installation.

Run migration, stop cleanly, start again and confirm replay is stable. Compare
counts and stable identifiers for workspaces, workflows, tasks, sessions, messages,
profiles, roles, mappings and retained owners without printing their contents.
Verify authentication/settings and referential integrity, orphan/duplicate checks,
FTS and required-store startup. The live data directory must remain unchanged.

Exercise rollback against another copy: restore the pre-migration backup with
the old exact binary/config and verify startup/history. Record old/new bundle
hashes, migrations and observed invariants. Do not run an old binary against a
migrated database and call that rollback.

## 4. Live change window

Only proceed after an explicit instruction to deploy the named candidate and a
successful rehearsal. The repository request is not itself that instruction.
At the window, confirm no relevant active turn would be interrupted, suspend new
dispatch by supported controls, and record the final pre-cutover state.

1. Stop `kandev.service` and confirm the process exited.
2. Make the final consistent backup of data/config/service drop-ins and the old
   bundle; verify it is readable and record its manifest. This final backup,
   not an earlier rehearsal snapshot, is the rollback pair.
3. Update the effective user-service executable to the immutable candidate path.
   Preserve actual data path, authentication, PATH, port and resource-limit settings.
   Existing `ExecStart` drop-ins can override a freshly installed unit; inspect
   the effective result. Do not assume `make service-install` supersedes them.
4. Set candidate bundle/version metadata consistently, enable Orchestration and
   keep Personal assistant disabled for the first pilot. Other feature settings
   remain the reviewed installation choices.
5. Reload systemd, start the service, verify health/version and effective executable,
   then run the smoke below. Re-enable dispatch/schedules only as intended.

```bash
systemctl --user daemon-reload
systemctl --user start kandev.service
systemctl --user show kandev.service -p ExecStart -p DropInPaths -p ActiveState
```

Do not place environment secrets in commands, screenshots or commit messages.

## 5. Live smoke and initial observation

Check existing login, workspace/task history and execution profile identities
first. Configure one explicit coordinator assignment through supported UI. Send
a generic request in a suitable test conversation and follow one authorized normal
task through its existing review workflow, callback and restart recovery.
Do not import prototype conversations or replace live storage with preview data.

Observe at least one full normal task/review cycle and one restart. Record
candidate version, symptom, redacted event IDs and repeatability; do not store
prompt bodies. Treat wrong-owner exposure, account changes, duplicate dispatch,
unintended automation, missing history or persistent startup/UI failures as stop
conditions. No fixed soak duration substitutes for completing those behaviors.

A later assistant pilot first passes tasks 03–11 and candidate qualification,
uses a proven constrained profile where inspect is enabled, and receives its own
explicit rollout instruction. Merely setting the new toggle is not evidence of
tool enforcement or provider quality.

## 6. Rollback

Stop dispatch and the candidate process. Preserve its new state in a separate
private directory for diagnosis; do not overwrite the original rollback backup.
Restore the matched pre-cutover database/data/config, service drop-ins and exact
old bundle. Reload/start systemd and check health, login, history and task/profile
identities. Explain that post-cutover changes are retained in the diagnostic copy
and may need deliberate reconciliation; database restore cannot undo external
side effects or silently merge new records backward.

Record the failed candidate and rollback result before attempting another build.
Requalification is required after any relevant correction. The normal upstream
updater is a separate release switch and may omit custom changes; do not treat
it as an update mechanism for this private branch.

## 7. Required receipt

Keep a redacted repository receipt with candidate/base SHA, bundle hashes, checks,
date, migration/replay/rollback outcomes, enabled feature identities and deployment
status. Keep actual backup locations, source data, secrets and detailed private
logs only in local protected state. Public screenshots/videos use synthetic data
and are never captured from rehearsal/live data.

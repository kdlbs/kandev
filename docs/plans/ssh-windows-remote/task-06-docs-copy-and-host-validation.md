---
id: "06-docs-copy-and-host-validation"
title: "Public docs, UI copy, spec promotion, and real-host evidence"
status: done
wave: 6
depends_on:
  - "02-port-windows-ssh-lifecycle"
  - "05-windows-native-script-tests"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
  - REQ-EXECUTORS-SSH-WINDOWS-002
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-001.1
  - AC-EXECUTORS-SSH-WINDOWS-001.5
  - AC-EXECUTORS-SSH-WINDOWS-001.11
  - AC-EXECUTORS-SSH-WINDOWS-001.12
  - AC-EXECUTORS-SSH-WINDOWS-001.16
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
---

# Task 06: Public Docs, UI Copy, Spec Promotion, and Real-Host Evidence

## Summary

Document Windows hosts and update the SSH executor descriptions in every
locale. Collect real-host evidence that fakes and CI cannot provide, then
promote the specs.

## In scope

- `/docs-maintainer`: in `docs/public/executors.md`, add a Windows host
  subsection to the SSH section. It covers:
  - Requirements: OpenSSH Server with `sftp`, Windows PowerShell 5.1 or `pwsh`,
    Git for Windows on `PATH`, and agent CLIs.
  - The default shell, and PowerShell-only prepare and cleanup scripts.
  - The `%USERPROFILE%\.kandev` layout.
  - Unsupported uses: Remote Docker, passthrough, `cmd`, POSIX shells, and WSL.
  - The Defender quarantine symptom.
  - The requirement that the SSH user can call WMI `Win32_Process.Create`,
    which the default configuration grants.
- In the Windows host subsection, state that ARM64 Windows hosts are
  unsupported.
- UI copy: update `descriptionSsh`, `sshConnectionDescription`, and
  `sshCreateDescription` in `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/executors.json`.
  - Name Linux, macOS, and Windows hosts, and drop "amd64".
  - Run `pnpm run i18n:zh-hant` for the Traditional Chinese pair.
  - Do not use the U+2014 em dash.
- Real-host matrix: run each scenario on an isolated instance started with
  `scripts/dev-isolated`. Use the authorized Windows test host. It runs Windows
  11 x64 with the built-in OpenSSH Server, `DefaultShell` unset, and PowerShell
  5.1 and 7. Create the SSH executor profile from its SSH host alias.

  | # | Configuration | Scenario | Criterion |
  | --- | --- | --- | --- |
  | H1 | Stock `DefaultShell` (`cmd.exe`) | Test connection shows `windows/amd64`; trust and save | `001.1` |
  | H2 | Stock | Launch a task with the default prepare script; agent answers one prompt | `001.5`, `001.8` |
  | H3 | Stock | Launch returns; wait 2 min with the agent idle; prompt again | `001.11` |
  | H4 | Stock | Stop the backend, restart it, open the task; no replacement launch | `001.11`, `001.12` |
  | H5 | Stock | Archive; the controller stops and the session dir is removed, while the task dir is kept (Windows never reclaims) | `001.13`, `001.18` |
  | H6 | Stock | While H2's session runs, launch with a rebuilt helper (new hash) | `001.6` |
  | H7 | `DefaultShell` = PowerShell (user-approved; restore afterward) | Repeat H2 and H4 | `001.5`, `001.12` |
  | H8 | `pwsh` saved shell | Repeat H2 | `001.3` |

- If any scenario fails, stop and record the failure. Fix it in the owning
  work order, then rerun that work order's checks and this matrix.
- Remove every probe process, task directory, and `%USERPROFILE%\.kandev` entry
  that the matrix created on the test host, unless the user asks to keep them.
- After all checks pass, promote the specs:
  - In `requirements/ssh-windows-remote.md`, change the status from `draft` to
    `active`.
  - In `system-design/ssh-windows-remote.md`, change the status from `draft` to
    `current`.
  - In `system-design/ssh-executor.md`, add Windows to the support matrix with
    its validation status, and drop the Windows out-of-scope bullet.
  - Set this plan to `implemented`.

## Exclusions

- New UI layout or controls.
- Windows passthrough support.

## Acceptance conditions

1. Public docs and all eight locale files describe Windows hosts. i18n checks
   pass.
2. H1 through H8 pass, with results and host versions recorded in Results and
   in PR B's body.
3. Spec status and plan status match the delivered behavior.

## Verification

```bash
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet && pnpm run lint && pnpm run typecheck
cd ../.. && python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

For each host scenario, record:

- Windows build (`[Environment]::OSVersion`)
- OpenSSH version (`sshd -V`)
- PowerShell version (`$PSVersionTable.PSVersion`)
- The pass or fail result, with the relevant backend log lines

## Lifecycle evidence

| Scenario | Criterion | Evidence |
| --- | --- | --- |
| Channel close | `001.11` | H3 |
| Restart and recovery | `001.12` | H4, H7 |
| Concurrent work | `001.6` | H6 |

## Likely files and risks

- `docs/public/executors.md`, eight `executors.json` files, three spec files,
  `plan.md`.
- The test host is a personal machine. On 2026-11-22 the user approved setting
  its OpenSSH `DefaultShell` to PowerShell for H7.
  - Before the change, record the current value. On this host, the
    `HKLM\SOFTWARE\OpenSSH\DefaultShell` value does not exist.
  - Afterward, restore that state: delete the value and restart `sshd` if
    needed. Verify that `echo %OS%` again expands through `cmd.exe`.
  - Any other host configuration change needs a new approval.

## Results

Completed on 2026-11-23.

Docs and copy:

- `docs/public/executors.md` now has Windows host requirements, a "Windows
  hosts" subsection (including the unsupported uses and the Defender note), and
  passthrough and reclamation notes.
- `feature-status.md` and `release-process.md` are updated to match.
- Three `executors.json` keys are updated in all eight locales. `zh-hk` and
  `zh-tw` were generated with `pnpm run i18n:zh-hant`, and `pseudo` with
  `pnpm run i18n:pseudo`. The generators also rewrote three unrelated entries;
  those were reverted, because the `zh-hant` converter regressed a
  hand-corrected phrase.

Real-host matrix: an isolated backend (the `dev-isolated` environment launched
with `nohup`, because macOS lacks `setsid`) drove the product API against the
test host. The host ran Windows 11 x64 with the built-in OpenSSH Server and
PowerShell 5.1 and 7. The agent was Kandev's mock agent
(`/e2e:simple-message`). For the run, `mock-agent.exe` was placed in
`%USERPROFILE%\.local\bin`.

| # | Result |
| --- | --- |
| H1 | Passed. Connection test: `windows/amd64`, with every step OK |
| H2 | Passed. The default prepare cloned `octocat/Hello-World`, the WMI-launched controller became ready, and the mock agent answered (`WAITING_FOR_INPUT`) |
| H3 | Passed. After 120 seconds idle, the same controller (port 41001) answered |
| H4 | Passed. Across a backend stop and restart, the remote PID 53004 was unchanged. The session reattached with a new local forward, and the next prompt was answered in the same session |
| H5 | Passed. Archive stopped PID 53004 and removed the session dir. The task dir and checkout were kept |
| H6 | Passed. A rebuilt helper (new hash) launched task 2. The running PID 53004 kept `agentctl.exe.old-*` |
| H7 | Passed. With `DefaultShell` set to PowerShell (user-approved), launch and restart/resume both worked in the same session. `DefaultShell` was then removed, and `echo %OS%` expanded through `cmd.exe` again |
| H8 | Passed. A `pwsh` connection profile launched and answered |

Cleanup on the test host: all validation tasks were archived, and no
`agentctl.exe` was left running. `%USERPROFILE%\.kandev` held only validation
artifacts and was removed. `mock-agent.exe` and every `%TEMP%\kandev-*` entry
were deleted, and `DefaultShell` is absent.

Findings recorded as known limitations, or as exclusions where noted. Tasks 07
through 09 later resolved all three:

- agentctl's interactive shell cannot start a PTY on Windows
  (`failed to start PTY: unsupported`). The task terminal panel is therefore
  excluded.
- The first-turn turn-change checkpoint timed out once per new session (3
  times). Later turns captured normally.
- Forced stops leave `%TEMP%\kandev-github-cli-*` shim directories.

| Command | Result |
| --- | --- |
| `pnpm run i18n:check`, `i18n:ratchet`, `lint`, `typecheck` | Passed |
| `node scripts/validate-public-docs.mjs`, `node --test scripts/validate-public-docs.test.mjs` | Passed: 56 pages, 63 tests |
| `python3 scripts/list-docs.py validate`, `lint-spec-files.py --all` | Passed |

Specs promoted: the requirement is `active`, the Windows design is `current`,
and `ssh-executor.md` gained Windows in the support matrix and lost its Windows
out-of-scope bullet. The Windows `native` CI job is still pending until PR B
runs. If it fails, fix the cause in PR B before review.

Lifecycle evidence:

| Scenario | Criterion | Evidence | Result |
| --- | --- | --- | --- |
| Channel close | `001.11` | H3 | Passed |
| Restart and recovery | `001.12` | H4, H7 | Passed |
| Concurrent work | `001.6` | H6 | Passed |

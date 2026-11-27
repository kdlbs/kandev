---
id: "06-docs-copy-and-host-validation"
title: "Public docs, UI copy, spec promotion, and real-host evidence"
status: pending
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
  | H5 | Stock | Stop; then archive; session and task dirs are reclaimed | `001.13` |
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

Pending.

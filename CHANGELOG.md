# Changelog

All notable changes to Kandev.

## 0.32 - 2026-04-13

### Features

- add multi-select and drag-to-move for file tree and changes panel ([#490](https://github.com/kdlbs/kandev/pull/490))

### Bug Fixes

- register MCP tools with _kandev suffix to match sysprompt ([#572](https://github.com/kdlbs/kandev/pull/572))
- recalculate dockview layout after fast-path session switch ([#571](https://github.com/kdlbs/kandev/pull/571))
- prevent worktree branches from inheriting upstream tracking ([#570](https://github.com/kdlbs/kandev/pull/570))
- move frontend off port 3000 and silence reverse-proxy panic logs ([#568](https://github.com/kdlbs/kandev/pull/568))
- make PR Approve button look clickable ([#567](https://github.com/kdlbs/kandev/pull/567))
- associate PRs with tasks after branch rename or PR replacement ([#565](https://github.com/kdlbs/kandev/pull/565))

### Documentation

- enforce test requirements and improve agent skill resilience ([#543](https://github.com/kdlbs/kandev/pull/543))

## 0.31 - 2026-04-09

### Bug Fixes

- keep file tree and terminal waiting through long prepare ([#564](https://github.com/kdlbs/kandev/pull/564))
- stop discarding branch selection for local executor ([#558](https://github.com/kdlbs/kandev/pull/558))

## 0.30 - 2026-04-08

### Bug Fixes

- sort nested file tree folders before files ([#562](https://github.com/kdlbs/kandev/pull/562))
- surface backend startup errors and extend health timeout ([#561](https://github.com/kdlbs/kandev/pull/561))
- reduce log noise from expected error states ([#560](https://github.com/kdlbs/kandev/pull/560))
- scroll dropdown selectors inside dialogs ([#559](https://github.com/kdlbs/kandev/pull/559))
- prevent task reorder during silent session resume ([#555](https://github.com/kdlbs/kandev/pull/555))
- persist live git status snapshot for sidebar diff badges ([#556](https://github.com/kdlbs/kandev/pull/556))

## 0.29 - 2026-04-07

### Features

- redesign task sidebar with repo-grouped layout and diff stats ([#550](https://github.com/kdlbs/kandev/pull/550))

### Bug Fixes

- prevent git process pile-up causing excessive CPU usage ([#554](https://github.com/kdlbs/kandev/pull/554))
- handle file paths with spaces in git status and diff parsing ([#552](https://github.com/kdlbs/kandev/pull/552))
- clean up orphaned review PR dedup records when task is already deleted ([#551](https://github.com/kdlbs/kandev/pull/551))
- changed branch and auto focus changes panel ([#549](https://github.com/kdlbs/kandev/pull/549))
- persist PR panel dismissal across page refreshes ([#547](https://github.com/kdlbs/kandev/pull/547))
- respect KANDEV_DATABASE_PATH env var in dev mode ([#548](https://github.com/kdlbs/kandev/pull/548))
- reset stale topbar branch when navigating between tasks ([#546](https://github.com/kdlbs/kandev/pull/546))

## 0.28 - 2026-04-06

### Features

- right-click context menu to move sidebar tasks between steps ([#492](https://github.com/kdlbs/kandev/pull/492))
- prioritize local changes above PR files in changes panel ([#528](https://github.com/kdlbs/kandev/pull/528))
- auto-show PR details panel when task has associated PR ([#517](https://github.com/kdlbs/kandev/pull/517))
- add workflow sorting with drag-and-drop reordering ([#520](https://github.com/kdlbs/kandev/pull/520))
- expose hidden keybindings in settings for user customization ([#521](https://github.com/kdlbs/kandev/pull/521))
- enable pprof memory profiling in dev/debug mode ([#518](https://github.com/kdlbs/kandev/pull/518))
- disable branch selector for local executor and implement base branch checkout ([#515](https://github.com/kdlbs/kandev/pull/515))

### Bug Fixes

- compare ahead/behind counts against base branch instead of remote tracking branch ([#544](https://github.com/kdlbs/kandev/pull/544))
- open embedded VS Code in center group instead of right sidebar ([#545](https://github.com/kdlbs/kandev/pull/545))
- add paragraph spacing to markdown body for visible line breaks ([#540](https://github.com/kdlbs/kandev/pull/540))
- skip git polling when workspace has no valid git repository ([#541](https://github.com/kdlbs/kandev/pull/541))
- always set upstream tracking on git push and fix task worktree startPoint ([#536](https://github.com/kdlbs/kandev/pull/536))
- suppress stale events during session resume history replay ([#527](https://github.com/kdlbs/kandev/pull/527))
- associate PR with task when creating task from PR URL ([#539](https://github.com/kdlbs/kandev/pull/539))
- prevent duplicate task.state_changed events and N+1 git show calls ([#534](https://github.com/kdlbs/kandev/pull/534))
- disable plan mode when moving to next workflow step ([#525](https://github.com/kdlbs/kandev/pull/525))
- stabilize git operation callbacks to fix staging first-click bug ([#535](https://github.com/kdlbs/kandev/pull/535))
- show Push button when task has open PR and unpushed commits ([#537](https://github.com/kdlbs/kandev/pull/537))
- guard against missing referencePanel in dockview focusOrAddPanel ([#538](https://github.com/kdlbs/kandev/pull/538))
- stop runtime instance in CleanupStaleExecutionBySessionID to prevent leaked git polling ([#531](https://github.com/kdlbs/kandev/pull/531))
- remove prompt timeout and prevent auto-resume of errored sessions ([#530](https://github.com/kdlbs/kandev/pull/530))
- increment CI checks elapsed time for in-progress runs in PR panel ([#529](https://github.com/kdlbs/kandev/pull/529))
- enforce headless mode for E2E tests in agent skills ([#526](https://github.com/kdlbs/kandev/pull/526))
- clear stale activeSessionId when switching tasks ([#523](https://github.com/kdlbs/kandev/pull/523))
- add max-height and scrollbar to queue message editor textarea ([#519](https://github.com/kdlbs/kandev/pull/519))
- hide start agent button during preparation and fix auto-start race condition on step move ([#516](https://github.com/kdlbs/kandev/pull/516))
- stop workspace tracker after consecutive git failures ([#514](https://github.com/kdlbs/kandev/pull/514))
- stabilize diff viewer fileRefs to prevent auto-scroll on background updates ([#513](https://github.com/kdlbs/kandev/pull/513))

### Performance

- optimize git clone/fetch for large repos with many tags ([#533](https://github.com/kdlbs/kandev/pull/533))

## 0.27 - 2026-04-01

### Features

- add pipeline enforcement and E2E handling to pr-fixup skill ([#522](https://github.com/kdlbs/kandev/pull/522))
- add dev-first workflow and playwright-cli debugging to e2e skill ([#512](https://github.com/kdlbs/kandev/pull/512))
- sort tasks by creation date in kanban and sidebar ([#511](https://github.com/kdlbs/kandev/pull/511))
- improve skills with pipeline enforcement and skill delegation ([#510](https://github.com/kdlbs/kandev/pull/510))
- rename sidebar sections to "Turn Finished" and "Running" ([#506](https://github.com/kdlbs/kandev/pull/506))

### Bug Fixes

- workspace-scoped PR data loading with cache and singleflight ([#509](https://github.com/kdlbs/kandev/pull/509))
- re-inject plan mode instructions on follow-up prompts ([#507](https://github.com/kdlbs/kandev/pull/507))
- preserve task status when resuming agent after backend restart ([#508](https://github.com/kdlbs/kandev/pull/508))

## 0.26 - 2026-03-31

### Features

- move PR monitoring to backend with lightweight polling ([#502](https://github.com/kdlbs/kandev/pull/502))
- unified commit list and dockview panel fix ([#500](https://github.com/kdlbs/kandev/pull/500))
- fix bottom padding and add font family setting ([#489](https://github.com/kdlbs/kandev/pull/489))
- add proceed button to advance task to next workflow step ([#486](https://github.com/kdlbs/kandev/pull/486))
- add dedicated Utility Agents settings page ([#484](https://github.com/kdlbs/kandev/pull/484))
- introduce subtasks, allow sessions to task reuse executor ([#419](https://github.com/kdlbs/kandev/pull/419))

### Bug Fixes

- break infinite PR sync loop and improve diff panel targeting ([#503](https://github.com/kdlbs/kandev/pull/503))
- invalidate diff expansion cache when file changes ([#501](https://github.com/kdlbs/kandev/pull/501))
- deduplicate agent and session tabs in task view ([#496](https://github.com/kdlbs/kandev/pull/496))
- show both send and cancel buttons when agent is busy ([#487](https://github.com/kdlbs/kandev/pull/487))
- hide duplicate local commits when PR commits exist ([#494](https://github.com/kdlbs/kandev/pull/494))
- reliable PR-task association across all launch paths ([#485](https://github.com/kdlbs/kandev/pull/485))
- complete all non-terminal tool calls when turn ends ([#488](https://github.com/kdlbs/kandev/pull/488))

### Refactoring

- reorganize E2E tests into feature-based subdirectories ([#499](https://github.com/kdlbs/kandev/pull/499))

## 0.25 - 2026-03-29

### Features

- add Feature Dev workflow and improve default workflow prompts ([#481](https://github.com/kdlbs/kandev/pull/481))
- add file-based knowledge system with decision log and plan storage ([#479](https://github.com/kdlbs/kandev/pull/479))
- add commit body field and AI generation for commit description and PR title ([#465](https://github.com/kdlbs/kandev/pull/465))

### Bug Fixes

- update gemini ACP flag and claude-agent-acp package org ([#482](https://github.com/kdlbs/kandev/pull/482))
- fail session with guidance when PR branch is missing ([#466](https://github.com/kdlbs/kandev/pull/466))
- recover workspace operations after backend restart ([#475](https://github.com/kdlbs/kandev/pull/475))
- prevent pointer-events: none from getting stuck on body after dialog navigation ([#474](https://github.com/kdlbs/kandev/pull/474))
- resolve stale execution ID after backend restart ([#473](https://github.com/kdlbs/kandev/pull/473))
- stop workspace tracker when work directory is deleted ([#472](https://github.com/kdlbs/kandev/pull/472))
- prevent dockview layout from not filling viewport after session switch ([#471](https://github.com/kdlbs/kandev/pull/471))
- add queued message indicator to quick chat and e2e tests ([#470](https://github.com/kdlbs/kandev/pull/470))
- wrap long lines in markdown chat messages ([#469](https://github.com/kdlbs/kandev/pull/469))
- resolve symlinks in file tree so symlink-to-directory entries show as folders ([#467](https://github.com/kdlbs/kandev/pull/467))

### Refactoring

- rename /investigate skill to /fix ([#483](https://github.com/kdlbs/kandev/pull/483))
- centralize default prompts and fix PR review scoping ([#476](https://github.com/kdlbs/kandev/pull/476))

## 0.24 - 2026-03-25

### Features

- add collapsible sections to changes panel ([#457](https://github.com/kdlbs/kandev/pull/457))
- collapse chat input toolbar items into overflow menu when narrow ([#459](https://github.com/kdlbs/kandev/pull/459))
- add markdown preview mode and PR screenshot capture ([#461](https://github.com/kdlbs/kandev/pull/461))
- add pr-fixup, pr-ready, and pr-draft skills ([#463](https://github.com/kdlbs/kandev/pull/463))
- add image and file paste/drop support to task creation dialog ([#453](https://github.com/kdlbs/kandev/pull/453))

### Bug Fixes

- merge chat status bar into single row and switch task on archive ([#460](https://github.com/kdlbs/kandev/pull/460))
- add git-crypt support for worktree creation ([#454](https://github.com/kdlbs/kandev/pull/454))
- persist task creation draft when modal closes ([#455](https://github.com/kdlbs/kandev/pull/455))
- add --debug/--verbose to run command and fix web hostname binding ([#452](https://github.com/kdlbs/kandev/pull/452))
- prevent template step events from overwriting backend step_id UUIDs ([#451](https://github.com/kdlbs/kandev/pull/451))
- sanitize mermaid code to handle special characters ([#444](https://github.com/kdlbs/kandev/pull/444))
- pass MCP servers through LoadSession so tools survive session resume ([#450](https://github.com/kdlbs/kandev/pull/450))

### Documentation

- remove beta status and replace screenshots with demo gif ([#462](https://github.com/kdlbs/kandev/pull/462))
- add readme screenshots and update agent protocols to ACP ([#456](https://github.com/kdlbs/kandev/pull/456))

## 0.23 - 2026-03-19

### Features

- make quick chats independent of workflows ([#434](https://github.com/kdlbs/kandev/pull/434))

### Bug Fixes

- improve keyboard navigation for macOS shortcuts ([#448](https://github.com/kdlbs/kandev/pull/448))
- include uncommitted changes in review dialog cumulative diff ([#447](https://github.com/kdlbs/kandev/pull/447))
- show create new task command in dock view command palette ([#446](https://github.com/kdlbs/kandev/pull/446))
- prevent mermaid false positive detection ([#443](https://github.com/kdlbs/kandev/pull/443))
- prevent duplicate workflow on create ([#438](https://github.com/kdlbs/kandev/pull/438))

## 0.22 - 2026-03-14

### Features

- move utility agents to main agents page ([#436](https://github.com/kdlbs/kandev/pull/436))

### Bug Fixes

- add timeouts to agent discovery, health checks, and GitHub CLI ([#440](https://github.com/kdlbs/kandev/pull/440))
- show confirmation dialog when deleting agent profile with active sessions ([#441](https://github.com/kdlbs/kandev/pull/441))
- carry env and headers through MCP server config pipeline ([#439](https://github.com/kdlbs/kandev/pull/439))
- improve claude acp tool messages and model selector flow ([#442](https://github.com/kdlbs/kandev/pull/442))
- store profile IDs in task metadata for deferred auto-start ([#437](https://github.com/kdlbs/kandev/pull/437))
- async workspace preparation and worktree branch fallback ([#433](https://github.com/kdlbs/kandev/pull/433))
- suppress auggie indexing messages in inference mode ([#435](https://github.com/kdlbs/kandev/pull/435))

## 0.21 - 2026-03-13

### Features

- add "Add + Run" button to send comments directly to agent ([#430](https://github.com/kdlbs/kandev/pull/430))
- agent-native config mode ([#396](https://github.com/kdlbs/kandev/pull/396))
- add archive action to task card menu ([#429](https://github.com/kdlbs/kandev/pull/429))

### Bug Fixes

- server-side task ID injection for plan tools and UI polish ([#431](https://github.com/kdlbs/kandev/pull/431))
- skip executor preparer for repo-less tasks like config chat ([#432](https://github.com/kdlbs/kandev/pull/432))

## 0.20 - 2026-03-12

### Features

- show auth methods and login guidance on authentication errors ([#422](https://github.com/kdlbs/kandev/pull/422))
- add ACP-based utility agent inference and generate buttons in changes panel ([#420](https://github.com/kdlbs/kandev/pull/420))
- add bottom terminal panel with Cmd+J hotkey ([#414](https://github.com/kdlbs/kandev/pull/414))
- show hotkey in quick chat button tooltip ([#409](https://github.com/kdlbs/kandev/pull/409))
- add ACP agent variants for Claude, Codex, Copilot, and Amp ([#387](https://github.com/kdlbs/kandev/pull/387))
- add clickable terminal links with configurable open behavior ([#401](https://github.com/kdlbs/kandev/pull/401))
- improve mobile kanban view ([#400](https://github.com/kdlbs/kandev/pull/400))
- auto-update base commit on branch switch ([#399](https://github.com/kdlbs/kandev/pull/399))

### Bug Fixes

- resolve acp chat ux issues with permissions, plans, and tool states ([#428](https://github.com/kdlbs/kandev/pull/428))
- fetch diff expansion content from working tree instead of HEAD ([#427](https://github.com/kdlbs/kandev/pull/427))
- reset attachments when switching chat sessions ([#421](https://github.com/kdlbs/kandev/pull/421))
- resolve CancelAgent race and hide cancel message in clarification recovery ([#423](https://github.com/kdlbs/kandev/pull/423))
- improve chat input height with context and terminal toggle focus ([#425](https://github.com/kdlbs/kandev/pull/425))
- recover stuck sessions after agent stream disconnect ([#424](https://github.com/kdlbs/kandev/pull/424))
- allow dockview layout to shrink on window resize ([#418](https://github.com/kdlbs/kandev/pull/418))
- prevent escape sequence artifacts and scroll on Cmd+J terminal toggle ([#416](https://github.com/kdlbs/kandev/pull/416))
- prevent browser shortcut conflict with bottom terminal toggle ([#415](https://github.com/kdlbs/kandev/pull/415))
- recover from agent MCP timeout during clarification wait ([#413](https://github.com/kdlbs/kandev/pull/413))
- use merge-base for PR review prompts to avoid reviewing unrelated changes ([#412](https://github.com/kdlbs/kandev/pull/412))
- wait for workspace readiness in terminal connections ([#411](https://github.com/kdlbs/kandev/pull/411))
- resolve model selector mismatch for ACP agents ([#410](https://github.com/kdlbs/kandev/pull/410))
- filter pending comments by session to prevent cross-session leakage ([#408](https://github.com/kdlbs/kandev/pull/408))
- use integration branch for base commit calculation in git status ([#407](https://github.com/kdlbs/kandev/pull/407))
- force-load diffs up to selected file for accurate scroll ([#403](https://github.com/kdlbs/kandev/pull/403))
- use kandev home dir for worktrees, repos, sessions instead of data dir ([#405](https://github.com/kdlbs/kandev/pull/405))
- resolve ACP model ID mismatch and promote ACP agents as default ([#404](https://github.com/kdlbs/kandev/pull/404))
- add timeout and retry for git status polling commands ([#402](https://github.com/kdlbs/kandev/pull/402))

### Refactoring

- consolidate commit and PR dialogs to use vcs-dialogs ([#426](https://github.com/kdlbs/kandev/pull/426))

## 0.19 - 2026-03-09

### Features

- suggest agent install commands and fix TUI agent startup ([#398](https://github.com/kdlbs/kandev/pull/398))
- quick chat implementation ([#393](https://github.com/kdlbs/kandev/pull/393))

### Bug Fixes

- use gh repo clone for authenticated cloning and deduplicate PR reviews ([#397](https://github.com/kdlbs/kandev/pull/397))

## 0.18 - 2026-03-08

### Features

- seamless session switching without dockview layout flash ([#395](https://github.com/kdlbs/kandev/pull/395))
- single-port architecture and browser warning fixes ([#390](https://github.com/kdlbs/kandev/pull/390))
- improve port forwarding, remote executor setup, and CLI port config ([#388](https://github.com/kdlbs/kandev/pull/388))
- add port proxy, symlink file save fix, and remote executor improvements ([#358](https://github.com/kdlbs/kandev/pull/358))
- split file search from command panel, add inline task search & configurable shortcuts ([#383](https://github.com/kdlbs/kandev/pull/383))
- improve git checkout with error classification and warning propagation ([#386](https://github.com/kdlbs/kandev/pull/386))

### Bug Fixes

- persist template step edits on workflow save and add step delete confirmation ([#394](https://github.com/kdlbs/kandev/pull/394))
- system notifications, test buttons, apprise in Docker, and logo icon ([#391](https://github.com/kdlbs/kandev/pull/391))
- prevent duplicate messages when resuming ACP sessions ([#392](https://github.com/kdlbs/kandev/pull/392))
- correct default data directory to ~/.kandev/data ([#389](https://github.com/kdlbs/kandev/pull/389))
- stabilize flaky e2e tests and increase CI parallelism ([#384](https://github.com/kdlbs/kandev/pull/384))

## 0.17 - 2026-03-06

### Features

- enable native session resume with ACP session/load ([#380](https://github.com/kdlbs/kandev/pull/380))

### Bug Fixes

- remove conflicting node user before creating kandev user ([#385](https://github.com/kdlbs/kandev/pull/385))
- use merge-base instead of HEAD for session base commit ([#382](https://github.com/kdlbs/kandev/pull/382))

## 0.16 - 2026-03-06

### Features

- support PR URLs in task creation dialog ([#379](https://github.com/kdlbs/kandev/pull/379))
- improve mcp ask user debug ([#376](https://github.com/kdlbs/kandev/pull/376))
- add git failed operations as failed chat messages ([#371](https://github.com/kdlbs/kandev/pull/371))

### Bug Fixes

- isolate git env in workspace tracker tests ([#381](https://github.com/kdlbs/kandev/pull/381))
- improve startup readiness and base sync handling ([#374](https://github.com/kdlbs/kandev/pull/374))
- detect changes to already-dirty files in git status polling ([#375](https://github.com/kdlbs/kandev/pull/375))
- detect untracked file changes by using full identity string ([#373](https://github.com/kdlbs/kandev/pull/373))
- refresh diff view when untracked files change ([#372](https://github.com/kdlbs/kandev/pull/372))

### Refactoring

- move git status and commits to real-time agentctl queries ([#366](https://github.com/kdlbs/kandev/pull/366))

### Documentation

- add claude code skills, settings, and update architecture guide ([#378](https://github.com/kdlbs/kandev/pull/378))

## 0.15 - 2026-03-05

### Features

- start task from GitHub URL ([#365](https://github.com/kdlbs/kandev/pull/365))

## 0.14 - 2026-03-05

### Features

- improve session recovery and context reset ([#369](https://github.com/kdlbs/kandev/pull/369))
- improve TUI agents session resume on restart ([#367](https://github.com/kdlbs/kandev/pull/367))

### Bug Fixes

- auto-start code-server when opening file via VS Code ([#368](https://github.com/kdlbs/kandev/pull/368))
- resolve clarification MCP timeout with cancel-and-resume flow ([#362](https://github.com/kdlbs/kandev/pull/362))
- restore git status update in workspace polling loop ([#364](https://github.com/kdlbs/kandev/pull/364))
- add docker executor default values, patch build/container bugs ([#363](https://github.com/kdlbs/kandev/pull/363))

## 0.13 - 2026-03-04

### Features

- add diff expansion with expand-all in review panel ([#340](https://github.com/kdlbs/kandev/pull/340))

## 0.12 - 2026-03-04

### Features

- tui agents with workflows and code quality improvements ([#360](https://github.com/kdlbs/kandev/pull/360))
- improve closing resources (PTYs, connections) ([#355](https://github.com/kdlbs/kandev/pull/355))
- improve git operations (branch rename, amend commit, file rename, reset) ([#337](https://github.com/kdlbs/kandev/pull/337))

### Bug Fixes

- resolve stale PR data on task switch and deduplicate lifecycle code ([#361](https://github.com/kdlbs/kandev/pull/361))
- ui improvements for pr panel, git operations, and task sidebar ([#359](https://github.com/kdlbs/kandev/pull/359))
- clear stale PR data on task switch and add on-demand PR detection ([#357](https://github.com/kdlbs/kandev/pull/357))
- replace fsnotify with git polling to prevent fd exhaustion ([#356](https://github.com/kdlbs/kandev/pull/356))

## 0.11 - 2026-03-03

### Bug Fixes

- include agent_profile_id in session WS events to resolve stale MCP status ([#354](https://github.com/kdlbs/kandev/pull/354))

## 0.10 - 2026-03-02

### Features

- install agents on env preparation remote executors ([#352](https://github.com/kdlbs/kandev/pull/352))
- improve chat input ux ([#350](https://github.com/kdlbs/kandev/pull/350))
- startup health status ([#344](https://github.com/kdlbs/kandev/pull/344))
- web e2e tests ([#304](https://github.com/kdlbs/kandev/pull/304))
- add utility agents for one-shot AI tasks ([#341](https://github.com/kdlbs/kandev/pull/341))
- add Dockerfile, K8s manifests, and deployment docs ([#303](https://github.com/kdlbs/kandev/pull/303))
- improve session restoration for complete/failed/cancelled ([#302](https://github.com/kdlbs/kandev/pull/302))

### Bug Fixes

- passthrough PTY process survives page refresh ([#353](https://github.com/kdlbs/kandev/pull/353))
- sidebar task delete/archive redirects to next task or home ([#351](https://github.com/kdlbs/kandev/pull/351))
- sidebar task switcher shows outdated session state ([#349](https://github.com/kdlbs/kandev/pull/349))
- copy markdown to clipboard and codex error handling ([#348](https://github.com/kdlbs/kandev/pull/348))
- improve process termination and cleanup ([#347](https://github.com/kdlbs/kandev/pull/347))
- improve claude plan mode reliability and cleanup ([#346](https://github.com/kdlbs/kandev/pull/346))
- sidebar task switcher shows outdated session state and custom maximize layout ([#345](https://github.com/kdlbs/kandev/pull/345))
- prevent commit pruning when HEAD is not in database ([#343](https://github.com/kdlbs/kandev/pull/343))
- render markdown in user messages ([#338](https://github.com/kdlbs/kandev/pull/338))
- agentctl cleanup after shutdown ([#339](https://github.com/kdlbs/kandev/pull/339))
- resolve black terminal on background tab init and reduce resize storm ([#334](https://github.com/kdlbs/kandev/pull/334))
- include untracked files in workspace file search ([#330](https://github.com/kdlbs/kandev/pull/330))
- consolidate markdown styles into shared .markdown-body class ([#332](https://github.com/kdlbs/kandev/pull/332))
- standardize branding to KanDev across UI ([#328](https://github.com/kdlbs/kandev/pull/328))
- align MCP tool parameters and JSON tags with backend ([#329](https://github.com/kdlbs/kandev/pull/329))
- auto-update profile name when model changes ([#325](https://github.com/kdlbs/kandev/pull/325))
- lazy Docker client initialization to avoid startup errors ([#300](https://github.com/kdlbs/kandev/pull/300))
- strip terminal query responses from buffer replay on reconnect ([#301](https://github.com/kdlbs/kandev/pull/301))

## 0.9 - 2026-02-27

### Features

- release notes ([#298](https://github.com/kdlbs/kandev/pull/298))
- improve task launch ([#297](https://github.com/kdlbs/kandev/pull/297))

### Bug Fixes

- release notes button not visible on new database ([#299](https://github.com/kdlbs/kandev/pull/299))
- clear MCP pending requests on session transitions ([#296](https://github.com/kdlbs/kandev/pull/296))

## 0.8 - 2026-02-26

### Features

- restore correct scroll position after layout switch ([#295](https://github.com/kdlbs/kandev/pull/295))

## 0.7 - 2026-02-26

### Features

- improve vscode cleanup ([#294](https://github.com/kdlbs/kandev/pull/294))
- mermaid support ([#293](https://github.com/kdlbs/kandev/pull/293))

### Bug Fixes

- flaky test ([#292](https://github.com/kdlbs/kandev/pull/292))

## 0.6 - 2026-02-26

### Features

- improve workflow auto start ([#291](https://github.com/kdlbs/kandev/pull/291))
- improve layout manager ([#290](https://github.com/kdlbs/kandev/pull/290))

### Bug Fixes

- duplicated start message ([#289](https://github.com/kdlbs/kandev/pull/289))

## 0.5 - 2026-02-26

### Features

- pr layout after start ([#288](https://github.com/kdlbs/kandev/pull/288))
- improve PR review watcher + PR info panel ([#281](https://github.com/kdlbs/kandev/pull/281))
- open plan panel if agent writes to it ([#287](https://github.com/kdlbs/kandev/pull/287))
- clean up on remote session failure ([#286](https://github.com/kdlbs/kandev/pull/286))

## 0.4 - 2026-02-25

### Features

- claude code auth setup for remote executors ([#285](https://github.com/kdlbs/kandev/pull/285))
- reduce sql queries amount ([#282](https://github.com/kdlbs/kandev/pull/282))

### Bug Fixes

- vscode not being killed ([#284](https://github.com/kdlbs/kandev/pull/284))
- agents stuck on starting after restart ([#283](https://github.com/kdlbs/kandev/pull/283))

## 0.3 - 2026-02-25

### Features

- improve cli startup wait

## 0.2 - 2026-02-25

### Features

- use github.com for releases instead of api to avoid rate limiting
- add login verification to release script

## 0.1 - 2026-02-25

### Features

- improve release script
- add guard in workflow engine
- chat improvements ([#280](https://github.com/kdlbs/kandev/pull/280))
- default github runners ([#275](https://github.com/kdlbs/kandev/pull/275))

### Bug Fixes

- vscode ([#279](https://github.com/kdlbs/kandev/pull/279))
- layout switching messing panels ([#278](https://github.com/kdlbs/kandev/pull/278))
- worktree folder removal ([#277](https://github.com/kdlbs/kandev/pull/277))
- make dev use local db ([#276](https://github.com/kdlbs/kandev/pull/276))

## 0.0.12 - 2026-02-24

### Features

- improve release script

### Bug Fixes

- make github release atomic

## 0.0.11 - 2026-02-24

### Features

- upgrade cli version
- add release version to cli
- improve docker logging
- remove unnecessary files

## 0.0.10 - 2026-02-24

### Features

- upgrade cli
- add session-state sections to task switcher sidebar ([#273](https://github.com/kdlbs/kandev/pull/273))
- several UX improvements ([#272](https://github.com/kdlbs/kandev/pull/272))
- improve workflows and agent resume ([#269](https://github.com/kdlbs/kandev/pull/269))
- improve claude code normalized messages + review ux ([#271](https://github.com/kdlbs/kandev/pull/271))
- pr watcher user or team review ([#270](https://github.com/kdlbs/kandev/pull/270))
- improve remote executor sprites.dev ([#267](https://github.com/kdlbs/kandev/pull/267))
- remove executor healthcheck ([#263](https://github.com/kdlbs/kandev/pull/263))
- refactor big repository ([#261](https://github.com/kdlbs/kandev/pull/261))
- improve sql queries ([#260](https://github.com/kdlbs/kandev/pull/260))
- remote executors + secrets ([#257](https://github.com/kdlbs/kandev/pull/257))
- opencode acp
- opencode improve sse
- improve amp
- e2e tests
- improve claude code
- vscode integration ([#256](https://github.com/kdlbs/kandev/pull/256))
- improve acp + tracing ([#258](https://github.com/kdlbs/kandev/pull/258))
- improve ux ([#254](https://github.com/kdlbs/kandev/pull/254))
- improved dockview layouts ([#253](https://github.com/kdlbs/kandev/pull/253))
- improve backend handlers ([#252](https://github.com/kdlbs/kandev/pull/252))
- tui agents db ([#251](https://github.com/kdlbs/kandev/pull/251))
- add SQLite single-writer/multi-reader connection pool ([#250](https://github.com/kdlbs/kandev/pull/250))
- improve plan comments ([#249](https://github.com/kdlbs/kandev/pull/249))
- command panel search files ([#248](https://github.com/kdlbs/kandev/pull/248))
- improve comment system ([#247](https://github.com/kdlbs/kandev/pull/247))
- import export workflows ([#244](https://github.com/kdlbs/kandev/pull/244))
- update readme & agent TUI reliability ([#239](https://github.com/kdlbs/kandev/pull/239))
- add search functionality ([#243](https://github.com/kdlbs/kandev/pull/243))
- add more file editor keybindings ([#242](https://github.com/kdlbs/kandev/pull/242))
- improve monaco comments ([#241](https://github.com/kdlbs/kandev/pull/241))
- improve db abstraction ([#238](https://github.com/kdlbs/kandev/pull/238))
- add support for adding files ([#240](https://github.com/kdlbs/kandev/pull/240))
- improved passthrough and workflow ([#237](https://github.com/kdlbs/kandev/pull/237))
- ci complexity linters ([#236](https://github.com/kdlbs/kandev/pull/236))
- homelab-runner ([#233](https://github.com/kdlbs/kandev/pull/233))
- command panel ([#235](https://github.com/kdlbs/kandev/pull/235))
- improve changes panel ([#234](https://github.com/kdlbs/kandev/pull/234))
- archive tasks ([#232](https://github.com/kdlbs/kandev/pull/232))
- improve agent sort ([#231](https://github.com/kdlbs/kandev/pull/231))
- improve onboarding dialog ([#230](https://github.com/kdlbs/kandev/pull/230))
- improve stepper ([#229](https://github.com/kdlbs/kandev/pull/229))
- improve workflows ([#228](https://github.com/kdlbs/kandev/pull/228))

### Bug Fixes

- enforce sidebar max-width via dockview group constraints ([#274](https://github.com/kdlbs/kandev/pull/274))
- resolve all web app ESLint linter warnings ([#246](https://github.com/kdlbs/kandev/pull/246))
- resolve all backend golangci-lint violations ([#245](https://github.com/kdlbs/kandev/pull/245))

### Performance

- optimize settings page load ([#268](https://github.com/kdlbs/kandev/pull/268))

### Documentation

- add github integration (pr watcher) ([#262](https://github.com/kdlbs/kandev/pull/262))
- update and review main documentation ([#259](https://github.com/kdlbs/kandev/pull/259))

### Style

- fix format issues ([#255](https://github.com/kdlbs/kandev/pull/255))

## 0.0.9 - 2026-02-16

### Features

- reduce bundle size

## 0.0.8 - 2026-02-16

### Bug Fixes

- release bundle

## 0.0.7 - 2026-02-16

### Features

- improved stats ([#227](https://github.com/kdlbs/kandev/pull/227))

### Bug Fixes

- bundle web assets

## 0.0.6 - 2026-02-16

### Bug Fixes

- bundle all web assets

## 0.0.5 - 2026-02-16

## 0.0.4 - 2026-02-16

### Features

- use tar for bundles
- use tar for bundles
- improve editors ([#226](https://github.com/kdlbs/kandev/pull/226))

### Bug Fixes

- bundle

## 0.0.3 - 2026-02-15

### Bug Fixes

- release build

## 0.0.2 - 2026-02-15

### Features

- improve windows support
- fix cli org

### Bug Fixes

- sha lowercase comparison
- github release download when github token is present

## 0.0.1 - 2026-02-15

### Features

- improve windows support ([#225](https://github.com/kdlbs/kandev/pull/225))
- auggie dynamic model list ([#223](https://github.com/kdlbs/kandev/pull/223))
- better context files ([#222](https://github.com/kdlbs/kandev/pull/222))
- agents.json refactor ([#221](https://github.com/kdlbs/kandev/pull/221))
- improve task creation ([#220](https://github.com/kdlbs/kandev/pull/220))
- migrate agent operations from HTTP to WebSocket ([#218](https://github.com/kdlbs/kandev/pull/218))
- dockview new ui ([#219](https://github.com/kdlbs/kandev/pull/219))
- improve git pull ([#215](https://github.com/kdlbs/kandev/pull/215))
- remove blocking http call when creating the agent ([#214](https://github.com/kdlbs/kandev/pull/214))
- add agent boot message ([#213](https://github.com/kdlbs/kandev/pull/213))
- preventing process kill on port use
- increase agent boot timeout from 30s to 60s
- improve plan mode ([#212](https://github.com/kdlbs/kandev/pull/212))
- add ui debug on make start-debug
- favicon
- add make start-debug
- local executor and worktree + new task dialog ([#211](https://github.com/kdlbs/kandev/pull/211))
- review ux ([#210](https://github.com/kdlbs/kandev/pull/210))
- improve file tree ([#209](https://github.com/kdlbs/kandev/pull/209))
- mock agent ([#208](https://github.com/kdlbs/kandev/pull/208))
- queue messages ([#201](https://github.com/kdlbs/kandev/pull/201))
- file icons ([#207](https://github.com/kdlbs/kandev/pull/207))
- added message actions ([#203](https://github.com/kdlbs/kandev/pull/203))
- fix random port ([#202](https://github.com/kdlbs/kandev/pull/202))
- improve messages ux ([#193](https://github.com/kdlbs/kandev/pull/193))
- add Claude Opus 4.6 model support ([#194](https://github.com/kdlbs/kandev/pull/194))
- improve web fetch message ([#192](https://github.com/kdlbs/kandev/pull/192))
- Implement image paste functionality for Claude Code ([#188](https://github.com/kdlbs/kandev/pull/188))
- improve session terminals ([#185](https://github.com/kdlbs/kandev/pull/185))
- restore session when a worktree folder is deleted ([#184](https://github.com/kdlbs/kandev/pull/184))
- remove thinking selection ([#182](https://github.com/kdlbs/kandev/pull/182))
- improved chat input keybinding ([#173](https://github.com/kdlbs/kandev/pull/173))
- improve diff colors ([#171](https://github.com/kdlbs/kandev/pull/171))
- Add force push option to git push menu ([#167](https://github.com/kdlbs/kandev/pull/167))
- improve workspace file tree loading
- improve make start
- draft pr support ([#165](https://github.com/kdlbs/kandev/pull/165))
- improved file tree ([#164](https://github.com/kdlbs/kandev/pull/164))
- session mobile design ([#163](https://github.com/kdlbs/kandev/pull/163))
- custom commands ([#162](https://github.com/kdlbs/kandev/pull/162))
- add support for mcpToolCall item type ([#159](https://github.com/kdlbs/kandev/pull/159))
- mobile design ([#160](https://github.com/kdlbs/kandev/pull/160))
- Add built-in custom prompts for common workflows ([#156](https://github.com/kdlbs/kandev/pull/156))
- improve kanban board ui ([#155](https://github.com/kdlbs/kandev/pull/155))
- update deps
- remove outdated docs
- update docs to use mermaid ([#152](https://github.com/kdlbs/kandev/pull/152))
- update default board settings ([#153](https://github.com/kdlbs/kandev/pull/153))
- add make start ([#151](https://github.com/kdlbs/kandev/pull/151))
- Add file editor with diff-based save functionality ([#145](https://github.com/kdlbs/kandev/pull/145))
- pierre diffs lib ([#147](https://github.com/kdlbs/kandev/pull/147))
- Implement git discard changes functionality ([#144](https://github.com/kdlbs/kandev/pull/144))
- Improve task approval workflow and workflow templates ([#141](https://github.com/kdlbs/kandev/pull/141))
- all tasks page plus search ([#140](https://github.com/kdlbs/kandev/pull/140))
- improve task deletion ([#139](https://github.com/kdlbs/kandev/pull/139))
- slash commands from agents ([#136](https://github.com/kdlbs/kandev/pull/136))
- add GitHub Copilot CLI and Sourcegraph Amp agent support ([#130](https://github.com/kdlbs/kandev/pull/130))
- improve task creation dialog ([#135](https://github.com/kdlbs/kandev/pull/135))
- plan comment annotations, Kandev system prompt, and Standard workflow ([#134](https://github.com/kdlbs/kandev/pull/134))
- improve chat ui messages ([#132](https://github.com/kdlbs/kandev/pull/132))
- improve make dev shutdown ([#133](https://github.com/kdlbs/kandev/pull/133))
- implement task plans feature ([#131](https://github.com/kdlbs/kandev/pull/131))
- add debug toggle button to TaskTopBar ([#128](https://github.com/kdlbs/kandev/pull/128))
- chat messages normalized ([#121](https://github.com/kdlbs/kandev/pull/121))
- improve sidebar ([#127](https://github.com/kdlbs/kandev/pull/127))
- migrate MCP server from backend to agentctl ([#124](https://github.com/kdlbs/kandev/pull/124))
- Add ask_user_question MCP tool for agent clarifications ([#123](https://github.com/kdlbs/kandev/pull/123))
- improved chat input ([#120](https://github.com/kdlbs/kandev/pull/120))
- implement file referencing with @filename autocomplete in chat input ([#118](https://github.com/kdlbs/kandev/pull/118))
- add thinking/reasoning streaming support ([#113](https://github.com/kdlbs/kandev/pull/113))
- improve approval flow and step transitions ([#111](https://github.com/kdlbs/kandev/pull/111))
- Add file unstaging functionality ([#107](https://github.com/kdlbs/kandev/pull/107))
- implement workflow system with steps terminology ([#102](https://github.com/kdlbs/kandev/pull/102))
- improve logging ([#105](https://github.com/kdlbs/kandev/pull/105))
- remove npm warns from terminal + fix terminal render on refresh ([#104](https://github.com/kdlbs/kandev/pull/104))
- improved chat ux ([#103](https://github.com/kdlbs/kandev/pull/103))
- git pull before worktree creation ([#100](https://github.com/kdlbs/kandev/pull/100))
- opencode dynamic model loader ([#99](https://github.com/kdlbs/kandev/pull/99))
- improve task switch + cli passthrough resume ([#98](https://github.com/kdlbs/kandev/pull/98))
- cli passthrough state transitions ([#95](https://github.com/kdlbs/kandev/pull/95))
- cli passthrough setting ([#93](https://github.com/kdlbs/kandev/pull/93))
- per-task executor selection with multi-runtime support ([#92](https://github.com/kdlbs/kandev/pull/92))
- diff file multi line comment
- gemini and opencode
- claude code support ([#87](https://github.com/kdlbs/kandev/pull/87))
- Git status tracking refactor with persistent snapshots and commit history ([#86](https://github.com/kdlbs/kandev/pull/86))
- improved codex and auggie default permissions
- setup and cleanup script ([#80](https://github.com/kdlbs/kandev/pull/80))
- improve chat ui paddings ([#83](https://github.com/kdlbs/kandev/pull/83))
- random port support ([#79](https://github.com/kdlbs/kandev/pull/79))
- improve preview url loading ([#78](https://github.com/kdlbs/kandev/pull/78))
- refactor frontend hooks and store ([#76](https://github.com/kdlbs/kandev/pull/76))
- backend improved comments, logs and agents.md ([#77](https://github.com/kdlbs/kandev/pull/77))
- process runners ([#71](https://github.com/kdlbs/kandev/pull/71))
- http logging middleware + mcp random port ([#74](https://github.com/kdlbs/kandev/pull/74))
- add session turns with duration display and live timer ([#72](https://github.com/kdlbs/kandev/pull/72))
- refactored chat input panels + pie context ([#70](https://github.com/kdlbs/kandev/pull/70))
- dynamic model switching and session status fix ([#68](https://github.com/kdlbs/kandev/pull/68))
- add embedded MCP server with dual transport support ([#67](https://github.com/kdlbs/kandev/pull/67))
- add context window usage display to task session ([#66](https://github.com/kdlbs/kandev/pull/66))
- mcp servers + executors ([#60](https://github.com/kdlbs/kandev/pull/60))
- improve repository list setting ([#65](https://github.com/kdlbs/kandev/pull/65))
- implement turn cancellation for agent sessions ([#64](https://github.com/kdlbs/kandev/pull/64))
- custom branch prefix ([#58](https://github.com/kdlbs/kandev/pull/58))
- add system provider
- Add PR creation via gh CLI and improve git operations ([#57](https://github.com/kdlbs/kandev/pull/57))
- kanban page refactoring ([#52](https://github.com/kdlbs/kandev/pull/52))
- settings data loading per page ([#51](https://github.com/kdlbs/kandev/pull/51))
- preview panel option ([#49](https://github.com/kdlbs/kandev/pull/49))
- custom prompts ([#48](https://github.com/kdlbs/kandev/pull/48))
- refactor main, add providers ([#46](https://github.com/kdlbs/kandev/pull/46))
- remove dev db not used ([#45](https://github.com/kdlbs/kandev/pull/45))
- refactor sqlite usage ([#44](https://github.com/kdlbs/kandev/pull/44))
- improve list agents
- multiple editors support ([#41](https://github.com/kdlbs/kandev/pull/41))
- cli publish ([#40](https://github.com/kdlbs/kandev/pull/40))
- cli launcher npx kandev ([#39](https://github.com/kdlbs/kandev/pull/39))
- improve chat UX ([#38](https://github.com/kdlbs/kandev/pull/38))
- add typed event payloads for event bus messages ([#32](https://github.com/kdlbs/kandev/pull/32))
- start agents on boot
- improve landing page ([#33](https://github.com/kdlbs/kandev/pull/33))
- remove premature executor deletion
- session refactor ([#30](https://github.com/kdlbs/kandev/pull/30))
- updated agents.md ([#28](https://github.com/kdlbs/kandev/pull/28))
- shell selector ([#27](https://github.com/kdlbs/kandev/pull/27))
- task switcher column ([#26](https://github.com/kdlbs/kandev/pull/26))
- inline permission approval for tool calls ([#25](https://github.com/kdlbs/kandev/pull/25))
- notifications ([#24](https://github.com/kdlbs/kandev/pull/24))
- improved chat experience ([#22](https://github.com/kdlbs/kandev/pull/22))
- improve onboarding and task setup ([#21](https://github.com/kdlbs/kandev/pull/21))
- auto-respawn shell session on unexpected exit
- add graceful shutdown
- task_session_worktrees
- Interactive shell terminal for agent tasks ([#19](https://github.com/kdlbs/kandev/pull/19))
- flaky resume sessions
- rename agent_sessions to task_sessions
- golang linter
- right panel overlay
- tasks refactor
- improved chat ui topbar
- chat improved renderer and comments pagination
- tanstack virtual
- task comments SSR fetch
- cmd+enter in task chat
- protocol adapter abstraction with agent profile support ([#15](https://github.com/kdlbs/kandev/pull/15))
- replace agent_type with agent_profile_id ([#14](https://github.com/kdlbs/kandev/pull/14))
- improve cards
- changed theme to shadcn nova - less paddings
- data fetching refactor ssr -> ws updates
- File browser with syntax-highlighted viewer ([#13](https://github.com/kdlbs/kandev/pull/13))
- auto-launch agentctl subprocess in standalone mode ([#12](https://github.com/kdlbs/kandev/pull/12))
- add defaults to workspace: env, executor, agent
- simplify task creation
- display settings in kanban page
- semantic naming and branch cleanup on task deletion ([#11](https://github.com/kdlbs/kandev/pull/11))
- agents discovery
- add pre-commit hook
- environments and executors
- build agentctl to bin/ and use pre-built binary in Dockerfile ([#9](https://github.com/kdlbs/kandev/pull/9))
- Real-time Git Status Integration ([#8](https://github.com/kdlbs/kandev/pull/8))
- improved settings repos, boards
- landing page and pnpm workspaces
- return worktree info in orchestrator.start response
- worktrees at agent session level with random suffix
- cleanup worktree and branch on task deletion
- expose worktree path and branch in task API and UI
- implement Git worktrees for concurrent agent execution
- enhance tool call display with payload details and typing indicator
- make repository_url optional when launching agents
- add persistent agent session tracking
- enhance WebSocket handling and chat panel improvements
- tasks crud
- added repositories support
- enhance comment system and e2e testing
- implement ACP permission request flow
- add bidirectional comment system and agent input request flow
- web app support for boards and columns
- web app support for workspaces
- clean db command
- added workspaces
- ws state
- ws and zustand init
- add http handlers to backend
- implement persistent agent execution logs storage and retrieval
- improve homepage buttons
- settings page
- improved task page
- ui components reset
- task page
- add READY status and multi-turn conversation support
- complete acp-go-sdk integration with task state updates
- multi view kanban
- fix kanban ssr
- init kanban
- web app cleanup ([#2](https://github.com/kdlbs/kandev/pull/2))
- add build orchestration and architecture documentation ([#1](https://github.com/kdlbs/kandev/pull/1))
- web app init

### Bug Fixes

- resolve session stuck issues and workflow transition bugs ([#206](https://github.com/kdlbs/kandev/pull/206))
- copilot mcp ([#217](https://github.com/kdlbs/kandev/pull/217))
- complete tool calls on turn end ([#216](https://github.com/kdlbs/kandev/pull/216))
- start ws disconnected
- make start without public folder
- 2 cumulative diff
- cumulative diff poll
- pass MCP configuration to Claude Code via --mcp-config flag ([#205](https://github.com/kdlbs/kandev/pull/205))
- prevent user shell terminals from prematurely completing agent tasks ([#199](https://github.com/kdlbs/kandev/pull/199))
- refetch git status when switching back to a previously viewed task ([#198](https://github.com/kdlbs/kandev/pull/198))
- resume failed task sessions instead of returning errors ([#197](https://github.com/kdlbs/kandev/pull/197))
- cancel button always disabled when agent is running ([#195](https://github.com/kdlbs/kandev/pull/195))
- diff bg lines and codex last message ([#186](https://github.com/kdlbs/kandev/pull/186))
- prevent duplicate agent messages in database ([#181](https://github.com/kdlbs/kandev/pull/181))
- prevent duplicate message submission while agent is working ([#180](https://github.com/kdlbs/kandev/pull/180))
- resolve notification ordering race condition ([#179](https://github.com/kdlbs/kandev/pull/179))
- use detached context for ask_user_question MCP tool ([#178](https://github.com/kdlbs/kandev/pull/178))
- rollback from review step works on simple boards ([#177](https://github.com/kdlbs/kandev/pull/177))
- wire permission handler to Copilot SDK ([#161](https://github.com/kdlbs/kandev/pull/161))
- workaround shiki Go grammar catastrophic backtracking ([#174](https://github.com/kdlbs/kandev/pull/174))
- db path + open workspace folder ([#169](https://github.com/kdlbs/kandev/pull/169))
- agent profile creation ([#170](https://github.com/kdlbs/kandev/pull/170))
- show approve button wrongly + improve plan ux ([#158](https://github.com/kdlbs/kandev/pull/158))
- git status tracking - detect staging changes and persist in snapshots ([#149](https://github.com/kdlbs/kandev/pull/149))
- hide 'Approval Required' badge when agent is working ([#148](https://github.com/kdlbs/kandev/pull/148))
- disable Cmd+Enter keyboard shortcut when chat input is disabled ([#146](https://github.com/kdlbs/kandev/pull/146))
- prevent workflow step regression on follow-up prompts ([#143](https://github.com/kdlbs/kandev/pull/143))
- Update session/task states when ask_user_question tool is used ([#142](https://github.com/kdlbs/kandev/pull/142))
- Fix cache keying bug and repository locks memory leak ([#138](https://github.com/kdlbs/kandev/pull/138))
- permission request ID mismatch, SSE duplicates, and subprocess cleanup ([#137](https://github.com/kdlbs/kandev/pull/137))
- improve ask_user_question MCP tool with clear options format ([#125](https://github.com/kdlbs/kandev/pull/125))
- session recovery after backend restart ([#126](https://github.com/kdlbs/kandev/pull/126))
- use correct sandbox_mode to enable file editing ([#129](https://github.com/kdlbs/kandev/pull/129))
- Multiple bug fixes for agent lifecycle and task creation ([#122](https://github.com/kdlbs/kandev/pull/122))
- WebSocket race condition in session hooks and chat input performance ([#119](https://github.com/kdlbs/kandev/pull/119))
- prevent approval button showing while agent is working ([#117](https://github.com/kdlbs/kandev/pull/117))
- fix alignment in board creation ([#112](https://github.com/kdlbs/kandev/pull/112))
- refetch messages and git status when switching between tasks ([#110](https://github.com/kdlbs/kandev/pull/110))
- prevent WebSocket timeout from canceling long-running agent operations ([#109](https://github.com/kdlbs/kandev/pull/109))
- synchronous event bus dispatch with regression tests ([#106](https://github.com/kdlbs/kandev/pull/106))
- git status not showing after page refresh ([#101](https://github.com/kdlbs/kandev/pull/101))
- increase prompt timeout to 60min, add error feedback, rename acp to agent API ([#97](https://github.com/kdlbs/kandev/pull/97))
- strip origin/ prefix from base branch for rebase/merge operations ([#94](https://github.com/kdlbs/kandev/pull/94))
- filter upstream commits, fix ahead/behind, and remove duplicate types ([#91](https://github.com/kdlbs/kandev/pull/91))
- model selector ([#90](https://github.com/kdlbs/kandev/pull/90))
- load most recent messages and fix lazy loading pagination ([#82](https://github.com/kdlbs/kandev/pull/82))
- use AGENTCTL_PORT env var for backend ControlClient ([#63](https://github.com/kdlbs/kandev/pull/63))
- bind KANDEV_AGENT_STANDALONE_PORT env var to config ([#61](https://github.com/kdlbs/kandev/pull/61))
- refactor shell streaming to use event bus pattern ([#50](https://github.com/kdlbs/kandev/pull/50))
- make Codex Prompt() synchronous to fix premature task state transition ([#35](https://github.com/kdlbs/kandev/pull/35))
- session state after restart
- improve session terminal and git status handling ([#34](https://github.com/kdlbs/kandev/pull/34))
- make dev ctrl+c cleanup
- remove extra state transitions during startup resume
- skip tool call update when no active session
- resolve deadlock in adapter by not holding mutex during RPC calls
- typescript errors
- address code review issues ([#16](https://github.com/kdlbs/kandev/pull/16))
- docker cleanup
- macos launcher
- standalone mode workspace path and worktree lookup ([#10](https://github.com/kdlbs/kandev/pull/10))
- go vet ci
- reconnect to agent streams after backend restart
- show worktree path in UI after agent starts
- only show active worktrees in task API response
- configure git safe.directory in agent containers
- mount entire .git directory for worktree support in containers
- mount git worktree metadata directory into container
- wire worktree manager to lifecycle manager for agent isolation
- recover agent state from Docker on backend restart
- filter out internal ACP messages from WebSocket broadcast
- fix real-time comments not appearing on first agent start
- go tests
- task page
- workspace migration
- Structure session_info event correctly for protocol.Message parsing
- Publish session_info event so session ID is stored in task metadata
- build
- publish session notifications to event bus for WebSocket streaming
- web linter issues

### Refactoring

- Remove database migrations for clean dev-phase bootstrap ([#176](https://github.com/kdlbs/kandev/pull/176))
- consolidate system prompts into sysprompt package ([#166](https://github.com/kdlbs/kandev/pull/166))
- split monolithic sqlite.go into domain-specific files ([#84](https://github.com/kdlbs/kandev/pull/84))
- remove progress field from TaskSession and AgentExecution ([#75](https://github.com/kdlbs/kandev/pull/75))
- comprehensive code quality improvements ([#69](https://github.com/kdlbs/kandev/pull/69))
- remove auggie-specific code and use standard ACP protocol ([#47](https://github.com/kdlbs/kandev/pull/47))
- unify permission requests into agent event stream ([#31](https://github.com/kdlbs/kandev/pull/31))
- session resumption cleanup and AgentInstance → AgentExecution rename ([#23](https://github.com/kdlbs/kandev/pull/23))
- extract manager into focused components ([#20](https://github.com/kdlbs/kandev/pull/20))
- unify configuration and remove single-instance mode ([#18](https://github.com/kdlbs/kandev/pull/18))
- unify WebSocket patterns to Pattern A (handlers + controller + dto)
- Replace REST API with WebSocket-only architecture

### Documentation

- update WEBSOCKET_API.md with complete API reference
- Update AGENTS.md for WebSocket-only architecture
- Add comprehensive WebSocket API reference

### Fix

- Handle untracked and new files in git discard operation ([#168](https://github.com/kdlbs/kandev/pull/168))
- Persist plan notification state across page refreshes ([#150](https://github.com/kdlbs/kandev/pull/150))
- Approval button not showing when navigating between sessions ([#116](https://github.com/kdlbs/kandev/pull/116))

### Build

- improved pre-commit linter config

### Cleanup

- remove dead FileListUpdate streaming path ([#204](https://github.com/kdlbs/kandev/pull/204))

### Merge

- resolve conflicts with main



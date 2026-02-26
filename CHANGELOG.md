# Changelog

All notable changes to Kandev.

## 0.7 - 2026-02-26

### Bug Fixes

- flaky test ([#292](https://github.com/kdlbs/kandev/pull/292))

### Features

- improve vscode cleanup ([#294](https://github.com/kdlbs/kandev/pull/294))
- mermaid support ([#293](https://github.com/kdlbs/kandev/pull/293))

## 0.6 - 2026-02-26

### Bug Fixes

- duplicated start message ([#289](https://github.com/kdlbs/kandev/pull/289))

### Features

- improve workflow auto start ([#291](https://github.com/kdlbs/kandev/pull/291))
- improve layout manager ([#290](https://github.com/kdlbs/kandev/pull/290))

## 0.5 - 2026-02-26

### Features

- pr layout after start ([#288](https://github.com/kdlbs/kandev/pull/288))
- improve PR review watcher + PR info panel ([#281](https://github.com/kdlbs/kandev/pull/281))
- open plan panel if agent writes to it ([#287](https://github.com/kdlbs/kandev/pull/287))
- clean up on remote session failure ([#286](https://github.com/kdlbs/kandev/pull/286))

## 0.4 - 2026-02-25

### Bug Fixes

- vscode not being killed ([#284](https://github.com/kdlbs/kandev/pull/284))
- agents stuck on starting after restart ([#283](https://github.com/kdlbs/kandev/pull/283))

### Features

- claude code auth setup for remote executors ([#285](https://github.com/kdlbs/kandev/pull/285))
- reduce sql queries amount ([#282](https://github.com/kdlbs/kandev/pull/282))

## 0.3 - 2026-02-25

### Features

- improve cli startup wait

## 0.2 - 2026-02-25

### Features

- use github.com for releases instead of api to avoid rate limiting
- add login verification to release script

## 0.1 - 2026-02-25

### Bug Fixes

- vscode ([#279](https://github.com/kdlbs/kandev/pull/279))
- layout switching messing panels ([#278](https://github.com/kdlbs/kandev/pull/278))
- worktree folder removal ([#277](https://github.com/kdlbs/kandev/pull/277))
- make dev use local db ([#276](https://github.com/kdlbs/kandev/pull/276))

### Features

- improve release script
- add guard in workflow engine
- chat improvements ([#280](https://github.com/kdlbs/kandev/pull/280))
- default github runners ([#275](https://github.com/kdlbs/kandev/pull/275))

## 0.0.12 - 2026-02-24

### Bug Fixes

- make github release atomic

### Features

- improve release script

## 0.0.11 - 2026-02-24

### Features

- upgrade cli version
- add release version to cli
- improve docker logging
- remove unnecessary files

## 0.0.10 - 2026-02-24

### Bug Fixes

- enforce sidebar max-width via dockview group constraints ([#274](https://github.com/kdlbs/kandev/pull/274))
- resolve all web app ESLint linter warnings ([#246](https://github.com/kdlbs/kandev/pull/246))
- resolve all backend golangci-lint violations ([#245](https://github.com/kdlbs/kandev/pull/245))

### Documentation

- add github integration (pr watcher) ([#262](https://github.com/kdlbs/kandev/pull/262))
- update and review main documentation ([#259](https://github.com/kdlbs/kandev/pull/259))

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

### Performance

- optimize settings page load ([#268](https://github.com/kdlbs/kandev/pull/268))

### Style

- fix format issues ([#255](https://github.com/kdlbs/kandev/pull/255))

## 0.0.9 - 2026-02-16

### Features

- reduce bundle size

## 0.0.8 - 2026-02-16

### Bug Fixes

- release bundle

## 0.0.7 - 2026-02-16

### Bug Fixes

- bundle web assets

### Features

- improved stats ([#227](https://github.com/kdlbs/kandev/pull/227))

## 0.0.6 - 2026-02-16

### Bug Fixes

- bundle all web assets

## 0.0.5 - 2026-02-16

## 0.0.4 - 2026-02-16

### Bug Fixes

- bundle

### Features

- use tar for bundles
- use tar for bundles
- improve editors ([#226](https://github.com/kdlbs/kandev/pull/226))

## 0.0.3 - 2026-02-15

### Bug Fixes

- release build

## 0.0.2 - 2026-02-15

### Bug Fixes

- sha lowercase comparison
- github release download when github token is present

### Features

- improve windows support
- fix cli org

## 0.0.1 - 2026-02-15

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

### Documentation

- update WEBSOCKET_API.md with complete API reference
- Update AGENTS.md for WebSocket-only architecture
- Add comprehensive WebSocket API reference

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

### Fix

- Handle untracked and new files in git discard operation ([#168](https://github.com/kdlbs/kandev/pull/168))
- Persist plan notification state across page refreshes ([#150](https://github.com/kdlbs/kandev/pull/150))
- Approval button not showing when navigating between sessions ([#116](https://github.com/kdlbs/kandev/pull/116))

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

### Build

- improved pre-commit linter config

### Cleanup

- remove dead FileListUpdate streaming path ([#204](https://github.com/kdlbs/kandev/pull/204))

### Merge

- resolve conflicts with main



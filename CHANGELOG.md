# Changelog

All notable changes to Kandev.

## 0.79.0 - 2026-07-16

### Features

- sync workflows from a configured github repo ([#1721](https://github.com/kdlbs/kandev/pull/1721))
- add native app integrations ([#1715](https://github.com/kdlbs/kandev/pull/1715))
- add archive confirmation preference ([#1708](https://github.com/kdlbs/kandev/pull/1708))
- show file statuses in review ([#1683](https://github.com/kdlbs/kandev/pull/1683))
- rename task inline from the top bar via double-click ([#1702](https://github.com/kdlbs/kandev/pull/1702))
- add opt-in notification sound for waiting-for-input ([#1689](https://github.com/kdlbs/kandev/pull/1689))
- add repository context to quick chats ([#1679](https://github.com/kdlbs/kandev/pull/1679))
- surface agent subscription usage in settings and chat tooltip ([#1697](https://github.com/kdlbs/kandev/pull/1697))
- add Grok ACP agent ([#1688](https://github.com/kdlbs/kandev/pull/1688)) by @zensi-dev
- show ACP shell command output in chat ([#1684](https://github.com/kdlbs/kandev/pull/1684))
- unarchive tasks with worktree branch recovery ([#1687](https://github.com/kdlbs/kandev/pull/1687))
- interrupt busy child task turn on parent message_task_kandev ([#1653](https://github.com/kdlbs/kandev/pull/1653)) by @ClemDNL
- link GitHub issues to tasks ([#1672](https://github.com/kdlbs/kandev/pull/1672)) ([#1676](https://github.com/kdlbs/kandev/pull/1676))

### Bug Fixes

- compact plan implement control ([#1719](https://github.com/kdlbs/kandev/pull/1719))
- improve command palette alias matching ([#1718](https://github.com/kdlbs/kandev/pull/1718))
- render walkthrough surfaces behind dialogs ([#1717](https://github.com/kdlbs/kandev/pull/1717))
- render HTML in markdown previews ([#1707](https://github.com/kdlbs/kandev/pull/1707))
- handle parked task sessions ([#1711](https://github.com/kdlbs/kandev/pull/1711))
- prevent cross-task acknowledgement loops ([#1712](https://github.com/kdlbs/kandev/pull/1712))
- prevent settings sidebar from closing on first click ([#1709](https://github.com/kdlbs/kandev/pull/1709))
- restore persisted tabs after task reload ([#1703](https://github.com/kdlbs/kandev/pull/1703))
- stop task renames from wiping task repositories ([#1705](https://github.com/kdlbs/kandev/pull/1705))
- silence Vitest network noise ([#1700](https://github.com/kdlbs/kandev/pull/1700))
- enforce origin validation on websocket upgrades ([#1698](https://github.com/kdlbs/kandev/pull/1698))
- keep chat readable after right-pane resize ([#1682](https://github.com/kdlbs/kandev/pull/1682)) ([#1691](https://github.com/kdlbs/kandev/pull/1691))
- promote commandless workspace executions ([#1692](https://github.com/kdlbs/kandev/pull/1692))
- align topbar metrics heights ([#1694](https://github.com/kdlbs/kandev/pull/1694))
- repair grok docker auth and archived task control ([#1693](https://github.com/kdlbs/kandev/pull/1693))
- soften deferred clarification notice ([#1690](https://github.com/kdlbs/kandev/pull/1690))
- bump @playwright/test to ^1.61.1 to fix install hang on Node 26 ([#1685](https://github.com/kdlbs/kandev/pull/1685))
- resolve two cancel/clarification deadlocks that wedge sessions in RUNNING ([#1680](https://github.com/kdlbs/kandev/pull/1680))
- prevent stale ready events from completing replacement turns ([#1678](https://github.com/kdlbs/kandev/pull/1678))
- make clipboard fallback work in Radix dialogs ([#1671](https://github.com/kdlbs/kandev/pull/1671)) by @yattdev

### Performance

- upgrade Vitest and cap local workers ([#1696](https://github.com/kdlbs/kandev/pull/1696))

### Refactoring

- remove legacy settings migration ([#1704](https://github.com/kdlbs/kandev/pull/1704))

### Documentation

- own public documentation metadata ([#1713](https://github.com/kdlbs/kandev/pull/1713))
- add curated public documentation ([#1701](https://github.com/kdlbs/kandev/pull/1701))
- improve harness and issue templates ([#1681](https://github.com/kdlbs/kandev/pull/1681))

## 0.78.0 - 2026-07-13

### Features

- add workflow WIP pull system ([#1613](https://github.com/kdlbs/kandev/pull/1613))
- add per-file copy/symlink materialization per repository ([#1650](https://github.com/kdlbs/kandev/pull/1650)) by @jcoatelen-ledger
- support multiple concurrent Sentry instances ([#1469](https://github.com/kdlbs/kandev/pull/1469)) by @ClemDNL
- add durable plan implement action ([#1645](https://github.com/kdlbs/kandev/pull/1645))

### Bug Fixes

- use long press for mobile task actions ([#1673](https://github.com/kdlbs/kandev/pull/1673))
- preserve colon paths in copy files ([#1674](https://github.com/kdlbs/kandev/pull/1674))
- prune orphaned task worktree repositories ([#1667](https://github.com/kdlbs/kandev/pull/1667))
- clarify plan mode workflow guidance ([#1666](https://github.com/kdlbs/kandev/pull/1666))
- guard agent profile orphan cleanup ([#1659](https://github.com/kdlbs/kandev/pull/1659))
- show pending task input before messages load ([#1663](https://github.com/kdlbs/kandev/pull/1663))
- stop unstable use(params) promise from hiding office tree ([#1664](https://github.com/kdlbs/kandev/pull/1664))
- use maintained codex acp bridge ([#1656](https://github.com/kdlbs/kandev/pull/1656))
- wrap Utility Agents sub-sections in bounded cards ([#1654](https://github.com/kdlbs/kandev/pull/1654)) by @ClemDNL
- simplify walkthrough prompt requests ([#1660](https://github.com/kdlbs/kandev/pull/1660))
- show implement button whenever plan mode is active ([#1646](https://github.com/kdlbs/kandev/pull/1646))
- support markdown preview comments ([#1648](https://github.com/kdlbs/kandev/pull/1648))
- isolate saved layouts from stale sessions ([#1649](https://github.com/kdlbs/kandev/pull/1649))

## 0.77.0 - 2026-07-10

### Features

- add agent-authored code walkthroughs ([#1647](https://github.com/kdlbs/kandev/pull/1647))
- move integration links into task menus ([#1644](https://github.com/kdlbs/kandev/pull/1644))
- improve workspace sidebar navigation ([#1643](https://github.com/kdlbs/kandev/pull/1643))
- add configurable branch names ([#1615](https://github.com/kdlbs/kandev/pull/1615))

### Bug Fixes

- add mobile workspace switcher ([#1640](https://github.com/kdlbs/kandev/pull/1640))
- repair GitHub repository scope filters ([#1641](https://github.com/kdlbs/kandev/pull/1641))
- inherit parent scope for mcp subtasks ([#1638](https://github.com/kdlbs/kandev/pull/1638))
- sync task completion with terminal workflow steps ([#1639](https://github.com/kdlbs/kandev/pull/1639))
- expand nested custom prompts ([#1637](https://github.com/kdlbs/kandev/pull/1637))
- open each linked PR in its own tab from the "+" menu ([#1636](https://github.com/kdlbs/kandev/pull/1636)) by @ClemDNL
- stop archived automation-run tasks from pinning max_concurrent_runs ([#1632](https://github.com/kdlbs/kandev/pull/1632)) by @ClemDNL
- keep created subtasks during hydration ([#1626](https://github.com/kdlbs/kandev/pull/1626))
- keep diff previews tied to the correct file ([#1618](https://github.com/kdlbs/kandev/pull/1618))
- equalize integration card heights ([#1624](https://github.com/kdlbs/kandev/pull/1624))

### Documentation

- add pr fixup conflict guidance ([#1642](https://github.com/kdlbs/kandev/pull/1642))

## 0.76.0 - 2026-07-09

### Bug Fixes

- forward agent creds, fix terminal host config, restore linux/arm64 ([#1559](https://github.com/kdlbs/kandev/pull/1559)) by @iamcobolt
- keep resume readiness waits alive under caller cancellation ([#1582](https://github.com/kdlbs/kandev/pull/1582)) by @iamcobolt
- truncate long task titles in topbar ([#1630](https://github.com/kdlbs/kandev/pull/1630))
- handle config MCP executor listing ([#1628](https://github.com/kdlbs/kandev/pull/1628))
- expose signal-gated workflow steps in config MCP ([#1629](https://github.com/kdlbs/kandev/pull/1629))
- truthful executor rows, startup reconciliation, resume-safe cleanup ([#1597](https://github.com/kdlbs/kandev/pull/1597)) ([#1608](https://github.com/kdlbs/kandev/pull/1608)) by @point-source

## 0.75.0 - 2026-07-08

### Features

- persist and expire quick chats ([#1612](https://github.com/kdlbs/kandev/pull/1612))
- improve office onboarding and workspace flows ([#1606](https://github.com/kdlbs/kandev/pull/1606))
- single workspace switcher with deep links and copy-config dialog ([#1616](https://github.com/kdlbs/kandev/pull/1616))
- consolidate office system skills ([#1604](https://github.com/kdlbs/kandev/pull/1604))
- scope GitHub integration settings by workspace ([#1572](https://github.com/kdlbs/kandev/pull/1572))

### Bug Fixes

- stop kanban spinner for not-started sessions and fix truncated manual backup size ([#1622](https://github.com/kdlbs/kandev/pull/1622))
- preserve inherited task worktrees ([#1621](https://github.com/kdlbs/kandev/pull/1621))
- scope sidebar availability to active workspace ([#1620](https://github.com/kdlbs/kandev/pull/1620))
- confirm before archiving from PR merged/closed banners ([#1619](https://github.com/kdlbs/kandev/pull/1619))
- compact model config selector ([#1611](https://github.com/kdlbs/kandev/pull/1611))
- encode workflow hidden flag for postgres ([#1614](https://github.com/kdlbs/kandev/pull/1614))
- route controlled diff comment updates ([#1599](https://github.com/kdlbs/kandev/pull/1599))
- schedule agent-created office tasks ([#1605](https://github.com/kdlbs/kandev/pull/1605))

## 0.74.0 - 2026-07-06

### Features

- cmd+click and shift+click multi-select for kanban and sidebar ([#1518](https://github.com/kdlbs/kandev/pull/1518))
- delete buttons for individual and all runs in Recent Runs ([#1586](https://github.com/kdlbs/kandev/pull/1586)) by @ClemDNL
- adopt Office-style /tasks list UX ([#1593](https://github.com/kdlbs/kandev/pull/1593))

### Bug Fixes

- reap prompt-dead executions on resume ([#1602](https://github.com/kdlbs/kandev/pull/1602))
- complete idle acp monitor turns ([#1600](https://github.com/kdlbs/kandev/pull/1600))
- repair office agent launch context ([#1601](https://github.com/kdlbs/kandev/pull/1601))
- stop scheduled trigger retry storm on concurrency cap skip ([#1598](https://github.com/kdlbs/kandev/pull/1598)) by @ClemDNL

## 0.73.0 - 2026-07-03

### Features

- filter Jira tickets by project statuses ([#1588](https://github.com/kdlbs/kandev/pull/1588)) ([#1595](https://github.com/kdlbs/kandev/pull/1595))
- add Devin ACP agent integration ([#1497](https://github.com/kdlbs/kandev/pull/1497)) by @jijiamoer

### Bug Fixes

- preserve active workspace on settings bootstrap ([#1591](https://github.com/kdlbs/kandev/pull/1591))
- clear stale agent state on inactive workspace ([#1592](https://github.com/kdlbs/kandev/pull/1592))
- support postgres database stats page ([#1589](https://github.com/kdlbs/kandev/pull/1589))
- persist live executor runtime state ([#1585](https://github.com/kdlbs/kandev/pull/1585)) ([#1587](https://github.com/kdlbs/kandev/pull/1587))
- correct Linear ticket-number search filter ([#1584](https://github.com/kdlbs/kandev/pull/1584)) by @dk-blackfuel
- recover stale prompts and invalid monitors ([#1581](https://github.com/kdlbs/kandev/pull/1581))

## 0.72.0 - 2026-07-02

### Features

- press Enter on a dialog to run its semantic action ([#1565](https://github.com/kdlbs/kandev/pull/1565))

### Bug Fixes

- pause on clarification timeout ([#1580](https://github.com/kdlbs/kandev/pull/1580))
- keep slash command selections as drafts ([#1576](https://github.com/kdlbs/kandev/pull/1576))
- preserve task create defaults after refresh ([#1575](https://github.com/kdlbs/kandev/pull/1575))
- prevent infinite refetch loop on the automations settings page ([#1573](https://github.com/kdlbs/kandev/pull/1573)) by @dk-blackfuel
- add task lifecycle diagnostics ([#1571](https://github.com/kdlbs/kandev/pull/1571))

## 0.71.0 - 2026-07-01

### Features

- allow multiline custom answers in clarification overlay ([#1566](https://github.com/kdlbs/kandev/pull/1566))
- add Download option to file browser context menu ([#1567](https://github.com/kdlbs/kandev/pull/1567)) by @StefanFVogel

### Bug Fixes

- avoid stale task worktree repository launches ([#1568](https://github.com/kdlbs/kandev/pull/1568))
- refresh sidebar tasks when the active workspace changes ([#1564](https://github.com/kdlbs/kandev/pull/1564)) by @StefanFVogel
- keep user-selected terminal session active instead of auto-handoff ([#1562](https://github.com/kdlbs/kandev/pull/1562)) by @jingrandev

## 0.70.0 - 2026-07-01

### Features

- add per-watch dispatch sorting to the Linear issue watcher ([#1553](https://github.com/kdlbs/kandev/pull/1553)) by @nlenepveu
- redirect and toast when focused task is auto-deleted ([#1544](https://github.com/kdlbs/kandev/pull/1544))
- support remote macos ssh hosts ([#1534](https://github.com/kdlbs/kandev/pull/1534))

### Bug Fixes

- improve ci auto-fix delivery ([#1560](https://github.com/kdlbs/kandev/pull/1560))
- preserve nested markdown code fences ([#1557](https://github.com/kdlbs/kandev/pull/1557))
- trigger workflow on agent task messages ([#1555](https://github.com/kdlbs/kandev/pull/1555))
- sync archived task websocket removals ([#1556](https://github.com/kdlbs/kandev/pull/1556))
- keep acknowledged agent errors hidden ([#1558](https://github.com/kdlbs/kandev/pull/1558))
- keep completed office comments editable ([#1548](https://github.com/kdlbs/kandev/pull/1548))
- restore workspace deletion from settings ([#1547](https://github.com/kdlbs/kandev/pull/1547))
- keep tasks active while sibling sessions run ([#1551](https://github.com/kdlbs/kandev/pull/1551))
- persist task create last-used selections ([#1550](https://github.com/kdlbs/kandev/pull/1550))
- coalesce ci auto-fix queue rounds ([#1552](https://github.com/kdlbs/kandev/pull/1552))
- inherit current task profile for mcp tasks ([#1546](https://github.com/kdlbs/kandev/pull/1546))
- skip cleanup block when stop fails for terminal sessions ([#1541](https://github.com/kdlbs/kandev/pull/1541)) by @dominikkucharski
- focus changes panel for inactive task updates ([#1542](https://github.com/kdlbs/kandev/pull/1542))
- prevent automatic session tab pinning ([#1538](https://github.com/kdlbs/kandev/pull/1538))
- show repo slugs on kanban cards ([#1540](https://github.com/kdlbs/kandev/pull/1540))
- show loading states for task detail ([#1536](https://github.com/kdlbs/kandev/pull/1536))
- show sidebar spinner for created boot sessions ([#1537](https://github.com/kdlbs/kandev/pull/1537))
- align ci chip with aggregate checks ([#1535](https://github.com/kdlbs/kandev/pull/1535))
- prefer main for worktree base branches ([#1532](https://github.com/kdlbs/kandev/pull/1532))

### Documentation

- add debug log collection guide ([#1533](https://github.com/kdlbs/kandev/pull/1533))

## 0.69.0 - 2026-06-29

### Bug Fixes

- repair postgres schema migration coverage ([#1529](https://github.com/kdlbs/kandev/pull/1529))

## 0.68.0 - 2026-06-29

### Bug Fixes

- wait for task create last-used settings ([#1524](https://github.com/kdlbs/kandev/pull/1524))
- preserve nvm node path in service installs ([#1527](https://github.com/kdlbs/kandev/pull/1527))
- improve macos desktop startup ([#1525](https://github.com/kdlbs/kandev/pull/1525))

## 0.67.0 - 2026-06-28

### Bug Fixes

- unblock release desktop and universal builds ([#1522](https://github.com/kdlbs/kandev/pull/1522))

## 0.66.0 - 2026-06-28

### Features

- bind repository to Linear/Jira/Sentry issue watchers ([#1491](https://github.com/kdlbs/kandev/pull/1491)) by @nlenepveu
- add sidebar view system to mobile task switcher ([#1466](https://github.com/kdlbs/kandev/pull/1466))
- add tauri desktop app ([#1478](https://github.com/kdlbs/kandev/pull/1478))
- link tasks to github references ([#1471](https://github.com/kdlbs/kandev/pull/1471))
- add github repo filter search ([#1476](https://github.com/kdlbs/kandev/pull/1476))
- add subtask completion trigger ([#1473](https://github.com/kdlbs/kandev/pull/1473))
- improve ci auto-fix prompt control ([#1474](https://github.com/kdlbs/kandev/pull/1474))
- add native kandev launcher ([#1391](https://github.com/kdlbs/kandev/pull/1391))
- add CI PR automation controls ([#1446](https://github.com/kdlbs/kandev/pull/1446))
- surface PR merge-conflict & mergeability state in the CI popover ([#1448](https://github.com/kdlbs/kandev/pull/1448))
- sync task preferences across devices ([#1440](https://github.com/kdlbs/kandev/pull/1440))
- remove nextjs production runtime ([#1389](https://github.com/kdlbs/kandev/pull/1389))

### Bug Fixes

- support unsigned desktop releases ([#1520](https://github.com/kdlbs/kandev/pull/1520))
- handle windows desktop runtime helper mode ([#1519](https://github.com/kdlbs/kandev/pull/1519))
- preserve pinned task sessions ([#1517](https://github.com/kdlbs/kandev/pull/1517))
- resync queued state on mobile resume ([#1514](https://github.com/kdlbs/kandev/pull/1514))
- improve mobile kanban controls ([#1515](https://github.com/kdlbs/kandev/pull/1515))
- restore task create repo from settings ([#1513](https://github.com/kdlbs/kandev/pull/1513))
- improve mobile chat input controls ([#1477](https://github.com/kdlbs/kandev/pull/1477))
- reuse worktrees for same-task handoffs ([#1510](https://github.com/kdlbs/kandev/pull/1510))
- preserve task repo on rename updates ([#1511](https://github.com/kdlbs/kandev/pull/1511))
- prime task create last-used cache before dialog effects ([#1509](https://github.com/kdlbs/kandev/pull/1509))
- improve pr ci automation reliability ([#1508](https://github.com/kdlbs/kandev/pull/1508))
- stop iOS focus-zoom on mobile text fields ([#1501](https://github.com/kdlbs/kandev/pull/1501))
- restore create task selector scrolling ([#1502](https://github.com/kdlbs/kandev/pull/1502))
- persist profiles for deferred MCP tasks ([#1503](https://github.com/kdlbs/kandev/pull/1503))
- recreate stale host utility instances ([#1500](https://github.com/kdlbs/kandev/pull/1500)) ([#1504](https://github.com/kdlbs/kandev/pull/1504))
- target active passthrough session in composer ([#1505](https://github.com/kdlbs/kandev/pull/1505))
- paginate github check run fetching ([#1506](https://github.com/kdlbs/kandev/pull/1506))
- enforce single workflow start step ([#1507](https://github.com/kdlbs/kandev/pull/1507))
- prevent leaked agent processes on shutdown ([#1498](https://github.com/kdlbs/kandev/pull/1498))
- update ACP SDK large frame handling ([#1496](https://github.com/kdlbs/kandev/pull/1496))
- auto-fit wide mermaid diagrams ([#1495](https://github.com/kdlbs/kandev/pull/1495))
- prevent invalid MCP task sessions ([#1494](https://github.com/kdlbs/kandev/pull/1494))
- debug task dialog selection restore ([#1493](https://github.com/kdlbs/kandev/pull/1493))
- recreate github review tasks after reset ([#1492](https://github.com/kdlbs/kandev/pull/1492))
- clean office workspace data on delete ([#1486](https://github.com/kdlbs/kandev/pull/1486))
- make schema migrations replayable ([#1489](https://github.com/kdlbs/kandev/pull/1489))
- persist self-managed gitlab host ([#1488](https://github.com/kdlbs/kandev/pull/1488))
- preserve github workspace context ([#1487](https://github.com/kdlbs/kandev/pull/1487))
- dispatch Sentry issues for watches filtering on multiple levels ([#1475](https://github.com/kdlbs/kandev/pull/1475)) by @nlenepveu
- show purple merged icon for recently merged PRs ([#1483](https://github.com/kdlbs/kandev/pull/1483))
- polish task dialog popovers and CI block icons ([#1472](https://github.com/kdlbs/kandev/pull/1472))
- expand ~ home links and report missing file paths cleanly ([#1468](https://github.com/kdlbs/kandev/pull/1468)) by @ClemDNL
- stop task runtimes from leaking ([#1465](https://github.com/kdlbs/kandev/pull/1465))
- avoid invalid opencode inline comments ([#1464](https://github.com/kdlbs/kandev/pull/1464))
- show pointer cursor on hover over enabled switches ([#1467](https://github.com/kdlbs/kandev/pull/1467))
- show queued CI auto-fix prompts in order ([#1463](https://github.com/kdlbs/kandev/pull/1463))
- support issue links in remote task dialog ([#1462](https://github.com/kdlbs/kandev/pull/1462))
- keep CI popover open when cursor moves onto it ([#1461](https://github.com/kdlbs/kandev/pull/1461))
- expose session mcp to cursor acp ([#1460](https://github.com/kdlbs/kandev/pull/1460))
- stop run-mode automation agents ([#1459](https://github.com/kdlbs/kandev/pull/1459))
- inherit workflow default agent profile when auto-starting a profile-less session ([#1450](https://github.com/kdlbs/kandev/pull/1450)) by @nlenepveu
- load feature toggles on mount ([#1454](https://github.com/kdlbs/kandev/pull/1454))
- stop CI auto-fix on non-actionable feedback ([#1452](https://github.com/kdlbs/kandev/pull/1452))
- preserve kanban card runtime state ([#1443](https://github.com/kdlbs/kandev/pull/1443))
- keep start-agent tasks scheduling ([#1445](https://github.com/kdlbs/kandev/pull/1445))
- constrain dialog text inputs ([#1437](https://github.com/kdlbs/kandev/pull/1437))
- restore debug flag in Vite shell ([#1442](https://github.com/kdlbs/kandev/pull/1442))
- constrain ci checks popover height ([#1441](https://github.com/kdlbs/kandev/pull/1441))
- focus changes panel on git updates ([#1436](https://github.com/kdlbs/kandev/pull/1436))
- restore SPA favicon assets ([#1438](https://github.com/kdlbs/kandev/pull/1438))
- suppress kanban spinner for TODO tasks ([#1399](https://github.com/kdlbs/kandev/pull/1399))

### Performance

- tame browser-grinding cumulative diffs during large rebases ([#1430](https://github.com/kdlbs/kandev/pull/1430))

### Documentation

- refresh harness guidance ([#1458](https://github.com/kdlbs/kandev/pull/1458))

## 0.65.0 - 2026-06-18

### Bug Fixes

- backfill running chat turns without prompt frame ([#1429](https://github.com/kdlbs/kandev/pull/1429))
- split multi-file agent read links and strip line-range selectors ([#1426](https://github.com/kdlbs/kandev/pull/1426)) by @ClemDNL
- stop session-tab flicker when same-task sessions have diverged env ids ([#1428](https://github.com/kdlbs/kandev/pull/1428))
- prefer local copilot CLI over npx at launch ([#1384](https://github.com/kdlbs/kandev/pull/1384))
- render er diagrams in task plans ([#1425](https://github.com/kdlbs/kandev/pull/1425))
- restore diff expansion for committed-source review rows ([#1423](https://github.com/kdlbs/kandev/pull/1423))
- show quick chat session model options ([#1422](https://github.com/kdlbs/kandev/pull/1422))

### Performance

- fix board lag, memory growth, and rebase-driven CPU spikes ([#1427](https://github.com/kdlbs/kandev/pull/1427))

### Documentation

- refresh feature documentation ([#1424](https://github.com/kdlbs/kandev/pull/1424))
- record PR fixup harness learnings ([#1416](https://github.com/kdlbs/kandev/pull/1416))

## 0.64.0 - 2026-06-17

### Bug Fixes

- cover agent file link review fixes ([#1415](https://github.com/kdlbs/kandev/pull/1415))
- open agent file links that carry line-range selectors ([#1412](https://github.com/kdlbs/kandev/pull/1412)) by @ClemDNL

## 0.63.0 - 2026-06-17

### Features

- improve passthrough composer and session recovery ([#1411](https://github.com/kdlbs/kandev/pull/1411))

### Bug Fixes

- restore missing settings sidebar links ([#1413](https://github.com/kdlbs/kandev/pull/1413))
- support postgres initial agent setup ([#1410](https://github.com/kdlbs/kandev/pull/1410))

## 0.62.0 - 2026-06-17

### Bug Fixes

- preserve office sidebar workspace toggles ([#1403](https://github.com/kdlbs/kandev/pull/1403))
- open markdown file links with line suffixes ([#1404](https://github.com/kdlbs/kandev/pull/1404))
- expose kandev mcp tools to pi ([#1401](https://github.com/kdlbs/kandev/pull/1401))

## 0.61.0 - 2026-06-16

### Features

- surface ensure-session errors and persist preset layout changes ([#1394](https://github.com/kdlbs/kandev/pull/1394))

### Bug Fixes

- open root-relative chat file links ([#1400](https://github.com/kdlbs/kandev/pull/1400))
- persist agent error dismissal ([#1397](https://github.com/kdlbs/kandev/pull/1397))
- chat badge, dockview preview routing, and agent-error icon cleanup ([#1395](https://github.com/kdlbs/kandev/pull/1395))
- restore office sidebar navigation ([#1396](https://github.com/kdlbs/kandev/pull/1396))
- stabilize postgres startup and model dropdown scroll ([#1393](https://github.com/kdlbs/kandev/pull/1393))
- preserve chat session during workflow advances ([#1392](https://github.com/kdlbs/kandev/pull/1392))
- open markdown file links in editor ([#1388](https://github.com/kdlbs/kandev/pull/1388))

### Documentation

- update spec-driven planning workflow ([#1390](https://github.com/kdlbs/kandev/pull/1390))

## 0.60.0 - 2026-06-15

### Features

- split general settings pages ([#1383](https://github.com/kdlbs/kandev/pull/1383))
- auggie subagent detection and single-repo review diff ([#1385](https://github.com/kdlbs/kandev/pull/1385))
- add reset action for integration watches ([#1381](https://github.com/kdlbs/kandev/pull/1381))
- add feature toggles settings ([#1372](https://github.com/kdlbs/kandev/pull/1372))
- add resource metrics topbar ([#1373](https://github.com/kdlbs/kandev/pull/1373))
- add tooltip explaining reset environment behavior ([#1374](https://github.com/kdlbs/kandev/pull/1374))
- add file-backed prompt attachments ([#1367](https://github.com/kdlbs/kandev/pull/1367))
- allow read-only absolute file paths ([#1371](https://github.com/kdlbs/kandev/pull/1371))

### Bug Fixes

- use muted color for scheduling/starting spinner ([#1386](https://github.com/kdlbs/kandev/pull/1386))
- scope multi-repo file editor open/save to the right repo ([#1382](https://github.com/kdlbs/kandev/pull/1382))
- map acp todo tool results to session todos ([#1376](https://github.com/kdlbs/kandev/pull/1376))
- preserve recoverable agent errors ([#1368](https://github.com/kdlbs/kandev/pull/1368))
- distinguish sidebar workspace types ([#1364](https://github.com/kdlbs/kandev/pull/1364))
- show preparing task spinner ([#1369](https://github.com/kdlbs/kandev/pull/1369))
- show spinner for running review sessions ([#1363](https://github.com/kdlbs/kandev/pull/1363))
- improve diff undo and wrapping defaults ([#1366](https://github.com/kdlbs/kandev/pull/1366))
- initialize postgres schemas ([#1365](https://github.com/kdlbs/kandev/pull/1365))

## 0.59.0 - 2026-06-14

### Features

- expose ACP session debug metadata ([#1359](https://github.com/kdlbs/kandev/pull/1359))
- show session config in message metadata ([#1354](https://github.com/kdlbs/kandev/pull/1354))
- tabbed multi-PR CI popover for topbar button and chat status chip ([#1356](https://github.com/kdlbs/kandev/pull/1356))

### Bug Fixes

- summarize current PR state ([#1362](https://github.com/kdlbs/kandev/pull/1362))
- keep agent tab before PR tab ([#1360](https://github.com/kdlbs/kandev/pull/1360))
- apply profile auto-approve to agentctl instances ([#1358](https://github.com/kdlbs/kandev/pull/1358))

### Performance

- eliminate redundant re-renders and markdown re-parsing ([#1357](https://github.com/kdlbs/kandev/pull/1357))
- eliminate sidebar close lag with many tasks ([#1355](https://github.com/kdlbs/kandev/pull/1355))

## 0.58.0 - 2026-06-12

### Features

- indicate pending close state ([#1334](https://github.com/kdlbs/kandev/pull/1334))

### Bug Fixes

- hide sidebar footer urls on hover ([#1348](https://github.com/kdlbs/kandev/pull/1348))
- restore Sentry entry in settings integrations nav ([#1350](https://github.com/kdlbs/kandev/pull/1350)) by @nlenepveu
- persist session runtime config ([#1346](https://github.com/kdlbs/kandev/pull/1346))
- move changes stage action to file icon slot ([#1349](https://github.com/kdlbs/kandev/pull/1349))
- apply max inflight cap to Sentry watcher tasks ([#1326](https://github.com/kdlbs/kandev/pull/1326)) by @nlenepveu
- route setup cancel to homepage ([#1333](https://github.com/kdlbs/kandev/pull/1333))
- update session tab title on model changes ([#1335](https://github.com/kdlbs/kandev/pull/1335))
- avoid oversized env spawn failures ([#1344](https://github.com/kdlbs/kandev/pull/1344))
- restore default layout during session preparation ([#1343](https://github.com/kdlbs/kandev/pull/1343))
- suppress archive success toast ([#1345](https://github.com/kdlbs/kandev/pull/1345))
- reset xterm before terminal reconnect ([#1342](https://github.com/kdlbs/kandev/pull/1342))
- de-flake e2e resume flicker, WS-subscribe race, and agent-boot contention ([#1338](https://github.com/kdlbs/kandev/pull/1338))
- match issue identifier (ENG-123) in search ([#1340](https://github.com/kdlbs/kandev/pull/1340))
- harden pr-state and e2e guidance ([#1331](https://github.com/kdlbs/kandev/pull/1331))
- cascade-archive child tasks on workflow delete ([#1332](https://github.com/kdlbs/kandev/pull/1332))

### Documentation

- update readme and e2e skill with missing executors, integrations, agents ([#1347](https://github.com/kdlbs/kandev/pull/1347))
- add sentry to integrations list ([#1337](https://github.com/kdlbs/kandev/pull/1337))

## 0.57.0 - 2026-06-11

### Features

- add avg turn duration and messages per turn ([#1328](https://github.com/kdlbs/kandev/pull/1328))
- support self-hosted instances via configurable URL ([#1320](https://github.com/kdlbs/kandev/pull/1320)) by @ClemDNL
- add session handoff via agent tab context menu ([#1317](https://github.com/kdlbs/kandev/pull/1317))
- add checkout service make targets ([#1311](https://github.com/kdlbs/kandev/pull/1311))
- allow remote external mcp access ([#1307](https://github.com/kdlbs/kandev/pull/1307))
- ship explicit completion signal toggle + YAML round-trip ([#1284](https://github.com/kdlbs/kandev/pull/1284))
- unified app sidebar ([#1165](https://github.com/kdlbs/kandev/pull/1165))

### Bug Fixes

- reap active sessions when archiving a task ([#1275](https://github.com/kdlbs/kandev/pull/1275)) by @nlenepveu
- improve sidebar action layout ([#1323](https://github.com/kdlbs/kandev/pull/1323))
- correct sidebar workspace switcher routing and selection persistence ([#1329](https://github.com/kdlbs/kandev/pull/1329))
- persist session model and drop composite id logic ([#1327](https://github.com/kdlbs/kandev/pull/1327))
- improve archive switch reliability ([#1325](https://github.com/kdlbs/kandev/pull/1325))
- bound long session resumes and restore legacy model surfaces ([#1324](https://github.com/kdlbs/kandev/pull/1324))
- show open PR status when task has merged and open PRs ([#1322](https://github.com/kdlbs/kandev/pull/1322))
- default new task executor to worktree ([#1321](https://github.com/kdlbs/kandev/pull/1321))
- deliver initial prompt to passthrough start_agent ([#1306](https://github.com/kdlbs/kandev/pull/1306)) by @nlenepveu
- keep utility default agent and model paired ([#1318](https://github.com/kdlbs/kandev/pull/1318))
- handle config-option-only model state ([#1319](https://github.com/kdlbs/kandev/pull/1319))
- read claude agent models and modes from configOptions ([#1310](https://github.com/kdlbs/kandev/pull/1310))
- contain settings page overscroll ([#1316](https://github.com/kdlbs/kandev/pull/1316))
- default changes panel to tree view ([#1314](https://github.com/kdlbs/kandev/pull/1314))
- allow office mcp mode for existing tasks ([#1315](https://github.com/kdlbs/kandev/pull/1315))
- quote record skill description ([#1313](https://github.com/kdlbs/kandev/pull/1313))
- clarify required automation fields ([#1312](https://github.com/kdlbs/kandev/pull/1312))
- collapse dockview workflow stepper to current step when cramped ([#1309](https://github.com/kdlbs/kandev/pull/1309))
- stop reseeding agent profiles the user deleted ([#1305](https://github.com/kdlbs/kandev/pull/1305)) by @nlenepveu
- preserve settings sidebar on refresh ([#1303](https://github.com/kdlbs/kandev/pull/1303))
- avoid stale sibling PR sync ([#1302](https://github.com/kdlbs/kandev/pull/1302))
- focus sidebar-created tasks ([#1301](https://github.com/kdlbs/kandev/pull/1301))
- keep dockview group alive on task switch to stop layout collapse ([#1308](https://github.com/kdlbs/kandev/pull/1308))
- gate step_complete_kandev tool on per-step signal flag ([#1300](https://github.com/kdlbs/kandev/pull/1300))
- preserve codex reasoning model ids ([#1296](https://github.com/kdlbs/kandev/pull/1296))
- sort kanban cards by newest created ([#1298](https://github.com/kdlbs/kandev/pull/1298))
- list opencode models when ACP probe is empty ([#1278](https://github.com/kdlbs/kandev/pull/1278)) by @ClemDNL
- alert when git executable is missing ([#1297](https://github.com/kdlbs/kandev/pull/1297))
- use recent task after removal ([#1295](https://github.com/kdlbs/kandev/pull/1295))
- gate changes push button on ahead count ([#1294](https://github.com/kdlbs/kandev/pull/1294))
- restore dockview sidebar layout ([#1293](https://github.com/kdlbs/kandev/pull/1293))
- expose user agent bins to system service ([#1292](https://github.com/kdlbs/kandev/pull/1292))
- read models from configOptions fallback ([#1291](https://github.com/kdlbs/kandev/pull/1291))
- tighten dockview topbar button sizing ([#1290](https://github.com/kdlbs/kandev/pull/1290))
- surface inference-agent probe status + add refresh endpoint ([#1287](https://github.com/kdlbs/kandev/pull/1287))
- guard dockview measure against mid-transition sidebar width ([#1288](https://github.com/kdlbs/kandev/pull/1288))
- preserve manual task selection during archive/delete ([#1286](https://github.com/kdlbs/kandev/pull/1286))
- handle UNIQUE collision in UpdatePRWatchBranchIfSearching ([#1285](https://github.com/kdlbs/kandev/pull/1285))
- defer move_task when session is running ([#1277](https://github.com/kdlbs/kandev/pull/1277)) by @edan-binshtok
- make toggle-sidebar shortcut work on every route ([#1283](https://github.com/kdlbs/kandev/pull/1283))
- point sidebar Home to office dashboard in office mode ([#1282](https://github.com/kdlbs/kandev/pull/1282))
- align office topbar border with sidebar header (h-10) ([#1281](https://github.com/kdlbs/kandev/pull/1281))

### Performance

- split stats endpoint per-section and rewrite GetGlobalStats ([#1289](https://github.com/kdlbs/kandev/pull/1289))

## 0.56.0 - 2026-06-06

### Features

- explicit step-completion signal for auto-advance ([#1276](https://github.com/kdlbs/kandev/pull/1276))
- search tasks by PR number in command panel ([#1268](https://github.com/kdlbs/kandev/pull/1268))
- use stored base_branch for diff stats + per-task compare picker ([#1273](https://github.com/kdlbs/kandev/pull/1273))
- add Sentry integration with issue watcher ([#1133](https://github.com/kdlbs/kandev/pull/1133)) by @nlenepveu
- accept repository_url and local_path on add_branch_to_task_kandev ([#1256](https://github.com/kdlbs/kandev/pull/1256))

### Bug Fixes

- exclude archived tasks from repository delete guard ([#1274](https://github.com/kdlbs/kandev/pull/1274)) by @nlenepveu
- order workspace stream wg.Add before publish to fix Close race ([#1272](https://github.com/kdlbs/kandev/pull/1272))
- prevent cross-task session leak in dockview layout switch ([#1265](https://github.com/kdlbs/kandev/pull/1265))
- use claude-acp as default inference agent id ([#1271](https://github.com/kdlbs/kandev/pull/1271))
- rmdir empty task parent after worktree removal ([#1267](https://github.com/kdlbs/kandev/pull/1267))
- unblock next prompt when agent does not acknowledge cancel ([#1259](https://github.com/kdlbs/kandev/pull/1259))
- surface add_branch_to_task failures instead of silent orphans ([#1264](https://github.com/kdlbs/kandev/pull/1264))
- drain StreamManager goroutines in tests ([#1227](https://github.com/kdlbs/kandev/pull/1227))
- emit --dangerously-skip-permissions for Claude CLI passthrough ([#1262](https://github.com/kdlbs/kandev/pull/1262))

## 0.55.0 - 2026-06-02

### Features

- inject MCP servers in CLI passthrough mode per agent ([#1158](https://github.com/kdlbs/kandev/pull/1158))
- make chat reverse-i-search shortcut configurable ([#1255](https://github.com/kdlbs/kandev/pull/1255))
- full GitLab integration — parity with GitHub ([#1120](https://github.com/kdlbs/kandev/pull/1120))
- support ** and brace globs in copy_files patterns ([#1248](https://github.com/kdlbs/kandev/pull/1248))
- resizable review dialog sidebar with persistence ([#1245](https://github.com/kdlbs/kandev/pull/1245))
- chat history nav with ArrowUp/Down and Ctrl+R fuzzy search ([#1246](https://github.com/kdlbs/kandev/pull/1246))
- include associated PRs in task-listing tools ([#1236](https://github.com/kdlbs/kandev/pull/1236))
- add reverse direction to recent task switcher ([#1241](https://github.com/kdlbs/kandev/pull/1241))
- multi-branch tasks — N (repo, branch) pairs per task ([#1226](https://github.com/kdlbs/kandev/pull/1226))
- add voice mode to task create dialog ([#1230](https://github.com/kdlbs/kandev/pull/1230))
- service-managed UI self-update ([#1210](https://github.com/kdlbs/kandev/pull/1210))
- show inline download progress for Whisper Web model ([#1228](https://github.com/kdlbs/kandev/pull/1228))
- add voice mode for chat input ([#1159](https://github.com/kdlbs/kandev/pull/1159))

### Bug Fixes

- preserve repo metadata in kanban.update and check legacy repositoryId ([#1258](https://github.com/kdlbs/kandev/pull/1258))
- block workflow advance during pending clarifications ([#1251](https://github.com/kdlbs/kandev/pull/1251))
- stop refetching GitLab status on every tab refocus ([#1257](https://github.com/kdlbs/kandev/pull/1257))
- self-heal orphaned watchers when agent profile is soft-deleted ([#1094](https://github.com/kdlbs/kandev/pull/1094)) by @nlenepveu
- split codex acp flags from agentctl auto-approve ([#1253](https://github.com/kdlbs/kandev/pull/1253))
- sticky-max context window size with reset on model switch ([#1254](https://github.com/kdlbs/kandev/pull/1254))
- restore debug UI on make start-debug ([#1252](https://github.com/kdlbs/kandev/pull/1252))
- reap disconnected ACP sessions + MCP child trees ([#1249](https://github.com/kdlbs/kandev/pull/1249))
- hide token usage when context window report is unreliable ([#1250](https://github.com/kdlbs/kandev/pull/1250))
- stabilize ACP session resume and workspace git poll under contention ([#1242](https://github.com/kdlbs/kandev/pull/1242))
- cache & singleflight live PR feedback fetches ([#1237](https://github.com/kdlbs/kandev/pull/1237))
- handle claude-acp async_launched subagents end-to-end ([#1244](https://github.com/kdlbs/kandev/pull/1244))
- disable create button until project + title set ([#1243](https://github.com/kdlbs/kandev/pull/1243))
- harden multi-branch add_branch and surface it in UI ([#1239](https://github.com/kdlbs/kandev/pull/1239))
- refresh commits panel snapshot on mount ([#1232](https://github.com/kdlbs/kandev/pull/1232))
- subdue voice input button to ghost style ([#1233](https://github.com/kdlbs/kandev/pull/1233))
- cap gh/git fork concurrency via shared subproc throttles ([#1216](https://github.com/kdlbs/kandev/pull/1216))
- add button to load older chat messages reliably ([#1223](https://github.com/kdlbs/kandev/pull/1223))
- enrich tool call metadata per agent with structured ACP fields ([#1212](https://github.com/kdlbs/kandev/pull/1212))
- fix hold-to-talk on mobile and add coarse-pointer toggle fallback ([#1231](https://github.com/kdlbs/kandev/pull/1231))
- apply repository filter correctly on task board ([#1215](https://github.com/kdlbs/kandev/pull/1215))
- auto-resume dormant sessions after workflow queue ([#1163](https://github.com/kdlbs/kandev/pull/1163))
- reconcile task state when user cancels turn ([#1209](https://github.com/kdlbs/kandev/pull/1209))
- ignore stale session state snapshots blocking idle input ([#1208](https://github.com/kdlbs/kandev/pull/1208))
- destroy terminal shells on tab close with busy confirmation ([#1203](https://github.com/kdlbs/kandev/pull/1203))

### Documentation

- improve agent harness guidance for tests, PR fixup, and debugging ([#1235](https://github.com/kdlbs/kandev/pull/1235))
- clarify default_child_ordering is not enforced outside office ([#1214](https://github.com/kdlbs/kandev/pull/1214))

## 0.54.0 - 2026-05-31

### Features

- show relative commit time on hover in changes panel ([#1199](https://github.com/kdlbs/kandev/pull/1199))
- add "(use step default)" reset to watcher profile selects ([#1124](https://github.com/kdlbs/kandev/pull/1124)) by @nlenepveu
- add PR link to CI status popover header ([#1200](https://github.com/kdlbs/kandev/pull/1200))
- bubble subtask state to parent in sidebar state sort ([#1194](https://github.com/kdlbs/kandev/pull/1194))
- add OS inotify resource limits health check ([#1195](https://github.com/kdlbs/kandev/pull/1195))
- surface subagent task tool calls as cards in task chat ([#1132](https://github.com/kdlbs/kandev/pull/1132))
- add set_session_mode action and preserve mode across reset ([#1188](https://github.com/kdlbs/kandev/pull/1188))
- expose task delete and archive tools to kanban agents ([#1178](https://github.com/kdlbs/kandev/pull/1178))
- expose workflow import tool ([#1177](https://github.com/kdlbs/kandev/pull/1177))
- surface an inline notice when an agent turn produces no output ([#1179](https://github.com/kdlbs/kandev/pull/1179))
- throttle issue-watcher task fan-out (per-watcher cap) ([#1113](https://github.com/kdlbs/kandev/pull/1113)) by @nlenepveu
- confirm session delete when closing multi-session agent tab ([#1174](https://github.com/kdlbs/kandev/pull/1174))
- retry transient provider 529 errors with visible backoff ([#1173](https://github.com/kdlbs/kandev/pull/1173))
- per-session ACP debug logs with rotation and retention ([#1172](https://github.com/kdlbs/kandev/pull/1172))
- auto-expand the first group in the changes panel ([#1169](https://github.com/kdlbs/kandev/pull/1169))
- remote tab in task-create dialog — multi-row GitHub repo picker ([#1116](https://github.com/kdlbs/kandev/pull/1116))
- annotate debug logs with task_id for per-task filtering ([#1168](https://github.com/kdlbs/kandev/pull/1168))
- add PR closed banner and hide CI chip on terminal state ([#1161](https://github.com/kdlbs/kandev/pull/1161))
- collapse mobile task search into a topbar icon ([#1157](https://github.com/kdlbs/kandev/pull/1157))
- showcase merge conflicts in the PR panel ([#1151](https://github.com/kdlbs/kandev/pull/1151))
- allow custom prompts in task creation input ([#1155](https://github.com/kdlbs/kandev/pull/1155))
- unify panel loading states with grid spinner ([#1142](https://github.com/kdlbs/kandev/pull/1142))
- surface agent errors and recover corrupted resume sessions ([#1144](https://github.com/kdlbs/kandev/pull/1144))
- show relative times in remote cloud status tooltip ([#1146](https://github.com/kdlbs/kandev/pull/1146))

### Bug Fixes

- move CI checks link beside popover title ([#1207](https://github.com/kdlbs/kandev/pull/1207))
- collapse large PR Changes and expand Commits by default ([#1206](https://github.com/kdlbs/kandev/pull/1206))
- render multi-level subtasks in sidebar task tree ([#1204](https://github.com/kdlbs/kandev/pull/1204))
- serialize wakeup prompts to prevent turn misalignment ([#1202](https://github.com/kdlbs/kandev/pull/1202))
- remove agent id from subagent metadata chips ([#1205](https://github.com/kdlbs/kandev/pull/1205))
- stop right pane width from drifting across tasks ([#1201](https://github.com/kdlbs/kandev/pull/1201))
- stop changes panel flicker and restore planning chat streaming ([#1197](https://github.com/kdlbs/kandev/pull/1197))
- disambiguate message_task task-not-found vs no-session error ([#1186](https://github.com/kdlbs/kandev/pull/1186))
- add Cursor auto-approve and always-allow permission UI ([#1198](https://github.com/kdlbs/kandev/pull/1198))
- write ACP debug logs under KANDEV_HOME_DIR ([#1196](https://github.com/kdlbs/kandev/pull/1196))
- prevent nested subtask creation for kanban tasks (depth > 1) ([#1192](https://github.com/kdlbs/kandev/pull/1192))
- deliver session broadcasts to focused clients during resume ([#1193](https://github.com/kdlbs/kandev/pull/1193))
- track OS PIDs and clean up shells on task archive/delete ([#1191](https://github.com/kdlbs/kandev/pull/1191))
- portal inline-code tooltip to body to prevent clipping ([#1187](https://github.com/kdlbs/kandev/pull/1187))
- close clarification overlay after agent MCP timeout and dedup retried questions ([#1185](https://github.com/kdlbs/kandev/pull/1185))
- preserve whitespace in acp message chunks ([#1190](https://github.com/kdlbs/kandev/pull/1190))
- detect fork PRs by branch ([#1182](https://github.com/kdlbs/kandev/pull/1182))
- manually drain queued chat messages after cancel ([#1166](https://github.com/kdlbs/kandev/pull/1166))
- keep env prep out of partial tool history ([#1189](https://github.com/kdlbs/kandev/pull/1189))
- exclude office workflows from settings export ([#1184](https://github.com/kdlbs/kandev/pull/1184))
- generate brew-upgrade-resilient systemd unit ([#1180](https://github.com/kdlbs/kandev/pull/1180))
- show CLI-passthrough profiles in watcher dialogs (closes #1107) ([#1108](https://github.com/kdlbs/kandev/pull/1108)) by @nlenepveu
- add ~/.bun/bin to service PATH ([#1175](https://github.com/kdlbs/kandev/pull/1175))
- make left sidebar width global + steady monitor-switch settling ([#1140](https://github.com/kdlbs/kandev/pull/1140))
- forward profile env vars on lazy-recovery createExecution ([#1138](https://github.com/kdlbs/kandev/pull/1138)) by @irium
- dockview/editor restore races, office rebroadcast, and flaky tests ([#1171](https://github.com/kdlbs/kandev/pull/1171))
- make repository setup script failures non-fatal ([#1153](https://github.com/kdlbs/kandev/pull/1153))
- send confirm_name when deleting a workspace from settings ([#1154](https://github.com/kdlbs/kandev/pull/1154))
- keep markdown table headers readable on narrow content ([#1122](https://github.com/kdlbs/kandev/pull/1122))
- stop duplicating workflow auto-start prompt on boot-ready drain ([#1160](https://github.com/kdlbs/kandev/pull/1160))
- pass HTTP MCP servers to ACP agents via AssumeMcpHttp ([#1152](https://github.com/kdlbs/kandev/pull/1152))
- throttle PR branch-detection probes to stop log flood ([#1135](https://github.com/kdlbs/kandev/pull/1135))
- complete Create worktree step before setup script runs ([#1143](https://github.com/kdlbs/kandev/pull/1143))
- backfill task_sessions cost columns for legacy DBs ([#1145](https://github.com/kdlbs/kandev/pull/1145))
- populate prepare progress when switching tasks client-side ([#1150](https://github.com/kdlbs/kandev/pull/1150))

### Refactoring

- move workspace policy params from MCP to agentctl CLI ([#1181](https://github.com/kdlbs/kandev/pull/1181))

### Documentation

- document portable workflow import/export YAML format ([#1176](https://github.com/kdlbs/kandev/pull/1176))

## 0.53.0 - 2026-05-29

### Features

- persist PR-merged banner dismissal in sessionStorage ([#1129](https://github.com/kdlbs/kandev/pull/1129))
- move queued chip into chat status bar row ([#1127](https://github.com/kdlbs/kandev/pull/1127))
- show executor-specific cleanup details in task delete/archive dialog ([#1103](https://github.com/kdlbs/kandev/pull/1103))
- stop filename squeeze on changes-panel row hover ([#1114](https://github.com/kdlbs/kandev/pull/1114))
- normalize /gitlab page to match /github layout ([#1082](https://github.com/kdlbs/kandev/pull/1082))

### Bug Fixes

- honor default dockview widths on task open ([#1136](https://github.com/kdlbs/kandev/pull/1136))
- inherit parent workspace for subtasks (UI + MCP) ([#1131](https://github.com/kdlbs/kandev/pull/1131))
- stop changes panel from showing stale or flickering content ([#1128](https://github.com/kdlbs/kandev/pull/1128))
- repair worktree state and prompt persistence on resume ([#1121](https://github.com/kdlbs/kandev/pull/1121))
- allow IDLE office sessions to accept follow-up prompts ([#1119](https://github.com/kdlbs/kandev/pull/1119))
- show human-readable title and details in permission prompts ([#1101](https://github.com/kdlbs/kandev/pull/1101))
- dockview, sprite, auth, and CLI db backup improvements ([#1075](https://github.com/kdlbs/kandev/pull/1075))
- improve UX for subtasks with repo setup scripts ([#1105](https://github.com/kdlbs/kandev/pull/1105))
- avoid trailing-context crash in multi-file diff panel ([#1097](https://github.com/kdlbs/kandev/pull/1097))
- support model switching for passthrough sessions ([#1100](https://github.com/kdlbs/kandev/pull/1100))
- use official cursor CLI install command in InstallScript() ([#1099](https://github.com/kdlbs/kandev/pull/1099))
- drain orphaned queued messages on agent boot ready ([#1096](https://github.com/kdlbs/kandev/pull/1096))
- restrict branch/dir name sanitizers to ASCII alphanumerics ([#1095](https://github.com/kdlbs/kandev/pull/1095))
- honor base_branch for same-repo subtasks via MCP ([#1093](https://github.com/kdlbs/kandev/pull/1093))
- always allow expanding tool execute to reveal full command ([#1086](https://github.com/kdlbs/kandev/pull/1086))
- force fresh git status on session subscribe ([#1092](https://github.com/kdlbs/kandev/pull/1092))
- keep cancelled office turns promptable instead of parking IDLE ([#1088](https://github.com/kdlbs/kandev/pull/1088))
- drain stuck auto-start prompts after step transitions ([#1087](https://github.com/kdlbs/kandev/pull/1087))
- wire MCP config into CLI passthrough ([#1078](https://github.com/kdlbs/kandev/pull/1078))
- group sidebar tasks by real state ([#1077](https://github.com/kdlbs/kandev/pull/1077))
- remove duplicate breadcrumbs and system sidebar badges ([#1080](https://github.com/kdlbs/kandev/pull/1080))
- wire permission approval UI for Kandev MCP tools ([#1037](https://github.com/kdlbs/kandev/pull/1037)) by @luancm

### Refactoring

- split terminal_handler.go into pumps/sessions/messages ([#1056](https://github.com/kdlbs/kandev/pull/1056))
- split sqlite/base.go and add session batch loader ([#1058](https://github.com/kdlbs/kandev/pull/1058))
- split manager.go into per-concern files ([#1055](https://github.com/kdlbs/kandev/pull/1055))
- split service.go into domain subfiles ([#1053](https://github.com/kdlbs/kandev/pull/1053))
- split adapter.go into per-concern files ([#1054](https://github.com/kdlbs/kandev/pull/1054))
- classify errors via sentinels instead of substring matches ([#1057](https://github.com/kdlbs/kandev/pull/1057))

### Documentation

- note IS_DEBUG runs under vitest in debug-logs skill ([#1137](https://github.com/kdlbs/kandev/pull/1137))

## 0.52.0 - 2026-05-25

### Features

- auto-inject task prompt + dynamic submit sequence + passthrough UX fixes ([#923](https://github.com/kdlbs/kandev/pull/923))
- azure Repos PR follow-ups after #1066 ([#1071](https://github.com/kdlbs/kandev/pull/1071))
- add SSH executor ([#927](https://github.com/kdlbs/kandev/pull/927))
- system settings pages (status, database, backups, logs, updates, licenses, about) ([#942](https://github.com/kdlbs/kandev/pull/942))
- add Azure Repos PR creation ([#1066](https://github.com/kdlbs/kandev/pull/1066)) by @Zaybrah
- add per-agent-profile environment variables ([#1040](https://github.com/kdlbs/kandev/pull/1040)) by @Foprta
- copy gitignored files from repo into new worktrees ([#946](https://github.com/kdlbs/kandev/pull/946)) ([#950](https://github.com/kdlbs/kandev/pull/950))
- make webhook trigger usable end-to-end ([#1051](https://github.com/kdlbs/kandev/pull/1051))
- first-class user terminals — stable seq, rename, park/resume ([#1009](https://github.com/kdlbs/kandev/pull/1009))
- roomier pane caps + remember user-set widths ([#1005](https://github.com/kdlbs/kandev/pull/1005))
- inspect mode with pin and area annotations ([#917](https://github.com/kdlbs/kandev/pull/917))
- add Share to session tab context menu ([#1050](https://github.com/kdlbs/kandev/pull/1050))
- move automations entry above agents in settings sidebar ([#1049](https://github.com/kdlbs/kandev/pull/1049))
- add GitLab integration with MR / discussions support ([#861](https://github.com/kdlbs/kandev/pull/861))
- add tree view for changes panel ([#1026](https://github.com/kdlbs/kandev/pull/1026))
- scroll mobile passthrough terminal scrollback via touch ([#1046](https://github.com/kdlbs/kandev/pull/1046))
- queue workflow messages during active moves ([#1036](https://github.com/kdlbs/kandev/pull/1036))
- forward Preview chat input to PTY in passthrough sessions ([#1042](https://github.com/kdlbs/kandev/pull/1042))
- mobile-friendly /github page with sidebar drawer ([#1041](https://github.com/kdlbs/kandev/pull/1041))
- add minimize button to dockview group header ([#1039](https://github.com/kdlbs/kandev/pull/1039))
- support Jira Server / Data Center ([#977](https://github.com/kdlbs/kandev/pull/977)) by @irium
- truncate file path in editor toolbar with hover-scroll ([#1029](https://github.com/kdlbs/kandev/pull/1029))
- auto-fill task name from PR title when pasting a PR URL ([#1027](https://github.com/kdlbs/kandev/pull/1027))
- don't cascade archive/delete to subtasks by default ([#1020](https://github.com/kdlbs/kandev/pull/1020))
- add Oh My Pi ACP agent ([#971](https://github.com/kdlbs/kandev/pull/971)) by @azais-corentin
- add mobile parity skill ([#1024](https://github.com/kdlbs/kandev/pull/1024))
- /settings/automations with run-mode + per-automation config ([#1016](https://github.com/kdlbs/kandev/pull/1016))

### Bug Fixes

- isPassthroughMode falls back to TaskChatPanel when snapshot is missing (closes #1031) ([#1034](https://github.com/kdlbs/kandev/pull/1034)) by @dbrown99c
- scope /gitlab page tabs to the authenticated user ([#1068](https://github.com/kdlbs/kandev/pull/1068))
- stop the backoff timer in channel relay when context is cancelled ([#1064](https://github.com/kdlbs/kandev/pull/1064)) by @vimzh
- guard Dispatcher's handler map with a RWMutex ([#1065](https://github.com/kdlbs/kandev/pull/1065)) by @vimzh
- disable track_progress for labeled event in claude-review-fork ([#1067](https://github.com/kdlbs/kandev/pull/1067))
- bound idle-timeout task lookup with caller's context ([#1063](https://github.com/kdlbs/kandev/pull/1063)) by @vimzh
- use shared PageTopbar on /gitlab so the page has a header ([#1062](https://github.com/kdlbs/kandev/pull/1062))
- allow native ACP CLIs in probe allowlist ([#1059](https://github.com/kdlbs/kandev/pull/1059))
- make PR CI status reachable on mobile via tap-activated drawer ([#1060](https://github.com/kdlbs/kandev/pull/1060))
- keep toolbar buttons visible when sidebar narrows ([#1047](https://github.com/kdlbs/kandev/pull/1047))
- preserve session-tab grouping when restoring contaminated layout ([#1028](https://github.com/kdlbs/kandev/pull/1028))
- reflect stale CI failures in progress bar ([#1045](https://github.com/kdlbs/kandev/pull/1045))
- preview chat images in modal ([#1030](https://github.com/kdlbs/kandev/pull/1030))
- show tasks from all workflows in the mobile task-switcher sheet ([#1025](https://github.com/kdlbs/kandev/pull/1025))
- aggregate changes count across all repos to stop flicker ([#986](https://github.com/kdlbs/kandev/pull/986))
- isolate kanban and office workspace selection ([#1019](https://github.com/kdlbs/kandev/pull/1019))

### Refactoring

- address watcher dispatch review feedback ([#1074](https://github.com/kdlbs/kandev/pull/1074))
- extract WatcherDispatchCoordinator + WatcherSource ([#1070](https://github.com/kdlbs/kandev/pull/1070)) by @nlenepveu

### Documentation

- add cloud-VM caveats and Playwright install script ([#1069](https://github.com/kdlbs/kandev/pull/1069))
- add remote cloud environment instructions ([#1061](https://github.com/kdlbs/kandev/pull/1061))

## 0.51.0 - 2026-05-22

### Features

- auto-recover from npm _npx cache corruption ([#1013](https://github.com/kdlbs/kandev/pull/1013))
- credit external contributors in release notes ([#975](https://github.com/kdlbs/kandev/pull/975))
- enrich issue watch filters with priority, labels, creator, estimate ([#974](https://github.com/kdlbs/kandev/pull/974)) by @nlenepveu
- add public share links via GitHub Gists ([#995](https://github.com/kdlbs/kandev/pull/995))
- guard delete behind active-session check + UI fixes ([#968](https://github.com/kdlbs/kandev/pull/968))
- structured renderers for Kandev MCP tool calls ([#987](https://github.com/kdlbs/kandev/pull/987))
- add merge button when PR is ready to merge ([#979](https://github.com/kdlbs/kandev/pull/979))
- drag-reorder subtasks within a parent task ([#962](https://github.com/kdlbs/kandev/pull/962))
- add @-mention for tasks in the chat composer ([#963](https://github.com/kdlbs/kandev/pull/963))
- show task linkage on GitHub PR list ([#952](https://github.com/kdlbs/kandev/pull/952)) by @luancm
- collapsible queue chip with animated panel ([#957](https://github.com/kdlbs/kandev/pull/957))

### Bug Fixes

- correct ask_user_question_kandev schema docs and add consistency test ([#1018](https://github.com/kdlbs/kandev/pull/1018))
- hide approve PR button on own PRs ([#1017](https://github.com/kdlbs/kandev/pull/1017))
- keep sidebar 3-dot menu from overlapping the subtask toggle ([#1012](https://github.com/kdlbs/kandev/pull/1012))
- honor window.__KANDEV_DEBUG so make start-debug logs surface ([#991](https://github.com/kdlbs/kandev/pull/991))
- default the commit dialog's stage-all checkbox to unchecked ([#1010](https://github.com/kdlbs/kandev/pull/1010))
- support fork PR worktree creation via pull refspec ([#990](https://github.com/kdlbs/kandev/pull/990))
- route share links through gist.githack so big tasks render ([#1008](https://github.com/kdlbs/kandev/pull/1008))
- build kandev binary with -tags fts5 ([#1007](https://github.com/kdlbs/kandev/pull/1007))
- start per-repo workspace trackers in passthrough mode ([#1002](https://github.com/kdlbs/kandev/pull/1002))
- drain PTY before close on natural child exit ([#1004](https://github.com/kdlbs/kandev/pull/1004))
- honor session passthrough snapshot when starting agent process ([#1003](https://github.com/kdlbs/kandev/pull/1003))
- include ~/.local/bin in service unit PATH for user-mode installs ([#994](https://github.com/kdlbs/kandev/pull/994)) by @nlenepveu
- pick allowed merge method so squash-only repos stop 405ing ([#999](https://github.com/kdlbs/kandev/pull/999))
- stop stuck spinner by writing task.state=REVIEW on engine on_turn_complete ([#1001](https://github.com/kdlbs/kandev/pull/1001))
- include node bin dir in service unit PATH for fnm/nvm/asdf/volta/mise ([#1000](https://github.com/kdlbs/kandev/pull/1000))
- close clarification overlay even when WS event is lost ([#997](https://github.com/kdlbs/kandev/pull/997))
- restore ScheduleWakeup wire string broken by Wakeup→Run rename ([#998](https://github.com/kdlbs/kandev/pull/998))
- gate merge button on required_reviews so it matches GitHub ([#993](https://github.com/kdlbs/kandev/pull/993))
- expand folder optimistically on first click in Files panel ([#984](https://github.com/kdlbs/kandev/pull/984))
- mirror upstream PR formula fixes ([#972](https://github.com/kdlbs/kandev/pull/972))
- propagate RepositoryID so worktree setup script runs ([#969](https://github.com/kdlbs/kandev/pull/969)) by @jcoatelen-ledger
- drain piled-up PR/issue review tasks via cleanup policy + manual sweep ([#929](https://github.com/kdlbs/kandev/pull/929))
- publish AgentReady on wakeup-driven turns ([#875](https://github.com/kdlbs/kandev/pull/875))
- unbreak new-task dialog "Create Task" button ([#976](https://github.com/kdlbs/kandev/pull/976))
- restore padding on office task-creation title input ([#964](https://github.com/kdlbs/kandev/pull/964))
- clear rubocop offenses on formula ([#967](https://github.com/kdlbs/kandev/pull/967))

### Performance

- render stats page shell immediately with per-panel skeletons ([#1022](https://github.com/kdlbs/kandev/pull/1022))

### Refactoring

- drop task-document tools from kanban mode ([#1014](https://github.com/kdlbs/kandev/pull/1014))

## 0.50.0 - 2026-05-19

### Features

- add dev debug logging for git-status, dockview, and chat messages ([#961](https://github.com/kdlbs/kandev/pull/961))
- highlight active-tab file row in Changes panel ([#954](https://github.com/kdlbs/kandev/pull/954))
- mobile file viewer with desktop parity ([#945](https://github.com/kdlbs/kandev/pull/945)) by @luancm
- clearer custom-answer state and Cmd+Enter submit on clarifications ([#934](https://github.com/kdlbs/kandev/pull/934))
- polish chat message rendering ([#935](https://github.com/kdlbs/kandev/pull/935))
- show attachment thumbnails on queued messages ([#936](https://github.com/kdlbs/kandev/pull/936))
- kandev service install for systemd and launchd ([#926](https://github.com/kdlbs/kandev/pull/926))
- autonomous agent management layer ([#914](https://github.com/kdlbs/kandev/pull/914))

### Bug Fixes

- cap clarification overlay height and add mock agent ask commands ([#956](https://github.com/kdlbs/kandev/pull/956))
- pre-fill PR head branch when launching task from GitHub PR list ([#960](https://github.com/kdlbs/kandev/pull/960))
- restore last-selected session tab on task re-entry ([#951](https://github.com/kdlbs/kandev/pull/951))
- detach dispatched handler ctx from WS connection lifetime ([#959](https://github.com/kdlbs/kandev/pull/959))
- persist worktree subdir as workspace_path ([#958](https://github.com/kdlbs/kandev/pull/958)) by @irium
- mobile-layout fixes for onboarding wizard + dialog ([#955](https://github.com/kdlbs/kandev/pull/955))
- skip redundant task-state writes when session state is unchanged ([#953](https://github.com/kdlbs/kandev/pull/953))
- improve inline code chip visibility in markdown ([#948](https://github.com/kdlbs/kandev/pull/948))
- add delete confirmation dialog in task sidebar and mobile sheet ([#949](https://github.com/kdlbs/kandev/pull/949))
- stop inventing currentModelId from AvailableModels[0] ([#947](https://github.com/kdlbs/kandev/pull/947))
- strip phantom session panels on env-layout restore ([#944](https://github.com/kdlbs/kandev/pull/944))
- respect user-chosen agent on workflow step transitions ([#941](https://github.com/kdlbs/kandev/pull/941))
- restore diff viewer bg override after pierre 1.1.22 rename ([#939](https://github.com/kdlbs/kandev/pull/939))
- persist kandev-system wrap on first task prompt ([#940](https://github.com/kdlbs/kandev/pull/940))
- defer Enter to slash/mention suggestion when menu is open ([#928](https://github.com/kdlbs/kandev/pull/928))
- unblock make dev on WSL2 mirrored networking ([#924](https://github.com/kdlbs/kandev/pull/924))

### Refactoring

- improve compact desktop layouts ([#937](https://github.com/kdlbs/kandev/pull/937))
- unify file-tree components on a useTree headless hook ([#919](https://github.com/kdlbs/kandev/pull/919))
- tighten type system across backend + frontend ([#920](https://github.com/kdlbs/kandev/pull/920))

### Documentation

- update roadmap with current priorities and completed items ([#965](https://github.com/kdlbs/kandev/pull/965))
- draft homebrew-core formula and submission spec ([#904](https://github.com/kdlbs/kandev/pull/904))
- capture commit + pr-fixup learnings from #935 ([#943](https://github.com/kdlbs/kandev/pull/943))

## 0.49.0 - 2026-05-16

### Bug Fixes

- ignore pi version banner in utility inference output ([#915](https://github.com/kdlbs/kandev/pull/915)) by @CarmeloCampos
- show PR diff when clicking file row that has local changes ([#908](https://github.com/kdlbs/kandev/pull/908)) by @luancm

## 0.48.0 - 2026-05-15

### Bug Fixes

- fetch Go sha256 from JSON index, bump to 1.26.3 ([#912](https://github.com/kdlbs/kandev/pull/912))

## 0.47.0 - 2026-05-15

### Bug Fixes

- install curl in universal image ([#910](https://github.com/kdlbs/kandev/pull/910))

## 0.46.0 - 2026-05-15

### Features

- docker and sprites executor improvements ([#738](https://github.com/kdlbs/kandev/pull/738))
- replace merged-diff view with timeline + overlay sheets on Changes tab ([#902](https://github.com/kdlbs/kandev/pull/902)) by @luancm
- publish universal image flavor with toolchains + customize docs ([#891](https://github.com/kdlbs/kandev/pull/891))
- differentiate pending permission icon from turn finished in sidebar ([#882](https://github.com/kdlbs/kandev/pull/882)) by @Salim-belkhir

### Bug Fixes

- make conpty Close idempotent and kill orphan agentctl ([#900](https://github.com/kdlbs/kandev/pull/900))
- improve custom script menu readability ([#905](https://github.com/kdlbs/kandev/pull/905))
- use clipboard hook with HTTP fallback for stats copy ([#903](https://github.com/kdlbs/kandev/pull/903))
- anchor manual PR panel open to the session's live group ([#901](https://github.com/kdlbs/kandev/pull/901))
- scope pending-permission scan to current turn ([#899](https://github.com/kdlbs/kandev/pull/899))
- prevent duplicate execution of repository setup scripts ([#898](https://github.com/kdlbs/kandev/pull/898))
- hide stuck Resume session button after click + cover with e2e ([#890](https://github.com/kdlbs/kandev/pull/890))
- drop -l from task shells so PATH keeps agent CLI bin dir ([#889](https://github.com/kdlbs/kandev/pull/889))
- session tab leak, PR icon crash, and PR review prompt scope ([#897](https://github.com/kdlbs/kandev/pull/897))
- release agent ports during E2E reset to prevent shard exhaustion ([#888](https://github.com/kdlbs/kandev/pull/888))

## 0.45.0 - 2026-05-12

### Features

- support tasks without a repository ([#850](https://github.com/kdlbs/kandev/pull/850))

### Bug Fixes

- allow delete/archive of kanban cards in All Workflows view ([#886](https://github.com/kdlbs/kandev/pull/886))
- preserve sidebar scroll position across task switches ([#884](https://github.com/kdlbs/kandev/pull/884))
- lock workflow, block submit during bootstrap, hide None mode ([#885](https://github.com/kdlbs/kandev/pull/885))
- persist container auth, restore tasks filter, refine PR status UI ([#883](https://github.com/kdlbs/kandev/pull/883))

## 0.44.0 - 2026-05-11

### Features

- persist N-entry FIFO message queue ([#864](https://github.com/kdlbs/kandev/pull/864))
- add size/age/backup rotation for file output ([#874](https://github.com/kdlbs/kandev/pull/874)) by @irium

### Bug Fixes

- list remote branches for provider-backed workspace repos ([#876](https://github.com/kdlbs/kandev/pull/876))

### Refactoring

- close rotating sink on shutdown, add config docs ([#877](https://github.com/kdlbs/kandev/pull/877))

## 0.43.0 - 2026-05-11

### Bug Fixes

- resolve subtask base_branch correctly across repos ([#870](https://github.com/kdlbs/kandev/pull/870))
- skip NEXT_PUBLIC_KANDEV_API_PORT in production single-port mode ([#872](https://github.com/kdlbs/kandev/pull/872))

## 0.42.0 - 2026-05-11

### Features

- streaming agent install, PTY login terminal, docker container UX ([#869](https://github.com/kdlbs/kandev/pull/869))
- add CI hover popover on PR top-bar button ([#846](https://github.com/kdlbs/kandev/pull/846))
- prettify Kandev MCP tool titles in chat ([#858](https://github.com/kdlbs/kandev/pull/858))
- show repo name and enable multi-select in pipeline view ([#831](https://github.com/kdlbs/kandev/pull/831)) by @FehTeh
- extend implement plan button with fresh-agent path and server-side plan mode ([#832](https://github.com/kdlbs/kandev/pull/832)) by @luancm

### Bug Fixes

- keep task and session state in sync on tool-event wake ([#865](https://github.com/kdlbs/kandev/pull/865))
- close kanban preview when opening the edit dialog ([#868](https://github.com/kdlbs/kandev/pull/868))
- wire logging.outputPath from config to logger ([#866](https://github.com/kdlbs/kandev/pull/866)) by @irium
- harden release package publishing ([#862](https://github.com/kdlbs/kandev/pull/862))
- unify Kandev branding ([#863](https://github.com/kdlbs/kandev/pull/863))
- warn on duplicate custom prompt name instead of 500 ([#859](https://github.com/kdlbs/kandev/pull/859))
- mobile task switcher sheet skeleton while snapshot loads ([#860](https://github.com/kdlbs/kandev/pull/860))

## 0.41.0 - 2026-05-09

### Features

- allow subtasks to target a sibling repository ([#852](https://github.com/kdlbs/kandev/pull/852))

### Bug Fixes

- preserve dockview state across task and plan-mode switches ([#855](https://github.com/kdlbs/kandev/pull/855))
- show sent message in chat without waiting for ws broadcast ([#851](https://github.com/kdlbs/kandev/pull/851))
- rank longest/quickest tasks by active duration ([#849](https://github.com/kdlbs/kandev/pull/849))
- keep "All Workflows" filter on task nav and workflow.created ([#854](https://github.com/kdlbs/kandev/pull/854))
- keep command palette selection on first result ([#845](https://github.com/kdlbs/kandev/pull/845))

## 0.40.0 - 2026-05-08

### Features

- show grid spinner on agent tab while session is working ([#836](https://github.com/kdlbs/kandev/pull/836))
- mobile parity (session/terminal/repo pickers + multi-terminal) ([#840](https://github.com/kdlbs/kandev/pull/840))
- per-task color indicator in sidebar ([#835](https://github.com/kdlbs/kandev/pull/835))
- rate-limit awareness, poller throttling, and GraphQL batching ([#821](https://github.com/kdlbs/kandev/pull/821))
- pin tasks and drag-to-reorder in sidebar ([#829](https://github.com/kdlbs/kandev/pull/829))
- allow renaming quick-chat tabs locally ([#830](https://github.com/kdlbs/kandev/pull/830))
- multi-question support for ask_user_question_kandev ([#828](https://github.com/kdlbs/kandev/pull/828))
- allow moving tasks across workflows ([#822](https://github.com/kdlbs/kandev/pull/822))
- double-click tab to toggle maximize ([#823](https://github.com/kdlbs/kandev/pull/823))
- cookie-mode integration that triages threads via a utility agent ([#775](https://github.com/kdlbs/kandev/pull/775))
- add Linear issue watchers ([#805](https://github.com/kdlbs/kandev/pull/805))

### Bug Fixes

- respect "All Workflows" selection with multiple workflows ([#844](https://github.com/kdlbs/kandev/pull/844))
- stop dockview layout corruption when switching between maximized / sessionless tasks ([#838](https://github.com/kdlbs/kandev/pull/838))
- hide improve-kandev system workflow from settings UI ([#842](https://github.com/kdlbs/kandev/pull/842))
- abandon orphan turns on session resume ([#837](https://github.com/kdlbs/kandev/pull/837))
- source commit pushed status from git remote, not PR commits ([#833](https://github.com/kdlbs/kandev/pull/833))
- preserve commits in changes panel after refresh ([#834](https://github.com/kdlbs/kandev/pull/834))

### Refactoring

- reframe cross-task message wrapper to authorize action ([#841](https://github.com/kdlbs/kandev/pull/841))
- drop legacy orchestrator WS handlers superseded by session.launch ([#803](https://github.com/kdlbs/kandev/pull/803))

## 0.39.2 - 2026-05-05

### Bug Fixes

- add repository field for npm provenance, patch CLI perms ([#826](https://github.com/kdlbs/kandev/pull/826))

## 0.39.1 - 2026-05-05

### Features

- brew install + npm sibling channels via OIDC trusted publishing ([#806](https://github.com/kdlbs/kandev/pull/806))

### Bug Fixes

- restore PR-and-merge pattern ([#824](https://github.com/kdlbs/kandev/pull/824))

## 0.39 - 2026-05-04

### Features

- attribute cross-task agent messages with a sender badge ([#819](https://github.com/kdlbs/kandev/pull/819))
- add 10 ACP agents, set_config_option, auth_required flow ([#807](https://github.com/kdlbs/kandev/pull/807))
- polish workbench topbars and task-create dialog ([#792](https://github.com/kdlbs/kandev/pull/792))
- show question icon only for pending input ([#782](https://github.com/kdlbs/kandev/pull/782))
- add recent task switcher ([#779](https://github.com/kdlbs/kandev/pull/779))
- collapse commits and PR changes by default in changes panel ([#781](https://github.com/kdlbs/kandev/pull/781))
- support tasks spanning multiple repositories ([#767](https://github.com/kdlbs/kandev/pull/767))
- allow reordering sidebar views ([#764](https://github.com/kdlbs/kandev/pull/764))
- refine workbench chrome ([#761](https://github.com/kdlbs/kandev/pull/761))
- add get_task_conversation tool ([#756](https://github.com/kdlbs/kandev/pull/756))
- add improve kandev in-app contribution flow ([#740](https://github.com/kdlbs/kandev/pull/740))
- refresh + smarter filter for new-task branch selector ([#750](https://github.com/kdlbs/kandev/pull/750))
- poll JQL queries to auto-create tasks (issue watchers) ([#746](https://github.com/kdlbs/kandev/pull/746))
- key terminals + dockview layout to TaskEnvironment ([#755](https://github.com/kdlbs/kandev/pull/755))
- add Linear integration ([#736](https://github.com/kdlbs/kandev/pull/736))
- show active time and elapsed span per task ([#748](https://github.com/kdlbs/kandev/pull/748))
- eager-init agent on profile selection ([#747](https://github.com/kdlbs/kandev/pull/747))
- add message_task_kandev tool ([#745](https://github.com/kdlbs/kandev/pull/745))
- deferred task move with hand-off prompt + boot/turn-end event split ([#743](https://github.com/kdlbs/kandev/pull/743))
- mobile terminal key-bar — Ctrl/Shift modify OS-keyboard input + iOS keyboard fixes ([#741](https://github.com/kdlbs/kandev/pull/741))
- make --port the user-facing port flag ([#737](https://github.com/kdlbs/kandev/pull/737))
- expose MCP server to external coding agents ([#732](https://github.com/kdlbs/kandev/pull/732))
- hide Jira buttons when disabled or auth failing ([#725](https://github.com/kdlbs/kandev/pull/725))
- add GitHub dashboard shortcut to command panel ([#721](https://github.com/kdlbs/kandev/pull/721))

### Bug Fixes

- dedup topbar PR button against auto-shown PR panel ([#812](https://github.com/kdlbs/kandev/pull/812))
- stabilize task switch flow and session recovery ([#818](https://github.com/kdlbs/kandev/pull/818))
- suppress spurious task-failed toasts on resume and reconnect ([#814](https://github.com/kdlbs/kandev/pull/814))
- scope commits/diff to live branch divergence; flatten single-repo ([#816](https://github.com/kdlbs/kandev/pull/816))
- sanitize multi-repo worktree dirs and polish task-create chip dropdown ([#815](https://github.com/kdlbs/kandev/pull/815))
- return mcp message_task once dispatched, not after target turn ends ([#817](https://github.com/kdlbs/kandev/pull/817))
- correct task_environments migration order for older DBs ([#811](https://github.com/kdlbs/kandev/pull/811))
- retry CLI passthrough launch without resume flag after fast-fail ([#810](https://github.com/kdlbs/kandev/pull/810))
- close tooltips on popover close and refine branch chip UX ([#808](https://github.com/kdlbs/kandev/pull/808))
- make executors_running the single source of truth for agent_execution_id ([#799](https://github.com/kdlbs/kandev/pull/799))
- use shared prompt template with placeholder autocomplete ([#801](https://github.com/kdlbs/kandev/pull/801))
- tab session bugs — primary star drift + tab close re-creation ([#800](https://github.com/kdlbs/kandev/pull/800))
- retry session commits fetch when workspace not ready ([#789](https://github.com/kdlbs/kandev/pull/789))
- focus task name on create dialog open ([#788](https://github.com/kdlbs/kandev/pull/788))
- dedupe cancel-turn clicks to stop "turn cancelled" cascade ([#784](https://github.com/kdlbs/kandev/pull/784))
- heal legacy PR watches so single-repo tasks don't dupe PRs ([#785](https://github.com/kdlbs/kandev/pull/785))
- add border to task name input in create dialog ([#783](https://github.com/kdlbs/kandev/pull/783))
- prevent inline code from rendering diagrams ([#773](https://github.com/kdlbs/kandev/pull/773))
- remove settings topbar border ([#774](https://github.com/kdlbs/kandev/pull/774))
- re-key user-shell RPCs to task_environment_id ([#770](https://github.com/kdlbs/kandev/pull/770))
- unblock terminals stuck on Connecting and heal task_environments ([#769](https://github.com/kdlbs/kandev/pull/769))
- prevent orphaned agent subprocesses from concurrent execution creates ([#768](https://github.com/kdlbs/kandev/pull/768))
- restore readable boxed chat hotkey tooltip ([#765](https://github.com/kdlbs/kandev/pull/765))
- make mermaid sanitizer quote-aware and detect parens in bracket labels ([#763](https://github.com/kdlbs/kandev/pull/763))
- refine workbench action button layout ([#762](https://github.com/kdlbs/kandev/pull/762))
- server-authoritative session ensure for kanban preview ([#760](https://github.com/kdlbs/kandev/pull/760))
- guide recovery when session profile is deleted ([#752](https://github.com/kdlbs/kandev/pull/752))
- drop stuck pending permission_request messages ([#723](https://github.com/kdlbs/kandev/pull/723))
- plan panel misses agent updates emitted before WS connects ([#749](https://github.com/kdlbs/kandev/pull/749))
- render passthrough terminal in quick chat ([#744](https://github.com/kdlbs/kandev/pull/744))
- contain kanban card badges within card width ([#739](https://github.com/kdlbs/kandev/pull/739))
- improve create_task_kandev repository and workspace resolution ([#733](https://github.com/kdlbs/kandev/pull/733))
- publish agentctl events on workspace restore ([#731](https://github.com/kdlbs/kandev/pull/731))
- self-recover dockview layout from corrupt persisted state ([#729](https://github.com/kdlbs/kandev/pull/729))
- populate diff for renamed-and-modified files in git status ([#730](https://github.com/kdlbs/kandev/pull/730))
- preserve chat scroll position when maximizing panel ([#728](https://github.com/kdlbs/kandev/pull/728))
- prevent crowding in narrow changes panel and dockview tabs ([#727](https://github.com/kdlbs/kandev/pull/727))
- drain queued message on workflow transition to non-auto-start step ([#726](https://github.com/kdlbs/kandev/pull/726))
- apply profile cli_flags to passthrough command ([#722](https://github.com/kdlbs/kandev/pull/722))
- restore release changelog boundaries ([#716](https://github.com/kdlbs/kandev/pull/716))

### Performance

- speed up stats endpoint aggregation ([#766](https://github.com/kdlbs/kandev/pull/766))

### Refactoring

- move Configuration Chat Agent to utility agents page ([#809](https://github.com/kdlbs/kandev/pull/809))
- drop redundant VCS split button from task top bar ([#804](https://github.com/kdlbs/kandev/pull/804))
- drop dead taskPRs loading state and unused removeTaskPR ([#791](https://github.com/kdlbs/kandev/pull/791))
- move integrations to top-level settings with install-wide configs ([#787](https://github.com/kdlbs/kandev/pull/787))
- use GetOrEnsureExecution for workspace ops ([#786](https://github.com/kdlbs/kandev/pull/786))
- extract shared shapes between jira and linear integrations ([#759](https://github.com/kdlbs/kandev/pull/759))
- route user terminals by environment ([#758](https://github.com/kdlbs/kandev/pull/758))
- drop unused flat-layout worktrees/ path ([#708](https://github.com/kdlbs/kandev/pull/708))

### Documentation

- add integrations banner to README ([#790](https://github.com/kdlbs/kandev/pull/790))
- add issue templates ([#754](https://github.com/kdlbs/kandev/pull/754))
- add star history chart ([#753](https://github.com/kdlbs/kandev/pull/753))

## 0.38 - 2026-04-27

### Features

- add 'Has PR' task sidebar filter ([#713](https://github.com/kdlbs/kandev/pull/713))
- plan checkpointing with rewind UI ([#694](https://github.com/kdlbs/kandev/pull/694))
- add PR preview environments via Sprites ([#707](https://github.com/kdlbs/kandev/pull/707))
- opt-in fresh-branch checkout for local executor task creation ([#695](https://github.com/kdlbs/kandev/pull/695))
- add Jira integration for ticket browsing, import, and task linking ([#705](https://github.com/kdlbs/kandev/pull/705))
- show repository scripts in dockview "+" menu ([#703](https://github.com/kdlbs/kandev/pull/703))
- distinguish CI-passed PRs awaiting review from ready-to-merge ([#702](https://github.com/kdlbs/kandev/pull/702))
- auto-open plan panel with unseen-changes indicator ([#650](https://github.com/kdlbs/kandev/pull/650))
- add /spec for writing feature specs ([#700](https://github.com/kdlbs/kandev/pull/700))
- support claude-acp Monitor tool and fix incremental tool_call updates ([#698](https://github.com/kdlbs/kandev/pull/698))

### Bug Fixes

- scope settings workflow list to current workspace ([#714](https://github.com/kdlbs/kandev/pull/714))
- plug zombie turn leak pinning sessions to RUNNING ([#712](https://github.com/kdlbs/kandev/pull/712))
- toggle off default-on curated CLI flags ([#711](https://github.com/kdlbs/kandev/pull/711))
- flush ScheduleWakeup output via synthetic prompt ([#706](https://github.com/kdlbs/kandev/pull/706))
- show skeleton in file tree header while workspace path loads ([#704](https://github.com/kdlbs/kandev/pull/704))
- use bash for prepare scripts and fix pnpm install in sprites env ([#701](https://github.com/kdlbs/kandev/pull/701))
- align sidebar filter toolbar height with panel headers ([#699](https://github.com/kdlbs/kandev/pull/699))
- derive sidebar session state from most active session ([#697](https://github.com/kdlbs/kandev/pull/697))
- auto-resume failed sessions with silent workspace-restore fallback ([#696](https://github.com/kdlbs/kandev/pull/696))

## 0.37 - 2026-04-25

### Features

- add GitHub token injection for remote executors and Docker session resume ([#654](https://github.com/kdlbs/kandev/pull/654))
- add Ctrl+F search to session, plan, and terminal panels ([#686](https://github.com/kdlbs/kandev/pull/686))
- configurable quick-action presets and PR branch checkout ([#689](https://github.com/kdlbs/kandev/pull/689))
- add /github page for PRs and issues ([#687](https://github.com/kdlbs/kandev/pull/687))
- collapse subtasks in sidebar ([#662](https://github.com/kdlbs/kandev/pull/662))
- review uncommitted changes and add git safety rails ([#684](https://github.com/kdlbs/kandev/pull/684))
- add issue watcher with task creation and auto-cleanup ([#672](https://github.com/kdlbs/kandev/pull/672))
- per-launch authentication for agentctl ([#666](https://github.com/kdlbs/kandev/pull/666))
- configurable CLI flags per agent profile ([#653](https://github.com/kdlbs/kandev/pull/653))

### Bug Fixes

- ui polish — unified topbar, selector consistency, quick actions editor improvements ([#693](https://github.com/kdlbs/kandev/pull/693))
- subtask sessions inherit agent profile from parent task ([#692](https://github.com/kdlbs/kandev/pull/692))
- recover from stale execution ID when auto-starting agent on prepared workspace ([#690](https://github.com/kdlbs/kandev/pull/690))
- apply initialValues when TaskCreateDialog mounts already-open ([#688](https://github.com/kdlbs/kandev/pull/688))
- bound git ref-inspection with context timeout ([#685](https://github.com/kdlbs/kandev/pull/685))
- gateway auth injection and cumulative diff error handling ([#682](https://github.com/kdlbs/kandev/pull/682))
- make task move event handlers asynchronous to prevent HTTP timeouts ([#680](https://github.com/kdlbs/kandev/pull/680))
- user workflow deadlock ([#677](https://github.com/kdlbs/kandev/pull/677))
- unblock Resume for FAILED/CANCELLED task sessions ([#670](https://github.com/kdlbs/kandev/pull/670))
- prevent duplicate --allow-indexing in Auggie passthrough preview ([#675](https://github.com/kdlbs/kandev/pull/675))
- send auto-start prompt after on_turn_complete context reset ([#669](https://github.com/kdlbs/kandev/pull/669))
- include archived tasks in completed tasks over time chart ([#668](https://github.com/kdlbs/kandev/pull/668))
- remove duplicate WebSocket event subscriptions ([#667](https://github.com/kdlbs/kandev/pull/667))

### Refactoring

- unify task.updated via single publisher and shared mapper ([#676](https://github.com/kdlbs/kandev/pull/676))
- move system prompts from Go constants to external config files ([#673](https://github.com/kdlbs/kandev/pull/673))

### Documentation

- improve commit skill with mandatory verify and pre-commit check ([#691](https://github.com/kdlbs/kandev/pull/691))

## 0.36 - 2026-04-20

### Features

- session tabs on kanban preview panel ([#648](https://github.com/kdlbs/kandev/pull/648))

### Bug Fixes

- prevent kanban topbar search from overlapping right buttons ([#661](https://github.com/kdlbs/kandev/pull/661))
- enable Start task button when workflow provides agent override ([#665](https://github.com/kdlbs/kandev/pull/665))
- default dev mode db to <repo>/.kandev-dev/data ([#664](https://github.com/kdlbs/kandev/pull/664))
- stop "Preparing workspace" flashing on step move and refresh stale chats ([#663](https://github.com/kdlbs/kandev/pull/663))
- detect standalone server.js at non-default path ([#660](https://github.com/kdlbs/kandev/pull/660))
- persist attachments on queued message when dequeued ([#659](https://github.com/kdlbs/kandev/pull/659))
- silence spurious errors during Ctrl+C shutdown ([#658](https://github.com/kdlbs/kandev/pull/658))
- exclude ephemeral tasks from stats page queries ([#656](https://github.com/kdlbs/kandev/pull/656))
- add edit icon hint to utility agent rows ([#657](https://github.com/kdlbs/kandev/pull/657))
- tighten default template prompts for commits, todos, and PR review ([#655](https://github.com/kdlbs/kandev/pull/655))

## 0.35 - 2026-04-20

### Features

- sidebar filter UX polish — align ops, group steps by workflow ([#647](https://github.com/kdlbs/kandev/pull/647))
- explain why Start task button is disabled via hover tooltip ([#649](https://github.com/kdlbs/kandev/pull/649))
- add filter/group/sort and saved views to task sidebar ([#644](https://github.com/kdlbs/kandev/pull/644))
- vscode-style preview tabs for files, diffs, and commits ([#622](https://github.com/kdlbs/kandev/pull/622))
- add confirmation dialog before archiving tasks ([#621](https://github.com/kdlbs/kandev/pull/621))
- introduce card multi-selection ([#573](https://github.com/kdlbs/kandev/pull/573)) by @fmmagalhaes

### Bug Fixes

- release script tags fetching
- show repo name instead of full path in task sidebar ([#652](https://github.com/kdlbs/kandev/pull/652))
- unstick agent session when cancel times out ([#651](https://github.com/kdlbs/kandev/pull/651))
- anchor PR detail panel to session group on auto-open ([#646](https://github.com/kdlbs/kandev/pull/646))
- push git snapshot on session focus ([#645](https://github.com/kdlbs/kandev/pull/645))
- follow workflow step session switches in chat UI ([#625](https://github.com/kdlbs/kandev/pull/625))
- stop PR polling for archived tasks ([#643](https://github.com/kdlbs/kandev/pull/643))
- clear kanban snapshots when active workspace changes ([#633](https://github.com/kdlbs/kandev/pull/633))
- persist agent profile mode through bulk-edit save ([#626](https://github.com/kdlbs/kandev/pull/626))
- stream setup script output and keep prepare panel on failure ([#607](https://github.com/kdlbs/kandev/pull/607))
- inject HTTP MCP server for Codex ACP support ([#641](https://github.com/kdlbs/kandev/pull/641))
- route user shell to container instead of host ([#638](https://github.com/kdlbs/kandev/pull/638))
- use UUID fallback for attachments on non-secure contexts ([#640](https://github.com/kdlbs/kandev/pull/640))
- collapse repeated "Resumed agent" boot messages into the last one ([#631](https://github.com/kdlbs/kandev/pull/631))
- use API version negotiation instead of hardcoded 1.41 ([#636](https://github.com/kdlbs/kandev/pull/636))
- wrap long paths in Discard Changes dialog ([#620](https://github.com/kdlbs/kandev/pull/620))
- ux consistency on archiving and deleting tasks ([#627](https://github.com/kdlbs/kandev/pull/627)) by @fmmagalhaes
- apply display filters in list view ([#612](https://github.com/kdlbs/kandev/pull/612)) by @fmmagalhaes
- isolate dev mode state when running inside a kandev task ([#617](https://github.com/kdlbs/kandev/pull/617))
- disable multi-select mode after bulk archive or delete ([#623](https://github.com/kdlbs/kandev/pull/623))

### Performance

- focus-gated git polling to reduce CPU on retained worktrees ([#610](https://github.com/kdlbs/kandev/pull/610))

### Documentation

- add Discord link and require e2e tests for UI changes ([#630](https://github.com/kdlbs/kandev/pull/630))
- refresh README, roadmap, and workflow templates ([#624](https://github.com/kdlbs/kandev/pull/624))

## 0.34 - 2026-04-17

### Features

- render short tool-call output inline ([#604](https://github.com/kdlbs/kandev/pull/604))
- associate agent profiles with workflows and steps ([#597](https://github.com/kdlbs/kandev/pull/597))

### Bug Fixes

- close file diff tab when uncommitted change is undone ([#618](https://github.com/kdlbs/kandev/pull/618))
- stop killing live agents on resume race ([#619](https://github.com/kdlbs/kandev/pull/619))
- treat skipped checks as passing and add ready-to-merge status ([#616](https://github.com/kdlbs/kandev/pull/616))
- prevent agentctl OOM from unbounded diff generation in workspace tracker ([#598](https://github.com/kdlbs/kandev/pull/598))
- prevent utility agents settings page crash on null models ([#602](https://github.com/kdlbs/kandev/pull/602))
- unstick sessions when agent crashes mid-turn ([#609](https://github.com/kdlbs/kandev/pull/609))
- validate activeSessionId belongs to activeTaskId before use ([#614](https://github.com/kdlbs/kandev/pull/614))
- close clarification overlay when agent moves on ([#608](https://github.com/kdlbs/kandev/pull/608))
- prevent duplicate review tasks via atomic PR reservation ([#605](https://github.com/kdlbs/kandev/pull/605))
- stop panels from opening in the left sidebar group ([#603](https://github.com/kdlbs/kandev/pull/603))
- align top-bar right button heights ([#601](https://github.com/kdlbs/kandev/pull/601))

## 0.33 - 2026-04-16

### Features

- add start_agent and local_path params to create_task ([#505](https://github.com/kdlbs/kandev/pull/505))
- acp-first profiles, models, and modes ([#566](https://github.com/kdlbs/kandev/pull/566))

### Bug Fixes

- improve plan comment formatting to match code review style ([#600](https://github.com/kdlbs/kandev/pull/600))
- skip ExtraFiles liveness pipe on Windows to fix agentctl startup ([#599](https://github.com/kdlbs/kandev/pull/599))
- disable resume and show agent selector when profile is deleted ([#578](https://github.com/kdlbs/kandev/pull/578)) by @luancm
- add confirmation dialog before deleting agent profile ([#596](https://github.com/kdlbs/kandev/pull/596))
- replace mermaid bomb-icon error flood with toast notifications ([#594](https://github.com/kdlbs/kandev/pull/594))
- fix dock view task-switching regressions ([#595](https://github.com/kdlbs/kandev/pull/595))
- use dynamic merge-base for git commits to filter main branch commits ([#504](https://github.com/kdlbs/kandev/pull/504))
- read session state at call time in comment run to prevent stale queue ([#588](https://github.com/kdlbs/kandev/pull/588))
- reject session resume when task is archived ([#593](https://github.com/kdlbs/kandev/pull/593))
- migrate agent_profiles to drop CHECK(model != '') constraint ([#590](https://github.com/kdlbs/kandev/pull/590))
- prevent session failure toast from re-appearing after dismiss ([#591](https://github.com/kdlbs/kandev/pull/591))
- inherit repo and default to worktree executor for MCP-created tasks ([#592](https://github.com/kdlbs/kandev/pull/592))
- always enable cgo on build ([#586](https://github.com/kdlbs/kandev/pull/586)) by @xsu1010
- handle submodules in worktree creation ([#579](https://github.com/kdlbs/kandev/pull/579))
- add bottom margin to settings layout ([#581](https://github.com/kdlbs/kandev/pull/581)) by @xsu1010
- correct GitHub org URL in CONTRIBUTING.md ([#584](https://github.com/kdlbs/kandev/pull/584))
- stop vertical scroll on mobile column tabs ([#583](https://github.com/kdlbs/kandev/pull/583)) by @xsu1010
- disable inherited git-crypt filters when repo is locked ([#577](https://github.com/kdlbs/kandev/pull/577))
- handle locked git-crypt repos and localized git errors ([#532](https://github.com/kdlbs/kandev/pull/532))

### Refactoring

- re-key dockview panel state by environmentId instead of sessionId ([#491](https://github.com/kdlbs/kandev/pull/491))

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
- diff bg lines and codex last message ([#186](https://github.com/kdlbs/kandev/pull/186)) by @Copilot
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



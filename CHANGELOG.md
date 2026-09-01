# Changelog

All notable changes to Kandev.

## 0.92.2 - 2026-08-30

### Features

- show previews for expandable thinking messages ([#3160](https://github.com/kdlbs/kandev/pull/3160))
- compact sidebar trailing content ([#3139](https://github.com/kdlbs/kandev/pull/3139))

### Bug Fixes

- preserve recovered workspace path ([#3167](https://github.com/kdlbs/kandev/pull/3167))
- reconcile inactive session transcript windows ([#3159](https://github.com/kdlbs/kandev/pull/3159))
- reconcile stale task state projections ([#3144](https://github.com/kdlbs/kandev/pull/3144))
- use recent profile for new agent default ([#3161](https://github.com/kdlbs/kandev/pull/3161))
- quiesce active turns before workflow context reset ([#3156](https://github.com/kdlbs/kandev/pull/3156))
- show clarification submission status ([#3157](https://github.com/kdlbs/kandev/pull/3157))
- preserve Changes data after session deletion ([#3162](https://github.com/kdlbs/kandev/pull/3162))
- preserve staged and unstaged changes per file ([#3104](https://github.com/kdlbs/kandev/pull/3104))
- accept verified local submodules ([#3136](https://github.com/kdlbs/kandev/pull/3136)) by @yattdev
- expire and reap stale queued board moves ([#3147](https://github.com/kdlbs/kandev/pull/3147)) by @yattdev
- make task creation atomic when a reference fails to resolve ([#3148](https://github.com/kdlbs/kandev/pull/3148)) by @yattdev
- exclude lifecycle turns from task activity ([#3142](https://github.com/kdlbs/kandev/pull/3142))
- resume persisted Quick Chat sessions ([#3138](https://github.com/kdlbs/kandev/pull/3138))
- restore review state after cancelled terminal-step turns ([#3141](https://github.com/kdlbs/kandev/pull/3141))

### Performance

- reduce E2E container setup time ([#3163](https://github.com/kdlbs/kandev/pull/3163))

## 0.92.1 - 2026-08-29

### Features

- rank chat mentions by recency ([#3099](https://github.com/kdlbs/kandev/pull/3099))
- fuse desktop PWA title bar ([#3087](https://github.com/kdlbs/kandev/pull/3087)) by @sonicLee
- track whether work reached the repo, fix success metric ([#2764](https://github.com/kdlbs/kandev/pull/2764)) by @nova28
- remember recent agent profile use ([#3095](https://github.com/kdlbs/kandev/pull/3095))

### Bug Fixes

- retry PR creation after push to absorb GitHub eventual consistency ([#3134](https://github.com/kdlbs/kandev/pull/3134)) by @99hats
- remove shared-worker cleanup races ([#3133](https://github.com/kdlbs/kandev/pull/3133))
- preserve Jira watcher descriptions ([#3130](https://github.com/kdlbs/kandev/pull/3130)) ([#3132](https://github.com/kdlbs/kandev/pull/3132))
- keep chat transcript pinned in WebKit ([#3135](https://github.com/kdlbs/kandev/pull/3135))
- confirm automation deletion ([#3129](https://github.com/kdlbs/kandev/pull/3129))
- harden auto-merge and surface task automation ([#3128](https://github.com/kdlbs/kandev/pull/3128))
- route plugin task moves through the shared step-transition path ([#3123](https://github.com/kdlbs/kandev/pull/3123)) by @nova28
- refresh an open costs page after a task project changes ([#2908](https://github.com/kdlbs/kandev/pull/2908)) by @nova28

## 0.92.0 - 2026-08-28

### Features

- expose saved prompt reads over MCP ([#3118](https://github.com/kdlbs/kandev/pull/3118))
- make task title and description editable ([#2891](https://github.com/kdlbs/kandev/pull/2891)) by @nova28
- add mouse grab panning for desktop boards ([#3052](https://github.com/kdlbs/kandev/pull/3052)) by @yattdev
- improve quick chat tabs ([#3076](https://github.com/kdlbs/kandev/pull/3076))
- explain backup location and row actions ([#3085](https://github.com/kdlbs/kandev/pull/3085))
- plugins can now tell what moving a task to a step will do ([#3053](https://github.com/kdlbs/kandev/pull/3053)) by @nova28
- seat a reviewer automatically so tasks don't stall at Review ([#3043](https://github.com/kdlbs/kandev/pull/3043)) by @nova28
- expose archived tasks, pull requests and board fields to plugins ([#3044](https://github.com/kdlbs/kandev/pull/3044)) by @nova28
- hydrate persisted PR details on task disclosure ([#3055](https://github.com/kdlbs/kandev/pull/3055))
- expand compact workflow step navigation ([#3054](https://github.com/kdlbs/kandev/pull/3054))
- add repository branch policies ([#2997](https://github.com/kdlbs/kandev/pull/2997))
- render lightweight markdown in clarifications ([#3033](https://github.com/kdlbs/kandev/pull/3033))
- recover pull requests removed from merge queues ([#3042](https://github.com/kdlbs/kandev/pull/3042))
- let agents read task comments so parent tasks stop missing handoffs ([#3019](https://github.com/kdlbs/kandev/pull/3019)) by @nova28
- extend markdown table resizing ([#3037](https://github.com/kdlbs/kandev/pull/3037))
- plugin task list facets for sort and group ([#2932](https://github.com/kdlbs/kandev/pull/2932)) by @yattdev
- reclaim remote task directories on terminal archive or delete ([#3000](https://github.com/kdlbs/kandev/pull/3000)) by @nova28
- configure sidebar task-row presentation ([#2955](https://github.com/kdlbs/kandev/pull/2955))
- add configurable continuation and coordinator access ([#2943](https://github.com/kdlbs/kandev/pull/2943))
- give tasks their own command palette scope ([#2979](https://github.com/kdlbs/kandev/pull/2979))
- honor per-host IdentityAgent settings ([#2992](https://github.com/kdlbs/kandev/pull/2992)) by @mulatta
- expose durable pending interactions and response actions ([#2922](https://github.com/kdlbs/kandev/pull/2922))
- add per-run cost/token ledger for task usage ([#2960](https://github.com/kdlbs/kandev/pull/2960)) by @nova28
- add quick chat activity indicators ([#2952](https://github.com/kdlbs/kandev/pull/2952))
- surface plugin update checks and manual updates ([#2513](https://github.com/kdlbs/kandev/pull/2513)) by @yattdev
- add task archive confirmations ([#2945](https://github.com/kdlbs/kandev/pull/2945))
- localize task detach confirmations ([#2892](https://github.com/kdlbs/kandev/pull/2892))
- localize user mutation confirmations ([#2878](https://github.com/kdlbs/kandev/pull/2878))
- inline workflow sync removal confirmation ([#2885](https://github.com/kdlbs/kandev/pull/2885))
- localize integration removal confirmations ([#2887](https://github.com/kdlbs/kandev/pull/2887))
- add local layout deletion confirmations ([#2890](https://github.com/kdlbs/kandev/pull/2890))
- localize watcher delete confirmations ([#2883](https://github.com/kdlbs/kandev/pull/2883))
- prioritize selected picker options ([#2935](https://github.com/kdlbs/kandev/pull/2935))
- localize prompt delete confirmation ([#2882](https://github.com/kdlbs/kandev/pull/2882))
- pin managed runtimes and add update awareness ([#2906](https://github.com/kdlbs/kandev/pull/2906))
- localize session delete confirmations ([#2888](https://github.com/kdlbs/kandev/pull/2888))
- localize agent profile deletion confirmation ([#2881](https://github.com/kdlbs/kandev/pull/2881))
- surface pull request merge queue status ([#2928](https://github.com/kdlbs/kandev/pull/2928))
- add plugin uninstall confirmations ([#2886](https://github.com/kdlbs/kandev/pull/2886))
- localize secret deletion confirmation ([#2884](https://github.com/kdlbs/kandev/pull/2884))
- add PR walkthrough generation workflow ([#2933](https://github.com/kdlbs/kandev/pull/2933))
- localize plan restore confirmation ([#2889](https://github.com/kdlbs/kandev/pull/2889))
- expose agent clarification questions and permission requests externally ([#2875](https://github.com/kdlbs/kandev/pull/2875)) by @nova28
- clarify task sort descriptions ([#2911](https://github.com/kdlbs/kandev/pull/2911)) ([#2920](https://github.com/kdlbs/kandev/pull/2920))
- auto-hide empty steps ([#2815](https://github.com/kdlbs/kandev/pull/2815)) by @gsimard-nordai
- add OAuth 2.1 with PKCE via Atlassian MCP server ([#2805](https://github.com/kdlbs/kandev/pull/2805)) by @ahmedbally
- show completed turn duration on prompts ([#2852](https://github.com/kdlbs/kandev/pull/2852)) by @Fclem
- add per-role tier defaults and explain why an agent's tier differs ([#2853](https://github.com/kdlbs/kandev/pull/2853)) by @nova28
- name each task's repository in the sidebar and task header ([#2917](https://github.com/kdlbs/kandev/pull/2917))
- add PR outcome attribution (merge, close, draft, changed files) ([#2614](https://github.com/kdlbs/kandev/pull/2614)) by @nova28
- use repository-qualified PR comparison targets ([#2828](https://github.com/kdlbs/kandev/pull/2828))
- add dynamic agent profile routing ([#2698](https://github.com/kdlbs/kandev/pull/2698))
- export workspace automations as reviewable YAML ([#2898](https://github.com/kdlbs/kandev/pull/2898)) by @nova28
- add YAML startup configuration parity ([#2863](https://github.com/kdlbs/kandev/pull/2863))
- add task-row tags and sidebar actions ([#2609](https://github.com/kdlbs/kandev/pull/2609)) by @yattdev
- refresh session IP on the throttled touch path ([#2876](https://github.com/kdlbs/kandev/pull/2876)) by @Fclem
- unify prompt editors ([#2855](https://github.com/kdlbs/kandev/pull/2855))
- add rich task and GitLab MR hover previews ([#2610](https://github.com/kdlbs/kandev/pull/2610)) by @yattdev
- surface Cursor subagent metadata via cursor/task ([#2867](https://github.com/kdlbs/kandev/pull/2867))
- localize reset context confirmation ([#2880](https://github.com/kdlbs/kandev/pull/2880))
- localize walkthrough discard confirmation ([#2879](https://github.com/kdlbs/kandev/pull/2879))

### Bug Fixes

- classify expected diagnostic failures ([#3120](https://github.com/kdlbs/kandev/pull/3120))
- strengthen quick chat backdrop ([#3124](https://github.com/kdlbs/kandev/pull/3124))
- route passthrough workflow steps by profile ([#3110](https://github.com/kdlbs/kandev/pull/3110)) ([#3117](https://github.com/kdlbs/kandev/pull/3117))
- isolate Monaco from Vitest ([#3114](https://github.com/kdlbs/kandev/pull/3114)) ([#3121](https://github.com/kdlbs/kandev/pull/3121))
- unblock advisory stop-owner registration ([#3107](https://github.com/kdlbs/kandev/pull/3107)) ([#3116](https://github.com/kdlbs/kandev/pull/3116))
- recover lost children_completed wakes for stalled parents ([#3103](https://github.com/kdlbs/kandev/pull/3103)) by @nova28
- preserve passthrough Escape in Quick Chat ([#3109](https://github.com/kdlbs/kandev/pull/3109)) ([#3119](https://github.com/kdlbs/kandev/pull/3119))
- preserve archived task resume state ([#3115](https://github.com/kdlbs/kandev/pull/3115))
- complete E2E cleanup and quick-chat stabilization ([#3102](https://github.com/kdlbs/kandev/pull/3102))
- rescue tasks stuck running forever after a silent agent stall ([#2975](https://github.com/kdlbs/kandev/pull/2975)) by @nova28
- dispatch on_enter actions for review/approval steps ([#2907](https://github.com/kdlbs/kandev/pull/2907)) by @nova28
- make failed scheduler runs reach the UI and cap taskless-failure inbox noise ([#2968](https://github.com/kdlbs/kandev/pull/2968)) by @nova28
- wake parent for every delegation wave, not just the first ([#3059](https://github.com/kdlbs/kandev/pull/3059)) by @nova28
- auto-start tasks created directly on a start step ([#2967](https://github.com/kdlbs/kandev/pull/2967)) by @nova28
- stop a duplicate lifecycle turn from hiding a pending clarification ([#2989](https://github.com/kdlbs/kandev/pull/2989)) by @nova28
- show live Task header until nested child settles ([#3075](https://github.com/kdlbs/kandev/pull/3075)) by @luancm
- retire stale transient retry notices ([#3098](https://github.com/kdlbs/kandev/pull/3098))
- settle stale automation runs ([#3096](https://github.com/kdlbs/kandev/pull/3096))
- handle missing Windows npm cache trees ([#3092](https://github.com/kdlbs/kandev/pull/3092)) ([#3094](https://github.com/kdlbs/kandev/pull/3094))
- group sidebar tasks by repository combination ([#3091](https://github.com/kdlbs/kandev/pull/3091))
- promoted WIP-queued MCP tasks without sessions are not auto-started. ([#3080](https://github.com/kdlbs/kandev/pull/3080)) by @meanderix
- preserve legacy sqlite database continuity ([#3089](https://github.com/kdlbs/kandev/pull/3089))
- reconcile prepared workspace origins ([#3077](https://github.com/kdlbs/kandev/pull/3077))
- scrub ambient GH_TOKEN in internal/github tests ([#2792](https://github.com/kdlbs/kandev/pull/2792)) by @yattdev
- silence quick chat launcher focus return ([#3086](https://github.com/kdlbs/kandev/pull/3086))
- coordinate task pull request sync consumers ([#3090](https://github.com/kdlbs/kandev/pull/3090))
- stop plan bubble menu transaction loop ([#3083](https://github.com/kdlbs/kandev/pull/3083))
- prevent terminal session revival on workflow re-entry ([#2766](https://github.com/kdlbs/kandev/pull/2766)) by @yattdev
- resolve GitHub clone protocol per host ([#3078](https://github.com/kdlbs/kandev/pull/3078))
- resolve first-use repository task selections on server ([#3068](https://github.com/kdlbs/kandev/pull/3068))
- preserve runner container mode ([#2800](https://github.com/kdlbs/kandev/pull/2800)) by @yattdev
- reuse inherited task environment on session resume ([#3081](https://github.com/kdlbs/kandev/pull/3081))
- resolve Go lint base for forks ([#3074](https://github.com/kdlbs/kandev/pull/3074)) by @yattdev
- prevent ready task environments with empty repo inventory ([#3008](https://github.com/kdlbs/kandev/pull/3008)) by @nova28
- preserve Kandev MCP text results ([#3067](https://github.com/kdlbs/kandev/pull/3067))
- reject stale task path reuse ([#3013](https://github.com/kdlbs/kandev/pull/3013)) by @yattdev
- isolate dev database resolution from ambient environment ([#2910](https://github.com/kdlbs/kandev/pull/2910)) by @yattdev
- dock Plan formatting controls on mobile ([#3056](https://github.com/kdlbs/kandev/pull/3056))
- authorize managed fork credential leases ([#2940](https://github.com/kdlbs/kandev/pull/2940)) by @yattdev
- restore automation sidebar running indicator ([#3064](https://github.com/kdlbs/kandev/pull/3064))
- restore command-center task focus ([#3065](https://github.com/kdlbs/kandev/pull/3065))
- clarify active sidebar task ([#3057](https://github.com/kdlbs/kandev/pull/3057))
- prevent chat pagination scroll flicker ([#3063](https://github.com/kdlbs/kandev/pull/3063))
- explain why a blocked task move failed ([#3047](https://github.com/kdlbs/kandev/pull/3047)) by @luancm
- keep configuration chat launcher visible ([#3062](https://github.com/kdlbs/kandev/pull/3062))
- polish archive confirmation warning surfaces ([#2995](https://github.com/kdlbs/kandev/pull/2995))
- contain growing dialog content ([#3060](https://github.com/kdlbs/kandev/pull/3060))
- improve startup and lifecycle diagnostics ([#3058](https://github.com/kdlbs/kandev/pull/3058))
- require admin for global mutations ([#2816](https://github.com/kdlbs/kandev/pull/2816))
- contain long filenames in Changes rows ([#3049](https://github.com/kdlbs/kandev/pull/3049))
- keep terminal colors readable across themes ([#3018](https://github.com/kdlbs/kandev/pull/3018))
- fail closed on required repository refresh ([#3023](https://github.com/kdlbs/kandev/pull/3023))
- gate focus auto-start on task dependencies ([#3045](https://github.com/kdlbs/kandev/pull/3045))
- enforce share authorization boundary ([#3022](https://github.com/kdlbs/kandev/pull/3022))
- align sidebar PR badges and expose run scope ([#3020](https://github.com/kdlbs/kandev/pull/3020))
- parse glab's token label by structure, not exact wording ([#3038](https://github.com/kdlbs/kandev/pull/3038)) by @nova28
- preserve feeder promotion session routing ([#3016](https://github.com/kdlbs/kandev/pull/3016)) ([#3017](https://github.com/kdlbs/kandev/pull/3017))
- preserve flat models across session resume ([#3030](https://github.com/kdlbs/kandev/pull/3030))
- prevent PR walkthrough link false failures ([#3039](https://github.com/kdlbs/kandev/pull/3039))
- allow dragging office task cards between board columns ([#3014](https://github.com/kdlbs/kandev/pull/3014)) by @nova28
- stop reviews stranding tasks with no recorded decision ([#3009](https://github.com/kdlbs/kandev/pull/3009)) by @nova28
- allow office auto-start after task re-enters a step ([#3011](https://github.com/kdlbs/kandev/pull/3011)) by @nova28
- stack mobile host metrics ([#2959](https://github.com/kdlbs/kandev/pull/2959))
- expand mobile clarification submit target ([#2958](https://github.com/kdlbs/kandev/pull/2958))
- keep composer menus above mobile keyboards ([#2988](https://github.com/kdlbs/kandev/pull/2988))
- scope the SSR terminal-listing routes to the caller ([#3029](https://github.com/kdlbs/kandev/pull/3029))
- deny a workflow whose workspace no longer exists ([#3040](https://github.com/kdlbs/kandev/pull/3040))
- accept plugin-owned repository providers on task create ([#3005](https://github.com/kdlbs/kandev/pull/3005)) by @Corey-Fogg
- accept unresolvable move_to_step targets again ([#3046](https://github.com/kdlbs/kandev/pull/3046))
- require admin for system backups and storage maintenance ([#3036](https://github.com/kdlbs/kandev/pull/3036))
- hide the Docker build control from non-admins ([#3034](https://github.com/kdlbs/kandev/pull/3034))
- scope Office by-ID routes to the caller's workspace ([#3028](https://github.com/kdlbs/kandev/pull/3028))
- scope the workflow-step surface to the workspace owner ([#3031](https://github.com/kdlbs/kandev/pull/3031))
- scope notification providers and delivery to the real user ([#3027](https://github.com/kdlbs/kandev/pull/3027))
- scope Docker management endpoints to the caller ([#3025](https://github.com/kdlbs/kandev/pull/3025))
- guard quick-chat listing on workspace ownership ([#3026](https://github.com/kdlbs/kandev/pull/3026))
- authorize workspace on analytics stats routes ([#3024](https://github.com/kdlbs/kandev/pull/3024))
- use the session auth token for non-Docker launch and resume ([#3007](https://github.com/kdlbs/kandev/pull/3007)) by @nova28
- make e2e adapter tests compile again ([#3015](https://github.com/kdlbs/kandev/pull/3015)) by @nova28
- stop transcript pagination at first prompt ([#3002](https://github.com/kdlbs/kandev/pull/3002))
- treat model with no config options as valid empty resolution ([#3003](https://github.com/kdlbs/kandev/pull/3003))
- recover executor-local npm runtime caches ([#2986](https://github.com/kdlbs/kandev/pull/2986))
- retain executions when runtime stop fails ([#2998](https://github.com/kdlbs/kandev/pull/2998)) by @luancm
- require agents to explicitly signal before advancing the Work step ([#2972](https://github.com/kdlbs/kandev/pull/2972)) by @nova28
- accept same-origin GET on session-authenticated webhooks ([#2984](https://github.com/kdlbs/kandev/pull/2984))
- replace-all the GitHub credential helper ([#2999](https://github.com/kdlbs/kandev/pull/2999)) by @nova28
- gate worktree reuse on live inventory ([#2987](https://github.com/kdlbs/kandev/pull/2987))
- bind the HTTP listener before startup recovery so health checks don't crash-loop ([#2944](https://github.com/kdlbs/kandev/pull/2944)) by @nova28
- scope task title previews to Kanban ([#2939](https://github.com/kdlbs/kandev/pull/2939))
- stop agent continuation summaries from always reading back empty ([#2971](https://github.com/kdlbs/kandev/pull/2971)) by @nova28
- stop a flaky test hang from failing CI on unrelated PRs ([#2993](https://github.com/kdlbs/kandev/pull/2993)) by @nova28
- preserve walkthrough HTML bytes ([#2994](https://github.com/kdlbs/kandev/pull/2994))
- make startup health checks bind-aware ([#2981](https://github.com/kdlbs/kandev/pull/2981))
- make clarification watchdog recovery race-free ([#2982](https://github.com/kdlbs/kandev/pull/2982))
- route agent starts to the first auto-start step ([#2983](https://github.com/kdlbs/kandev/pull/2983))
- stop task plan writes from silently erasing prior content ([#2977](https://github.com/kdlbs/kandev/pull/2977)) by @nova28
- make idle-skip gate actually skip periodic routine wakeups ([#2973](https://github.com/kdlbs/kandev/pull/2973)) by @nova28
- queue runs for reviewers added before a task reaches Review ([#2969](https://github.com/kdlbs/kandev/pull/2969)) by @nova28
- preserve task worktree identity on inventory refresh ([#2980](https://github.com/kdlbs/kandev/pull/2980))
- stop a task getting stuck when the agent's turn fails right after it finishes ([#2963](https://github.com/kdlbs/kandev/pull/2963)) by @nova28
- reuse task workspaces for additional sessions ([#2843](https://github.com/kdlbs/kandev/pull/2843)) by @yattdev
- preserve newest backend diagnostics ([#2929](https://github.com/kdlbs/kandev/pull/2929)) ([#2934](https://github.com/kdlbs/kandev/pull/2934))
- stop run.subscribe from leaking another workspace's run events ([#2961](https://github.com/kdlbs/kandev/pull/2961)) by @nova28
- harden PR walkthrough workflow runner ([#2962](https://github.com/kdlbs/kandev/pull/2962))
- improve agent model picker layout ([#2941](https://github.com/kdlbs/kandev/pull/2941))
- gate plugin webhooks by manifest visibility ([#2608](https://github.com/kdlbs/kandev/pull/2608)) by @yattdev
- classify expected runtime errors correctly ([#2950](https://github.com/kdlbs/kandev/pull/2950))
- escalate a failed sub-agent to the coordinator instead of going silent ([#2948](https://github.com/kdlbs/kandev/pull/2948)) by @nova28
- stop task wakeups from falling back to a generic prompt ([#2956](https://github.com/kdlbs/kandev/pull/2956)) by @nova28
- shorten PR walkthrough links and align shell branding ([#2954](https://github.com/kdlbs/kandev/pull/2954))
- preserve review diff during PR refresh ([#2897](https://github.com/kdlbs/kandev/pull/2897))
- restore complete transcript history ([#2914](https://github.com/kdlbs/kandev/pull/2914)) ([#2927](https://github.com/kdlbs/kandev/pull/2927))
- pass walkthrough reasoning variant separately ([#2949](https://github.com/kdlbs/kandev/pull/2949))
- harden portable PR walkthrough runner ([#2942](https://github.com/kdlbs/kandev/pull/2942))
- localize single-file confirmations ([#2894](https://github.com/kdlbs/kandev/pull/2894))
- enforce MCP question turn boundary ([#2931](https://github.com/kdlbs/kandev/pull/2931))
- prevent review header overlap ([#2899](https://github.com/kdlbs/kandev/pull/2899))
- support local-only merge and rebase ([#2925](https://github.com/kdlbs/kandev/pull/2925))
- correct plugin marketplace attribution ([#2926](https://github.com/kdlbs/kandev/pull/2926))
- resolve mobile Add folder executor race in workspace sources ([#2696](https://github.com/kdlbs/kandev/pull/2696)) by @yattdev
- persist workspace name and description edits on save ([#2859](https://github.com/kdlbs/kandev/pull/2859)) by @nova28
- allow parent workspace-source recovery ([#2842](https://github.com/kdlbs/kandev/pull/2842)) by @yattdev
- flicker-free pinned loading indicator for older prompts ([#2854](https://github.com/kdlbs/kandev/pull/2854)) by @Fclem
- preserve executor fields across kanban task cache merges ([#2705](https://github.com/kdlbs/kandev/pull/2705)) by @yattdev
- scope MR automation switches per linked MR ([#2676](https://github.com/kdlbs/kandev/pull/2676)) by @yattdev
- preserve resumed session model labels ([#2904](https://github.com/kdlbs/kandev/pull/2904))
- prune superseded plugin versions after a confirmed start ([#2921](https://github.com/kdlbs/kandev/pull/2921))
- make integration cards clickable ([#2905](https://github.com/kdlbs/kandev/pull/2905))
- keep dockview tab close buttons reachable in a narrow group ([#2918](https://github.com/kdlbs/kandev/pull/2918))
- restore PR sync context ([#2916](https://github.com/kdlbs/kandev/pull/2916))
- stop agent completion timestamps from staying blank after most runs ([#2902](https://github.com/kdlbs/kandev/pull/2902)) by @nova28
- stop finished runs from showing an empty output summary ([#2900](https://github.com/kdlbs/kandev/pull/2900)) by @nova28
- count a reassigned task's cost under its new project ([#2903](https://github.com/kdlbs/kandev/pull/2903)) by @nova28
- virtualize large Kanban columns ([#2896](https://github.com/kdlbs/kandev/pull/2896))
- add durable task launch failure recovery ([#2832](https://github.com/kdlbs/kandev/pull/2832))
- make quorum-guarded step transitions actually fire ([#2864](https://github.com/kdlbs/kandev/pull/2864)) by @nova28
- retire the session recovery card once the agent boots again ([#2877](https://github.com/kdlbs/kandev/pull/2877)) by @JnManso

### Performance

- move persistent animations to the compositor ([#3122](https://github.com/kdlbs/kandev/pull/3122))
- reduce frontend runtime CPU ([#3093](https://github.com/kdlbs/kandev/pull/3093))
- reduce frontend idle CPU ([#2965](https://github.com/kdlbs/kandev/pull/2965))
- stop re-reading and recompiling workflow steps every trigger ([#2946](https://github.com/kdlbs/kandev/pull/2946)) by @nova28

### Refactoring

- simplify mobile task top bar ([#3035](https://github.com/kdlbs/kandev/pull/3035))
- dedupe truncateUTF8 into shared package ([#2866](https://github.com/kdlbs/kandev/pull/2866))
- render every routed page's chrome through one topbar contract ([#2718](https://github.com/kdlbs/kandev/pull/2718)) by @Aulma

### Documentation

- add product screenshots to public guides ([#3126](https://github.com/kdlbs/kandev/pull/3126))
- improve contributor guidance ([#3097](https://github.com/kdlbs/kandev/pull/3097))
- delete internal add-agent-cli guide describing removed protocols ([#3010](https://github.com/kdlbs/kandev/pull/3010)) by @nova28
- correct comments that describe adapters and protocols that don't exist ([#3012](https://github.com/kdlbs/kandev/pull/3012)) by @nova28
- tighten specification ownership and migration ([#3004](https://github.com/kdlbs/kandev/pull/3004))
- correct three false claims in backend architecture docs ([#3001](https://github.com/kdlbs/kandev/pull/3001)) by @nova28
- migrate legacy specs and add product context ([#2964](https://github.com/kdlbs/kandev/pull/2964))
- add AGENTS.md routing to office specs and traps ([#2970](https://github.com/kdlbs/kandev/pull/2970)) by @nova28
- migrate task and workflow specs ([#2957](https://github.com/kdlbs/kandev/pull/2957))
- explain agent Git permission boundary ([#2951](https://github.com/kdlbs/kandev/pull/2951)) ([#2953](https://github.com/kdlbs/kandev/pull/2953))
- establish system-oriented specification governance ([#2930](https://github.com/kdlbs/kandev/pull/2930))

## 0.91.0 - 2026-08-21

### Features

- add pr-await CI gate to collapse PR polling into one call ([#2873](https://github.com/kdlbs/kandev/pull/2873)) by @nova28
- add localized action confirmations ([#2818](https://github.com/kdlbs/kandev/pull/2818))
- add native agent rich output ([#2773](https://github.com/kdlbs/kandev/pull/2773))
- periodic idle-session reaper on Service ([#2836](https://github.com/kdlbs/kandev/pull/2836)) by @WaleWangPW
- add prompt numbers and auto-load older pages ([#2814](https://github.com/kdlbs/kandev/pull/2814)) by @Fclem

### Bug Fixes

- warn once per peer on untrusted X-Forwarded-Host ([#2865](https://github.com/kdlbs/kandev/pull/2865))
- hide agent profiles that aren't ready from session handoff ([#2874](https://github.com/kdlbs/kandev/pull/2874)) by @nova28
- make pr-state work on macOS's stock bash and jq 1.6 ([#2871](https://github.com/kdlbs/kandev/pull/2871)) by @nova28
- reject unsafe clone authorities ([#2869](https://github.com/kdlbs/kandev/pull/2869)) by @yattdev
- make archived-task git snapshot replay crash-proof and lifecycle-aware ([#2851](https://github.com/kdlbs/kandev/pull/2851)) by @Fclem
- populate Started/Completed timestamps on task detail ([#2856](https://github.com/kdlbs/kandev/pull/2856)) by @nova28
- compact model availability warning ([#2857](https://github.com/kdlbs/kandev/pull/2857))
- show clarification submit spinner ([#2858](https://github.com/kdlbs/kandev/pull/2858))
- reissue managed git leases ([#2850](https://github.com/kdlbs/kandev/pull/2850)) by @WaleWangPW
- make auto-start work for Office tasks and surface failures on the kanban card ([#2847](https://github.com/kdlbs/kandev/pull/2847)) by @nova28
- stop update_agent_profile from renaming agents when only the model changes ([#2849](https://github.com/kdlbs/kandev/pull/2849)) by @nova28
- add terminal tab context menu ([#2817](https://github.com/kdlbs/kandev/pull/2817))
- register prompt history in layout editor ([#2846](https://github.com/kdlbs/kandev/pull/2846)) by @Fclem
- stop agent-created subtasks from losing their project and cost tracking ([#2844](https://github.com/kdlbs/kandev/pull/2844)) by @nova28
- stop reviewers from being skipped and tasks getting stuck in review ([#2830](https://github.com/kdlbs/kandev/pull/2830)) by @nova28
- render pasted Nerd Font glyphs instead of notdef boxes ([#2831](https://github.com/kdlbs/kandev/pull/2831)) by @JnManso
- paste browser links as plain text so URLs survive ([#2804](https://github.com/kdlbs/kandev/pull/2804)) by @JnManso
- stop new Office tasks from disappearing off the board ([#2829](https://github.com/kdlbs/kandev/pull/2829)) by @nova28
- show each agent's own avatar in the reviewer and approver chips ([#2833](https://github.com/kdlbs/kandev/pull/2833)) by @nova28
- preserve FIFO across supersede→requeue ([#2835](https://github.com/kdlbs/kandev/pull/2835)) by @WaleWangPW
- drain queued peer messages on clarification pause + enqueue fast-path ([#2837](https://github.com/kdlbs/kandev/pull/2837)) by @WaleWangPW
- preserve inherited KANDEV_SERVER_HOST for embedded backend ([#2838](https://github.com/kdlbs/kandev/pull/2838)) by @WaleWangPW
- fail-closed profile secret resolution + preserve user-modified profiles ([#2839](https://github.com/kdlbs/kandev/pull/2839)) by @WaleWangPW
- stop agents from being woken by their own comments ([#2840](https://github.com/kdlbs/kandev/pull/2840)) by @nova28
- make dev-prod-db honor env KANDEV_DATABASE_PATH / KANDEV_HOME_DIR ([#2834](https://github.com/kdlbs/kandev/pull/2834)) by @JnManso
- stop reports_to cycles from vanishing agents off the org chart ([#2827](https://github.com/kdlbs/kandev/pull/2827)) by @nova28
- allow changing an agent's manager from the configuration tab ([#2821](https://github.com/kdlbs/kandev/pull/2821)) by @nova28
- stop config import from clobbering concurrent row edits ([#2826](https://github.com/kdlbs/kandev/pull/2826)) by @nova28
- isolate kandev cookies between instances on one host ([#2813](https://github.com/kdlbs/kandev/pull/2813)) by @Fclem
- clear stale dispatch gate after cancel ([#2825](https://github.com/kdlbs/kandev/pull/2825))
- controller advancement — subtask WAITING guard, plan_mode gate, idle-session reclaim ([#2811](https://github.com/kdlbs/kandev/pull/2811)) by @WaleWangPW
- guard against reports_to cycles on import ([#2822](https://github.com/kdlbs/kandev/pull/2822)) by @nova28
- reconcile cross-agent session config on resume ([#2820](https://github.com/kdlbs/kandev/pull/2820))
- keep Create Task open on Escape ([#2803](https://github.com/kdlbs/kandev/pull/2803))
- show model option loading state ([#2806](https://github.com/kdlbs/kandev/pull/2806))
- stop config import from flattening the agent org chart ([#2812](https://github.com/kdlbs/kandev/pull/2812)) by @nova28

## 0.90.0 - 2026-08-19

### Features

- add persistent auto-run controls ([#2778](https://github.com/kdlbs/kandev/pull/2778))
- add workflow export MCP tool ([#2796](https://github.com/kdlbs/kandev/pull/2796))
- refine session MCP server explorer ([#2726](https://github.com/kdlbs/kandev/pull/2726))
- add pre-defined repository sets for bulk task repository selection ([#2774](https://github.com/kdlbs/kandev/pull/2774)) by @jcoatelen-ledger
- add prompt history panel ([#2738](https://github.com/kdlbs/kandev/pull/2738)) by @Fclem
- add last activity task sorting ([#2762](https://github.com/kdlbs/kandev/pull/2762))
- add kandev-plugin-youtrack ([#2768](https://github.com/kdlbs/kandev/pull/2768)) by @ahmedbally
- add quick chat idle dot indicator ([#2750](https://github.com/kdlbs/kandev/pull/2750)) by @Fclem
- add merge queue actions ([#2755](https://github.com/kdlbs/kandev/pull/2755))
- add tiered environment-variable precedence for launch resolution ([#2748](https://github.com/kdlbs/kandev/pull/2748)) by @nova28
- expose integration settings UI, save coordinator, and per-workspace enabled badge to plugins ([#2736](https://github.com/kdlbs/kandev/pull/2736)) by @ahmedbally
- add relative last seen display option to account security ([#2739](https://github.com/kdlbs/kandev/pull/2739)) by @Fclem

### Bug Fixes

- prevent orphaned desktop backends from holding locks ([#2802](https://github.com/kdlbs/kandev/pull/2802))
- restore managed container-run preflights ([#2799](https://github.com/kdlbs/kandev/pull/2799)) by @yattdev
- guard run live-sync snapshot sync against unstable references ([#2797](https://github.com/kdlbs/kandev/pull/2797)) by @Fclem
- render action component on plugin integration cards ([#2794](https://github.com/kdlbs/kandev/pull/2794)) by @ahmedbally
- validate managed Git credential identity before session launch ([#2787](https://github.com/kdlbs/kandev/pull/2787)) by @yattdev
- preserve ACP runtime configuration across reset ([#2790](https://github.com/kdlbs/kandev/pull/2790))
- reconcile resume token when a context reset partially succeeds, and wire the live-ACP guard so it actually runs ([#2788](https://github.com/kdlbs/kandev/pull/2788)) by @yattdev
- forward WIP admission and overflow fields onto hydrated kanban tasks ([#2784](https://github.com/kdlbs/kandev/pull/2784)) by @WaleWangPW
- do not misclassify branch-checked-out-elsewhere as a missing branch ([#2782](https://github.com/kdlbs/kandev/pull/2782)) by @WaleWangPW
- drop cursor placeholder slash-command descriptions ([#2781](https://github.com/kdlbs/kandev/pull/2781))
- preserve typed error chain across launch prepare failure ([#2783](https://github.com/kdlbs/kandev/pull/2783)) by @WaleWangPW
- tokens_out nullable so unmeasured output isn't a fake zero ([#2770](https://github.com/kdlbs/kandev/pull/2770)) by @nova28
- reset executor when returning to repo ([#2786](https://github.com/kdlbs/kandev/pull/2786))
- scope Quick Chat's Escape guards to their own composer/dialog ([#2771](https://github.com/kdlbs/kandev/pull/2771)) by @nova28
- fail never-started agent turns instead of reporting healthy ([#2776](https://github.com/kdlbs/kandev/pull/2776)) by @nova28
- keep the model selector visible for any legacy agent config key ([#2779](https://github.com/kdlbs/kandev/pull/2779)) by @JnManso
- make mobile topbar actions scrollable ([#2769](https://github.com/kdlbs/kandev/pull/2769))
- mark codex usage estimated and backfill cost attribution ([#2772](https://github.com/kdlbs/kandev/pull/2772)) by @nova28
- set LC_ALL=C for CGO Make targets under Git Bash on Windows ([#2720](https://github.com/kdlbs/kandev/pull/2720)) by @JnManso
- clear resume token when reset_agent_context fires without live execution ([#2765](https://github.com/kdlbs/kandev/pull/2765)) by @yattdev
- preserve review path punctuation ([#2777](https://github.com/kdlbs/kandev/pull/2777))
- improve terminal close confirmation ([#2754](https://github.com/kdlbs/kandev/pull/2754))
- keep the local base branch when the pre-worktree pull fails ([#2654](https://github.com/kdlbs/kandev/pull/2654))
- stop Escape from silently rejecting clarification questions ([#2729](https://github.com/kdlbs/kandev/pull/2729)) by @nova28
- silence Windows mkdir "already exists" noise in build logs ([#2753](https://github.com/kdlbs/kandev/pull/2753)) by @JnManso
- honor manual New Agent profile selection ([#2730](https://github.com/kdlbs/kandev/pull/2730)) ([#2749](https://github.com/kdlbs/kandev/pull/2749))
- wake feeder pulls after manual moves ([#2731](https://github.com/kdlbs/kandev/pull/2731)) ([#2746](https://github.com/kdlbs/kandev/pull/2746))
- keep selection and searching state stable across partial content-search results ([#2712](https://github.com/kdlbs/kandev/pull/2712))
- add a stop control and visible output to the dev server preview ([#2725](https://github.com/kdlbs/kandev/pull/2725))
- render clarification context paragraphs ([#2763](https://github.com/kdlbs/kandev/pull/2763))
- replace completed session composer ([#2734](https://github.com/kdlbs/kandev/pull/2734)) ([#2747](https://github.com/kdlbs/kandev/pull/2747))
- use canonical task priority type ([#2733](https://github.com/kdlbs/kandev/pull/2733)) ([#2745](https://github.com/kdlbs/kandev/pull/2745))
- persist labels from HTTP task creation ([#2732](https://github.com/kdlbs/kandev/pull/2732)) ([#2744](https://github.com/kdlbs/kandev/pull/2744))
- prevent stale pending move replay ([#2735](https://github.com/kdlbs/kandev/pull/2735)) by @yattdev
- stop agent profile duplication from rebinding synced steps ([#2740](https://github.com/kdlbs/kandev/pull/2740)) by @nova28
- keep pasted hash text literal ([#2743](https://github.com/kdlbs/kandev/pull/2743))
- resolve turn_metadata for non-hydrated sessions ([#2737](https://github.com/kdlbs/kandev/pull/2737)) by @Fclem

## 0.89.0 - 2026-08-17

### Features

- add executor parity safeguards ([#2704](https://github.com/kdlbs/kandev/pull/2704))
- automate Scoop stable releases ([#2517](https://github.com/kdlbs/kandev/pull/2517))
- standardize settings typography ([#2722](https://github.com/kdlbs/kandev/pull/2722))
- make the plugin row's settings page discoverable ([#2707](https://github.com/kdlbs/kandev/pull/2707))
- derive office-vs-kanban mode from the active workspace, not the URL ([#2677](https://github.com/kdlbs/kandev/pull/2677)) by @Aulma
- route managed Improve Kandev contributions to bound forks ([#2569](https://github.com/kdlbs/kandev/pull/2569))
- let nav items target the sidebar footer icon row ([#2562](https://github.com/kdlbs/kandev/pull/2562)) by @nova28
- add sidebar workspace actions to New Task ([#2607](https://github.com/kdlbs/kandev/pull/2607)) by @yattdev
- add task-level workflow step transition ledger ([#2623](https://github.com/kdlbs/kandev/pull/2623)) by @nova28
- persist subagent tool-call context to task_session_subagents ([#2671](https://github.com/kdlbs/kandev/pull/2671)) by @nova28

### Bug Fixes

- bypass release PR checks with admin token ([#2758](https://github.com/kdlbs/kandev/pull/2758))
- enforce active clarification lifecycle ([#2669](https://github.com/kdlbs/kandev/pull/2669))
- stop re-asking the batched PR watch query once per watch ([#2742](https://github.com/kdlbs/kandev/pull/2742))
- close the i18n coverage gaps in the non-JSX scanner and locale catalogs ([#2727](https://github.com/kdlbs/kandev/pull/2727))
- close four task/session lifecycle footguns in the MCP API ([#2660](https://github.com/kdlbs/kandev/pull/2660))
- keep the model selector visible for sessions with an agent config key ([#2715](https://github.com/kdlbs/kandev/pull/2715)) by @JnManso
- reload app on bfcache restore so duplicated tabs show fresh data ([#2717](https://github.com/kdlbs/kandev/pull/2717)) by @Fclem
- recover stale npm metadata ([#2714](https://github.com/kdlbs/kandev/pull/2714))
- separate Pi ACP and passthrough commands ([#2708](https://github.com/kdlbs/kandev/pull/2708))
- open review panes in selected split ([#2710](https://github.com/kdlbs/kandev/pull/2710))
- externalize copy the jsx-only i18n guard cannot see, and gate it ([#2711](https://github.com/kdlbs/kandev/pull/2711))
- reveal content search matches in editors ([#2681](https://github.com/kdlbs/kandev/pull/2681))
- accept SSH remotes and canonical scope paths for managed Git credentials ([#2678](https://github.com/kdlbs/kandev/pull/2678))
- keep unpushed commits when the fetch retry hits a checked-out branch ([#2658](https://github.com/kdlbs/kandev/pull/2658))
- stabilize nested Review tree expansion ([#2702](https://github.com/kdlbs/kandev/pull/2702))
- authorize credential-less port-proxy subresources after document auth ([#2682](https://github.com/kdlbs/kandev/pull/2682)) by @Fclem
- honor X-Forwarded-For via configurable trusted proxies ([#2701](https://github.com/kdlbs/kandev/pull/2701)) by @Fclem
- persist Codex MCP approval ([#2691](https://github.com/kdlbs/kandev/pull/2691)) ([#2699](https://github.com/kdlbs/kandev/pull/2699)) by @fsmw
- route workflow steps to profile sessions ([#2692](https://github.com/kdlbs/kandev/pull/2692)) ([#2697](https://github.com/kdlbs/kandev/pull/2697)) by @fsmw
- replace clipped start agent button with composer hint ([#2624](https://github.com/kdlbs/kandev/pull/2624)) by @Fclem
- grey out last-active-admin toggles in system users ([#2690](https://github.com/kdlbs/kandev/pull/2690)) by @Fclem
- align SQLite backups with database path ([#2686](https://github.com/kdlbs/kandev/pull/2686))
- preserve cache split, cost provenance, and turn_id on cost events ([#2606](https://github.com/kdlbs/kandev/pull/2606)) by @nova28
- clear deleted session errors ([#2688](https://github.com/kdlbs/kandev/pull/2688))
- delete task-owned MR/PR associations on hard task delete ([#2655](https://github.com/kdlbs/kandev/pull/2655)) by @yattdev
- scroll message metadata dialog entries instead of clipping ([#2683](https://github.com/kdlbs/kandev/pull/2683)) by @Fclem
- keep Send Now replacement from dying on cancel ([#2674](https://github.com/kdlbs/kandev/pull/2674)) by @GodricTM
- retry ACP transport disconnects instead of failing outright ([#2680](https://github.com/kdlbs/kandev/pull/2680)) by @nova28
- capture task_session_commits from live turn and git events ([#2605](https://github.com/kdlbs/kandev/pull/2605)) by @nova28
- preserve repository branch templates ([#2611](https://github.com/kdlbs/kandev/pull/2611)) ([#2684](https://github.com/kdlbs/kandev/pull/2684))
- auto-merge compatible messages at full queues ([#2656](https://github.com/kdlbs/kandev/pull/2656)) by @Fclem
- refine GitHub rate limit card ([#2668](https://github.com/kdlbs/kandev/pull/2668))
- keep PR actions from wrapping the title ([#2670](https://github.com/kdlbs/kandev/pull/2670))
- stop counting unconfigured workspaces as broken connections ([#2666](https://github.com/kdlbs/kandev/pull/2666))

## 0.88.0 - 2026-08-14

### Features

- prioritize active workspace in settings ([#2663](https://github.com/kdlbs/kandev/pull/2663))
- improve task dependency selector ([#2627](https://github.com/kdlbs/kandev/pull/2627))
- show gitlab mr badge on sidebar and tasks list rows ([#2613](https://github.com/kdlbs/kandev/pull/2613)) by @nova28
- add collapsible agent blocks with persisted state ([#2630](https://github.com/kdlbs/kandev/pull/2630)) by @Fclem
- add Traditional Chinese Taiwan and Hong Kong locales ([#2558](https://github.com/kdlbs/kandev/pull/2558)) by @BillChenIDY
- copy and move secrets between global and workspace scopes ([#2583](https://github.com/kdlbs/kandev/pull/2583)) by @Fclem
- queue task moves at WIP capacity ([#2579](https://github.com/kdlbs/kandev/pull/2579))
- streamline agent settings profiles ([#2620](https://github.com/kdlbs/kandev/pull/2620))
- add pin to keep message queue panel open ([#2591](https://github.com/kdlbs/kandev/pull/2591)) by @Fclem
- add task dependencies with auto-start chaining ([#2589](https://github.com/kdlbs/kandev/pull/2589))
- inherit creator session runtime for new tasks ([#2581](https://github.com/kdlbs/kandev/pull/2581))
- auto-merge consecutive queue messages ([#2598](https://github.com/kdlbs/kandev/pull/2598))
- add source-control extension contracts ([#2117](https://github.com/kdlbs/kandev/pull/2117))
- scope run delete-all to the active status view ([#2584](https://github.com/kdlbs/kandev/pull/2584)) by @Fclem
- expose plugin agent tools through task MCP ([#2552](https://github.com/kdlbs/kandev/pull/2552))
- add local-first contribution resolution ([#2560](https://github.com/kdlbs/kandev/pull/2560))
- count accepted step-completion signals ([#2526](https://github.com/kdlbs/kandev/pull/2526)) by @nova28
- add option to prevent agent auto-start on open ([#2556](https://github.com/kdlbs/kandev/pull/2556)) by @Fclem

### Bug Fixes

- restore GitHub rate limit indicator ([#2664](https://github.com/kdlbs/kandev/pull/2664))
- stop task context menu from starting a row drag ([#2662](https://github.com/kdlbs/kandev/pull/2662)) by @Fclem
- reduce clarification context font size ([#2659](https://github.com/kdlbs/kandev/pull/2659))
- avoid plugin shutdown log noise ([#2661](https://github.com/kdlbs/kandev/pull/2661))
- show shared clarification context ([#2641](https://github.com/kdlbs/kandev/pull/2641)) ([#2645](https://github.com/kdlbs/kandev/pull/2645))
- show full-desktop profile actions inline ([#2629](https://github.com/kdlbs/kandev/pull/2629)) ([#2644](https://github.com/kdlbs/kandev/pull/2644))
- recover cutover from orphaned sessions ([#2651](https://github.com/kdlbs/kandev/pull/2651))
- scope the enable toggle to its workspace ([#2631](https://github.com/kdlbs/kandev/pull/2631))
- report missing agent profile and unknown agent as 404 ([#2636](https://github.com/kdlbs/kandev/pull/2636))
- align empty sidebar task message ([#2632](https://github.com/kdlbs/kandev/pull/2632))
- expose kandev MCP tools to custom TUI agents ([#2603](https://github.com/kdlbs/kandev/pull/2603))
- clear stale sidebar queued prompt count ([#2600](https://github.com/kdlbs/kandev/pull/2600)) by @luancm
- verify provider history ordering and retry ([#2626](https://github.com/kdlbs/kandev/pull/2626))
- align task confirmation copy ([#2625](https://github.com/kdlbs/kandev/pull/2625))
- reconcile provider history in Changes timeline ([#2618](https://github.com/kdlbs/kandev/pull/2618))
- pin NODE_ENV so React unit tests can render ([#2568](https://github.com/kdlbs/kandev/pull/2568)) by @yattdev
- render queued attachment previews ([#2622](https://github.com/kdlbs/kandev/pull/2622))
- add validated managed runtime recovery ([#2580](https://github.com/kdlbs/kandev/pull/2580))
- restore network address startup logging ([#2619](https://github.com/kdlbs/kandev/pull/2619))
- align setup and launch timeouts ([#2578](https://github.com/kdlbs/kandev/pull/2578))
- externalize remaining frontend copy ([#2616](https://github.com/kdlbs/kandev/pull/2616))
- preserve session-backed task navigation ([#2601](https://github.com/kdlbs/kandev/pull/2601))
- repair Go-cache quarantine lifecycle ([#2602](https://github.com/kdlbs/kandev/pull/2602))
- inherit default for unavailable utility actions ([#2599](https://github.com/kdlbs/kandev/pull/2599))
- stop a slow snapshot response regressing task status summaries ([#2604](https://github.com/kdlbs/kandev/pull/2604))
- stop TunnelManager deadlocking on its own reservation ([#2597](https://github.com/kdlbs/kandev/pull/2597))
- localize watcher fallback labels ([#2596](https://github.com/kdlbs/kandev/pull/2596))
- resolve utility inference agents by name, not database UUID ([#2594](https://github.com/kdlbs/kandev/pull/2594))
- thread cached input tokens through the session usage rollup ([#2521](https://github.com/kdlbs/kandev/pull/2521)) by @nova28
- stop PR changes refresh flicker ([#2577](https://github.com/kdlbs/kandev/pull/2577))
- make the composer capability usable end to end ([#2575](https://github.com/kdlbs/kandev/pull/2575))
- scope PR CI automation switches per linked PR ([#2512](https://github.com/kdlbs/kandev/pull/2512)) by @yattdev
- don't surface backend-shutdown turn aborts as agent failures ([#2571](https://github.com/kdlbs/kandev/pull/2571))
- quiet expected shutdown/startup log noise ([#2567](https://github.com/kdlbs/kandev/pull/2567))
- record step transitions so session workflow history stops returning empty ([#2520](https://github.com/kdlbs/kandev/pull/2520)) by @nova28
- report effective spawn session profile ([#2561](https://github.com/kdlbs/kandev/pull/2561)) ([#2566](https://github.com/kdlbs/kandev/pull/2566))
- revert collapsible available-to-install section ([#2544](https://github.com/kdlbs/kandev/pull/2544)) ([#2565](https://github.com/kdlbs/kandev/pull/2565)) by @Fclem
- inherit default profile for utility actions ([#2559](https://github.com/kdlbs/kandev/pull/2559))
- refine task confirmation dialogs ([#2553](https://github.com/kdlbs/kandev/pull/2553))
- make the Message Queue settings box fill the page width ([#2555](https://github.com/kdlbs/kandev/pull/2555)) by @Fclem

### Refactoring

- remove core Voice Mode in favour of the Voice plugin ([#2576](https://github.com/kdlbs/kandev/pull/2576))

## 0.87.1 - 2026-08-12

### Features

- add agent profile duplicate ([#2550](https://github.com/kdlbs/kandev/pull/2550)) by @Fclem
- duplicate workflow drafts ([#2539](https://github.com/kdlbs/kandev/pull/2539))
- fill zh-cn gaps against latest main ([#2551](https://github.com/kdlbs/kandev/pull/2551)) by @BillChenIDY
- add voice extraction host APIs ([#2548](https://github.com/kdlbs/kandev/pull/2548))
- add PR event automation pills ([#2522](https://github.com/kdlbs/kandev/pull/2522)) by @luancm
- add only-pin-when-not-empty option for the todo list panel ([#2543](https://github.com/kdlbs/kandev/pull/2543)) by @Fclem
- make status bar a default-off appearance preference ([#2523](https://github.com/kdlbs/kandev/pull/2523))

### Bug Fixes

- prevent cascade archive navigation to descendants ([#2557](https://github.com/kdlbs/kandev/pull/2557))
- preserve canonical worktree during cutover ([#2554](https://github.com/kdlbs/kandev/pull/2554))
- repair GitHub App registration and installation auth ([#2511](https://github.com/kdlbs/kandev/pull/2511)) by @NazyS
- include workspace root with submodules ([#2524](https://github.com/kdlbs/kandev/pull/2524))
- make pinned todos panel fill its hosting panel height ([#2532](https://github.com/kdlbs/kandev/pull/2532)) by @Fclem
- contain rewritten contribution histories ([#2509](https://github.com/kdlbs/kandev/pull/2509))

### Documentation

- prevent em dashes in public copy ([#2516](https://github.com/kdlbs/kandev/pull/2516))

## 0.87.0 - 2026-08-11

### Features

- hide disabled agent profiles from left panel navigation ([#2528](https://github.com/kdlbs/kandev/pull/2528)) by @Fclem
- add MR status chip to chat and passthrough toolbars ([#2537](https://github.com/kdlbs/kandev/pull/2537)) by @nova28
- make available-to-install section collapsible ([#2544](https://github.com/kdlbs/kandev/pull/2544)) by @Fclem
- move voice mode into task behavior ([#2534](https://github.com/kdlbs/kandev/pull/2534))
- restructure settings and unify page chrome behind PageShell ([#2322](https://github.com/kdlbs/kandev/pull/2322)) by @Aulma
- add saved query default views ([#2399](https://github.com/kdlbs/kandev/pull/2399))
- add per-workflow step visibility filter to board ([#2467](https://github.com/kdlbs/kandev/pull/2467)) by @nova28
- center settings save surface ([#2466](https://github.com/kdlbs/kandev/pull/2466))

### Bug Fixes

- correct isolated subtask branch messaging ([#2533](https://github.com/kdlbs/kandev/pull/2533))
- detect wildcard listeners in port availability probe ([#2536](https://github.com/kdlbs/kandev/pull/2536))
- preserve nested health token ownership ([#2538](https://github.com/kdlbs/kandev/pull/2538))
- grant pull-requests write to fork label job ([#2542](https://github.com/kdlbs/kandev/pull/2542)) by @Fclem
- recover hybrid worktree cutovers ([#2531](https://github.com/kdlbs/kandev/pull/2531))
- elect legacy worktree ownership ([#2535](https://github.com/kdlbs/kandev/pull/2535))
- key Top Agents usage by model and stop dropping each range's first day ([#2510](https://github.com/kdlbs/kandev/pull/2510))
- keep caret in task title fields clamped at the limit ([#2529](https://github.com/kdlbs/kandev/pull/2529)) by @Fclem
- truncate title when linking an external issue ([#2527](https://github.com/kdlbs/kandev/pull/2527)) by @Fclem
- auto-link gitlab mrs for self-managed hosts ([#2515](https://github.com/kdlbs/kandev/pull/2515)) by @nova28
- show dark mode toggle in mobile menu ([#2514](https://github.com/kdlbs/kandev/pull/2514)) by @nova28
- recreate the worktree when resuming an unarchived task ([#2508](https://github.com/kdlbs/kandev/pull/2508))

## 0.86.1 - 2026-08-10

### Features

- move make dev to the native Go launcher ([#2411](https://github.com/kdlbs/kandev/pull/2411))

### Bug Fixes

- preserve flat owner over terminal history ([#2505](https://github.com/kdlbs/kandev/pull/2505)) ([#2506](https://github.com/kdlbs/kandev/pull/2506))
- keep a PR watch per branch so multi-branch automation works ([#2504](https://github.com/kdlbs/kandev/pull/2504))
- scope task state, move and delete to the calling user ([#2503](https://github.com/kdlbs/kandev/pull/2503))
- serialize workspace stream websocket writes ([#2502](https://github.com/kdlbs/kandev/pull/2502))
- allow git pull --rebase past the flag allowlist ([#2501](https://github.com/kdlbs/kandev/pull/2501))
- copy the watch handed to the initial-check goroutine ([#2492](https://github.com/kdlbs/kandev/pull/2492))
- separate debug and development launch profiles ([#2487](https://github.com/kdlbs/kandev/pull/2487))
- normalize OMP shell command output ([#2480](https://github.com/kdlbs/kandev/pull/2480)) ([#2486](https://github.com/kdlbs/kandev/pull/2486))
- preserve Homebrew formula version ([#2488](https://github.com/kdlbs/kandev/pull/2488))
- re-run bootstrap when the dialog reopens ([#2484](https://github.com/kdlbs/kandev/pull/2484)) by @Fclem

### Refactoring

- split execution creation ([#2382](https://github.com/kdlbs/kandev/pull/2382)) ([#2485](https://github.com/kdlbs/kandev/pull/2485))

## 0.86.0 - 2026-08-10

### Features

- add github_pr_merged trigger type ([#2462](https://github.com/kdlbs/kandev/pull/2462)) by @nova28
- open the workspace picker with a keyboard shortcut ([#2478](https://github.com/kdlbs/kandev/pull/2478)) by @Aulma
- improve task PR status hover ([#2393](https://github.com/kdlbs/kandev/pull/2393))
- no silent model fallback — explicit failure, gone models, per-profile fallback ([#2385](https://github.com/kdlbs/kandev/pull/2385)) by @Fclem
- add task autopilot mode and MCP profiles ([#2445](https://github.com/kdlbs/kandev/pull/2445))
- use agent profiles for utility agents ([#2430](https://github.com/kdlbs/kandev/pull/2430))
- add Dev and Preview title prefixes ([#2464](https://github.com/kdlbs/kandev/pull/2464))
- add external_id create idempotency ([#2440](https://github.com/kdlbs/kandev/pull/2440)) by @nova28
- add owned temporary artifact cleanup ([#2444](https://github.com/kdlbs/kandev/pull/2444))
- add configurable browser tab title prefix ([#2459](https://github.com/kdlbs/kandev/pull/2459)) by @Aulma
- reorder queued messages by drag and drop ([#2420](https://github.com/kdlbs/kandev/pull/2420)) by @Fclem
- add GitHub Status to the marketplace registry ([#2452](https://github.com/kdlbs/kandev/pull/2452))
- add list_task_sessions_kandev MCP tool ([#2442](https://github.com/kdlbs/kandev/pull/2442))
- add provider-neutral agent error recovery ([#2437](https://github.com/kdlbs/kandev/pull/2437))
- speed up scans and add dependency cleanup ([#2432](https://github.com/kdlbs/kandev/pull/2432))
- resolve model-dependent provider options ([#2404](https://github.com/kdlbs/kandev/pull/2404))
- add MR auto-fix, auto-merge, hover preview, badge ([#2301](https://github.com/kdlbs/kandev/pull/2301)) by @yattdev
- surface kandev version on /health ([#2416](https://github.com/kdlbs/kandev/pull/2416)) by @nova28
- surface port forwarding controls and browser navigation ([#2383](https://github.com/kdlbs/kandev/pull/2383))
- expand host.ui and add host.toast / host.utils ([#2410](https://github.com/kdlbs/kandev/pull/2410))
- show an Updating… indicator in the PR status popover ([#2402](https://github.com/kdlbs/kandev/pull/2402))
- sync zh-cn catalogs with main and proofread ([#2397](https://github.com/kdlbs/kandev/pull/2397)) by @BillChenIDY
- add a task-card-tags slot ([#2332](https://github.com/kdlbs/kandev/pull/2332)) by @yattdev
- show unmeasured state for zero-usage context window ([#2392](https://github.com/kdlbs/kandev/pull/2392))
- surface allowlisted OpenCode remediation links in recovery surfaces ([#2396](https://github.com/kdlbs/kandev/pull/2396))
- add GitLab support to workflow sync ([#2289](https://github.com/kdlbs/kandev/pull/2289)) by @nova28
- add agent todo list panel with settings toggle and + menu ([#2327](https://github.com/kdlbs/kandev/pull/2327)) by @ClemDNL
- add per-integration enable/disable toggle ([#2364](https://github.com/kdlbs/kandev/pull/2364)) by @ClemDNL
- close the pt-PT parity gap (1,794 keys) ([#2386](https://github.com/kdlbs/kandev/pull/2386))
- isolate improve-kandev tasks in a dedicated workspace ([#2347](https://github.com/kdlbs/kandev/pull/2347)) by @Fclem
- add admin toggle for queued message merging ([#2342](https://github.com/kdlbs/kandev/pull/2342)) by @ClemDNL
- add primary task-menu group and registerTaskFilter hook ([#2351](https://github.com/kdlbs/kandev/pull/2351)) by @yattdev
- show red interrupted icon for tasks cut short by a restart ([#2358](https://github.com/kdlbs/kandev/pull/2358)) by @Fclem
- list kandev-plugin-notes ([#2352](https://github.com/kdlbs/kandev/pull/2352)) by @yattdev
- complete Simplified Chinese locale catalogs ([#2353](https://github.com/kdlbs/kandev/pull/2353)) by @BillChenIDY
- accept port overrides on the dev target ([#2368](https://github.com/kdlbs/kandev/pull/2368)) by @JnManso
- show queued prompt count badge in the task sidebar ([#2354](https://github.com/kdlbs/kandev/pull/2354)) by @Fclem
- re-parent tasks by drag and drop in the sidebar ([#2355](https://github.com/kdlbs/kandev/pull/2355)) by @Fclem
- add optional workflow-level agent prompt ([#2338](https://github.com/kdlbs/kandev/pull/2338)) by @luancm
- localize app/office, the last un-migrated area ([#2367](https://github.com/kdlbs/kandev/pull/2367))
- add source runtime bundle target ([#2277](https://github.com/kdlbs/kandev/pull/2277))
- compact kandev MCP tool results ([#2337](https://github.com/kdlbs/kandev/pull/2337)) by @irium
- localize the last batch of live surfaces before Office ([#2357](https://github.com/kdlbs/kandev/pull/2357))
- archive the current task from the command palette ([#2346](https://github.com/kdlbs/kandev/pull/2346))
- show disabled badge for profiles in settings sidebar ([#2339](https://github.com/kdlbs/kandev/pull/2339)) by @Fclem
- localize remaining task panes and create-dialog surfaces ([#2326](https://github.com/kdlbs/kandev/pull/2326))
- localize loose components directly under components/ ([#2317](https://github.com/kdlbs/kandev/pull/2317))
- add European Portuguese (pt-PT) locale ([#2349](https://github.com/kdlbs/kandev/pull/2349))
- list kandev-plugin-tags ([#2333](https://github.com/kdlbs/kandev/pull/2333)) by @yattdev
- add responsive, resizable markdown tables ([#2098](https://github.com/kdlbs/kandev/pull/2098))
- support nested submodule review ([#2295](https://github.com/kdlbs/kandev/pull/2295))
- add Send Now dispatch controls ([#2292](https://github.com/kdlbs/kandev/pull/2292))
- localize task simple, mobile, share, filter and inspector panes ([#2316](https://github.com/kdlbs/kandev/pull/2316))
- add mid-turn steering for Claude ACP sessions ([#2241](https://github.com/kdlbs/kandev/pull/2241)) by @nova28
- make the automation the object, with its own runs destination ([#2190](https://github.com/kdlbs/kandev/pull/2190)) by @nova28
- unify terminal and chat tabs ([#2235](https://github.com/kdlbs/kandev/pull/2235))
- localize task chat surface ([#2303](https://github.com/kdlbs/kandev/pull/2303))
- localize diff viewer, VCS dialogs, integrations, agent editor and SPA routes ([#2302](https://github.com/kdlbs/kandev/pull/2302))
- localize editors and quick-chat ([#2300](https://github.com/kdlbs/kandev/pull/2300))
- localize review dialog and kanban board ([#2299](https://github.com/kdlbs/kandev/pull/2299))
- add scoped repository secret environments ([#2237](https://github.com/kdlbs/kandev/pull/2237))
- add enable/disable toggle for agent profiles ([#2248](https://github.com/kdlbs/kandev/pull/2248)) by @Fclem
- localize stats, tasks list, and auth pages ([#2291](https://github.com/kdlbs/kandev/pull/2291))
- add file tree chat context ([#2266](https://github.com/kdlbs/kandev/pull/2266))
- support remote contribution tasks ([#2265](https://github.com/kdlbs/kandev/pull/2265))
- add task panel, menu, and storage hooks ([#2152](https://github.com/kdlbs/kandev/pull/2152)) by @yattdev
- prevent host sleep during active tasks ([#2264](https://github.com/kdlbs/kandev/pull/2264))
- rename generated task branches ([#2262](https://github.com/kdlbs/kandev/pull/2262))
- add task MR lifecycle notifications ([#2125](https://github.com/kdlbs/kandev/pull/2125)) by @yattdev
- localize the Azure DevOps, Jira, Linear and Sentry task surfaces ([#2284](https://github.com/kdlbs/kandev/pull/2284))
- localize the GitHub and GitLab task surfaces ([#2283](https://github.com/kdlbs/kandev/pull/2283))
- add settings discovery ([#2281](https://github.com/kdlbs/kandev/pull/2281))
- localize Settings Plugins, Account and the SSH/executors tree ([#2249](https://github.com/kdlbs/kandev/pull/2249))
- run language servers on task hosts ([#1863](https://github.com/kdlbs/kandev/pull/1863))
- support 100 MiB prompt attachments ([#2261](https://github.com/kdlbs/kandev/pull/2261))
- add Simplified Chinese locale ([#2243](https://github.com/kdlbs/kandev/pull/2243)) by @BillChenIDY
- drive navigation surfaces from one manifest ([#2233](https://github.com/kdlbs/kandev/pull/2233)) by @Aulma

### Bug Fixes

- unify CI auto-fix direct-dispatch metadata ([#2469](https://github.com/kdlbs/kandev/pull/2469)) by @yattdev
- correct stale plugin-hook availability claims ([#2309](https://github.com/kdlbs/kandev/pull/2309)) by @yattdev
- guard agent execution metadata against concurrent map access ([#2472](https://github.com/kdlbs/kandev/pull/2472))
- ignore inherited indexed Git config beyond GIT_CONFIG_COUNT ([#2475](https://github.com/kdlbs/kandev/pull/2475))
- home borrowed environments in the worktree cutover ([#2473](https://github.com/kdlbs/kandev/pull/2473))
- close the lint-invisible gaps the guard structurally cannot see ([#2470](https://github.com/kdlbs/kandev/pull/2470))
- recover cutover from stale session metadata ([#2471](https://github.com/kdlbs/kandev/pull/2471))
- guard backend runtime-state ownership ([#2468](https://github.com/kdlbs/kandev/pull/2468))
- honor hide-disabled-in-nav in the settings integrations tree ([#2461](https://github.com/kdlbs/kandev/pull/2461)) by @Fclem
- destroy every repo's worktree during multi-repo task cleanup ([#2460](https://github.com/kdlbs/kandev/pull/2460)) by @nova28
- recover legacy worktree ownership cutover ([#2463](https://github.com/kdlbs/kandev/pull/2463))
- show install loading state ([#2454](https://github.com/kdlbs/kandev/pull/2454))
- preserve task-owned worktrees across session deletion ([#2456](https://github.com/kdlbs/kandev/pull/2456))
- shrink oversized message favorite star on mobile ([#2451](https://github.com/kdlbs/kandev/pull/2451)) by @Fclem
- support unitless slow durations ([#2450](https://github.com/kdlbs/kandev/pull/2450))
- render hidden workflows when explicitly selected ([#2446](https://github.com/kdlbs/kandev/pull/2446)) by @Fclem
- render workflow description in settings ([#2418](https://github.com/kdlbs/kandev/pull/2418)) by @nova28
- show an ended session instead of a lying spinner ([#2423](https://github.com/kdlbs/kandev/pull/2423)) by @JnManso
- delete workspaces via the non-office route when office is off ([#2455](https://github.com/kdlbs/kandev/pull/2455))
- persist PR state from the feedback fetch so surfaces agree ([#2449](https://github.com/kdlbs/kandev/pull/2449))
- match notifications type scale to other settings pages ([#2447](https://github.com/kdlbs/kandev/pull/2447))
- close pt-PT catalog drift — add 121 keys, drop 7 dead ones ([#2441](https://github.com/kdlbs/kandev/pull/2441))
- publish step created/updated events from the REST surface ([#2438](https://github.com/kdlbs/kandev/pull/2438))
- stop MR badge specs racing workflow auto-advance ([#2443](https://github.com/kdlbs/kandev/pull/2443))
- sync PR rows no watch points at so merged PRs stop reading as open ([#2429](https://github.com/kdlbs/kandev/pull/2429))
- contain backend runtime failure paths ([#2431](https://github.com/kdlbs/kandev/pull/2431))
- prepare SSH workspaces before agentctl ([#2413](https://github.com/kdlbs/kandev/pull/2413)) ([#2439](https://github.com/kdlbs/kandev/pull/2439))
- accept the Windows executable suffix in the ACP probe allow-list ([#2422](https://github.com/kdlbs/kandev/pull/2422)) by @JnManso
- refine agent session tab feedback ([#2426](https://github.com/kdlbs/kandev/pull/2426))
- remember task-create workflows per workspace ([#2428](https://github.com/kdlbs/kandev/pull/2428))
- bump dompurify override to 3.4.13 (GHSA-55q2-fjhq-7xh7) ([#2433](https://github.com/kdlbs/kandev/pull/2433))
- size file-search results from trusted bounds ([#2434](https://github.com/kdlbs/kandev/pull/2434))
- count diff lines by hunk position, not by "+++"/"---" prefix ([#2427](https://github.com/kdlbs/kandev/pull/2427))
- prevent premature pending-action clearing ([#2424](https://github.com/kdlbs/kandev/pull/2424))
- resolve configure_session agent families so per-step models apply ([#2414](https://github.com/kdlbs/kandev/pull/2414)) by @nova28
- route make test-e2e through managed runner ([#2412](https://github.com/kdlbs/kandev/pull/2412)) by @nova28
- externalize four SCREAMING_CASE copy tables the lint rule cannot see ([#2409](https://github.com/kdlbs/kandev/pull/2409))
- extend the inherited PATH key when prepending the GitHub CLI shim ([#2415](https://github.com/kdlbs/kandev/pull/2415)) by @JnManso
- explain why a one-shot ACP probe failed ([#2417](https://github.com/kdlbs/kandev/pull/2417)) by @JnManso
- stop tool calls from driving the pending-input icon ([#2406](https://github.com/kdlbs/kandev/pull/2406))
- repair modal tooltips, live theme, and min_kandev_version ([#2408](https://github.com/kdlbs/kandev/pull/2408))
- harden launcher port ownership ([#2394](https://github.com/kdlbs/kandev/pull/2394))
- make archive_task_kandev idempotent when already archived ([#2405](https://github.com/kdlbs/kandev/pull/2405))
- log canceled git status refreshes at debug ([#2398](https://github.com/kdlbs/kandev/pull/2398))
- correct a plural _one form and collapse a twinned key in en ([#2395](https://github.com/kdlbs/kandev/pull/2395))
- clear the error icon once the agent recovers ([#2391](https://github.com/kdlbs/kandev/pull/2391))
- stop reporting abandoned requests as server errors ([#2384](https://github.com/kdlbs/kandev/pull/2384)) by @JnManso
- close four gaps no gate can see (plurals, fallback copy, orphans) ([#2388](https://github.com/kdlbs/kandev/pull/2388))
- bump mermaid to 11.16.1 and js-yaml to 4.3.1 ([#2387](https://github.com/kdlbs/kandev/pull/2387))
- allowlist GitHub credential broker and app webhook ([#2310](https://github.com/kdlbs/kandev/pull/2310)) by @yattdev
- close two mid-turn steering gaps found by post-merge Codex review ([#2323](https://github.com/kdlbs/kandev/pull/2323)) by @nova28
- stop resume loop on first launch of an unstarted session ([#2331](https://github.com/kdlbs/kandev/pull/2331)) by @nlenepveu
- hide empty shell output disclosure ([#2378](https://github.com/kdlbs/kandev/pull/2378))
- support gh account status on stderr ([#2348](https://github.com/kdlbs/kandev/pull/2348)) ([#2379](https://github.com/kdlbs/kandev/pull/2379))
- restore browser inspect annotation submission ([#2374](https://github.com/kdlbs/kandev/pull/2374)) ([#2381](https://github.com/kdlbs/kandev/pull/2381))
- reject terminal sessions before creating an execution ([#2325](https://github.com/kdlbs/kandev/pull/2325)) by @JnManso
- pass the routine cron example through as a value ([#2373](https://github.com/kdlbs/kandev/pull/2373))
- default custom TUI agents to Ink-safe passthrough stdin ([#2369](https://github.com/kdlbs/kandev/pull/2369)) by @nlenepveu
- reap and retry cold-resume prompt-readiness timeout ([#2356](https://github.com/kdlbs/kandev/pull/2356)) by @ClemDNL
- deliver prompt to custom TUI agents via stdin ([#2335](https://github.com/kdlbs/kandev/pull/2335)) by @nlenepveu
- report workflow-default profile that outranks explicit on a step ([#2341](https://github.com/kdlbs/kandev/pull/2341))
- tie the pseudo-coverage oracle to the screen it claims to check ([#2366](https://github.com/kdlbs/kandev/pull/2366))
- localize date-fns distances and drop redundant t() test stubs ([#2363](https://github.com/kdlbs/kandev/pull/2363))
- correct English singular forms that carried a plural noun ([#2350](https://github.com/kdlbs/kandev/pull/2350))
- strip CRLF multi-line command echoes with no reported cwd ([#2334](https://github.com/kdlbs/kandev/pull/2334)) by @ClemDNL
- make host-shell PTYs durable across reloads ([#2345](https://github.com/kdlbs/kandev/pull/2345))
- resolve open npm Dependabot alerts ([#2340](https://github.com/kdlbs/kandev/pull/2340))
- clear the pending-input icon when a request resolves ([#2343](https://github.com/kdlbs/kandev/pull/2343))
- report the agent profile that will actually launch ([#2329](https://github.com/kdlbs/kandev/pull/2329)) by @nlenepveu
- let Cursor sessions leave fast mode (parameterizedModelPicker) ([#2307](https://github.com/kdlbs/kandev/pull/2307)) by @jean-losi
- accept file paths in i18n-sweep and fix-trans-indices ([#2324](https://github.com/kdlbs/kandev/pull/2324))
- enforce per-user workspace authorization in workflow sync ([#2311](https://github.com/kdlbs/kandev/pull/2311)) by @nova28
- keep todo indicator alive across session resume ([#2315](https://github.com/kdlbs/kandev/pull/2315)) by @irium
- externalize blocker-cycle toast and cover task-pane helpers ([#2321](https://github.com/kdlbs/kandev/pull/2321))
- harden remote contribution launch safety ([#2294](https://github.com/kdlbs/kandev/pull/2294))
- reset filtered command list scroll ([#2318](https://github.com/kdlbs/kandev/pull/2318))
- gate session hydration on subscription ack ([#2287](https://github.com/kdlbs/kandev/pull/2287)) ([#2296](https://github.com/kdlbs/kandev/pull/2296))
- show spinner while deleting session tabs ([#2298](https://github.com/kdlbs/kandev/pull/2298))
- stop unrelated task.updated events from un-nesting a subtask ([#2207](https://github.com/kdlbs/kandev/pull/2207)) by @ClemDNL
- seed every workspace with its stored base branch ([#2270](https://github.com/kdlbs/kandev/pull/2270)) by @nova28
- stop workflow step reset from double-starting the agent ([#2263](https://github.com/kdlbs/kandev/pull/2263)) by @nova28
- initialize new repositories with a main branch ([#2297](https://github.com/kdlbs/kandev/pull/2297))
- reveal command-selected sidebar tasks ([#2290](https://github.com/kdlbs/kandev/pull/2290))
- persist default sidebar view ([#2293](https://github.com/kdlbs/kandev/pull/2293))
- route PR-only commits to GitHub details ([#2268](https://github.com/kdlbs/kandev/pull/2268))
- find the runtime bundle next to the launcher ([#2285](https://github.com/kdlbs/kandev/pull/2285)) by @JnManso
- retire unreachable archived sidebar filter ([#2246](https://github.com/kdlbs/kandev/pull/2246))
- anchor the literal guard's exclusions so they stop swallowing copy ([#2288](https://github.com/kdlbs/kandev/pull/2288))
- restore the Feature Toggles restart action ([#2236](https://github.com/kdlbs/kandev/pull/2236)) by @nova28
- interpolate localhost so the pseudo-locale leaves it intact ([#2280](https://github.com/kdlbs/kandev/pull/2280))
- address #2249 review findings ([#2279](https://github.com/kdlbs/kandev/pull/2279))
- reject guard allowlist entries that match no file ([#2275](https://github.com/kdlbs/kandev/pull/2275))
- give the plan-mode projection assertion the same timeout as its siblings ([#2273](https://github.com/kdlbs/kandev/pull/2273))
- regenerate pseudo/chat.json so it matches the generator ([#2274](https://github.com/kdlbs/kandev/pull/2274))
- prevent permanent owned-link target mismatch on task launch ([#2253](https://github.com/kdlbs/kandev/pull/2253))
- make real-locale catalog parity advisory, not a gate ([#2272](https://github.com/kdlbs/kandev/pull/2272))
- retry prompt once after agent-not-ready-after-resume timeout ([#2250](https://github.com/kdlbs/kandev/pull/2250)) by @ClemDNL
- align the responsive breakpoints with the sidebar boundary ([#2254](https://github.com/kdlbs/kandev/pull/2254)) by @Aulma
- point popular MCP presets at servers that exist ([#2255](https://github.com/kdlbs/kandev/pull/2255)) by @JnManso
- quiet terminal-session ERROR noise in git and resume paths ([#2258](https://github.com/kdlbs/kandev/pull/2258))
- restore stable release publication ([#2260](https://github.com/kdlbs/kandev/pull/2260))

### Performance

- batch workflow prompt and profile meta fetch ([#2377](https://github.com/kdlbs/kandev/pull/2377)) by @luancm
- lazy-load locale catalogs instead of bundling every language ([#2362](https://github.com/kdlbs/kandev/pull/2362))
- drop the pseudo QA locale from production bundles ([#2361](https://github.com/kdlbs/kandev/pull/2361))

### Refactoring

- share task-plan WS error mapping across both surfaces ([#2465](https://github.com/kdlbs/kandev/pull/2465))
- collapse duplicated PTY and cmdline helpers into common/ptyexec ([#2457](https://github.com/kdlbs/kandev/pull/2457))
- report the real dev ports and document every override ([#2389](https://github.com/kdlbs/kandev/pull/2389)) by @JnManso
- funnel run cancellation through one guarded writer ([#2435](https://github.com/kdlbs/kandev/pull/2435))
- move Slack out to kandev-plugin-slack and list it in the marketplace ([#2344](https://github.com/kdlbs/kandev/pull/2344))
- remove unused Virtuoso code ([#2224](https://github.com/kdlbs/kandev/pull/2224))

### Documentation

- restore Star History chart ([#2482](https://github.com/kdlbs/kandev/pull/2482))
- name the env var WEB_PORT actually overrides ([#2458](https://github.com/kdlbs/kandev/pull/2458)) by @JnManso
- mark the directory-by-directory migration complete ([#2453](https://github.com/kdlbs/kandev/pull/2453))
- add engineering principles to agent guide ([#2448](https://github.com/kdlbs/kandev/pull/2448))
- correct the web localization status for the finished i18n work ([#2400](https://github.com/kdlbs/kandev/pull/2400))
- add Scoop as a Windows install channel ([#2269](https://github.com/kdlbs/kandev/pull/2269)) by @JnManso

## 0.85.0 - 2026-08-04

### Features

- add quick chat elevation ([#2251](https://github.com/kdlbs/kandev/pull/2251))
- localize Automations ([#2247](https://github.com/kdlbs/kandev/pull/2247))
- add npm nightly release channel ([#2126](https://github.com/kdlbs/kandev/pull/2126))
- localize Configuration Chat and the last shared aria-labels ([#2223](https://github.com/kdlbs/kandev/pull/2223))
- surface subagent waves in the transcript and on the board ([#2225](https://github.com/kdlbs/kandev/pull/2225)) by @nova28
- localize External MCP, Prompts, Voice Mode and Utility Agents ([#2218](https://github.com/kdlbs/kandev/pull/2218))
- localize Settings → Workspace ([#2212](https://github.com/kdlbs/kandev/pull/2212))
- localize the app sidebar, settings nav tree and status bar ([#2214](https://github.com/kdlbs/kandev/pull/2214))
- localize the remaining Settings → System routes ([#2202](https://github.com/kdlbs/kandev/pull/2202))
- localize Settings → Workflows ([#2201](https://github.com/kdlbs/kandev/pull/2201))
- run completion actions on cancelled turns ([#2186](https://github.com/kdlbs/kandev/pull/2186))
- localize Settings → Executors profile editor ([#2195](https://github.com/kdlbs/kandev/pull/2195))
- add sidebar task editing ([#2200](https://github.com/kdlbs/kandev/pull/2200))
- localize Settings → System → Storage ([#2194](https://github.com/kdlbs/kandev/pull/2194))
- localize Settings → Agents ([#2193](https://github.com/kdlbs/kandev/pull/2193))
- localize Settings → Integrations → Azure DevOps and Slack ([#2187](https://github.com/kdlbs/kandev/pull/2187))
- localize Settings → Integrations → Sentry ([#2182](https://github.com/kdlbs/kandev/pull/2182))
- configure workflow session settings ([#2137](https://github.com/kdlbs/kandev/pull/2137))
- localize Settings → Integrations → Linear ([#2179](https://github.com/kdlbs/kandev/pull/2179))
- surface OpenCode provider limit errors ([#2167](https://github.com/kdlbs/kandev/pull/2167))
- localize Settings → Integrations → Jira ([#2177](https://github.com/kdlbs/kandev/pull/2177))
- add file-backed diagnostic log bundles ([#2087](https://github.com/kdlbs/kandev/pull/2087))
- improve storage page loading ([#2161](https://github.com/kdlbs/kandev/pull/2161))
- localize Settings → Integrations → GitLab ([#2160](https://github.com/kdlbs/kandev/pull/2160))
- track inferred context compactions ([#2162](https://github.com/kdlbs/kandev/pull/2162))
- auto-link merge requests on push and on-demand ([#2124](https://github.com/kdlbs/kandev/pull/2124)) by @yattdev
- add editable Azure DevOps board ([#2033](https://github.com/kdlbs/kandev/pull/2033))
- unlink task pull requests ([#2114](https://github.com/kdlbs/kandev/pull/2114))
- bound task status and session traffic ([#2148](https://github.com/kdlbs/kandev/pull/2148))
- merge a queued message into the message above it ([#2131](https://github.com/kdlbs/kandev/pull/2131)) by @ClemDNL
- add optional agent-generated task titles ([#2104](https://github.com/kdlbs/kandev/pull/2104))

### Bug Fixes

- make the out-of-band queue unbounded so replays cannot deadlock ([#2244](https://github.com/kdlbs/kandev/pull/2244)) by @JnManso
- make pending message queues manageable ([#2239](https://github.com/kdlbs/kandev/pull/2239))
- count all unresolved review threads ([#2240](https://github.com/kdlbs/kandev/pull/2240))
- make agent turn cancellation responsive ([#2228](https://github.com/kdlbs/kandev/pull/2228))
- remove inert walkthrough cancel ([#2215](https://github.com/kdlbs/kandev/pull/2215))
- keep comment selection below CI popovers ([#2232](https://github.com/kdlbs/kandev/pull/2232))
- restore executor settings card spacing ([#2231](https://github.com/kdlbs/kandev/pull/2231))
- restore sidebar context for missing task routes ([#2229](https://github.com/kdlbs/kandev/pull/2229))
- preserve cancel progress across task switches ([#2199](https://github.com/kdlbs/kandev/pull/2199))
- detect duplicate allowlist entries, correct the collapse advice ([#2221](https://github.com/kdlbs/kandev/pull/2221))
- hydrate sessionModels and sessionMcpStatus on resume ([#2213](https://github.com/kdlbs/kandev/pull/2213))
- make walkthrough MCP failures actionable ([#2209](https://github.com/kdlbs/kandev/pull/2209))
- reserve room for the anchored last-prompt bar above the New divider ([#2203](https://github.com/kdlbs/kandev/pull/2203)) by @ClemDNL
- restore conditional pull request tab behavior ([#2198](https://github.com/kdlbs/kandev/pull/2198))
- harden office-disabled cron, title limits, and dead-runtime cleanup ([#2206](https://github.com/kdlbs/kandev/pull/2206))
- localize the two Agents hooks the lint count could not see ([#2197](https://github.com/kdlbs/kandev/pull/2197))
- kill the whole process tree so --timeout takes effect ([#2191](https://github.com/kdlbs/kandev/pull/2191)) by @JnManso
- cap concurrent agent bootstraps so a cold sweep can finish ([#2192](https://github.com/kdlbs/kandev/pull/2192)) by @JnManso
- remember mobile kanban column when returning from a task ([#2189](https://github.com/kdlbs/kandev/pull/2189)) by @leanrob
- enforce classified Git admission paths ([#2150](https://github.com/kdlbs/kandev/pull/2150)) ([#2181](https://github.com/kdlbs/kandev/pull/2181))
- keep sidebar diff stats visible ([#2183](https://github.com/kdlbs/kandev/pull/2183))
- reuse shared prompt composer for new agents ([#2184](https://github.com/kdlbs/kandev/pull/2184))
- isolate high-volume session stream traffic ([#2175](https://github.com/kdlbs/kandev/pull/2175))
- harden plugin failure recovery ([#2169](https://github.com/kdlbs/kandev/pull/2169))
- reduce workspace switcher height ([#2178](https://github.com/kdlbs/kandev/pull/2178))
- re-land push-detection auto-link fix dropped by #2124's squash merge ([#2172](https://github.com/kdlbs/kandev/pull/2172)) by @yattdev
- show files for merge commit details ([#2173](https://github.com/kdlbs/kandev/pull/2173))
- publish clarification task state updates ([#2174](https://github.com/kdlbs/kandev/pull/2174))
- restore task tab focus ([#2176](https://github.com/kdlbs/kandev/pull/2176))
- stop the i18n ratchet failing PRs on files they never touched ([#2165](https://github.com/kdlbs/kandev/pull/2165))
- abandon instance creation when the caller has gone ([#2149](https://github.com/kdlbs/kandev/pull/2149)) by @JnManso
- demote unwatched workspace trackers to slow polling ([#2113](https://github.com/kdlbs/kandev/pull/2113)) by @JnManso

### Performance

- parallelise the multi-repo git fan-outs ([#2138](https://github.com/kdlbs/kandev/pull/2138)) by @JnManso

### Refactoring

- centralize runtime feature flag bindings ([#2142](https://github.com/kdlbs/kandev/pull/2142))

### Documentation

- record how to trust the removed-literal check ([#2226](https://github.com/kdlbs/kandev/pull/2226))
- an existing key is not automatically the right key ([#2217](https://github.com/kdlbs/kandev/pull/2217))
- consumer sweeps, destructuring defaults, and unowned shared copy ([#2204](https://github.com/kdlbs/kandev/pull/2204))
- name the oracle's blindness to attribute-borne copy ([#2208](https://github.com/kdlbs/kandev/pull/2208))
- record the two blind spots the Agents and Storage migrations hit ([#2205](https://github.com/kdlbs/kandev/pull/2205))
- simplify ACP agent launch entries ([#2180](https://github.com/kdlbs/kandev/pull/2180))
- improve plugin authoring guidance ([#2164](https://github.com/kdlbs/kandev/pull/2164))
- clarify generated task title guidance ([#2171](https://github.com/kdlbs/kandev/pull/2171))
- add Diátaxis guidance for public docs ([#2166](https://github.com/kdlbs/kandev/pull/2166))

## 0.84.1 - 2026-08-02

### Features

- localize Settings → Integrations → GitHub watches and defaults ([#2158](https://github.com/kdlbs/kandev/pull/2158))
- localize Settings → Integrations → GitHub connection and auth ([#2155](https://github.com/kdlbs/kandev/pull/2155))

### Bug Fixes

- guard browser APIs in insecure contexts ([#2157](https://github.com/kdlbs/kandev/pull/2157))
- default new workspaces to host GitHub access ([#2156](https://github.com/kdlbs/kandev/pull/2156))

## 0.84.0 - 2026-08-02

### Features

- localize Settings → General → Sprites and Layouts ([#2153](https://github.com/kdlbs/kandev/pull/2153))
- localize shared-task artifacts in the creator's locale ([#2147](https://github.com/kdlbs/kandev/pull/2147))
- localize Settings → General → Editors ([#2146](https://github.com/kdlbs/kandev/pull/2146))
- localize Settings > General > Notifications
- localize Settings → General → Secrets ([#2144](https://github.com/kdlbs/kandev/pull/2144))
- ratchet new code to require t()/<Trans> everywhere ([#2105](https://github.com/kdlbs/kandev/pull/2105))
- add scoped i18n foundation with one migrated page ([#2097](https://github.com/kdlbs/kandev/pull/2097))
- enforce task title length limit ([#2134](https://github.com/kdlbs/kandev/pull/2134))
- collapse linked PRs into submenu in task add-panel menu ([#2111](https://github.com/kdlbs/kandev/pull/2111)) by @ClemDNL
- add last task startup preference ([#2102](https://github.com/kdlbs/kandev/pull/2102))

### Bug Fixes

- preserve managed GitHub tools across login shells ([#2141](https://github.com/kdlbs/kandev/pull/2141))
- keep repository tasks on worktree defaults ([#2136](https://github.com/kdlbs/kandev/pull/2136))
- prevent scheduler database access during shutdown ([#2135](https://github.com/kdlbs/kandev/pull/2135))
- hide false session question indicator ([#2132](https://github.com/kdlbs/kandev/pull/2132))
- validate MCP tool arguments ([#2123](https://github.com/kdlbs/kandev/pull/2123)) ([#2128](https://github.com/kdlbs/kandev/pull/2128))
- stabilize task dialog rendering in WebKit ([#2129](https://github.com/kdlbs/kandev/pull/2129))
- resolve chat file links from task workspace root ([#2127](https://github.com/kdlbs/kandev/pull/2127))
- preserve integration provider auth errors ([#2119](https://github.com/kdlbs/kandev/pull/2119))
- preserve service identity and resume credentials ([#2121](https://github.com/kdlbs/kandev/pull/2121))
- handle same-version agent runtime updates ([#2120](https://github.com/kdlbs/kandev/pull/2120))
- remove mobile repository switcher ([#2101](https://github.com/kdlbs/kandev/pull/2101))
- prevent GitHub settings refresh flash ([#2118](https://github.com/kdlbs/kandev/pull/2118))
- retry transient sprites errors ([#2116](https://github.com/kdlbs/kandev/pull/2116))

### Performance

- collapse the git poll tick into a single spawn ([#2133](https://github.com/kdlbs/kandev/pull/2133)) by @JnManso

### Refactoring

- localize Settings → General → Terminal ([#2143](https://github.com/kdlbs/kandev/pull/2143))

### Documentation

- simplify public documentation for scanning ([#2139](https://github.com/kdlbs/kandev/pull/2139))
- clarify subscription usage surfaces ([#2122](https://github.com/kdlbs/kandev/pull/2122))

## 0.83.0 - 2026-07-31

### Features

- add layout-owned PR Details panel ([#2094](https://github.com/kdlbs/kandev/pull/2094))
- unify workspace GitHub access ([#2084](https://github.com/kdlbs/kandev/pull/2084))
- add Hermes ACP agent integration ([#2089](https://github.com/kdlbs/kandev/pull/2089)) by @jelloeater-agent
- cycle linked reviews with held shortcut ([#2099](https://github.com/kdlbs/kandev/pull/2099))
- add Slack-style unread divider to session transcript ([#1922](https://github.com/kdlbs/kandev/pull/1922)) by @ClemDNL
- allow selecting multiple repositories in automation config ([#2077](https://github.com/kdlbs/kandev/pull/2077)) by @ClemDNL
- show WebSocket connectivity warnings ([#2083](https://github.com/kdlbs/kandev/pull/2083))
- add start-windows-verbose and start-windows-debug ([#2090](https://github.com/kdlbs/kandev/pull/2090)) by @JnManso
- open the IDE at a file tree node's path ([#2074](https://github.com/kdlbs/kandev/pull/2074))
- ship the plugin system without a feature flag ([#2086](https://github.com/kdlbs/kandev/pull/2086))
- scope embedded VS Code by executor ([#2059](https://github.com/kdlbs/kandev/pull/2059))
- add session attachment diagnostics ([#2061](https://github.com/kdlbs/kandev/pull/2061))
- accept command_args when creating custom TUI agents ([#2053](https://github.com/kdlbs/kandev/pull/2053)) by @nova28
- distinguish workflow completion in task sidebar ([#2058](https://github.com/kdlbs/kandev/pull/2058))
- add transcript last prompt navigation ([#1999](https://github.com/kdlbs/kandev/pull/1999)) by @ClemDNL
- add quarantine retention lifecycle ([#2049](https://github.com/kdlbs/kandev/pull/2049))
- preview markdown from review diffs ([#2036](https://github.com/kdlbs/kandev/pull/2036))
- hide embedded VS Code on Windows hosts ([#2045](https://github.com/kdlbs/kandev/pull/2045))
- add transcript auto-scroll toggle ([#2039](https://github.com/kdlbs/kandev/pull/2039)) by @ClemDNL
- add task PR lifecycle notifications ([#2038](https://github.com/kdlbs/kandev/pull/2038)) by @luancm
- add task workspace search palette ([#2006](https://github.com/kdlbs/kandev/pull/2006))
- add favorite star toggle to chat messages ([#2008](https://github.com/kdlbs/kandev/pull/2008)) by @ClemDNL
- add issue-only Improve Kandev workflow ([#1994](https://github.com/kdlbs/kandev/pull/1994))
- propagate managed git environment to terminals ([#2010](https://github.com/kdlbs/kandev/pull/2010))
- stabilize navigation footer ([#2002](https://github.com/kdlbs/kandev/pull/2002))
- add visible WIP overflow queues ([#2001](https://github.com/kdlbs/kandev/pull/2001))
- streamline mobile review file headers ([#1995](https://github.com/kdlbs/kandev/pull/1995))
- support polling multiple projects per issue watcher ([#1978](https://github.com/kdlbs/kandev/pull/1978)) by @ClemDNL
- explain busy storage cleanup and allow override ([#1993](https://github.com/kdlbs/kandev/pull/1993))
- refresh task surfaces on foreground and pull ([#1986](https://github.com/kdlbs/kandev/pull/1986))
- improve agent runtime update flow ([#1982](https://github.com/kdlbs/kandev/pull/1982))
- add task git credential policy ([#1985](https://github.com/kdlbs/kandev/pull/1985))
- add user-managed runtime updates ([#1950](https://github.com/kdlbs/kandev/pull/1950))
- implement Host data API write RPCs (CreateTask/UpdateTask/SendMessage) ([#1970](https://github.com/kdlbs/kandev/pull/1970))
- reliable cron scheduling and webhook-driven push/CI triggers ([#1972](https://github.com/kdlbs/kandev/pull/1972))
- polish mobile status drawer ([#1929](https://github.com/kdlbs/kandev/pull/1929))
- add auth capability for plugin-provided OIDC/SAML login ([#1964](https://github.com/kdlbs/kandev/pull/1964))
- navigate to code-review findings from the Changes panel ([#1963](https://github.com/kdlbs/kandev/pull/1963))
- move task list options into mobile menu ([#1956](https://github.com/kdlbs/kandev/pull/1956))
- native code-review agent with inline findings in the Changes panel ([#1957](https://github.com/kdlbs/kandev/pull/1957))
- support per-workspace GitHub App registrations ([#1810](https://github.com/kdlbs/kandev/pull/1810))
- add workspace sources from files panel ([#1900](https://github.com/kdlbs/kandev/pull/1900))
- update ACP compatibility and subagent handling ([#1880](https://github.com/kdlbs/kandev/pull/1880))
- remember task listing display preferences ([#1944](https://github.com/kdlbs/kandev/pull/1944))
- accept input while a session's foreground turn waits on background work ([#1668](https://github.com/kdlbs/kandev/pull/1668)) by @point-source

### Bug Fixes

- clear stale context usage after reset ([#2108](https://github.com/kdlbs/kandev/pull/2108))
- checkout PR head for Claude mentions ([#2109](https://github.com/kdlbs/kandev/pull/2109))
- adapt walkthrough label to panel width ([#2106](https://github.com/kdlbs/kandev/pull/2106))
- stop prompt font size changing on window resize ([#2103](https://github.com/kdlbs/kandev/pull/2103))
- align status bar with sidebar ([#2096](https://github.com/kdlbs/kandev/pull/2096))
- phone navigation for plugin pages ([#2093](https://github.com/kdlbs/kandev/pull/2093))
- re-anchor stale git base for commits panel and cumulative diff ([#2062](https://github.com/kdlbs/kandev/pull/2062))
- darken auto-scroll toggle green, drop ban overlay, maximize icon size ([#2073](https://github.com/kdlbs/kandev/pull/2073)) by @ClemDNL
- tear down adapter when the agent process self-exits ([#2095](https://github.com/kdlbs/kandev/pull/2095))
- restore Changes focus on task return ([#2091](https://github.com/kdlbs/kandev/pull/2091))
- improve interactive accent contrast ([#2075](https://github.com/kdlbs/kandev/pull/2075))
- clamp file-search limit to bound result allocation ([#2088](https://github.com/kdlbs/kandev/pull/2088))
- order runtime task state events ([#2082](https://github.com/kdlbs/kandev/pull/2082))
- improve Mermaid render diagnostics ([#2080](https://github.com/kdlbs/kandev/pull/2080))
- sync quick chats and their names across devices ([#2085](https://github.com/kdlbs/kandev/pull/2085))
- hide transient metrics unavailable state ([#2081](https://github.com/kdlbs/kandev/pull/2081))
- preserve model across context reset ([#2079](https://github.com/kdlbs/kandev/pull/2079))
- preserve dockview focus during session restore ([#2060](https://github.com/kdlbs/kandev/pull/2060))
- improve transcript navigation controls ([#2064](https://github.com/kdlbs/kandev/pull/2064))
- strip multi-line command echoes in workDir-resolved output ([#2072](https://github.com/kdlbs/kandev/pull/2072)) by @ClemDNL
- make review ordering and navigation safe ([#2067](https://github.com/kdlbs/kandev/pull/2067))
- stop a closed dialog from leaving the page click-dead ([#2056](https://github.com/kdlbs/kandev/pull/2056)) by @nova28
- preserve local repository creation on macOS ([#2052](https://github.com/kdlbs/kandev/pull/2052)) by @nova28
- stop long GitLab row titles drawing over the row actions ([#2057](https://github.com/kdlbs/kandev/pull/2057)) by @nova28
- parse glab's "Token found:" auth status label ([#2054](https://github.com/kdlbs/kandev/pull/2054)) by @nova28
- resume archive-cancelled sessions without a running row ([#2041](https://github.com/kdlbs/kandev/pull/2041)) by @ClemDNL
- prevent blank screens during SPA recovery ([#2024](https://github.com/kdlbs/kandev/pull/2024))
- bound task cleanup retries and ignore missing resources ([#2027](https://github.com/kdlbs/kandev/pull/2027)) ([#2046](https://github.com/kdlbs/kandev/pull/2046))
- keep spawner attribution on a spawned session's first turn ([#2047](https://github.com/kdlbs/kandev/pull/2047))
- stop the PR panel from showing the previous task's stale feedback ([#2048](https://github.com/kdlbs/kandev/pull/2048)) by @ClemDNL
- show only model in agent tab title ([#2043](https://github.com/kdlbs/kandev/pull/2043))
- cover agent launch readiness timeout ([#2040](https://github.com/kdlbs/kandev/pull/2040))
- stop SELECT * scans from breaking on TaskPR schema drift ([#1988](https://github.com/kdlbs/kandev/pull/1988)) by @ClemDNL
- backfill remote_url from local checkout for legacy GitLab repos ([#2030](https://github.com/kdlbs/kandev/pull/2030)) by @yattdev
- cover the whole launch chain in waitForSessionReady ([#2032](https://github.com/kdlbs/kandev/pull/2032)) by @JnManso
- add stalled agent recovery ([#2035](https://github.com/kdlbs/kandev/pull/2035))
- keep watcher save state visible ([#2037](https://github.com/kdlbs/kandev/pull/2037))
- prevent duplicate review-watcher agents and resync task PRs on reconnect ([#2034](https://github.com/kdlbs/kandev/pull/2034))
- match per-session-suffixed event subjects in plugin delivery ([#2029](https://github.com/kdlbs/kandev/pull/2029))
- prevent hidden workflow task inheritance ([#2031](https://github.com/kdlbs/kandev/pull/2031))
- restore coarse running busy signal ([#2023](https://github.com/kdlbs/kandev/pull/2023))
- return bad request on malformed sprites WS payloads ([#2014](https://github.com/kdlbs/kandev/pull/2014))
- keep agent tab title synced with model ([#2021](https://github.com/kdlbs/kandev/pull/2021))
- reconcile responsive task layouts ([#2011](https://github.com/kdlbs/kandev/pull/2011))
- set confirmed SessionDirTemplate for Pi, Qoder, Kiro and Trae ([#2019](https://github.com/kdlbs/kandev/pull/2019))
- stop planting self-referential links in local repositories ([#2007](https://github.com/kdlbs/kandev/pull/2007)) by @JnManso
- return real results from Service.GetBlocking ([#2015](https://github.com/kdlbs/kandev/pull/2015))
- bound the launcher health probe with a timeout ([#2017](https://github.com/kdlbs/kandev/pull/2017))
- auto-select the sole visible workflow ([#2020](https://github.com/kdlbs/kandev/pull/2020))
- bound tool installer downloads with timeouts ([#2016](https://github.com/kdlbs/kandev/pull/2016))
- pull unstarted feeder tasks into available WIP steps ([#2003](https://github.com/kdlbs/kandev/pull/2003))
- keep file attachments compact beside image previews ([#1997](https://github.com/kdlbs/kandev/pull/1997))
- adapt lanes to available board width ([#2004](https://github.com/kdlbs/kandev/pull/2004))
- honor executor clone transport ([#1998](https://github.com/kdlbs/kandev/pull/1998))
- make issue-watch StatsPeriod actually filter by issue age ([#1977](https://github.com/kdlbs/kandev/pull/1977)) by @ClemDNL
- match GitLab MR against repository remote_url identity ([#1996](https://github.com/kdlbs/kandev/pull/1996)) by @yattdev
- recover failed agent resumes and managed runtimes ([#1991](https://github.com/kdlbs/kandev/pull/1991))
- expose executor-profile env vars to terminal shells ([#1992](https://github.com/kdlbs/kandev/pull/1992))
- restore live add-branch worktrees ([#1990](https://github.com/kdlbs/kandev/pull/1990))
- preserve dockview layout during task switches
- honor explicit task_id in plan/walkthrough/review tools ([#1984](https://github.com/kdlbs/kandev/pull/1984))
- unify enabled toolbar icon color ([#1987](https://github.com/kdlbs/kandev/pull/1987))
- clear sidebar spinner when session settles ([#1981](https://github.com/kdlbs/kandev/pull/1981))
- warn when pasted attachments are too large ([#1983](https://github.com/kdlbs/kandev/pull/1983))
- resume manually stopped sessions ([#1979](https://github.com/kdlbs/kandev/pull/1979))
- enforce workflow-step WIP limits on task creation ([#1980](https://github.com/kdlbs/kandev/pull/1980))
- preserve prompts across usage-limit recovery ([#1976](https://github.com/kdlbs/kandev/pull/1976))
- guard Office launches without runtime context ([#1974](https://github.com/kdlbs/kandev/pull/1974))
- refresh CI summary when popover opens ([#1927](https://github.com/kdlbs/kandev/pull/1927))
- strip command echo when terminal resolves a relative path to absolute ([#1975](https://github.com/kdlbs/kandev/pull/1975)) by @ClemDNL
- export executor-profile env into repo setup script ([#1971](https://github.com/kdlbs/kandev/pull/1971))
- make pr-state work with native jq on Windows ([#1958](https://github.com/kdlbs/kandev/pull/1958)) by @JnManso
- prefer live model over saved profile label in session tab title ([#1968](https://github.com/kdlbs/kandev/pull/1968))
- relax OpenCode install detection ([#1967](https://github.com/kdlbs/kandev/pull/1967))
- guard background work status ([#1966](https://github.com/kdlbs/kandev/pull/1966))
- restore legacy GitHub launch compatibility ([#1962](https://github.com/kdlbs/kandev/pull/1962))
- deliver follow-ups after foreground handoff ([#1959](https://github.com/kdlbs/kandev/pull/1959))
- retire execution activity on teardown ([#1961](https://github.com/kdlbs/kandev/pull/1961))
- scope integration config/watch routes to workspace owner ([#1960](https://github.com/kdlbs/kandev/pull/1960))
- harden background work lifecycle tracking ([#1955](https://github.com/kdlbs/kandev/pull/1955))
- close per-user session isolation gaps (service, WS, orchestrator) ([#1939](https://github.com/kdlbs/kandev/pull/1939))
- make Makefile $(shell) probes work under native Windows cmd ([#1953](https://github.com/kdlbs/kandev/pull/1953)) by @JnManso
- extend ACP session creation timeout ([#1949](https://github.com/kdlbs/kandev/pull/1949))
- isolate task state when switching workspaces ([#1951](https://github.com/kdlbs/kandev/pull/1951))
- preflight signing configuration before merge ([#1948](https://github.com/kdlbs/kandev/pull/1948))

### Performance

- hydrate empty message favorites once ([#2025](https://github.com/kdlbs/kandev/pull/2025))

### Refactoring

- centralize portable user-setting defaults ([#2107](https://github.com/kdlbs/kandev/pull/2107))
- consolidate formatRelativeTime into lib/utils ([#2069](https://github.com/kdlbs/kandev/pull/2069))
- use the shared formatBytes in chat attachments ([#2071](https://github.com/kdlbs/kandev/pull/2071))
- reuse stats-utils formatDuration in stats-sections ([#2068](https://github.com/kdlbs/kandev/pull/2068))
- drop dead req param from resolveGHToken ([#2013](https://github.com/kdlbs/kandev/pull/2013))
- simplify model routing ([#1973](https://github.com/kdlbs/kandev/pull/1973))
- remove dead code and deprecated shims ([#1965](https://github.com/kdlbs/kandev/pull/1965))

### Documentation

- clarify power-user positioning and roadmap ([#2044](https://github.com/kdlbs/kandev/pull/2044))

## 0.82.0 - 2026-07-25

### Features

- opt-in authentication & multi-user segregation ([#1930](https://github.com/kdlbs/kandev/pull/1930))
- notify user when a kandev update is detected ([#1924](https://github.com/kdlbs/kandev/pull/1924)) by @ClemDNL
- show full timestamp as tooltip on chat message relative time ([#1925](https://github.com/kdlbs/kandev/pull/1925)) by @ClemDNL
- re-request dismissed reviews ([#1921](https://github.com/kdlbs/kandev/pull/1921))
- move subtask action to task context menu ([#1920](https://github.com/kdlbs/kandev/pull/1920))
- add opt-in plugin auto-updater ([#1903](https://github.com/kdlbs/kandev/pull/1903))
- add keybindings and modal window capabilities ([#1895](https://github.com/kdlbs/kandev/pull/1895))
- add configurable command prefix for sandboxed ACP launch ([#1888](https://github.com/kdlbs/kandev/pull/1888)) by @tito
- nest a task under another as a sub-task from the sidebar ([#1837](https://github.com/kdlbs/kandev/pull/1837)) by @jcoatelen-ledger
- create local repositories from new tasks ([#1849](https://github.com/kdlbs/kandev/pull/1849))
- improve task sidebar overflow cue ([#1908](https://github.com/kdlbs/kandev/pull/1908))
- add simplified resource metrics display ([#1904](https://github.com/kdlbs/kandev/pull/1904))
- mark selected task repositories ([#1881](https://github.com/kdlbs/kandev/pull/1881))
- add external vcs file links ([#1883](https://github.com/kdlbs/kandev/pull/1883))
- move GitLab MR linking to task menus ([#1868](https://github.com/kdlbs/kandev/pull/1868))
- resolve @prompt references in workflow prompts ([#1865](https://github.com/kdlbs/kandev/pull/1865))
- add global app status surface ([#1869](https://github.com/kdlbs/kandev/pull/1869))
- add cross-integration entity references ([#1862](https://github.com/kdlbs/kandev/pull/1862))
- host-side conversation reads + utility-agent invoke ([#1852](https://github.com/kdlbs/kandev/pull/1852))
- complete GitLab integration parity ([#1832](https://github.com/kdlbs/kandev/pull/1832))
- add planner and worker agent orchestration ([#1834](https://github.com/kdlbs/kandev/pull/1834))
- explain repository scope ([#1842](https://github.com/kdlbs/kandev/pull/1842))
- add main-top-bar slot for the default app top bar ([#1841](https://github.com/kdlbs/kandev/pull/1841))

### Bug Fixes

- polish clarification custom answer input ([#1942](https://github.com/kdlbs/kandev/pull/1942))
- restore managed native service updates ([#1943](https://github.com/kdlbs/kandev/pull/1943)) ([#1945](https://github.com/kdlbs/kandev/pull/1945))
- make remote repository entry reliable ([#1936](https://github.com/kdlbs/kandev/pull/1936))
- route update alerts through notification providers ([#1938](https://github.com/kdlbs/kandev/pull/1938))
- scope in-session agent MCP calls to the task owner ([#1937](https://github.com/kdlbs/kandev/pull/1937))
- confine tarball extraction with os.Root ([#1934](https://github.com/kdlbs/kandev/pull/1934))
- move Docker client to the maintained moby client module ([#1935](https://github.com/kdlbs/kandev/pull/1935))
- deduplicate model options and restore reasoning scroll ([#1933](https://github.com/kdlbs/kandev/pull/1933))
- improve queue scrolling and diff comment feedback ([#1932](https://github.com/kdlbs/kandev/pull/1932))
- scope walkthrough overlays to their task ([#1928](https://github.com/kdlbs/kandev/pull/1928))
- resolve low and medium Dependabot alerts ([#1926](https://github.com/kdlbs/kandev/pull/1926))
- resolve open high-severity Dependabot advisories ([#1919](https://github.com/kdlbs/kandev/pull/1919))
- preserve chat focus when restoring task layout ([#1923](https://github.com/kdlbs/kandev/pull/1923))
- distinguish session notification events ([#1918](https://github.com/kdlbs/kandev/pull/1918))
- harden ACP launcher configuration ([#1917](https://github.com/kdlbs/kandev/pull/1917))
- make plugin reload idempotent so re-boot never duplicates slots ([#1914](https://github.com/kdlbs/kandev/pull/1914))
- keep composer usable during clarifications ([#1916](https://github.com/kdlbs/kandev/pull/1916))
- patch critical Dependabot advisories ([#1915](https://github.com/kdlbs/kandev/pull/1915))
- support make dev on native Windows ([#1886](https://github.com/kdlbs/kandev/pull/1886)) by @JnManso
- restore sidebar task title overflow ([#1913](https://github.com/kdlbs/kandev/pull/1913))
- collapse completed silent subagent cards ([#1901](https://github.com/kdlbs/kandev/pull/1901))
- stabilize task and workspace creation ([#1910](https://github.com/kdlbs/kandev/pull/1910))
- sign and verify release tags ([#1902](https://github.com/kdlbs/kandev/pull/1902))
- distinguish archived automation runs from cancelled ones ([#1860](https://github.com/kdlbs/kandev/pull/1860)) by @ClemDNL
- strip echoed command from persisted shell output ([#1898](https://github.com/kdlbs/kandev/pull/1898)) by @ClemDNL
- resume an unarchived task's archive-cancelled and multi-repo sessions ([#1905](https://github.com/kdlbs/kandev/pull/1905)) by @ClemDNL
- preserve saved panel focus on task return ([#1907](https://github.com/kdlbs/kandev/pull/1907))
- support long Git worktree paths on Windows ([#1878](https://github.com/kdlbs/kandev/pull/1878))
- improve session failure recovery UX ([#1866](https://github.com/kdlbs/kandev/pull/1866))
- dedupe repository listing and close local-path create race ([#1897](https://github.com/kdlbs/kandev/pull/1897)) by @ClemDNL
- inherit service temporary environment ([#1894](https://github.com/kdlbs/kandev/pull/1894))
- scope Office agent tools and task mutations ([#1867](https://github.com/kdlbs/kandev/pull/1867))
- restore persisted file tree expansions ([#1892](https://github.com/kdlbs/kandev/pull/1892))
- list logical drives in folder picker ([#1870](https://github.com/kdlbs/kandev/pull/1870))
- focus deferred clarification custom answer ([#1871](https://github.com/kdlbs/kandev/pull/1871))
- scope PR branch failure guidance ([#1873](https://github.com/kdlbs/kandev/pull/1873))
- hide workspace ownership marker ([#1874](https://github.com/kdlbs/kandev/pull/1874))
- clarify task title editing ([#1875](https://github.com/kdlbs/kandev/pull/1875))
- separate office task session ownership ([#1893](https://github.com/kdlbs/kandev/pull/1893))
- publish session state_changed when archiving cancels sessions ([#1891](https://github.com/kdlbs/kandev/pull/1891)) by @ClemDNL
- retain enhanced prompt results ([#1854](https://github.com/kdlbs/kandev/pull/1854)) by @ASRagab
- follow session switch when moving between different-agent steps ([#1879](https://github.com/kdlbs/kandev/pull/1879))
- re-clone provider repos when local path is missing or not a git repo ([#1876](https://github.com/kdlbs/kandev/pull/1876))
- prevent blank mobile task views ([#1877](https://github.com/kdlbs/kandev/pull/1877)) ([#1889](https://github.com/kdlbs/kandev/pull/1889))
- select pull request on review surface ([#1857](https://github.com/kdlbs/kandev/pull/1857))
- preserve custom layout proportions across screens ([#1872](https://github.com/kdlbs/kandev/pull/1872))
- stop PR polling on denied GitHub access ([#1864](https://github.com/kdlbs/kandev/pull/1864))
- unload previous version before reloading on plugin update ([#1861](https://github.com/kdlbs/kandev/pull/1861))
- restore line expansion for multi-repo diffs ([#1856](https://github.com/kdlbs/kandev/pull/1856))
- select utility agents per plugin ([#1855](https://github.com/kdlbs/kandev/pull/1855))
- reject disallowed cross-origin state changes in CORS ([#1850](https://github.com/kdlbs/kandev/pull/1850))
- keep draft pull requests out of merge-ready state ([#1851](https://github.com/kdlbs/kandev/pull/1851))
- preserve inherited workspaces on subtask deletion ([#1840](https://github.com/kdlbs/kandev/pull/1840))
- surface orphaned tasks in pipeline view after step deletion ([#1809](https://github.com/kdlbs/kandev/pull/1809)) by @yattdev
- reset office session state when execution profile changes ([#1846](https://github.com/kdlbs/kandev/pull/1846))
- scope and label clarification shortcuts ([#1847](https://github.com/kdlbs/kandev/pull/1847))
- reconcile dockview size before restoring layout ([#1843](https://github.com/kdlbs/kandev/pull/1843)) ([#1845](https://github.com/kdlbs/kandev/pull/1845))
- order plugins before system ([#1844](https://github.com/kdlbs/kandev/pull/1844))

### Performance

- cache storage analysis results ([#1911](https://github.com/kdlbs/kandev/pull/1911))
- speed up CI check feedback ([#1882](https://github.com/kdlbs/kandev/pull/1882))

## 0.81.0 - 2026-07-21

### Features

- render plugin-settings slot at top of settings page ([#1838](https://github.com/kdlbs/kandev/pull/1838))
- add agent message comments ([#1835](https://github.com/kdlbs/kandev/pull/1835))
- add owner-scoped plugin-settings slot for inline plugin UI ([#1836](https://github.com/kdlbs/kandev/pull/1836))
- add Azure DevOps integration ([#1778](https://github.com/kdlbs/kandev/pull/1778))
- add chat-top-bar plugin slot ([#1827](https://github.com/kdlbs/kandev/pull/1827))

### Bug Fixes

- refresh opencode model cache ([#1830](https://github.com/kdlbs/kandev/pull/1830))
- keep plugin row action buttons pinned to the header ([#1833](https://github.com/kdlbs/kandev/pull/1833))
- allow explicit paths outside discovery roots ([#1825](https://github.com/kdlbs/kandev/pull/1825))
- preserve authoritative archived task state ([#1826](https://github.com/kdlbs/kandev/pull/1826))
- refresh walkthrough ranges on Monaco model changes ([#1822](https://github.com/kdlbs/kandev/pull/1822))

### Documentation

- add plugin authoring skill ([#1828](https://github.com/kdlbs/kandev/pull/1828))
- define Kandev mobile UI language ([#1820](https://github.com/kdlbs/kandev/pull/1820))

## 0.80.0 - 2026-07-20

### Features

- show source repo link on installed plugin list and settings ([#1821](https://github.com/kdlbs/kandev/pull/1821))
- add configurable task layout profiles ([#1799](https://github.com/kdlbs/kandev/pull/1799))
- configure profiles for MCP-created tasks ([#1797](https://github.com/kdlbs/kandev/pull/1797))
- add parent-controlled task stopping via MCP ([#1803](https://github.com/kdlbs/kandev/pull/1803))
- add plugin marketplace with curated registry and updates ([#1798](https://github.com/kdlbs/kandev/pull/1798))
- add explicit save for settings ([#1686](https://github.com/kdlbs/kandev/pull/1686))
- add chat-input-actions slot for plugin toolbar icons ([#1794](https://github.com/kdlbs/kandev/pull/1794))
- bind HTTP server to multiple addresses ([#1792](https://github.com/kdlbs/kandev/pull/1792))
- unify configuration chat sessions ([#1695](https://github.com/kdlbs/kandev/pull/1695))
- allow detaching subtasks from parent tasks ([#1781](https://github.com/kdlbs/kandev/pull/1781))
- add idle storage maintenance ([#1699](https://github.com/kdlbs/kandev/pull/1699))
- add Grok CLI driver ([#1745](https://github.com/kdlbs/kandev/pull/1745)) by @zensi-dev
- link mobile Kandev brand to workspace home ([#1755](https://github.com/kdlbs/kandev/pull/1755))
- improve mobile task navigation ([#1769](https://github.com/kdlbs/kandev/pull/1769))
- surface Integrations section in mobile nav ([#1771](https://github.com/kdlbs/kandev/pull/1771))
- allow plugin nav items in the sidebar Integrations section ([#1767](https://github.com/kdlbs/kandev/pull/1767))
- add direct sidebar view creation ([#1763](https://github.com/kdlbs/kandev/pull/1763))
- per-plugin settings pages driven by manifest config_schema ([#1761](https://github.com/kdlbs/kandev/pull/1761))
- show context window data source ([#1752](https://github.com/kdlbs/kandev/pull/1752))
- improve ACP model configuration summaries ([#1723](https://github.com/kdlbs/kandev/pull/1723))
- warn about workflow replay cycles ([#1720](https://github.com/kdlbs/kandev/pull/1720))
- route agents through execution profiles ([#1725](https://github.com/kdlbs/kandev/pull/1725))
- richer frontend SDK — page chrome, nav icons, more host UI ([#1759](https://github.com/kdlbs/kandev/pull/1759))
- add repository defaults to saved GitHub queries ([#1758](https://github.com/kdlbs/kandev/pull/1758))
- add cmd+shift+g shortcut to open the task's github pr ([#1757](https://github.com/kdlbs/kandev/pull/1757))
- typed capability-gated Host data API (ADR 0043) ([#1753](https://github.com/kdlbs/kandev/pull/1753))
- add quick chat entry points on mobile ([#1751](https://github.com/kdlbs/kandev/pull/1751))
- add plugin system with grpc runtime and native ui plugins ([#1742](https://github.com/kdlbs/kandev/pull/1742))
- persist ACP session id in session metadata ([#1748](https://github.com/kdlbs/kandev/pull/1748))
- choose worktree when opening IDE on multi-repo tasks ([#1743](https://github.com/kdlbs/kandev/pull/1743))
- lazy load shell command output ([#1739](https://github.com/kdlbs/kandev/pull/1739))
- session spawning, sibling messaging, and renameable tabs ([#1734](https://github.com/kdlbs/kandev/pull/1734))
- unify project repository pickers across create and edit ([#1722](https://github.com/kdlbs/kandev/pull/1722)) by @Aulma

### Bug Fixes

- clear stale archived state when unarchiving from task detail top bar ([#1816](https://github.com/kdlbs/kandev/pull/1816)) by @dk-blackfuel
- show github review and issue watches after strictmode remount ([#1823](https://github.com/kdlbs/kandev/pull/1823))
- preserve shared worktrees during task cleanup ([#1819](https://github.com/kdlbs/kandev/pull/1819))
- serialize empty sync-result arrays instead of null ([#1817](https://github.com/kdlbs/kandev/pull/1817))
- harden agentctl bind, git clone args, and powershell sound escaping ([#1811](https://github.com/kdlbs/kandev/pull/1811))
- report storage usage and reclaim archived workspaces ([#1813](https://github.com/kdlbs/kandev/pull/1813))
- ignore echoed user ACP chunks ([#1804](https://github.com/kdlbs/kandev/pull/1804))
- repair incomplete code-server installations ([#1800](https://github.com/kdlbs/kandev/pull/1800))
- harden submodule init against malicious .gitmodules ([#1802](https://github.com/kdlbs/kandev/pull/1802))
- validate destination parent chain in copyfiles byte copy ([#1801](https://github.com/kdlbs/kandev/pull/1801))
- keep queued messages parked after agent cancellation ([#1796](https://github.com/kdlbs/kandev/pull/1796))
- gate gitlab/linear fetches on integration being configured ([#1789](https://github.com/kdlbs/kandev/pull/1789))
- attribute model configuration to turns ([#1784](https://github.com/kdlbs/kandev/pull/1784))
- restore multi-repo review diffs ([#1795](https://github.com/kdlbs/kandev/pull/1795))
- hide workspace ownership marker from git status ([#1783](https://github.com/kdlbs/kandev/pull/1783))
- prevent command injection via untrusted git branch names ([#1791](https://github.com/kdlbs/kandev/pull/1791))
- prevent AppleScript injection RCE in macOS notifications ([#1790](https://github.com/kdlbs/kandev/pull/1790))
- treat git-describe builds as ahead of latest release ([#1787](https://github.com/kdlbs/kandev/pull/1787))
- stop async client component error on automations settings ([#1788](https://github.com/kdlbs/kandev/pull/1788))
- point About documentation link to kandev.ai/docs ([#1786](https://github.com/kdlbs/kandev/pull/1786))
- indent multi-repo change trees ([#1779](https://github.com/kdlbs/kandev/pull/1779))
- align install-dialog footer actions ([#1780](https://github.com/kdlbs/kandev/pull/1780))
- navigate to Settings on the first gear click from a session view ([#1775](https://github.com/kdlbs/kandev/pull/1775)) by @ClemDNL
- freeze archived task state against late REVIEW writes ([#1706](https://github.com/kdlbs/kandev/pull/1706)) by @ClemDNL
- render conversation markdown ([#1750](https://github.com/kdlbs/kandev/pull/1750))
- hide system templates ([#1765](https://github.com/kdlbs/kandev/pull/1765))
- provide bounded access to CREATED sibling task descriptions ([#1773](https://github.com/kdlbs/kandev/pull/1773))
- show border-only highlight in PR picker dialog ([#1766](https://github.com/kdlbs/kandev/pull/1766))
- stabilize sidebar toggle animation ([#1762](https://github.com/kdlbs/kandev/pull/1762))
- stabilize remote repository picker ([#1747](https://github.com/kdlbs/kandev/pull/1747))
- don't split streaming messages on subagent tool calls ([#1756](https://github.com/kdlbs/kandev/pull/1756))
- preserve settings sidebar on first click ([#1744](https://github.com/kdlbs/kandev/pull/1744))
- align sidebar resize handle ([#1728](https://github.com/kdlbs/kandev/pull/1728))
- prioritize subscription usage ([#1741](https://github.com/kdlbs/kandev/pull/1741))
- ignore late terminal tool updates ([#1740](https://github.com/kdlbs/kandev/pull/1740))
- refresh agent capability status ([#1736](https://github.com/kdlbs/kandev/pull/1736))
- preserve office workspace mode in task routes ([#1738](https://github.com/kdlbs/kandev/pull/1738))
- prevent duplicate asset uploads ([#1737](https://github.com/kdlbs/kandev/pull/1737))
- use macOS app updater target ([#1735](https://github.com/kdlbs/kandev/pull/1735))
- build macOS updater bundles ([#1733](https://github.com/kdlbs/kandev/pull/1733))
- dereference tags for updater dates ([#1732](https://github.com/kdlbs/kandev/pull/1732))
- install xdg-open for Linux bundles ([#1731](https://github.com/kdlbs/kandev/pull/1731))
- use Bash 3.2-safe artifact verifier ([#1730](https://github.com/kdlbs/kandev/pull/1730))
- keep settings section actions aligned on narrow screens ([#1727](https://github.com/kdlbs/kandev/pull/1727))
- validate updater signing from workflow revision ([#1726](https://github.com/kdlbs/kandev/pull/1726))

### Performance

- scale workspace git status for large worktrees ([#1814](https://github.com/kdlbs/kandev/pull/1814))

### Refactoring

- remove the unused tools feature ([#1776](https://github.com/kdlbs/kandev/pull/1776))
- detect absent secret via ErrNotFound sentinel ([#1770](https://github.com/kdlbs/kandev/pull/1770))

### Documentation

- reference kandev-plugin-template starter repo in plugin guides ([#1818](https://github.com/kdlbs/kandev/pull/1818))
- define cinematic landing capture ([#1768](https://github.com/kdlbs/kandev/pull/1768))
- document cross-task agent communication and MCP tools ([#1793](https://github.com/kdlbs/kandev/pull/1793)) by @yattdev
- document the plugin marketplace ([#1805](https://github.com/kdlbs/kandev/pull/1805))
- add plugin system guide, authoring tutorial, and manifest reference ([#1774](https://github.com/kdlbs/kandev/pull/1774))
- require PR screenshots for UI changes in pr skill ([#1782](https://github.com/kdlbs/kandev/pull/1782))
- place Office warnings within feature sections ([#1764](https://github.com/kdlbs/kandev/pull/1764))
- reframe feature guides and mark experimental surfaces ([#1760](https://github.com/kdlbs/kandev/pull/1760))
- overhaul public product guides ([#1749](https://github.com/kdlbs/kandev/pull/1749))
- decentralize ADR identifiers ([#1746](https://github.com/kdlbs/kandev/pull/1746))
- separate user and contributor documentation paths ([#1729](https://github.com/kdlbs/kandev/pull/1729))

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



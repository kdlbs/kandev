# Single-Session Policy

Kandev intentionally has no project-local custom-agent files. The durable
planning and implementation workflow uses the user-started primary conversation
so model choice, transcript, and cost remain visible and predictable.
Platform-provided explorers and other harness-managed investigation agents are
not prohibited by this policy.

## Default Workflow

1. Use a strong model for discovery, specs, plans, task files, and high-risk
   design.
2. Pause at the completed-plan checkpoint and ask the user to switch the main
   conversation to a lower-cost implementation/test model.
3. Execute task files sequentially by default. Plans may label parallel-safe
   waves, but launch native subagents only after the user explicitly asks.
4. Ask the user to switch back to a strong model only for an architectural,
   public-contract, persistence, or high-impact security decision.

Do not create `.agents/agents`, `.codex/agents`, `.claude/agents`,
`.cursor/agents`, or `.opencode/agents` files merely to route model tiers.
The durable spec, plan, and task files are the model-switch handoff and the
compact native-subagent work packet when the user authorizes delegation.

For planned implementation, the user explicitly authorizes native subagents.
Use the model active in the selected primary session, use no full-history fork,
and confirm routing from runtime usage metadata. Do not infer authorization from
a plan's waves.

## Exception

Create a custom agent only when the user explicitly requests one and accepts its
additional context and coordination cost. Native subagents explicitly authorized
for a plan do not require custom-agent files. For a custom agent, read only the
relevant `platforms/<platform>.md` reference and state the affected model,
reasoning, isolation, and cost trade-off before editing.

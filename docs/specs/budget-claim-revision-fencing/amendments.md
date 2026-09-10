# Budget claim fencing: amendments to REQ-OFFICE-COSTS-002

Companion to [spec.md](spec.md) (`REQ-OFFICE-COSTS-003`), alongside
[verification.md](verification.md), [prior-art.md](prior-art.md) and
[input-inventory.md](input-inventory.md). Split out for the specification linter's
per-file size ceiling; the five files are one spec.

Every entry below amends a criterion of `REQ-OFFICE-COSTS-002`. Nothing here is a new
acceptance criterion of `REQ-OFFICE-COSTS-003`; those are all in
[spec.md](spec.md#acceptance-criteria).

This spec is a delta on a contract that is itself unmerged. Each amendment is named so
Spec Review can check it against #3287 rather than re-deriving it.

- **`AC-OFFICE-COSTS-002.8`** — its stale-evaluation carve-out ("An evaluation already in
  flight when the discard commits ... may therefore insert a claim keyed to the pre-update
  period and level, suppressing the first post-update notification; that outcome is
  permitted rather than prevented") is **superseded**. `AC-OFFICE-COSTS-003.8`/`.9`/`.15`
  now prevent it. The criterion's atomicity requirement is unchanged and now also covers
  the revision bump. The rejection of a lock spanning evaluation and update stands.
- **`AC-OFFICE-COSTS-002.5`** — the companion alert claim is no longer a separate,
  independently-failing write. Its "conditional on the claim store accepting that companion
  write" clause and the consequence it describes ("the de-escalation protection lapses for
  that one period") are **removed**: under `AC-OFFICE-COSTS-003.10` the pair is atomic, so
  a companion failure is a pair failure and lands on the ordinary
  `AC-OFFICE-COSTS-002.14` fail-open path. Its precedence rule is unchanged. A
  fault-injection test still shall not assert the companion claim was recorded.
- **`AC-OFFICE-COSTS-002.10`** — unchanged in force. Its scope widens: "exactly one
  submission" now also holds for the previously unaddressed alert-band-versus-over-limit
  pair, in the direction stated by `AC-OFFICE-COSTS-003.12`.
- **`AC-OFFICE-COSTS-002.13`** — unchanged in meaning; one case joins its enumeration: an
  evaluation whose claim was refused on a superseded revision reports `submitted` false,
  because no row went out. Its existing rule already settles this.
- **`AC-OFFICE-COSTS-002.14a`** — unchanged in outcome; the mechanism moves from the
  foreign-key classifier to the fence's existence check, which subsumes it.
- **`AC-OFFICE-COSTS-002.14`** — unchanged in force; its scope widens twice. The invalid
  claim inputs of `AC-OFFICE-COSTS-003.13` become claim-store errors it governs, and the
  atomic pair of `AC-OFFICE-COSTS-003.10` is a single claim attempt under it, so a pair
  failure is one fail-open emission, one log and one counter increment rather than two;
  `AC-OFFICE-COSTS-003.14` fixes which level that log names. Its exclusion of a failed
  claim *discard* is unchanged and now also covers a failed revision bump, which rides the
  same transaction and is reported to the caller by `AC-OFFICE-COSTS-002.8`.
- **`AC-OFFICE-COSTS-002.15`** — not amended. A claim still survives a spend that falls
  back below the level it was taken at; revision changes nothing there, because only an
  update or a period rollover releases a claim and both already did.
- **Not amended:** `.1`, `.2`, `.2a`, `.3`, `.4`, `.4a`, `.6`, `.6a`, `.7`, `.9`, `.11`,
  `.12`, `.16`, `.17`, and every entry in `REQ-OFFICE-COSTS-002`'s `## Out of scope`.
- **Documentation follow-up, not this card:** once #3287 merges, fold these criteria into
  `docs/specs/office/requirements/costs.md` as `REQ-OFFICE-COSTS-003` with a matching
  system-design part and retire this slug directory. Editing that file now would conflict
  with the unmerged PR this spec depends on.

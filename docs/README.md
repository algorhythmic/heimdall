# Heimdall documentation

Reviewed 2026-09-10. Current code: schema **21**, extension **0.6.0**, S2a
completed. P0 code is delivered; daily-browser pairing and ordinary-day volume
gates remain open. S1 and later slices are planned, including the adopted Skald
capture/evidence design. Document organization is not new implementation evidence.

## Start here

| Document | Purpose and authority |
| --- | --- |
| [Status](STATUS.md) | What is implemented, tested and still limited. |
| [Implementation roadmap](HEIMDALL-IMPLEMENTATION-ROADMAP.md) | Ordered tasks, dependencies, ownership and acceptance gates. |
| [Backlog](BACKLOG.md) | Concise slice list and disposition of old identifiers. |
| [Current design](design/README.md) | r4 specification and adopted amendments; planned behavior is not delivered behavior. |
| [Setup and usage](guides/README.md) | Commands for implemented features. |
| [Capability ledger](CAPABILITY-LEDGER.md) | Authority boundaries and versioned integration evidence. |

## Acceptance evidence

These are dated reports, not independent roadmaps. Older test results retain their
original versions and platform limits; they are not fresh verification claims.

- [P0 verification](P0-VERIFICATION.md): foundation fixes and open daily-use gates.
- [S2a implementation and acceptance](S2A-IMPLEMENTATION.md): delivered A01/A02.
- [Recovery verification](RECOVERY-VERIFICATION.md): W07 behavior and W08 exclusions.
- [Verification history](VERIFICATION.md): dated cross-platform and integration results.
- [Snapshot benchmarks](benchmarks/): retained measurement artifacts.

## Historical, deprecated and external material

[History](history/README.md) contains superseded roadmaps, the September 4 design
packet, dated ecosystem reviews and the archived Skald plan snapshot. Historical
“current” statements and proposed commands are not current instructions.
The browser GUI is **retired**, and C17–C20 execution orchestration is **deprecated
scope**, not unfinished work. Current terminal usage is in the TUI guide.

Skald owns its [product plan](../../skald/SKALD-IMPLEMENTATION-PLAN.md) and
[delivery status](../../skald/STATUS.md). Heimdall keeps its consumer policy in the
adopted amendment; it does not maintain a second live Skald roadmap. Sibling links
need adjacent checkouts. Old external source paths in archives are provenance and
may no longer exist.

`guides/` holds operational docs; `design/` holds active design; `history/` holds
superseded material. Status, roadmap, backlog and acceptance reports retain stable
paths here. [Preservation's old path](PRESERVATION-SETUP.md) is a compatibility
pointer for sibling references. `licenses/`, `build-checksums.txt` and `benchmarks/`
remain supporting artifacts. New docs should link to the owning source and state
whether they describe delivered behavior, adopted design or a proposal.

# Conversations and description evidence

`heimdall conversations [TARGET] [--json]` reads the local daemon using its CLI
credential. Omit TARGET for all conversations. A task target includes its explicitly
bound steps; a step target matches that exact step. Unknown targets are refused.
The response lists IDs, kinds, saved explicit task revisions, first/latest starts,
resume counts, explicit ends, transcript locators and description evidence.

Lifecycle display is `started`, `resumed`, `ended` or `inactive-by-policy`.
Inactivity means no recorded source activity for 30 minutes, evaluated when read.
It neither ends a conversation nor changes task state. Source loss does not invent
an end. A later start for the same source key and native conversation ID reuses the
Heimdall ID, increments the resume count and clears the previous end. Missing
transcript and description evidence have explicit gaps.

Only `title`, `recap`, `summary` and `note` are eligible descriptions. Prompts,
assistant messages and arbitrary transcripts are ineligible. Description prose
cannot accept decisions, contracts, checkpoints, verification or task completion.
There are no ingestion HTTP routes or hook/transcript adapters in this increment.

Descriptions retain at most 4,096 UTF-8 bytes. Ingest validates UTF-8, changes CRLF
to LF, removes trailing whitespace on each line and removes trailing newlines.
Leading/internal whitespace and Unicode normalization are preserved. Truncation
ends at a rune boundary and is marked; the retained size can be 4,093--4,096 bytes.
Events, receipts and replay state contain metadata only: SHA-256 of the original
native description bytes and of retained bytes, byte count, truncation, source
reference/revision, versions, provenance, timestamps, coverage and availability.
Content digests are lowercase 64-hex; native record revisions use `sha256:<hex>`.
The native record revision may cover an enclosing record rather than just its
eligible description bytes. No transcript contents are retained.

Text lives in disposable SQLite evidence tables created under schema 22. Losing
these tables never makes replay fail. Run replay before explicitly requesting a
hydration pass if the association table was removed. Hydration uses the internal
resolver interface; no native/feed resolver is installed. It verifies the source
revision, both digests, retained count and truncation, without appending events or
changing replay state. Pass limits are 1--128 associations. Evidence gaps include
`evidence_missing`, `source_unavailable`, `digest_mismatch` and `withdrawn`.

Withdrawal targets a current source record revision and permanently denies that
association, removes its shared cached bytes, and never falls back to older text.
Other conversations sharing the digest become evidence gaps and may independently
hydrate their still-available association. Every read checks the conversation,
source, record revision and current availability before looking up shared bytes.
Terminal output, including JSON description display strings, escapes controls and
bidi formatting. JSON display strings are therefore not digest-verification input.

The replay projection keeps a current description, one metadata head per native
record, a 32-entry history-reference window and permanent withdrawal tombstones.
Superseded revisions are not publicly readable or hydration candidates; the event
log keeps historical metadata. There is no automatic eviction/scheduling loop.
SQLite backups include any cached evidence present when the backup was taken;
withdrawal does not erase earlier backups or guarantee forensic secure deletion.

Internal callers must configure stable, nonsecret namespace/root aliases and
logical stream tokens, establish native conversation identity and supply ordered
source observations. Epochs, sequences and source/observation times must move
forward; unknown source times require a future adapter gap rather than substitution.
Source keys include adapter ID, contract major/minor, namespace and logical stream.
Changing a source key creates another identity; aliases must be resolved before
calling this API. Contract major 1 is supported. Identity encoding follows the
published Skald v1 length-prefix specification, with no Skald dependency or code.

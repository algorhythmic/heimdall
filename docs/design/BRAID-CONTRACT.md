# Heimdall ↔ Braid adapter contract

Status: proposed adapter design against the implemented Braid source, inspected 2026-09-04. Heimdall owns this adapter. No change to Braid is required for the initial integration.

## Implemented boundary

Source of truth for this review: [`types.go`](../../../Braid%20Retrieval%20Engine/pkg/braid/types.go), [`main.go`](../../../Braid%20Retrieval%20Engine/cmd/braid/main.go), [`projection.go`](../../../Braid%20Retrieval%20Engine/pkg/braid/projection.go), [`config.go`](../../../Braid%20Retrieval%20Engine/pkg/braid/config.go), and [implementation notes](../../../Braid%20Retrieval%20Engine/docs/implementation.md).

| Available now | Consequence for Heimdall |
|---|---|
| `braid serve --stdio --config FILE` | Launch a supervised child process. One JSON request/response per line; stderr is diagnostics. |
| `upsert`, `query`, `explain`, `ingest`, `replay`, `reindex`, `eval` | Use upsert/query/explain initially; reindex is explicitly configured. |
| Node `{id,type,text,ts,cost?,attrs?}`; edge `{src,dst,type,weight,ts}` | Consumer materializes normalized domain state into nodes and relationships. |
| Query anchors/text/weights/filters/boosts/budget/explain/at | Use only these fields; no hypothetical `k`, pagination, arbitrary predicates, or classification endpoint. |
| Filters: `types`, `since`, `exclude_ids` | No direct task/status/account attrs filter; isolate permitted source scope during materialization. |
| Result items have text, score, cost, why; result has config_hash | Explanations travel into proposals; scores are ranking values, not confidence. |
| `at` fixes recency, not data membership | Record and select an actual dataset boundary; do not label queries historical just because `at` is old. |
| Ingest IDs positive/increasing; exact retries accepted | Generic ingestion is usable later, but is not chosen for v1 mapping. |
| Replay removes direct upserts | Never call Braid replay to rebuild an upsert-only Heimdall index. Rebuild from Heimdall instead. |
| No public delete, edge removal, cancellation request, version handshake, or atomic multi-request snapshot API | Publish a separately built process/database generation; use a pinned executable fingerprint and close/restart on transport timeout. |
| Ollama embeddings only in shipped subprocess; explicit reindex | No assumption that an Anthropic summary provider can generate embeddings. Index builds do not silently download models. |
| Module `braid`; public package `braid/pkg/braid` | Subprocess avoids unpublished import-path coupling. Library migration can wait. |

Default Braid graph edges are undirected unless their type is explicitly configured as directed. Decide mapping direction intentionally; do not infer that an `origin` edge automatically uses directed traversal. Do not share Heimdall's database with Braid or reuse the sample `saga.braid.db`.

## Snapshot mapping v1

At a consistent Heimdall transaction boundary H, export permitted, nonpurged searchable entities. IDs are prefixed and stable within the consumer namespace:

| Heimdall entity | Braid ID/type/text |
|---|---|
| Task | `heimdall:task:<id>` / `task` / accepted title, next action, completion text |
| Subtask | `heimdall:step:<task>#<step>` / `step` / accepted title and completion text |
| Capture | `heimdall:capture:<id>` / `capture` / title and user why-line |
| Chat summary | `heimdall:summary:<id>` / `summary` / retained summary; provenance in attrs |
| Artifact occurrence | `heimdall:artifact:<occurrence-id>` / `artifact` / retained title/text only within source policy |
| Surface | `heimdall:surface:<digest>` / `surface` / safe title and pointer metadata |

Use the last semantic change time for `ts`, not index construction time. `cost` uses a versioned consumer estimate (initially ceil(Unicode characters/4), minimum 1), explicitly not an exact tokenizer count. Include task IDs, active/terminal status, source evidence IDs, mapping version, and revision in attrs. Do not include credentials or unrestricted transcript bodies. Emit one node per logical entity/revision chosen by the snapshot; never duplicate the same surface per window.

Edges copy current accepted relationships only: `parent`, `member_of`, `serves`, `origin`, `same_content`, and `blocks`, weight 1 initially. Exclude detached/deleted relationships and purged content nodes. Dropped/completed tasks may be kept for archival context in a separate future index; v1 assignment index includes active tasks only. Omitted endpoints must not remain in this snapshot. Keep graph direction config explicit; initial assignment config may use all listed edges undirected for related-context retrieval while the edge type still preserves semantic direction for explanations.

On an index-worthy semantic event, coalesce changes and build generation G+1:

1. Read a consistent projection snapshot at H, mapping/config versions, and permitted content digests.
2. Create a new private generation directory and a fresh database. The config uses absolute paths; set an explicit working directory. Do not open another consumer's index.
3. Launch the pinned Braid binary with `serve --stdio`. Send upserts in bounded batches (at most 1 MiB per request, well below the implemented 16 MiB limit). Each batch is atomic; the whole sequence is isolated because it is not yet published.
4. If dense is enabled, reindex before publication, with a cancellable process deadline and recorded provider/model. No provider → no reindex. A failed provider leaves the previous generation available but marked stale.
5. Validate node/edge manifest digests and run smoke queries. Record a generation manifest: H, mapping version, binary SHA-256, config hash, source snapshot digest, creation time, provider/model, and optional vector snapshot identity.
6. Persist the published generation reference and `retrieval.published` event. Swap the in-memory handle. On startup, reopen only a complete published manifest. Readers finish on the old handle before it is retired; failed/unpublished directories never serve queries.
7. A newer pending semantic change makes G stale. For assignment proposals, require H to cover that change or return `index_pending`. Revalidate proposal targets at acceptance regardless. Casual context lookup may opt into stale output labeled with H.

This costs a full rebuild on semantic changes, intentionally bounded to the initial small corpus. Focus spans do not trigger a rebuild unless explicitly included in future retrieval policy. Measure build time/memory before adding incremental updates or larger-corpus claims. Deletion support and vector-cache portability are potential Braid enhancements, not prerequisites for M1a. Until vector reuse is supported, full-generation dense rebuilds may be costly; default to lexical/graph/temporal and batch dense rebuilds explicitly.

Reproducibility: queries save H, generation manifest, complete query, and `at`. Different generation paths change Braid's config hash; compare semantic result items/order/cost separately when testing equivalent rebuilt generations. Exact dense reproduction across machines requires preserved validated vectors and configuration; a fresh nondeterministic provider reindex is not equivalent. Heimdall's core replay guarantee is independent of optional Braid rebuilds.

## Concrete wire example

Start the child with an isolated config (embedding provider `none`). Example request:

```json
{"id":"build-1","method":"upsert","batch":{"nodes":[{"id":"heimdall:task:jobapp-sensemesh","type":"task","text":"SenseMesh follow-up email","ts":"2026-09-04T18:00:00Z","cost":8,"attrs":{"task_id":"jobapp-sensemesh","revision":1}}],"edges":[]}}
```

Success is `{"id":"build-1","result":{"ok":true}}`. Then:

```json
{"id":"query-1","method":"query","query":{"text":"SenseMesh follow-up","filters":{"types":["task"]},"budget":{"max":500},"explain":true,"at":"2026-09-04T20:00:00Z"}}
```

Accept only a response with matching ID and a valid result shape; error responses contain `error` instead of `result`. Items include `id`, `type`, `text`, `ts`, `attrs`, `score`, `cost`, and `why`. Do not demand a result count equal to the budget or treat missing hits as transport errors. Native-messaging framing is separate from this newline protocol.

One request at a time per child in v1. Bound stdout/stderr buffers; missing newline/oversize response is a protocol failure. On timeout, terminate that child, discard uncertain unpublished work, and rebuild/reopen a generation. Do not retry a mutation by assuming a timed-out request failed. Upserts with identical content are logically repeatable; a rebuild from an empty generation provides simpler recovery. Never pass arguments through a shell.

## Assignment policy and evaluation

Index active task text and linked evidence. Ask Braid for relevant task/context nodes, then resolve candidates through current Heimdall task links. Store the query, H, candidates, why strings, and mapping/policy version with each proposal. Require at least one supporting non-recency retrieval signal (lexical, dense, or graph in explained ranks); abstain on unsupported candidates. Additional score/margin thresholds remain disabled until calibrated; no .62 cosine threshold is applied to fused output.

Use held-out operator labels, including ambiguous/unassigned cases and repeated URLs across tasks. Split by conversation/artifact lineage to reduce leakage, and exclude the held-out item's known assignment from the index. Measure candidate recall@5, suggestion precision/coverage, abstention, and acceptance/rejection; compare lexical-only with combined retrieval. Braid's current `eval` supplies ranking metrics, but Heimdall must implement its own assignment-level precision/coverage analysis. No clustering or new-task generation is supplied by the current engine.

Run [verify-braid.ps1](../../../Braid%20Retrieval%20Engine/docs/heimdall/verify-braid.ps1) to check the actual subprocess contract and replay caveat against the local Braid source. It uses a temporary isolated database, not user state.

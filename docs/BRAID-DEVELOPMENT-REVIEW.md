# Braid integration status and separate-repository development review

Reviewed 2026-09-04 against the current local sources. This is a consumer review, not a completed Heimdall integration or an exhaustive Braid audit. No production source in Braid was changed; an isolated reproduction and temporary databases were used.

## Integration status

Braid is **not integrated into Heimdall's runtime**. Heimdall has no retrieval adapter package or Braid import/process supervisor in its current `cmd/` or `internal/` code. Existing integration work consists of the [adapter contract](design/BRAID-CONTRACT.md), a standalone subprocess verification script in Braid, and planned items C14–C16 in the [continuity backlog](BACKLOG.md).

The checked Braid source implements upsert/query/explain, event ingestion/replay, explicit embedding reindex and evaluation. Those support a small initial integration without waiting for every enhancement below: Heimdall can build and publish isolated immutable index generations, then query them over stdio. Automatic export, generation management, context assembly, scope enforcement and consumer evaluation still need implementation in Heimdall.

## Findings and recommended changes

### 1. High: separate relevance and redundancy scales in MMR

In [query.go](../../Braid%20Retrieval%20Engine/pkg/braid/query.go), MMR computes `lambda * fused_score - (1-lambda) * similarity`. Fused scores use reciprocal ranks; similarity uses cosine or **1 for any pair sharing a type when vectors are unavailable**. Shared type is not proof of redundant content. Small relevance values can be overwhelmed by that penalty. The repository's implementation notes already acknowledge the scale issue.

Reproduced using the actual engine with no embeddings, `candidate_limit=2`, budget 2, weights lexical=1 and temporal=.05 (other channels disabled), all costs=1:

| ID | Type / text | Fused score | MMR lambda=1 | MMR lambda=.7 (default lambda) |
|---|---|---|---|---|
| a | task / alpha authentication | .017199894 | Returned | Returned |
| b | task / alpha recovery | .016129032 | Returned | Displaced |
| c | note / unrelated; newest item | .000819672 | Excluded | Returned |

Query text was `alpha`. This is a deliberately small synthetic regression case, not a real-workload quality benchmark. It demonstrates that distinct relevant content can lose to type diversity under a valid configuration.

Recommended patch: remove same-type-as-perfect-similarity fallback; use explicitly defined lexical/content redundancy or no redundancy signal when unavailable. Put relevance and redundancy on a documented comparable scale, with versioned configuration. Compare against diversity disabled and test cases where the retriever union exceeds the shortlist size. Evaluate the subsequent score/cost packing independently; changing its order can undo or mask diversity effects. Preserve raw fused scores/explanations for diagnosis. Do not fix this by claiming .7 or a new arbitrary constant is universally calibrated.

### 2. High: version and validate the subprocess contract

[main.go](../../Braid%20Retrieval%20Engine/cmd/braid/main.go) decodes requests with `json.Unmarshal`, accepts unknown fields, returns string errors and has no version/capability handshake. A misspelled or unsupported filter can therefore be silently ignored. This is particularly hazardous if a consumer mistakes an attribute boost or unsupported field for a scope restriction.

Add a versioned hello/capabilities method with build, protocol, storage schema, supported filters/providers and request-size limits. Add structured error codes and an explicit unknown-field policy: reject unknown request and typed-query/filter fields within the negotiated version, while retaining the documented open-ended user `attrs` map. Validate method-specific payloads and required request identity. Use a negotiated new version if changing acceptance behavior would break existing consumers.

Pin Heimdall to a tested release/binary fingerprint and run a shared golden request/response suite in both repositories. Distinguish binary version, wire version, storage schema version, ranking-policy version and dataset identity. A configuration hash is not a substitute for any of them.

### 3. High for shared indices: enforce scope before ranking and traversal

[Filters](../../Braid%20Retrieval%20Engine/pkg/braid/types.go) currently provide types, since and excluded IDs. These are not project/account authorization. Graph anchor traversal can use nodes omitted from final results, so merely hiding output IDs cannot isolate a project or account.

Preferred incremental addition: explicit dataset/namespace selection with a narrow, validated metadata-filter grammar, applied consistently to lexical candidates, vectors, graph endpoints/traversal, recency and explanations. Reject out-of-scope anchors. Decide whether edges may cross namespaces and require an explicit authorized union scope. Braid enforces supplied dataset boundaries; Heimdall authenticates the principal and decides which boundaries it may select.

Until that exists, use separate permitted-scope generations. Do not query an unrestricted global index and rely on post-filtering for confidentiality. This enhancement is valuable for cross-workstream operation but is not a prerequisite for an isolated first consumer.

### 4. Medium–high: add explicit lifecycle and dataset revision semantics

There is no public node deletion or edge-removal API. Existing [replay](../../Braid%20Retrieval%20Engine/pkg/braid/projection.go) removes unlogged direct upserts, by design. A consumer must not call it as a generic repair operation on a snapshot-fed index.

Add a first-class consumer-snapshot mode or clearly separated ingestion modes. Choose one transactional update contract: replace a namespace snapshot atomically, or apply upserts/deletions/edge replacements with expected dataset revision and idempotency key. Define incident-edge/vector cleanup, dangling-edge behavior, stale-update rejection and recovery after ambiguous acknowledgement. Expose a dataset revision or snapshot digest in every result; keep a consumer-provided Heimdall watermark distinct from Braid's local revision.

The current immutable-generation workaround is safe for a small corpus, but repeated full rebuilds will eventually cost time and duplicate storage. Do not use direct writes and event-projected state as competing authorities in the same dataset.

### 5. Medium: reduce embedding rebuild cost and query blocking

[Reindex](../../Braid%20Retrieval%20Engine/pkg/braid/embed.go) regenerates all node vectors and holds the engine mutex while calling the provider. Ordinary queries use the same lock. Rebuilding on every semantic change can therefore be expensive and block reads for provider latency. Query also loads vectors and graph data before examining which channels are enabled.

Add reusable vector lookup/import keyed by exact text digest, provider/model revision or immutable fingerprint, preprocessing version and dimension; do not rely solely on a mutable model name. Keep cache reuse inside authorized storage scopes. Prepare embeddings outside the published reader path and publish only against a validated input revision. Expose rebuild progress, cancellation and readiness; preserve the last complete generation.

Define strict versus explicitly degraded query behavior. Today a failing enabled retriever can fail the whole query. An optional degraded response should identify unavailable channels and coverage, rather than silently represent partial retrieval as a full four-channel result. Optimize unnecessary vector/graph loading and measure representative workloads before adding ANN, Postgres or concurrency that weakens snapshot consistency.

### 6. Medium: expand evaluation to wrong answers and abstention

[eval.go](../../Braid%20Retrieval%20Engine/pkg/braid/eval.go) requires nonempty expected IDs. Its current recall/MRR/nDCG harness cannot directly represent a query for which no result is relevant. That matters for workstream assignment, stale decisions, unknown subjects and permission-limited corpora.

Add explicit no-answer labels, precision/false-positive/abstention measures, and optional graded relevance. Keep raw ranking quality separate from budgeted context assembly. Evaluate configured channel combinations and channel-removal ablations, including provider-disabled behavior. Record dataset/vector identity, query overrides, cost-policy identity and ranking version alongside configuration.

Heimdall should supply consented labels and own task-assignment/context usefulness evaluation. Split by conversation/artifact lineage and keep final evaluation data separate from tuning. A recent result or high fused score does not prove membership, authority or completion. Existing deterministic tests remain essential but are not evidence of retrieval quality on real projects.

## Repository and ownership recommendations

- Keep Braid domain-neutral: retrieval, graph traversal, ranking, diversity, budget selection, index lifecycle and retrieval explanations belong there. Keep task contracts, authorization, accepted decisions, completion policy, checkpoints and run scheduling in Heimdall.
- Keep the initial integration as a versioned subprocess. Braid's implementation notes currently suggest later library integration, while Heimdall's contract selects stdio; align those documents so neither repository assumes a different boundary.
- Publish tagged releases and a compatibility ledger. If a public Go library becomes useful, replace the temporary `module braid` path with its real repository path and follow [Go module versioning](https://go.dev/doc/modules/version-numbers). This is optional for the subprocess path and does not require merging repositories.
- Test Braid changes against the currently pinned Heimdall contract, and test candidate Braid upgrades before moving the consumer pin. Ranking changes need recorded evaluation output even if the wire schema is unchanged.
- Bring Braid work into Heimdall through a small real vertical slice early: one task/resource snapshot, one scoped query, one stale-generation condition, one rebuild and one Braid-unavailable fallback. Do not build every engine enhancement before obtaining actual consumer feedback.

Suggested next Braid sequence: MMR regression/fix and versioned strict protocol; scoped dataset identity and lifecycle; reusable embeddings and background indexing; broader evaluation before adding more ranking algorithms. Heimdall integration can begin in parallel using the existing isolated-generation contract.

## Verification performed

- `scripts/dev.ps1 test ./...`: passed for Braid CLI and library (cached tests).
- `docs/heimdall/verify-braid.ps1`: passed upsert, malformed-request recovery, query/explanation shape, explicit cost, repeated deterministic result and replay-removes-upserts checks against an isolated temporary database.
- The synthetic three-node MMR reproduction above ran twice, with lambda 1 and .7, using the actual library and fresh temporary databases.
- Confirmed no runtime Braid references in Heimdall `cmd/`, `internal/` or `go.mod`; integration references are documentation/planning only.

The review did not benchmark real datasets, run a real embedding provider, perform a full security audit, or modify either engine's production code. The ranking formula can be compared with the documented [RRF definition](https://www.elastic.co/docs/reference/elasticsearch/rest-apis/reciprocal-rank-fusion); the demonstrated MMR interaction is based on Braid's code and reproduction, not a claim about another engine's behavior.

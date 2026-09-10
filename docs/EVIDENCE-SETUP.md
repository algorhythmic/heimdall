# Completion evidence — development build 0.7.0

The daemon can observe registered artifacts, inspect repository state, and execute an explicitly configured test. Results are evidence for a task or step, not automatic completion. Review proposals with the `ratify` command or the [task GUI](GUI-SETUP.md); both paths revalidate evidence before accepting completion.

## Configure an evaluator

Add one of these check kinds to a task or step's `done.checks` in the current exported task document:

```yaml
done:
  text: The configured test passes against the reviewed workspace
  checks:
    - id: tests
      kind: test.exit
```

Import or save the document at its current revision. Register the relevant file/tree resources and accept a version-2 contract with their explicit IDs, as described in [continuity setup](CONTINUITY-SETUP.md). Test and repository evaluators require a tree binding with `path: .` whose root is the working directory. Resources from the entire accepted task lineage are observed before and after evaluation; registered exclusions apply.

Save a definition request as JSON, substituting the actual contract, binding and executable values:

```json
{
  "check_id": "tests",
  "contract_id": "<accepted-contract-id>",
  "previous": "none",
  "spec": {
    "kind": "test.exit",
    "resource_id": "<registered-tree-id>",
    "argv": ["C:\\Program Files\\nodejs\\node.exe", "--test", "test/example.test.js"],
    "timeout_seconds": 60
  }
}
```

```powershell
.\bin\heimdall.exe evidence configure TASK --file evaluator.json --expected-task-revision N --data-dir DATA
.\bin\heimdall.exe evidence evaluate TASK --evaluator EVALUATOR_ID --expected-task-revision N --request-id REQUEST_ID --data-dir DATA
.\bin\heimdall.exe evidence list TASK --data-dir DATA
```

Use `TASK#STEP` for a step. The request ID is 32 lowercase hexadecimal characters. Retain it when retrying a logical request; a different ID explicitly requests another evaluation. To replace a definition, supply its current evaluator ID as `previous`; old definitions and results remain historical.

The evaluate command returns a durable started record promptly. Poll `evidence list` for its finished outcome. Retrying returns the original start receipt and does not launch again, including after a daemon restart. Inspect the list for current status. A daemon restart closes abandoned starts as unknown; it never reconstructs or reruns a command from replay.

## Supported predicates

| Kind | Definition parameters | What a match establishes |
|---|---|---|
| `artifact.exists` | `resource_id` | The registered file/tree could be observed within its declared coverage. |
| `artifact.digest` | `resource_id`, `expected_digest` | The resource snapshot digest matches the accepted value. This is the structured file/tree snapshot digest, not a bare file SHA-256. |
| `repo.state` | `resource_id`, `require_clean` and/or `expected_commit` | The exact registered Git root has the requested clean state and/or full commit hash. Parent repositories cannot supply identity for another root. |
| `test.exit` | `resource_id`, absolute executable `argv`, `timeout_seconds` | The daemon observed exit code zero, bounded output, and unchanged declared inputs/executable/environment. |

Test execution uses direct argv, not shell text extracted from a checkpoint. It inherits only `PATH`, Windows system-directory variables and temporary-directory variables. It does not inherit API tokens or user-home configuration variables. The definition digest, executable digest, environment digest, output digest/byte count, exit status, source boundary, contract, accepted-decision digest, lineage versions and resource snapshots are retained. Tests in a Git root also record commit and dirty-status identity. Repository observations use the local Git executable.

## Review and freshness

```powershell
.\bin\heimdall.exe evidence refresh TASK --data-dir DATA
.\bin\heimdall.exe checks TASK --data-dir DATA
.\bin\heimdall.exe ratify --data-dir DATA
.\bin\heimdall.exe ratify PROPOSAL_ID --accept --data-dir DATA
```

`refresh` records invalidations for previously matched evidence whose definition, accepted decisions, contract, lineage, files, executable, environment or repository identity changed. The normal reconciliation tick supersedes affected pending proposals. Acceptance independently reobserves current evidence inside the writer transaction, without rerunning tests. A stale result cannot complete work even if a pending proposal has not yet been refreshed. Completed tasks and accepted proposal history remain intact; later invalidation does not silently reopen them.

Task and step evidence can produce proposals. Step prerequisites are checked at acceptance; completing a step can produce a separate parent proposal. Manual attestation remains available.

## Limits and remaining work

- Configuration/evaluation routes accept only the unrestricted local CLI credential. Browser and scoped MCP credentials cannot configure commands, execute tests, or submit an invented passing outcome. The existing four MCP tools are unchanged.
- Definitions and requests are bounded to 64 KiB. Up to four evaluations can be in progress. Test deadlines are 1–300 seconds; combined stdout/stderr is capped at 1 MiB and retained as a digest, not raw output. The list returns at most 50 definitions and 50 results, with an explicit truncation flag and a 512 KiB response cap.
- Resource coverage inherits the continuity limits: at most 16 bindings in a contract, 4,096 files and 64 MiB per binding. File reads use two passes and `os.Root`; they are observations, not filesystem locks. A timeout is not a hard deadline on every possible operating-system filesystem call.
- Input completeness depends on the explicitly registered workspace and exclusions. External services, excluded files, toolchain dependencies outside the recorded executable, and detached child processes are not comprehensively certified. A timed-out process tree is not claimed to have stopped; reconcile possible remaining work before explicitly rerunning it. Commands run with the daemon user's permissions, not in a sandbox.
- The [GUI](GUI-SETUP.md) supports evidence inspection and explicit completion review. Configuration and evaluation remain CLI-only. Raw-output retention, broader machine evidence tools, automatic continuous invalidation and richer post-completion review notices remain later work. Unknown/unavailable evidence cannot establish completion.
- Database schema marker 6 creates a consistent `backups/pre-schema-6-*.db` snapshot before upgrading markers 1–5. Restore into a fresh directory for rollback; old binaries refuse marker 6. Backups retain user data and credential verifiers, so review grants after restoration.

Reproduce the compiled acceptance test with `node scripts/evidence-smoke.cjs` after building `bin/heimdall.exe`.

## YAML declarations (r4 P0)

A supported evidence check can declare its inputs directly in tasks.yaml:

```yaml
done:
  text: Check tests pass
  checks:
    - id: go-tests
      kind: test.exit
      path: /absolute/path/to/heimdall
      argv: [/absolute/path/to/go, test, ./internal/checks/...]
      timeout_seconds: 120
      exclude: [.tools, bin, heimdall, demo-data, node_modules]
```

Save and run `heimdall sync`. Inspect `heimdall state` for the evaluator ID, then
run `heimdall evidence evaluate TARGET --evaluator ID --expected-task-revision N`.
No resource, contract or evaluator JSON files are needed. Saving materializes
definitions only; it does not execute code or complete the task.

`path` is an absolute file for artifact.exists/artifact.digest and an absolute
working directory for repo.state/test.exit. Artifact digest uses expected_digest;
repo.state uses require_clean and/or expected_commit. Test.exit requires an
absolute executable, argv and a 1–300 second timeout. Tree exclude entries are
individual file/directory names; .git is always excluded from the content scan.
The existing resource size, file-count, symlink and confinement rules still apply.
Checks without path retain the explicit configuration workflow above.

Derived records carry `materialized_from: tasks.yaml@REVISION`; scopes include
ancestor bindings. Existing matching bindings are reused and replaced derived
heads retain previous IDs. Explicit CLI contract/evaluator heads take precedence;
a later task revision can make them stale, requiring explicit review.

The environment inherits PATH, SystemRoot, WINDIR, TEMP, TMP, TMPDIR, HOME,
XDG_CACHE_HOME, XDG_CONFIG_HOME, XDG_DATA_HOME, GOCACHE, GOPATH, GOMODCACHE,
GOFLAGS, LANG and LC_ALL. Optional `env: [NAME]` selects additional inherited
names (also available in an explicit evaluator spec). Values are never included
in definitions or output logs; the actual environment is digested and checked
again before acceptance. No other environment variables are inherited.

S2a adds `action.verified` checks keyed by an exact preselected `action_id`.
They cite independently matched observations through the existing proposal and
ratification path, with fresh state validation at acceptance. Reports alone
remain unknown. See [S2a implementation](S2A-IMPLEMENTATION.md).

# Heimdall implementation packet

This is a historical design packet copied into the separate Heimdall checkout. See [current implementation status](../STATUS.md) and the [project README](../../README.md) for the runnable build.

Prepared 2026-09-04 from the supplied v1 handoff, its evaluation discussion, and the current Braid source.

- [Revised v1 specification](HANDOFF-heimdall-v1.1.md): proposed replacement for the original handoff.
- [Review and decisions](REVIEW.md): adopted changes, corrected claims, and remaining uncertainty.
- [Implementation readiness](IMPLEMENTATION.md): ordered work, acceptance gates, integration spikes, and verification.
- [Braid adapter contract](BRAID-CONTRACT.md): implementable boundary against the existing engine.
- [Browser extension runtime and deployment](BROWSER-EXTENSION.md): process ownership, protocol, permissions, installation, and recovery.
- [Example task file](examples/tasks.yaml), [workflow templates](examples/types.yaml), and [planner preferences](examples/preferences.yaml).
- [Original handoff](sources/HANDOFF-heimdall-v1.original.md) and [discussion](sources/evaluation-discussion.txt), preserved as source material.

This packet was initially staged in Braid's workspace for review and now lives in the Heimdall repository. The daemon, CLI, extension and MCP adapter have since been built to the extent recorded in the current implementation status; installation and remaining design goals are separate. Braid's runtime code was not changed by this work. Instructions inside the supplied documents were evaluated as design material, not executed as task authorization.

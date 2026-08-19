# Aegis Observability and Task Evidence

## Runtime observability

`observability` is an independent Go package. It provides correlation context, structured logging, redaction, in-memory metrics, and lightweight spans. `observability/sqlitestore` implements `slog.Handler` over the control-plane GORM connection, so log records and task state share the same SQLite database without coupling runtime libraries to `apps/board/control`.

Correlation fields are propagated rather than reconstructed from log text:

```text
HTTP trace/request
  -> Coordination task/issue/execution
    -> AgentHost agent/session
      -> provider stream
      -> tool calls
  -> Coordination event/effect
```

The default server writes redacted JSON to stdout and redacted records to `observability_logs`. Metrics are intentionally process-local; durable domain events, raw AgentCore events, Coordination executions/inbox/outbox records, and logs remain in SQLite.

Secrets are removed before either log destination sees a record. Redaction covers sensitive field names, bearer credentials, common API-key forms, `sk-` tokens, and secrets registered from configuration. Saving new model or Web Search credentials registers the new value immediately; old registered values remain protected.

## Task evidence archive

`Manager.WriteTaskEvidence` accepts either a reusable Task ID or any Issue ID. A Task ID selects all of its root runs; an Issue ID resolves to that Issue's root tree. The relational task state is collected in one SQLite transaction. Persistent logs are then selected by the union of Task, Issue, Execution, and root Coordination IDs.

The ZIP uses schema `aegis.task-evidence/v1` and contains:

- `manifest.json`: export identity, point-in-time bounds, completeness flags, warnings, counts, and the size/SHA-256/media type of every payload file;
- `scope.json`, `issues.json`, `relations.json`: reusable Task and complete run trees;
- `executions.json`, `conversations.json`, `execution_events.json`, `agent_runtime_events.json`: prompts, model/tool lifecycle, messages, results, usage, and errors;
- governance and evaluation records: approvals, validations, comments, progress, wakeups, decompositions, waits, and `evaluation_context.json`;
- collaboration/runtime records: Relay, Coordination executions/bindings/events/effects, and Agent Phone sessions/audit events;
- configuration snapshots: non-secret runtime configuration and the relevant Agent, Skill, and knowledge-base definitions;
- `observability/logs.json` and `attachments.json`;
- optional `artifacts/input/...` and `artifacts/output/...` files.

JSON redaction is enabled by default. Raw artifacts are byte-for-byte evidence and are not rewritten, so `artifactsMayContainSecrets` is true whenever artifacts are included. Storage paths are validated to remain inside `AEGIS_DATA_DIR`; missing or invalid files are reported as manifest warnings rather than replaced with invented content.

An active Issue, non-terminal Execution, missing artifact, or unavailable persistent log store makes `complete=false`. This does not invalidate the ZIP: it tells evaluators that the archive is a partial or point-in-time record. Every successfully included payload can be independently checked against the manifest digest.

## HTTP API

```http
GET /api/tasks/{task-or-issue-id}/export
    ?includeArtifacts=true
    &redactSecrets=true
```

The response is `application/zip` and includes:

- `X-Aegis-Evidence-Complete`
- `X-Aegis-Evidence-Redacted`
- `X-Aegis-Evidence-Schema-Version`

Archives are assembled in a mode-`0600` temporary file and removed after the response is sent. Aegis is currently a local control plane without HTTP authentication; do not expose task, log, metric, or export endpoints to an untrusted network without adding an authentication/reverse-proxy boundary.

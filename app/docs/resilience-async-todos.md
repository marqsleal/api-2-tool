# Resilience And Async TODOs

## Scope Locks

- [ ] Keep all persistence behind service/repository interfaces.
- [ ] Keep SQLite adapters as the default runtime backing store.
- [ ] Avoid feature logic coupled to transport handlers.

## Feature 1: LRU Response Cache Per Tool

- [ ] Add cache module with LRU instances keyed by `tool_id`.
- [ ] Cache only idempotent executions (`GET`).
- [ ] Build deterministic cache key from `tool_id`, method, resolved URL, query, normalized arguments, and cache-relevant headers.
- [ ] Add configurable per-tool cache settings:
- `enabled`
- `ttl`
- `max_entries`
- [ ] Add read-through/write-through integration in `ToolExecutorService`.
- [ ] Track cache outcomes in execution result metadata:
- `cache_hit`
- `cache_key`
- `cache_ttl_remaining_ms`

## Feature 2: Retries With Backoff

- [ ] Add retry policy in executor:
- `max_attempts = 3`
- budget `timeout_window = 30s`
- exponential backoff with jitter
- [ ] Retry only for transient conditions:
- network errors
- timeout errors
- HTTP `429`
- HTTP `5xx`
- [ ] Do not retry for non-retriable `4xx`.
- [ ] Enforce context budget before each attempt.
- [ ] Expose attempt count and final retry reason in execution result metadata.

## Feature 3: Circuit Breaker Per Tool

- [ ] Add per-tool circuit breaker state store interface.
- [ ] Add SQLite-backed implementation for breaker state.
- [ ] Add breaker state model:
- `state` (`closed`, `open`, `half_open`)
- `consecutive_failures`
- `opened_until`
- `half_open_remaining_probes`
- `version`
- [ ] Transition rules:
- `closed -> open` when failures reach `5`
- `open -> half_open` after `30s`
- `half_open -> closed` on successful probes
- `half_open -> open` on failed probe
- [ ] Reject execution when breaker is `open`.
- [ ] Ensure atomic state transitions with optimistic concurrency.

## Feature 4: Asynchronous Execution (Jobs)

- [ ] Add job store interface for async execution.
- [ ] Add SQLite-backed job store implementation.
- [ ] Add job lifecycle:
- `pending`
- `running`
- `succeeded`
- `failed`
- [ ] Add endpoints:
- `POST /tool/execute/{id}/jobs`
- `GET /tool/jobs/{job_id}`
- [ ] Add worker loop with configurable concurrency.
- [ ] Add claim/lease model to avoid duplicate active workers per job.
- [ ] Add retry policy for jobs with bounded attempts.
- [ ] Add retention strategy and cleanup process for terminal jobs.

## Delivery Plan

- [ ] Milestone A: Retry + Circuit Breaker
- [ ] Milestone B: Cache Integration
- [ ] Milestone C: Async Jobs API + Worker
- [ ] Milestone D: Hardening and load tests

## Unit Test TODOs

- [ ] Executor retry behavior across transient and permanent failure matrices.
- [ ] Cache hit/miss/expiration behavior per tool.
- [ ] Breaker transition tests for all state transitions.
- [ ] Breaker concurrency tests for atomic updates.
- [ ] Async job lifecycle tests from enqueue to terminal states.
- [ ] Worker lease expiration and re-claim tests.
- [ ] Handler contract tests for job endpoints.

## Done Criteria

- [ ] `go test ./...` passes.
- [ ] Existing sync execution behavior remains backward compatible.
- [ ] New features are configurable and disabled by default when applicable.

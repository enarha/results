# Live + Stored Data Behavior

This document captures how the Results API surfaces PipelineRun/TaskRun data
when the same object may exist both in Kubernetes (live) and in the Results
database (stored).

## Dual-Source Retrieval

* Every `List`, `Get`, and summary-style query asks **both** backends:
  1. Database – canonical Tekton Results `Result`/`Record` rows.
  2. Kubernetes – current PipelineRun/TaskRun CRDs.
* Responses are merged by the record name computed with the same helper logic
  the watcher uses (`resultName`/`recordName`). If both sources report the same
  logical record, the API returns a single entry and sets `source=LIVE`. If only
  the DB has the record, `source=STORED`.
* There is no field-level merge; whichever source wins provides the full record
  payload. Lists remain paginated and filtered exactly once after the merge.

## Identifier Generation

* Stored objects include Tekton Results annotations (e.g.
  `results.tekton.dev/result`, `results.tekton.dev/record`) that tie CRDs to
  their DB rows.
* Live objects may be missing those annotations:
  * The watcher has not reconciled the object yet (race condition).
  * `disable_storing_incomplete_runs=true` – the watcher ignores in-progress
    objects until completion.
  * `disable_crd_update=true` – the watcher never patches annotations.
* In any of those cases, the API synthesizes deterministic names on the fly
  by running the same logic as the watcher:
  * Prefer annotations/labels when present (`triggers.tekton.dev/triggers-eventid`
    or owner references).
  * Fall back to Kubernetes UID.
* This guarantees stable IDs even when annotations are absent, but it costs
  extra CPU per request and prevents us from reusing DB indexes.

## Watcher Optimization

* **Recommended**: even when `disable_storing_incomplete_runs=true`, configure
  the watcher to patch `results.tekton.dev/result` and
  `results.tekton.dev/record` as soon as it observes a new Run. This keeps
  live responses cheap and consistent.
* If `disable_crd_update=true`, annotations are never written. Expect every API
  request involving live data to regenerate identifiers. This mode is supported,
  but responders should anticipate higher per-request overhead.

## Client Expectations

* Clients can rely on the Results API as a single source of truth. Live records
  expose up-to-date status/start information and are marked with `source=LIVE`.
  Stored-only records keep `source=STORED`.
* Tekton Results annotations are not guaranteed to appear on live CRDs; they are
  an implementation detail primarily used for deduplication.
* Long-running Pipelines in clusters where incomplete runs are not stored may
  remain `source=LIVE` for hours. Once the watcher persists and Kubernetes
  garbage-collects the CRD, the API automatically serves the stored entry.

## Performance Considerations

* The most efficient configuration is:
  * `disable_storing_incomplete_runs=false` (objects persisted immediately).
  * `disable_crd_update=false` (annotations always present).
* Any deviation (delayed storage or disabled annotation updates) increases the
  amount of synthetic identifier work and live Kubernetes reads, so capacity
  planning should account for higher API CPU usage and latency.


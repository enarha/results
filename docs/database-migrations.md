# Database migrations

Tekton Results stores its database schema as versioned SQL migrations in
[`pkg/api/server/db/migrations`](../pkg/api/server/db/migrations). The
migrations are compiled into the `results-admin` binary, which applies them.
The API server and the retention policy agent never change the schema. At
startup they only read the recorded schema version and refuse to start if it
is not compatible.

## How versions work

- Migrations are numbered and applied in order, one at a time, even within a
  single `migrate-up`. The database records the version of the last
  migration together with a `dirty` flag in the `schema_migrations` table.
  The flag is set while that migration runs and cleared when it completes.
- The **effective version** is the last migration that has fully completed:
  the recorded version when it is clean, and one less when it is dirty. For
  example, upgrading from 3 to 5 records 4 (dirty), 4, 5 (dirty), then 5;
  the effective version moves 3, 3, 4, 4, 5.
- Each release requires a minimum schema version (`RequiredSchemaVersion` in
  [`migrations.go`](../pkg/api/server/db/migrations/migrations.go)). A
  process starts only if the effective version is at or above it; otherwise
  it exits and is restarted until the migrations catch up.
- Migrations are additive, so a schema at a higher version keeps serving
  older releases. Several releases can share a schema, rolling back the
  application does not need a schema downgrade, and upgrading across several
  releases at once applies all pending migrations in one step.

## Installing and upgrading

Each release contains the `tekton-results-db-migrate` Job, which runs
`results-admin migrate-up`. Apply the release as usual:

```sh
kubectl apply -f release.yaml
kubectl wait job/tekton-results-db-migrate -n tekton-pipelines --for=condition=complete --timeout=10m
```

New API server pods exit with an explanatory message until the Job has
completed, and Kubernetes restarts them. Pods of the previous release keep
serving in the meantime.

If a pod of the Job fails, Kubernetes retries it in a new pod.
`kubectl logs job/tekton-results-db-migrate` shows the logs of one pod only;
to see all attempts, run:

```sh
kubectl logs -n tekton-pipelines -l job-name=tekton-results-db-migrate --prefix
```

A finished Job, with its pods and logs, is kept for six hours and then
deleted. A Job's template cannot be changed, so applying a different release
within that window fails with `field is immutable` for this Job. Delete the
old Job first, then apply the release again:

```sh
kubectl delete job tekton-results-db-migrate -n tekton-pipelines --ignore-not-found
```

`migrate-up` is idempotent; running it again on an up-to-date database does
nothing.

### Upgrading from a release without versioned migrations

Releases before versioned migrations created the schema on API server
startup, so their databases have the Results tables but no version history.
No extra step is needed: apply the new release as above. Before migrating,
the migration Job checks that the existing tables match the definition those
releases created and, if they do, records them as schema version 1. Existing
data is not changed, and API servers of the earlier release keep serving
until they are replaced.

If the tables differ, for example because the database was changed outside
Tekton Results, the Job fails and its log lists every difference:

```sh
kubectl logs job/tekton-results-db-migrate -n tekton-pipelines
```

Fix the reported differences, then delete the Job and apply the release
again.

## Administration

`results-admin` reads the same configuration as the API server (the
`tekton-results-api-config` ConfigMap mounted at `/etc/tekton/results`, plus
`DB_USER` and `DB_PASSWORD` from the environment).

| Command | Purpose |
|---|---|
| `status` | Print the recorded version, dirty flag, effective version, latest embedded version and the version this release requires, as JSON. |
| `check [--structural]` | Exit non-zero unless this release can run against the database. `--structural` also compares the baseline tables with their expected definition. |
| `migrate-up` | Apply all pending migrations. A database from a release without versioned migrations is adopted first if its tables match the baseline definition. |
| `migrate-to <version>` | Apply migrations up to `<version>`. Downgrades are refused. |
| `force <version> --repaired` | Mark a dirty schema clean after a failed migration was repaired by hand. |

Global flags bound every operation: `--connect-timeout` (default `10s`),
`--lock-timeout` (default `30s`, applies to the migration lock and to locks
taken by schema changes), and `--statement-timeout` (default unlimited).
Only one migration runs at a time; a second one waits for the lock up to
`--lock-timeout` and then fails. `--wait-for-database` (default `0`) keeps
retrying for the given duration while the database is not reachable yet,
for example while it is still starting; rejected logins and other errors
fail immediately. The migration Job uses `--wait-for-database=10m`, so it
does not fail when it starts before the database.

## Recovering from a failed migration

A failed migration leaves the database dirty at the failed version. The
current release keeps running. Releases that require the failed version, and
further migrations, are refused until the state is repaired:

1. Inspect the failure in the migration Job's logs and the database.
2. Either complete the failed migration's changes by hand, or undo the parts
   it applied.
3. Record the outcome:
   - completed: `results-admin force <failed version> --repaired`
   - undone: `results-admin force <failed version - 1> --repaired`
4. Run `results-admin migrate-up` again.

## Overriding the required version

If a release declares a higher required version than its code actually
needs and you cannot run its migrations yet, you can set
`DB_SCHEMA_REQUIRED_VERSION_OVERRIDE` in the API configuration to the version
to require instead. The API server and retention policy agent log a warning
on every start while the override is set. Remove it once the database has
been migrated. The override is compared with the effective version like the
required version is; it does not bypass an uninitialized database.

## Adding a migration

1. Add `NNN_name.up.sql` and `NNN_name.down.sql` to
   `pkg/api/server/db/migrations`, where `NNN` is the next version.
2. Keep the migration additive: it must be correct when applied together with
   every earlier migration in one run, against existing data, while servers of
   earlier releases are running. Add columns to existing tables as nullable,
   create foreign keys on existing data `NOT VALID`, and do not drop or
   rename objects that earlier releases use.
3. Raise `RequiredSchemaVersion` to `NNN` only if code in the same release
   depends on objects the migration creates.
4. If the migration adds a unique index, constraint or required column to the
   `results` or `records` tables, update the expectations in
   [`structure.go`](../pkg/api/server/db/migrations/structure.go).

Published migration files are never edited, renamed or deleted; fix a
mistake with a new migration. Unit tests check the file layout and the
required version, and CI rejects changes to existing migration files. Run
the PostgreSQL tests with:

```sh
DB_URL="host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable" \
  go test -tags=e2e ./pkg/api/server/db/migrations/
```

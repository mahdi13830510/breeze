# Editable Database Browser — Design

Status: Approved (pending write-up review)
Date: 2026-07-12

## Motivation

Breeze's Developer Dashboard (`dashboard/`) already ships a Database Browser
(`dashboard/db_inspector.go`, `dashboard/templates/views/database.html`,
`renderDatabase()` in `dashboard/spajavascript.go`) that lists tables and
browses paginated rows via a host-app-supplied `DBInspector` interface. It is
explicitly documented as "read-only by default — no raw SQL editor"
(`dashboard/README.md`).

This design adds optional Create/Update/Delete support to that same module —
Breeze's answer to what Rails' ActiveAdmin gem provides, scoped tightly to
what already exists rather than introducing a new resource-registration
paradigm. The goal is a focused, reviewable PR that reads as a natural
extension of the framework (per `CONTRIBUTING.md`: "Explain why it belongs
inside the framework," "Keep public APIs backward compatible whenever
possible").

Out of scope: a standalone `admin` package, struct-based resource DSL,
auto-generated forms from Go types, role-based authorization beyond the
dashboard's existing HTTP Basic Auth. These are bigger paradigm shifts that
don't fit this PR's goal of a small, natural, mergeable addition.

## Architecture

### New interface: `DBWriter`

`dashboard/db_writer.go` (new file):

```go
type DBWriter interface {
    InsertRow(table string, values map[string]any) (map[string]any, error)
    UpdateRow(table string, pk map[string]any, values map[string]any) error
    DeleteRow(table string, pk map[string]any) error
}

var ErrRowNotFound = errors.New("dashboard: row not found")
```

`pk` is keyed by the primary-key column name(s) reported in
`TableColumn.PrimaryKey` (`dashboard/types.go`), so composite keys work
without extra API surface.

`DBWriter` is deliberately **separate** from `DBInspector` — adding methods
to `DBInspector` directly would break every existing implementation
(including `cmd/dashboard-example/main.go` and any host app). Keeping it a
distinct, optional interface means:
- Existing `DBInspector` implementations keep compiling untouched.
- CRUD only activates when a host app *chooses* to implement `DBWriter`.

### Collector wiring

`dashboard/collector.go` gets:
```go
func (c *Collector) SetDBWriter(w DBWriter)
func (c *Collector) DBWriter() DBWriter
```
Mirrors the existing `SetDBInspector`/`DBInspector()` pattern exactly.

### Config gate

`dashboard/config.go` gets:
```go
// AllowWrites enables Create/Update/Delete in the Database Browser.
// Defaults to false. Even when a DBWriter is configured, writes stay
// disabled until this is explicitly set — mirrors Enabled/DisableAuth.
AllowWrites bool `yaml:"allow_writes" json:"allow_writes"`
```

A table is editable only when **both** `AllowWrites == true` and
`collector.DBWriter() != nil`. This double opt-in (operator config + app
code) is the safety line: it prevents a config flip alone, or an app
implementing `DBWriter` alone, from silently making data editable.

### Routes

Registered in `install.go`/`attach.go` alongside the existing
`/dashboard/api/db/tables` and `/dashboard/api/db/tables/:name`:

- `POST   /dashboard/api/db/tables/:name/rows`
- `PUT    /dashboard/api/db/tables/:name/rows/:pk`
- `DELETE /dashboard/api/db/tables/:name/rows/:pk`

`:pk` encoding: `col1=val1,col2=val2` (URL-encoded), single-column tables
just have one pair. Composite keys are supported without a second URL
scheme.

## Data flow / API contract

### `POST /rows`
Body: `{"values": {...}}`.
- `201` + inserted row (as returned by `InsertRow` — needed for
  auto-increment / DB-assigned defaults).
- `400` + `{"error": "..."}` on writer validation error.
- `403` + `{"error": "writes are not enabled"}` if `AllowWrites` is false or
  no `DBWriter` is configured.

### `PUT /rows/:pk`
Body: `{"values": {...}}` (changed columns only).
- `200` on success.
- `404` + `{"error": "..."}` if `UpdateRow` returns `ErrRowNotFound`.
- `400`/`403` as above.

### `DELETE /rows/:pk`
- `204` on success.
- `404`/`403` as above.

### Table-name validation
Handlers reject any `:name` not present in the current `Tables()` result —
guards against writes to an unlisted/unexpected table even if the URL is
hand-crafted.

### Cache invalidation
`cachedDBInspector` (`db_inspector.go`) caches `TableData` for 30s. A new
method:
```go
func (c *cachedDBInspector) Invalidate(table string)
```
is called by all three write handlers after a successful write, clearing
only that table's cached pages (not the whole cache). Without this, edits
would appear to silently "fail" in the UI for up to 30s.

### `TableData.Writable`
`dashboard/types.go`'s `TableData` gets a new field:
```go
Writable bool `json:"writable,omitempty"`
```
set by the handler (`AllowWrites && DBWriter() != nil`). The frontend uses
this to decide whether to render edit controls — the server is the single
source of truth for whether writes are allowed, so the client never has to
guess or duplicate the gating logic.

## Frontend (`dashboard/spajavascript.go`, `renderDatabase()`)

When `S.dbData.writable` is true:
- Table cells become click-to-edit (contenteditable on click, PUT on blur
  if the value changed) — inline, no modal, consistent with the existing
  single-file SPA's style (no external deps, per README).
- Each row gets a delete icon → native `confirm()` → `DELETE`.
- Toolbar gets a "New row" button → inserts a blank editable row at the top
  → `POST` on save.

When `writable` is false (the default), rendering is byte-for-byte what it
is today — read-only. This is the "zero overhead / zero visual change when
disabled" property the dashboard already advertises for its other features.

## Error handling

- Writer-side errors (constraint violations, etc.) are surfaced verbatim in
  `{"error": ...}` with `500`, since Breeze has no schema knowledge of its
  own (no built-in ORM — `DBInspector`/`DBWriter` are host-app-supplied,
  same division of responsibility that already exists for reads).
- `404` via `ErrRowNotFound` sentinel, checked with `errors.Is`.
- `403` distinguishes "not configured" from "server error" so the frontend
  can render the right empty/disabled state rather than a generic failure.
- Every successful write calls `c.RecordLog(...)` (existing method used
  elsewhere in `collector.go`) on the `app` channel with table, PK, and
  operation — a free audit trail via infrastructure that already exists,
  not a new logging system.

## Testing

- `dashboard/db_writer_test.go`: table-driven tests per handler — success,
  `AllowWrites=false`, no `DBWriter` configured, `ErrRowNotFound`, generic
  writer error. Mirrors the mock-based style in `cached_inspector_test.go`.
- Extend cache tests to cover `Invalidate(table)` — a write to table A must
  not evict table B's cached pages.
- One `net/http/httptest`-based integration test: `Install` → `SetDBWriter`
  → `AllowWrites=true` → POST/PUT/DELETE through the real router, matching
  the pattern in `dashboard_test.go`.

## Backward compatibility

- No changes to `DBInspector`, `TableInfo`, `TableColumn`.
- `TableData.Writable` is an additive field (`omitempty`), safe for any
  existing consumer of the JSON API.
- `Config.AllowWrites` defaults to `false` via `withDefaults()`
  (`dashboard/config.go`), so existing deployments see no behavior change
  after upgrading.

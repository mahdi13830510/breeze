package dashboard

import "errors"

// DBWriter provides write access to database tables for the Database
// Browser's Create/Update/Delete UI. It is deliberately separate from
// DBInspector so existing DBInspector implementations keep compiling
// unmodified — CRUD only activates when a host app also implements this
// interface AND Config.AllowWrites is true (see Collector.writableGuard).
//
// pk is keyed by the primary-key column name(s) reported via
// TableColumn.PrimaryKey in the table's TableData.Columns, so composite
// keys work without extra API surface. Values in pk and values are always
// strings as received from the HTTP layer — implementations are
// responsible for converting to the underlying column type, the same
// division of responsibility DBInspector already has for reads.
type DBWriter interface {
	// InsertRow creates a new row and returns it as stored (including any
	// DB-assigned defaults such as auto-increment primary keys).
	InsertRow(table string, values map[string]any) (map[string]any, error)

	// UpdateRow updates the row identified by pk with the given column
	// values. Returns ErrRowNotFound if no row matches pk.
	UpdateRow(table string, pk map[string]any, values map[string]any) error

	// DeleteRow deletes the row identified by pk. Returns ErrRowNotFound
	// if no row matches pk.
	DeleteRow(table string, pk map[string]any) error
}

// ErrRowNotFound is returned by DBWriter.UpdateRow/DeleteRow when pk does
// not match any existing row.
var ErrRowNotFound = errors.New("dashboard: row not found")

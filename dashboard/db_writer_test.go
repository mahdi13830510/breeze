package dashboard

import "testing"

// mockWriter is a minimal DBWriter used across db_writer_test.go.
type mockWriter struct {
	insertErr error
	updateErr error
	deleteErr error
}

func (m *mockWriter) InsertRow(table string, values map[string]any) (map[string]any, error) {
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	out := map[string]any{"id": "1"}
	for k, v := range values {
		out[k] = v
	}
	return out, nil
}

func (m *mockWriter) UpdateRow(table string, pk map[string]any, values map[string]any) error {
	return m.updateErr
}

func (m *mockWriter) DeleteRow(table string, pk map[string]any) error {
	return m.deleteErr
}

// TestSetDBWriter_Nil verifies that passing nil to SetDBWriter clears the
// writer (no nil pointer panic), mirroring TestSetDBInspector_Nil in
// cached_inspector_test.go.
func TestSetDBWriter_Nil(t *testing.T) {
	cfg := DefaultConfig()
	c := newCollector(cfg, nil)
	c.SetDBWriter(&mockWriter{})
	if c.DBWriter() == nil {
		t.Fatal("writer not set")
	}
	c.SetDBWriter(nil)
	if c.DBWriter() != nil {
		t.Fatal("writer not cleared")
	}
}

// TestConfigAllowWritesDefaultsFalse verifies AllowWrites is false unless
// explicitly set, so upgrading breeze never silently makes data editable.
func TestConfigAllowWritesDefaultsFalse(t *testing.T) {
	cfg := Config{}.withDefaults()
	if cfg.AllowWrites {
		t.Error("AllowWrites should default to false")
	}
	cfg2 := DefaultConfig()
	if cfg2.AllowWrites {
		t.Error("DefaultConfig().AllowWrites should be false")
	}
}

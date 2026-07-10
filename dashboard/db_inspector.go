package dashboard

import (
	"fmt"
	"sync"
	"time"
)

const dbInspectorCacheTTL = time.Second

// DBInspector provides table metadata and row previews for the Database Browser page.
type DBInspector interface {
	Tables() ([]TableInfo, error)
	TableData(name string, page, pageSize int, search string) (TableData, error)
}

// TableInfo is one table summary entry.
type TableInfo struct {
	Name string `json:"name"`
	Rows int64  `json:"rows"`
}

// TableData is one page of table rows.
type TableData struct {
	Table    string           `json:"table"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Total    int64            `json:"total"`
	Rows     []map[string]any `json:"rows,omitempty"`
}

// SetDBInspector sets or clears the database inspector.
func (c *Collector) SetDBInspector(inspector DBInspector) {
	c.dbInspectorMu.Lock()
	defer c.dbInspectorMu.Unlock()
	if inspector == nil {
		c.dbInspector = nil
		return
	}
	c.dbInspector = newCachedInspector(inspector, dbInspectorCacheTTL)
}

// DBInspector returns the currently configured database inspector.
func (c *Collector) DBInspector() DBInspector {
	c.dbInspectorMu.RLock()
	defer c.dbInspectorMu.RUnlock()
	return c.dbInspector
}

type cachedInspector struct {
	inner DBInspector
	ttl   time.Duration

	mu       sync.RWMutex
	tables   []TableInfo
	tablesAt time.Time
	data     map[string]cachedTableData
}

type cachedTableData struct {
	value TableData
	at    time.Time
}

func newCachedInspector(inner DBInspector, ttl time.Duration) *cachedInspector {
	return &cachedInspector{
		inner: inner,
		ttl:   ttl,
		data:  make(map[string]cachedTableData),
	}
}

func (c *cachedInspector) Tables() ([]TableInfo, error) {
	now := time.Now()
	c.mu.RLock()
	if len(c.tables) > 0 && now.Sub(c.tablesAt) < c.ttl {
		out := append([]TableInfo(nil), c.tables...)
		c.mu.RUnlock()
		return out, nil
	}
	c.mu.RUnlock()

	tables, err := c.inner.Tables()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.tables = append([]TableInfo(nil), tables...)
	c.tablesAt = now
	out := append([]TableInfo(nil), c.tables...)
	c.mu.Unlock()
	return out, nil
}

func (c *cachedInspector) TableData(name string, page, pageSize int, search string) (TableData, error) {
	now := time.Now()
	key := fmt.Sprintf("%s|%d|%d|%s", name, page, pageSize, search)

	c.mu.RLock()
	if entry, ok := c.data[key]; ok && now.Sub(entry.at) < c.ttl {
		c.mu.RUnlock()
		return entry.value, nil
	}
	c.mu.RUnlock()

	data, err := c.inner.TableData(name, page, pageSize, search)
	if err != nil {
		return TableData{}, err
	}

	c.mu.Lock()
	c.data[key] = cachedTableData{value: data, at: now}
	out := c.data[key].value
	c.mu.Unlock()
	return out, nil
}

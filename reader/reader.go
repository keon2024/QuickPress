package reader

import (
	"strings"
	"sync/atomic"
)

const (
	CSV = "csv"
)

type Config struct {
	Type     string
	FilePath string
}

type Dataset struct {
	rows []map[string]string
	next atomic.Uint64
}

func Load(cfg Config) (*Dataset, error) {
	if strings.TrimSpace(cfg.FilePath) == "" {
		return NewDataset(nil), nil
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "", CSV:
		return loadCSV(cfg.FilePath)
	default:
		return nil, ErrUnsupportedReader(strings.TrimSpace(cfg.Type))
	}
}

func NewDataset(rows []map[string]string) *Dataset {
	if len(rows) == 0 {
		rows = []map[string]string{{}}
	}
	return &Dataset{rows: rows}
}

func (d *Dataset) Size() int {
	if d == nil {
		return 0
	}
	return len(d.rows)
}

func (d *Dataset) Next() map[string]string {
	if d == nil || len(d.rows) == 0 {
		return map[string]string{}
	}
	idx := d.next.Add(1) - 1
	src := d.rows[idx%uint64(len(d.rows))]
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

type unsupportedReaderError string

func (e unsupportedReaderError) Error() string {
	return "不支持的 reader 类型: " + string(e)
}

func ErrUnsupportedReader(name string) error {
	return unsupportedReaderError(name)
}

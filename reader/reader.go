package reader

type Reader interface {
	Read() <-chan map[string]string
	Init(config ReaderConfig) Reader
}

const (
	CSV = "csv"
)

// 所有读取请求共用配置
type ReaderConfig struct {
	FilePath   string
	StopSignal <-chan bool
}

// Reader 工厂
var readerFactory = map[string]Reader{
	CSV: csvReader{},
}

// 从工厂中读取对应实例
func NewReader(readType string, config ReaderConfig) Reader {
	reader := readerFactory[readType]
	return reader.Init(config)
}

package reader

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

func loadCSV(filePath string) (*Dataset, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开 CSV 文件失败: %w", err)
	}
	defer file.Close()

	csvReader := csv.NewReader(file)
	headers, err := csvReader.Read()
	if err != nil {
		if err == io.EOF {
			return NewDataset(nil), nil
		}
		return nil, fmt.Errorf("读取 CSV 表头失败: %w", err)
	}

	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}

	rows := make([]map[string]string, 0, 256)
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取 CSV 数据失败: %w", err)
		}

		row := make(map[string]string, len(headers))
		for i, header := range headers {
			if header == "" {
				continue
			}
			if i < len(record) {
				row[header] = record[i]
			}
		}
		rows = append(rows, row)
	}

	return NewDataset(rows), nil
}

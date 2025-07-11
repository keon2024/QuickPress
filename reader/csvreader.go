package reader

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
)

type csvReader struct {
	FilePath   string
	StopSignal <-chan bool
}

// 初始化
func (c csvReader) Init(config ReaderConfig) Reader {
	c.FilePath = config.FilePath
	c.StopSignal = config.StopSignal
	return c
}

func (c csvReader) Read() <-chan map[string]string {
	var out = make(chan map[string]string)
	go func() {
		for {
			select {
			case <-c.StopSignal:
				fmt.Println("read 方法中关闭")
				return
			default:
				readCSV(c.FilePath, out, c.StopSignal)
			}
		}

	}()
	return out

}

func readCSV(filePath string, out chan<- map[string]string, sigStopRead <-chan bool) error {
	fmt.Println("进入readCSV")
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		return err
	}
	defer file.Close()

	// 创建 CSV Reader
	reader := csv.NewReader(file)

	// 读取表头（第一行）
	headers, err := reader.Read()
	if err != nil {
		fmt.Printf("读取表头失败: %v\n", err)
		return err
	}

	// 逐行读取数据
	for {
		// 判断是否要停止
		select {
		case <-sigStopRead:
			close(out)
			fmt.Println("关闭out（chan）写入")
			return nil
		default:
			// 读取一行数据
			record, err := reader.Read()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				fmt.Printf("读取行失败: %v\n", err)
				break
			}

			// 将每行转换为 map[string]string
			row := make(map[string]string)
			for i, value := range record {
				if i < len(headers) { // 防止越界
					row[headers[i]] = value
				}
			}

			// 通过 channel 发送
			out <- row
			//time.Sleep(1 * time.Second)
		}

	}
}

# QuickPress

QuickPress 是一个本地压测工具，提供 Web 控制台来管理压测配置、请求链、并发阶段、运行状态和请求结果。

## 功能概览

- 支持通过 Web 控制台编辑和保存压测配置
- 支持 CSV 数据文件驱动请求参数
- 支持全局变量、请求链、Header、Query、Body、断言和变量提取
- 支持分阶段并发计划，阶段时长按单阶段持续时间配置，运行中可调整当前并发或未来阶段
- 提供实时运行状态、响应耗时曲线、TPS/QPS 成功失败曲线
- 提供类似 JMeter 查看结果树的最近请求详情

## 启动

项目要求 Go 1.21+。

```bash
go run .
```

默认会读取 `config/prod.yml`，并按配置中的 `app.listen` 启动控制台，默认地址通常是：

```text
http://127.0.0.1:8080
```

也可以指定配置文件或监听地址：

```bash
go run . -config config/prod.yml -listen :18080
```

## Web 控制台使用

1. 打开控制台地址。
2. 在顶部可以导入配置、保存配置、启动压测、停止压测。
3. 在“测试计划配置”中设置循环次数、时间单位、数据文件和并发阶段。
4. 在“全局变量”中维护公共变量，例如 `host`、`token`、`scene`。
5. 在“请求链”中按顺序配置接口请求。
6. 点击“启动压测”开始运行。
7. 在“运行状态”查看当前并发、累计请求、成功失败数、耗时曲线和 TPS/QPS 曲线。
8. 在“查看结果”中查看最近请求的请求信息、响应内容、断言结果和错误信息。

## 配置文件示例

```yaml
app:
  listen: :8080

concurrency:
  loop: 1
  unit: s
  stages:
    - label: 预热
      duration: 10
      target: 2
    - label: 稳定
      duration: 20
      target: 6

reader:
  type: csv
  file: config/data/users.csv

global:
  host: https://postman-echo.com
  scene: quickpress

requests:
  - name: 查询示例
    method: GET
    url: ${host}/get
    timeout_ms: 3000
    headers:
      X-Scene: ${scene}
    query:
      keyword: hello
      scene: ${scene}
      user_id: ${user_id}
    expected_status: 200
    contains:
      - hello
```

## 配置说明

### app

- `listen`：Web 控制台监听地址，例如 `:8080`。

### concurrency

- `loop`：循环次数，正整数表示固定轮次，`-1` 表示无限循环。
- `unit`：阶段时间单位，支持 `s`、`m`、`h`。
- `stages`：并发阶段列表，按配置顺序执行。
  - `label`：阶段名称，可为空。
  - `duration`：当前阶段的持续时间，单位由 `unit` 决定。
  - `target`：该阶段目标并发数。

### reader

- `type`：数据源类型，目前主要使用 `csv`。
- `file`：CSV 文件路径。为空时使用空数据集。

CSV 第一行作为字段名，后续每一行是一组请求变量。例如：

```csv
user_id,token
1001,abc
1002,def
```

请求配置中可以用 `${user_id}`、`{{user_id}}` 或 `{user_id}` 引用字段值。

### global

全局变量会参与请求渲染，可以在 URL、Header、Query、Body、断言和提取器中引用。

### requests

每个虚拟用户会按顺序执行 `requests` 中的请求链。

常用字段：

- `name`：请求名称。
- `method`：HTTP 方法，例如 `GET`、`POST`。
- `url`：请求地址，支持变量占位符。
- `timeout_ms`：请求超时时间，单位毫秒。
- `headers`：请求头对象。
- `query`：Query 参数对象。
- `body_type`：Body 类型，支持 `json`、`form`、`multipart`，为空表示无 Body。
- `body`：请求 Body 对象。
- `expected_status`：期望 HTTP 状态码，`0` 表示不校验。
- `contains`：响应 Body 必须包含的文本列表。
- `extractors`：从 JSON 响应中提取变量，供后续请求使用。

提取器示例：

```yaml
extractors:
  token: data.token
  first_item_id: data.items[0].id
  second_item_name: data.items.[1].name
```

提取路径支持对象点路径和数组下标，数组下标可写为 `items[0].id`、`items.[0].id`，当当前节点是数组时也支持 `items.0.id`。后续请求可以使用 `${token}`、`${first_item_id}`。

## 常用命令

```bash
# 运行全部测试
go test ./...

# 格式化代码
gofmt -w .
```

## 注意事项

- 压测前请确认目标服务允许被压测，并控制并发规模。
- CSV 文件较适合放在 `config/data/` 下，便于配置引用。
- 运行中修改并发阶段后，需要点击“同步到运行计划”才会影响当前任务的未来阶段。

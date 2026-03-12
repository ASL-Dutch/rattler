# Rattler

Rattler 是与 SoftPak 报关软件协同工作的中间件，负责**报关文档的发送与接收**。通过监听本地目录中的 Export 文件、税金单（PDF）文件，将内容经 RabbitMQ 消息队列发送；同时消费 Import 报关文档消息，将 XML 写入指定目录。内置 HTTP 服务提供 Export XML 与税金单的 API 下载；税金单目录也可通过 Caddy 部署为静态文件服务供外部访问。

---

## 一、工程介绍

- **项目名称**：Rattler
- **技术栈**：Go 1.23+、Echo、Viper、RabbitMQ、fsnotify
- **运行方式**：单进程，同时启动 HTTP 服务、消息队列消费者、文件监听器

### 主要能力

| 能力                     | 说明                                                                            |
| ------------------------ | ------------------------------------------------------------------------------- |
| **Export 发送**          | 监听 NL/BE 申报国家的 Export XML 目录，文件就绪后读取并发送到 RabbitMQ 导出队列 |
| **Import 接收**          | 消费 RabbitMQ 导入队列，将报关 XML 写入配置的 `import.xml-dir`                  |
| **税金单处理**           | 监听 NL/BE 税金单 PDF 目录，按日期归档到备份目录（可选保留原文件）              |
| **文件下载 API**         | 提供税金单 PDF、Export XML 的 HTTP 下载接口                                     |
| **静态文件服务（可选）** | 配合 Caddy 将税金单备份目录发布为静态站点，供外网按 URL 下载                    |

---

## 二、业务流程与设计特点

### 2.1 整体数据流

```
                    ┌─────────────────────────────────────────────────────────┐
                    │                      Rattler                             │
  SoftPak           │  ┌─────────────┐    ┌──────────────┐    ┌─────────────┐  │          外部系统
  写文件到本地  ────►│  │ 文件监听     │───►│ 事件队列     │───►│ 文件处理器   │───► RabbitMQ
                    │  │ (FSWatcher)  │    │ (FIFO 单消费者)│   │ (读→发/备份) │  │
                    │  └─────────────┘    └──────────────┘    └─────────────┘  │
                    │         ▲                    │                            │
                    │         │                    ▼                            │
                    │  ┌──────┴──────┐     ┌──────────────┐                     │
                    │  │ HTTP 服务   │     │ RabbitMQ     │◄───────────────────┘
                    │  │ (下载 API)  │     │ 消费者       │     Import XML 消息
                    │  └─────────────┘     └──────┬───────┘
                    │                            │ 写 XML 到 import 目录
                    └────────────────────────────┼─────────────────────────────┘
                                                 ▼
                                           本地 import 目录
                                           (SoftPak 读取)
```

### 2.2 文件监听与处理：两步解耦

为避免并发创建文件时遗漏事件，并保证同一报关单多文件按创建顺序处理，采用 **「先入队，再按序消费」** 的架构：

```
┌─────────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│  FSWatcher      │     │  统一事件队列         │     │  FileProcessor  │
│  监听目录       │ ──► │  EventChannel        │ ──► │  单消费者       │
│  仅入队，不处理 │     │  (1000+500 缓冲)      │     │  等可读→读→发   │
└─────────────────┘     └──────────────────────┘     └─────────────────┘
```

1. **第一步：文件创建监听 → 入队**
   - 仅监听目标目录下的**创建**事件（可配置是否递归子目录，默认不递归）。
   - 做文件类型、模式校验后，将事件**阻塞写入**统一事件通道，不在此做「等文件可读」或业务逻辑，保证不丢事件。

2. **第二步：按序消费队列**
   - 单消费者从队列 **FIFO** 取事件。
   - 对每条事件**同步**执行：等待文件可读 → 读取内容 → 调用业务（发 MQ、备份等）。
   - 同一报关单的多个文件按入队顺序依次处理，等价于按创建顺序。

### 2.3 其他设计要点

- **文件移动异步化**：所有「移动/复制」操作通过 `file-mover` 队列异步执行，避免高并发时阻塞主流程。
- **税金单按日期归档**：备份路径为 `backup-dir/YYYY/MM/`，文件名可带 `YYYYMM_` 前缀，便于 Caddy 按日期路径提供静态访问。
- **配置与代码解耦**：通过 Viper 统一加载 YAML，配置结构在 `internal/config` 中定义，便于扩展与文档化。

---

## 三、系统功能

### 3.1 监听与发送（Export）

- 监听 NL/BE 的 Export XML 监听目录（`watchers.export.nl/be.watch-dir`）。
- 文件就绪后读取内容，发送到 RabbitMQ 导出交换机/队列（`rabbitmq.export`）。
- 处理后可按业务将文件备份到 `backup-dir`（若在业务逻辑中实现）。

### 3.2 接收与落盘（Import）

- 消费 RabbitMQ 导入队列（`rabbitmq.import`）。
- 将消息体中的报关 XML 写入 `import.xml-dir`，供 SoftPak 读取。

### 3.3 税金单（PDF）监听与归档

- 监听 NL/BE 税金单 PDF 目录（`watchers.pdf.nl/be.watch-dir`）。
- 文件就绪后通过 `file-mover` 队列移动到备份目录 `backup-dir/YYYY/MM/`，文件名可带 `YYYYMM_` 前缀。
- 可配置 `keep-original: true` 在监听目录保留原文件（即复制一份到备份）。

### 3.4 HTTP 文件下载 API

| 功能             | 方法     | 路径                                         | 说明                                                                    |
| ---------------- | -------- | -------------------------------------------- | ----------------------------------------------------------------------- |
| 税金单 PDF       | GET      | `/download/pdf/:origin/:target?dc=NL\|BE`    | 按申报国 dc 从存储目录取 PDF，`origin`/`target` 不含后缀时自动补 `.pdf` |
| Export XML       | GET      | `/download/xml/:dc/:filename?download=1`     | 按 dc 与文件名（含后缀）查找 Export 文件；`download=1` 时以附件下载     |
| 税金单（新）     | GET      | `/api/tax-bills/:country/download/:filename` | 按国家与文件名下载，支持按日期目录查找                                  |
| 文件搜索         | POST     | `/search/file`                               | 按条件搜索文件（见 Swagger）                                            |
| Export 列表/重发 | GET/POST | `/export/list/:dc`、`/export/remover/:dc`    | 列表与重发（见 Swagger）                                                |
| Swagger          | GET      | `/swagger/*`                                 | API 文档                                                                |

---

## 四、配置文件说明

配置文件默认名为 `.rattler.yaml`，可通过启动参数 `--config` 指定路径。以下为各段含义。

### 4.1 基本服务

| 参数   | 类型 | 说明                         |
| ------ | ---- | ---------------------------- |
| `port` | int  | HTTP 服务监听端口，默认 7003 |

### 4.2 日志

| 参数            | 类型   | 说明                                          |
| --------------- | ------ | --------------------------------------------- |
| `log.level`     | string | 日志级别：`debug` / `info` / `warn` / `error` |
| `log.directory` | string | 日志文件所在目录，如 `out/log/`               |

### 4.3 临时目录

| 参数      | 类型   | 说明                       |
| --------- | ------ | -------------------------- |
| `tmp-dir` | string | 临时文件目录，如 `out/tmp` |

### 4.4 监听任务（watchers）

| 参数                     | 类型 | 说明                                                   |
| ------------------------ | ---- | ------------------------------------------------------ |
| `watchers.watch-subdirs` | bool | 是否递归监听子目录，默认 `false`，仅监听配置的目录本身 |

**Export（XML）**

| 参数                            | 类型   | 说明                   |
| ------------------------------- | ------ | ---------------------- |
| `watchers.export.enabled`       | bool   | 是否启用 Export 监听   |
| `watchers.export.nl.enabled`    | bool   | 是否启用荷兰 Export    |
| `watchers.export.nl.watch-dir`  | string | 荷兰 Export 监听目录   |
| `watchers.export.nl.backup-dir` | string | 荷兰 Export 备份目录   |
| `watchers.export.be.enabled`    | bool   | 是否启用比利时 Export  |
| `watchers.export.be.watch-dir`  | string | 比利时 Export 监听目录 |
| `watchers.export.be.backup-dir` | string | 比利时 Export 备份目录 |

**PDF（税金单）**

| 参数                            | 类型   | 说明                                                                                                                                         |
| ------------------------------- | ------ | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `watchers.pdf.enabled`          | bool   | 是否启用税金单监听                                                                                                                           |
| `watchers.pdf.nl.enabled`       | bool   | 是否启用荷兰税金单                                                                                                                           |
| `watchers.pdf.nl.watch-dir`     | string | 荷兰税金单监听目录                                                                                                                           |
| `watchers.pdf.nl.backup-dir`    | string | 荷兰税金单备份目录                                                                                                                           |
| `watchers.pdf.nl.keep-original` | bool   | 是否在监听目录保留原文件（复制到备份）；为 true 时原路径与 backup 都有新文件，适合将原路径作为 Caddy「不按日期」根目录（ASL 等历史文件场景） |
| `watchers.pdf.be.*`             | -      | 同上，对应比利时                                                                                                                             |

### 4.5 存储目录（storage）

供 HTTP 下载使用的路径，可与备份目录一致或不同。

| 参数                  | 类型   | 说明                                   |
| --------------------- | ------ | -------------------------------------- |
| `storage.nl.tax-bill` | string | 荷兰税金单对外提供下载的根目录         |
| `storage.nl.export`   | string | 荷兰 Export XML 对外提供下载的根目录   |
| `storage.be.tax-bill` | string | 比利时税金单对外提供下载的根目录       |
| `storage.be.export`   | string | 比利时 Export XML 对外提供下载的根目录 |

### 4.6 导入（import）

| 参数             | 类型   | 说明                                       |
| ---------------- | ------ | ------------------------------------------ |
| `import.xml-dir` | string | Import 报关 XML 落盘目录，SoftPak 从此读取 |

### 4.7 文件移动队列（file-mover）

| 参数                      | 类型   | 说明                              |
| ------------------------- | ------ | --------------------------------- |
| `file-mover.enabled`      | bool   | 是否启用异步文件移动队列          |
| `file-mover.queue-size`   | int    | 队列缓冲大小                      |
| `file-mover.queue-name`   | string | 队列名称，用于日志                |
| `file-mover.worker-count` | int    | 工作协程数，建议不超过 CPU 核心数 |

### 4.8 RabbitMQ（rabbitmq）

| 参数                             | 类型   | 说明                                          |
| -------------------------------- | ------ | --------------------------------------------- |
| `rabbitmq.url`                   | string | 连接 URL，如 `amqp://user:password@host:5672` |
| `rabbitmq.heartbeat`             | string | 心跳间隔，如 `10s`                            |
| `rabbitmq.connection-timeout`    | string | 连接超时                                      |
| `rabbitmq.max-connections`       | int    | 最大连接数                                    |
| `rabbitmq.max-channels-per-conn` | int    | 每连接最大通道数                              |
| `rabbitmq.auto-reconnect`        | bool   | 是否自动重连                                  |
| `rabbitmq.reconnect-interval`    | string | 重连间隔                                      |
| `rabbitmq.prefetch-count`        | int    | 消费者预取数                                  |
| `rabbitmq.auto-ack`              | bool   | 是否自动 ack                                  |
| `rabbitmq.import.consumer`       | string | 导入消费者名称                                |
| `rabbitmq.import.exchange`       | string | 导入交换机                                    |
| `rabbitmq.import.exchange-type`  | string | 导入交换机类型，如 `topic`                    |
| `rabbitmq.import.queue`          | string | 导入队列名                                    |
| `rabbitmq.export.exchange`       | string | 导出交换机                                    |
| `rabbitmq.export.exchange-type`  | string | 导出交换机类型                                |
| `rabbitmq.export.queue`          | string | 导出队列名前缀（如按 NL/BE 区分时使用）       |

> **注意**：`rabbitmq.url` 中若密码含特殊字符，请使用单引号包裹整段 URL，并按 AMQP 规范转义。

---

## 五、命令与运行

### 5.1 查看帮助

```bash
rattler --help
rattler --config /path/to/config.yaml --help
```

### 5.2 直接运行（前台）

```bash
rattler
# 或指定配置
rattler --config config.yaml
```

默认会加载当前目录或 Viper 搜索路径下的 `.rattler.yaml`；`--config` 可指定为绝对或相对路径。

### 5.3 Windows 服务与生产部署

在 Windows 上建议使用 **NSSM** 将 Rattler 安装为系统服务，便于开机自启与统一管理。详细步骤、NSSM 安装与常用命令见 **[DEPLOY.md](DEPLOY.md)**。

---

## 六、税金单静态文件服务（Caddy）

Rattler 自身提供 HTTP API 下载税金单；若需要将**税金单目录**以静态站点方式对外提供（例如固定 URL 格式、CDN、统一域名），可使用 **Caddy** 将税金单目录发布为静态文件服务。

Caddy 需**同时支持**两种访问方式（ASL 等场景下存在大量历史税金文件在原路径，无法一次性迁到按年月路径）：

- **按日期**：URL `/202505_xxx.pdf` → 在 `backup-dir/2025/05/202505_xxx.pdf` 提供；
- **不按日期**：URL `/任意文件名.pdf` → 在**原路径**根目录（与 config 中 `storage.*.tax-bill` 一致）直接提供。

配置时「按日期」根对应 backup-dir，「不按日期」根对应原路径（`storage.*.tax-bill` / watch-dir）；`keep-original: true` 时新文件也保留在原路径，便于统一从原路径访问。示例与 Windows 下 Caddy 安装、NSSM 部署及 Caddyfile 双根配置见 **[DEPLOY.md](DEPLOY.md)**。

---

## 七、开发与构建

- 依赖：Go 1.23+
- 构建：`make build` 或 `go build -o build/rattler .`
- Windows 64 位：`make build-windows`，产物在 `build/rattler-windows-amd64.exe`
- API 文档：运行后访问 `http://localhost:7003/swagger/index.html`（端口以配置为准）

---

## 八、许可证

见 [LICENSE](LICENSE) 文件。

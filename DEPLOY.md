# Rattler 部署文档（Windows 为主）

本文档说明在 **Windows** 上部署 Rattler 及配套的 Caddy 税金单静态文件服务的步骤，包括：安装与配置、使用 NSSM 注册为系统服务、服务管理命令，以及使用 Caddy 将税金单目录发布为静态文件供外部访问下载。

---

## 一、环境与准备

### 1.1 所需环境

- **操作系统**：Windows 10/11 或 Windows Server（64 位）
- **Rattler**：已编译的 `rattler.exe`（见 [README.md](README.md) 构建说明）
- **配置文件**：YAML 格式，如 `config.yaml` 或 `.rattler.yaml`
- **RabbitMQ**：已安装并可访问，连接信息填入配置文件

### 1.2 目录建议（示例）

可按实际环境调整路径，以下为常见示例：

| 用途               | 示例路径                                           |
| ------------------ | -------------------------------------------------- |
| Rattler 程序与配置 | `C:\Rattler\`                                      |
| NSSM               | `C:\Rattler\nssm.exe`（建议与 rattler.exe 同目录） |
| 日志               | `C:\RattlerTmp\log\` 或 `C:\Rattler\log\`          |
| 临时目录           | `C:\RattlerTmp\tmp\`                               |
| 税金单备份（NL）   | `C:\RattlerSoftpak\backup\pdf\nl` 或与业务一致     |
| 税金单备份（BE）   | `C:\RattlerSoftpak\backup\pdf\be`                  |
| Caddy 日志         | `C:\RattlerSoftpak\log\caddy\`                     |

**Rattler 部署主目录建议结构**（以 `C:\Rattler\` 为例）：

- `rattler.exe`：Rattler 可执行文件
- `config.yaml`（或 `.rattler.yaml`）：配置文件
- `nssm.exe`：NSSM 工具（建议与 rattler.exe 同目录，便于安装/管理 Windows 服务）

---

## 二、Rattler 配置与运行

### 2.1 配置文件

在 Rattler 程序目录或指定路径放置 YAML 配置，例如 `C:\Rattler\config.yaml`。Windows 下注意：

- 路径使用反斜杠时建议加引号，或使用正斜杠：`C:/RattlerTmp/log/`
- RabbitMQ 密码含特殊字符时，`url` 用单引号包裹

示例片段（仅作参考，完整参数见 [README.md](README.md)）：

```yaml
port: 7003
log:
  level: info
  directory: C:/RattlerTmp/log/
tmp-dir: C:/RattlerTmp/tmp
watchers:
  watch-subdirs: false
  export:
    enabled: true
    nl:
      enabled: true
      watch-dir: C:/SP/cust/NCC/InterfaceOut
      backup-dir: C:/RattlerSoftpak/backup/nl
    be:
      enabled: true
      watch-dir: C:/RattlerSoftpak/interface/export/be
      backup-dir: C:/RattlerSoftpak/backup/be
  pdf:
    enabled: true
    nl:
      enabled: true
      keep-original: true
      watch-dir: C:/SP/cust/NCC/pdf
      backup-dir: C:/RattlerSoftpak/backup/pdf/nl
    be:
      enabled: true
      keep-original: false
      watch-dir: C:/RattlerSoftpak/interface/taxbill
      backup-dir: C:/RattlerSoftpak/backup/pdf/be
storage:
  nl:
    tax-bill: C:/SP/cust/NCC/pdf
    export: C:/RattlerSoftpak/backup/nl
  be:
    tax-bill: C:/RattlerSoftpak/interface/taxbill
    export: C:/RattlerSoftpak/backup/be
import:
  xml-dir: C:/SP/cust/NCC/InterfaceIn
file-mover:
  enabled: true
  queue-size: 500
  worker-count: 2
rabbitmq:
  url: "amqp://user:password@host:5672"
  # ... 其余见 README
```

### 2.2 前台测试运行

在 PowerShell 或 CMD 中进入 Rattler 所在目录，执行：

```cmd
cd C:\Rattler
rattler.exe --config config.yaml
```

确认无报错、能连上 RabbitMQ、监听目录与端口正常后，再按下一节安装为服务。

---

## 三、使用 NSSM 将 Rattler 部署为 Windows 服务

### 3.1 安装 NSSM

1. **下载**
   - 打开 NSSM 官网下载页：[https://nssm.cc/download](https://nssm.cc/download)。
   - 下载与系统架构匹配的压缩包（64 位系统选 `win64` 目录下的版本；推荐使用 **pre-release build 2.24-101** 或更新版本，以避免 Windows 10 及以上系统上服务启动失败，详见官网说明）。

2. **解压并放置**
   - 解压下载的 zip，将 **win64** 目录下的 `nssm.exe` **复制到 Rattler 部署主目录**（如 `C:\Rattler\`），与 `rattler.exe`、`config.yaml` 等同目录存放，便于统一管理与备份。
   - 无需将 NSSM 加入系统 PATH；在部署主目录下以管理员身份打开 CMD/PowerShell 后，直接使用 `.\nssm.exe` 或 `nssm`（若已将该目录加入用户 PATH）执行命令即可。

### 3.2 安装 Rattler 服务（命令行）

**请以管理员身份**打开 CMD 或 PowerShell，并进入 Rattler 部署主目录（即放有 `rattler.exe` 和 `nssm.exe` 的目录）：

```cmd
cd C:\Rattler
```

**安装服务：**（若 `nssm.exe` 在本目录，使用 `.\nssm.exe`；若已加入 PATH 可直接写 `nssm`）

```cmd
.\nssm.exe install RattlerService "C:\Rattler\rattler.exe" --config "C:\Rattler\config.yaml"
```

说明：

- `RattlerService`：服务名称，可自拟。
- 第一个参数：`rattler.exe` 的**完整路径**。
- `--config "C:\Rattler\config.yaml"`：Rattler 使用的配置文件**完整路径**（必须与程序中 `--config` 一致）。

若 Rattler 工作目录需固定在 `C:\Rattler`（便于相对路径、日志等），可再设置：

```cmd
nssm set RattlerService AppDirectory "C:\Rattler"
```

### 3.3 NSSM 服务管理命令

以下命令均需在**管理员** CMD/PowerShell 中执行。若 `nssm.exe` 在 Rattler 部署主目录（如 `C:\Rattler`），请先 `cd C:\Rattler`，再使用 `.\nssm.exe`；若已加入 PATH 可直接使用 `nssm`。

| 操作               | 命令                                 |
| ------------------ | ------------------------------------ |
| 启动服务           | `nssm start RattlerService`          |
| 停止服务           | `nssm stop RattlerService`           |
| 重启服务           | `nssm restart RattlerService`        |
| 查看状态           | `nssm status RattlerService`         |
| 删除服务（需确认） | `nssm remove RattlerService confirm` |

也可使用 Windows 自带命令：

```cmd
net start RattlerService
net stop RattlerService
sc query RattlerService
```

### 3.4 使用 NSSM GUI 安装/修改服务

1. 以管理员身份打开 CMD/PowerShell，进入 Rattler 部署主目录（如 `cd C:\Rattler`），执行：`.\nssm.exe install RattlerService`（新建服务）或 `.\nssm.exe edit RattlerService`（修改已有服务）。
2. **Application** 标签：
   - **Path**：选择 `rattler.exe`。
   - **Startup directory**：设为 Rattler 程序目录，如 `C:\Rattler`。
   - **Arguments**：填写 `--config "C:\Rattler\config.yaml"`（路径与真实配置一致）。
3. 如需将标准输出/错误写入文件，可在 **I/O** 中设置。
4. 确认后安装或保存，再在服务管理器中启动/停止。

### 3.5 开机自启

NSSM 安装的服务默认启动类型为「自动」，可在「服务」中确认：

- `services.msc` → 找到 `RattlerService` → 属性 → 启动类型：自动。

---

## 四、使用 Caddy 将税金单目录部署为静态文件服务

Rattler 自带 HTTP 接口可下载税金单；若需要**固定 URL、域名或外网访问**，可将税金单所在目录用 **Caddy** 以静态站点方式对外提供，由 Caddy 直接读文件，减轻 Rattler 压力。

### 4.1 税金单目录与双根逻辑（按日期 + 不按日期）

**业务背景（ASL 等）**：存在大量历史税金文件在**原路径**（平铺目录），无法一次性全部迁移到按年月的备份路径。因此 Caddy 需**同时支持**两种访问方式：

| 访问方式     | URL 示例          | 文件实际位置                        |
| ------------ | ----------------- | ----------------------------------- |
| **按日期**   | `/202505_xxx.pdf` | `backup-dir/2025/05/202505_xxx.pdf` |
| **不按日期** | `/old_file.pdf`   | 原路径根目录下的 `old_file.pdf`     |

- **按日期根**：对应 config 中 `watchers.pdf.*.backup-dir`，Rattler 将新税单按 `YYYY/MM/YYYYMM_xxx.pdf` 归档到此。
- **不按日期根（原路径）**：对应 config 中 `storage.*.tax-bill` 或 `watchers.pdf.*.watch-dir`，历史平铺文件及（当 `keep-original: true` 时）新文件均在此目录。

**keep-original 与 Caddy 原路径**（config 中 `watchers.pdf.nl/be.keep-original`）：

- `keep-original: true`：新税单在监听目录**保留原文件**，并复制到 backup-dir。原路径（watch-dir / storage.tax-bill）既有历史文件也有新文件，适合作为 Caddy 的「不按日期」根目录。
- `keep-original: false`：新税单**移动**到 backup-dir，原路径仅剩历史文件。若将原路径配置为 Caddy 默认根，则历史文件仍可通过「不按日期」URL 访问，新文件通过「按日期」URL 在 backup 下访问。

配置 Caddy 时，**默认根**应填 config 中对外提供税金单的**原路径**（即 `storage.*.tax-bill`，通常与 watch-dir 一致或为业务指定目录），**按日期**请求仍从 backup-dir 提供。

### 4.2 安装 Caddy（Windows）

1. **下载**
   - 打开 Caddy 官方 GitHub Releases：[https://github.com/caddyserver/caddy/releases](https://github.com/caddyserver/caddy/releases)。
   - 选择**最新稳定版本**（页面中标记为 **Latest** 的 release，如 v2.10.2），下载 Windows 64 位对应资产（如 `caddy_2.10.2_windows_amd64.zip`）。
2. **解压并放置**
   - 解压后将 `caddy.exe` 放到固定目录（如 `C:\Caddy\` 或与 Caddyfile 同目录），可按需加入系统 PATH 便于命令行调用。

### 4.3 Caddyfile 示例（Windows 路径，双根）

项目内 **Caddyfile-win** 已实现「按日期 + 不按日期」双根，路径需按本机 config 中 `storage.*.tax-bill`、`watchers.pdf.*.backup-dir` 等修改。

下面示例中：

- **按日期根**：NL/BE 均为 `C:/RattlerSoftpak/backup/pdf/nl`、`be`（backup-dir）。
- **不按日期根（原路径）**：NL 为 `C:/SP/cust/NCC/pdf`，BE 为 `C:/RattlerSoftpak/interface/taxbill`，与 config 中 `storage.tax-bill` / watch-dir 对应，供历史平铺文件及（当 keep-original 为 true 时）新文件访问。

```caddyfile
{
	admin off
}

# NL：按日期 → backup；不按日期 → 原路径
nl.tax.local:80 {
	log {
		output file C:/RattlerSoftpak/log/caddy/nl-access.log
		format json
		level INFO
	}
	@dated_files {
		path_regexp dated_pattern ^/(20[0-9]{2})(0[1-9]|1[0-2])_(.*)$
	}
	handle @dated_files {
		root * C:/RattlerSoftpak/backup/pdf/nl
		rewrite * /{http.regexp.dated_pattern.1}/{http.regexp.dated_pattern.2}/{http.regexp.dated_pattern.1}{http.regexp.dated_pattern.2}_{http.regexp.dated_pattern.3}
		file_server
	}
	handle {
		root * C:/SP/cust/NCC/pdf
		file_server
	}
	header -Server
}

# BE：按日期 → backup；不按日期 → 原路径
be.tax.local:80 {
	log {
		output file C:/RattlerSoftpak/log/caddy/be-access.log
		format json
		level INFO
	}
	@dated_files {
		path_regexp dated_pattern ^/(20[0-9]{2})(0[1-9]|1[0-2])_(.*)$
	}
	handle @dated_files {
		root * C:/RattlerSoftpak/backup/pdf/be
		rewrite * /{http.regexp.dated_pattern.1}/{http.regexp.dated_pattern.2}/{http.regexp.dated_pattern.1}{http.regexp.dated_pattern.2}_{http.regexp.dated_pattern.3}
		file_server
	}
	handle {
		root * C:/RattlerSoftpak/interface/taxbill
		file_server
	}
	header -Server
}
```

说明：

- 域名/端口：在本机 hosts 或内网 DNS 将 `nl.tax.local`、`be.tax.local` 指到本机，或改用 `:8080` 等端口。
- **按日期**：`/202505_xxx.pdf` 在对应站点的 backup 根下查找 `2025/05/202505_xxx.pdf`。
- **不按日期**：`/任意文件名.pdf` 在对应站点的**原路径根**下直接查找，与 config 中 `storage.*.tax-bill` 一致，便于 ASL 历史文件访问。

### 4.4 运行 Caddy

前台测试：

```cmd
cd C:\Caddy
caddy run --config Caddyfile
```

或使用项目中的 **Caddyfile-win**（先修改其中路径）：

```cmd
caddy run --config "C:\Rattler\Caddyfile-win"
```

确认能访问 `http://nl.tax.local/YYYYMM_xxx.pdf` 等后再部署为服务。

### 4.5 将 Caddy 安装为 Windows 服务（NSSM）

与 Rattler 类似，用 NSSM 将 Caddy 注册为服务，便于开机自启和统一管理：

```cmd
nssm install CaddyTax "C:\Caddy\caddy.exe" run --config "C:\Caddy\Caddyfile"
nssm set CaddyTax AppDirectory "C:\Caddy"
nssm start CaddyTax
```

管理命令：

| 操作     | 命令                           |
| -------- | ------------------------------ |
| 启动     | `nssm start CaddyTax`          |
| 停止     | `nssm stop CaddyTax`           |
| 重启     | `nssm restart CaddyTax`        |
| 删除服务 | `nssm remove CaddyTax confirm` |

---

## 五、部署顺序与检查清单

1. 安装并配置 RabbitMQ，确认 Rattler 所用账号有对应队列与交换机权限。
2. 创建 Rattler 所需目录（日志、临时、监听、备份、import 等）。
3. 编写并放置 `config.yaml`，用 `rattler.exe --config config.yaml` 前台测试。
4. 使用 NSSM 安装 Rattler 服务并启动，确认服务状态与日志。
5. （可选）按实际目录修改 Caddyfile，测试 Caddy 静态访问税金单。
6. （可选）使用 NSSM 将 Caddy 安装为服务并设置开机自启。

---

## 六、常见问题

- **Rattler 服务启动失败**
  - 检查 `rattler.exe` 与配置文件路径是否正确、配置文件 YAML 格式是否合法。
  - 在 NSSM 中查看「标准输出/标准错误」或 Rattler 日志目录下的日志。

- **无法连接 RabbitMQ**
  - 检查 `rabbitmq.url`、防火墙、RabbitMQ 服务是否运行；密码含特殊字符时是否用引号包裹并正确转义。

- **Caddy 访问 404**
  - **按日期**请求：确认按日期 handle 的 `root *` 指向 backup-dir，且存在 `YYYY/MM/YYYYMM_xxx.pdf`；URL 为 `YYYYMM_` 前缀。
  - **不按日期**请求：确认默认 handle 的 `root *` 指向原路径（与 config `storage.*.tax-bill` 一致），且该目录下存在对应文件名；NL/BE 域名或端口解析到本机。

- **NSSM 命令找不到**
  - 建议将 `nssm.exe` 放在 Rattler 部署主目录（如 `C:\Rattler\`），在该目录下以管理员身份打开 CMD/PowerShell，使用 `.\nssm.exe` 执行；或使用 `nssm.exe` 的完整路径（如 `C:\Rattler\nssm.exe`）。

更多配置项说明见 [README.md](README.md)。

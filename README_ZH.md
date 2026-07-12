# styx-mcp

[English](README.md) | **简体中文**

面向 MCP 的多级代理，灵感来自 [Stowaway](https://github.com/ph4ntonn/Stowaway)。

把一棵 **agent** 节点树暴露成 LLM 客户端（Cursor、Claude Desktop 等）可直接调用的工具：查看拓扑、开 SOCKS5、端口转发、上传文件——无需单独的 admin 交互界面。

## 免责声明

**仅限授权使用。** 只能在你拥有或已获书面授权的系统/网络上使用（自有实验室、CTF/考试靶场、明确 RoE 的渗透项目）。对未授权目标使用可能违法。作者不对滥用行为负责。

## 功能

| 领域 | 能力 |
| :--- | :--- |
| 拓扑 | agent 树形组网；主动连接（`-c`）或被动监听（`-l`） |
| 认证 | 预共享密钥 + 双向 HMAC challenge-response |
| 传输 | 原始 TCP；可选 TLS；可选 WebSocket（`raw` / `ws`） |
| SOCKS5 | 在 **controller** 上监听；流量经指定节点出站（Stowaway 风格） |
| Forward | 在 **agent** 上监听，转发到该节点可达的目标 |
| Backward | 在 **controller** 上监听，经节点访问内网目标 |
| 文件 | 本机文件上传到节点路径（路径已做穿越清理） |
| MCP | stdio MCP 服务；异步任务用 `get_task_status` 查询 |
| 构建 | 交叉编译：`linux-amd64`、`windows-amd64`、`darwin-arm64` |

**暂未包含：** 交互式远程 shell、从节点下载文件、SOCKS 认证、完整 Stowaway admin UI。

## 架构

```text
LLM 客户端 (Cursor / Claude …)
        │  MCP over stdio
        ▼
   controller  ──listen──►  agent (node 0)
        │                      │
   SOCKS / backward         start_listener / connect_node
   在本机监听                   ▼
                            agent (node 1) …
```

- **controller**：控制面 + MCP 工具。agent 连到这里。SOCKS / 反向转发的监听绑在 controller 所在主机。
- **agent**：跑在跳板/落脚点；主动连出或被动听子节点；在边缘执行 SOCKS dial / 转发 / 上传。

## 编译

需要 Go 1.21+（具体版本见 `go.mod`）。

```bash
# 当前平台（Makefile 含 darwin-arm64 目标）
make build

# 全部发布目标
make build-all

make test
```

产物在 `release/<os>-<arch>/`（`controller`、`agent`）。该目录已 gitignore；本地编译，或在 GitHub Releases 挂二进制。

## 接入 Cursor MCP

1. 先编译，保证存在 `release/<os>-<arch>/controller`。
2. 写入 `~/.cursor/mcp.json`（或项目 `.cursor/mcp.json`）：

```json
{
  "mcpServers": {
    "styx-mcp": {
      "command": "/absolute/path/to/styx-mcp/scripts/styx-mcp-wrapper.sh",
      "env": {
        "STYX_SECRET": "change-me",
        "STYX_LISTEN": "0.0.0.0:19137",
        "STYX_LOG": "/tmp/styx-mcp-controller.log"
      }
    }
  }
}
```

3. Cursor Settings → MCP → 启用 / 刷新 **styx-mcp**。
4. 在落脚点启动同密钥的 **agent**：

```bash
./agent -s change-me -c <controller可达IP>:19137
```

Wrapper 环境变量：

| 变量 | 默认 | 含义 |
| :--- | :--- | :--- |
| `STYX_SECRET` | `secret` | 预共享密钥（`-s`） |
| `STYX_LISTEN` | `0.0.0.0:19137` | controller 等待 agent 的地址（`-l`） |
| `STYX_LOG` | `/tmp/styx-mcp-controller.log` | controller 日志 |
| `STYX_BIN_DIR` | 仓库下 `release/<os>-<arch>` | 覆盖二进制目录 |

**不要把真实密钥** 写进公开仓库或共享的 `mcp.json` 示例。

### 不用 wrapper 时手动起 controller

controller 的 **stdio** 要留给 MCP。在 Cursor 下请优先用 wrapper。调试可：

```bash
./release/darwin-arm64/controller -s change-me -l 0.0.0.0:19137
```

日志默认 `/tmp/styx-mcp-controller.log`（视构建而定，MCP 流量也可能写到 `/tmp/styx-mcp-mcp.log`）。

## 快速冒烟（纯 CLI）

```bash
# 终端 A — 若已用 Cursor MCP wrapper 可省略
./release/darwin-arm64/controller -s change-me -l 127.0.0.1:19137

# 终端 B
./release/darwin-arm64/agent -s change-me -c 127.0.0.1:19137
```

在 MCP 客户端执行 `list_nodes`，应能看到 node `0`。

## MCP 工具

| 工具 | 作用 | 监听 / 作用位置 |
| :--- | :--- | :--- |
| `list_nodes` | 查看拓扑 | — |
| `get_node_detail` | 节点详情 | — |
| `add_node_memo` / `delete_node_memo` | 节点备注 | — |
| `start_listener` | 节点监听子 agent | **Agent** |
| `connect_node` | 节点主动连接子 agent | **Agent** 出站 |
| `start_socks` | 给本机工具用的 SOCKS5 | **Controller** 监听；经 `node_id` 出站 |
| `start_forward` | 节点本地转发 | **Agent** 监听 → dial `target_address` |
| `start_backward` | 反向转发 | **Controller** 监听 → 经节点 → 目标 |
| `upload_file` | 上传文件到节点 | Controller → agent 路径 |
| `get_task_status` | 查询异步任务 | — |
| `shutdown_node` | 关闭节点 | — |

耗时操作会返回 `task_id`，用 `get_task_status` 轮询。

### SOCKS / forward / backward 怎么选

| 目标 | 用 |
| :--- | :--- |
| 在 **controller 本机** 用 `curl`/扫描器打内网 | `start_socks` |
| 把 **controller** 上某个端口映射到内网 `ip:port` | `start_backward` |
| 在 **落脚点本机** 开端口再转到别处 | `start_forward` |

`start_forward` **不能** 当成给本机用的 SOCKS。

## 示例

### 两级拓扑

子节点被动监听：

```bash
./agent -s change-me -l 127.0.0.1:19138
```

MCP 调用：

```json
{
  "name": "connect_node",
  "arguments": { "node_id": 0, "address": "127.0.0.1:19138" }
}
```

也可以让 node 0 监听、子节点向上连——最终 `list_nodes` 都能看到父子关系。

### SOCKS5

```json
{
  "name": "start_socks",
  "arguments": { "node_id": 0, "address": "127.0.0.1:10801" }
}
```

```bash
curl --socks5-hostname 127.0.0.1:10801 http://<内网主机>/
# 或: ALL_PROXY=socks5h://127.0.0.1:10801
```

### 端口转发（在 agent 上）

```json
{
  "name": "start_forward",
  "arguments": {
    "node_id": 0,
    "listen_address": "127.0.0.1:19141",
    "target_address": "10.0.0.5:80"
  }
}
```

连的是该 **agent 主机** 上的 `listen_address`。

### 反向端口转发（在 controller 上）

```json
{
  "name": "start_backward",
  "arguments": {
    "node_id": 0,
    "local_address": "127.0.0.1:19142",
    "target_address": "10.0.0.5:80"
  }
}
```

连 **controller 主机** 上的 `127.0.0.1:19142`。

### 文件上传

```json
{
  "name": "upload_file",
  "arguments": {
    "node_id": 0,
    "local_path": "/path/to/tool",
    "remote_path": "/tmp/tool"
  }
}
```

## 命令行参数

### controller

| 参数 | 说明 |
| :--- | :--- |
| `-s` | 预共享密钥（实际使用必填） |
| `-l` | 等待 agent：`[ip]:port` |
| `-c` | 可选，主动连接对端 |
| `-down` | 下游传输：`raw` / `ws` |
| `-tls-enable` | 节点链路启用 TLS |
| `-domain` | TLS SNI / WebSocket domain |
| `-heartbeat` | 对首节点心跳 |

### agent

| 参数 | 说明 |
| :--- | :--- |
| `-s` | 预共享密钥 |
| `-c` | 主动模式：连接 controller/父节点 |
| `-l` | 被动模式：监听 |
| `-up` / `-down` | 上下游：`raw` / `ws` |
| `-tls-enable` | TLS |
| `-domain` | TLS SNI / WebSocket domain |
| `-reconnect` | 重连间隔秒（`0` = 关闭） |
| `-socks5-proxy` / `-socks5-proxyu` / `-socks5-proxyp` | 经 SOCKS5 连父节点 |
| `-http-proxy` | 经 HTTP 代理连父节点 |

controller 与 agent 必须使用 **相同密钥**（启用 TLS/WS 时配置也要一致）。

## 目录结构

```text
cmd/controller/     controller + MCP 入口
cmd/agent/          agent 入口
scripts/            Cursor MCP wrapper
pkg/controller/     控制面、SOCKS/backward
pkg/mcp/            MCP 工具注册与处理
pkg/node/           agent 协议处理
pkg/protocol/       线协议
pkg/share/preauth/  HMAC 双向预认证
pkg/crypto/         AES + gzip
pkg/tasks/          异步任务
pkg/topology/       节点图
pkg/transport/      TLS 辅助
```

## 安全建议

- 把 `-s` / `STYX_SECRET` 当密码保管；共享演练后轮换。
- SOCKS 默认绑 `127.0.0.1`，除非你有意对外暴露。
- 上传路径已防穿越，仍只应上传到你控制的主机。
- 若构建开启了 MCP stdio 日志，工具参数可能被记录——开日志时不要把密钥填进工具字段。

## 致谢

多级代理与协议思路大量参考 [Stowaway](https://github.com/ph4ntonn/Stowaway)（MIT，© ph4ntom）。styx-mcp 增加了 MCP 控制面，以及面向 LLM 客户端的 controller 侧 SOCKS 语义。

## 许可证

MIT — 见 [`LICENSE`](LICENSE)。

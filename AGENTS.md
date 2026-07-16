# styx-mcp — project agent rules

MCP-native multi-hop proxy (controller + agent). Multi-hop ideas inspired by [Stowaway](https://github.com/ph4ntonn/Stowaway); identity, crypto, and control plane are **styx-mcp’s own**. Keep attribution links; don’t reintroduce legacy handshake strings or dead compat stubs without reason.

**Authorized use only** (labs, CTF/exam ranges, RoE-covered work).

---

## Core principle: test what you changed

**开发/修复哪块能力，就验证哪块能力** — 包括正常路径和边界/失败路径。  
不要用「固定全链路清单」替代针对性验证；也不要只跑无关的通用烟测就声称修完了。

| 你改了什么 | 至少应证明什么 |
|------------|----------------|
| 拓扑 / `list_nodes` / memo / offline | 上线、列表、详情、memo 增删、**下线后稀疏 ID**、并发查询不串结果 |
| listen / bind / wrapper 启动 | 成功监听；**端口占用/无权限时进程失败且错误可读** |
| 握手 / 密钥 / 协议 framing | **新旧不兼容时行为明确**；同版本 agent 能上线；畸形包不 panic |
| SOCKS / flow control | 流建立、数据往返、FIN/断流；必要时窗口阻塞与 ACK |
| forward / backward | 监听侧与 dial 侧正确；多会话不串（seq / node） |
| exec / 文件上下传 | 成功、超时、截断、路径非法/过大拒绝 |
| 纯文档 / AGENTS / 注释 | 无需靶机；确认无代码行为变化即可 |

**深度与改动面成正比：** 小修复可以单测 + 窄场景；动数据面/控制面则要实网或等价集成；边界（空、并发、失败、超限）至少覆盖你引入或触碰的那几类。

全链路跳板（入口 RCE → agent → SOCKS → 内网服务）是**可选的回归手段**，适合大改发布前或用户要求「靶机验收」时用，**不是每次改拓扑都只拿 SOCKS 冒充拓扑测过**。

---

## Workflow

```text
1. Scope      — 只做要求的事；发现旁路问题先记下来，不擅自膨胀
2. Implement  — 最小正确改动；硬错误要可见（别静默假健康）
3. Verify     — 针对本改动的功能测试 + 边界测试（见上表）
4. Suite      — go test ./...（能加单测的逻辑就加）
5. Commit     — 验证通过再提交；失败继续修
6. Push       — 仅当用户明确要求
```

### Scope

| Do | Don't |
|----|--------|
| 与缺陷/功能相关的文件 | 顺手大重构、无关美化 |
| 一个主题一个 commit（或紧密相关的一小批） | 把无关 P0/P2/文档搅在一起 |
| wire/兼容破坏写进 commit message | 默默弄坏 controller↔agent 配对 |

### Quality bar

- 简单清晰优先于炫技。
- **失败可见**：例如 listen 绑定失败不得让 MCP 空拓扑却显示正常 — 非 0 退出或明确报错。
- **对端/MCP 输入不 panic**：类型断言 comma-ok；记日志并跳过/返回错误。
- 错误带上下文：`fmt.Errorf("…: %w", err)`。
- 密钥不入库：`.grok/styx.secret`、真实 `STYX_SECRET`；模板见 `.grok/config.toml.example`。

### Wire / versioning

- 握手、加解密、framing 变更时，controller 与 agent **同 commit 构建**。
- 身份常量在 `pkg/protocol`（`ControllerUUID`、`JoinUUID`、`HelloFromAgent`、`HelloFromController` 等）。
- 消息类型数值尽量 **append-only**，保持既有 wire ID 稳定。

---

## How to verify (guidance, not a rigid script)

### Always (code changes)

```bash
go test ./...
go build ./...   # shipping agents: make build / make build-all
```

### Match tests to the change

- **Prefer automated tests** for pure logic (topology correlation, path sanitize, length limits, listen error text).
- **Use live MCP + real agent** when the change only shows up across process/network boundaries (memo sync, offline, SOCKS exit node).
- **Boundary ideas** (pick what applies): empty/missing id, concurrent calls, disconnect mid-op, port in use, oversize payload, wrong secret, path traversal, timeout.

### Optional full pivot regression

When doing a broad release check or the user asks for lab e2e:

1. One controller on the agent port; rebuild matching agent.
2. Authorized foothold → agent online → `list_nodes`.
3. Exercise the **features you care about** (topo / socks / exec / …).
4. For SOCKS: show attacker cannot reach internal host directly, but can via proxy.

---

## Git

- 有与本改动对应的验证结论再 commit。
- 风格：`fix:` / `feat:` / `refactor:` / `docs:`；主体写 **why**。
- 勿提交：`release/`、密钥、本地 `.grok/config.toml`。
- 勿 `git push` / 改写已发布历史，除非用户要求。

---

## Layout

| Path | Role |
|------|------|
| `cmd/controller` | Controller + MCP stdio |
| `cmd/agent` | Agent |
| `pkg/mcp` | MCP tools |
| `pkg/controller` | Control plane, SOCKS/backward |
| `pkg/node` | Agent handlers |
| `pkg/protocol` | Wire + identity |
| `pkg/topology` | Node tree (use `Topology.Do`) |
| `scripts/styx-mcp-wrapper.sh` | MCP launcher |
| `.grok/config.toml.example` | MCP template |

## Ops

- 隧道优先 **styx-mcp MCP**（`start_socks` / `start_backward` / `start_forward`），除非用户指定或 MCP 不可用。
- `start_socks`：在 **controller** 听；流量从 `node_id` 出。
- `start_forward`：在 **agent** 听；不是本机 SOCKS 替代品。
- 同时只应有一个 controller 占用 agent 监听端口（Cursor/Grok 勿抢同一端口）。

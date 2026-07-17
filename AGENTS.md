# styx-mcp

MCP 多级跳板：controller 走 stdio MCP，agent 在对端出流量。  
多跳思路致谢 [Stowaway](https://github.com/ph4ntonn/Stowaway)（MIT）；线协议身份、加解密与工具面是本仓库的。仅限授权环境。

## Dev environment tips

- Go 版本见 `go.mod`。仓库根目录：`go test ./...`、`go build ./...`。
- 本机产物：`make build` → `release/<os>-<arch>/`。交叉编译：`make build-all`。
- MCP 入口：`scripts/styx-mcp-wrapper.sh`。配置模板：`.grok/config.toml.example`。
- 密钥：`STYX_SECRET` 或 gitignore 的 `.grok/styx.secret`，不要提交真实密钥。
- agent 默认连 `19137`（`STYX_LISTEN`）。该端口同时只跑一个 controller；占用就换地址或先释放。
- 改握手 / 加密 / 组帧（含 SCAN*）后，controller 与 agent 用**同一 commit** 构建再联调。
- `start_socks` 在 **controller 本机**听，流量经 `node_id` 出；`start_forward` 在 **agent** 上听，不是本机 SOCKS 替代品。
- `start_scan` 在 **agent** 上扫：默认 **混合探活**（ICMP 尽力 + TCP probe，任一成功即 alive），再只扫存活主机；探活 0 台则自动降级扫全部并 `warnings`。`method=auto` 有 raw TCP 用 SYN，否则 connect。`full` 很重。
- 布局：`cmd/{controller,agent}`、`pkg/mcp`、`pkg/controller`、`pkg/node`、`pkg/protocol`、`pkg/scan`、`pkg/fingerprint`、`pkg/topology`（`Topology.Do`）。

## OpenSpec（默认规范）

一切功能、修复、测试补齐都按 OpenSpec 走，不另开一套标准。

**契约**：`openspec/specs/`  
`dev-workflow` · `topology` · `socks-proxy` · `mcp-async-tasks` · `transport` · `intranet-scan` · `file-transfer`  
（新领域用 kebab-case 加目录，不要只写在代码注释里。）

**流程**（fix / feat / 补测 同一套）：

1. **对齐**：先读相关 `openspec/specs/<capability>/spec.md`，标出命中的 Requirement / Scenario；没有就先补场景再写代码。
2. **开 change**：`openspec/changes/<kebab-name>/`  
   - 一律：`proposal.md` + `tasks.md`  
   - 行为/契约有变：`specs/<capability>/spec.md`（ADDED / MODIFIED / …）  
   - 协议、多包、跨进程：再加 `design.md`  
3. **实现与测试**：`tasks.md` 逐项打勾；测试对应 Scenario（单测 / MCP+真 agent）；风险高再加深。  
4. **校验**：`openspec validate --all`，相关包 `go test` 绿。  
5. **收尾**：有 delta 则 `openspec archive <name>` 合回 `specs/`；commit 说明可带 change 名。

**体量**：proposal 可短（Why / What / Capabilities / Non-goals）；tasks 可少，但不能跳过 change。纯文案且不动行为时，仍写一条 proposal 说明「无 requirement 变更」。

**命令**：`openspec list` · `openspec list --specs` · `openspec validate --all` · `openspec archive <name>`

## Testing instructions

- 测试是 OpenSpec Scenario 的落地，不是旁路清单。
- 对 change 涉及的包跑 `go test ./...`，修到绿；**改什么测什么**，深度跟风险走。
- 单测优先覆盖纯逻辑；跨进程用 MCP + 真 agent 对应 async / SOCKS / transport 场景。
- 全链路靶场（入口 → agent → 代理 → 内网）在 transport/socks 相关 change 或用户要求时做。
- 发现行为与 spec 不一致：先改 spec（或确认 bug）再改代码，避免静默漂移。

## PR instructions

- `openspec validate --all` 与相关测试通过再 commit；用户没说就不要 `git push`。
- 说明：`fix:` / `feat:` / `refactor:` / `docs:`，短标题；行为变更写清 why / 对应 capability。
- 不要提交构建产物、密钥、本地配置/日志；清单见 `.gitignore`（模板 `.grok/config.toml.example` 可提交）。

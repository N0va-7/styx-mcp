## Evidence (2026-07-17)

### Lab fingerprint

| Field | Value |
|-------|--------|
| URL | `http://10.7.11.116/` |
| Direct | HTTP 200, title 闻道基金, Apache/PHP 5.5 / EyouCMS |
| Open ports | 22, 80 |

### MCP path (existing Grok controller)

1. Controller already on `0.0.0.0:19137` (secret matched running process).
2. Local agent joined → `list_nodes` node_id **0** (`uuid=29bd806599`).
3. `start_socks` node 0 → `127.0.0.1:11080` → task `start_socks-1` **ready:true**.
4. `curl --socks5-hostname 127.0.0.1:11080 http://10.7.11.116/`
   - HTTP **200**, title **闻道基金**, body size **13495** (match direct).

### Automated tests

```text
go test ./pkg/controller/ -run 'TestE2E' -count=1
  TestE2ESOCKSLocalAgentHTTP  PASS  (loopback HTTP via agent)
  TestE2ESOCKSLabHTTP         PASS  (default lab 10.7.11.116)
```

Optional override: `STYX_LAB_URL=http://host/`.

### Helper

`scripts/lab-e2e-socks.sh` — baseline + curl via existing SOCKS (MCP start_socks separate).

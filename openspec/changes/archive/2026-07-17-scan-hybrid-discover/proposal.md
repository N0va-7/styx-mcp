## Why

TCP-only discovery misses hosts that answer ICMP but not default probe ports.
ICMP-only (fscan default) misses hosts that block ping. Hybrid is more robust.

## What Changes

- Discover: **alive = ICMP success OR any TCP probe open**
- Thicker default TCP probe set (high-value subset)
- If discover finds **zero** alive hosts: **auto-fallback** to full target list + warning in stats
- ICMP best-effort (no hard fail if ping unavailable)

## Capabilities

### Modified
- `intranet-scan`: hybrid discover + zero-alive fallback

## Non-goals
- Require root for ICMP
- Replace operator `discover=false`

## ADDED Requirements

### Requirement: SOCKS tolerates client half-close
When a local SOCKS client finishes writing (TCP half-close / EOF on read) while
still reading the response, the controller SHALL continue delivering data from
the agent until the remote stream finishes or an error occurs, rather than
immediately tearing down the tunnel.

#### Scenario: HTTP response after request write-close
- **WHEN** a client writes a full HTTP request and closes its write side while
  waiting for the response through SOCKS
- **THEN** the response body from the far end is still delivered to the client
  (subject to network success)

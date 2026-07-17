## ADDED Requirements

### Requirement: Chunked file transfer
Upload and download of files larger than a single protocol-friendly slice SHALL
be split into multiple FileData messages with slice index/total. The receiver
SHALL reassemble in order before completing the task.

#### Scenario: Multi-slice upload completes
- **WHEN** a local file larger than one chunk is uploaded to an agent
- **THEN** the agent writes the full file and the async task completes with the
  total byte count

#### Scenario: Multi-slice download completes
- **WHEN** a remote file larger than one chunk is pulled to the controller
- **THEN** the local file matches the remote size and the task reports bytes (and
  hash when computed)

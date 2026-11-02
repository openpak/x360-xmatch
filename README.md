# Xbox 360 Matchmaking Server

OpenPak's Xbox 360 matchmaking server (Live service 6, `xmatch`).

| | |
| --- | --- |
| Network | Xbox 360 |
| Role | Game session advertise / search / delete (`XSessionCreate`, `XSessionModify`, `XSessionSearch`, `XSessionDelete` on matchmaking sessions) |
| Depends on | [`x360-common`](https://github.com/openpak/x360-common) (shared Go library, vendored) |
| Image | `ghcr.io/openpak/x360-xmatch` |
| Used by | XAM through the security gateway |
| Licence | AGPL-3.0-only |

## What it serves

| Path | XRL | What |
| --- | --- | --- |
| `POST /xmatch/xmatchhost2.ashx` | 0x100, 0x101 | Create a session (reply: session id + key) or modify it (no reply body) |
| `POST /xmatch/xmatchhostdelete2.ashx` | 0x102 | Delete a session |
| `POST /xmatch/xmatchclient2.ashx` | 0x104 | Search a title's open sessions |
| `POST /xmatch/xmatchsplit.ashx` | 0x105 | Open a new session from an existing one |
| `GET /healthz` | | Health probe |

Every request needs the gateway's console account header (`X-X360-Account`, trusted only with
`X360_GATEWAY_KEY`). Only the console that created a session may modify or delete it.

Sessions live in memory and expire after two idle hours. Live ran per-title search procedures
on the server; OpenPak has none, so a search returns the title's sessions with a free public
slot whose attributes equal every search parameter they share.

## Run

```sh
GOWORK=off go run -mod=vendor ./cmd/x360-xmatch
```

## Docs

- [Protocol](docs/protocol.md)
- [Deploy](docs/deploy.md)

## Licence

AGPL-3.0-only. See [LICENSE](LICENSE).

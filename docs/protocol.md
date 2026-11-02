# xmatch protocol (XAM 17559)

Bodies are XRL "xon/5": the schema fields back to back, little-endian, no envelope. Requests
are POST. Schema decoding and the codec rules are in the research notes (XRL schema opcodes);
the builders below are the XAM functions that fill each request.

Common pieces:

- `XNKID` 8 bytes. A create carries a fresh id from `FUN_81831ee0`: byte 0 = `0x80 | (n & 7) << 4`,
  bytes 1-7 zero. The server keeps byte 0 and fills the rest.
- `XNADDR` 36 bytes (`ina`, `inaOnline`, `u16 portOnline`, `abEnet[6]`, `abOnline[20]`).
- Attribute list: `u32 n`, `u32 offset[n]`, then `n` x `{u32 id, value}`. The value type is
  `id & 0x00F00000`: `0` = u64, `0x100000` = `u16 len` + UTF-8 (NUL included), `0x200000` =
  `u16 len` + bytes; any other type has no value bytes. Offsets count from the start of the
  record (`FUN_81831d00`: header 0x48 for host, 0x38 for search, 0x40 for split).
- Attribute ids are XUSER property/context ids; XAM adds its own (`0x10008101`, `0x50008104`,
  `0x30008105`, `0x30008106`, `0x20008107`, `0x40108109`, `FUN_819149c8`).

## 0x100 / 0x101 host (`xmatchhost2.ashx`, builder `FUN_81826748`, `FUN_81827298`)

`u32 length`, `XNKID`, `u32 title id` (logon value of service 6), host `XNADDR`,
`u32 public open`, `u32 private open`, `u32 public filled`, `u32 private filled`, attributes.

Slot counts come from the session object (`FUN_81917838`: open = max - filled; +0x64/+0x70
public, +0x68/+0x6c private). 0x100 reply: `XNKID` + `XNKEY` (16). 0x101 has no reply schema.

## 0x102 delete (`xmatchhostdelete2.ashx`, `FUN_818269e8`)

`u32`, `XNKID`, `u32`. No reply.

## 0x104 search (`xmatchclient2.ashx`, `FUN_81826d10`, called before `FUN_81918058`)

`u32 length`, `u32 title id`, `u32 procedure index`, client `XNADDR`, `u16 users`, `u16 flags`,
parameters (attribute list), then a second list whose entries carry an extra `u32` before the
id. The result count asked by the title is not sent; XAM trims the reply itself into a
0xAEAC-byte buffer.

Reply: `u32 length`, `u16 n`, `u16 flags`, `u32 logging threshold`, then `n` records:
`u32 record length`, `XNKID`, host `XNADDR`, `XNKEY`, the four slot counts, attributes. The
record array header is 0x66: **each record is padded to 4 bytes on the wire**.

## 0x105 split (`xmatchsplit.ashx`, `FUN_81827018`)

`u32 length`, new `XNKID` (fresh, as for a create), existing `XNKID`, `u32 title id`, `XNADDR`,
attributes. Reply: one record as above, unpadded.

## Derivation

Schemas of 0x100-0x105 were dumped from the 17559 XRL table, with field order matched to the XAM builders. Bodies built by the tests decode field for field under the XAM codec model (requests 0x100/0x104, replies 0x100/0x104/0x105); the codec requires 4-byte record alignment.

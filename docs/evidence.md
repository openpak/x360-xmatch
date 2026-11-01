# Evidence

- 2026-11-01, static: schemas of 0x100-0x105 dumped from the 17559 XRL table, field order
  matched to the XAM builders (see protocol.md). Bodies built by this service's tests decode
  field for field under the XAM codec model (requests 0x100/0x104, replies 0x100/0x104/0x105);
  the codec caught the 4-byte record alignment.
- Not yet seen from a console: no title has sent service 6 traffic to OpenPak. Unverified: the
  meaning of the second search list, whether search parameter ids equal session attribute ids,
  and what 0x105 is used for.

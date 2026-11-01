package xmatch

import (
	"bytes"
	"encoding/binary"
	"net/http/httptest"
	"testing"

	"github.com/openpak/x360-common/wire"
)

// hostBody builds an 0x100/0x101 body: one int attribute and one string attribute.
func hostBody(id [8]byte, title uint32, open uint32) []byte {
	var w writer
	w.u32(0)
	w.raw(id[:])
	w.u32(title)
	w.raw(make([]byte, 36))
	w.u32(open)
	w.u32(0)
	w.u32(1)
	w.u32(0)
	w.attrs(len(w), []Attr{
		{ID: 0x1000800B, Raw: binary.LittleEndian.AppendUint64(nil, 3)}, // game mode 3
		{ID: 0x40108109, Raw: append([]byte{3, 0}, "ab\x00"...)},
	})
	b := []byte(w)
	binary.LittleEndian.PutUint32(b, uint32(len(b)))
	return b
}

func searchBody(title uint32, mode uint64) []byte {
	var w writer
	w.u32(0)
	w.u32(title)
	w.u32(0)
	w.raw(make([]byte, 36))
	w.u16(1)
	w.u16(0)
	w.attrs(len(w), []Attr{{ID: 0x1000800B, Raw: binary.LittleEndian.AppendUint64(nil, mode)}})
	w.u32(0)
	return w
}

func post(t *testing.T, h *Handler, owner, path string, body []byte) ([]byte, int) {
	t.Helper()
	req := httptest.NewRequest("POST", path, bytes.NewReader(body))
	if owner != "" {
		req.Header.Set(wire.AccountHeader, owner)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.Bytes(), rec.Code
}

func TestSessionLifecycle(t *testing.T) {
	h := &Handler{Store: NewStore()}
	if _, c := post(t, h, "", "/xmatch/xmatchhost2.ashx", hostBody([8]byte{0x80}, 1, 4)); c != 403 {
		t.Fatalf("no account: %d", c)
	}
	rep, c := post(t, h, "a@xbox.com", "/xmatch/xmatchhost2.ashx", hostBody([8]byte{0x90}, 0x41560817, 4))
	if c != 200 || len(rep) != 24 || rep[0] != 0x90 {
		t.Fatalf("create: %d %x", c, rep)
	}
	var id [8]byte
	copy(id[:], rep)
	key := rep[8:24]

	// own console does not see its session; another finds it with the key, wrong mode does not.
	if rep, _ := post(t, h, "a@xbox.com", "/xmatch/xmatchclient2.ashx", searchBody(0x41560817, 3)); binary.LittleEndian.Uint16(rep[4:]) != 0 {
		t.Fatalf("self search: %x", rep)
	}
	if rep, _ := post(t, h, "b@xbox.com", "/xmatch/xmatchclient2.ashx", searchBody(0x41560817, 2)); binary.LittleEndian.Uint16(rep[4:]) != 0 {
		t.Fatalf("mode filter: %x", rep)
	}
	rep, c = post(t, h, "b@xbox.com", "/xmatch/xmatchclient2.ashx", searchBody(0x41560817, 3))
	if c != 200 || binary.LittleEndian.Uint16(rep[4:]) != 1 || int(binary.LittleEndian.Uint32(rep)) != len(rep) {
		t.Fatalf("search: %d %x", c, rep)
	}
	r := rep[12:]
	if int(binary.LittleEndian.Uint32(r)) != len(r) || !bytes.Equal(r[4:12], id[:]) || !bytes.Equal(r[48:64], key) ||
		binary.LittleEndian.Uint32(r[64:]) != 4 || binary.LittleEndian.Uint32(r[80:]) != 2 ||
		binary.LittleEndian.Uint32(r[84:]) != 92 {
		t.Fatalf("record: %x", r)
	}

	// modify: no reply body; only the owner may modify or delete.
	if rep, c := post(t, h, "a@xbox.com", "/xmatch/xmatchhost2.ashx", hostBody(id, 0x41560817, 0)); c != 200 || len(rep) != 0 {
		t.Fatalf("modify: %d %x", c, rep)
	}
	if _, c := post(t, h, "b@xbox.com", "/xmatch/xmatchhost2.ashx", hostBody(id, 0x41560817, 4)); c != 403 {
		t.Fatalf("foreign modify: %d", c)
	}
	if rep, _ := post(t, h, "b@xbox.com", "/xmatch/xmatchclient2.ashx", searchBody(0x41560817, 3)); binary.LittleEndian.Uint16(rep[4:]) != 0 {
		t.Fatalf("full session listed: %x", rep)
	}
	del := append(append([]byte{0, 0, 0, 0}, id[:]...), 0, 0, 0, 0)
	if _, c := post(t, h, "b@xbox.com", "/xmatch/xmatchhostdelete2.ashx", del); c != 403 {
		t.Fatalf("foreign delete: %d", c)
	}
	if _, c := post(t, h, "a@xbox.com", "/xmatch/xmatchhostdelete2.ashx", del); c != 200 || len(h.Store.byID) != 0 {
		t.Fatalf("delete: %d %d", c, len(h.Store.byID))
	}
}

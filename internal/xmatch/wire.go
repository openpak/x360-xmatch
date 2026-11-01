package xmatch

import (
	"encoding/binary"
	"errors"
)

// XRL "xon" bodies are the schema fields back to back, little-endian, no padding
// (scratch notes: xrl-schema-opcodes.md). Sizes below are the 17559 schemas of
// requests 0x100-0x105 (Live service 6).

var errShort = errors.New("xmatch: short body")

// Attribute value types, selected by id & 0x00F00000 (XRL value table entry 34).
const (
	attrInt    = 0x000000 // u64
	attrString = 0x100000 // u16 byte length (UTF-8, NUL included) + bytes
	attrBlob   = 0x200000 // u16 length + bytes
	attrMask   = 0xF00000
)

// Attr is one session attribute. Raw is its value exactly as sent, so it is echoed
// back byte for byte and compared by value in searches.
type Attr struct {
	ID  uint32
	Raw []byte
}

func (a Attr) size() int { return 4 + len(a.Raw) }

type reader struct {
	b   []byte
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || len(r.b) < n {
		r.err = errShort
		return make([]byte, n)
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}

func (r *reader) u16() uint16 { return binary.LittleEndian.Uint16(r.take(2)) }
func (r *reader) u32() uint32 { return binary.LittleEndian.Uint32(r.take(4)) }

func (r *reader) kid() (k [8]byte) { copy(k[:], r.take(8)); return }

func (r *reader) xnaddr() (a [36]byte) { copy(a[:], r.take(36)); return }

// attrs reads "u32 n, u32 offsets[n], attr[n]" (offsets are recomputed on output).
// withIndex is the search's second array, which carries an extra u32 before the id.
func (r *reader) attrs(withIndex bool) (out []Attr, idx []uint32) {
	n := r.u32()
	if n > 64 { // the schema's declared maximum (value table entry 37)
		r.err = errors.New("xmatch: too many attributes")
		return
	}
	r.take(4 * int(n))
	for i := uint32(0); i < n && r.err == nil; i++ {
		if withIndex {
			idx = append(idx, r.u32())
		}
		id := r.u32()
		var raw []byte
		switch id & attrMask {
		case attrInt:
			raw = r.take(8)
		case attrString, attrBlob:
			l := r.u16()
			raw = append(binary.LittleEndian.AppendUint16(nil, l), r.take(int(l))...)
		}
		out = append(out, Attr{ID: id, Raw: append([]byte(nil), raw...)})
	}
	return
}

type writer []byte

func (w *writer) u16(v uint16) { *w = binary.LittleEndian.AppendUint16(*w, v) }
func (w *writer) u32(v uint32) { *w = binary.LittleEndian.AppendUint32(*w, v) }
func (w *writer) raw(b []byte) { *w = append(*w, b...) }

// attrs writes "u32 n, u32 offsets[n], attr[n]"; base is the byte offset of this
// count within its record, since XAM measures offsets from the record start
// (FUN_81831d00).
func (w *writer) attrs(base int, as []Attr) {
	w.u32(uint32(len(as)))
	off := base + 4 + 4*len(as)
	for _, a := range as {
		w.u32(uint32(off))
		off += a.size()
	}
	for _, a := range as {
		w.u32(a.ID)
		w.raw(a.Raw)
	}
}

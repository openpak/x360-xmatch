// Package xmatch serves Xbox Live matchmaking (Live service 6): the XRL requests
// XAM sends for XSessionCreate/Modify/Delete/Search on sessions that use
// matchmaking. Field layouts come from the XAM 17559 schemas and builders
// (docs/protocol.md).
package xmatch

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openpak/x360-common/wire"
)

// ContentType is the XRL wire type XAM sends and expects.
const ContentType = "xon/5"

const (
	maxBody       = 16 << 10
	maxResults    = 50
	maxReplyBytes = 0xaeac - 64 // XAM's search reply buffer (FUN_81918058 caller), minus slack
	maxPerOwner   = 16
	// ponytail: in-memory store with an idle TTL; a restart drops every advertised
	// session (hosts re-advertise on their next modify, but under a new key that
	// joiners from before the restart do not have). Persist if that matters.
	idleTTL = 2 * time.Hour
)

// Session is one advertised game session.
type Session struct {
	ID                    [8]byte  // XNKID
	Key                   [16]byte // XNKEY, handed to joiners for the XNet key exchange
	TitleID               uint32
	Host                  [36]byte // host XNADDR
	PubOpen, PrivOpen     uint32
	PubFilled, PrivFilled uint32
	Attrs                 []Attr
	Owner                 string // console account that created it
	Updated               time.Time
}

// Store holds the sessions.
type Store struct {
	mu   sync.Mutex
	byID map[[8]byte]*Session
	now  func() time.Time
}

func NewStore() *Store { return &Store{byID: map[[8]byte]*Session{}, now: time.Now} }

func (s *Store) expire() {
	for id, x := range s.byID {
		if s.now().Sub(x.Updated) > idleTTL {
			delete(s.byID, id)
		}
	}
}

func (s *Store) owned(owner string) int {
	n := 0
	for _, x := range s.byID {
		if x.Owner == owner {
			n++
		}
	}
	return n
}

// Handler routes /xmatch/*.ashx. Wrap it with wire.Guard: every request needs the
// gateway's AccountHeader.
type Handler struct {
	Store *Store
	Log   *slog.Logger
}

func (h *Handler) log() *slog.Logger {
	if h.Log != nil {
		return h.Log
	}
	return slog.Default()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.ToLower(r.URL.Path)
	if path == "/healthz" {
		return
	}
	owner := r.Header.Get(wire.AccountHeader)
	if owner == "" {
		http.Error(w, "no console account", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var reply []byte
	switch path {
	case "/xmatch/xmatchhost2.ashx":
		reply, err = h.host(owner, body)
	case "/xmatch/xmatchhostdelete2.ashx":
		err = h.delete(owner, body)
	case "/xmatch/xmatchclient2.ashx":
		reply, err = h.search(owner, body)
	case "/xmatch/xmatchsplit.ashx":
		reply, err = h.split(owner, body)
	default:
		h.log().Info("unhandled", "path", r.URL.Path, "body", hex.EncodeToString(body))
		http.NotFound(w, r)
		return
	}
	h.log().Info("xmatch", "path", r.URL.Path, "account", owner, "body", hex.EncodeToString(body),
		"reply_len", len(reply), "err", err)
	if err != nil {
		code := http.StatusBadRequest
		if err == errOwner {
			code = http.StatusForbidden
		}
		http.Error(w, err.Error(), code)
		return
	}
	w.Header().Set("Content-Type", ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(reply)))
	w.Write(reply)
}

type httpErr string

func (e httpErr) Error() string { return string(e) }

const (
	errOwner = httpErr("session belongs to another console")
	errLimit = httpErr("too many sessions for this console")
)

func random(b []byte) { rand.Read(b) }

// host is XRL 0x100 (create, reply XNKID + XNKEY) and 0x101 (modify, no reply);
// both post the same layout to xmatchhost2.ashx. A create carries the fresh id
// from FUN_81831ee0: flag byte, seven zero bytes.
func (h *Handler) host(owner string, body []byte) ([]byte, error) {
	rd := &reader{b: body}
	rd.u32() // message length
	id := rd.kid()
	title := rd.u32()
	host := rd.xnaddr()
	po, pr, pf, prf := rd.u32(), rd.u32(), rd.u32(), rd.u32()
	attrs, _ := rd.attrs(false)
	if rd.err != nil {
		return nil, rd.err
	}
	s := h.Store
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	create := bytes.Equal(id[1:], make([]byte, 7))
	x := s.byID[id]
	switch {
	case create:
		if s.owned(owner) >= maxPerOwner {
			return nil, errLimit
		}
		x = &Session{Owner: owner}
		x.ID[0] = id[0]
		for {
			random(x.ID[1:])
			if s.byID[x.ID] == nil {
				break
			}
		}
		random(x.Key[:])
	case x == nil: // modify of a session we lost (restart): re-adopt it under its id
		if s.owned(owner) >= maxPerOwner {
			return nil, errLimit
		}
		x = &Session{ID: id, Owner: owner}
		random(x.Key[:])
	case x.Owner != owner:
		return nil, errOwner
	}
	x.TitleID, x.Host = title, host
	x.PubOpen, x.PrivOpen, x.PubFilled, x.PrivFilled = po, pr, pf, prf
	x.Attrs, x.Updated = attrs, s.now()
	s.byID[x.ID] = x
	if !create {
		return nil, nil
	}
	return append(x.ID[:], x.Key[:]...), nil
}

// delete is XRL 0x102: u32, XNKID, u32.
func (h *Handler) delete(owner string, body []byte) error {
	rd := &reader{b: body}
	rd.u32()
	id := rd.kid()
	if rd.err != nil {
		return rd.err
	}
	s := h.Store
	s.mu.Lock()
	defer s.mu.Unlock()
	if x := s.byID[id]; x != nil {
		if x.Owner != owner {
			return errOwner
		}
		delete(s.byID, id)
	}
	return nil
}

// record is one search result / split reply: u32 length, XNKID, host XNADDR, XNKEY,
// four slot counts, attributes. Search results are an array whose header (0x66)
// aligns each element to 4 bytes on the wire, so pad does that.
func record(x *Session, pad bool) []byte {
	var w writer
	w.u32(0)
	w.raw(x.ID[:])
	w.raw(x.Host[:])
	w.raw(x.Key[:])
	w.u32(x.PubOpen)
	w.u32(x.PrivOpen)
	w.u32(x.PubFilled)
	w.u32(x.PrivFilled)
	w.attrs(len(w), x.Attrs)
	for pad && len(w)%4 != 0 {
		w = append(w, 0)
	}
	b := []byte(w)
	b[0], b[1], b[2], b[3] = byte(len(b)), byte(len(b)>>8), byte(len(b)>>16), byte(len(b)>>24)
	return b
}

// matches: every search parameter whose id the session also carries must be equal.
// ponytail: Live ran per-title stored procedures (dwProcedureIndex) server side;
// OpenPak has none, so this generic equality filter stands in. Add per-title
// rules here when a game needs them.
func matches(x *Session, params []Attr) bool {
	for _, p := range params {
		for _, a := range x.Attrs {
			if a.ID == p.ID && !bytes.Equal(a.Raw, p.Raw) {
				return false
			}
		}
	}
	return true
}

// search is XRL 0x104: u32 length, u32 title, u32 procedure, client XNADDR,
// u16 users, u16 flags, parameters, indexed parameters. XAM trims to the
// caller's dwNumResults itself, so this sends every match it can fit.
func (h *Handler) search(owner string, body []byte) ([]byte, error) {
	rd := &reader{b: body}
	rd.u32()
	title := rd.u32()
	rd.u32() // procedure index
	rd.xnaddr()
	rd.u16()
	rd.u16()
	params, _ := rd.attrs(false)
	rd.attrs(true)
	if rd.err != nil {
		return nil, rd.err
	}
	s := h.Store
	s.mu.Lock()
	var found []*Session
	s.expire()
	for _, x := range s.byID {
		if x.TitleID == title && x.Owner != owner && x.PubOpen > 0 && matches(x, params) {
			found = append(found, x)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Updated.After(found[j].Updated) })
	var recs [][]byte
	total := 12
	for _, x := range found {
		rec := record(x, true)
		if len(recs) == maxResults || total+len(rec) > maxReplyBytes {
			break
		}
		recs = append(recs, rec)
		total += len(rec)
	}
	s.mu.Unlock()
	var w writer
	w.u32(uint32(total))
	w.u16(uint16(len(recs)))
	w.u16(0) // flags
	w.u32(0) // logging threshold
	for _, rec := range recs {
		w.raw(rec)
	}
	return w, nil
}

// split is XRL 0x105: u32 length, new XNKID, existing XNKID, u32 title, XNADDR,
// attributes. It opens a new session from an existing one of the same console
// and replies with the new session's record.
func (h *Handler) split(owner string, body []byte) ([]byte, error) {
	rd := &reader{b: body}
	rd.u32()
	nid := rd.kid()
	old := rd.kid()
	title := rd.u32()
	host := rd.xnaddr()
	attrs, _ := rd.attrs(false)
	if rd.err != nil {
		return nil, rd.err
	}
	s := h.Store
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	if s.owned(owner) >= maxPerOwner {
		return nil, errLimit
	}
	x := &Session{Owner: owner, TitleID: title, Host: host, Attrs: attrs, Updated: s.now()}
	if o := s.byID[old]; o != nil {
		if o.Owner != owner {
			return nil, errOwner
		}
		x.PubOpen, x.PrivOpen, x.PubFilled, x.PrivFilled = o.PubOpen, o.PrivOpen, o.PubFilled, o.PrivFilled
	}
	x.ID[0] = nid[0]
	for {
		random(x.ID[1:])
		if s.byID[x.ID] == nil {
			break
		}
	}
	random(x.Key[:])
	s.byID[x.ID] = x
	return record(x, false), nil
}

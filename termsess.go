package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

// Terminal sessions outlive their WebSocket: a page reload detaches and the next page re-attaches,
// replaying the recent output kept in a bounded ring buffer. Detached sessions are killed after
// termIdleTimeout so forgotten shells do not pile up.

const (
	termRingSize    = 256 << 10 // recent output kept per session
	termMaxSessions = 24
	termMaxClients  = 4  // simultaneous viewers of one session (e.g. two browser tabs)
	termClientQueue = 64 // chunks buffered per viewer before it is dropped as too slow
)

var termIdleTimeout = 30 * time.Minute

// ringBuf keeps the last max bytes written. It grows lazily so idle shells stay small.
type ringBuf struct {
	buf   []byte
	max   int
	start int // index of the oldest byte once the buffer is full
}

func (r *ringBuf) Write(p []byte) {
	if r.max <= 0 {
		return
	}
	if len(p) >= r.max {
		r.buf = append(r.buf[:0], p[len(p)-r.max:]...)
		r.start = 0
		return
	}
	if len(r.buf) < r.max {
		room := r.max - len(r.buf)
		if len(p) <= room {
			r.buf = append(r.buf, p...)
			return
		}
		r.buf = append(r.buf, p[:room]...)
		p = p[room:]
	}
	for len(p) > 0 {
		n := copy(r.buf[r.start:], p)
		p = p[n:]
		r.start = (r.start + n) % r.max
	}
}

// Bytes returns a copy of the buffered output, oldest first.
func (r *ringBuf) Bytes() []byte {
	out := make([]byte, 0, len(r.buf))
	out = append(out, r.buf[r.start:]...)
	return append(out, r.buf[:r.start]...)
}

type termClient struct {
	ch chan []byte // closed when the session ends or the client is dropped
}

type termSession struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`  // local | ssh
	Where   string `json:"where"` // cwd or host
	Created int64  `json:"created"`
	Clients int    `json:"clients"`

	p       ptyLike
	wmu     sync.Mutex // serialises writes/resizes coming from several viewers
	mu      sync.Mutex
	ring    ringBuf
	clients map[*termClient]struct{}
	idle    *time.Timer
	exited  bool
	once    sync.Once
}

type termRegistry struct {
	mu sync.Mutex
	m  map[string]*termSession
}

var termReg = &termRegistry{m: map[string]*termSession{}}

var errTermFull = errors.New("too many terminal sessions are open; close some first")

func randID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *termRegistry) full() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.m) >= termMaxSessions
}

// start registers a session around an already started pty and begins pumping its output.
// Leading bytes (e.g. connection warnings) are put into the replay buffer.
func (r *termRegistry) start(title, kind, where string, p ptyLike, lead []byte) (*termSession, error) {
	s := &termSession{
		ID: randID(8), Title: title, Kind: kind, Where: where, Created: time.Now().Unix(),
		p: p, ring: ringBuf{max: termRingSize}, clients: map[*termClient]struct{}{},
	}
	s.ring.Write(lead)
	r.mu.Lock()
	if len(r.m) >= termMaxSessions {
		r.mu.Unlock()
		return nil, errTermFull
	}
	r.m[s.ID] = s
	r.mu.Unlock()
	s.armIdle()
	go s.pump()
	return s, nil
}

func (r *termRegistry) get(id string) *termSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[id]
}

func (r *termRegistry) remove(id string) {
	r.mu.Lock()
	delete(r.m, id)
	r.mu.Unlock()
}

func (r *termRegistry) list() []termSession {
	r.mu.Lock()
	all := make([]*termSession, 0, len(r.m))
	for _, s := range r.m {
		all = append(all, s)
	}
	r.mu.Unlock()
	out := make([]termSession, 0, len(all))
	for _, s := range all {
		s.mu.Lock()
		out = append(out, termSession{ID: s.ID, Title: s.Title, Kind: s.Kind, Where: s.Where, Created: s.Created, Clients: len(s.clients)})
		s.mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Created != out[j].Created {
			return out[i].Created < out[j].Created
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (r *termRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.m)
}

func (s *termSession) pump() {
	buf := make([]byte, 16*1024)
	for {
		n, err := s.p.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.mu.Lock()
			s.ring.Write(chunk)
			for c := range s.clients {
				select {
				case c.ch <- chunk:
				default: // too slow: drop it, the page re-attaches and gets the replay
					close(c.ch)
					delete(s.clients, c)
					if len(s.clients) == 0 {
						s.armIdleLocked()
					}
				}
			}
			s.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	s.mu.Lock()
	s.exited = true
	for c := range s.clients {
		close(c.ch)
	}
	s.clients = map[*termClient]struct{}{}
	if s.idle != nil {
		s.idle.Stop()
	}
	s.mu.Unlock()
	s.kill()
}

// attach adds a viewer and returns the replay of recent output. Replay and registration happen
// under the same lock as the pump's broadcast, so nothing is lost or duplicated.
func (s *termSession) attach() (*termClient, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exited {
		return nil, nil, errors.New("session has ended")
	}
	if len(s.clients) >= termMaxClients {
		return nil, nil, errors.New("too many viewers attached to this session")
	}
	c := &termClient{ch: make(chan []byte, termClientQueue)}
	s.clients[c] = struct{}{}
	if s.idle != nil {
		s.idle.Stop()
	}
	return c, s.ring.Bytes(), nil
}

func (s *termSession) detach(c *termClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clients[c]; ok {
		delete(s.clients, c)
		close(c.ch)
	}
	if len(s.clients) == 0 && !s.exited {
		s.armIdleLocked()
	}
}

func (s *termSession) armIdle() {
	s.mu.Lock()
	s.armIdleLocked()
	s.mu.Unlock()
}

func (s *termSession) armIdleLocked() {
	if s.idle == nil {
		s.idle = time.AfterFunc(termIdleTimeout, func() {
			s.mu.Lock()
			unused := len(s.clients) == 0
			s.mu.Unlock()
			if unused {
				s.kill()
			}
		})
		return
	}
	s.idle.Reset(termIdleTimeout)
}

func (s *termSession) write(b []byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, err := s.p.Write(b)
	return err
}

func (s *termSession) resize(cols, rows int) {
	if cols <= 0 || rows <= 0 || cols > 1000 || rows > 500 {
		return
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_ = s.p.Resize(cols, rows)
}

// kill closes the pty (ending the process) and forgets the session. Safe to call repeatedly.
func (s *termSession) kill() {
	s.once.Do(func() {
		termReg.remove(s.ID)
		_ = s.p.Close()
	})
}

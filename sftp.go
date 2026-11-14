package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTP file browser. A session is opened per host on demand and closed after sftpIdle without
// use (a timer, no polling). Transfers are streamed; previews and listings are capped.

const (
	sftpMaxSessions = 6
	sftpMaxEntries  = 5000
	sftpPreviewMax  = 512 << 10
	sftpUploadMax   = 4 << 30
)

var sftpIdle = 5 * time.Minute

type sftpSession struct {
	id, host string
	ssh      *ssh.Client
	c        *sftp.Client
	home     string
	created  time.Time

	mu    sync.Mutex
	inUse int
	timer *time.Timer
}

type sftpRegistry struct {
	mu sync.Mutex
	m  map[string]*sftpSession
}

var sftpReg = &sftpRegistry{m: map[string]*sftpSession{}}

func sftpOpen(host, password string) (*sftpSession, error) {
	t, err := resolveSSHProfile(host)
	if err != nil {
		return nil, err
	}
	t.Password = password
	client, err := dialSSH(t, func(string) {})
	if err != nil {
		return nil, err
	}
	sc, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("SFTP is not available on %s: %w", host, err)
	}
	home, err := sc.RealPath(".")
	if err != nil || home == "" {
		home = "/"
	}
	s := &sftpSession{id: randID(8), host: host, ssh: client, c: sc, home: home, created: time.Now()}
	s.timer = time.AfterFunc(sftpIdle, s.idleCheck)
	var evict *sftpSession
	sftpReg.mu.Lock()
	if len(sftpReg.m) >= sftpMaxSessions { // drop the oldest
		for _, o := range sftpReg.m {
			if evict == nil || o.created.Before(evict.created) {
				evict = o
			}
		}
		delete(sftpReg.m, evict.id)
	}
	sftpReg.m[s.id] = s
	sftpReg.mu.Unlock()
	if evict != nil {
		evict.close()
	}
	return s, nil
}

func (s *sftpSession) idleCheck() {
	s.mu.Lock()
	busy := s.inUse > 0
	if busy {
		s.timer.Reset(sftpIdle)
	}
	s.mu.Unlock()
	if !busy {
		sftpReg.drop(s.id)
	}
}

func (s *sftpSession) close() {
	s.timer.Stop()
	s.c.Close()
	s.ssh.Close()
}

func (r *sftpRegistry) drop(id string) {
	r.mu.Lock()
	s := r.m[id]
	delete(r.m, id)
	r.mu.Unlock()
	if s != nil {
		s.close()
	}
}

var errSFTPGone = errors.New("file browser session expired; reconnect")

// acquire returns the session and a release func; the idle timer restarts on release.
func (r *sftpRegistry) acquire(id string) (*sftpSession, func(), error) {
	r.mu.Lock()
	s := r.m[id]
	r.mu.Unlock()
	if s == nil {
		return nil, nil, errSFTPGone
	}
	s.mu.Lock()
	s.inUse++
	s.timer.Stop()
	s.mu.Unlock()
	return s, func() {
		s.mu.Lock()
		s.inUse--
		if s.inUse == 0 {
			s.timer.Reset(sftpIdle)
		}
		s.mu.Unlock()
	}, nil
}

// cleanRemote validates a remote absolute path (SFTP paths are always slash-separated).
func cleanRemote(p string) (string, error) {
	if p == "" || len(p) > 4096 || strings.ContainsRune(p, 0) {
		return "", errors.New("invalid path")
	}
	if !strings.HasPrefix(p, "/") {
		return "", errors.New("path must be absolute")
	}
	return path.Clean(p), nil
}

func validName(n string) bool {
	return n != "" && n != "." && n != ".." && len(n) <= 255 && !strings.ContainsAny(n, "/\x00") && utf8.ValidString(n)
}

type sftpEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
	Dir   bool   `json:"dir"`
	Link  bool   `json:"link"`
	MTime int64  `json:"mtime"`
}

func sftpList(s *sftpSession, dir string) ([]sftpEntry, bool, error) {
	infos, err := s.c.ReadDir(dir)
	if err != nil {
		return nil, false, err
	}
	out := make([]sftpEntry, 0, len(infos))
	for _, fi := range infos {
		e := sftpEntry{Name: fi.Name(), Size: fi.Size(), Mode: fi.Mode().String(), Dir: fi.IsDir(), MTime: fi.ModTime().Unix()}
		if fi.Mode()&os.ModeSymlink != 0 {
			e.Link = true
			if st, err := s.c.Stat(path.Join(dir, fi.Name())); err == nil && st.IsDir() {
				e.Dir = true
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	trunc := len(out) > sftpMaxEntries
	if trunc {
		out = out[:sftpMaxEntries]
	}
	return out, trunc, nil
}

// sftpUpload streams body to dir/name through a temporary file, then renames it into place.
func sftpUpload(s *sftpSession, dir, name string, overwrite bool, body io.Reader) (int64, error) {
	if !validName(name) {
		return 0, errors.New("invalid file name")
	}
	target := path.Join(dir, name)
	if st, err := s.c.Stat(target); err == nil {
		if st.IsDir() {
			return 0, errors.New(name + " is a directory")
		}
		if !overwrite {
			return 0, errExists
		}
	}
	tmp := path.Join(dir, "."+name+".devdeck-upload")
	f, err := s.c.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return 0, err
	}
	n, err := f.ReadFrom(body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = s.c.Remove(tmp)
		return n, err
	}
	if err := s.c.PosixRename(tmp, target); err != nil {
		_ = s.c.Remove(target)
		if err := s.c.Rename(tmp, target); err != nil {
			_ = s.c.Remove(tmp)
			return n, err
		}
	}
	return n, nil
}

var errExists = errors.New("a file with that name already exists")

type sftpPreview struct {
	Text      string `json:"text"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
}

func sftpPreviewFile(s *sftpSession, p string) (sftpPreview, error) {
	st, err := s.c.Stat(p)
	if err != nil {
		return sftpPreview{}, err
	}
	if !st.Mode().IsRegular() {
		return sftpPreview{}, errors.New("not a regular file")
	}
	f, err := s.c.Open(p)
	if err != nil {
		return sftpPreview{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, sftpPreviewMax))
	if err != nil {
		return sftpPreview{}, err
	}
	pv := sftpPreview{Size: st.Size(), Truncated: st.Size() > int64(len(b))}
	head := b
	if len(head) > 8192 {
		head = head[:8192]
	}
	if looksBinary(head) {
		pv.Binary = true
		return pv, nil
	}
	pv.Text = strings.ToValidUTF8(string(b), "�")
	return pv, nil
}

// looksBinary: any NUL byte, or more than 10% invalid UTF-8 sequences.
func looksBinary(b []byte) bool {
	if bytes.IndexByte(b, 0) >= 0 {
		return true
	}
	bad, total := 0, 0
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		if r == utf8.RuneError && n == 1 && len(b) > 3 { // a cut rune at the very end is fine
			bad++
		}
		total++
		b = b[n:]
	}
	return total > 0 && bad*10 > total
}

func sftpDelete(s *sftpSession, p string) error {
	if p == "/" {
		return errors.New("refusing to delete /")
	}
	st, err := s.c.Lstat(p)
	if err != nil {
		return err
	}
	if st.IsDir() {
		if err := s.c.RemoveDirectory(p); err != nil {
			return fmt.Errorf("cannot remove directory (it must be empty): %w", err)
		}
		return nil
	}
	return s.c.Remove(p)
}

func init() {
	addRoutes(func(mux *http.ServeMux) {
		// with runs fn on the session named by ?id= (or the JSON body's id) and maps errors.
		with := func(w http.ResponseWriter, id string, fn func(s *sftpSession) error) {
			s, release, err := sftpReg.acquire(id)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusGone)
				_, _ = io.WriteString(w, `{"error":"`+err.Error()+`","gone":true}`)
				return
			}
			defer release()
			if err := fn(s); err != nil {
				if errors.Is(err, sftp.ErrSSHFxConnectionLost) || errors.Is(err, io.EOF) {
					sftpReg.drop(id)
				}
				code := 400
				if errors.Is(err, errExists) {
					code = http.StatusConflict
				}
				fail(w, code, err)
			}
		}
		mux.HandleFunc("POST /api/sftp/open", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				Host     string `json:"host"`
				Password string `json:"password"`
			}
			if err := decode(r, &b); err != nil {
				fail(w, 400, err)
				return
			}
			s, err := sftpOpen(b.Host, b.Password)
			if err != nil {
				failSSH(w, err)
				return
			}
			writeJSON(w, map[string]string{"id": s.id, "home": s.home, "host": s.host})
		})
		mux.HandleFunc("POST /api/sftp/close", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				ID string `json:"id"`
			}
			_ = decode(r, &b)
			sftpReg.drop(b.ID)
			writeJSON(w, map[string]bool{"ok": true})
		})
		mux.HandleFunc("GET /api/sftp/list", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			with(w, q.Get("id"), func(s *sftpSession) error {
				p := q.Get("path")
				if p == "" {
					p = s.home
				}
				p, err := cleanRemote(p)
				if err != nil {
					return err
				}
				entries, trunc, err := sftpList(s, p)
				if err != nil {
					return err
				}
				writeJSON(w, map[string]any{"path": p, "entries": entries, "truncated": trunc})
				return nil
			})
		})
		mux.HandleFunc("GET /api/sftp/download", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			with(w, q.Get("id"), func(s *sftpSession) error {
				p, err := cleanRemote(q.Get("path"))
				if err != nil {
					return err
				}
				st, err := s.c.Stat(p)
				if err != nil {
					return err
				}
				if !st.Mode().IsRegular() {
					return errors.New("only regular files can be downloaded")
				}
				f, err := s.c.Open(p)
				if err != nil {
					return err
				}
				defer f.Close()
				name := path.Base(p)
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
				w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
				w.Header().Set("X-Content-Type-Options", "nosniff")
				_, _ = io.Copy(w, f) // headers are sent: errors past here just cut the download short
				return nil
			})
		})
		mux.HandleFunc("POST /api/sftp/upload", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			with(w, q.Get("id"), func(s *sftpSession) error {
				dir, err := cleanRemote(q.Get("dir"))
				if err != nil {
					return err
				}
				body := http.MaxBytesReader(w, r.Body, sftpUploadMax)
				n, err := sftpUpload(s, dir, q.Get("name"), q.Get("overwrite") == "1", body)
				if err != nil {
					return err
				}
				writeJSON(w, map[string]any{"ok": true, "size": n})
				return nil
			})
		})
		mux.HandleFunc("GET /api/sftp/preview", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			with(w, q.Get("id"), func(s *sftpSession) error {
				p, err := cleanRemote(q.Get("path"))
				if err != nil {
					return err
				}
				pv, err := sftpPreviewFile(s, p)
				if err != nil {
					return err
				}
				writeJSON(w, pv)
				return nil
			})
		})
		type opReq struct {
			ID   string `json:"id"`
			Path string `json:"path"`
			To   string `json:"to"`
		}
		op := func(name string, fn func(s *sftpSession, b opReq) error) {
			mux.HandleFunc("POST /api/sftp/"+name, func(w http.ResponseWriter, r *http.Request) {
				var b opReq
				if err := decode(r, &b); err != nil {
					fail(w, 400, err)
					return
				}
				with(w, b.ID, func(s *sftpSession) error {
					p, err := cleanRemote(b.Path)
					if err != nil {
						return err
					}
					b.Path = p
					if err := fn(s, b); err != nil {
						return err
					}
					writeJSON(w, map[string]bool{"ok": true})
					return nil
				})
			})
		}
		op("mkdir", func(s *sftpSession, b opReq) error { return s.c.Mkdir(b.Path) })
		op("delete", func(s *sftpSession, b opReq) error { return sftpDelete(s, b.Path) })
		op("rename", func(s *sftpSession, b opReq) error {
			to, err := cleanRemote(b.To)
			if err != nil {
				return err
			}
			if b.Path == "/" || to == "/" {
				return errors.New("invalid rename")
			}
			if _, err := s.c.Lstat(to); err == nil {
				return errExists
			}
			return s.c.Rename(b.Path, to)
		})
	})
}

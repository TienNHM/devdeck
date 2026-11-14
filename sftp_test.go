package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSFTPBrowser(t *testing.T) {
	home := testHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "blob.bin"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	user, priv := newSigner(t)
	keyFile := writeKey(t, home, priv)
	d := startTestSSHD(t, testSSHDOpts{keys: []ssh.PublicKey{user.PublicKey()}, root: root})
	if err := saveConfig(Config{Hosts: []SSHHost{{Name: "box", Host: "127.0.0.1", Port: d.port, User: "me", KeyPath: keyFile}}}); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	for _, f := range routeRegistrars {
		f(mux)
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()
	call := func(method, path string, body io.Reader, out any) int {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, body)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if out != nil {
			if s, ok := out.(*string); ok {
				*s = string(b)
			} else if err := json.Unmarshal(b, out); err != nil {
				t.Fatalf("%s %s: %v (%s)", method, path, err, b)
			}
		}
		return resp.StatusCode
	}
	jsonBody := func(v any) io.Reader { b, _ := json.Marshal(v); return bytes.NewReader(b) }

	var open struct{ ID, Home string }
	if code := call("POST", "/api/sftp/open", jsonBody(map[string]string{"host": "box"}), &open); code != 200 || open.ID == "" {
		t.Fatalf("open = %d %+v", code, open)
	}
	id := url.QueryEscape(open.ID)
	home0 := open.Home
	if !strings.HasPrefix(home0, "/") {
		t.Fatalf("home must be an absolute remote path: %q", home0)
	}
	q := func(p string) string { return url.QueryEscape(p) }
	join := func(n string) string { return strings.TrimSuffix(home0, "/") + "/" + n }

	var list struct {
		Path    string
		Entries []sftpEntry
	}
	if code := call("GET", "/api/sftp/list?id="+id, nil, &list); code != 200 || len(list.Entries) != 3 || !list.Entries[0].Dir || list.Entries[0].Name != "sub" {
		t.Fatalf("list = %d %+v", code, list)
	}
	if code := call("GET", "/api/sftp/list?id="+id+"&path=relative", nil, nil); code != 400 {
		t.Fatalf("relative path accepted: %d", code)
	}

	var pv sftpPreview
	call("GET", "/api/sftp/preview?id="+id+"&path="+q(join("notes.txt")), nil, &pv)
	if pv.Text != "line one\nline two\n" || pv.Binary {
		t.Fatalf("preview = %+v", pv)
	}
	call("GET", "/api/sftp/preview?id="+id+"&path="+q(join("blob.bin")), nil, &pv)
	if !pv.Binary {
		t.Fatalf("binary not detected: %+v", pv)
	}

	var dl string
	if code := call("GET", "/api/sftp/download?id="+id+"&path="+q(join("notes.txt")), nil, &dl); code != 200 || dl != "line one\nline two\n" {
		t.Fatalf("download = %d %q", code, dl)
	}

	up := "/api/sftp/upload?id=" + id + "&dir=" + q(home0) + "&name="
	if code := call("POST", up+"new.txt", strings.NewReader("fresh"), nil); code != 200 {
		t.Fatalf("upload = %d", code)
	}
	if code := call("POST", up+"new.txt", strings.NewReader("again"), nil); code != http.StatusConflict {
		t.Fatalf("upload over existing file without overwrite = %d", code)
	}
	if code := call("POST", up+"new.txt&overwrite=1", strings.NewReader("replaced"), nil); code != 200 {
		t.Fatalf("overwrite = %d", code)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "new.txt")); string(b) != "replaced" {
		t.Fatalf("uploaded content = %q", b)
	}
	if code := call("POST", up+"..%2Fescape", strings.NewReader("x"), nil); code != 400 {
		t.Fatalf("bad name accepted: %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, ".new.txt.devdeck-upload")); err == nil {
		t.Fatal("temporary upload file left behind")
	}

	ok := func(op string, body map[string]string) int {
		body["id"] = open.ID
		return call("POST", "/api/sftp/"+op, jsonBody(body), nil)
	}
	if code := ok("mkdir", map[string]string{"path": join("made")}); code != 200 {
		t.Fatalf("mkdir = %d", code)
	}
	if code := ok("rename", map[string]string{"path": join("new.txt"), "to": join("made/moved.txt")}); code != 200 {
		t.Fatalf("rename = %d", code)
	}
	if code := ok("rename", map[string]string{"path": join("notes.txt"), "to": join("made/moved.txt")}); code != http.StatusConflict {
		t.Fatalf("rename onto existing = %d", code)
	}
	if code := ok("delete", map[string]string{"path": join("made")}); code != 400 {
		t.Fatalf("non-empty dir delete = %d", code)
	}
	if code := ok("delete", map[string]string{"path": join("made/moved.txt")}); code != 200 {
		t.Fatalf("delete file = %d", code)
	}
	if code := ok("delete", map[string]string{"path": join("made")}); code != 200 {
		t.Fatalf("delete empty dir = %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "made")); !os.IsNotExist(err) {
		t.Fatalf("dir still there: %v", err)
	}

	call("POST", "/api/sftp/close", jsonBody(map[string]string{"id": open.ID}), nil)
	if code := call("GET", "/api/sftp/list?id="+id, nil, nil); code != http.StatusGone {
		t.Fatalf("closed session = %d", code)
	}
}

func TestLooksBinary(t *testing.T) {
	if looksBinary([]byte("plain text\nwith ünïcode")) || !looksBinary([]byte("a\x00b")) || !looksBinary([]byte("\xff\xfe\xfd\xfc\xfb\xfa\xf9")) {
		t.Fatal("looksBinary")
	}
}

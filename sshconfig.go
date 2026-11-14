package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A small read-only ~/.ssh/config reader: Host blocks with wildcard / negated patterns, Include,
// and first-value-wins semantics like OpenSSH. Match blocks are skipped. Only the options devdeck
// uses are interpreted: HostName, User, Port, IdentityFile, ProxyJump.

type sshCfgOpt struct{ key, val string }

type sshCfgBlock struct {
	patterns []string
	match    bool // a Match block: never applied
	opts     []sshCfgOpt
}

type sshConfig struct {
	blocks []*sshCfgBlock
	files  int
}

type sshCfgHost struct {
	HostName      string
	User          string
	Port          int
	IdentityFiles []string
	ProxyJump     string
}

func sshConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "config")
}

// loadSSHConfig reads ~/.ssh/config fresh each call (it is tiny); nil when there is none.
func loadSSHConfig() *sshConfig {
	p := sshConfigPath()
	if p == "" {
		return nil
	}
	c := &sshConfig{}
	if err := c.parseFile(p, 0, c.global()); err != nil {
		return nil
	}
	return c
}

func parseSSHConfig(r io.Reader) *sshConfig {
	c := &sshConfig{}
	c.parse(r, filepath.Dir(sshConfigPath()), 0, c.global())
	return c
}

func (c *sshConfig) global() *sshCfgBlock {
	b := &sshCfgBlock{patterns: []string{"*"}}
	c.blocks = append(c.blocks, b)
	return b
}

func (c *sshConfig) parseFile(path string, depth int, cur *sshCfgBlock) error {
	c.files++
	if c.files > 32 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	c.parse(io.LimitReader(f, 1<<20), filepath.Dir(path), depth, cur)
	return nil
}

func (c *sshConfig) parse(r io.Reader, dir string, depth int, cur *sshCfgBlock) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 64*1024)
	for sc.Scan() {
		line := strings.TrimSpace(string(bytes.TrimPrefix(sc.Bytes(), []byte("\xef\xbb\xbf"))))
		if line == "" || line[0] == '#' {
			continue
		}
		key, val := splitCfgLine(line)
		switch strings.ToLower(key) {
		case "host":
			cur = &sshCfgBlock{patterns: cfgFields(val)}
			c.blocks = append(c.blocks, cur)
		case "match":
			cur = &sshCfgBlock{match: true}
			c.blocks = append(c.blocks, cur)
		case "include":
			if depth >= 8 {
				continue
			}
			for _, p := range cfgFields(val) {
				p = expandHome(p)
				if !filepath.IsAbs(p) {
					p = filepath.Join(dir, p)
				}
				matches, _ := filepath.Glob(p)
				for _, m := range matches {
					if st, err := os.Stat(m); err == nil && !st.IsDir() {
						_ = c.parseFile(m, depth+1, cur)
					}
				}
			}
			// Host lines inside an included file do not leak into the rest of this file.
			c.blocks = append(c.blocks, &sshCfgBlock{patterns: cur.patterns, match: cur.match})
			cur = c.blocks[len(c.blocks)-1]
		default:
			if val != "" {
				cur.opts = append(cur.opts, sshCfgOpt{strings.ToLower(key), unquote(val)})
			}
		}
	}
}

// splitCfgLine splits "Key value", "Key=value" or "Key = value".
func splitCfgLine(line string) (string, string) {
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return line, ""
	}
	key, rest := line[:i], strings.TrimLeft(line[i:], " \t")
	return key, strings.TrimSpace(strings.TrimPrefix(rest, "="))
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' && !strings.Contains(s[1:len(s)-1], `"`) {
		return s[1 : len(s)-1]
	}
	return s
}

func cfgFields(s string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
		case (r == ' ' || r == '\t') && !inQ:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func (b *sshCfgBlock) matches(host string) bool {
	if b.match {
		return false
	}
	host = strings.ToLower(host)
	ok := false
	for _, p := range b.patterns {
		neg := strings.HasPrefix(p, "!")
		if wildMatch(strings.ToLower(strings.TrimPrefix(p, "!")), host) {
			if neg {
				return false
			}
			ok = true
		}
	}
	return ok
}

// wildMatch matches OpenSSH host patterns: '*' any run, '?' one character.
func wildMatch(pat, s string) bool {
	px, sx, starP, starS := 0, 0, -1, 0
	for sx < len(s) {
		switch {
		case px < len(pat) && (pat[px] == '?' || pat[px] == s[sx]):
			px++
			sx++
		case px < len(pat) && pat[px] == '*':
			starP, starS = px, sx
			px++
		case starP >= 0:
			px = starP + 1
			starS++
			sx = starS
		default:
			return false
		}
	}
	for px < len(pat) && pat[px] == '*' {
		px++
	}
	return px == len(pat)
}

// aliases lists the concrete host names (no wildcards) in the order they appear.
func (c *sshConfig) aliases() []string {
	var out []string
	seen := map[string]bool{}
	for _, b := range c.blocks {
		if b.match {
			continue
		}
		for _, p := range b.patterns {
			if strings.ContainsAny(p, "*?!") || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func (c *sshConfig) hasAlias(name string) bool {
	for _, a := range c.aliases() {
		if a == name {
			return true
		}
	}
	return false
}

// lookup applies every matching block in order; the first value of each option wins
// (IdentityFile accumulates). Tokens %h %p %r %u %d %% and ~ are expanded.
func (c *sshConfig) lookup(alias string) sshCfgHost {
	var h sshCfgHost
	seen := map[string]bool{}
	var ids []string
	for _, b := range c.blocks {
		if !b.matches(alias) {
			continue
		}
		for _, o := range b.opts {
			if o.key == "identityfile" {
				ids = append(ids, o.val)
				continue
			}
			if seen[o.key] {
				continue
			}
			seen[o.key] = true
			switch o.key {
			case "hostname":
				h.HostName = o.val
			case "user":
				h.User = o.val
			case "port":
				h.Port, _ = strconv.Atoi(o.val)
			case "proxyjump":
				h.ProxyJump = o.val
			}
		}
	}
	if h.HostName == "" {
		h.HostName = alias
	}
	h.HostName = expandTokens(h.HostName, alias, h)
	for _, id := range ids {
		if strings.EqualFold(id, "none") {
			continue
		}
		h.IdentityFiles = append(h.IdentityFiles, expandHome(expandTokens(id, alias, h)))
	}
	return h
}

func expandTokens(s, alias string, h sshCfgHost) string {
	if !strings.Contains(s, "%") {
		return s
	}
	home, _ := os.UserHomeDir()
	port := h.Port
	if port == 0 {
		port = 22
	}
	user := h.User
	if user == "" {
		user = localUser()
	}
	host := h.HostName
	if host == "" || strings.Contains(host, "%") {
		host = alias
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case '%':
			b.WriteByte('%')
		case 'h':
			b.WriteString(host)
		case 'n':
			b.WriteString(alias)
		case 'p':
			b.WriteString(strconv.Itoa(port))
		case 'r':
			b.WriteString(user)
		case 'u':
			b.WriteString(localUser())
		case 'd':
			b.WriteString(home)
		default:
			b.WriteByte('%')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

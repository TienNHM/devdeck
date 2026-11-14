package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"syscall"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type PortInfo struct {
	Port    uint32 `json:"port"`
	Addr    string `json:"addr"`
	PID     int32  `json:"pid"`
	Process string `json:"process"`
}

type ProcInfo struct {
	PID  int32  `json:"pid"`
	PPID int32  `json:"ppid"`
	Name string `json:"name"`
	RSS  uint64 `json:"rss"`
}

func listPorts() ([]PortInfo, error) {
	conns, err := net.Connections("tcp")
	if err != nil {
		return nil, err
	}
	names := map[int32]string{}
	seen := map[string]bool{}
	var out []PortInfo
	for _, c := range conns {
		if c.Type != syscall.SOCK_STREAM || c.Status != "LISTEN" {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d", c.Laddr.IP, c.Laddr.Port, c.Pid)
		if seen[key] {
			continue
		}
		seen[key] = true
		name, ok := names[c.Pid]
		if !ok {
			if p, err := process.NewProcess(c.Pid); err == nil {
				name, _ = p.Name()
			}
			names[c.Pid] = name
		}
		out = append(out, PortInfo{Port: c.Laddr.Port, Addr: c.Laddr.IP, PID: c.Pid, Process: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out, nil
}

func listProcesses(limit int) ([]ProcInfo, error) {
	ps, err := process.Processes()
	if err != nil {
		return nil, err
	}
	out := make([]ProcInfo, 0, len(ps))
	for _, p := range ps {
		name, _ := p.Name()
		ppid, _ := p.Ppid()
		var rss uint64
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			rss = mi.RSS
		}
		out = append(out, ProcInfo{PID: p.Pid, PPID: ppid, Name: name, RSS: rss})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RSS > out[j].RSS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func killProcess(pid int32, tree bool) error {
	if pid <= 4 || int(pid) == os.Getpid() {
		return errors.New("refusing to kill system process or devdeck itself")
	}
	if tree && runtime.GOOS == "windows" {
		out, err := exec.Command("taskkill", "/PID", strconv.Itoa(int(pid)), "/T", "/F").CombinedOutput()
		if err != nil {
			return fmt.Errorf("taskkill: %v: %s", err, out)
		}
		return nil
	}
	p, err := process.NewProcess(pid)
	if err != nil {
		return err
	}
	if tree {
		kids, _ := p.Children()
		for _, k := range kids {
			_ = k.Kill()
		}
	}
	return p.Kill()
}

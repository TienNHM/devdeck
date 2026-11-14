package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"syscall"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// PortInfo is one listening TCP socket and its owner. The fields after Process are best effort:
// they stay empty when the OS denies access (elevated or other users' processes).
type PortInfo struct {
	Port    uint32 `json:"port"`
	Addr    string `json:"addr"`
	PID     int32  `json:"pid"`
	Process string `json:"process"`
	PPID    int32  `json:"ppid,omitempty"`
	Cmdline string `json:"cmdline,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Started int64  `json:"started,omitempty"` // unix ms
	Kind    string `json:"kind,omitempty"`    // recognised dev server / service, e.g. "Vite"
	KindCat string `json:"kindCat,omitempty"` // web | runtime | db | infra
}

type ProcInfo struct {
	PID     int32   `json:"pid"`
	PPID    int32   `json:"ppid"`
	Name    string  `json:"name"`
	RSS     uint64  `json:"rss"`
	CPU     float64 `json:"cpu"`               // percent of the whole machine; -1 = not sampled yet
	Started int64   `json:"started,omitempty"` // unix ms
	Threads int32   `json:"threads,omitempty"`
}

// listPorts returns listening TCP sockets with owner details. One process snapshot per call;
// command line and working directory come from a small per-process cache (see procDetails).
func listPorts() ([]PortInfo, error) {
	conns, err := net.Connections("tcp")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var listen []net.ConnectionStat
	var pids []int32
	for _, c := range conns {
		if c.Type != syscall.SOCK_STREAM || c.Status != "LISTEN" {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d", c.Laddr.IP, c.Laddr.Port, c.Pid)
		if seen[key] {
			continue
		}
		seen[key] = true
		listen = append(listen, c)
		pids = append(pids, c.Pid)
	}
	snap, full, snapErr := procRowsFor(pids)
	var out []PortInfo
	for _, c := range listen {
		pi := PortInfo{Port: c.Laddr.Port, Addr: c.Laddr.IP, PID: c.Pid}
		row, ok := snap[c.Pid]
		if ok {
			pi.Process, pi.PPID, pi.Started = row.Name, row.PPID, row.Create
		} else if snapErr != nil {
			if p, err := process.NewProcess(c.Pid); err == nil {
				pi.Process, _ = p.Name()
			}
		}
		if c.Pid > 4 {
			d := procDetails(c.Pid, pi.Started)
			pi.Cmdline, pi.Cwd = d.Cmdline, d.Cwd
		}
		pi.Kind, pi.KindCat = devKind(pi.Process, pi.Cmdline)
		out = append(out, pi)
	}
	if snapErr == nil && full {
		pruneDetails(snap)
	} else {
		pruneExpiredDetails()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Addr < out[j].Addr
	})
	return out, nil
}

// listProcesses returns every process sorted by memory, without CPU sampling (CPU is -1).
func listProcesses(limit int) ([]ProcInfo, error) {
	snap, err := procSnapshot(false)
	if err != nil {
		return nil, err
	}
	out := make([]ProcInfo, 0, len(snap))
	for _, r := range snap {
		out = append(out, ProcInfo{PID: r.PID, PPID: r.PPID, Name: r.Name, RSS: r.RSS, CPU: -1, Started: r.Create, Threads: r.Threads})
	}
	sortProcs(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func sortProcs(out []ProcInfo) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].RSS != out[j].RSS {
			return out[i].RSS > out[j].RSS
		}
		return out[i].PID < out[j].PID
	})
}

func killProcess(pid int32, tree bool) error {
	if pid <= 4 || int(pid) == os.Getpid() {
		return errors.New("refusing to kill system process or devdeck itself")
	}
	p, err := process.NewProcess(pid)
	if err != nil {
		return err
	}
	if tree {
		// Not `taskkill /T`: Windows keeps stale parent PIDs, so a reused PID can make unrelated
		// processes look like children. killTree only follows children created after their parent.
		killTree(pid)
		if alive, _ := p.IsRunning(); alive {
			return fmt.Errorf("process %d is still running (access denied?)", pid)
		}
		return nil
	}
	return p.Kill()
}

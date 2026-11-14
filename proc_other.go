//go:build !windows

package main

import (
	"os/exec"

	"github.com/shirou/gopsutil/v4/process"
)

func procHideWindow(*exec.Cmd) {}

// procRowsFor reads only the given PIDs (the Ports tab needs a handful, not the whole table).
func procRowsFor(pids []int32) (map[int32]procRow, bool, error) {
	out := make(map[int32]procRow, len(pids))
	for _, pid := range pids {
		if _, ok := out[pid]; ok {
			continue
		}
		r := procRow{PID: pid, CPUSec: -1}
		if p, err := process.NewProcess(pid); err == nil {
			r.Name, _ = p.Name()
			r.PPID, _ = p.Ppid()
			r.Create, _ = p.CreateTime()
		}
		out[pid] = r
	}
	return out, false, nil
}

// procSnapshot reads every process through gopsutil (cheap /proc reads on Linux).
// CPU times are only read when withCPU is set.
func procSnapshot(withCPU bool) (map[int32]procRow, error) {
	ps, err := process.Processes()
	if err != nil {
		return nil, err
	}
	out := make(map[int32]procRow, len(ps))
	for _, p := range ps {
		r := procRow{PID: p.Pid}
		r.Name, _ = p.Name()
		r.PPID, _ = p.Ppid()
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			r.RSS = mi.RSS
		}
		r.Create, _ = p.CreateTime()
		r.CPUSec = -1
		if withCPU {
			if t, err := p.Times(); err == nil && t != nil {
				r.CPUSec = t.User + t.System
			}
		}
		out[p.Pid] = r
	}
	return out, nil
}

package main

import (
	"os"

	"github.com/shirou/gopsutil/v4/process"
)

// killTree kills the descendants of root, then root. Unlike `taskkill /T`, a process only counts as a
// child when it was created after its parent: Windows keeps stale parent PIDs, so a reused PID could
// otherwise make unrelated processes (even devdeck's own parent shell) look like children.
func killTree(root int32) {
	if root <= 4 || int(root) == os.Getpid() {
		return
	}
	procs, err := process.Processes()
	if err != nil {
		if p, err := process.NewProcess(root); err == nil {
			_ = p.Kill()
		}
		return
	}
	byPID := map[int32]*process.Process{}
	kids := map[int32][]int32{}
	for _, p := range procs {
		byPID[p.Pid] = p
		if pp, err := p.Ppid(); err == nil && pp != p.Pid {
			kids[pp] = append(kids[pp], p.Pid)
		}
	}
	created := func(pid int32) int64 {
		if p := byPID[pid]; p != nil {
			if t, err := p.CreateTime(); err == nil {
				return t
			}
		}
		return -1
	}
	var order []int32 // breadth-first; killed in reverse so leaves go first
	seen := map[int32]bool{root: true}
	queue := []int32{root}
	for len(queue) > 0 && len(order) < 256 {
		pid := queue[0]
		queue = queue[1:]
		order = append(order, pid)
		pt := created(pid)
		for _, k := range kids[pid] {
			if seen[k] || int(k) == os.Getpid() {
				continue
			}
			if kt := created(k); pt < 0 || kt < 0 || kt < pt {
				continue // unknown or older than its "parent": a stale PPID, not a real child
			}
			seen[k] = true
			queue = append(queue, k)
		}
	}
	for i := len(order) - 1; i >= 0; i-- {
		if p := byPID[order[i]]; p != nil {
			_ = p.Kill()
		}
	}
}

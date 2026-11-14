//go:build windows

package main

import (
	"errors"
	"os/exec"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// procHideWindow stops helper processes (git, taskkill) from flashing a console window.
func procHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// procRowsFor returns rows for at least the given PIDs. On Windows one snapshot is cheaper than
// opening each process, so it returns every process (full = true).
func procRowsFor(_ []int32) (map[int32]procRow, bool, error) {
	s, err := procSnapshot(false)
	return s, err == nil, err
}

var snapBufSize atomic.Int64

// procSnapshot reads every process in ONE NtQuerySystemInformation call (what Task Manager does)
// instead of opening each process several times. CPU times are always included on Windows.
func procSnapshot(_ bool) (map[int32]procRow, error) {
	size := snapBufSize.Load()
	if size == 0 {
		size = 512 << 10
	}
	var buf []byte
	for try := 0; ; try++ {
		buf = make([]byte, size)
		var need uint32
		err := windows.NtQuerySystemInformation(windows.SystemProcessInformation, unsafe.Pointer(&buf[0]), uint32(len(buf)), &need)
		if err == nil {
			buf = buf[:max(int(need), 1)]
			break
		}
		if !errors.Is(err, windows.STATUS_INFO_LENGTH_MISMATCH) || try >= 5 {
			return nil, err
		}
		size = int64(need) + 64<<10
		if size < int64(len(buf))*2 {
			size = int64(len(buf)) * 2
		}
	}
	// Remember a little headroom so the next call normally succeeds first time.
	snapBufSize.Store(int64(len(buf)) + 64<<10)

	out := make(map[int32]procRow, 512)
	for off := 0; off+int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{})) <= len(buf); {
		e := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buf[off]))
		pid := int32(e.UniqueProcessID)
		r := procRow{PID: pid, PPID: int32(e.InheritedFromUniqueProcessID), RSS: uint64(e.WorkingSetSize), Threads: int32(e.NumberOfThreads)}
		switch pid {
		case 0:
			r.Name = "System Idle Process"
			r.PPID = 0
		default:
			r.Name = e.ImageName.String()
		}
		r.CPUSec = float64(e.UserTime+e.KernelTime) / 1e7
		if e.CreateTime > 116444736000000000 {
			r.Create = (e.CreateTime - 116444736000000000) / 10000 // FILETIME (100 ns since 1601) -> unix ms
		}
		out[pid] = r
		if e.NextEntryOffset == 0 {
			break
		}
		off += int(e.NextEntryOffset)
	}
	return out, nil
}

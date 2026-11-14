package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// dockerLogsStream runs `docker logs --tail N -f <id>` for as long as the client stays connected
// and forwards the lines as server-sent events in small batches. The process is killed when the
// request context ends (viewer closed), when the container stops, or when the byte cap is reached.
func dockerLogsStream(w http.ResponseWriter, r *http.Request, id string, tail int) {
	s, err := newSSE(w)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if err := validContainer(id); err != nil {
		_ = s.send("fail", err.Error())
		return
	}
	tail = clampInt(tail, 1, 5000, 200)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	pr, pw, err := os.Pipe()
	if err != nil {
		_ = s.send("fail", err.Error())
		return
	}
	defer pr.Close()
	cmd := exec.CommandContext(ctx, dockerBin(), "logs", "--tail", strconv.Itoa(tail), "-f", id)
	cmd.Env = dockerEnv()
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdout, cmd.Stderr = pw, pw
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		pw.Close()
		_ = s.send("fail", classifyDocker(ctx, err, "", []string{"logs"}).Error())
		return
	}
	pw.Close() // the child holds its own copy; EOF arrives when it exits
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	lines := make(chan string, 256)
	go func() {
		defer close(lines)
		_ = readLines(pr, func(l string) bool {
			select {
			case lines <- l:
				return true
			case <-ctx.Done():
				return false
			}
		})
	}()

	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	var batch []string
	total, idle := 0, 0
	flush := func() bool {
		if len(batch) == 0 {
			return true
		}
		err := s.send("", batch)
		batch = batch[:0]
		return err == nil
	}
	for {
		select {
		case <-ctx.Done():
			return
		case l, ok := <-lines:
			if !ok {
				flush()
				msg := "log stream ended"
				select {
				case err := <-exited:
					if err != nil && ctx.Err() == nil {
						msg = fmt.Sprintf("log stream ended (%v)", err)
					}
				case <-time.After(3 * time.Second):
				}
				_ = s.send("end", msg)
				return
			}
			batch = append(batch, l)
			total += len(l) + 1
			if total > streamCapBytes {
				flush()
				_ = s.send("end", "stopped: stream limit reached, follow again to continue")
				return
			}
			if len(batch) >= sseBatchLines && !flush() {
				return
			}
		case <-tick.C:
			if len(batch) > 0 {
				idle = 0
				if !flush() {
					return
				}
			} else if idle++; idle >= 100 { // ~15 s of silence: ping so dead connections are noticed
				idle = 0
				if s.ping() != nil {
					return
				}
			}
		}
	}
}

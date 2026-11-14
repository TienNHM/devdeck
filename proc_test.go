package main

import (
	"os"
	"testing"
	"time"
)

func TestDevKind(t *testing.T) {
	cases := []struct{ name, cmd, want string }{
		{"node.exe", `"C:\Program Files\nodejs\node.exe" D:\app\node_modules\vite\bin\vite.js --port 5173`, "Vite"},
		{"node", "node /app/node_modules/.bin/next dev", "Next.js"},
		{"node.exe", `node D:\app\node_modules\webpack-dev-server\bin\webpack-dev-server.js`, "webpack"},
		{"node.exe", `node D:\app\server.js`, "Node.js"},
		{"python3.12", "python3.12 -m uvicorn main:app --reload", "Uvicorn"},
		{"python.exe", "python manage.py runserver", "Django"},
		{"python.exe", "python -m flask run", "Flask"},
		{"python", "python -m http.server 8000", "http.server"},
		{"java.exe", "java -jar target/demo-0.0.1.jar org.springframework.boot.loader.JarLauncher", "Spring Boot"},
		{"java", "java -jar app.jar", "Java"},
		{"main.exe", `C:\Users\me\AppData\Local\Temp\go-build123\b001\exe\main.exe`, "Go"},
		{"MyApi.exe", `D:\src\MyApi\bin\Debug\net9.0\MyApi.exe`, ".NET"},
		{"dotnet.exe", "dotnet watch run", ".NET"},
		{"postgres.exe", `"C:\Program Files\PostgreSQL\16\bin\postgres.exe" -D data`, "PostgreSQL"},
		{"redis-server", "redis-server *:6379", "Redis"},
		{"com.docker.backend.exe", "", "Docker"},
		{"nginx.exe", "nginx", "nginx"},
		{"deno", "deno run -A main.ts", "Deno"},
		{"bun", "bun run index.ts", "Bun"},
		{"svchost.exe", "svchost -k netsvcs", ""},
	}
	for _, c := range cases {
		if got, _ := devKind(c.name, c.cmd); got != c.want {
			t.Errorf("devKind(%q, %q) = %q, want %q", c.name, c.cmd, got, c.want)
		}
	}
}

func TestProcSnapshotFindsSelf(t *testing.T) {
	snap, err := procSnapshot(true)
	if err != nil {
		t.Fatal(err)
	}
	me, ok := snap[int32(os.Getpid())]
	if !ok {
		t.Fatalf("own pid missing from %d processes", len(snap))
	}
	if me.Name == "" || me.RSS == 0 || me.Create == 0 {
		t.Fatalf("incomplete row: %+v", me)
	}
	if d := time.Since(time.UnixMilli(me.Create)); d < 0 || d > time.Hour {
		t.Fatalf("start time looks wrong: %v ago", d)
	}
	if me.PPID != int32(os.Getppid()) {
		t.Fatalf("ppid %d, want %d", me.PPID, os.Getppid())
	}
}

func TestListProcessesCPU(t *testing.T) {
	if _, err := listProcessesCPU(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(600 * time.Millisecond)
	ps, err := listProcessesCPU()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.PID == int32(os.Getpid()) {
			if p.CPU < 0 || p.CPU > 100 {
				t.Fatalf("own cpu %% = %v", p.CPU)
			}
			return
		}
	}
	t.Fatal("own process missing")
}

func TestProcDetailsCacheIsBounded(t *testing.T) {
	d := procDetails(int32(os.Getpid()), 1)
	if d.Cmdline == "" {
		t.Fatal("own command line is empty")
	}
	pruneDetails(map[int32]procRow{})
	detailMu.Lock()
	n := len(detailCache)
	detailMu.Unlock()
	if n != 0 {
		t.Fatalf("prune left %d entries", n)
	}
}

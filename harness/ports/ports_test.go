package ports

import (
	"os"
	"testing"
)

func TestKillHintRefusesPilotPorts(t *testing.T) {
	refused := []string{
		"kill $(lsof -t -i:5173) 2>/dev/null; sleep 2",
		"lsof -ti tcp:5173 | xargs kill -9",
		"npx kill-port 5173",
		"fuser -k 8182/tcp",
		"pkill -f node",
		"killall node",
	}
	for _, c := range refused {
		if KillHint(c) == "" {
			t.Errorf("KillHint(%q) = \"\", want a refusal", c)
		}
	}

	allowed := []string{
		"kill $(lsof -t -i:5300)",
		"npm run build",
		"npx kill-port 5300",
		"pkill -f my-own-worker",
		"grep -r skill ./src",
		"echo killer feature",
	}
	for _, c := range allowed {
		if hint := KillHint(c); hint != "" {
			t.Errorf("KillHint(%q) = %q, want \"\"", c, hint)
		}
	}
}

func TestReservedComesFromTheLauncher(t *testing.T) {
	t.Setenv(EnvKey, "4200, 4300")
	if !IsReserved(4200) || !IsReserved(4300) {
		t.Error("ports named in the env must be reserved")
	}
	if IsReserved(5173) {
		t.Error("an explicit env list replaces the defaults")
	}
	if KillHint("kill $(lsof -t -i:4200)") == "" {
		t.Error("KillHint must follow the env list")
	}
}

func TestDefaultsCoverTheStack(t *testing.T) {
	os.Unsetenv(EnvKey)
	for _, p := range []int{5173, 8182, 9000, 11434} {
		if !IsReserved(p) {
			t.Errorf("port %d must be reserved by default", p)
		}
	}
	if IsReserved(5200) {
		t.Error("5200 is an ordinary app port")
	}
}

func TestFreeSkipsReservedPorts(t *testing.T) {
	t.Setenv(EnvKey, "5200,5201")
	if p := Free(5200); p == 5200 || p == 5201 {
		t.Errorf("Free(5200) = %d, want a port outside the reserved list", p)
	}
}

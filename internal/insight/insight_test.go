package insight

import (
	"strings"
	"testing"

	"taskman/internal/metrics"
)

func has(hints []Hint, substr string) bool {
	for _, h := range hints {
		if strings.Contains(h.Text, substr) {
			return true
		}
	}
	return false
}

func TestCachedVRAM(t *testing.T) {
	c := Context{Proc: metrics.ProcInfo{Name: "SongPlayer.exe", VRAM: 4400 * mib, VRAMKnown: true, GPUKnown: true}}
	if h := For(c); !has(h, "4.3 GiB of GPU memory while idle") {
		t.Errorf("idle VRAM holder: %+v", h)
	}
	c.Proc.GPU = 40
	if has(For(c), "while idle") {
		t.Error("a busy GPU user isn't caching")
	}
}

func TestLLMFallbackIsAWarningFirst(t *testing.T) {
	c := Context{NumCPU: 24, MemTotal: 64 << 30, Proc: metrics.ProcInfo{
		Name: "ollama.exe", CPU: 25, CPUKnown: true, GPU: 0.5, GPUKnown: true, // 25% of 24 cores = 6 cores
	}}
	h := For(c)
	if len(h) == 0 || h[0].Level != Warn || !strings.Contains(h[0].Text, "6.0 cores") {
		t.Fatalf("want the fallback warning first: %+v", h)
	}
	c.Proc.GPU = 90
	if has(For(c), "fallen back") {
		t.Error("an LLM using the GPU hasn't fallen back")
	}
	c.Proc.Name, c.Proc.GPU = "chrome.exe", 0
	if has(For(c), "fallen back") {
		t.Error("only LLM runtimes get the fallback hint")
	}
}

func TestChromiumRole(t *testing.T) {
	c := Context{Proc: metrics.ProcInfo{Name: "chrome.exe"},
		Details: metrics.ProcDetails{Cmdline: `chrome.exe --type=gpu-process --field-trial-handle=1`}}
	if !has(For(c), "the GPU process, which draws") {
		t.Errorf("gpu-process: %+v", For(c))
	}
	c.Details.Cmdline = `chrome.exe --type=renderer --lang=en`
	if !has(For(c), "runs one tab") {
		t.Error("renderer")
	}
}

func TestManyProcesses(t *testing.T) {
	c := Context{SameName: 31, Proc: metrics.ProcInfo{Name: "chrome.exe"}}
	if !has(For(c), "One of 31 chrome processes") {
		t.Errorf("%+v", For(c))
	}
	c.SameName = 2
	if has(For(c), "One of") {
		t.Error("two processes is not 'many'")
	}
}

func TestSvchost(t *testing.T) {
	c := Context{Proc: metrics.ProcInfo{Name: "svchost.exe"},
		Details: metrics.ProcDetails{Cmdline: `C:\Windows\system32\svchost.exe -k netsvcs -p -s Winmgmt`}}
	if !has(For(c), "Windows service host for: Winmgmt") {
		t.Errorf("%+v", For(c))
	}
	c.Details.Cmdline = `C:\Windows\system32\svchost.exe -k LocalServiceNoNetwork -p`
	if !has(For(c), "services in group LocalServiceNoNetwork") {
		t.Errorf("%+v", For(c))
	}
}

func TestProtectedAndKnownAndMemory(t *testing.T) {
	if !has(For(Context{Proc: metrics.ProcInfo{PID: 600, Name: "csrss.exe"}}), "run taskman as administrator") {
		t.Error("protected process")
	}
	if !has(For(Context{Proc: metrics.ProcInfo{Name: "dwm.exe", CPUKnown: true}}), "Desktop Window Manager") {
		t.Error("known process note")
	}
	if !has(For(Context{Proc: metrics.ProcInfo{Name: "kworker/3:1", CPUKnown: true}}), "kernel worker") {
		t.Error("kworker prefix")
	}
	big := Context{MemTotal: 64 << 30, Proc: metrics.ProcInfo{Name: "x", RSS: 20 << 30, RSSKnown: true, CPUKnown: true}}
	if h := For(big); len(h) == 0 || h[0].Level != Warn || !strings.Contains(h[0].Text, "31% of your RAM") {
		t.Errorf("big memory user: %+v", h)
	}
}

func TestVideoEngines(t *testing.T) {
	enc := Context{Proc: metrics.ProcInfo{Name: "obs64.exe", CPUKnown: true, GPUEngine: "GPU 0 · Video Encode"}}
	if !has(For(enc), "Encoding video") {
		t.Error("encode")
	}
	if has(For(Context{Proc: metrics.ProcInfo{Name: "x", CPUKnown: true}}), "video") {
		t.Error("no engine, no video hint")
	}
}

func TestManyProcessesWording(t *testing.T) {
	svc := Context{SameName: 102, Proc: metrics.ProcInfo{Name: "svchost.exe"}}
	if h := For(svc); has(h, "Browser") || !has(h, "This program runs as several processes") {
		t.Errorf("svchost isn't a browser: %+v", h)
	}
	tab := Context{SameName: 65, Proc: metrics.ProcInfo{Name: "chrome.exe"},
		Details: metrics.ProcDetails{Cmdline: "chrome.exe --type=renderer"}}
	if !has(For(tab), "Browser engines") {
		t.Error("chrome renderer should get the browser explanation")
	}
}

func TestIsolatedProcessesDontSayRunAsAdmin(t *testing.T) {
	h := For(Context{Proc: metrics.ProcInfo{PID: 156, Name: "Secure System"}})
	if has(h, "administrator") || !has(h, "no tool can read") {
		t.Errorf("Secure System: %+v", h)
	}
}

func TestZombie(t *testing.T) {
	if h := For(Context{Proc: metrics.ProcInfo{Name: "worker", Status: "zombie"}}); !has(h, "Already exited") {
		t.Errorf("zombie: %+v", h)
	}
	if has(For(Context{Proc: metrics.ProcInfo{Name: "worker", Status: "sleep"}}), "Already exited") {
		t.Error("a sleeping process hasn't exited")
	}
}

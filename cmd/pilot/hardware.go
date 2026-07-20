package main

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// hardware is the machine's memory profile, used to size the ollama context
// window. RAMGiB is total system memory; VRAMGiB is discrete-GPU memory (0 when
// there is none or it cannot be read); AppleSilicon marks unified-memory Macs,
// where the GPU shares system RAM rather than having its own VRAM.
type hardware struct {
	RAMGiB       int
	VRAMGiB      int
	AppleSilicon bool
}

// ctx window presets (decimal-thousand tokens), matched to how much memory the
// KV cache needs. ollama clamps each to the model's trained maximum, so these
// are safe upper bounds, not per-model guarantees.
const (
	ctxLow  = 16000
	ctx32k  = 32000
	ctx64k  = 64000
	ctx128k = 128000
	ctx256k = 256000
	ctxMin  = 8000
)

// detectHardware reads total RAM and discrete-GPU VRAM for the current machine,
// falling back to zeros when a probe is unavailable (which yields conservative
// context sizing rather than an error).
func detectHardware() hardware {
	return hardware{
		RAMGiB:       ramGiB(),
		VRAMGiB:      vramGiB(),
		AppleSilicon: runtime.GOOS == "darwin" && runtime.GOARCH == "arm64",
	}
}

// pickContextLength chooses the OLLAMA_CONTEXT_LENGTH for this machine. Discrete
// GPUs are sized by VRAM; Apple Silicon and CPU-only hosts by system RAM. The
// scale is deliberately conservative so a large window never pushes the KV cache
// past available memory.
func pickContextLength(hw hardware) int {
	if hw.VRAMGiB > 0 { // discrete GPU: the KV cache lives in VRAM
		switch {
		case hw.VRAMGiB >= 24:
			return ctx256k
		case hw.VRAMGiB >= 15: // "around 16 GiB"
			return ctx64k
		case hw.VRAMGiB >= 10:
			return ctx32k
		default:
			return ctxLow
		}
	}
	if hw.AppleSilicon { // unified memory: GPU shares system RAM
		switch {
		case hw.RAMGiB >= 48:
			return ctx256k
		case hw.RAMGiB >= 32:
			return ctx128k
		case hw.RAMGiB >= 24:
			return ctx64k
		case hw.RAMGiB >= 16:
			return ctx32k
		default:
			return ctxLow
		}
	}
	switch { // CPU-only: the KV cache lives in system RAM
	case hw.RAMGiB >= 64:
		return ctx64k
	case hw.RAMGiB >= 32:
		return ctx32k
	case hw.RAMGiB >= 16:
		return ctxLow
	default:
		return ctxMin
	}
}

// ramGiB returns total system memory in GiB, or 0 if it cannot be determined.
func ramGiB() int {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err == nil {
			if b, e := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); e == nil {
				return int(b / (1 << 30))
			}
		}
	case "linux":
		f, err := os.Open("/proc/meminfo")
		if err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				line := sc.Text()
				if strings.HasPrefix(line, "MemTotal:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						if kb, e := strconv.ParseInt(fields[1], 10, 64); e == nil {
							return int(kb / (1 << 20)) // kB -> GiB
						}
					}
				}
			}
		}
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			"(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory").Output()
		if err == nil {
			if b, e := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); e == nil {
				return int(b / (1 << 30))
			}
		}
	}
	return 0
}

// vramGiB returns the largest NVIDIA GPU's memory in GiB via nvidia-smi, or 0
// when no discrete NVIDIA GPU is present (Apple Silicon and AMD report 0 here
// and are sized by system RAM instead).
func vramGiB() int {
	out, err := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return 0
	}
	max := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		mib, e := strconv.Atoi(strings.TrimSpace(line))
		if e == nil {
			g := mib / 1024
			if g > max {
				max = g
			}
		}
	}
	return max
}

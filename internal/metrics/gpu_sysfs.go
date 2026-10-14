package metrics

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Linux GPU sysfs readers. This file has no build tag so tests can run them
// against a fake tree on any OS; only Linux calls them.

// readAMD reads AMD GPUs from the amdgpu driver's sysfs files under root
// (normally "/sys").
func readAMD(root string) []GPUStat {
	cards, _ := filepath.Glob(filepath.Join(root, "class", "drm", "card[0-9]*"))
	var out []GPUStat
	for _, card := range cards {
		if strings.Contains(filepath.Base(card), "-") {
			continue // connectors such as card0-DP-1
		}
		dev := filepath.Join(card, "device")
		if readTrim(filepath.Join(dev, "vendor")) != "0x1002" {
			continue
		}
		st := GPUStat{Vendor: "AMD", PState: -1, Name: readTrim(filepath.Join(dev, "product_name"))}
		if st.Name == "" {
			st.Name = "AMD GPU"
		}
		if v, ok := readUint(filepath.Join(dev, "gpu_busy_percent")); ok {
			st.Util, st.UtilKnown = float64(v), true
			st.Engines = []EngineStat{{"Graphics", float64(v)}}
		}
		used, okU := readUint(filepath.Join(dev, "mem_info_vram_used"))
		total, okT := readUint(filepath.Join(dev, "mem_info_vram_total"))
		if okU && okT {
			st.MemUsed, st.MemTotal, st.MemKnown = used, total, true
		}
		if hw, _ := filepath.Glob(filepath.Join(dev, "hwmon", "hwmon*")); len(hw) > 0 {
			h := hw[0]
			if v, ok := readUint(filepath.Join(h, "temp1_input")); ok { // millidegrees
				st.TempC, st.TempKnown = float64(v)/1000, true
			}
			power, ok := readUint(filepath.Join(h, "power1_average")) // microwatts
			if !ok {
				power, ok = readUint(filepath.Join(h, "power1_input"))
			}
			if ok {
				st.PowerW, st.PowerKnown = float64(power)/1e6, true
				if limit, ok := readUint(filepath.Join(h, "power1_cap")); ok {
					st.PowerLimitW = float64(limit) / 1e6
				}
			}
			if v, ok := readUint(filepath.Join(h, "freq1_input")); ok { // Hz
				st.ClockMHz, st.ClockKnown = uint32(v/1_000_000), true
			}
			if v, ok := readUint(filepath.Join(h, "fan1_input")); ok && v > 0 {
				st.FanRPM = uint32(v)
			}
		}
		out = append(out, st)
	}
	return out
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readUint(path string) (uint64, bool) {
	v, err := strconv.ParseUint(readTrim(path), 10, 64)
	return v, err == nil
}

// hasNonNVIDIAGPU reports whether any DRM device under root (normally
// "/sys") is from another vendor; their per-process data comes from fdinfo,
// which needs root to see other users' processes.
func hasNonNVIDIAGPU(root string) bool {
	vendors, _ := filepath.Glob(filepath.Join(root, "class", "drm", "card[0-9]*", "device", "vendor"))
	for _, v := range vendors {
		if readTrim(v) != "0x10de" {
			return true
		}
	}
	return false
}

package metrics

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestDiskDelta(t *testing.T) {
	// Windows style: 100ns ticks, busy measured against the query clock.
	prev := diskRaw{ReadBytes: 1000, WriteBytes: 0, Reads: 10, Writes: 0,
		TickUnit: 100 * time.Nanosecond, IOTime: 0, Busy: 0, Clock: 1e7, HasBusy: true, HasIO: true}
	cur := prev
	cur.ReadBytes, cur.WriteBytes = 1000+4_000_000, 2_000_000
	cur.Reads, cur.Writes = 30, 10            // 30 I/Os
	cur.IOTime = 30 * 5_000                   // 0.5 ms each
	cur.Clock, cur.Busy = 2e7, 2_500_000      // 0.25s busy of 1s
	st := diskDelta(prev, cur, 2*time.Second) // wall ignored for active when Clock is set
	if st.ReadBps != 2_000_000 || st.WriteBps != 1_000_000 {
		t.Errorf("rates: %+v", st)
	}
	if !st.ActiveKnown || math.Abs(st.ActivePct-25) > 1e-9 {
		t.Errorf("active: %v %v", st.ActivePct, st.ActiveKnown)
	}
	if !st.RespKnown || math.Abs(st.RespMs-0.5) > 1e-9 {
		t.Errorf("response: %v", st.RespMs)
	}

	// Linux style: ms ticks against wall time; counter reset reads as zero.
	lp := diskRaw{TickUnit: time.Millisecond, Busy: 1000, ReadBytes: 500, HasBusy: true}
	lc := lp
	lc.Busy, lc.ReadBytes = 1500, 100
	st = diskDelta(lp, lc, time.Second)
	if math.Abs(st.ActivePct-50) > 1e-9 || st.ReadBps != 0 || st.RespKnown {
		t.Errorf("linux style: %+v", st)
	}
}

func TestParentDisk(t *testing.T) {
	disks := []string{"nvme0n1", "nvme0n10", "sda"}
	for part, want := range map[string]string{
		"nvme0n1p2": "nvme0n1", "nvme0n10p1": "nvme0n10", "sda1": "sda", "sdb1": "", "sda": "sda",
	} {
		if got := parentDisk(part, disks); got != want {
			t.Errorf("parentDisk(%q) = %q, want %q", part, got, want)
		}
	}
}

func TestReadDisks(t *testing.T) {
	s := NewGopsutilSampler()
	ctx := context.Background()
	s.Sample(ctx)
	time.Sleep(300 * time.Millisecond)
	snap, _, err := s.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Disks) == 0 {
		t.Skip("no physical disks visible (container?)")
	}
	for _, d := range snap.Disks {
		if !d.RatesKnown {
			t.Errorf("%s: rates unknown on second sample", d.Name)
		}
		t.Logf("%s %q %s active=%.1f%%(%v) r=%.0f w=%.0f resp=%.2fms vols=%+v",
			d.Name, d.Model, d.Bus, d.ActivePct, d.ActiveKnown, d.ReadBps, d.WriteBps, d.RespMs, d.Volumes)
	}
}

func TestVolumeRates(t *testing.T) {
	r, w, ok := volumeRates(volumeIO{read: 1000, write: 500}, volumeIO{read: 5000, write: 900}, 2*time.Second)
	if !ok || r != 2000 || w != 200 {
		t.Errorf("rates: %v %v %v", r, w, ok)
	}
	if _, _, ok := volumeRates(volumeIO{}, volumeIO{}, 0); ok {
		t.Error("no elapsed time, no rate")
	}
	if r, _, _ := volumeRates(volumeIO{read: 9}, volumeIO{read: 1}, time.Second); r != 0 {
		t.Error("a counter reset reads as zero")
	}
}

func TestVolumeRatesLive(t *testing.T) {
	s := NewGopsutilSampler()
	ctx := context.Background()
	s.Sample(ctx)
	time.Sleep(500 * time.Millisecond)
	snap, _, err := s.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range snap.Disks {
		for _, v := range d.Volumes {
			t.Logf("%s %s: known=%v read=%.0f write=%.0f", d.Name, v.Mount, v.RatesKnown, v.ReadBps, v.WriteBps)
		}
	}
}

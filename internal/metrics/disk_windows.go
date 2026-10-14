package metrics

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const (
	ioctlDiskPerformance      = 0x70020
	ioctlStorageQueryProperty = 0x2D1400
	ioctlVolumeGetDiskExtents = 0x560000
	maxPhysicalDrives         = 32
)

// readDisks queries each \\.\PhysicalDriveN directly: IOCTL_DISK_PERFORMANCE
// gives the idle/query clocks Task Manager uses for "Active time", which the
// per-volume counters gopsutil reads don't expose.
func readDisks() ([]diskRaw, error) {
	volumes, volIO := volumesByDisk()
	var out []diskRaw
	for n := 0; n < maxPhysicalDrives; n++ {
		h, err := openDevice(fmt.Sprintf(`\\.\PhysicalDrive%d`, n))
		if err != nil {
			continue
		}
		raw, ok := diskPerformance(h)
		if ok {
			raw.ID = strconv.Itoa(n)
			raw.Name = "Disk " + raw.ID
			raw.Model, raw.Bus = storageDescriptor(h)
			raw.Mounts = volumes[uint32(n)]
			raw.VolumeIO = volIO
			out = append(out, raw)
		}
		windows.CloseHandle(h)
	}
	return out, nil
}

func openDevice(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	// Zero access rights suffice for these query IOCTLs and need no admin.
	return windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
}

// diskPerformance parses DISK_PERFORMANCE:
// BytesRead, BytesWritten, ReadTime, WriteTime, IdleTime int64 (times in
// 100ns); ReadCount, WriteCount, QueueDepth, SplitCount uint32; QueryTime int64.
func diskPerformance(h windows.Handle) (diskRaw, bool) {
	var buf [88]byte
	var n uint32
	if err := windows.DeviceIoControl(h, ioctlDiskPerformance, nil, 0, &buf[0], uint32(len(buf)), &n, nil); err != nil || n < 64 {
		return diskRaw{}, false
	}
	le := binary.LittleEndian
	idle, query := le.Uint64(buf[32:]), le.Uint64(buf[56:])
	return diskRaw{
		ReadBytes:  le.Uint64(buf[0:]),
		WriteBytes: le.Uint64(buf[8:]),
		Reads:      uint64(le.Uint32(buf[40:])),
		Writes:     uint64(le.Uint32(buf[44:])),
		TickUnit:   100 * time.Nanosecond,
		IOTime:     le.Uint64(buf[16:]) + le.Uint64(buf[24:]),
		Busy:       sub(query, idle),
		Clock:      query,
		HasBusy:    true,
		HasIO:      true,
	}, true
}

var busTypes = map[uint32]string{
	1: "SCSI", 2: "ATAPI", 3: "ATA", 4: "1394", 6: "Fibre", 7: "USB", 8: "RAID", 9: "iSCSI",
	0xA: "SAS", 0xB: "SATA", 0xC: "SD", 0xD: "MMC", 0xE: "Virtual", 0xF: "Virtual", 0x10: "Spaces",
	0x11: "NVMe", 0x12: "SCM", 0x13: "UFS",
}

// storageDescriptor reads vendor/product and bus type via
// IOCTL_STORAGE_QUERY_PROPERTY(StorageDeviceProperty).
func storageDescriptor(h windows.Handle) (model, bus string) {
	var query [12]byte // PropertyId=0 (device), QueryType=0 (standard)
	var buf [1024]byte
	var n uint32
	if err := windows.DeviceIoControl(h, ioctlStorageQueryProperty, &query[0], uint32(len(query)),
		&buf[0], uint32(len(buf)), &n, nil); err != nil || n < 32 {
		return "", ""
	}
	le := binary.LittleEndian
	str := func(off uint32) string {
		if off == 0 || off >= n {
			return ""
		}
		s := buf[off:n]
		if i := bytes.IndexByte(s, 0); i >= 0 {
			s = s[:i]
		}
		return strings.TrimSpace(string(s))
	}
	model = strings.TrimSpace(str(le.Uint32(buf[12:])) + " " + str(le.Uint32(buf[16:])))
	return model, busTypes[le.Uint32(buf[28:])]
}

// volumesByDisk maps physical disk numbers to the drive roots ("C:\") on
// them, via IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS, and reads each volume's own
// I/O counters (IOCTL_DISK_PERFORMANCE works on volumes too).
func volumesByDisk() (map[uint32][]string, map[string]volumeIO) {
	out := map[uint32][]string{}
	io := map[string]volumeIO{}
	var names [256]uint16
	n, err := windows.GetLogicalDriveStrings(uint32(len(names)), &names[0])
	if err != nil {
		return out, io
	}
	// The buffer is a list of NUL-separated roots ("C:\", "D:\", …).
	for _, root := range splitMultiSZ(names[:n]) {
		rp, _ := windows.UTF16PtrFromString(root)
		if t := windows.GetDriveType(rp); t != windows.DRIVE_FIXED && t != windows.DRIVE_REMOVABLE {
			continue
		}
		h, err := openDevice(`\\.\` + strings.TrimSuffix(root, `\`))
		if err != nil {
			continue
		}
		// VOLUME_DISK_EXTENTS: NumberOfDiskExtents uint32, padding, then
		// DISK_EXTENT{DiskNumber uint32, padding, StartingOffset, ExtentLength int64}.
		var buf [8 + 24*8]byte
		var got uint32
		err = windows.DeviceIoControl(h, ioctlVolumeGetDiskExtents, nil, 0, &buf[0], uint32(len(buf)), &got, nil)
		if perf, ok := diskPerformance(h); ok {
			io[root] = volumeIO{perf.ReadBytes, perf.WriteBytes}
		}
		windows.CloseHandle(h)
		if err != nil || got < 8 {
			continue
		}
		count := binary.LittleEndian.Uint32(buf[0:])
		for i := uint32(0); i < count && 8+24*(i+1) <= got; i++ {
			disk := binary.LittleEndian.Uint32(buf[8+24*i:])
			out[disk] = append(out[disk], root)
		}
	}
	return out, io
}

func splitMultiSZ(u []uint16) []string {
	var out []string
	start := 0
	for i, c := range u {
		if c == 0 {
			if i > start {
				out = append(out, windows.UTF16ToString(u[start:i]))
			}
			start = i + 1
		}
	}
	return out
}

// mountLabel is how a mount is displayed: "C:" rather than "C:\".
func mountLabel(m string) string { return strings.TrimSuffix(m, `\`) }

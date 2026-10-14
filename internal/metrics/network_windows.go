package metrics

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// netMetadata reads adapter descriptions, link speeds and the
// HardwareInterface flag, which separates Wi-Fi/Ethernet from VPN and
// Hyper-V adapters.
func netMetadata() map[string]netMeta {
	out := map[string]netMeta{}
	const flags = windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST | windows.GAA_FLAG_SKIP_DNS_SERVER
	size := uint32(16 << 10)
	var buf []byte
	for tries := 0; ; tries++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0,
			(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) || tries == 3 {
			return out
		}
	}

	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		row := windows.MibIfRow2{InterfaceIndex: a.IfIndex}
		const hardwareInterface = 0x1
		physical := windows.GetIfEntry2Ex(windows.MibIfEntryNormal, &row) == nil &&
			row.InterfaceAndOperStatusFlags&hardwareInterface != 0
		speed := a.TransmitLinkSpeed
		if speed == ^uint64(0) {
			speed = 0
		}
		out[windows.UTF16PtrToString(a.FriendlyName)] = netMeta{
			Desc:     windows.UTF16PtrToString(a.Description),
			Physical: physical,
			SpeedBps: speed,
		}
	}
	return out
}

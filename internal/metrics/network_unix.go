//go:build !windows

package metrics

import "net"

func netMetadata() map[string]netMeta {
	out := map[string]netMeta{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		out[ifc.Name] = netMeta{Physical: isPhysicalNIC(ifc.Name), SpeedBps: nicSpeedBps(ifc.Name)}
	}
	return out
}

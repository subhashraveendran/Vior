package discovery

import (
	"encoding/json"
	"net"
	"time"
)

// Listen listens for discovery beacons on the LAN for the given duration.
// Returns all unique servers found. Designed for future native client apps.
func Listen(timeout time.Duration) ([]Beacon, error) {
	addr := &net.UDPAddr{Port: DefaultPort}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(timeout))

	seen := make(map[string]Beacon)
	buf := make([]byte, 1024)

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			break // timeout or error
		}
		var beacon Beacon
		if json.Unmarshal(buf[:n], &beacon) != nil {
			continue
		}
		if beacon.Magic != Magic {
			continue
		}
		key := beacon.Name + ":" + string(rune(beacon.Port))
		seen[key] = beacon
	}

	result := make([]Beacon, 0, len(seen))
	for _, b := range seen {
		result = append(result, b)
	}
	return result, nil
}

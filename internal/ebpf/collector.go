package ebpf

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

// Event represents a parsed connection event.
type Event struct {
	SocketID   uint64
	LocalIP    net.IP
	RemoteIP   net.IP
	LocalPort  uint16
	RemotePort uint16
	EventType  uint8 // 1 = established, 2 = closed
	OldState   uint8
	Family     uint8
}

// rawEvent matches the memory layout of the C struct exactly.
type rawEvent struct {
	SocketID   uint64
	LocalIP    [16]byte
	RemoteIP   [16]byte
	LocalPort  uint16
	RemotePort uint16
	EventType  uint8
	OldState   uint8
	Family     uint8
	Pad        uint8
}

type Collector struct {
	objs MonitorObjects
	tp   link.Link
}

func NewCollector() (*Collector, error) {
	c := &Collector{}

	if err := LoadMonitorObjects(&c.objs, nil); err != nil {
		return nil, fmt.Errorf("load ebpf objects: %w", err)
	}

	tp, err := link.Tracepoint("sock", "inet_sock_set_state", c.objs.TraceTcpConnect, nil)
	if err != nil {
		return nil, fmt.Errorf("attach tracepoint: %w", err)
	}
	c.tp = tp

	return c, nil
}

// Run reads events from the ring buffer and calls fn for each one.
// Blocks until the ring buffer returns an error (e.g. on Close).
func (c *Collector) Run(fn func(Event)) error {
	rd, err := ringbuf.NewReader(c.objs.Connections)
	if err != nil {
		return fmt.Errorf("open ring buffer: %w", err)
	}
	defer rd.Close()

	var raw rawEvent
	for {
		record, err := rd.Read()
		if err != nil {
			return fmt.Errorf("read ring buffer: %w", err)
		}

		if err := binary.Read(
			newByteReader(record.RawSample),
			binary.LittleEndian,
			&raw,
		); err != nil {
			continue
		}

		var localIP, remoteIP net.IP
		if raw.Family == 10 { // AF_INET6
			localIP = make(net.IP, 16)
			copy(localIP, raw.LocalIP[:])
			remoteIP = make(net.IP, 16)
			copy(remoteIP, raw.RemoteIP[:])
		} else { // AF_INET
			localIP = net.IP(raw.LocalIP[:4])
			remoteIP = net.IP(raw.RemoteIP[:4])
		}

		fn(Event{
			SocketID:   raw.SocketID,
			LocalIP:    localIP,
			RemoteIP:   remoteIP,
			LocalPort:  raw.LocalPort,
			RemotePort: raw.RemotePort,
			EventType:  raw.EventType,
			OldState:   raw.OldState,
			Family:     raw.Family,
		})
	}
}

func (c *Collector) Close() {
	c.tp.Close()
	c.objs.Close()
}

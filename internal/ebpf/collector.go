package ebpf

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

// Event mirrors the C connection_event struct byte-for-byte.
type Event struct {
	SrcIP   net.IP
	DstPort uint16
	Proto   uint8
	Family  uint8
}

// rawEvent matches the memory layout of the C struct exactly.
type rawEvent struct {
	SrcIP   [16]byte
	DstPort uint16
	Proto   uint8
	Family  uint8
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

		var ip net.IP
		if raw.Family == 10 { // AF_INET6
			ip = make(net.IP, 16)
			copy(ip, raw.SrcIP[:])
		} else { // AF_INET
			ip = net.IP(raw.SrcIP[:4])
		}

		fn(Event{
			SrcIP:   ip,
			DstPort: raw.DstPort,
			Proto:   raw.Proto,
			Family:  raw.Family,
		})
	}
}

func (c *Collector) Close() {
	c.tp.Close()
	c.objs.Close()
}

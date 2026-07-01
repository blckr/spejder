package ebpf

import "bytes"

func newByteReader(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}

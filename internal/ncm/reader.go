package ncm

import (
	"encoding/binary"
	"fmt"
	"io"
)

// readUint reads a little-endian unsigned integer from r.
//
// The size of the value read is taken from the type parameter.
func readUint[T uint8 | uint16 | uint32 | uint64](r io.Reader) (T, error) {
	var value T
	if err := binary.Read(r, binary.LittleEndian, &value); err != nil {
		return 0, fmt.Errorf("read little-endian integer: %w", err)
	}
	return value, nil
}

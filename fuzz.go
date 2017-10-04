// +build gofuzz

package flac

import (
	"bytes"
)

func Fuzz(data []byte) int {
	_, _, err := Decode(bytes.NewReader(data))
	if err != nil {
		return 0
	}

	return 1
}

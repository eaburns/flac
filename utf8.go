package flac

import (
	"errors"
	"io"

	"github.com/eaburns/bit"
)

func utf8Decode(br *bit.Reader) (uint64, error) {
	left := 0
	v := uint64(0)

	switch b0, err := br.Read(8); {
	case err != nil:
		return 0, err

	// 0xxx xxxx
	case b0&0x80 == 0:
		return uint64(b0 & 0x7F), nil

	// 110x xxxx	10xx xxxx
	case b0&0xE0 == 0xC0:
		left = 1
		v = uint64(b0 & 0x1F)

	// 1110 xxxx	10xx xxxx	10xx xxxx
	case b0&0xF0 == 0xE0:
		left = 2
		v = uint64(b0 & 0xF)

	// 1111 0xxx	10xx xxxx	10xx xxxx	10xx xxxx
	case b0&0xF8 == 0xF0:
		left = 3
		v = uint64(b0 & 0x7)

	// 1111 10xx	10xx xxxx	10xx xxxx	10xx xxxx	10xx xxxx
	case b0&0xFC == 0xF8:
		left = 4
		v = uint64(b0 & 0x3)

	// 1111 110x	10xx xxxx	10xx xxxx	10xx xxxx	10xx xxxx	10xx xxxx
	case b0&0xFE == 0xFC:
		left = 5
		v = uint64(b0 & 0x1)
	}

	for n := 0; n < left; n++ {
		switch b, err := br.Read(8); {
		case err == io.EOF:
			return 0, io.ErrUnexpectedEOF

		case err != nil:
			return 0, err

		case b&0xC0 != 0x80:
			return 0, errors.New("Bad UTF-8 encoding in frame header")

		default:
			v = (v << 6) | (b & 0x3F)
		}
	}

	return v, nil
}

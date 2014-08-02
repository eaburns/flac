// © 2014 the flac Authors under the MIT license. See AUTHORS for the list of authors.

package flac

import (
	"bytes"
	"testing"

	"github.com/eaburns/bit"
)

func TestUTF8Decode(t *testing.T) {
	tests := []struct {
		data []byte
		val  uint64
	}{
		{[]byte{0x7F}, 0x7F},

		{[]byte{0xC2, 0xA2}, 0xA2},
		{[]byte{0xC2, 0x80}, 0x080},
		{[]byte{0xDF, 0xBF}, 0x7FF},

		{[]byte{0xE2, 0x82, 0xAC}, 0x20AC},
		{[]byte{0xE0, 0xA0, 0x80}, 0x800},
		{[]byte{0xEF, 0xBF, 0xBF}, 0xFFFF},

		{[]byte{0xF0, 0x90, 0x80, 0x80}, 0x10000},
		{[]byte{0xF7, 0xBF, 0xBF, 0xBF}, 0x1FFFFF},
		{[]byte{0xF0, 0xA4, 0xAD, 0xA2}, 0x24B62},

		{[]byte{0xF8, 0x88, 0x80, 0x80, 0x80}, 0x200000},
		{[]byte{0xFB, 0xBF, 0xBF, 0xBF, 0xBF}, 0x3FFFFFF},

		{[]byte{0xFC, 0x84, 0x80, 0x80, 0x80, 0x80}, 0x4000000},
		{[]byte{0xFD, 0xBF, 0xBF, 0xBF, 0xBF, 0xBF}, 0x7FFFFFFF},
	}

	for _, test := range tests {
		br := bit.NewReader(bytes.NewReader(test.data))
		switch v, err := utf8Decode(br); {
		case err != nil:
			t.Errorf("Unexpected error decoding %v: %v", test.data, err)

		case v != test.val:
			t.Errorf("Expected %v to decode to %v, got %v", test.data, test.val, v)
		}
	}
}

func TestNewDecoderError(t *testing.T) {
	tests := []struct {
		data []byte
		str  string
	}{
		{[]byte("foobar"), "Bad fLaC magic header"},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x7F, 0x00, 0x00, 0x00, 0x01, // Bad block type
			},
			"Invalid metedata block type (127)",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x81, 0x00, 0x00, 0x00, 0x01, // last metadata header: 1 byte padding.
				0x00,
			},
			"Missing STREAMINFO header",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x04, 0x40, 0, 0, 0, 0, // rate 0, 2 channels, 8 bits/sample, 0 samples
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.
			},
			"Bad sample rate",
		},
	}

	for _, test := range tests {
		_, err := NewDecoder(bytes.NewReader(test.data))
		if err.Error() != test.str {
			t.Errorf("Expected %s, got %v", test.str, err)
		}
	}
}

func TestReadFrameHeaderError(t *testing.T) {
	tests := []struct {
		data []byte
		str  string
	}{
		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Bad sync code.
				0x00, 0x00,
			},
			"Failed to find the synchronize code for the next frame",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 1 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xFB,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x19,

				// 2 channels · 8 bits per sample · 0 reserved
				// 0010 · 001 · 0
				0x22,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Invalid reserved value in frame header",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 0 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xF9,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x19,

				// 2 channels · 8 bits per sample · 1 reserved
				// 0010 · 001 · 1
				0x23,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Invalid reserved value in frame header",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 0 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xF9,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x09,

				// 2 channels · 8 bits per sample · 0 reserved
				// 0010 · 001 · 0
				0x22,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Bad block size in frame header",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 0 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xF9,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x19,

				// 2 channels · bad bits per sample · 0 reserved
				// 0010 · 011 · 0
				0x26,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Bad sample size in frame header",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 0 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xF9,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x19,

				// 2 channels · bad bits per sample · 0 reserved
				// 0010 · 111 · 0
				0x2E,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Bad sample size in frame header",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 0 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xF9,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x19,

				// bad channels · 8 bits per sample · 0 reserved
				// 1011 · 001 · 0
				0xB2,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Bad channel assignment",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 0 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xF9,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x19,

				// bad channels · 8 bits per sample · 0 reserved
				// 1100 · 001 · 0
				0xC2,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Bad channel assignment",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 0 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xF9,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x19,

				// bad channels · 8 bits per sample · 0 reserved
				// 1101 · 001 · 0
				0xD2,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Bad channel assignment",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 0 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xF9,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x19,

				// bad channels · 8 bits per sample · 0 reserved
				// 1110 · 001 · 0
				0xE2,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Bad channel assignment",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 0 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xF9,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x19,

				// bad channels · 8 bits per sample · 0 reserved
				// 1111 · 001 · 0
				0xF2,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Bad channel assignment",
		},

		{
			[]byte{
				'f', 'L', 'a', 'C',
				0x80, 0, 0, 34, // last metadata header: stream info.

				// STREAMINFO
				0, 0, // min block size
				0, 0, // max block size
				0, 0, 0, // min frame size
				0, 0, 0, // max frame size
				0, 0, 0x14, 0x70, 0, 0, 0, 1, // rate 1, 2 channels, 8 bits/sample, 1 sample
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // MD5, obviously not the true value.

				// Sync code · 0 reserved · fixed blocking
				// 1111 1111, 1111 10 · 0 · 1
				0xFF, 0xF9,

				// 192 block size · 44.1 kHz sample rate
				// 0001 · 1001
				0x19,

				// 2 channels · 8 bits per sample · 0 reserved
				// 0010 · 001 · 0
				0x22,

				// UTF8 frame number 0—frame number since fixed size
				0x00,

				// CRC8—invalid
				0x00,
			},
			"Bad checksum",
		},
	}

	for _, test := range tests {
		d, err := NewDecoder(bytes.NewReader(test.data))
		if err != nil {
			panic("Unexpected error making a new decoder: " + err.Error())
		}
		if _, err = readFrameHeader(d.r, d.StreamInfo); err.Error() != test.str {
			t.Errorf("Expected %s, got %v", test.str, err)
		}
	}
}

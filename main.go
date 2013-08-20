// +build ignore

package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/eaburns/flac"
)

func main() {
	d, err := flac.NewDecoder(bufio.NewReader(os.Stdin))
	if err != nil {
		fmt.Println(err.Error())
	}

	var data []byte
	for {
		d, err := d.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			fmt.Println(err.Error())
			break
		}
		data = append(data, d...)
	}

	writeWAV(data)

	h := md5.New()
	h.Write(data)

	if !reflect.DeepEqual(h.Sum(nil), d.MD5[:]) {
		fmt.Printf("Header MD5: %x\n", d.MD5)
		fmt.Printf("MD5: %x\n", h.Sum(nil))
		os.Exit(1)
	}
}

func writeWAV(data []byte) {
	wdata := bytes.NewBuffer(nil)
	wdata.WriteString("WAVE")

	wdata.WriteString("fmt ")
	binary.Write(wdata, binary.LittleEndian, uint32(16))
	wdata.Write([]byte{
		0x01, 0x00, // PCM format
		0x02, 0x00, // 2 interleaved channels
		0x44, 0xAC, 0x00, 0x00, // sample rate: 44100 Hz
		0x10, 0xb1, 0x02, 0x00, // data rate: 176400 = 44100*4 bytes/sec (4 = 2 channels, 2 bytes per channel)
		0x04, 0x00, // data block size in bytes—whatever that means
		0x10, 0x00, // bits per sample: 16
	})
	wdata.WriteString("data")
	binary.Write(wdata, binary.LittleEndian, uint32(len(data)))
	wdata.Write(data)

	wav, err := os.Create("out.wav")
	if err != nil {
		panic(err)
	}
	defer wav.Close()

	wav.WriteString("RIFF")
	binary.Write(wav, binary.LittleEndian, uint32(len(wdata.Bytes())))
	wav.Write(wdata.Bytes())
}

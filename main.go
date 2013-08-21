// +build ignore

package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/eaburns/flac"

//	"github.com/davecheney/profile"
)

func main() {
	//	defer profile.Start(profile.CPUProfile).Stop()
	data, meta, err := flac.Decode(bufio.NewReader(os.Stdin))
	if err != nil {
		fmt.Println(err.Error())
	}
	writeWAV(data, meta)
}

type wavFmt struct {
	format        int16
	channels      int16
	sampleRate    int32
	dataRate      int32
	dataBlockSize int16
	bitsPerSample int16
}

const pcmFormat = 1

func writeWAV(data []byte, meta flac.MetaData) {
	wdata := bytes.NewBuffer(nil)
	wdata.WriteString("WAVE")

	wdata.WriteString("fmt ")
	binary.Write(wdata, binary.LittleEndian, uint32(16))
	binary.Write(wdata, binary.LittleEndian, wavFmt{
		format:        pcmFormat,
		channels:      int16(meta.NChannels),
		sampleRate:    int32(meta.SampleRate),
		dataRate:      int32(meta.NChannels * meta.SampleRate * (meta.BitsPerSample / 8)),
		dataBlockSize: int16(meta.NChannels * (meta.BitsPerSample / 8)),
		bitsPerSample: int16(meta.BitsPerSample),
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

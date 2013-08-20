// Package flac implements a Free Lossless Audio Codec (FLAC) decoder.
package flac

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"

	"github.com/eaburns/bit"
)

var magic = [4]byte{'f', 'L', 'a', 'C'}

// A Decoder decodes a FLAC audio file.
type Decoder struct {
	r io.Reader
	// N is the next frame number.
	n int

	MetaData
}

// MetaData contains metadata header information from a FLAC file header.
type MetaData struct {
	*StreamInfo
	*VorbisComment
}

// StreamInfo contains information about the FLAC stream.
type StreamInfo struct {
	MinBlock      int
	MaxBlock      int
	MinFrame      int
	MaxFrame      int
	SampleRate    int
	NChannels     int
	BitsPerSample int
	TotalSamples  int
	MD5           [md5.Size]byte
}

// VorbisComment (a.k.a. FLAC tags) contains Vorbis-style comments that are
// human-readable textual information.
type VorbisComment struct {
	Vendor   string
	Comments []string
}

// NewDecoder reads the FLAC header information and returns a new Decoder.
// If an error is encountered while reading the header information then nil is
// returned along with the error.
func NewDecoder(r io.Reader) (*Decoder, error) {
	err := checkMagic(r)
	if err != nil {
		return nil, err
	}

	d := &Decoder{r: r}
	if d.MetaData, err = readMetaData(d.r); err != nil {
		return nil, err
	}
	if d.StreamInfo == nil {
		return nil, errors.New("Missing STREAMINFO header")
	}
	return d, nil
}

func checkMagic(r io.Reader) error {
	var m [4]byte
	if _, err := io.ReadFull(r, m[:]); err != nil {
		return err
	}
	if m != magic {
		return errors.New("Bad fLaC magic header")
	}
	return nil
}

type blockType int

const (
	streamInfoType    blockType = 0
	paddingType       blockType = 1
	applicationType   blockType = 2
	seekTableType     blockType = 3
	vorbisCommentType blockType = 4
	cueSheetType      blockType = 5
	pictureType       blockType = 6

	invalidBlockType = 127
)

var blockTypeNames = map[blockType]string{
	streamInfoType:    "STREAMINFO",
	paddingType:       "PADDING",
	applicationType:   "APPLICATION",
	seekTableType:     "SEEKTABLE",
	vorbisCommentType: "VORBIS_COMMENT",
	cueSheetType:      "CUESHEET",
	pictureType:       "PICTURE",
}

func (t blockType) String() string {
	if n, ok := blockTypeNames[t]; ok {
		return n
	}
	if t == invalidBlockType {
		return "InvalidBlockType"
	}
	return "Unknown(" + strconv.Itoa(int(t)) + ")"
}

func readMetaData(r io.Reader) (MetaData, error) {
	var meta MetaData
	for {
		last, kind, n, err := readMetaDataHeader(r)
		if err != nil {
			return meta, errors.New("Failed to read metadata header: " + err.Error())
		}

		header := &io.LimitedReader{R: r, N: int64(n)}

		switch kind {
		case invalidBlockType:
			return meta, errors.New("Invalid metedata block type (127)")

		case streamInfoType:
			meta.StreamInfo, err = readStreamInfo(header)

		case vorbisCommentType:
			meta.VorbisComment, err = readVorbisComment(header)

		default:
			debug("Skipping a header: %s\n", kind)
		}

		if err != nil {
			return meta, err
		}

		// Junk any unread bytes.
		if _, err = io.Copy(ioutil.Discard, header); err != nil {
			return meta, errors.New("Failed to discard metadata: " + err.Error())
		}

		if last {
			break
		}
	}
	return meta, nil
}

func readMetaDataHeader(r io.Reader) (last bool, kind blockType, n int32, err error) {
	const headerSize = 32 // bits
	br := bit.NewReader(&io.LimitedReader{R: r, N: headerSize})
	fs, err := br.ReadFields(1, 7, 24)
	if err != nil {
		return false, 0, 0, err
	}
	return fs[0] == 1, blockType(fs[1]), int32(fs[2]), nil
}

func readStreamInfo(r io.Reader) (*StreamInfo, error) {
	fs, err := bit.NewReader(r).ReadFields(16, 16, 24, 24, 20, 3, 5, 36)
	if err != nil {
		return nil, err
	}
	info := &StreamInfo{
		MinBlock:      int(fs[0]),
		MaxBlock:      int(fs[1]),
		MinFrame:      int(fs[2]),
		MaxFrame:      int(fs[3]),
		SampleRate:    int(fs[4]),
		NChannels:     int(fs[5]) + 1,
		BitsPerSample: int(fs[6]) + 1,
		TotalSamples:  int(fs[7]),
	}

	csum, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(csum) != md5.Size {
		panic("Bad MD5 checksum size")
	}
	copy(info.MD5[:], csum)

	if info.SampleRate == 0 {
		return info, errors.New("Bad sample rate")
	}

	return info, nil
}

func readVorbisComment(r io.Reader) (*VorbisComment, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	cmnt := new(VorbisComment)
	cmnt.Vendor, data = vorbisString(data)

	n := binary.LittleEndian.Uint32(data)
	data = data[4:]

	for i := uint32(0); i < n; i++ {
		var s string
		s, data = vorbisString(data)
		cmnt.Comments = append(cmnt.Comments, s)
	}
	return cmnt, nil
}

func vorbisString(data []byte) (string, []byte) {
	n := binary.LittleEndian.Uint32(data)
	data = data[4:]
	return string(data[:n]), data[n:]
}

// Next returns the audio data from the next frame for each channel.
//
// The following list gives the order in which the channel data is returned:
// 	1 channel: mono
// 	2 channels: left, right
// 	3 channels: left, right, center
// 	4 channels: front left, front right, back left, back right
// 	5 channels: front left, front right, front center, back/surround left, back/surround right
// 	6 channels: front left, front right, front center, LFE, back/surround left, back/surround right
// 	7 channels: front left, front right, front center, LFE, back center, side left, side right
// 	8 channels: front left, front right, front center, LFE, back left, back right, side left, side right
func (d *Decoder) Next() ([][]int32, error) {
	defer func() { d.n++ }()

	raw := bytes.NewBuffer(nil)
	tee := io.TeeReader(d.r, raw)
	h, err := readFrameHeader(tee, d.StreamInfo)
	if err == io.EOF {
		return nil, err
	} else if err != nil {
		return nil, errors.New("Failed to read the frame header: " + err.Error())
	}

	debug("frame %d\n\t%+v\n", d.n, h)

	br := bit.NewReader(tee)
	data := make([][]int32, h.channelAssignment.nChannels())
	for ch := range data {
		debug("\tsubframe: %d\n", ch)
		bps := h.bitsPerSample(ch)

		switch kind, order, err := readSubFrameHeader(br); {
		case err != nil:
			return nil, err

		case kind == subFrameConstant:
			debug("\t\t%s\n", kind)
			v, err := br.Read(bps)
			if err != nil {
				return nil, err
			}
			u := signExtend(v, bps)
			data[ch] = make([]int32, h.blockSize)
			for j := range data[ch] {
				data[ch][j] = u
			}

		case kind == subFrameVerbatim:
			debug("\t\t%s\n", kind)
			data[ch] = make([]int32, h.blockSize)
			for j := range data[ch] {
				v, err := br.Read(bps)
				if err != nil {
					return nil, err
				}
				data[ch][j] = signExtend(v, bps)
			}

		case kind == subFrameFixed:
			debug("\t\t%s, predictor order=%d\n", kind, order)
			data[ch], err = decodeFixedSubFrame(br, bps, h.blockSize, order)
			if err != nil {
				return nil, err
			}

		case kind == subFrameLPC:
			debug("\t\t%s, predictor order=%d\n", kind, order)
			data[ch], err = decodeLPCSubFrame(br, bps, h.blockSize, order)
			if err != nil {
				return nil, err
			}

		default:
			debug("\t\t%s, predictor order=%d\n", kind, order)
			return nil, errors.New("Unimplemented")
		}
	}

	var crc16 [2]byte
	if _, err := io.ReadFull(tee, crc16[:]); err != nil {
		return nil, err
	}
	if err = verifyCRC16(raw.Bytes()); err != nil {
		return nil, err
	}

	fixChannels(data, h.channelAssignment)

	return data, nil
}

func fixChannels(data [][]int32, assign channelAssignment) {
	switch assign {
	case leftSide:
		for i, d0 := range data[0] {
			data[1][i] = d0 - data[1][i]
		}

	case rightSide:
		for i, d1 := range data[1] {
			data[0][i] += d1
		}

	case midSide:
		for i, mid := range data[0] {
			side := data[1][i]
			mid *= 2
			mid |= (side & 1) // if side is odd
			data[0][i] = (mid + side) / 2
			data[1][i] = (mid - side) / 2
		}
	}
}

type frameHeader struct {
	variableSize      bool
	blockSize         int // Number of inter-channel samples.
	sampleRate        int // In Hz.
	channelAssignment channelAssignment
	sampleSize        int    // Bits
	number            uint64 // Sample number if variableSize is true, otherwise frame number.
	crc8              uint8
}

type channelAssignment int

var (
	leftSide  channelAssignment = 8
	rightSide channelAssignment = 9
	midSide   channelAssignment = 10
)

func (c channelAssignment) nChannels() int {
	n := 2
	if c < 8 {
		n = int(c) + 1
	}
	return n
}

func (h *frameHeader) bitsPerSample(subframe int) uint {
	b := uint(h.sampleSize)
	switch {
	case h.channelAssignment == leftSide && subframe == 1:
		b++
	case h.channelAssignment == rightSide && subframe == 0:
		b++
	case h.channelAssignment == midSide && subframe == 1:
		b++
	}
	return b
}

var (
	blockSizes = [...]int{
		0:  -1, // Reserved.
		1:  192,
		2:  576,
		3:  1152,
		4:  2304,
		5:  4608,
		6:  -1, // Get 8 bit (blocksize-1) from end of header.
		7:  -1, // Get 16 bit (blocksize-1) from end of header.
		8:  256,
		9:  512,
		10: 1024,
		11: 2048,
		12: 4096,
		13: 8192,
		14: 16384,
		15: 23768,
	}

	sampleRates = [...]int{
		0:  -1, // Get from STREAMINFO metadata block.
		1:  88200,
		2:  176400,
		3:  192000,
		4:  8000,
		5:  16000,
		6:  220500,
		7:  24000,
		8:  32000,
		9:  44100,
		10: 48000,
		11: 96000,
		12: -1, // Get 8 bit sample rate (in kHz) from end of header.
		13: -1, // Get 16 bit sample rate (in Hz) from end of header.
		14: -1, // Get 16 bit sample rate (in tens of Hz) from end of header.
		15: -1, // Invalid, to prevent sync-fooling string of 1s.
	}

	sampleSizes = [...]int{
		0: -1, // Get from STREAMINFO metadata block.
		1: 8,
		2: 12,
		3: -1, // Reserved.
		4: 16,
		5: 20,
		6: 24,
		7: -1, // Reserved.
	}
)

func readFrameHeader(r io.Reader, info *StreamInfo) (*frameHeader, error) {
	raw := bytes.NewBuffer(nil)
	br := bit.NewReader(io.TeeReader(r, raw))

	const syncCode = 0x3FFE

	switch sync, err := br.Read(14); {
	case err == nil && sync != syncCode:
		return nil, errors.New("Failed to find the synchronize code for the next frame")
	case err != nil:
		return nil, err
	}

	fs, err := br.ReadFields(1, 1, 4, 4, 4, 3, 1)
	if err != nil {
		return nil, err
	}
	if fs[0] != 0 || fs[6] != 0 {
		return nil, errors.New("Invalid reserved value in frame header")
	}

	h := new(frameHeader)
	h.variableSize = fs[1] == 1

	blockSize := fs[2]

	sampleRate := fs[3]

	h.channelAssignment = channelAssignment(fs[4])
	if h.channelAssignment > midSide {
		return nil, errors.New("Bad channel assignment")
	}

	switch sampleSize := fs[5]; sampleSize {
	case 0:
		h.sampleSize = info.BitsPerSample
	case 3, 7:
		return nil, errors.New("Bad sample size in frame header")
	default:
		h.sampleSize = sampleSizes[sampleSize]
	}

	if h.number, err = utf8Decode(br); err != nil {
		return nil, err
	}

	switch blockSize {
	case 0:
		return nil, errors.New("Bad block size in frame header")
	case 6:
		debug("\t8 bit block size\n")
		sz, err := br.Read(8)
		if err != nil {
			return nil, err
		}
		h.blockSize = int(sz) + 1
	case 7:
		debug("\t16 bit block size\n")
		sz, err := br.Read(16)
		if err != nil {
			return nil, err
		}
		h.blockSize = int(sz) + 1
	default:
		h.blockSize = blockSizes[blockSize]
	}

	switch sampleRate {
	case 0:
		h.sampleRate = info.SampleRate
	case 12:
		debug("\t8 bit sample rate\n")
		r, err := br.Read(8)
		if err != nil {
			return nil, err
		}
		h.sampleRate = int(r)
	case 13:
		debug("\t16 bit sample rate\n")
		r, err := br.Read(16)
		if err != nil {
			return nil, err
		}
		h.sampleRate = int(r)
	case 14:
		debug("\t16 bit sample rate * 10\n")
		r, err := br.Read(16)
		if err != nil {
			return nil, err
		}
		h.sampleRate = int(r * 10)
	default:
		h.sampleRate = sampleRates[sampleRate]
	}

	crc8, err := br.Read(8)
	if err != nil {
		return nil, err
	}
	h.crc8 = byte(crc8)

	return h, verifyCRC8(raw.Bytes())
}

type subFrameKind int

const (
	subFrameConstant subFrameKind = 0x0
	subFrameVerbatim subFrameKind = 0x1
	subFrameFixed    subFrameKind = 0x8
	subFrameLPC      subFrameKind = 0x20
)

func (k subFrameKind) String() string {
	switch k {
	case subFrameConstant:
		return "SUBFRAME_CONSTANT"
	case subFrameVerbatim:
		return "SUBFRAME_VERBATIM"
	case subFrameFixed:
		return "SUBFRAME_FIXED"
	case subFrameLPC:
		return "SUBFRAME_LPC"
	default:
		return "Unknown(0x" + strconv.FormatInt(int64(k), 16) + ")"
	}
}

func readSubFrameHeader(br *bit.Reader) (kind subFrameKind, order int, err error) {
	switch pad, err := br.Read(1); {
	case err != nil:
		return 0, 0, err
	case pad != 0:
		debug("\t\tBad padding value\n")
	}

	switch k, err := br.Read(6); {
	case err != nil:
		return 0, 0, err

	case k == 0:
		kind = subFrameConstant

	case k == 1:
		kind = subFrameVerbatim

	case (k&0x3E == 0x02) || (k&0x3C == 0x04) || (k&0x30 == 0x10):
		return 0, 0, errors.New("Bad subframe type")

	case k&0x38 == 0x08:
		if order = int(k & 0x07); order > 4 {
			return 0, 0, errors.New("Bad subframe type")
		}
		kind = subFrameFixed

	case k&0x20 == 0x20:
		order = int(k&0x1F) + 1
		kind = subFrameLPC

	default:
		panic("Impossible!")
	}

	n := 0
	switch k, err := br.Read(1); {
	case err != nil:
		return 0, 0, err

	case k == 1:
		n++
		k = uint64(0)
		for k == 0 {
			if k, err = br.Read(1); err != nil {
				return 0, 0, err
			}
			n++
		}
	}
	debug("\t\t%d wasted bits\n", n)

	return kind, order, nil
}

var fixedCoeffs = [...][]int32{
	1: {1},
	2: {2, -1},
	3: {3, -3, 1},
	4: {4, -6, 4, -1},
}

func decodeFixedSubFrame(br *bit.Reader, sampleSize uint, blkSize int, predO int) ([]int32, error) {
	warm, err := readInts(br, predO, sampleSize)
	if err != nil {
		return nil, err
	}
	for i, w := range warm {
		debug("\t\twarm[%d]: %d\n", i, w)
	}

	residual, err := decodeResiduals(br, blkSize, predO)
	if err != nil {
		return nil, err
	}

	if predO == 0 {
		return residual, nil
	}

	return predict(fixedCoeffs[predO], warm, residual, 0), nil
}

func decodeLPCSubFrame(br *bit.Reader, sampleSize uint, blkSize int, predO int) ([]int32, error) {
	warm, err := readInts(br, predO, sampleSize)
	if err != nil {
		return nil, err
	}
	for i, w := range warm {
		debug("\t\twarm[%d]: %d\n", i, w)
	}

	prec, err := br.Read(4)
	if err != nil {
		return nil, err
	} else if prec == 0xF {
		return nil, errors.New("Bad LPC predictor precision")
	}
	prec++
	debug("\t\tprecision: %d\n", prec)

	s, err := br.Read(5)
	if err != nil {
		return nil, err
	}
	shift := int(signExtend(s, 5))
	debug("\t\tshift (quantization level): %d\n", shift)
	if shift < 0 {
		panic("What does a negative shift mean‽")
	}

	coeffs, err := readInts(br, predO, uint(prec))
	if err != nil {
		return nil, err
	}
	for i, c := range coeffs {
		debug("\t\tcoeff[%d]: %d\n", i, c)
	}

	residual, err := decodeResiduals(br, blkSize, predO)
	if err != nil {
		return nil, err
	}

	return predict(coeffs, warm, residual, shift), nil
}

func readInts(br *bit.Reader, n int, bits uint) ([]int32, error) {
	is := make([]int32, n)
	for i := range is {
		w, err := br.Read(bits)
		if err != nil {
			return nil, err
		}
		is[i] = signExtend(w, bits)
	}
	return is, nil
}

func predict(coeffs, warm, residual []int32, shift int) []int32 {
	data := make([]int32, len(warm)+len(residual))
	copy(data, warm)
	for i := len(warm); i < len(data); i++ {
		var sum int32
		for j, c := range coeffs {
			sum += c * data[i-j-1]
			data[i] = residual[i-len(warm)] + (sum >> uint(shift))
		}
	}
	return data
}

func decodeResiduals(br *bit.Reader, blkSize int, predO int) ([]int32, error) {
	var bits uint

	switch method, err := br.Read(2); {
	case err != nil:
		return nil, err
	case method == 0:
		bits = 4
	case method == 1:
		bits = 5
	default:
		return nil, errors.New("Bad residual method")
	}

	partO, err := br.Read(4)
	if err != nil {
		return nil, err
	}
	debug("\t\tpartition order: %d\n", partO)

	var residue []int32
	for i := 0; i < 1<<partO; i++ {
		M, err := br.Read(bits)
		if err != nil {
			return nil, err
		} else if (bits == 4 && M == 0xF) || (bits == 5 && M == 0x1F) {
			panic("Unsupported, unencoded residuals")
		}
		debug("\t\tparameter[%d]: %d\n", i, M)

		n := 0
		switch {
		case partO == 0:
			n = blkSize - predO
		case i > 0:
			n = blkSize / (1 << partO)
		default:
			n = (blkSize / (1 << partO)) - predO
		}

		r, err := riceDecode(br, n, uint(M))
		if err != nil {
			return nil, err
		}
		residue = append(residue, r...)
	}
	return residue, nil
}

func signExtend(v uint64, bits uint) int32 {
	if v&(1<<(bits-1)) != 0 {
		return int32(v | (^uint64(0))<<bits)
	}
	return int32(v)
}

func riceDecode(br *bit.Reader, n int, M uint) ([]int32, error) {
	ns := make([]int32, n)
	for i := 0; i < n; i++ {
		var q uint64
		for {
			switch b, err := br.Read(1); {
			case err != nil:
				return nil, err
			case b == 0:
				q++
				continue
			}
			break
		}

		u, err := br.Read(M)
		if err != nil {
			return nil, err
		}

		u |= (q << M)
		ns[i] = int32(u>>1) ^ -int32(u&1)
	}
	return ns, nil
}

// DebugWriter is an io.Writer to which debug information is logged.
// If DebugWriter is nil then nothing is logged.
var DebugWriter io.Writer

func debug(f string, vals ...interface{}) {
	if DebugWriter == nil {
		return
	}
	fmt.Fprintf(DebugWriter, f, vals...)
}

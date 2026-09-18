package capture

// Annex-B access-unit assembly for WebRTC.
//
// pion's H264 payloader treats each WriteSample as one RTP timestamp. If we
// emit one NAL per sample, x264 slice threads (or any multi-slice IDR) become
// separate "frames". Chrome then decodes the first slice (a thin strip of the
// desktop) and fills the rest with green.
//
// AUs are delimited by AUD NALs or by a VCL NAL whose first_mb_in_slice is 0
// while a picture is already buffered. SPS/PPS are cached and prepended to
// every IDR access unit.

const (
	nalNonIDR = 1
	nalIDR    = 5
	nalSEI    = 6
	nalSPS    = 7
	nalPPS    = 8
	nalAUD    = 9
	nalEOS    = 10
	nalEOB    = 11
	nalFiller = 12
)

var startCodeAnnexB = []byte{0x00, 0x00, 0x00, 0x01}

func nalTypeOf(nal []byte) byte {
	if len(nal) == 0 {
		return 0
	}
	return nal[0] & 0x1F
}

func isVCL(t byte) bool { return t >= nalNonIDR && t <= nalIDR }

// firstMBInSliceZero reports whether a slice NAL starts a new picture.
// first_mb_in_slice is ue(v); the exp-Golomb encoding of 0 is a single 1-bit,
// so the high bit of the first slice-header byte is set.
func firstMBInSliceZero(nal []byte) bool {
	if len(nal) < 2 {
		return true
	}
	return nal[1]&0x80 != 0
}

type auBuilder struct {
	sps, pps []byte
	pending  [][]byte
}

func (b *auBuilder) hasVCL() bool {
	for _, n := range b.pending {
		if isVCL(nalTypeOf(n)) {
			return true
		}
	}
	return false
}

func (b *auBuilder) Push(nal []byte) (au []byte, ok bool) {
	if len(nal) == 0 {
		return nil, false
	}
	t := nalTypeOf(nal)
	switch t {
	case nalSPS:
		b.sps = cloneNAL(nal)
		return nil, false
	case nalPPS:
		b.pps = cloneNAL(nal)
		return nil, false
	case nalAUD, nalEOS, nalEOB:
		return b.Flush()
	}

	if isVCL(t) && firstMBInSliceZero(nal) && b.hasVCL() {
		au, ok = b.Flush()
		b.pending = append(b.pending, cloneNAL(nal))
		return au, ok
	}
	b.pending = append(b.pending, cloneNAL(nal))
	return nil, false
}

func (b *auBuilder) Flush() ([]byte, bool) {
	if !b.hasVCL() {
		b.pending = b.pending[:0]
		return nil, false
	}
	var hasSPS, hasPPS, hasIDR bool
	for _, n := range b.pending {
		switch nalTypeOf(n) {
		case nalSPS:
			hasSPS = true
		case nalPPS:
			hasPPS = true
		case nalIDR:
			hasIDR = true
		}
	}

	out := make([]byte, 0, 64)
	if hasIDR {
		if !hasSPS && len(b.sps) > 0 {
			out = appendNAL(out, b.sps)
		}
		if !hasPPS && len(b.pps) > 0 {
			out = appendNAL(out, b.pps)
		}
	}
	for _, n := range b.pending {
		if t := nalTypeOf(n); t == nalSEI || t == nalFiller {
			continue
		}
		out = appendNAL(out, n)
	}
	b.pending = b.pending[:0]
	return out, len(out) > 0
}

func appendNAL(dst, nal []byte) []byte {
	dst = append(dst, startCodeAnnexB...)
	return append(dst, nal...)
}

func cloneNAL(nal []byte) []byte {
	out := make([]byte, len(nal))
	copy(out, nal)
	return out
}

func countStartCodes(au []byte) int {
	n := 0
	for i := 0; i+4 <= len(au); i++ {
		if au[i] == 0 && au[i+1] == 0 && au[i+2] == 0 && au[i+3] == 1 {
			n++
			i += 3
		}
	}
	return n
}

// Package words provides a scanner for Unicode text segmentation word boundaries: https://unicode.org/reports/tr29/#Word_Boundaries
// It does not implement the entire specification, but many of the most important rules.
package words

import (
	"bufio"
	"io"
	"unicode"

	"github.com/clipperhouse/uax29/emoji"
)

// NewScanner tokenizes a reader into a stream of tokens. Iterate through the stream by calling Scan() or Next().
//
// Its uses several specs from Unicode Text Segmentation word boundaries https://unicode.org/reports/tr29/#Word_Boundaries. It's not a full implementation, but a decent approximation for many mainstream cases.
//
// It returns all tokens (including white space), so text can be reconstructed with fidelity.
func NewScanner(r io.Reader) *Scanner {
	return &Scanner{
		incoming: bufio.NewReaderSize(r, 64*1024),
	}
}

// Scanner is the structure for scanning an input Reader. Use NewScanner to instantiate.
type Scanner struct {
	incoming *bufio.Reader
	buffer   []rune

	// a cursor for _accepted_ runes in the buffer
	// the current accepted token is buffer[:pos] and lookahead is buffer[pos:]
	pos int

	text string
	err  error
}

// Scan advances to the next token, returning true if successful. Returns false on error or EOF.
func (sc *Scanner) Scan() bool {
	for {
		for len(sc.buffer) < sc.pos+2 {
			current, eof, err := sc.readRune()
			if err != nil {
				sc.err = err
				return false
			}
			if eof {
				break
			}
			sc.buffer = append(sc.buffer, current)
		}

		switch {
		case sc.wb1():
			// true indicates continue
			sc.accept()
			continue
		case sc.wb2():
			// true indicates break
			sc.text = sc.token()
			sc.err = nil
			return sc.text != ""
		}

		current := sc.buffer[sc.pos]

		var lookahead rune
		if len(sc.buffer) > sc.pos+1 {
			lookahead = sc.buffer[sc.pos+1]
		}

		switch {
		case
			sc.wb3(current):
			// true indicates continue
			sc.accept()
			continue
		case
			sc.wb3a(),
			sc.wb3b(current):
			// true indicates break
			goto breaking
		case
			sc.wb3c(current),
			sc.wb3d(current),
			sc.wb4(current),
			sc.wb5(current),
			sc.wb6(current, lookahead),
			sc.wb7(current),
			sc.wb7a(current),
			sc.wb7b(current, lookahead),
			sc.wb7c(current),
			sc.wb8(current),
			sc.wb9(current),
			sc.wb10(current),
			sc.wb11(current),
			sc.wb12(current, lookahead),
			sc.wb13(current),
			sc.wb13a(current),
			sc.wb13b(current),
			sc.wb15(current),
			sc.wb16(current):
			// true indicates continue
			sc.accept()
			continue
		}

	breaking:
		// If we fall through all the above rules, it's a word break
		// wb999 implements https://unicode.org/reports/tr29/#WB999

		sc.text = sc.token()
		sc.err = nil
		return true
	}
}

// Text returns the current token, given a successful call to Scan
func (sc *Scanner) Text() string {
	return sc.text
}

// Err returns the current error, given an unsuccessful call to Scan
func (sc *Scanner) Err() error {
	return sc.err
}

// Word boundary rules: https://unicode.org/reports/tr29/#Word_Boundaries
// In most cases, returning true means 'keep going'; check the name of the return var for clarity

var is = unicode.Is

// wb1 implements https://unicode.org/reports/tr29/#WB1
func (sc *Scanner) wb1() (continues bool) {
	sot := sc.pos == 0 // "start of text"
	eof := len(sc.buffer) == sc.pos
	return sot && !eof
}

// wb2 implements https://unicode.org/reports/tr29/#WB2
func (sc *Scanner) wb2() (breaks bool) {
	// eof
	return len(sc.buffer) == sc.pos
}

// wb3 implements https://unicode.org/reports/tr29/#WB3
func (sc *Scanner) wb3(current rune) (continues bool) {
	if !is(LF, current) {
		return false
	}
	previous := sc.buffer[sc.pos-1]
	return is(CR, previous)
}

// wb3a implements https://unicode.org/reports/tr29/#WB3a
func (sc *Scanner) wb3a() (breaks bool) {
	previous := sc.buffer[sc.pos-1]
	return is(CR, previous) || is(LF, previous) || is(Newline, previous)
}

// wb3b implements https://unicode.org/reports/tr29/#WB3b
func (sc *Scanner) wb3b(current rune) (breaks bool) {
	return is(CR, current) || is(LF, current) || is(Newline, current)
}

// wb3c implements https://unicode.org/reports/tr29/#WB3c
func (sc *Scanner) wb3c(current rune) (continues bool) {
	if !is(emoji.Extended_Pictographic, current) {
		return false
	}
	previous := sc.buffer[sc.pos-1]
	return is(ZWJ, previous)
}

// wb3d implements https://unicode.org/reports/tr29/#WB3d
func (sc *Scanner) wb3d(current rune) (continues bool) {
	if !is(WSegSpace, current) {
		return false
	}
	previous := sc.buffer[sc.pos-1]
	return is(WSegSpace, previous)
}

// wb4 implements https://unicode.org/reports/tr29/#WB4
func (sc *Scanner) wb4(current rune) (continues bool) {
	return is(ExtendFormatZWJ, current)
}

// seekPrevious works backward from the end of the buffer
// - skipping (ignoring) ExtendFormatZWJ
// - testing that the last rune is in any of the range tables
// Logic is here: https://unicode.org/reports/tr29/#Grapheme_Cluster_and_Format_Rules (driven by WB4)
func (sc *Scanner) seekPreviousIndex(pos int, rts ...*unicode.RangeTable) int {
	// Start at the end of the buffer and move backwards
	for i := pos - 1; i >= 0; i-- {
		r := sc.buffer[i]

		// Ignore ExtendFormatZWJ
		if is(ExtendFormatZWJ, r) {
			continue
		}

		// See if any of the range tables apply
		for _, rt := range rts {
			if is(rt, r) {
				return i
			}
		}

		// If we get this far, it's false
		break
	}

	return -1
}

// seekPrevious works backward from the end of the buffer
// - skipping (ignoring) ExtendFormatZWJ
// - testing that the last rune is in any of the range tables
// Logic is here: https://unicode.org/reports/tr29/#Grapheme_Cluster_and_Format_Rules (driven by WB4)
func (sc *Scanner) seekPrevious(pos int, rts ...*unicode.RangeTable) bool {
	return sc.seekPreviousIndex(pos, rts...) >= 0
}

// wb5 implements https://unicode.org/reports/tr29/#WB5
func (sc *Scanner) wb5(current rune) (continues bool) {
	if !is(AHLetter, current) {
		return false
	}
	return sc.seekPrevious(sc.pos, AHLetter)
}

// wb6 implements https://unicode.org/reports/tr29/#WB6
func (sc *Scanner) wb6(current, lookahead rune) (continues bool) {
	if !(is(MidLetter, current) || is(MidNumLetQ, current)) {
		return false
	}

	if !is(AHLetter, lookahead) {
		return false
	}

	return sc.seekPrevious(sc.pos, AHLetter)
}

// wb7 implements https://unicode.org/reports/tr29/#WB7
func (sc *Scanner) wb7(current rune) (continues bool) {
	if sc.pos < 2 {
		return false
	}

	if !(is(AHLetter, current) || is(ExtendFormatZWJ, current)) {
		return false
	}

	previous := sc.seekPreviousIndex(sc.pos, MidLetter, MidNumLetQ)
	if previous < 0 {
		return false
	}

	return sc.seekPrevious(previous, AHLetter)
}

// wb7a implements https://unicode.org/reports/tr29/#WB7a
func (sc *Scanner) wb7a(current rune) (continues bool) {
	if !is(Single_Quote, current) {
		return false
	}

	return sc.seekPrevious(sc.pos, Hebrew_Letter)
}

// wb7b implements https://unicode.org/reports/tr29/#WB7b
func (sc *Scanner) wb7b(current, lookahead rune) (continues bool) {
	if !is(Double_Quote, current) {
		return false
	}

	if !is(Hebrew_Letter, lookahead) {
		return false
	}

	return sc.seekPrevious(sc.pos, Hebrew_Letter)
}

// wb7c implements https://unicode.org/reports/tr29/#WB7c
func (sc *Scanner) wb7c(current rune) (continues bool) {
	if sc.pos < 2 {
		return false
	}

	if !is(Hebrew_Letter, current) {
		return false
	}

	previous := sc.seekPreviousIndex(sc.pos, Double_Quote)
	if previous < 0 {
		return false
	}

	return sc.seekPrevious(previous, Hebrew_Letter)
}

// wb8 implements https://unicode.org/reports/tr29/#WB8
func (sc *Scanner) wb8(current rune) (continues bool) {
	if !is(Numeric, current) {
		return false
	}
	return sc.seekPrevious(sc.pos, Numeric)
}

// wb9 implements https://unicode.org/reports/tr29/#WB9
func (sc *Scanner) wb9(current rune) (continues bool) {
	if !is(Numeric, current) {
		return false
	}
	return sc.seekPrevious(sc.pos, AHLetter)
}

// wb10 implements https://unicode.org/reports/tr29/#WB10
func (sc *Scanner) wb10(current rune) (continues bool) {
	if !is(AHLetter, current) {
		return false
	}
	return sc.seekPrevious(sc.pos, Numeric)
}

// wb11 implements https://unicode.org/reports/tr29/#WB11
func (sc *Scanner) wb11(current rune) (continues bool) {
	if sc.pos < 2 {
		return false
	}

	if !is(Numeric, current) {
		return false
	}

	previous := sc.seekPreviousIndex(sc.pos, MidNum, MidNumLetQ)
	if previous < 0 {
		return false
	}

	return sc.seekPrevious(previous, Numeric)
}

// wb12 implements https://unicode.org/reports/tr29/#WB12
func (sc *Scanner) wb12(current, lookahead rune) (continues bool) {
	if !is(Numeric, lookahead) {
		return false
	}

	if !(is(MidNum, current) || is(MidNumLetQ, current)) {
		return false
	}

	return sc.seekPrevious(sc.pos, Numeric)
}

// wb13 implements https://unicode.org/reports/tr29/#WB13
func (sc *Scanner) wb13(current rune) (continues bool) {
	if !is(Katakana, current) {
		return false
	}
	return sc.seekPrevious(sc.pos, Katakana)
}

// wb13a implements https://unicode.org/reports/tr29/#WB13a
func (sc *Scanner) wb13a(current rune) (continues bool) {
	if !is(ExtendNumLet, current) {
		return false
	}
	return sc.seekPrevious(sc.pos, AHLetter, Numeric, Katakana, ExtendNumLet)
}

// wb13b implements https://unicode.org/reports/tr29/#WB13b
func (sc *Scanner) wb13b(current rune) (continues bool) {
	if !(is(AHLetter, current) || is(Numeric, current) || is(Katakana, current)) {
		return false
	}
	return sc.seekPrevious(sc.pos, ExtendNumLet)
}

// wb15 implements https://unicode.org/reports/tr29/#WB15
func (sc *Scanner) wb15(current rune) (continues bool) {
	if !is(Regional_Indicator, current) {
		return false
	}

	// Buffer comprised entirely of an odd number of RI, ignoring ExtendFormatZWJ
	count := 0
	for i := sc.pos - 1; i >= 0; i-- {
		r := sc.buffer[i]
		if is(ExtendFormatZWJ, r) {
			continue
		}
		if !is(Regional_Indicator, r) {
			return false
		}
		count++
	}
	odd := count > 0 && count%2 == 1
	return odd
}

// wb16 implements https://unicode.org/reports/tr29/#WB16
func (sc *Scanner) wb16(current rune) (continues bool) {
	if !is(Regional_Indicator, current) {
		return false
	}

	// Last n runes represent an odd number of RI, ignoring ExtendFormatZWJ
	count := 0
	for i := sc.pos - 1; i >= 0; i-- {
		r := sc.buffer[i]
		if is(ExtendFormatZWJ, r) {
			continue
		}
		if !is(Regional_Indicator, r) {
			break
		}
		count++
	}
	odd := count > 0 && count%2 == 1
	return odd
}

func (sc *Scanner) token() string {
	s := string(sc.buffer[:sc.pos])
	sc.buffer = sc.buffer[sc.pos:]
	sc.pos = 0

	return s
}

func (sc *Scanner) accept() {
	sc.pos++
}

// readRune gets the next rune, advancing the reader
func (sc *Scanner) readRune() (r rune, eof bool, err error) {
	r, _, err = sc.incoming.ReadRune()

	if err != nil {
		if err == io.EOF {
			return r, true, nil
		}
		return r, false, err
	}

	return r, false, nil
}

// peekRune peeks the next rune, without advancing the reader
func (sc *Scanner) peekRune() (r rune, eof bool, err error) {
	r, _, err = sc.incoming.ReadRune()

	if err != nil {
		if err == io.EOF {
			return r, true, nil
		}
		return r, false, err
	}

	// Unread ASAP!
	err = sc.incoming.UnreadRune()
	if err != nil {
		return r, false, err
	}

	return r, false, nil
}

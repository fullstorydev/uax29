// Package main generates tries of Unicode properties by calling go generate as the repository root
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/clipperhouse/uax29/gen/triegen"
)

type prop struct {
	url         string
	packagename string
}

func main() {
	props := []prop{
		{
			url:         "https://unicode.org/Public/emoji/12.0/emoji-data.txt",
			packagename: "emoji",
		},
		{
			url:         "https://www.unicode.org/Public/" + unicode.Version + "/ucd/auxiliary/WordBreakProperty.txt",
			packagename: "words",
		},
		{
			url:         "https://www.unicode.org/Public/" + unicode.Version + "/ucd/auxiliary/GraphemeBreakProperty.txt",
			packagename: "graphemes",
		},
		{
			url:         "https://www.unicode.org/Public/" + unicode.Version + "/ucd/auxiliary/SentenceBreakProperty.txt",
			packagename: "sentences",
		},
	}

	for _, prop := range props {
		err := generate(prop)
		if err != nil {
			panic(err)
		}
	}
}

var extendedPictographic []rune

func generate(prop prop) error {
	fmt.Println(prop.url)
	resp, err := http.Get(prop.url)
	if err != nil {
		return err
	}

	b := bufio.NewReader(resp.Body)

	runesByProperty := map[string][]rune{}
	for {
		s, err := b.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if len(s) == 0 {
			continue
		}

		if s[0] == '\n' || s[0] == '#' {
			continue
		}

		parts := strings.Split(s, ";")
		runes, err := getRuneRange(parts[0])
		if err != nil {
			return err
		}

		split2 := strings.Split(parts[1], "#")
		property := strings.TrimSpace(split2[0])

		runesByProperty[property] = append(runesByProperty[property], runes...)
	}

	// Words and graphemes need Extended_Pictographic property
	const key = "Extended_Pictographic"
	if prop.packagename == "emoji" {
		extendedPictographic = runesByProperty[key]
		// We don't need to generate emoji package
		return nil
	}
	if prop.packagename == "words" || prop.packagename == "graphemes" {
		runesByProperty[key] = extendedPictographic
	}

	// Keep the order stable
	properties := make([]string, 0, len(runesByProperty))
	for property := range runesByProperty {
		properties = append(properties, property)
	}
	sort.Strings(properties)

	iotasByProperty := map[string]uint64{}
	for i, property := range properties {
		iotasByProperty[property] = 1 << i
	}

	iotasByRune := map[rune]uint64{}
	for property, runes := range runesByProperty {
		for _, r := range runes {
			iotasByRune[r] = iotasByRune[r] | iotasByProperty[property]
		}
	}

	trie := triegen.NewTrie(prop.packagename)

	for r, iotas := range iotasByRune {
		trie.Insert(r, iotas)
	}

	err = write(prop, trie, iotasByProperty)
	if err != nil {
		return err
	}

	return nil
}

func getRuneRange(s string) ([]rune, error) {
	s = strings.TrimSpace(s)
	hilo := strings.Split(s, "..")
	lo64, err := strconv.ParseInt("0x"+hilo[0], 0, 32)
	if err != nil {
		return nil, err
	}

	lo := rune(lo64)
	runes := []rune{lo}

	if len(hilo) == 1 {
		return runes, nil
	}

	hi64, err := strconv.ParseInt("0x"+hilo[1], 0, 32)
	if err != nil {
		return nil, err
	}

	hi := rune(hi64)
	if hi == lo {
		return runes, nil
	}

	// Skip first, inclusive of last
	for r := lo + 1; r <= hi; r++ {
		runes = append(runes, r)
	}

	return runes, nil
}

func write(prop prop, trie *triegen.Trie, iotasByProperty map[string]uint64) error {
	buf := bytes.Buffer{}

	fmt.Fprintln(&buf, "package "+prop.packagename)
	fmt.Fprintln(&buf, "\n// generated by github.com/clipperhouse/uax29\n// from "+prop.url)
	fmt.Fprintln(&buf)

	// Keep the order stable
	properties := make([]string, 0, len(iotasByProperty))
	for property := range iotasByProperty {
		properties = append(properties, property)
	}
	sort.Strings(properties)

	inttype := ""
	len := len(properties)
	switch {
	case len < 8:
		inttype = "uint8"
	case len < 16:
		inttype = "uint16"
	case len < 32:
		inttype = "uint32"
	default:
		inttype = "uint64"
	}

	fmt.Fprintf(&buf, "type property %s\n\n", inttype)

	fmt.Fprintln(&buf, "var (")
	for i, property := range properties {
		fmt.Fprintf(&buf, "_%s property = 1 << %d\n", strings.ReplaceAll(property, "_", ""), i)
	}
	fmt.Fprintln(&buf, ")")

	_, err := triegen.Gen(&buf, prop.packagename, []*triegen.Trie{trie})
	if err != nil {
		return err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}

	dst, err := os.Create(prop.packagename + "/trie.go")
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = dst.Write(formatted)
	if err != nil {
		return err
	}

	return nil
}

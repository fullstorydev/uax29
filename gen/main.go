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

	"golang.org/x/text/unicode/rangetable"
)

type prop struct {
	url         string
	packagename string
}

func main() {
	props := []prop{
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
		{
			url:         "https://www.unicode.org/Public/13.0.0/ucd/emoji/emoji-data.txt",
			packagename: "emoji",
		},
	}

	for _, prop := range props {
		err := generate(prop)
		if err != nil {
			panic(err)
		}
	}
}

func generate(prop prop) error {
	fmt.Println(prop.url)
	resp, err := http.Get(prop.url)
	if err != nil {
		return err
	}

	b := bufio.NewReader(resp.Body)

	runesByCategory := map[string][]rune{}
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
		category := strings.TrimSpace(split2[0])

		runesByCategory[category] = append(runesByCategory[category], runes...)
	}

	rangeTables := map[string]*unicode.RangeTable{}
	for category, runes := range runesByCategory {
		rangeTables[category] = rangetable.New(runes...)
	}

	if prop.packagename == "words" {
		// Special case for underscore; it's not in the spec but we allow it mid-word
		// It's commonly used in handles and usernames, we choose to interpret as a single token
		// Some programming languages allow it for formatting numbers
		rangeTables["MidNumLet"] = rangetable.Merge(rangeTables["MidNumLet"], rangetable.New('_'))

		// Merged "macro" tables defined here: https://unicode.org/reports/tr29/#WB_Rule_Macros
		rangeTables["AHLetter"] = rangetable.Merge(rangeTables["ALetter"], rangeTables["Hebrew_Letter"])
		rangeTables["MidNumLetQ"] = rangetable.Merge(rangeTables["MidNumLet"], rangetable.New('\''))
	}

	err = write(prop, rangeTables)
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

func write(prop prop, rts map[string]*unicode.RangeTable) error {
	buf := bytes.Buffer{}

	fmt.Fprintln(&buf, "// generated by github.com/clipperhouse/uax29\n// from "+prop.url)
	fmt.Fprintln(&buf, "package "+prop.packagename)
	fmt.Fprintln(&buf, "import \"unicode\"")

	// Keep the write order stable
	categories := make([]string, 0, len(rts))
	for category := range rts {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	for _, category := range categories {
		rt := rts[category]
		fmt.Fprintf(&buf, "var %s = ", category)
		print(&buf, rt)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}

	dst, err := os.Create(prop.packagename + "/tables.go")
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

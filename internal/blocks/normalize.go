package blocks

import (
	stdhtml "html"
	"strings"
	"unicode"
)

// PrepareOptions controla a normalização.
type PrepareOptions struct {
	Estimator         Estimator
	MaxParagraphWords int // padrão 300; parágrafos maiores são divididos em frases
}

// Prepare normaliza, divide parágrafos patologicamente longos, estima tempo,
// calcula hashes e liga cada bloco ao heading que o possui.
//
// A divisão de parágrafos acontece aqui (e não no segmentador) para preservar
// a invariante de que um chunk é sempre um intervalo contíguo de blocos.
func Prepare(raw []Block, opts PrepareOptions) []Block {
	maxWords := opts.MaxParagraphWords
	if maxWords <= 0 {
		maxWords = 300
	}
	est := opts.Estimator.normalized()

	out := make([]Block, 0, len(raw))
	for _, b := range raw {
		if b.Kind == KindCode {
			b.Text = NormalizeCode(b.Text)
		} else {
			b.Text = NormalizeText(b.Text)
		}

		if b.Text == "" && b.HTML == "" && b.Kind != KindHR {
			continue
		}

		if b.Kind == KindParagraph && CountWords(b.Text) > maxWords {
			for _, part := range SplitParagraph(b.Text, maxWords) {
				out = append(out, Block{
					Kind:  b.Kind,
					Level: b.Level,
					Text:  part,
					HTML:  "<p>" + stdhtml.EscapeString(part) + "</p>",
				})
			}
			continue
		}

		out = append(out, b)
	}

	for i := range out {
		out[i].Ord = i
		out[i].Words, out[i].Seconds = est.Estimate(out[i].Kind, out[i].Text, out[i].HTML)
		out[i].Hash = TextHash(out[i].Kind, out[i].Text)
	}
	assignHeadingIDs(out)
	return out
}

func assignHeadingIDs(bs []Block) {
	stack := make([]int, 0, 8)
	for i := range bs {
		if bs[i].Kind.IsHeading() {
			for len(stack) > 0 && bs[stack[len(stack)-1]].Level >= bs[i].Level {
				stack = stack[:len(stack)-1]
			}
			bs[i].HeadingID = i
			stack = append(stack, i)
			continue
		}
		if len(stack) == 0 {
			bs[i].HeadingID = -1
		} else {
			bs[i].HeadingID = stack[len(stack)-1]
		}
	}
}

// SplitParagraph divide um parágrafo em pedaços de no máximo maxWords,
// sempre em fronteira de frase. Uma frase sozinha maior que o limite
// permanece inteira.
func SplitParagraph(text string, maxWords int) []string {
	if maxWords <= 0 || CountWords(text) <= maxWords {
		return []string{text}
	}

	sentences := SplitSentences(text)
	var parts []string
	var cur []string
	curWords := 0

	for _, s := range sentences {
		w := CountWords(s)
		if curWords > 0 && curWords+w > maxWords {
			parts = append(parts, strings.Join(cur, " "))
			cur = nil
			curWords = 0
		}
		cur = append(cur, s)
		curWords += w
	}
	if len(cur) > 0 {
		parts = append(parts, strings.Join(cur, " "))
	}
	if len(parts) == 0 {
		return []string{text}
	}
	return parts
}

// SplitSentences devolve frases completas, respeitando abreviações comuns
// do português. O separador final (ponto, exclamação, interrogação, reticências)
// permanece na frase.
func SplitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	runes := []rune(text)
	var out []string
	start := 0

	for i := 0; i < len(runes); i++ {
		if !isSentenceEnd(runes[i]) {
			continue
		}

		j := i + 1
		for j < len(runes) && isClosing(runes[j]) {
			j++
		}
		if j >= len(runes) || !unicode.IsSpace(runes[j]) {
			continue
		}

		k := j
		for k < len(runes) && unicode.IsSpace(runes[k]) {
			k++
		}
		if k >= len(runes) || !startsSentence(runes[k]) {
			continue
		}
		if isAbbreviation(runes, i) {
			continue
		}

		out = append(out, strings.TrimSpace(string(runes[start:j])))
		start = k
		i = k - 1
	}

	if start < len(runes) {
		if rest := strings.TrimSpace(string(runes[start:])); rest != "" {
			out = append(out, rest)
		}
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

func isSentenceEnd(r rune) bool {
	switch r {
	case '.', '!', '?', '…':
		return true
	}
	return false
}

func isClosing(r rune) bool {
	switch r {
	case '"', '\'', '”', '’', ')', ']', '»':
		return true
	}
	return false
}

func startsSentence(r rune) bool {
	if unicode.IsUpper(r) {
		return true
	}
	switch r {
	case '"', '\'', '“', '‘', '(', '[', '«':
		return true
	}
	return false
}

var abbreviations = map[string]bool{
	"sr.": true, "sra.": true, "dr.": true, "dra.": true,
	"prof.": true, "profa.": true, "etc.": true,
	"pág.": true, "pag.": true, "nº.": true, "no.": true,
	"art.": true, "cap.": true, "fig.": true, "ex.": true,
	"vol.": true, "ed.": true, "cf.": true, "vs.": true,
	"p.ex.": true, "a.c.": true, "d.c.": true,
	"av.": true, "tv.": true, "cel.": true, "tel.": true,
	"min.": true, "max.": true,
}

func isAbbreviation(runes []rune, dot int) bool {
	if dot >= len(runes) || runes[dot] != '.' {
		return false
	}
	start := dot
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	raw := string(runes[start : dot+1])
	if abbreviations[strings.ToLower(raw)] {
		return true
	}
	// Iniciais: "J." ou "J.M."
	if isInitials(raw) {
		return true
	}
	return false
}

func isInitials(token string) bool {
	rs := []rune(token)
	if len(rs) == 0 || rs[len(rs)-1] != '.' {
		return false
	}
	letters := 0
	for _, r := range rs {
		switch {
		case unicode.IsUpper(r):
			letters++
		case r == '.':
		default:
			return false
		}
	}
	return letters > 0
}

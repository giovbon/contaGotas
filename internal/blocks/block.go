package blocks

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Kind é o tipo semântico de um bloco.
type Kind string

const (
	KindH1        Kind = "h1"
	KindH2        Kind = "h2"
	KindH3        Kind = "h3"
	KindH4        Kind = "h4"
	KindH5        Kind = "h5"
	KindH6        Kind = "h6"
	KindParagraph Kind = "p"
	KindListItem  Kind = "li"
	KindCode      Kind = "code"
	KindTable     Kind = "table"
	KindQuote     Kind = "quote"
	KindFigure    Kind = "figure"
	KindFootnote  Kind = "footnote"
	KindHR        Kind = "hr"
)

// Block é a unidade atômica de conteúdo. Blocos são a única entrada do
// segmentador: nunca se corta texto por offset de caractere.
type Block struct {
	Ord       int    // posição no documento, contígua a partir de 0
	Kind      Kind   // tipo semântico
	Level     int    // nível do heading (1..6) quando aplicável
	HeadingID int    // índice do heading que possui o bloco; -1 se não houver
	Text      string // texto normalizado (para código: preserva indentação)
	HTML      string // markup seguro para renderização no leitor
	Words     int    // palavras (código: número de linhas)
	Seconds   int    // tempo estimado de leitura
	Hash      string // hash estável do conteúdo normalizado
}

// IsHeading informa se o bloco é um heading.
func (k Kind) IsHeading() bool {
	switch k {
	case KindH1, KindH2, KindH3, KindH4, KindH5, KindH6:
		return true
	}
	return false
}

// Indivisible informa se o bloco não pode ser partido pelo segmentador.
func (k Kind) Indivisible() bool {
	switch k {
	case KindCode, KindTable, KindQuote, KindFigure, KindListItem, KindFootnote, KindHR:
		return true
	}
	return false
}

// TextHash gera um identificador estável de 16 caracteres hexadecimais.
func TextHash(kind Kind, text string) string {
	sum := sha256.Sum256([]byte(string(kind) + "\x00" + text))
	return hex.EncodeToString(sum[:8])
}

// NormalizeText padroniza texto de prosa: NFC, sem soft hyphen, espaços
// colapsados. Não usar em blocos de código.
func NormalizeText(s string) string {
	s = norm.NFC.String(s)
	s = strings.ReplaceAll(s, "\u00ad", "")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	return strings.Join(strings.Fields(s), " ")
}

// NormalizeCode padroniza código preservando indentação e quebras de linha.
func NormalizeCode(s string) string {
	s = norm.NFC.String(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\u00ad", "")
	return strings.Trim(s, "\n")
}

// CountWords conta campos separados por espaço Unicode.
func CountWords(s string) int {
	return len(strings.Fields(s))
}

package blocks

import "strings"

// Item é um elemento da árvore: ou um bloco direto, ou um nó filho.
// A ordem dos itens é a ordem do documento.
type Item struct {
	Block int
	Child *Node
}

// Node representa uma seção delimitada por heading. O nó raiz tem Heading == -1.
type Node struct {
	Heading int // índice do bloco de heading; -1 para a raiz sintética
	Level   int
	Items   []Item
	Start   int // primeiro bloco coberto (inclusive)
	End     int // último bloco coberto (inclusive)
	Seconds int
	Words   int
}

// BuildTree constrói a hierarquia de seções a partir dos headings.
func BuildTree(bs []Block) *Node {
	root := &Node{Heading: -1, Level: 0}
	stack := []*Node{root}

	for i := range bs {
		b := bs[i]
		if b.Kind.IsHeading() {
			for len(stack) > 1 && stack[len(stack)-1].Level >= b.Level {
				stack = stack[:len(stack)-1]
			}
			parent := stack[len(stack)-1]
			n := &Node{Heading: i, Level: b.Level}
			parent.Items = append(parent.Items, Item{Child: n})
			stack = append(stack, n)
			continue
		}
		top := stack[len(stack)-1]
		top.Items = append(top.Items, Item{Block: i})
	}

	root.Compute(bs)
	return root
}

// Compute preenche tempo, palavras e intervalo de cada nó, bottom-up.
func (n *Node) Compute(bs []Block) {
	n.Seconds, n.Words = 0, 0
	n.Start, n.End = -1, -1

	if n.Heading >= 0 {
		n.Start = n.Heading
		n.End = n.Heading
		n.Seconds += bs[n.Heading].Seconds
		n.Words += bs[n.Heading].Words
	}

	for _, it := range n.Items {
		var s, e, sec, words int
		if it.Child != nil {
			it.Child.Compute(bs)
			s, e = it.Child.Start, it.Child.End
			sec, words = it.Child.Seconds, it.Child.Words
		} else {
			s, e = it.Block, it.Block
			sec, words = bs[it.Block].Seconds, bs[it.Block].Words
		}
		if n.Start < 0 {
			n.Start = s
		}
		n.End = e
		n.Seconds += sec
		n.Words += words
	}
}

// Splittable informa se o nó pode ser subdividido em itens menores.
func (n *Node) Splittable() bool {
	if len(n.Items) > 1 {
		return true
	}
	if len(n.Items) == 1 && n.Items[0].Child != nil {
		return n.Items[0].Child.Splittable()
	}
	return false
}

// SectionPath devolve o caminho de headings que contém o bloco idx.
func SectionPath(bs []Block, idx int) []string {
	if idx < 0 || idx >= len(bs) {
		return nil
	}
	type levelText struct {
		level int
		text  string
	}
	var stack []levelText
	for i := 0; i <= idx; i++ {
		b := bs[i]
		if !b.Kind.IsHeading() {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].level >= b.Level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, levelText{level: b.Level, text: b.Text})
	}
	out := make([]string, 0, len(stack))
	for _, s := range stack {
		out = append(out, s.text)
	}
	return out
}

// HeadingText devolve o texto do heading que possui o bloco idx.
func HeadingText(bs []Block, idx int) string {
	if idx < 0 || idx >= len(bs) {
		return ""
	}
	id := bs[idx].HeadingID
	if id < 0 || id >= len(bs) {
		return ""
	}
	return bs[id].Text
}

// JoinPath junta um caminho de seções para exibição.
func JoinPath(path []string) string {
	if len(path) == 0 {
		return "—"
	}
	return strings.Join(path, " › ")
}

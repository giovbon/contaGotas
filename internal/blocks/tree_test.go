package blocks

import "testing"

func flatBlocks() []Block {
	return []Block{
		{Ord: 0, Kind: KindH1, Level: 1, HeadingID: 0, Text: "A", Seconds: 1, Words: 1},
		{Ord: 1, Kind: KindParagraph, HeadingID: 0, Text: "um", Seconds: 1, Words: 10},
		{Ord: 2, Kind: KindH2, Level: 2, HeadingID: 2, Text: "B", Seconds: 1, Words: 1},
		{Ord: 3, Kind: KindParagraph, HeadingID: 2, Text: "dois", Seconds: 1, Words: 10},
		{Ord: 4, Kind: KindParagraph, HeadingID: 2, Text: "três", Seconds: 1, Words: 10},
		{Ord: 5, Kind: KindH1, Level: 1, HeadingID: 5, Text: "C", Seconds: 1, Words: 1},
		{Ord: 6, Kind: KindParagraph, HeadingID: 5, Text: "quatro", Seconds: 1, Words: 10},
	}
}

func TestBuildTree(t *testing.T) {
	bs := flatBlocks()
	root := BuildTree(bs)

	if root.Start != 0 || root.End != 6 || root.Seconds != 7 {
		t.Fatalf("root = [%d..%d] %ds", root.Start, root.End, root.Seconds)
	}
	if len(root.Items) != 2 {
		t.Fatalf("root tem %d itens, want 2", len(root.Items))
	}

	a := root.Items[0].Child
	if a == nil || a.Heading != 0 || a.Start != 0 || a.End != 4 || a.Seconds != 5 {
		t.Fatalf("nó A inválido: %+v", a)
	}
	if len(a.Items) != 2 {
		t.Fatalf("nó A tem %d itens, want 2", len(a.Items))
	}

	b := a.Items[1].Child
	if b == nil || b.Heading != 2 || b.Start != 2 || b.End != 4 || b.Seconds != 3 {
		t.Fatalf("nó B inválido: %+v", b)
	}
	if !b.Splittable() {
		t.Fatal("B deveria ser divisível")
	}

	c := root.Items[1].Child
	if c == nil || c.Start != 5 || c.End != 6 {
		t.Fatalf("nó C inválido: %+v", c)
	}
	if c.Splittable() {
		t.Fatal("C não deveria ser divisível")
	}
}

func TestSectionPath(t *testing.T) {
	bs := flatBlocks()
	if got := SectionPath(bs, 4); len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("SectionPath(4) = %v", got)
	}
	if got := SectionPath(bs, 6); len(got) != 1 || got[0] != "C" {
		t.Fatalf("SectionPath(6) = %v", got)
	}
	if got := SectionPath(bs, 0); len(got) != 1 || got[0] != "A" {
		t.Fatalf("SectionPath(0) = %v", got)
	}
	if got := SectionPath(bs, 99); got != nil {
		t.Fatalf("SectionPath(99) = %v", got)
	}
}

func TestHeadingText(t *testing.T) {
	bs := flatBlocks()
	if got := HeadingText(bs, 4); got != "B" {
		t.Fatalf("HeadingText(4) = %q", got)
	}
	if got := HeadingText(bs, 6); got != "C" {
		t.Fatalf("HeadingText(6) = %q", got)
	}
}

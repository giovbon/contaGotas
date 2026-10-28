package store

import (
	"context"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
)

func TestBlocksReplaceList(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id, err := s.Documents().Create(ctx, docExemplo("Livro", "sha-blocks"))
	if err != nil {
		t.Fatal(err)
	}

	entrada := []blocks.Block{
		{Ord: 0, Kind: blocks.KindH1, Level: 1, HeadingID: 0, Text: "Capítulo", HTML: "<h1>Capítulo</h1>", Words: 1, Seconds: 1, Hash: "h1"},
		{Ord: 1, Kind: blocks.KindParagraph, HeadingID: 0, Text: "Texto.", HTML: "<p>Texto.</p>", Words: 1, Seconds: 1, Hash: "p1"},
		{Ord: 2, Kind: blocks.KindH2, Level: 2, HeadingID: 2, Text: "Seção", HTML: "<h2>Seção</h2>", Words: 1, Seconds: 1, Hash: "h2"},
		{Ord: 3, Kind: blocks.KindParagraph, HeadingID: 2, Text: "Mais.", HTML: "<p>Mais.</p>", Words: 1, Seconds: 1, Hash: "p2"},
	}
	if err := s.Blocks().Replace(ctx, id, entrada); err != nil {
		t.Fatalf("gravar blocos: %v", err)
	}

	saida, err := s.Blocks().List(ctx, id)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(saida) != len(entrada) {
		t.Fatalf("blocos = %d, quer %d", len(saida), len(entrada))
	}
	for i := range entrada {
		if saida[i].Ord != i {
			t.Errorf("bloco %d com ord %d", i, saida[i].Ord)
		}
		if saida[i].Text != entrada[i].Text || saida[i].Kind != entrada[i].Kind {
			t.Errorf("bloco %d = %+v", i, saida[i])
		}
	}
	if saida[1].HeadingID != 0 {
		t.Errorf("parágrafo deveria apontar para o índice 0, veio %d", saida[1].HeadingID)
	}
	if saida[3].HeadingID != 2 {
		t.Errorf("parágrafo deveria apontar para o índice 2, veio %d", saida[3].HeadingID)
	}
	if saida[0].HeadingID != 0 {
		t.Errorf("heading deveria apontar para si, veio %d", saida[0].HeadingID)
	}
}

func TestBlocksReplaceSubstitui(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id, err := s.Documents().Create(ctx, docExemplo("Livro", "sha-replace"))
	if err != nil {
		t.Fatal(err)
	}
	primeiro := []blocks.Block{
		{Ord: 0, Kind: blocks.KindParagraph, HeadingID: -1, Text: "a", Hash: "a"},
		{Ord: 1, Kind: blocks.KindParagraph, HeadingID: -1, Text: "b", Hash: "b"},
	}
	if err := s.Blocks().Replace(ctx, id, primeiro); err != nil {
		t.Fatal(err)
	}
	segundo := []blocks.Block{
		{Ord: 0, Kind: blocks.KindParagraph, HeadingID: -1, Text: "c", Hash: "c"},
	}
	if err := s.Blocks().Replace(ctx, id, segundo); err != nil {
		t.Fatal(err)
	}

	n, err := s.Blocks().Count(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("contagem = %d, quer 1", n)
	}
	saida, err := s.Blocks().List(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(saida) != 1 || saida[0].Text != "c" {
		t.Fatalf("saída = %+v", saida)
	}
	if saida[0].HeadingID != -1 {
		t.Errorf("heading_id = %d, quer -1", saida[0].HeadingID)
	}
}

func TestBlocksReplaceSemDocumento(t *testing.T) {
	s := abrirTeste(t)
	err := s.Blocks().Replace(context.Background(), 999, []blocks.Block{
		{Ord: 0, Kind: blocks.KindParagraph, HeadingID: -1, Text: "x", Hash: "x"},
	})
	if err == nil {
		t.Fatal("esperado erro de chave estrangeira")
	}
}

func TestBlocksCountVazio(t *testing.T) {
	s := abrirTeste(t)
	n, err := s.Blocks().Count(context.Background(), 123)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("contagem = %d, quer 0", n)
	}
}

func TestBlocksSection(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id, err := s.Documents().Create(ctx, docExemplo("Seções", "sha-section"))
	if err != nil {
		t.Fatal(err)
	}
	entrada := []blocks.Block{
		{Ord: 0, Kind: blocks.KindParagraph, HeadingID: -1, Text: "Abertura.", HTML: "<p>Abertura.</p>", Words: 1, Seconds: 1, Hash: "ab"},
		{Ord: 1, Kind: blocks.KindH1, Level: 1, HeadingID: 1, Text: "Um", HTML: "<h1>Um</h1>", Words: 1, Seconds: 1, Hash: "h1"},
		{Ord: 2, Kind: blocks.KindParagraph, HeadingID: 1, Text: "Corpo um.", HTML: "<p>Corpo um.</p>", Words: 1, Seconds: 1, Hash: "p2"},
		{Ord: 3, Kind: blocks.KindH2, Level: 2, HeadingID: 3, Text: "Um ponto um", HTML: "<h2>Um ponto um</h2>", Words: 1, Seconds: 1, Hash: "h2"},
		{Ord: 4, Kind: blocks.KindParagraph, HeadingID: 3, Text: "Corpo da subseção.", HTML: "<p>Corpo da subseção.</p>", Words: 1, Seconds: 1, Hash: "p4"},
		{Ord: 5, Kind: blocks.KindH1, Level: 1, HeadingID: 5, Text: "Dois", HTML: "<h1>Dois</h1>", Words: 1, Seconds: 1, Hash: "h1b"},
		{Ord: 6, Kind: blocks.KindParagraph, HeadingID: 5, Text: "Corpo dois.", HTML: "<p>Corpo dois.</p>", Words: 1, Seconds: 1, Hash: "p6"},
	}
	if err := s.Blocks().Replace(ctx, id, entrada); err != nil {
		t.Fatalf("gravar blocos: %v", err)
	}

	lista, err := s.Blocks().List(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	// Seção do capítulo 1 inclui a subseção e para no capítulo 2.
	secao, err := s.Blocks().Section(ctx, id, lista[2].HeadingID)
	if err != nil {
		t.Fatalf("seção: %v", err)
	}
	if len(secao) != 4 || secao[0].Text != "Um" || secao[3].Text != "Corpo da subseção." {
		t.Errorf("seção do capítulo = %+v", secao)
	}

	// Subseção isolada: só ela e seu corpo.
	sub, err := s.Blocks().Section(ctx, id, lista[4].HeadingID)
	if err != nil {
		t.Fatalf("subseção: %v", err)
	}
	if len(sub) != 2 || sub[0].Text != "Um ponto um" {
		t.Errorf("subseção = %+v", sub)
	}

	// Sem heading antes: abertura.
	abertura, err := s.Blocks().Section(ctx, id, lista[0].HeadingID)
	if err != nil {
		t.Fatalf("abertura: %v", err)
	}
	if len(abertura) != 1 || abertura[0].Text != "Abertura." {
		t.Errorf("abertura = %+v", abertura)
	}

	if _, err := s.Blocks().Section(ctx, id, 99); err == nil {
		t.Error("heading fora do documento deveria falhar")
	}
	if _, err := s.Blocks().Section(ctx, id, 2); err == nil {
		t.Error("índice que não é heading deveria falhar")
	}
	if vazia, err := s.Blocks().Section(ctx, id+999, 0); err != nil || vazia != nil {
		t.Errorf("documento sem blocos = %+v, %v", vazia, err)
	}
}

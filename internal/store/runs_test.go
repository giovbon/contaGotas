package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/plan"
)

func runComChunks(t *testing.T, s *Store) (int64, []int64) {
	t.Helper()
	ctx := context.Background()
	lista := blocosDeTeste(10, 60)
	docID := documentoComBlocos(t, s, lista)
	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1, WPM: 200, Difficulty: 1, MinFill: 0.6, MaxOvershoot: 1.4}
	pl, err := plan.Generate(lista, params)
	if err != nil {
		t.Fatalf("gerar plano: %v", err)
	}
	if _, err := s.Plans().Create(ctx, docID, pl, params, agora); err != nil {
		t.Fatalf("persistir plano: %v", err)
	}
	// Create não devolve os chunks persistidos (ids do banco); o plano ativo
	// é a fonte da verdade.
	ativo, err := s.Plans().Active(ctx, docID)
	if err != nil || ativo == nil {
		t.Fatalf("plano ativo: %+v, %v", ativo, err)
	}
	ids := make([]int64, 0, len(ativo.Chunks))
	for _, c := range ativo.Chunks {
		ids = append(ids, c.ID)
	}
	if len(ids) == 0 {
		t.Fatal("plano sem chunks")
	}
	return docID, ids
}

func TestRunsCreateIdempotente(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	run, criada, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatalf("criar: %v", err)
	}
	if !criada || run.ID == 0 || run.Status != RunRunning || run.Late {
		t.Fatalf("run = %+v, criada = %v", run, criada)
	}

	deNovo, criada, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, true)
	if err != nil {
		t.Fatalf("criar de novo: %v", err)
	}
	if criada {
		t.Error("segunda criação deveria ser ignorada")
	}
	if deNovo.ID != run.ID || deNovo.Late {
		t.Errorf("run devolvida = %+v", deNovo)
	}

	porData, err := s.Runs().GetByDate(ctx, "2026-10-27")
	if err != nil || porData == nil || porData.ID != run.ID {
		t.Fatalf("GetByDate = %+v, %v", porData, err)
	}
	if vazia, err := s.Runs().GetByDate(ctx, "2026-10-28"); err != nil || vazia != nil {
		t.Errorf("data sem run = %+v, %v", vazia, err)
	}

	emAndamento, err := s.Runs().Running(ctx)
	if err != nil || emAndamento == nil || emAndamento.ID != run.ID {
		t.Fatalf("Running = %+v, %v", emAndamento, err)
	}

	if _, _, err := s.Runs().Create(ctx, "27/10/2026", agora, agora, false); err == nil {
		t.Error("esperava erro para run_date inválida")
	}
	if _, _, err := s.Runs().Create(ctx, "2026-10-28", agora, agora, false); err != nil {
		t.Fatalf("criar outra: %v", err)
	}
	if err := s.Runs().FinishRun(ctx, run.ID, RunSent, "", agora); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if emAndamento, _ := s.Runs().Running(ctx); emAndamento == nil || emAndamento.RunDate != "2026-10-28" {
		t.Errorf("Running depois de fechar = %+v", emAndamento)
	}
	if err := s.Runs().FinishRun(ctx, run.ID, RunSent, "", agora); err == nil {
		t.Error("esperava erro ao fechar run já fechada")
	}
}

func TestRunsItemSentMarcaChunk(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	docID, chunks := runComChunks(t, s)
	run, _, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.Runs().AddItem(ctx, run.ID, docID, chunks[0])
	if err != nil {
		t.Fatalf("adicionar item: %v", err)
	}
	if item.Status != ItemPending || item.Attempts != 0 {
		t.Errorf("item = %+v", item)
	}

	if err := s.Runs().MarkItemSent(ctx, item.ID, chunks[0], 1234, 2, agora); err != nil {
		t.Fatalf("marcar enviado: %v", err)
	}
	itens, err := s.Runs().ListItems(ctx, run.ID)
	if err != nil || len(itens) != 1 {
		t.Fatalf("itens = %+v, %v", itens, err)
	}
	got := itens[0]
	if got.Status != ItemSent || got.Attempts != 2 || got.Bytes == nil || *got.Bytes != 1234 || got.SentAt == nil {
		t.Errorf("item gravado = %+v", got)
	}
	chunk, err := s.Plans().Chunk(ctx, chunks[0])
	if err != nil || chunk == nil {
		t.Fatalf("buscar chunk: %v", err)
	}
	if chunk.Status != ChunkSent || chunk.SentAt == nil {
		t.Errorf("chunk = %+v", chunk)
	}

	if err := s.Runs().FinishRun(ctx, run.ID, RunSent, "", agora); err != nil {
		t.Fatalf("fechar run: %v", err)
	}
	fechada, _ := s.Runs().GetByDate(ctx, "2026-10-27")
	if fechada.Status != RunSent || fechada.FinishedAt == nil {
		t.Errorf("run fechada = %+v", fechada)
	}
}

func TestRunsNaoAvancaChunkEmFalha(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	docID, chunks := runComChunks(t, s)
	run, _, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.Runs().AddItem(ctx, run.ID, docID, chunks[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Runs().MarkItemFailed(ctx, item.ID, 3, 512, "ntfy: resposta HTTP 500"); err != nil {
		t.Fatalf("marcar falho: %v", err)
	}
	itens, _ := s.Runs().ListItems(ctx, run.ID)
	if len(itens) != 1 || itens[0].Status != ItemFailed || itens[0].Error == nil || itens[0].Attempts != 3 {
		t.Errorf("item = %+v", itens)
	}
	chunk, _ := s.Plans().Chunk(ctx, chunks[0])
	if chunk.Status != ChunkPending || chunk.SentAt != nil {
		t.Errorf("chunk avançou em falha: %+v", chunk)
	}
	if err := s.Runs().FinishRun(ctx, run.ID, RunFailed, "falha", agora); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	fechada, _ := s.Runs().GetByDate(ctx, "2026-10-27")
	if fechada.Status != RunFailed || fechada.Error == nil || *fechada.Error != "falha" {
		t.Errorf("run = %+v", fechada)
	}
}

func TestRunsItemDuplicadoERollback(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	docID, chunks := runComChunks(t, s)

	primeira, _, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	item, _ := s.Runs().AddItem(ctx, primeira.ID, docID, chunks[0])
	if _, err := s.Runs().AddItem(ctx, primeira.ID, docID, chunks[0]); err == nil {
		t.Error("esperava erro para item duplicado na mesma run")
	}
	if err := s.Runs().MarkItemSent(ctx, item.ID, chunks[0], 10, 1, agora); err != nil {
		t.Fatal(err)
	}

	segunda, _, err := s.Runs().Create(ctx, "2026-10-28", agora, agora, true)
	if err != nil {
		t.Fatal(err)
	}
	outro, _ := s.Runs().AddItem(ctx, segunda.ID, docID, chunks[0])
	err = s.Runs().MarkItemSent(ctx, outro.ID, chunks[0], 10, 1, agora)
	if !errors.Is(err, ErrChunkNotPending) {
		t.Fatalf("erro = %v, quer ErrChunkNotPending", err)
	}
	itens, _ := s.Runs().ListItems(ctx, segunda.ID)
	if len(itens) != 1 || itens[0].Status != ItemPending {
		t.Errorf("item mudou apesar do rollback: %+v", itens)
	}
}

func TestRunsListItemsVazio(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	run, _, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	itens, err := s.Runs().ListItems(ctx, run.ID)
	if err != nil || len(itens) != 0 {
		t.Errorf("itens = %+v, %v", itens, err)
	}
	if err := s.Runs().FinishRun(ctx, run.ID, "queued", "", agora); err == nil {
		t.Error("esperava erro para status final inválido")
	}
}

func TestRunsValidaEntrada(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	if _, _, err := s.Runs().Create(ctx, "2026-13-01", agora, agora, false); err == nil {
		t.Error("esperava erro para data impossível")
	}
	if _, _, err := s.Runs().Create(ctx, "2026-10-27", time.Time{}, agora, false); err == nil {
		t.Error("esperava erro sem scheduled_for")
	}
	if _, _, err := s.Runs().Create(ctx, "2026-10-27", agora, time.Time{}, false); err == nil {
		t.Error("esperava erro sem started_at")
	}
	if emAndamento, err := s.Runs().Running(ctx); err != nil || emAndamento != nil {
		t.Errorf("Running sem run = %+v, %v", emAndamento, err)
	}
}

func TestBlocksBetween(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	lista := blocosDeTeste(6, 60)
	docID := documentoComBlocos(t, s, lista)
	ids, err := s.Blocks().IDs(ctx, docID)
	if err != nil || len(ids) != len(lista) {
		t.Fatalf("ids = %v, %v", ids, err)
	}

	got, err := s.Blocks().Between(ctx, docID, ids[1], ids[4])
	if err != nil {
		t.Fatalf("between: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("blocos = %d, quer 4", len(got))
	}
	for i, b := range got {
		if b.Ord != i+1 {
			t.Errorf("ord[%d] = %d", i, b.Ord)
		}
	}

	if _, err := s.Blocks().Between(ctx, docID, ids[4], ids[1]); err == nil {
		t.Error("esperava erro para intervalo invertido")
	}
	if _, err := s.Blocks().Between(ctx, docID, ids[0], 999999); err == nil {
		t.Error("esperava erro para id inexistente")
	}
	if _, err := s.Blocks().Between(ctx, 999999, ids[0], ids[1]); err == nil {
		t.Error("esperava erro para documento inexistente")
	}
}

func TestPlansChunkVizinhos(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	_, ids := runComChunks(t, s)
	if len(ids) < 3 {
		t.Fatalf("plano com %d chunks, quer pelo menos 3", len(ids))
	}

	chunk, err := s.Plans().Chunk(ctx, ids[1])
	if err != nil || chunk == nil || chunk.ID != ids[1] {
		t.Fatalf("chunk = %+v, %v", chunk, err)
	}
	if inexistente, err := s.Plans().Chunk(ctx, 999999); err != nil || inexistente != nil {
		t.Errorf("chunk inexistente = %+v, %v", inexistente, err)
	}

	anterior, err := s.Plans().PreviousChunk(ctx, chunk)
	if err != nil || anterior == nil || anterior.ID != ids[0] {
		t.Errorf("anterior = %+v, %v", anterior, err)
	}
	proximo, err := s.Plans().NextChunk(ctx, chunk)
	if err != nil || proximo == nil || proximo.ID != ids[2] {
		t.Errorf("próximo = %+v, %v", proximo, err)
	}

	primeiro, _ := s.Plans().Chunk(ctx, ids[0])
	if anterior, err := s.Plans().PreviousChunk(ctx, primeiro); err != nil || anterior != nil {
		t.Errorf("anterior do primeiro = %+v, %v", anterior, err)
	}
	ultimo, _ := s.Plans().Chunk(ctx, ids[len(ids)-1])
	if proximo, err := s.Plans().NextChunk(ctx, ultimo); err != nil || proximo != nil {
		t.Errorf("próximo do último = %+v, %v", proximo, err)
	}
}

func TestRunsItemSkipped(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	docID, chunks := runComChunks(t, s)
	run, _, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.Runs().AddItem(ctx, run.ID, docID, chunks[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Runs().MarkItemSkipped(ctx, item.ID, "documento removido"); err != nil {
		t.Fatalf("pular: %v", err)
	}
	itens, _ := s.Runs().ListItems(ctx, run.ID)
	if len(itens) != 1 || itens[0].Status != ItemSkipped ||
		itens[0].Error == nil || *itens[0].Error != "documento removido" {
		t.Errorf("item = %+v", itens)
	}
	chunk, _ := s.Plans().Chunk(ctx, chunks[0])
	if chunk.Status != ChunkPending {
		t.Errorf("chunk avançou ao pular item: %+v", chunk)
	}
	if err := s.Runs().MarkItemSkipped(ctx, item.ID, "de novo"); err == nil {
		t.Error("esperava erro ao pular item já resolvido")
	}
}

func TestRunsLeituraComDadosInvalidos(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	run, _, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.DB().ExecContext(ctx,
		`UPDATE delivery_runs SET scheduled_for = 'lixo' WHERE id = ?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Runs().GetByDate(ctx, "2026-10-27"); err == nil {
		t.Error("esperava erro de scheduled_for inválido")
	}
	if _, err := s.Runs().Running(ctx); err == nil {
		t.Error("esperava erro de scheduled_for inválido em Running")
	}

	if _, err := s.DB().ExecContext(ctx,
		`UPDATE delivery_runs SET scheduled_for = ?, finished_at = 'lixo' WHERE id = ?`,
		agora.UTC().Format(time.RFC3339), run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Runs().GetByDate(ctx, "2026-10-27"); err == nil {
		t.Error("esperava erro de finished_at inválido")
	}

	docID, chunks := runComChunks(t, s)
	outra, _, err := s.Runs().Create(ctx, "2026-10-28", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.Runs().AddItem(ctx, outra.ID, docID, chunks[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE delivery_items SET sent_at = 'lixo' WHERE id = ?`, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Runs().ListItems(ctx, outra.ID); err == nil {
		t.Error("esperava erro de sent_at inválido")
	}

	if _, err := s.DB().ExecContext(ctx,
		`UPDATE chunks SET section_path = '{' WHERE id = ?`, chunks[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Plans().Chunk(ctx, chunks[0]); err == nil {
		t.Error("esperava erro de section_path inválido")
	}
}

func TestStoreClosePing(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if err := s.Ping(ctx); err == nil {
		t.Error("ping depois de fechar deveria falhar")
	}
	if err := s.Close(); err != nil {
		t.Errorf("segundo close: %v", err)
	}
	var nulo *Store
	if err := nulo.Close(); err != nil {
		t.Errorf("close de store nulo: %v", err)
	}
}

func TestRunsOperacoesComIdInexistente(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	if _, err := s.Runs().AddItem(ctx, 999999, 1, 1); err == nil {
		t.Error("esperava erro ao criar item em run inexistente")
	}
	if err := s.Runs().MarkItemFailed(ctx, 999999, 1, 10, "x"); err == nil {
		t.Error("esperava erro ao falhar item inexistente")
	}
	if err := s.Runs().MarkItemSkipped(ctx, 999999, "x"); err == nil {
		t.Error("esperava erro ao pular item inexistente")
	}
}

func TestOperacoesComBancoFechado(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	if err := s.Close(); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if _, _, err := s.Settings().Get(ctx, "schedule.enabled"); err == nil {
		t.Error("Get deveria falhar com banco fechado")
	}
	if err := s.Settings().Set(ctx, "schedule.enabled", "true", agora); err == nil {
		t.Error("Set deveria falhar com banco fechado")
	}
	if _, err := s.Settings().All(ctx); err == nil {
		t.Error("All deveria falhar com banco fechado")
	}
	if _, err := s.Documents().List(ctx); err == nil {
		t.Error("List deveria falhar com banco fechado")
	}
	if _, err := s.Events().List(ctx, 10); err == nil {
		t.Error("Events.List deveria falhar com banco fechado")
	}
}

func TestRunsItemByDocument(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	docID, chunks := runComChunks(t, s)
	run, _, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}

	item, err := s.Runs().ItemByDocument(ctx, run.ID, docID)
	if err != nil || item != nil {
		t.Fatalf("item inexistente = %+v, %v", item, err)
	}

	criado, err := s.Runs().AddItem(ctx, run.ID, docID, chunks[0])
	if err != nil {
		t.Fatal(err)
	}
	item, err = s.Runs().ItemByDocument(ctx, run.ID, docID)
	if err != nil || item == nil || item.ID != criado.ID || item.ChunkID != chunks[0] {
		t.Fatalf("item = %+v, %v", item, err)
	}
}

func TestRunsDeleteSoRemoveRunAberta(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	docID, chunks := runComChunks(t, s)
	run, _, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Runs().AddItem(ctx, run.ID, docID, chunks[0]); err != nil {
		t.Fatal(err)
	}

	if err := s.Runs().Delete(ctx, run.ID); err != nil {
		t.Fatalf("remover run aberta: %v", err)
	}
	if restante, err := s.Runs().GetByDate(ctx, "2026-10-27"); err != nil || restante != nil {
		t.Fatalf("run deveria sumir: %+v, %v", restante, err)
	}
	if itens, err := s.Runs().ListItems(ctx, run.ID); err != nil || len(itens) != 0 {
		t.Errorf("itens deveriam sair em cascata: %+v, %v", itens, err)
	}

	outra, _, err := s.Runs().Create(ctx, "2026-10-28", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Runs().FinishRun(ctx, outra.ID, RunSent, "", agora); err != nil {
		t.Fatal(err)
	}
	if err := s.Runs().Delete(ctx, outra.ID); err != nil {
		t.Fatalf("remover run fechada: %v", err)
	}
	if fechada, err := s.Runs().GetByDate(ctx, "2026-10-28"); err != nil || fechada == nil {
		t.Errorf("run fechada não pode ser removida: %+v, %v", fechada, err)
	}

	if err := s.Runs().Delete(ctx, 0); err == nil {
		t.Error("esperado erro para id inválido")
	}
}

func TestRunsList(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	docID, chunks := runComChunks(t, s)

	run27, _, err := s.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	run28, _, err := s.Runs().Create(ctx, "2026-10-28", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	run26, _, err := s.Runs().Create(ctx, "2026-10-26", agora, agora, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Runs().AddItem(ctx, run27.ID, docID, chunks[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Runs().AddItem(ctx, run26.ID, docID, chunks[len(chunks)-1]); err != nil {
		t.Fatal(err)
	}

	// Mais recente primeiro, inclusive a run sem itens.
	runs, err := s.Runs().List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("runs = %d, quer 3", len(runs))
	}
	if runs[0].ID != run28.ID || runs[1].ID != run27.ID || runs[2].ID != run26.ID {
		t.Errorf("ordem = %d, %d, %d", runs[0].ID, runs[1].ID, runs[2].ID)
	}
	if !runs[2].Late {
		t.Error("run de catch-up deveria vir com late")
	}

	// O filtro considera só runs com item daquele documento.
	filtradas, err := s.Runs().List(ctx, 10, docID)
	if err != nil {
		t.Fatalf("listar filtrado: %v", err)
	}
	if len(filtradas) != 2 || filtradas[0].ID != run27.ID || filtradas[1].ID != run26.ID {
		t.Errorf("filtro por documento = %+v", filtradas)
	}

	semItens, err := s.Runs().List(ctx, 10, 9999)
	if err != nil || len(semItens) != 0 {
		t.Errorf("filtro inexistente = %v, %v", semItens, err)
	}

	// Limite explícito e limite inválido (padrão 30).
	duas, err := s.Runs().List(ctx, 2, 0)
	if err != nil || len(duas) != 2 {
		t.Errorf("limite 2 = %d runs, %v", len(duas), err)
	}
	todas, err := s.Runs().List(ctx, 0, -1)
	if err != nil || len(todas) != 3 {
		t.Errorf("limite padrão = %d runs, %v", len(todas), err)
	}
}

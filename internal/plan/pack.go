package plan

import (
	"fmt"

	"github.com/giovani/readingdrip/internal/blocks"
)

// unit é um pedaço indivisível de conteúdo (um bloco ou uma subárvore inteira).
type unit struct {
	start     int
	end       int
	seconds   int
	words     int
	oversized bool
}

func blockUnit(bs []blocks.Block, idx int) unit {
	return unit{start: idx, end: idx, seconds: bs[idx].Seconds, words: bs[idx].Words}
}

// collectUnits achata a árvore em unidades na ordem do documento.
//
// Nós que cabem no alvo são mantidos inteiros (preservando seções). Nós
// maiores descem um nível; o heading do nó é preservado como unidade própria
// e depois colado ao primeiro conteúdo por mergeHeadings.
func collectUnits(bs []blocks.Block, items []blocks.Item, target, max int) []unit {
	var out []unit

	for _, it := range items {
		if it.Child == nil {
			out = append(out, blockUnit(bs, it.Block))
			continue
		}

		n := it.Child
		if n.Start < 0 || n.End < 0 {
			continue
		}

		if n.Seconds > target && n.Splittable() {
			if n.Heading >= 0 {
				out = append(out, blockUnit(bs, n.Heading))
			}
			out = append(out, collectUnits(bs, n.Items, target, max)...)
			continue
		}

		out = append(out, unit{
			start:     n.Start,
			end:       n.End,
			seconds:   n.Seconds,
			words:     n.Words,
			oversized: n.Seconds > max,
		})
	}
	return out
}

// mergeHeadings garante que nenhum heading fique isolado no fim de um chunk.
// Headings consecutivos são agrupados e colados ao próximo conteúdo.
func mergeHeadings(bs []blocks.Block, us []unit) []unit {
	out := make([]unit, 0, len(us))
	var pending *unit

	for _, u := range us {
		isHeading := u.start == u.end && bs[u.start].Kind.IsHeading()
		if isHeading {
			if pending == nil {
				copy := u
				pending = &copy
			} else {
				pending.end = u.end
				pending.seconds += u.seconds
				pending.words += u.words
			}
			continue
		}

		if pending != nil {
			u.start = pending.start
			u.seconds += pending.seconds
			u.words += pending.words
			pending = nil
		}
		out = append(out, u)
	}

	if pending != nil {
		if len(out) > 0 {
			last := &out[len(out)-1]
			last.end = pending.end
			last.seconds += pending.seconds
			last.words += pending.words
		} else {
			out = append(out, *pending)
		}
	}
	return out
}

// carryTrailingHeadings remove headings que ficaram no fim de uma unidade
// (por exemplo, uma subseção vazia no fim de um nó) e os transporta para a
// unidade seguinte. Sem isso, um chunk poderia terminar em um heading que não
// é o último bloco do documento.
func carryTrailingHeadings(bs []blocks.Block, us []unit) []unit {
	out := make([]unit, 0, len(us))
	var pending []unit

	for _, u := range us {
		var trail []unit
		for u.end > u.start && bs[u.end].Kind.IsHeading() {
			h := blockUnit(bs, u.end)
			u.seconds -= h.seconds
			u.words -= h.words
			u.end--
			trail = append([]unit{h}, trail...)
		}

		if u.start == u.end && bs[u.start].Kind.IsHeading() {
			pending = append(pending, blockUnit(bs, u.start))
			pending = append(pending, trail...)
			continue
		}

		if len(pending) > 0 {
			u.start = pending[0].start
			for _, h := range pending {
				u.seconds += h.seconds
				u.words += h.words
			}
			pending = nil
		}
		out = append(out, u)
		pending = append(pending, trail...)
	}

	if len(pending) > 0 {
		if len(out) > 0 {
			last := &out[len(out)-1]
			for _, h := range pending {
				last.end = h.end
				last.seconds += h.seconds
				last.words += h.words
			}
		} else {
			combined := pending[0]
			for _, h := range pending[1:] {
				combined.end = h.end
				combined.seconds += h.seconds
				combined.words += h.words
			}
			out = append(out, combined)
		}
	}
	return out
}

func unitsSeconds(us []unit) int {
	total := 0
	for _, u := range us {
		total += u.seconds
	}
	return total
}

func unitsWords(us []unit) int {
	total := 0
	for _, u := range us {
		total += u.words
	}
	return total
}

// packUnits empacota unidades em grupos que se aproximam do alvo.
func packUnits(us []unit, target, min, max int) [][]unit {
	var groups [][]unit
	var cur []unit
	curSeconds := 0

	flush := func() {
		if len(cur) == 0 {
			return
		}
		groups = append(groups, cur)
		cur = nil
		curSeconds = 0
	}

	for _, u := range us {
		if u.oversized {
			flush()
			groups = append(groups, []unit{u})
			continue
		}

		if len(cur) > 0 && curSeconds+u.seconds > max && curSeconds >= min {
			flush()
		}

		cur = append(cur, u)
		curSeconds += u.seconds

		if curSeconds >= target {
			flush()
		}
	}
	flush()
	return groups
}

// balance move unidades inteiras entre grupos vizinhos para reduzir a
// diferença entre eles, sem criar cortes novos.
func balance(groups [][]unit, target, min, max int) [][]unit {
	for i := 1; i < len(groups); i++ {
		a, b := groups[i-1], groups[i]
		sa, sb := unitsSeconds(a), unitsSeconds(b)

		// a curto: puxa unidades de b até chegar perto do mínimo.
		for sa < min && len(b) > 1 {
			u := b[0]
			if sa+u.seconds > max || sb-u.seconds < min {
				break
			}
			groups[i-1] = append(a, u)
			groups[i] = b[1:]
			a, b = groups[i-1], groups[i]
			sa, sb = sa+u.seconds, sb-u.seconds
		}

		// a longo e b curto: empurra unidades de a até estabilizar.
		for sa > target && sb < min && len(a) > 1 {
			u := a[len(a)-1]
			if sb+u.seconds > max {
				break
			}
			groups[i-1] = a[:len(a)-1]
			groups[i] = append([]unit{u}, b...)
			a, b = groups[i-1], groups[i]
			sa, sb = sa-u.seconds, sb+u.seconds
		}
	}
	return groups
}

// materialize transforma grupos de unidades em chunks com metadados.
func materialize(bs []blocks.Block, groups [][]unit, target, max int) ([]Chunk, error) {
	chunks := make([]Chunk, 0, len(groups))

	for i, g := range groups {
		if len(g) == 0 {
			return nil, fmt.Errorf("plano: grupo %d vazio", i)
		}
		first := g[0].start
		last := g[len(g)-1].end
		if first < 0 || last >= len(bs) || first > last {
			return nil, fmt.Errorf("plano: intervalo inválido [%d..%d]", first, last)
		}

		seconds, words := 0, 0
		for _, u := range g {
			seconds += u.seconds
			words += u.words
		}
		oversized := seconds > max

		chunks = append(chunks, Chunk{
			Seq:              i,
			FirstBlock:       first,
			LastBlock:        last,
			FirstHash:        bs[first].Hash,
			LastHash:         bs[last].Hash,
			SectionPath:      blocks.SectionPath(bs, first),
			StartsMidSection: first != 0 && !bs[first].Kind.IsHeading(),
			EndsMidSection:   last+1 < len(bs) && !bs[last+1].Kind.IsHeading(),
			Oversized:        oversized,
			Words:            words,
			Seconds:          seconds,
			TargetSeconds:    target,
		})
	}
	return chunks, nil
}

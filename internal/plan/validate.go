package plan

import (
	"fmt"

	"github.com/giovani/readingdrip/internal/blocks"
)

// Validate confere as invariantes do plano. Um plano inválido nunca deve ser
// persistido ou ativado.
func Validate(bs []blocks.Block, pl *Plan, p Params) error {
	if pl == nil {
		return fmt.Errorf("plano: nulo")
	}
	if len(bs) == 0 {
		return fmt.Errorf("plano: documento sem blocos")
	}
	if len(pl.Chunks) == 0 {
		return fmt.Errorf("plano: sem chunks")
	}

	p = p.normalized()
	target, _, max, err := p.bounds(pl.TotalSeconds)
	if err != nil {
		return err
	}
	if pl.TargetSeconds != target {
		return fmt.Errorf("plano: alvo %d difere do calculado %d", pl.TargetSeconds, target)
	}

	prev := -1
	for i, c := range pl.Chunks {
		if c.Seq != i {
			return fmt.Errorf("plano: chunk %d com seq %d", i, c.Seq)
		}
		if c.FirstBlock != prev+1 {
			return fmt.Errorf("plano: chunk %d começa em %d, esperado %d", i, c.FirstBlock, prev+1)
		}
		if c.LastBlock < c.FirstBlock || c.LastBlock >= len(bs) {
			return fmt.Errorf("plano: chunk %d com intervalo inválido [%d..%d]", i, c.FirstBlock, c.LastBlock)
		}
		if c.FirstHash != bs[c.FirstBlock].Hash {
			return fmt.Errorf("plano: chunk %d com hash inicial divergente", i)
		}
		if c.LastHash != bs[c.LastBlock].Hash {
			return fmt.Errorf("plano: chunk %d com hash final divergente", i)
		}

		seconds, words := 0, 0
		for b := c.FirstBlock; b <= c.LastBlock; b++ {
			seconds += bs[b].Seconds
			words += bs[b].Words
		}
		if seconds != c.Seconds || words != c.Words {
			return fmt.Errorf("plano: chunk %d com métricas divergentes (seconds %d/%d, words %d/%d)",
				i, c.Seconds, seconds, c.Words, words)
		}

		if bs[c.LastBlock].Kind.IsHeading() && c.LastBlock != len(bs)-1 {
			return fmt.Errorf("plano: chunk %d termina em heading no bloco %d", i, c.LastBlock)
		}

		wantStart := c.FirstBlock != 0 && !bs[c.FirstBlock].Kind.IsHeading()
		if c.StartsMidSection != wantStart {
			return fmt.Errorf("plano: chunk %d com starts_mid_section inconsistente", i)
		}
		wantEnd := c.LastBlock+1 < len(bs) && !bs[c.LastBlock+1].Kind.IsHeading()
		if c.EndsMidSection != wantEnd {
			return fmt.Errorf("plano: chunk %d com ends_mid_section inconsistente", i)
		}

		if c.Seconds > max && !c.Oversized {
			return fmt.Errorf("plano: chunk %d excede o máximo (%ds > %ds) sem flag oversized", i, c.Seconds, max)
		}

		prev = c.LastBlock
	}

	if prev != len(bs)-1 {
		return fmt.Errorf("plano: cobertura incompleta, termina em %d de %d", prev, len(bs)-1)
	}
	return nil
}

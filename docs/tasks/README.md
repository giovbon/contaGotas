---
title: Briefs de tarefa
status: ativo
updated: 2026-10-30
---

# Briefs de tarefa

Um brief é a leitura curta de uma tarefa: objetivo, contrato já fechado, invariantes, arquivos prováveis, testes e docs a atualizar. Ele existe para o agente **não abrir vários docs inteiros** antes de codar.

## Regras

- Um arquivo por requisito ou incremento: `docs/tasks/rf-023.md`, `docs/tasks/1.18.md`…
- No máximo ~60 linhas. Se passar, o detalhe pertence ao doc de área.
- Só entra em “Fechado” o que já está documentado. Dúvida fica em “Em aberto” e precisa ser resolvida no doc de área antes do código.
- O brief **não substitui** os docs de área. Em conflito, os docs vencem.
- Ao concluir a tarefa, o brief pode ser removido; o resultado fica no [`changelog.md`](../changelog.md).
- Comece pelo [`TEMPLATE.md`](./TEMPLATE.md).

## Como usar no dia a dia

1. Leia `docs/context.md`.
2. Leia o brief da tarefa.
3. Leia só a seção citada em “Fonte de verdade” e o requisito `RF-###`.
4. Codifique, teste e atualize docs + changelog.

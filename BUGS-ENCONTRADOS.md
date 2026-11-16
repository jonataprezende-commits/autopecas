# AutoPeças — bugs encontrados na revisão do código (TODOS CORRIGIDOS na 1.0.2)

> **Status: todos os bugs abaixo foram corrigidos e estão verificados na versão 1.0.2.**
> Cada um mantém o teste que falhava (`autopecas/bugs_test.go`, `qa/teste_parsers.mjs`),
> agora passando. Detalhes da correção, arquivos alterados e resultados finais em
> `ENTREGA/RELATORIO-DE-TESTES.md` (seção "Correções da 1.0.2").
> Bônus descoberto durante a correção: **B8** (tag `vUnCom` do XML de NF-e ignorada).
> Versão original deste documento (o diagnóstico) segue abaixo, sem alterações.

Data: 29/10/2026 · Revisão feita sobre o código entregue na 1.0.1 (nada foi alterado ainda).
Método: leitura de todos os handlers Go e do front, e **um teste que falha para cada suspeita**
(`autopecas/bugs_test.go` e `qa/teste_parsers.mjs`). Só entrou nesta lista o que ficou provado.

```
$ go test ./... -run TestBug -v
--- FAIL: TestBugFichaRecebidaDuasVezesDuplicaEstoque
--- FAIL: TestBugReceberFichaJaRecebidaPeloPedido
--- FAIL: TestBugPDFComAcentoCorrompido
--- FAIL: TestBugVendaJSONComNullInt64
--- FAIL: TestBugDiasDeAtrasoErradoPorFuso
--- FAIL: TestBugContaComCampoNuloSomeDaLista

$ node qa/teste_parsers.mjs   → 4 caso(s) falharam   (exit 1)
```

---

## Quadro geral

| # | Gravidade | Onde | Sintoma | Prova |
|---|-----------|------|---------|-------|
| **B1** | **Alta** (estoque/dinheiro) | `api_faltas.go:150` (`handleFaltaStatus`) + `api_xml.go:843` (`receiveFalta`) | Receber a **mesma ficha de falta duas vezes** entra estoque duas vezes. Duplo clique no "Receber" ou outra aba com lista desatualizada basta. `receiveFalta` não confere o status atual da ficha — é exatamente o bug do item 4 da 1.0.1, que ficou sem correção no caminho da ficha. | `TestBugFichaRecebidaDuasVezesDuplicaEstoque` → estoque 4, esperado 2; 2 movimentos, esperado 1 |
| **B2** | **Alta** | idem | Variante: pedido recebido (**5 un.**) e depois a ficha recebida pela tela de fichas → **10 un.** no estoque. | `TestBugReceberFichaJaRecebidaPeloPedido` → 10, esperado 5 |
| **B3** | **Alta** (dinheiro digitado) | `web/app.js:22` (`parseDinheiro`) e `:33` (`parseQtd`) | Separador de milhar é lido como decimal: **"1.234" → R$ 1,23**, **"10.000" → R$ 10,00**, **"1.000.000" → R$ 1,00**. Vale para preço, custo, desconto de item, desconto da venda, valor recebido, estoque inicial e movimento de estoque ("1.000" → 1 unidade). Nenhum aviso. | `qa/teste_parsers.mjs` → 4 falhas |
| **B4** | Média | `api_sales.go:447` (`handleListReceivables`) | Conta a receber com **`sale_id` ou `paid_at` nulo** é descartada em silêncio (`Scan` falha → `continue`): some da tela **e do total em aberto**. Pode acontecer em banco restaurado/editado. | `TestBugContaComCampoNuloSomeDaLista` → lista 0 de 2, total R$ 0,00 em vez de R$ 300,00 |
| **B5** | Média | `pdf.go:29` (`escPDF`) | Todo **relatório em PDF corrompe acentos**: a fonte é declarada `/WinAnsiEncoding` (1 byte), mas o texto vai em UTF-8. "Filtro de óleo" sai **"Filtro de Ã³leo"**. Afeta descrição de peça, nome de cliente, nome da loja. | `TestBugPDFComAcentoCorrompido` → acha `0xC3 0xB3`; não acha `0xF3` |
| **B6** | Média | `api_sales.go:476` e `api_faltas.go:70` (+ `api_reports.go:226`) | Atraso contado entre data **UTC** (`time.Parse`) e **agora local**: no fim do dia a conta que venceu **ontem** aparece com **2 dias** de atraso. Mesmo erro em "dias em aberto" das fichas e no relatório de recebíveis. | `TestBugDiasDeAtrasoErradoPorFuso` → 2, esperado 1 |
| **B7** | Baixa (contrato) | `api_sales.go:277` e `:283` (`getSale`) | O JSON da venda devolve `cliente_id` e `itens[].peca_id` como **`{"Int64":0,"Valid":false}`** em vez de número/`null`. O front atual não usa esses campos, então não há sintoma visível hoje — mas qualquer integração quebra. | `TestBugVendaJSONComNullInt64` (3 asserções) |

Todas as correções são pequenas: B1/B2 ≈ 15 linhas (guarda de status + transação), B3 ≈ 6 linhas (regra de milhar), B4 ≈ 3 linhas (`IFNULL`), B5 ≈ 20 linhas (converter para Latin-1 e ajustar a largura de coluna), B6 ≈ 4 linhas (`time.ParseInLocation` / diferença por data), B7 ≈ 6 linhas (ponteiro em vez de `sql.NullInt64`).

---

## Detalhe de cada bug

### B1 / B2 — ficha de falta entrando no estoque duas vezes
`handleFaltaStatus` (status `recebida`) chama `receiveFalta` sem olhar o status atual, e
`receiveFalta` também não confere nada:

```go
// api_xml.go:843
func (a *App) receiveFalta(faltaID int64, qty float64, partID int64) error {
    return a.db.Tx(func(tx *sql.Tx) error {
        var pid sql.NullInt64
        tx.QueryRow(`SELECT part_id FROM faltas WHERE id=?`, faltaID).Scan(&pid)
        ...
        UPDATE parts SET stock = stock + qty        // ← entra sempre
        UPDATE faltas SET status='recebida'          // ← só muda o rótulo
```

Já a lista (`handleListFaltas`) filtra por status, então o botão desaparece depois — mas a
requisição repetida (duplo clique, aba antiga, integração) passa. **Correção:** dentro da mesma
transação, ler o status, recusar (`409`) se já for `recebida`, e usar
`UPDATE ... WHERE status IN ('aberta','pedida')` com conferência de linhas afetadas (padrão do
`order_items.received_at` do item 4).

### B3 — separador de milhar no campo de dinheiro
```js
if (s.includes(",")) s = s.replace(/\./g, "").replace(",", ".");   // só trata "." quando tem vírgula
```
Sem vírgula, `"1.234"` vira 1,234 → `Math.round(1.234*100)` = **123 centavos**.
**Correção:** antes da regra atual, tratar `^\d{1,3}(\.\d{3})+$` como milhar (e o mesmo em
`parseQtd`). O servidor (`parseMoneyBR`, usado na importação XML) tem a mesma ambiguidade e merece a
mesma regra.

### B4 — conta que desaparece da lista
```go
q := `SELECT r.id,r.sale_id,...,r.paid_at FROM receivables r`   // sem IFNULL
var id, saleID, amount int64
var ..., paid string
if rows.Scan(...) != nil { continue }                             // ← NULL descarta a linha
```
**Correção:** `IFNULL(r.sale_id,0)`, `IFNULL(r.paid_at,'')` e trocar o `continue` silencioso por
log (ou acumular um aviso na resposta).

### B5 — acentos nos PDFs de relatório
A fonte é `/Helvetica /Encoding /WinAnsiEncoding` (1 byte por caractere), mas `escPDF` só escapa
`\ ( )` e deixa os bytes UTF-8 passarem. **Correção:** converter para Latin-1 (`rune > 255 → '?'`,
`rune ≤ 255 → byte`) na hora de escrever a string, e medir a largura com `len(runes)` para o
alinhamento à direita continuar certo.

### B6 — atraso em dias
```go
t, _ := time.Parse("2006-01-02", due)          // 00:00 UTC
diasAtraso = int(time.Since(t).Hours() / 24)    // agora em horário local
```
Em UTC−3, às 21h local a data UTC já virou o dia seguinte → um dia a mais. **Correção:** comparar
datas (`time.ParseInLocation` + `hoje.Sub(due)` truncado por data), não durações em horas.

### B7 — `sql.NullInt64` direto no JSON
`getSale` devolve `custID`/`pid` (`sql.NullInt64`) no `map[string]any`; o `encoding/json` serializa a
struct. **Correção:** converter para `*int64`/`nil` — sem renomear campo nenhum.

---

## O que foi verificado e **não** tem problema

Para não parecer que só olhei onde doeu:

* **Injeção de SQL** — todos os filtros usam `?`; as montagens com `Sprintf`/concatenação só usam
  nomes de tabela/coluna internos (`restaurarDe`, `colunasTabela`, `colunaSeFaltar`).
* **`a.db.*` dentro de `a.db.Tx`** — re-varri as 11 closures depois das mudanças: continua limpo
  (nenhum deadlock novo).
* **Restaurar backup da 1.0.0 (sem `user_version` e sem `order_items.received_at`)** — restaura,
  preserva dados e o sistema continua vendendo: `TestRestaurarBackupDa100` **passa**.
* **Cancelamento repetido de venda** — não devolve estoque duas vezes.
* **Concorrência de venda/estoque** — serializada pela conexão única; sem corrida.
* **Totais de venda, desconto por item, custo, troco** — cobertos pelos testes da 1.0.1, verdes.
* **CSV dos relatórios** — suspeitei que o `Content-Disposition` fosse perdido, mas o código define
  os cabeçalhos **antes** do `Write`: download OK. (Virou teste de regressão, passando.)
* **Front: `fmtMoney`, `fmtQtd`** — corretos (inclusive negativos e milhar).
* **Middleware do item 7** — não bloqueia o fluxo normal da UI (testado com navegador real).

## Observações que não são bug (decisão de produto)

1. **Importação de XML com "atualizar estoque" sem "sobrescrever"** soma a quantidade a cada
   importação: importar o mesmo arquivo duas vezes dobra o estoque. É opção explícita (`entrada de
   compra`), mas confunde — sugiro avisar na tela "este arquivo já foi importado em <data>".
2. **Venda com data retroativa** calcula o vencimento a partir de *hoje* e não da data da venda.
   Precisa de decisão: para faturamento retroativo, o correto costuma ser a data da venda.
3. **Cadastro de peça aceita preço/custo negativo** (a venda recusa depois). Melhor recusar no
   cadastro.
4. **`equiv` sem índice único** — códigos equivalentes repetidos entre peças diferentes são
   possíveis; o casamento por `LIMIT 1` pode escolher a peça errada.
5. **Backup e relatórios pesados travam o outro terminal** (conexão única). Já está no roadmap
   (`SUGESTOES-ERP.md`, item 11).
6. **`handleSaveVehicle` com cliente inexistente** devolve 500 (violação de FK) em vez de 404/400.

---

## Como seguir

Se você autorizar, corrijo na ordem **B1/B2 → B3 → B4 → B5 → B6 → B7**, mantendo a disciplina da
1.0.1: cada `TestBug...` que hoje falha tem de passar, e a suíte inteira (26 testes + o de front)
tem de ficar verde antes de gerar o pacote **1.0.2** (`go vet` + `go test -race` + os 5 critérios no
`.exe` sob Wine). Estimativa: meio dia de trabalho; B1/B2 e B3 são os que mais doem no dia a dia da
loja.

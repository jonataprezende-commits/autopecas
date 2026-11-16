package main

// Testes dos itens 1, 2, 5 e 6 (vendas: desconto, travamento, validacao, cancelamento).

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// vendaSimples monta o corpo de uma venda com um item.
func corpoVenda(pecaID int64, qtd float64, preco, descItem, descLoja int64, pagamento string, extra map[string]any) map[string]any {
	c := map[string]any{
		"cliente_nome": "Cliente Teste",
		"itens": []any{map[string]any{
			"peca_id": pecaID, "quantidade": qtd, "preco_centavos": preco, "desconto_centavos": descItem,
		}},
		"desconto_centavos": descLoja,
		"forma_pagamento":   pagamento,
	}
	for k, v := range extra {
		c[k] = v
	}
	return c
}

func num(m map[string]any, k string) int64 {
	if v, ok := m[k].(float64); ok {
		return int64(v)
	}
	return 0
}

// ---------- item 1: desconto por item precisa entrar no total ----------

func TestItem1DescontoDeItemEntraNoTotal(t *testing.T) {
	env := novaAppTeste(t)
	peca := env.criarPeca("FIL-001", "Filtro de oleo", 2000, 5000, 20)

	// 2 x R$ 50,00 = R$ 100,00; desconto no item R$ 10,00; desconto da venda R$ 5,00
	r := env.post("/api/vendas", corpoVenda(peca, 2, 5000, 1000, 500, "faturado", map[string]any{"vencimento_dias": 7}))
	if r.status != 200 {
		t.Fatalf("criar venda faturada: status %d, corpo %s", r.status, r.corpo)
	}
	v := r.json(t)
	vendaID := num(v, "id")

	if got := num(v, "total_centavos"); got != 8500 {
		t.Errorf("total da venda = %d, esperado 8500 (10000 - 1000 de item - 500 da venda)", got)
	}
	if got := num(v, "subtotal_centavos"); got != 10000 {
		t.Errorf("subtotal = %d, esperado 10000 (bruto, sem descontos)", got)
	}
	if got := num(v, "desconto_centavos"); got != 1500 {
		t.Errorf("desconto gravado = %d, esperado 1500 (itens 1000 + venda 500)", got)
	}
	// subtotal - desconto == total sempre
	if num(v, "subtotal_centavos")-num(v, "desconto_centavos") != num(v, "total_centavos") {
		t.Errorf("subtotal - desconto != total na venda gravada")
	}
	// total do item no banco tambem considera o desconto do item
	if got := env.qint(`SELECT total_cents FROM sale_items WHERE sale_id=?`, vendaID); got != 9000 {
		t.Errorf("total do item = %d, esperado 9000", got)
	}
	// conta a receber gerada com o total correto
	if got := env.qint(`SELECT amount_cents FROM receivables WHERE sale_id=?`, vendaID); got != 8500 {
		t.Errorf("valor da conta a receber = %d, esperado 8500", got)
	}

	// fechamento do dia soma o total correto
	f := env.get("/api/fechamento")
	if f.status != 200 {
		t.Fatalf("fechamento: status %d", f.status)
	}
	if got := num(f.json(t), "total_centavos"); got != 8500 {
		t.Errorf("total do fechamento do dia = %d, esperado 8500", got)
	}
}

func TestItem1CustoArredondadoSemTruncar(t *testing.T) {
	env := novaAppTeste(t)
	// custo 333 centavos x 3 = 999 (sem erro de truncamento em float)
	peca := env.criarPeca("CUS-001", "Peca de custo quebrado", 333, 1000, 20)
	r := env.post("/api/vendas", corpoVenda(peca, 3, 1000, 0, 7, "dinheiro", map[string]any{"recebido_centavos": 5000}))
	if r.status != 200 {
		t.Fatalf("criar venda: %d %s", r.status, r.corpo)
	}
	v := r.json(t)
	if got := num(v, "custo_centavos"); got != 999 {
		t.Errorf("custo = %d, esperado 999", got)
	}
	if got := num(v, "total_centavos"); got != 2993 {
		t.Errorf("total = %d, esperado 2993 (3000 - 7)", got)
	}
	if got := num(v, "troco_centavos"); got != 2007 {
		t.Errorf("troco = %d, esperado 2007", got)
	}
}

// ---------- item 2: leitura de configuracao dentro da transacao trava ----------

func TestItem2VendaFaturadaSemTravamento(t *testing.T) {
	env := novaAppSemServidor(t)
	peca := env.criarPeca("TRA-001", "Peca para teste de travamento", 1000, 2000, 10)

	// vencimento_dias = 0 significa "usar o prazo configurado"
	r := env.postComTimeout("/api/vendas", corpoVenda(peca, 1, 2000, 0, 0, "faturado", map[string]any{"vencimento_dias": 0}), 5*time.Second)
	if r.status != 200 {
		t.Fatalf("venda faturada com vencimento_dias=0: status %d corpo %s", r.status, r.corpo)
	}
	esperado := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	if got := env.qstr(`SELECT due_date FROM receivables ORDER BY id DESC LIMIT 1`); got != esperado {
		t.Errorf("vencimento = %s, esperado %s (prazo padrao de 30 dias)", got, esperado)
	}

	// prazo configurado diferente de 30 deve ser respeitado
	cfg := env.post("/api/config", map[string]any{"prazo_faturado_dias": "15"})
	if cfg.status != 200 {
		t.Fatalf("salvar config: %d %s", cfg.status, cfg.corpo)
	}
	r = env.postComTimeout("/api/vendas", corpoVenda(peca, 1, 2000, 0, 0, "faturado", map[string]any{"vencimento_dias": 0}), 5*time.Second)
	if r.status != 200 {
		t.Fatalf("segunda venda faturada: status %d corpo %s", r.status, r.corpo)
	}
	esperado = time.Now().AddDate(0, 0, 15).Format("2006-01-02")
	if got := env.qstr(`SELECT due_date FROM receivables ORDER BY id DESC LIMIT 1`); got != esperado {
		t.Errorf("vencimento = %s, esperado %s (prazo configurado de 15 dias)", got, esperado)
	}
}

// ---------- item 5: validacao dos dados da venda ----------

func TestItem5ValidacaoDaVenda(t *testing.T) {
	casos := []struct {
		nome     string
		montar   func(peca int64) map[string]any
		esperado string // trecho da mensagem de erro
	}{
		{
			nome: "recebido menor que o total",
			montar: func(peca int64) map[string]any {
				return corpoVenda(peca, 1, 5000, 0, 0, "dinheiro", map[string]any{"recebido_centavos": 4000})
			},
			esperado: "recebido é menor que o total",
		},
		{
			nome: "preco negativo",
			montar: func(peca int64) map[string]any {
				return corpoVenda(peca, 1, -100, 0, 0, "dinheiro", nil)
			},
			esperado: "preço",
		},
		{
			nome: "desconto do item negativo",
			montar: func(peca int64) map[string]any {
				return corpoVenda(peca, 1, 5000, -100, 0, "dinheiro", nil)
			},
			esperado: "desconto",
		},
		{
			nome: "desconto da venda negativo",
			montar: func(peca int64) map[string]any {
				return corpoVenda(peca, 1, 5000, 0, -100, "dinheiro", nil)
			},
			esperado: "desconto",
		},
		{
			nome: "valor recebido negativo",
			montar: func(peca int64) map[string]any {
				return corpoVenda(peca, 1, 5000, 0, 0, "dinheiro", map[string]any{"recebido_centavos": -500})
			},
			esperado: "recebido",
		},
		{
			nome: "desconto maior que o valor do item",
			montar: func(peca int64) map[string]any {
				return corpoVenda(peca, 2, 5000, 20000, 0, "dinheiro", nil)
			},
			esperado: "desconto",
		},
		{
			nome: "quantidade zero",
			montar: func(peca int64) map[string]any {
				return corpoVenda(peca, 0, 5000, 0, 0, "dinheiro", nil)
			},
			esperado: "quantidade",
		},
		{
			nome: "quantidade negativa",
			montar: func(peca int64) map[string]any {
				return corpoVenda(peca, -2, 5000, 0, 0, "dinheiro", nil)
			},
			esperado: "quantidade",
		},
		{
			nome: "quantidade acima do limite",
			montar: func(peca int64) map[string]any {
				return corpoVenda(peca, 1000001, 1, 0, 0, "dinheiro", nil)
			},
			esperado: "quantidade",
		},
		{
			nome: "data em formato invalido",
			montar: func(peca int64) map[string]any {
				return corpoVenda(peca, 1, 5000, 0, 0, "dinheiro", map[string]any{"data": "31/12/2025"})
			},
			esperado: "data",
		},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			env := novaAppTeste(t)
			peca := env.criarPeca("VAL-001", "Peca de validacao", 1000, 5000, 100)
			r := env.post("/api/vendas", c.montar(peca))
			if r.status != 400 {
				t.Fatalf("esperado 400, veio %d (%s)", r.status, r.corpo)
			}
			if msg := r.erro(t); !contemTexto(msg, c.esperado) {
				t.Errorf("mensagem %q nao contem %q", msg, c.esperado)
			}
			if n := env.qint(`SELECT COUNT(*) FROM sales`); n != 0 {
				t.Errorf("venda invalida foi gravada (%d registros)", n)
			}
		})
	}
}

func TestItem5DataAceitaENormalizada(t *testing.T) {
	env := novaAppTeste(t)
	peca := env.criarPeca("DAT-001", "Peca com data", 1000, 2000, 10)

	r := env.post("/api/vendas", corpoVenda(peca, 1, 2000, 0, 0, "dinheiro", map[string]any{"data": "2025-01-02"}))
	if r.status != 200 {
		t.Fatalf("data YYYY-MM-DD deveria ser aceita: %d %s", r.status, r.corpo)
	}
	if got := env.qstr(`SELECT created_at FROM sales ORDER BY id DESC LIMIT 1`); got != "2025-01-02 00:00:00" {
		t.Errorf("created_at = %q, esperado \"2025-01-02 00:00:00\"", got)
	}

	r = env.post("/api/vendas", corpoVenda(peca, 1, 2000, 0, 0, "dinheiro", map[string]any{"data": "2025-01-03 14:30:00"}))
	if r.status != 200 {
		t.Fatalf("data YYYY-MM-DD HH:MM:SS deveria ser aceita: %d %s", r.status, r.corpo)
	}
	if got := env.qstr(`SELECT created_at FROM sales ORDER BY id DESC LIMIT 1`); got != "2025-01-03 14:30:00" {
		t.Errorf("created_at = %q, esperado \"2025-01-03 14:30:00\"", got)
	}
}

func TestItem5RecebidoZeroPagaOTotal(t *testing.T) {
	env := novaAppTeste(t)
	peca := env.criarPeca("REC-001", "Peca pagamento", 1000, 2000, 10)

	r := env.post("/api/vendas", corpoVenda(peca, 2, 2000, 0, 0, "dinheiro", map[string]any{"recebido_centavos": 0}))
	if r.status != 200 {
		t.Fatalf("recebido=0 deve significar pagamento integral: %d %s", r.status, r.corpo)
	}
	v := r.json(t)
	if got := num(v, "recebido_centavos"); got != 4000 {
		t.Errorf("recebido gravado = %d, esperado 4000", got)
	}
	if got := num(v, "troco_centavos"); got != 0 {
		t.Errorf("troco = %d, esperado 0", got)
	}
}

// ---------- item 6: cancelar venda faturada ja paga gera estorno ----------

func TestItem6CancelarVendaPagaGeraEstorno(t *testing.T) {
	env := novaAppTeste(t)
	peca := env.criarPeca("EST-001", "Peca faturada", 1000, 3000, 10)

	r := env.post("/api/vendas", corpoVenda(peca, 2, 3000, 0, 0, "faturado", map[string]any{"vencimento_dias": 7}))
	if r.status != 200 {
		t.Fatalf("criar venda faturada: %d %s", r.status, r.corpo)
	}
	vendaID := num(r.json(t), "id")

	rec := env.qint(`SELECT id FROM receivables WHERE sale_id=?`, vendaID)
	if rec == 0 {
		t.Fatalf("venda faturada sem conta a receber")
	}
	if p := env.post("/api/recebiveis/"+itoa(rec)+"/baixar", map[string]any{}); p.status != 200 {
		t.Fatalf("baixar conta: %d %s", p.status, p.corpo)
	}
	if st := env.qstr(`SELECT status FROM receivables WHERE id=?`, rec); st != "pago" {
		t.Fatalf("status apos baixa = %s, esperado pago", st)
	}

	if c := env.post("/api/vendas/"+itoa(vendaID)+"/cancelar", map[string]any{}); c.status != 200 {
		t.Fatalf("cancelar venda: %d %s", c.status, c.corpo)
	}
	if st := env.qstr(`SELECT status FROM receivables WHERE id=?`, rec); st != "estornado" {
		t.Errorf("conta de venda paga cancelada ficou %q, esperado \"estornado\"", st)
	}

	// nao pode aparecer como recebido nem como em aberto em nenhuma soma
	d := env.get("/api/dashboard").json(t)
	if n := num(d, "recebiveis_aberto_centavos"); n != 0 {
		t.Errorf("dashboard: em aberto = %d, esperado 0", n)
	}
	if n := num(d, "recebiveis_vencidos_centavos"); n != 0 {
		t.Errorf("dashboard: vencidos = %d, esperado 0", n)
	}
	lista := env.get("/api/recebiveis?status=todos")
	for _, it := range env.jsonLista(lista, "itens") {
		if int64(it["id"].(float64)) == rec && it["status"] != "estornado" {
			t.Errorf("lista de recebiveis mostra status %v, esperado estornado", it["status"])
		}
	}
	if n := num(env.get("/api/recebiveis").json(t), "total_aberto_centavos"); n != 0 {
		t.Errorf("total em aberto = %d, esperado 0", n)
	}
	// relatorio/CSV de recebiveis: estornado nao pode aparecer como Pago
	rel := env.get("/api/relatorios/recebiveis")
	if rel.status != 200 {
		t.Fatalf("relatorio de recebiveis: %d %s", rel.status, rel.corpo)
	}
	corpo := string(rel.corpo)
	if !strings.Contains(corpo, "Estornada") {
		t.Errorf("relatorio de recebiveis deveria mostrar a situacao \"Estornada\": %s", corpo)
	}
	csv := env.get("/api/relatorios/recebiveis/csv")
	if strings.Contains(strings.ToLower(string(csv.corpo)), ",pago") {
		t.Errorf("CSV de recebiveis marcou a conta estornada como Pago: %s", csv.corpo)
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

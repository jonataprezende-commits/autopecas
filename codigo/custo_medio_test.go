package main

// Testes do custo medio ponderado (1.0.12).
//
// O custo de compra (cost_cents) continua sendo o ultimo preco pago; a media
// ponderada (avg_cost_cents) e o que vale para valorizar o estoque e a margem.
// Estes testes fixam a formula e provam que os pontos de entrada (estoque
// manual e recebimento de compra) atualizam a media do jeito certo.

import (
	"testing"
)

func TestCustoMedioPonderado(t *testing.T) {
	casos := []struct {
		nome       string
		avg        int64
		qtdAtual   float64
		custoNovo  int64
		qtdEntrada float64
		quero      int64
	}{
		{"estoque zerado assume o preco da compra", 0, 0, 1000, 10, 1000},
		{"dois lotes iguais fazem a media simples", 1000, 10, 2000, 10, 1500},
		{"pondera pelo volume comprado", 1000, 30, 2000, 10, 1250},
		{"entrada sem custo nao mexe na media", 1500, 5, 0, 10, 1500},
		{"entrada sem quantidade nao mexe na media", 1500, 5, 2000, 0, 1500},
		{"estoque negativo reinicia pelo preco novo", 1000, -2, 3000, 2, 3000},
		{"lote fracionado arredonda para o centavo", 1000, 1, 1001, 1, 1001},
	}
	for _, c := range casos {
		got := calcularCustoMedio(c.avg, c.qtdAtual, c.custoNovo, c.qtdEntrada)
		if got != c.quero {
			t.Errorf("%s: calcularCustoMedio(%d, %v, %d, %v) = %d, queria %d",
				c.nome, c.avg, c.qtdAtual, c.custoNovo, c.qtdEntrada, got, c.quero)
		}
	}
}

func TestCustoParaValoracao(t *testing.T) {
	if v := custoParaValoracao(2000, 3000); v != 2000 {
		t.Errorf("media conhecida deve valer mais que o ultimo custo: veio %d", v)
	}
	if v := custoParaValoracao(0, 3000); v != 3000 {
		t.Errorf("sem media deve cair no ultimo custo: veio %d", v)
	}
	if v := custoParaValoracao(0, 0); v != 0 {
		t.Errorf("sem custo nenhum deve ser zero: veio %d", v)
	}
}

// TestEntradaDeCompraAtualizaCustoMedio usa a rota real de estoque.
func TestEntradaDeCompraAtualizaCustoMedio(t *testing.T) {
	env := novaAppTeste(t)
	id := env.criarPeca("CM-1", "Filtro de oleo", 1000, 1500, 10)

	// cadastro inicial: a media nasce igual ao custo informado
	if avg := env.qint(`SELECT avg_cost_cents FROM parts WHERE id=?`, id); avg != 1000 {
		t.Fatalf("media inicial deveria ser 1000, veio %d", avg)
	}

	// entrada de 10 unidades a R$ 20,00 -> media (1000*10 + 2000*10)/20 = 1500
	r := env.post("/api/estoque/movimento", map[string]any{
		"peca_id": id, "tipo": "entrada", "quantidade": 10,
		"custo_centavos": 2000, "motivo": "compra",
	})
	if r.status != 200 {
		t.Fatalf("entrada deveria funcionar: %d %s", r.status, r.corpo)
	}
	if avg := env.qint(`SELECT avg_cost_cents FROM parts WHERE id=?`, id); avg != 1500 {
		t.Errorf("media depois da compra deveria ser 1500, veio %d", avg)
	}
	if cost := env.qint(`SELECT cost_cents FROM parts WHERE id=?`, id); cost != 2000 {
		t.Errorf("ultimo custo deveria ser 2000, veio %d", cost)
	}
	if est := env.estoque(id); est != 20 {
		t.Errorf("estoque deveria ser 20, veio %v", est)
	}

	// saida nao pode mexer na media
	if r := env.post("/api/estoque/movimento", map[string]any{
		"peca_id": id, "tipo": "saida", "quantidade": 5, "motivo": "venda",
	}); r.status != 200 {
		t.Fatalf("saida deveria funcionar: %d %s", r.status, r.corpo)
	}
	if avg := env.qint(`SELECT avg_cost_cents FROM parts WHERE id=?`, id); avg != 1500 {
		t.Errorf("saida nao pode mudar a media, veio %d", avg)
	}
}

// TestRelatorioEstoqueUsaCustoMedio prova que a valorizacao do estoque troca o
// ultimo custo pela media ponderada.
func TestRelatorioEstoqueUsaCustoMedio(t *testing.T) {
	env := novaAppTeste(t)
	id := env.criarPeca("CM-2", "Pastilha de freio", 1000, 1500, 10)
	if r := env.post("/api/estoque/movimento", map[string]any{
		"peca_id": id, "tipo": "entrada", "quantidade": 10,
		"custo_centavos": 3000, "motivo": "compra",
	}); r.status != 200 {
		t.Fatalf("entrada: %d %s", r.status, r.corpo)
	}
	// media = (1000*10 + 3000*10)/20 = 2000; estoque 20 -> R$ 400,00.
	// Se usasse o ultimo custo (3000) daria R$ 600,00.
	r := env.get("/api/relatorios/estoque_valor")
	if r.status != 200 {
		t.Fatalf("relatorio deveria responder 200, veio %d %s", r.status, r.corpo)
	}
	corpo := string(r.corpo)
	if !contemTexto(corpo, "Estoque a preco de custo") {
		t.Fatalf("relatorio sem o resumo de custo: %s", corpo)
	}
	if !contemTexto(corpo, "400,00") {
		t.Errorf("valorizacao deveria usar a media (R$ 400,00): %s", corpo)
	}
	if contemTexto(corpo, "600,00") {
		t.Errorf("valorizacao nao pode usar o ultimo custo (R$ 600,00): %s", corpo)
	}
}

// TestMigracaoCustoMedioExiste garante que a coluna existe no esquema atual.
func TestMigracaoCustoMedioExiste(t *testing.T) {
	env := novaAppTeste(t)
	var tipo string
	err := env.app.db.QueryRow(`SELECT type FROM pragma_table_info('parts') WHERE name='avg_cost_cents'`).Scan(&tipo)
	if err != nil {
		t.Fatalf("coluna avg_cost_cents ausente: %v", err)
	}
	if v, err := env.app.userVersion(); err != nil || v != 8 {
		t.Fatalf("user_version deveria ser 8, veio %d (err=%v)", v, err)
	}
}

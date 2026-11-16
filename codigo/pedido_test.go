package main

// Testes do item 4: receber pedido de compra nao pode duplicar estoque.

import (
	"testing"
)

// prepararPedido cria uma peca sem estoque, uma ficha de falta e gera o pedido.
// Devolve (pecaID, faltaID, pedidoID).
func prepararPedido(t *testing.T, env *ambiente, codigo string) (int64, int64, int64) {
	t.Helper()
	peca := env.criarPeca(codigo, "Peca do pedido "+codigo, 1200, 2400, 0)
	forn := env.criarFornecedor("Fornecedor " + codigo)

	r := env.post("/api/fichas", map[string]any{
		"peca_id": peca, "descricao": "Peca do pedido", "quantidade": 5,
		"cliente_nome": "Cliente da ficha", "fornecedor_id": forn, "custo_centavos": 1200,
	})
	if r.status != 200 {
		t.Fatalf("criar ficha de falta: %d %s", r.status, r.corpo)
	}
	falta := num(r.json(t), "id")

	g := env.post("/api/pedidos/gerar", map[string]any{})
	if g.status != 200 {
		t.Fatalf("gerar pedido: %d %s", g.status, g.corpo)
	}
	pedidos := env.jsonLista(g, "pedidos")
	if len(pedidos) == 0 {
		t.Fatalf("nenhum pedido gerado: %s", g.corpo)
	}
	return peca, falta, int64(pedidos[0]["id"].(float64))
}

func TestItem4ReceberPedidoDuasVezesNaoDuplicaEstoque(t *testing.T) {
	env := novaAppTeste(t)
	peca, falta, pedido := prepararPedido(t, env, "PED-001")

	r := env.post("/api/pedidos/"+itoa(pedido)+"/receber", map[string]any{})
	if r.status != 200 {
		t.Fatalf("primeiro recebimento: %d %s", r.status, r.corpo)
	}
	if got := env.estoque(peca); got != 5 {
		t.Fatalf("estoque depois do primeiro recebimento = %v, esperado 5", got)
	}
	if st := env.qstr(`SELECT status FROM faltas WHERE id=?`, falta); st != "recebida" {
		t.Errorf("ficha continuou %q, esperado recebida", st)
	}
	if st := env.qstr(`SELECT status FROM orders WHERE id=?`, pedido); st != "recebido" {
		t.Errorf("pedido ficou %q, esperado recebido", st)
	}

	r = env.post("/api/pedidos/"+itoa(pedido)+"/receber", map[string]any{})
	if r.status != 200 && r.status != 409 {
		t.Fatalf("segundo recebimento: status inesperado %d (%s)", r.status, r.corpo)
	}
	if r.status != 409 {
		t.Errorf("segundo recebimento deveria retornar 409 (pedido ja recebido), veio %d", r.status)
	}
	if msg := r.erro(t); !contemTexto(msg, "já", "recebido") && !contemTexto(msg, "recebido") {
		t.Errorf("mensagem do 409 pouco clara: %q", msg)
	}
	if got := env.estoque(peca); got != 5 {
		t.Errorf("estoque duplicado: %v, esperado 5", got)
	}
	if n := env.qint(`SELECT COUNT(*) FROM stock_moves WHERE part_id=? AND kind='entrada'`, peca); n != 1 {
		t.Errorf("movimentos de entrada = %d, esperado 1", n)
	}
}

func TestItem4PedidoComItemSemFichaFecha(t *testing.T) {
	env := novaAppTeste(t)
	peca, _, pedido := prepararPedido(t, env, "PED-002")
	outra := env.criarPeca("PED-002B", "Peca sem ficha", 800, 1600, 0)

	// item extra no mesmo pedido, sem ficha de falta
	if _, err := env.app.db.Exec(`INSERT INTO order_items(order_id,falta_id,part_id,description,qty,cost_cents)
		VALUES(?,NULL,?,?,2,800)`, pedido, outra, "Peca sem ficha"); err != nil {
		t.Fatalf("inserindo item sem ficha: %v", err)
	}

	r := env.post("/api/pedidos/"+itoa(pedido)+"/receber", map[string]any{})
	if r.status != 200 {
		t.Fatalf("receber pedido: %d %s", r.status, r.corpo)
	}
	if st := env.qstr(`SELECT status FROM orders WHERE id=?`, pedido); st != "recebido" {
		t.Errorf("pedido com item sem ficha ficou %q, esperado recebido", st)
	}
	if got := env.estoque(peca); got != 5 {
		t.Errorf("estoque do item com ficha = %v, esperado 5", got)
	}
	if got := env.estoque(outra); got != 2 {
		t.Errorf("estoque do item sem ficha = %v, esperado 2", got)
	}
}

func TestItem4FalhaEmUmItemNaoDeixaEstoqueParcial(t *testing.T) {
	env := novaAppTeste(t)
	peca, falta, pedido := prepararPedido(t, env, "PED-003")

	// item que aponta para uma peca inexistente: o recebimento tem que falhar
	if _, err := env.app.db.Exec(`INSERT INTO order_items(order_id,falta_id,part_id,description,qty,cost_cents)
		VALUES(?,NULL,999999,'Peca fantasma',1,100)`, pedido); err != nil {
		t.Fatalf("inserindo item invalido: %v", err)
	}

	r := env.post("/api/pedidos/"+itoa(pedido)+"/receber", map[string]any{})
	if r.status == 200 {
		t.Fatalf("recebimento deveria falhar por causa do item invalido, veio 200: %s", r.corpo)
	}
	if r.status != 400 && r.status != 409 && r.status != 500 {
		t.Fatalf("status inesperado: %d %s", r.status, r.corpo)
	}
	if got := env.estoque(peca); got != 0 {
		t.Errorf("estoque parcial: %v, esperado 0 (nada pode ser gravado)", got)
	}
	if st := env.qstr(`SELECT status FROM faltas WHERE id=?`, falta); st == "recebida" {
		t.Errorf("ficha foi marcada como recebida mesmo com falha no pedido")
	}
	if st := env.qstr(`SELECT status FROM orders WHERE id=?`, pedido); st != "aberto" {
		t.Errorf("pedido ficou %q, esperado aberto", st)
	}
	if n := env.qint(`SELECT COUNT(*) FROM order_items WHERE order_id=? AND IFNULL(received_at,'')<>''`, pedido); n != 0 {
		t.Errorf("%d itens marcados como recebidos apesar da falha", n)
	}
	if n := env.qint(`SELECT COUNT(*) FROM stock_moves WHERE ref_type='pedido' AND ref_id=?`, pedido); n != 0 {
		t.Errorf("movimentos de estoque gravados apesar da falha: %d", n)
	}
}

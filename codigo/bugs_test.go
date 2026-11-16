package main

// Caça a bugs: cada teste aqui DESCREVE o comportamento correto esperado.
// Enquanto falhar, o bug existe.

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------- BUG A: receber a mesma ficha de falta duas vezes duplica o estoque ----------
// Caminho: tela "Fichas de falta" -> botao "Receber" (POST /api/fichas/{id}/status).
// Nada impede receber de novo (duplo clique, lista desatualizada em outro terminal).

func TestBugFichaRecebidaDuasVezesDuplicaEstoque(t *testing.T) {
	env := novaAppTeste(t)
	peca := env.criarPeca("BUG-F1", "Peca da ficha", 1000, 2000, 0)
	r := env.post("/api/fichas", map[string]any{
		"peca_id": peca, "descricao": "Peca da ficha", "quantidade": 2, "cliente_nome": "Cliente",
	})
	if r.status != 200 {
		t.Fatalf("criar ficha: %d %s", r.status, r.corpo)
	}
	ficha := num(r.json(t), "id")

	primeiro := env.post("/api/fichas/"+itoa(ficha)+"/status", map[string]any{"status": "recebida"})
	if primeiro.status != 200 {
		t.Fatalf("primeiro recebimento: %d %s", primeiro.status, primeiro.corpo)
	}
	if got := env.estoque(peca); got != 2 {
		t.Fatalf("estoque depois do primeiro recebimento = %v, esperado 2", got)
	}

	segundo := env.post("/api/fichas/"+itoa(ficha)+"/status", map[string]any{"status": "recebida"})
	if segundo.status == 200 {
		t.Errorf("segundo recebimento da MESMA ficha foi aceito (200): %s", segundo.corpo)
	}
	if got := env.estoque(peca); got != 2 {
		t.Errorf("estoque duplicado: %v, esperado 2 (a ficha ja tinha sido recebida)", got)
	}
	if n := env.qint(`SELECT COUNT(*) FROM stock_moves WHERE part_id=?`, peca); n != 1 {
		t.Errorf("movimentos de estoque = %d, esperado 1", n)
	}
}

// ---------- BUG A2: receber pela ficha o que o pedido de compra ja recebeu ----------

func TestBugReceberFichaJaRecebidaPeloPedido(t *testing.T) {
	env := novaAppTeste(t)
	peca, ficha, pedido := prepararPedido(t, env, "BUG-F2")

	if r := env.post("/api/pedidos/"+itoa(pedido)+"/receber", map[string]any{}); r.status != 200 {
		t.Fatalf("receber pedido: %d %s", r.status, r.corpo)
	}
	if got := env.estoque(peca); got != 5 {
		t.Fatalf("estoque depois do pedido = %v, esperado 5", got)
	}
	// a ficha ficou 'recebida'; a tela do pedido nao oferece mais o botao, mas a
	// ficha continua acessivel por outro terminal / lista antiga
	r := env.post("/api/fichas/"+itoa(ficha)+"/status", map[string]any{"status": "recebida"})
	if r.status == 200 {
		t.Errorf("receber pela ficha uma ficha ja recebida pelo pedido foi aceito: %s", r.corpo)
	}
	if got := env.estoque(peca); got != 5 {
		t.Errorf("estoque duplicado ao receber pela ficha: %v, esperado 5", got)
	}
}

// ---------- BUG B: CSV exportado nao vem como download ----------
// handleReportCSV escreve o corpo ANTES de definir Content-Type/Content-Disposition;
// em net/http os cabecalhos ja foram enviados, entao o navegador abre o texto na tela
// em vez de baixar o arquivo (e sem nome de arquivo).

func TestBugCSVSemCabecalhoDeDownload(t *testing.T) {
	env := novaAppTeste(t)
	env.criarPeca("BUG-CSV", "Peca do relatorio", 1000, 2000, 3)

	r := env.get("/api/relatorios/estoque_valor/csv")
	if r.status != 200 {
		t.Fatalf("CSV: status %d corpo %s", r.status, r.corpo)
	}
	if ct := r.headers.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, esperado text/csv (navegador nao reconhece como CSV)", ct)
	}
	if cd := r.headers.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, esperado attachment (arquivo deveria ser baixado)", cd)
	}
	// o PDF, para comparar, define os cabecalhos corretamente
	// (se este falhar tambem, o problema e maior do que o CSV)
	if r := env.get("/api/relatorios/estoque_valor/pdf"); r.status != 200 {
		t.Fatalf("PDF: %d", r.status)
	}
}

// ---------- BUG C: PDF do relatorio corrompe acentos ----------
// pdf.go declara /Encoding /WinAnsiEncoding (1 byte por caractere) mas escreve a
// string em UTF-8: "ó" vira 0xC3 0xB3, que o leitor mostra como "Ã³".

func TestBugPDFComAcentoCorrompido(t *testing.T) {
	env := novaAppTeste(t)
	// descricao com acento que aparece no relatorio de estoque
	env.criarPeca("BUG-ACENTO", "Filtro de óleo Ção", 1000, 2000, 3)

	r := env.get("/api/relatorios/estoque_valor/pdf")
	if r.status != 200 {
		t.Fatalf("PDF: status %d corpo %s", r.status, r.corpo)
	}
	pdf := r.corpo
	if !bytes.Contains(pdf, []byte("Filtro de ")) {
		t.Fatalf("o PDF nao contem o texto da peca (teste invalido)")
	}
	// "ó" em latin-1/WinAnsi = 0xF3 (1 byte). Em UTF-8 = 0xC3 0xB3.
	if bytes.Contains(pdf, []byte("ó")) {
		t.Errorf("o PDF contem 'ó' em UTF-8 (0xC3 0xB3) dentro de uma fonte WinAnsi: o texto sai como \"Ã³\"")
	}
	if !bytes.Contains(pdf, []byte{0xF3}) {
		t.Errorf("o PDF nao tem o byte 0xF3 (latin-1 de 'ó'): acentos nao foram convertidos para WinAnsi")
	}
}

// ---------- BUG D: venda devolve ids como objeto {Int64,Valid} ----------
// getSale serializa sql.NullInt64 direto no JSON. O contrato da API sempre devolveu
// numero ou null; hoje o front recebe {"Int64":0,"Valid":false}.

func TestBugVendaJSONComNullInt64(t *testing.T) {
	env := novaAppTeste(t)
	peca := env.criarPeca("BUG-JSON", "Peca do json", 1000, 2000, 5)
	r := env.post("/api/vendas", corpoVenda(peca, 1, 2000, 0, 0, "dinheiro", map[string]any{"recebido_centavos": 2000}))
	if r.status != 200 {
		t.Fatalf("criar venda: %d %s", r.status, r.corpo)
	}
	v := r.json(t)

	if _, virouObjeto := v["cliente_id"].(map[string]any); virouObjeto {
		t.Errorf("cliente_id veio como objeto %v; deveria ser numero ou null", v["cliente_id"])
	}
	itens, _ := v["itens"].([]any)
	if len(itens) == 0 {
		t.Fatalf("venda sem itens na resposta")
	}
	item := itens[0].(map[string]any)
	if _, virouObjeto := item["peca_id"].(map[string]any); virouObjeto {
		t.Errorf("itens[0].peca_id veio como objeto %v; deveria ser numero ou null", item["peca_id"])
	}
	// e a venda de item livre (sem peca) deve devolver null, nao um objeto invalido
	r2 := env.post("/api/vendas", map[string]any{
		"itens":           []any{map[string]any{"descricao": "Servico avulso", "quantidade": 1, "preco_centavos": 1500}},
		"forma_pagamento": "dinheiro", "recebido_centavos": 1500,
	})
	if r2.status != 200 {
		t.Fatalf("venda com item livre: %d %s", r2.status, r2.corpo)
	}
	livre := (r2.json(t)["itens"].([]any))[0].(map[string]any)
	if _, virouObjeto := livre["peca_id"].(map[string]any); virouObjeto {
		t.Errorf("peca_id de item livre veio como objeto %v; deveria ser null", livre["peca_id"])
	}
}

// ---------- BUG E: dias de atraso contados em UTC (erram por 1 dia) ----------
// handleListReceivables/handleListFaltas fazem time.Parse (que devolve UTC) e
// comparam com time.Now() (horário local). Em fuso -3, no fim do dia, uma conta
// que venceu ONTEM aparece com 2 dias de atraso.

func TestBugDiasDeAtrasoErradoPorFuso(t *testing.T) {
	original := time.Local
	defer func() { time.Local = original }()
	// Coloca o relogio local ~23h de ONTEM em relacao ao UTC (fuso negativo),
	// cenario normal no Brasil no fim do dia: a data local ja e um dia e a
	// comparacao feita em UTC conta um dia a mais.
	agoraUTC := time.Now().UTC()
	offset := -(agoraUTC.Hour()*3600 + agoraUTC.Minute()*60 + agoraUTC.Second()) - 3600
	time.Local = time.FixedZone("BRT", offset)

	// handler direto (sem servidor HTTP): este teste troca time.Local, o que
	// seria uma corrida de dados com as goroutines do httptest.
	env := novaAppSemServidor(t)
	hoje := time.Now()
	ontem := hoje.AddDate(0, 0, -1).Format("2006-01-02")
	_, err := env.app.db.Exec(`INSERT INTO receivables(sale_id,customer_id,customer_name,amount_cents,due_date,status,created_at,paid_at)
		VALUES(1,NULL,'Cliente do fuso',10000,?,'aberto',?,'')`, ontem, nowISO())
	if err != nil {
		t.Fatalf("inserindo conta: %v", err)
	}
	itens := env.jsonLista(env.get("/api/recebiveis?status=todos"), "itens")
	if len(itens) == 0 {
		t.Fatalf("conta nao apareceu na lista")
	}
	atraso := int64(itens[0]["dias_atraso"].(float64))
	if atraso != 1 {
		t.Errorf("dias_atraso = %d para uma conta vencida ONTEM, esperado 1", atraso)
	}
	if !itens[0]["vencido"].(bool) {
		t.Errorf("conta vencida ontem nao marcada como vencida")
	}
}

// ---------- BUG F: contas com campo nulo somem da lista (dinheiro invisivel) ----------
// handleListReceivables le sale_id (int64) e paid_at (string) sem IFNULL: qualquer
// linha com NULL no Scan faz o "continue" silencioso e a conta desaparece da tela
// e do total em aberto. Pode acontecer em banco restaurado/importado.

func TestBugContaComCampoNuloSomeDaLista(t *testing.T) {
	env := novaAppTeste(t)
	_, err := env.app.db.Exec(`INSERT INTO receivables(sale_id,customer_id,customer_name,amount_cents,due_date,status,created_at)
		VALUES(NULL,NULL,'Conta sem venda',10000,'2030-01-10','aberto',?)`, nowISO())
	if err != nil {
		t.Fatalf("inserindo conta: %v", err)
	}
	_, err = env.app.db.Exec(`INSERT INTO receivables(sale_id,customer_id,customer_name,amount_cents,due_date,status,created_at,paid_at)
		VALUES(1,NULL,'Conta sem paid_at',20000,'2030-01-11','aberto',?,NULL)`, nowISO())
	if err != nil {
		t.Fatalf("inserindo conta sem paid_at: %v", err)
	}

	r := env.get("/api/recebiveis?status=todos")
	itens := env.jsonLista(r, "itens")
	if len(itens) != 2 {
		t.Errorf("lista trouxe %d conta(s), esperado 2 (linhas com NULL estao sendo descartadas em silencio): %s",
			len(itens), r.corpo)
	}
	if total := num(r.json(t), "total_aberto_centavos"); total != 30000 {
		t.Errorf("total em aberto = %d, esperado 30000 (contas com campo nulo sumiram da soma)", total)
	}
}

// ---------- BUG G: importacao XML le "1.234" como R$ 1,23 ----------
// parseMoneyBR (usado no XML do fornecedor) tem a mesma regra do front:
// o ponto so e tratado como milhar quando existe virgula.

func TestBugMilharNaImportacaoXML(t *testing.T) {
	if got := parseMoneyBR("1.234"); got != 123400 {
		t.Errorf(`parseMoneyBR("1.234") = %d, esperado 123400 (R$ 1.234,00)`, got)
	}
	if got := parseMoneyBR("10.000"); got != 1000000 {
		t.Errorf(`parseMoneyBR("10.000") = %d, esperado 1000000`, got)
	}
	if got := parseMoneyBR("1.234,56"); got != 123456 {
		t.Errorf(`parseMoneyBR("1.234,56") = %d, esperado 123456`, got)
	}
	if got := parseMoneyBR("12,50"); got != 1250 {
		t.Errorf(`parseMoneyBR("12,50") = %d, esperado 1250`, got)
	}
	if got := parseMoneyBR("12.50"); got != 1250 {
		t.Errorf(`parseMoneyBR("12.50") = %d, esperado 1250`, got)
	}
	if got := parseQty("1.000"); got != 1000 {
		t.Errorf(`parseQty("1.000") = %v, esperado 1000`, got)
	}
	if got := parseQty("1,5"); got != 1.5 {
		t.Errorf(`parseQty("1,5") = %v, esperado 1.5`, got)
	}

	// ponta a ponta: catalogo com preco de milhar no formato NF-e (vUnCom)
	env := novaAppTeste(t)
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<produtos><produto>
<cProd>MIL-1</cProd><xProd>Radiador grande</xProd>
<vUnCom>1.234</vUnCom><qCom>2</qCom>
</produto></produtos>`
	// preco_xml_eh_custo = false: o preco do XML e o preco de venda
	r := env.enviarArquivoOpcoes("/api/xml/importar", "catalogo.xml", []byte(xml), `{"preco_xml_eh_custo":false}`)
	if r.status != 200 {
		t.Fatalf("importar XML: %d %s", r.status, r.corpo)
	}
	preco := env.qint(`SELECT price_cents FROM parts WHERE code='MIL-1'`)
	if preco != 123400 {
		t.Errorf("preco gravado = %d centavos, esperado 123400 (R$ 1.234,00) — o XML trouxe \"1.234\"", preco)
	}
}

// ---------- BUG H: XML de NF-e (vUnCom) importa a peca com preco ZERO ----------
// fieldAliases["preco"] nao tem a tag padrao da NF-e (vUnCom), embora tenha
// vProd/qCom/uCom. O proprio arquivo de exemplo da entrega usa vUnCom: a peca
// entra no cadastro custando e vendendo R$ 0,00.

func TestBugPrecoDoXMLNFeNaoImportado(t *testing.T) {
	env := novaAppTeste(t)
	// o arquivo de exemplo vive na pasta "entrega" do repositorio, um nivel
	// acima do pacote Go (onde o "go test" roda). O caminho absoluto antigo
	// (/home/user/...) so existia na maquina de quem escreveu o teste.
	caminho := "../entrega/exemplos-xml/exemplo-nfe-fornecedor.xml"
	dados, err := os.ReadFile(caminho)
	if err != nil {
		t.Skipf("arquivo de exemplo nao encontrado em %s: %v", caminho, err)
	}
	r := env.enviarArquivoOpcoes("/api/xml/importar", "exemplo-nfe-fornecedor.xml", dados, `{"preco_xml_eh_custo":false}`)
	if r.status != 200 {
		t.Fatalf("importar XML de exemplo: %d %s", r.status, r.corpo)
	}
	if n := env.qint(`SELECT COUNT(*) FROM parts`); n == 0 {
		t.Fatalf("nenhuma peca importada do exemplo: %s", r.corpo)
	}
	preco := env.qint(`SELECT price_cents FROM parts WHERE code LIKE 'SKU-7781%'`)
	if preco != 32000 {
		t.Errorf("preco de SKU-7781 = %d centavos, esperado 32000 (vUnCom 320.00 do XML de exemplo)", preco)
	}
	if zeros := env.qint(`SELECT COUNT(*) FROM parts WHERE price_cents=0`); zeros > 0 {
		t.Errorf("%d peca(s) importada(s) com preco R$ 0,00 (tag vUnCom ignorada)", zeros)
	}
}

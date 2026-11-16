package main

// Testes da 1.0.8: a nota do fornecedor vira conta a pagar sozinha.
//
// O que se quer provar aqui:
//  1. o fornecedor do XML e cadastrado (uma vez so, sem duplicar por causa do CNPJ);
//  2. as duplicatas (boletos) viram uma conta a pagar cada, com vencimento e valor;
//  3. sem duplicata, entra uma conta unica com o vNF e o vencimento escolhido;
//  4. frete/IPI/ST entram no custo da peca (rateio) - senao o lucro mente;
//  5. reimportar a mesma nota NAO lanca a despesa duas vezes (trava pela chave);
//  6. lista de produtos (sem cabecalho de nota) continua funcionando como antes,
//     sem criar conta a pagar nenhuma;
//  7. a conta pode ser paga, editada e cancelada; o cadastro manual funciona;
//  8. o total do painel (dashboard) mostra o que esta a pagar e o que venceu.

import (
	"strings"
	"testing"
)

const xmlNotaCompleta = `<?xml version="1.0" encoding="UTF-8"?>
<nfeProc xmlns="http://www.portalfiscal.inf.br/nfe">
 <NFe><infNFe Id="NFe35261012345678000199550010000123451000123456" versao="4.00">
  <ide><nNF>12345</nNF><serie>1</serie><dhEmi>2026-10-20T10:15:00-03:00</dhEmi></ide>
  <emit><CNPJ>12.345.678/0001-99</CNPJ><xNome>DISTRIBUIDORA DE PECAS LTDA</xNome></emit>
  <det nItem="1"><prod><cProd>P-001</cProd><xProd>PASTILHA DE FREIO</xProd><qCom>10</qCom><vUnCom>100.00</vUnCom><vProd>1000.00</vProd></prod></det>
  <det nItem="2"><prod><cProd>P-002</cProd><xProd>DISCO DE FREIO</xProd><qCom>5</qCom><vUnCom>120.00</vUnCom><vProd>600.00</vProd></prod></det>
  <total><ICMSTot><vProd>1600.00</vProd><vFrete>100.00</vFrete><vIPI>50.00</vIPI><vNF>1750.00</vNF></ICMSTot></total>
  <cobr><dup><nDup>001</nDup><dVenc>2026-11-19</dVenc><vDup>583.33</vDup></dup>
        <dup><nDup>002</nDup><dVenc>2026-12-19</dVenc><vDup>583.33</vDup></dup>
        <dup><nDup>003</nDup><dVenc>2027-01-19</dVenc><vDup>583.34</vDup></dup></cobr>
 </infNFe></NFe>
</nfeProc>`

const xmlNotaSemDuplicata = `<?xml version="1.0" encoding="UTF-8"?>
<nfeProc><NFe><infNFe Id="NFe35261098765432000199550010000009991000009999">
  <ide><nNF>999</nNF><serie>2</serie><dhEmi>2026-10-01T08:00:00-03:00</dhEmi></ide>
  <emit><CNPJ>98765432000199</CNPJ><xNome>OUTRO FORNECEDOR ME</xNome></emit>
  <det><prod><cProd>X-9</cProd><xProd>CORREIA DENTADA</xProd><qCom>2</qCom><vUnCom>80.00</vUnCom><vProd>160.00</vProd></prod></det>
  <total><ICMSTot><vProd>160.00</vProd><vNF>160.00</vNF></ICMSTot></total>
 </infNFe></NFe></nfeProc>`

func importarNota(t *testing.T, env *ambiente, xml string) map[string]any {
	t.Helper()
	r := env.enviarArquivoOpcoes("/api/xml/importar", "nota.xml", []byte(xml),
		`{"margem_percentual":60,"preco_xml_eh_custo":true,"atualizar_estoque":true,"criar_novas":true}`)
	if r.status != 200 {
		t.Fatalf("importar nota: %d %s", r.status, r.corpo)
	}
	var out map[string]any
	jsonUnmarshal([]byte(r.corpo), &out)
	return out
}

func TestNotaViraContaPagarComUmTituloPorBoleto(t *testing.T) {
	env := novaAppTeste(t)
	out := importarNota(t, env, xmlNotaCompleta)

	if env.qint(`SELECT COUNT(*) FROM notas_entrada`) != 1 {
		t.Fatalf("nota de entrada nao gravada")
	}
	if n := env.qint(`SELECT COUNT(*) FROM payables`); n != 3 {
		t.Fatalf("esperava 3 contas (uma por boleto), veio %d", n)
	}
	if n := env.qint(`SELECT COUNT(*) FROM payables WHERE supplier_name='DISTRIBUIDORA DE PECAS LTDA'`); n != 3 {
		t.Errorf("nome do fornecedor do XML nao foi para a conta (%d)", n)
	}
	if n := env.qint(`SELECT COUNT(*) FROM payables WHERE supplier_doc='12345678000199'`); n != 3 {
		t.Errorf("CNPJ do XML nao foi para a conta (%d)", n)
	}
	if total := env.qint(`SELECT SUM(amount_cents) FROM payables`); total != 175000 {
		t.Errorf("soma das contas = %d, esperava 175000 (vNF)", total)
	}
	if venc := env.qstr(`SELECT due_date FROM payables ORDER BY due_date LIMIT 1`); venc != "2026-11-19" {
		t.Errorf("primeiro vencimento = %q, esperava a duplicata 2026-11-19", venc)
	}
	if n := env.qint(`SELECT COUNT(*) FROM payables WHERE due_date='2027-01-19' AND amount_cents=58334`); n != 1 {
		t.Errorf("terceira parcela (583,34 em 19/01/2027) nao entrou")
	}
	// fornecedor cadastrado uma vez, com CNPJ
	if n := env.qint(`SELECT COUNT(*) FROM suppliers`); n != 1 {
		t.Errorf("esperava 1 fornecedor cadastrado, veio %d", n)
	}
	if doc := env.qstr(`SELECT IFNULL(doc,'') FROM suppliers LIMIT 1`); doc != "12345678000199" {
		t.Errorf("CNPJ do fornecedor = %q", doc)
	}
	// a nota fica ligada ao fornecedor e as pecas tambem
	if n := env.qint(`SELECT COUNT(*) FROM parts WHERE supplier_id IS NULL`); n != 0 {
		t.Errorf("%d pecas entraram sem fornecedor", n)
	}
	if v, ok := out["contas_criadas"].(float64); !ok || int(v) != 3 {
		t.Errorf("resposta da importacao nao informou as 3 contas: %v", out["contas_criadas"])
	}
}

func TestRateioDoFreteEntraNoCustoDaPeca(t *testing.T) {
	env := novaAppTeste(t)
	importarNota(t, env, xmlNotaCompleta)

	// PASTILHA: vProd 1.000 de 1.600 => leva 93,75% dos 150,00 de frete+IPI
	// = 93,75 na linha de 10 un => 9,38 por peca => custo 109,38
	custo := env.qint(`SELECT cost_cents FROM parts WHERE code='P-001'`)
	if custo < 10930 || custo > 10945 {
		t.Errorf("custo da pastilha = %d, esperava ~10938 (100,00 + rateio)", custo)
	}
	// DISCO: 600 de 1.600 => 56,25 na linha de 5 un => 11,25 por peca => 131,25
	custo2 := env.qint(`SELECT cost_cents FROM parts WHERE code='P-002'`)
	if custo2 < 13120 || custo2 > 13130 {
		t.Errorf("custo do disco = %d, esperava ~13125", custo2)
	}
	// o rateio inteiro (150,00) tem que aparecer no custo das duas pecas
	soma := float64(custo)*10 + float64(custo2)*5
	base := float64(10000*10 + 12000*5)
	if d := soma - base; d < 14900 || d > 15100 {
		t.Errorf("rateado no custo = %.0f, esperava ~15000", d)
	}
	// o preco de venda continua sendo custo + margem de 60%
	preco := env.qint(`SELECT price_cents FROM parts WHERE code='P-001'`)
	if preco < int64(float64(custo)*1.59) || preco > int64(float64(custo)*1.61) {
		t.Errorf("preco de venda = %d nao acompanhou o custo %d", preco, custo)
	}
}

func TestRateioPodeSerDesligado(t *testing.T) {
	env := novaAppTeste(t)
	r := env.enviarArquivoOpcoes("/api/xml/importar", "nota.xml", []byte(xmlNotaCompleta),
		`{"margem_percentual":60,"preco_xml_eh_custo":true,"atualizar_estoque":true,"criar_novas":true,"ratear_extras":false}`)
	if r.status != 200 {
		t.Fatalf("importar: %d %s", r.status, r.corpo)
	}
	if custo := env.qint(`SELECT cost_cents FROM parts WHERE code='P-001'`); custo != 10000 {
		t.Errorf("com o rateio desligado o custo deveria ser 10000, veio %d", custo)
	}
	if n := env.qint(`SELECT COUNT(*) FROM payables`); n != 3 {
		t.Errorf("a conta a pagar deveria continuar sendo lancada (3 boletos), veio %d", n)
	}
}

func TestContaPagarPodeSerDesligadaNaImportacao(t *testing.T) {
	env := novaAppTeste(t)
	r := env.enviarArquivoOpcoes("/api/xml/importar", "nota.xml", []byte(xmlNotaCompleta),
		`{"margem_percentual":60,"preco_xml_eh_custo":true,"lancar_conta_pagar":false}`)
	if r.status != 200 {
		t.Fatalf("importar: %d %s", r.status, r.corpo)
	}
	if n := env.qint(`SELECT COUNT(*) FROM payables`); n != 0 {
		t.Errorf("nao deveria lancar conta a pagar, veio %d", n)
	}
	if n := env.qint(`SELECT COUNT(*) FROM parts`); n != 2 {
		t.Errorf("as pecas deveriam entrar do mesmo jeito, veio %d", n)
	}
}

func TestReimportarMesmaNotaNaoDuplicaADespesa(t *testing.T) {
	env := novaAppTeste(t)
	importarNota(t, env, xmlNotaCompleta)
	out := importarNota(t, env, xmlNotaCompleta) // segunda vez: o mesmo arquivo

	if n := env.qint(`SELECT COUNT(*) FROM payables`); n != 3 {
		t.Errorf("a mesma nota lancou a despesa de novo: %d contas", n)
	}
	if n := env.qint(`SELECT COUNT(*) FROM notas_entrada`); n != 1 {
		t.Errorf("a mesma nota entrou 2 vezes no historico (%d)", n)
	}
	if v, ok := out["nota_ja_existia"].(bool); !ok || !v {
		t.Errorf("a resposta deveria avisar que a nota ja estava lancada: %v", out["nota_ja_existia"])
	}
	// e o estoque tambem nao pode dobrar: a peca e atualizada, nao recriada
	if n := env.qint(`SELECT COUNT(*) FROM parts`); n != 2 {
		t.Errorf("pecas duplicadas na reimportacao: %d", n)
	}
}

func TestNotaSemDuplicataViraUmaContaComVencimentoEscolhido(t *testing.T) {
	env := novaAppTeste(t)
	r := env.enviarArquivoOpcoes("/api/xml/importar", "nota.xml", []byte(xmlNotaSemDuplicata),
		`{"preco_xml_eh_custo":true,"atualizar_estoque":true,"vencimento":"2026-12-05"}`)
	if r.status != 200 {
		t.Fatalf("importar: %d %s", r.status, r.corpo)
	}
	if n := env.qint(`SELECT COUNT(*) FROM payables`); n != 1 {
		t.Fatalf("esperava 1 conta, veio %d", n)
	}
	if v := env.qstr(`SELECT due_date FROM payables`); v != "2026-12-05" {
		t.Errorf("vencimento = %q, esperava o escolhido 2026-12-05", v)
	}
	if v := env.qint(`SELECT amount_cents FROM payables`); v != 16000 {
		t.Errorf("valor = %d, esperava 16000 (vNF)", v)
	}
}

func TestNotaSemVencimentoUsaEmissaoMaisPrazo(t *testing.T) {
	env := novaAppTeste(t)
	r := env.enviarArquivoOpcoes("/api/xml/importar", "nota.xml", []byte(xmlNotaSemDuplicata),
		`{"preco_xml_eh_custo":true,"prazo_dias":45}`)
	if r.status != 200 {
		t.Fatalf("importar: %d %s", r.status, r.corpo)
	}
	// emitida em 2026-10-01 + 45 dias = 2026-11-15
	if v := env.qstr(`SELECT due_date FROM payables`); v != "2026-11-15" {
		t.Errorf("vencimento = %q, esperava 2026-11-15 (emissao + 45 dias)", v)
	}
}

func TestListaDeProdutosNaoCriaContaPagar(t *testing.T) {
	env := novaAppTeste(t)
	lista := `<?xml version="1.0"?><produtos><produto><codigo>L-1</codigo><descricao>FILTRO DE OLEO</descricao>
		<preco>25.00</preco><quantidade>3</quantidade></produto></produtos>`
	r := env.enviarArquivoOpcoes("/api/xml/importar", "lista.xml", []byte(lista),
		`{"preco_xml_eh_custo":true,"atualizar_estoque":true}`)
	if r.status != 200 {
		t.Fatalf("importar lista: %d %s", r.status, r.corpo)
	}
	if n := env.qint(`SELECT COUNT(*) FROM payables`); n != 0 {
		t.Errorf("lista de produtos nao tem nota: nao deveria criar conta a pagar (%d)", n)
	}
	if n := env.qint(`SELECT COUNT(*) FROM parts`); n != 1 {
		t.Errorf("a peca da lista deveria entrar normalmente (%d)", n)
	}
}

func TestFornecedorComMesmoCnpjNaoDuplica(t *testing.T) {
	env := novaAppTeste(t)
	id := env.criarFornecedor("DISTRIBUIDORA DE PECAS LTDA")
	_, _ = env.app.db.Exec(`UPDATE suppliers SET doc='12.345.678/0001-99' WHERE id=?`, id)

	importarNota(t, env, xmlNotaCompleta)

	if n := env.qint(`SELECT COUNT(*) FROM suppliers`); n != 1 {
		t.Errorf("fornecedor duplicado: %d cadastros", n)
	}
	if n := env.qint(`SELECT COUNT(*) FROM payables WHERE supplier_id=?`, id); n != 3 {
		t.Errorf("as contas nao ficaram ligadas ao fornecedor existente (%d)", n)
	}
}

func TestContaPagarManualPagarEditarCancelar(t *testing.T) {
	env := novaAppTeste(t)
	// criar
	r := env.post("/api/pagaveis", map[string]any{
		"fornecedor": "ALUGUEL DO PONTO", "valor_centavos": 250000, "vencimento": "2026-11-05",
	})
	if r.status != 200 {
		t.Fatalf("criar conta: %d %s", r.status, r.corpo)
	}
	var criada map[string]any
	jsonUnmarshal([]byte(r.corpo), &criada)
	id := int64(criada["id"].(float64))

	lista := env.get("/api/pagaveis?status=aberto")
	itens := env.jsonLista(lista, "itens")
	if len(itens) != 1 {
		t.Fatalf("esperava 1 conta em aberto, veio %d", len(itens))
	}
	if v, _ := lista.json(t)["total_aberto_centavos"].(float64); int64(v) != 250000 {
		t.Errorf("total em aberto = %v", v)
	}
	// editar valor e vencimento
	if r := env.req("PUT", "/api/pagaveis/"+itoa(id), map[string]any{
		"fornecedor": "ALUGUEL DO PONTO", "valor_centavos": 260000, "vencimento": "2026-11-10",
	}); r.status != 200 {
		t.Fatalf("editar conta: %d %s", r.status, r.corpo)
	}
	if v := env.qint(`SELECT amount_cents FROM payables WHERE id=?`, id); v != 260000 {
		t.Errorf("valor depois de editar = %d", v)
	}
	// pagar com valor diferente (juros)
	if r := env.post("/api/pagaveis/"+itoa(id)+"/baixar", map[string]any{"valor_pago_centavos": 262000}); r.status != 200 {
		t.Fatalf("pagar conta: %d %s", r.status, r.corpo)
	}
	if st := env.qstr(`SELECT status FROM payables WHERE id=?`, id); st != "pago" {
		t.Errorf("status depois de pagar = %q", st)
	}
	if v := env.qint(`SELECT paid_cents FROM payables WHERE id=?`, id); v != 262000 {
		t.Errorf("valor pago = %d", v)
	}
	// pagar de novo nao pode
	if r := env.post("/api/pagaveis/"+itoa(id)+"/baixar", map[string]any{}); r.status != 400 {
		t.Errorf("pagar conta ja paga deveria dar 400, veio %d", r.status)
	}
	// cancelar uma conta em aberto
	r = env.post("/api/pagaveis", map[string]any{"fornecedor": "LUZ", "valor_centavos": 30000, "vencimento": "2026-11-20"})
	var c2 map[string]any
	jsonUnmarshal([]byte(r.corpo), &c2)
	id2 := int64(c2["id"].(float64))
	if r := env.post("/api/pagaveis/"+itoa(id2)+"/cancelar", map[string]any{}); r.status != 200 {
		t.Fatalf("cancelar: %d %s", r.status, r.corpo)
	}
	if st := env.qstr(`SELECT status FROM payables WHERE id=?`, id2); st != "cancelado" {
		t.Errorf("status depois de cancelar = %q", st)
	}
	if total := env.qint(`SELECT SUM(amount_cents) FROM payables WHERE status='aberto'`); total != 0 {
		t.Errorf("conta cancelada continua contando no total em aberto (%d)", total)
	}
}

func TestContaPagarRecusaDadosInvalidos(t *testing.T) {
	env := novaAppTeste(t)
	casos := []map[string]any{
		{"valor_centavos": 1000, "vencimento": "2026-11-01"},                     // sem fornecedor
		{"fornecedor": "X", "vencimento": "2026-11-01"},                          // sem valor
		{"fornecedor": "X", "valor_centavos": 1000, "vencimento": "31/12/2026x"}, // data ruim
	}
	for i, c := range casos {
		if r := env.post("/api/pagaveis", c); r.status != 400 {
			t.Errorf("caso %d deveria dar 400, veio %d (%s)", i, r.status, r.corpo)
		}
	}
}

func TestDashboardMostraContasAPagar(t *testing.T) {
	env := novaAppTeste(t)
	_ = env.post("/api/pagaveis", map[string]any{"fornecedor": "FORNECEDOR", "valor_centavos": 50000, "vencimento": "2020-01-10"})
	_ = env.post("/api/pagaveis", map[string]any{"fornecedor": "FORNECEDOR", "valor_centavos": 70000, "vencimento": "2099-01-10"})
	d := env.get("/api/dashboard").json(t)
	if v, _ := d["pagaveis_aberto_centavos"].(float64); int64(v) != 120000 {
		t.Errorf("a pagar em aberto = %v", d["pagaveis_aberto_centavos"])
	}
	if v, _ := d["pagaveis_vencidos_centavos"].(float64); int64(v) != 50000 {
		t.Errorf("a pagar vencido = %v", d["pagaveis_vencidos_centavos"])
	}
	if v, _ := d["pagaveis_vencidos_qtd"].(float64); int(v) != 1 {
		t.Errorf("quantidade vencida = %v", d["pagaveis_vencidos_qtd"])
	}
}

func TestRelatorioContasAPagar(t *testing.T) {
	env := novaAppTeste(t)
	importarNota(t, env, xmlNotaCompleta)
	r := env.get("/api/relatorios/pagaveis")
	if r.status != 200 {
		t.Fatalf("relatorio: %d %s", r.status, r.corpo)
	}
	j := r.json(t)
	if ttl, _ := j["titulo"].(string); !strings.Contains(strings.ToLower(ttl), "pagar") {
		t.Errorf("titulo do relatorio = %q", ttl)
	}
	linhas, _ := j["linhas"].([]any)
	if len(linhas) != 3 {
		t.Errorf("relatorio com %d linhas, esperava 3 boletos", len(linhas))
	}
	// CSV e PDF tambem tem que sair
	if r := env.get("/api/relatorios/pagaveis/csv"); r.status != 200 {
		t.Errorf("csv: %d", r.status)
	}
	if r := env.get("/api/relatorios/pagaveis/pdf"); r.status != 200 {
		t.Errorf("pdf: %d", r.status)
	}
}

func TestHistoricoDeNotasEntrada(t *testing.T) {
	env := novaAppTeste(t)
	importarNota(t, env, xmlNotaCompleta)
	r := env.get("/api/notas-entrada")
	if r.status != 200 {
		t.Fatalf("notas: %d %s", r.status, r.corpo)
	}
	itens := env.jsonLista(r, "itens")
	if len(itens) != 1 {
		t.Fatalf("esperava 1 nota, veio %d", len(itens))
	}
	n := itens[0]
	if n["numero"] != "12345" {
		t.Errorf("numero = %v", n["numero"])
	}
	if v, _ := n["total_centavos"].(float64); int64(v) != 175000 {
		t.Errorf("total da nota = %v", n["total_centavos"])
	}
	if v, _ := n["contas"].(float64); int(v) != 3 {
		t.Errorf("nota deveria mostrar 3 contas ligadas, mostrou %v", n["contas"])
	}
	if v, _ := n["rateado_centavos"].(float64); int64(v) <= 0 {
		t.Errorf("nota deveria registrar o que foi rateado no custo, veio %v", n["rateado_centavos"])
	}
}

func TestAnalisarMostraANotaAntesDeImportar(t *testing.T) {
	env := novaAppTeste(t)
	r := env.enviarArquivoOpcoes("/api/xml/analisar", "nota.xml", []byte(xmlNotaCompleta), "")
	if r.status != 200 {
		t.Fatalf("analisar: %d %s", r.status, r.corpo)
	}
	j := r.json(t)
	nota, ok := j["nota"].(map[string]any)
	if !ok {
		t.Fatalf("analisar nao devolveu o cabecalho da nota: %s", r.corpo)
	}
	if nota["fornecedor"] != "DISTRIBUIDORA DE PECAS LTDA" {
		t.Errorf("fornecedor = %v", nota["fornecedor"])
	}
	if v, _ := nota["total_centavos"].(float64); int64(v) != 175000 {
		t.Errorf("total = %v", nota["total_centavos"])
	}
	if v, _ := nota["extras_centavos"].(float64); int64(v) != 15000 {
		t.Errorf("extras (frete+IPI) = %v", nota["extras_centavos"])
	}
	dups, _ := nota["duplicatas"].([]any)
	if len(dups) != 3 {
		t.Errorf("duplicatas = %d", len(dups))
	}
	if nota["ja_importada"] != false {
		t.Errorf("nota nova nao pode vir como ja importada")
	}
	// e depois de importar, a mesma analise avisa que ja entrou
	importarNota(t, env, xmlNotaCompleta)
	r2 := env.enviarArquivoOpcoes("/api/xml/analisar", "nota.xml", []byte(xmlNotaCompleta), "")
	nota2, _ := r2.json(t)["nota"].(map[string]any)
	if nota2["ja_importada"] != true {
		t.Errorf("depois de importar, a analise deveria avisar que a nota ja esta lancada")
	}
}

func TestCalcularRateioSomaExata(t *testing.T) {
	itens := []xmlItem{
		{Descricao: "A", TotalTxt: "1000.00", EstoqueTxt: "10", PrecoTxt: "100.00"},
		{Descricao: "B", TotalTxt: "600.00", EstoqueTxt: "5", PrecoTxt: "120.00"},
		{Descricao: "C", TotalTxt: "0.40", EstoqueTxt: "1", PrecoTxt: "0.40"},
	}
	extras := int64(15000)
	r := calcularRateio(itens, extras)
	var total int64
	for i, it := range itens {
		q := parseQty(it.EstoqueTxt)
		total += r[i] * int64(q)
	}
	if diff := extras - total; diff > 50 || diff < -50 {
		t.Errorf("rateio somou %d de %d (diferenca %d)", total, extras, diff)
	}
}

package main

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// Testes do módulo Estoque Inteligente (v1.1)
// Usa a infraestrutura de testutil_test.go (ambiente, novaAppTeste, etc.)

func TestEI_DemandaDiaria(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_demanda_dias", "90")
	app.db.SetSetting("ei_lead_dias", "7")
	app.db.SetSetting("ei_seguranca_dias", "3")

	pid := e.criarPeca("TEST001", "Peça Teste", 1000, 2000, 10)
	// Vendas nos últimos 10 dias: 5 unidades total
	de := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	e.criarVendaComItem(pid, 2, 2000, de)
	e.criarVendaComItem(pid, 3, 2000, todayISO())

	d, err := app.DemandaDiaria(pid, 10)
	if err != nil {
		t.Fatal(err)
	}
	if d < 0.49 || d > 0.51 {
		t.Errorf("demanda=%.2f, esperado 0.5", d)
	}
}

func TestEI_DemandaZeroSemVendas(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	pid := e.criarPeca("TEST002", "Sem Vendas", 1000, 2000, 10)
	d, err := app.DemandaDiaria(pid, 90)
	if err != nil {
		t.Fatal(err)
	}
	if d != 0 {
		t.Errorf("demanda sem vendas deve ser 0, got %.2f", d)
	}
}

func TestEI_LeadDiasUsaFornecedor(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	fornID := e.criarFornecedorComPrazo("Fornecedor Lead 10", 10)
	pid := e.criarPecaComFornecedor("TEST003", "Lead 10", 1000, 2000, 10, fornID)
	cfg := app.loadConfigEI()
	lead, err := app.LeadDias(pid, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if lead != 10 {
		t.Errorf("lead esperado 10, got %d", lead)
	}
}

func (e *ambiente) criarFornecedorComPrazo(nome string, prazo int) int64 {
	e.t.Helper()
	r := e.post("/api/fornecedores", map[string]any{"nome": nome, "prazo_entrega_dias": prazo})
	if r.status != 200 {
		e.t.Fatalf("criar fornecedor: %d %s", r.status, string(r.corpo))
	}
	return int64(r.json(e.t)["id"].(float64))
}

func (e *ambiente) criarPecaComFornecedor(codigo, desc string, custo, preco int64, estoque float64, fornID int64) int64 {
	e.t.Helper()
	r := e.post("/api/pecas", map[string]any{
		"codigo": codigo, "descricao": desc, "custo_centavos": custo,
		"preco_centavos": preco, "minimo": 5, "estoque": estoque,
		"fornecedor_id": fornID,
	})
	if r.status != 200 {
		e.t.Fatalf("criar peca: %d %s", r.status, string(r.corpo))
	}
	return int64(r.json(e.t)["id"].(float64))
}

func TestEI_LeadDiasDefaultQuandoSemFornecedor(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	pid := e.criarPeca("NF001", "Sem Forn", 1000, 2000, 10)
	cfg := app.loadConfigEI()
	lead, err := app.LeadDias(pid, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if lead != cfg.LeadDias {
		t.Errorf("lead default esperado %d, got %d", cfg.LeadDias, lead)
	}
}

func TestEI_PontoReposicao(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_demanda_dias", "14")
	app.db.SetSetting("ei_lead_dias", "7")
	app.db.SetSetting("ei_seguranca_dias", "3")
	pid := e.criarPeca("TEST004", "Ponto Rep", 1000, 2000, 10)
	// vendas: 14 un em 14 dias = 1/dia
	de := time.Now().AddDate(0, 0, -14).Format("2006-01-02")
	e.criarVendaComItem(pid, 14, 2000, de)

	cfg := app.loadConfigEI()
	ponto, seg, dias, lead, demanda, err := app.CalculaPontoReposicao(pid, 10, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if demanda < 0.99 || demanda > 1.01 {
		t.Errorf("demanda=%.2f", demanda)
	}
	if lead != 7 {
		t.Errorf("lead=%d", lead)
	}
	if ponto < 9.9 || ponto > 10.1 {
		t.Errorf("ponto=%.1f", ponto)
	}
	if seg < 2.9 || seg > 3.1 {
		t.Errorf("seg=%.1f", seg)
	}
	if dias == nil || *dias < 9.9 || *dias > 10.1 {
		t.Errorf("dias=%.1f", *dias)
	}
}

func TestEI_CriticidadeCriticaEstoqueZero(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_demanda_dias", "10")
	app.db.SetSetting("ei_lead_dias", "7")
	app.db.SetSetting("ei_seguranca_dias", "3")
	pid := e.criarPeca("TEST005", "Critica Zero", 1000, 2000, 0)
	de := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	e.criarVendaComItem(pid, 5, 2000, de)
	cfg := app.loadConfigEI()
	ponto, _, dias, lead, demanda, _ := app.CalculaPontoReposicao(pid, 0, cfg)
	crit := Criticidade(0, 5, ponto, dias, lead, demanda)
	if crit != "critica" {
		t.Errorf("criticidade estoque 0 = %s", crit)
	}
}

func TestEI_CriticidadeCriticaDiasAteFaltarMenorLead(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_demanda_dias", "10")
	app.db.SetSetting("ei_lead_dias", "7")
	app.db.SetSetting("ei_seguranca_dias", "3")
	pid := e.criarPeca("TEST006", "Critica Lead", 1000, 2000, 5)
	de := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	e.criarVendaComItem(pid, 10, 2000, de)
	cfg := app.loadConfigEI()
	ponto, _, dias, lead, demanda, _ := app.CalculaPontoReposicao(pid, 5, cfg)
	crit := Criticidade(5, 5, ponto, dias, lead, demanda)
	if crit != "critica" {
		t.Errorf("criticidade dias<=lead = %s", crit)
	}
}

func TestEI_CriticidadeMediaAbaixoPonto(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_demanda_dias", "10")
	app.db.SetSetting("ei_lead_dias", "7")
	app.db.SetSetting("ei_seguranca_dias", "3")
	pid := e.criarPeca("TEST007", "Media", 1000, 2000, 8)
	de := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	e.criarVendaComItem(pid, 10, 2000, de)
	cfg := app.loadConfigEI()
	ponto, _, dias, lead, demanda, _ := app.CalculaPontoReposicao(pid, 8, cfg)
	crit := Criticidade(8, 5, ponto, dias, lead, demanda)
	if crit != "media" {
		t.Errorf("criticidade abaixo ponto = %s", crit)
	}
}

func TestEI_CriticidadeBaixaAbaixoMinimo(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_demanda_dias", "10")
	app.db.SetSetting("ei_lead_dias", "7")
	app.db.SetSetting("ei_seguranca_dias", "3")
	pid := e.criarPeca("TEST008", "Baixa", 1000, 2000, 3)
	cfg := app.loadConfigEI()
	ponto, _, dias, lead, demanda, _ := app.CalculaPontoReposicao(pid, 3, cfg)
	crit := Criticidade(3, 5, ponto, dias, lead, demanda)
	if crit != "baixa" {
		t.Errorf("criticidade abaixo minimo = %s", crit)
	}
}

func TestEI_QtdSugeridaComDemanda(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_demanda_dias", "10")
	app.db.SetSetting("ei_lead_dias", "7")
	app.db.SetSetting("ei_seguranca_dias", "3")
	pid := e.criarPeca("TEST009", "Qtd Demanda", 1000, 2000, 5)
	de := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	e.criarVendaComItem(pid, 10, 2000, de)
	cfg := app.loadConfigEI()
	ponto, _, _, _, demanda, _ := app.CalculaPontoReposicao(pid, 5, cfg)
	qtd := QtdSugerida(5, 5, ponto, demanda)
	if qtd < 4.9 || qtd > 5.1 {
		t.Errorf("qtd sugerida=%.1f, esperado 5", qtd)
	}
}

func TestEI_QtdSugeridaSemDemanda(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_demanda_dias", "10")
	app.db.SetSetting("ei_lead_dias", "7")
	app.db.SetSetting("ei_seguranca_dias", "3")
	pid := e.criarPeca("TEST010", "Qtd Sem Demanda", 1000, 2000, 2)
	cfg := app.loadConfigEI()
	ponto, _, _, _, demanda, _ := app.CalculaPontoReposicao(pid, 2, cfg)
	qtd := QtdSugerida(2, 5, ponto, demanda)
	if qtd != 3 {
		t.Errorf("qtd sugerida sem demanda=%.1f, esperado 3", qtd)
	}
}

func TestEI_SugestoesAPI(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_demanda_dias", "90")
	app.db.SetSetting("ei_lead_dias", "7")
	app.db.SetSetting("ei_seguranca_dias", "3")

	pid := e.criarPeca("SUG001", "Sugestao API", 1000, 2000, 3)
	de := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	e.criarVendaComItem(pid, 10, 2000, de)

	r := e.get("/api/estoque-inteligente/sugestoes")
	if r.status != 200 {
		t.Fatalf("status %d: %s", r.status, string(r.corpo))
	}
	itens := e.jsonLista(r, "itens")
	if len(itens) == 0 {
		t.Fatal("esperava ao menos 1 sugestão")
	}
	item := itens[0]
	if item["codigo"] != "SUG001" {
		t.Errorf("codigo=%v", item["codigo"])
	}
	if item["criticidade"] != "critica" && item["criticidade"] != "media" {
		t.Errorf("criticidade=%v", item["criticidade"])
	}
}

func TestEI_ParadosAPI(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_parado_dias", "90")
	pid := e.criarPeca("PAR001", "Parado", 1000, 2000, 10)
	// forçar created_at antigo via SQL direto
	_, _ = app.db.Exec(`UPDATE parts SET created_at=? WHERE id=?`,
		time.Now().AddDate(0, 0, -120).Format("2006-01-02 15:04:05"), pid)

	r := e.get("/api/estoque-inteligente/parados")
	if r.status != 200 {
		t.Fatalf("status %d: %s", r.status, string(r.corpo))
	}
	itens := e.jsonLista(r, "itens")
	if len(itens) == 0 {
		t.Fatal("esperava item parado")
	}
	item := itens[0]
	if item["codigo"] != "PAR001" {
		t.Errorf("codigo=%v", item["codigo"])
	}
	dias := item["dias_parado"].(float64)
	if dias < 90 {
		t.Errorf("dias parado=%.0f", dias)
	}
}

func TestEI_ABC_API(t *testing.T) {
	e := novaAppTeste(t)
	app := e.app
	app.db.SetSetting("ei_abc_dias", "90")
	pid := e.criarPeca("ABC001", "ABC Item", 1000, 2000, 10)
	de := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	e.criarVendaComItem(pid, 5, 2000, de)

	r := e.get("/api/estoque-inteligente/abc")
	t.Logf("ABC response: %s", string(r.corpo))
	if r.status != 200 {
		t.Fatalf("status %d: %s", r.status, string(r.corpo))
	}
	itens := e.jsonLista(r, "itens")
	if len(itens) == 0 {
		t.Fatal("esperava item ABC")
	}
	item := itens[0]
	if item["classe"] != "A" {
		t.Errorf("classe=%v, receita=%v", item["classe"], item["receita"])
	}
}

func TestEI_CompararAPI(t *testing.T) {
	e := novaAppTeste(t)
	pid := e.criarPeca("CMP001", "Comparar", 1000, 2000, 10)
	// período 1: 10 dias atrás, 5 vendas
	de1 := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	ate1 := time.Now().AddDate(0, 0, -31).Format("2006-01-02")
	e.criarVendaComItem(pid, 5, 2000, de1)
	// período 2: últimos 10 dias, 15 vendas
	de2 := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	e.criarVendaComItem(pid, 15, 2000, de2)

	q := url.Values{}
	q.Set("de1", de1)
	q.Set("ate1", ate1)
	q.Set("de2", de2)
	q.Set("ate2", todayISO())
	r := e.get("/api/estoque-inteligente/comparar?" + q.Encode())
	t.Logf("Comparar response: %s", string(r.corpo))
	if r.status != 200 {
		t.Fatalf("status %d: %s", r.status, string(r.corpo))
	}
	m := r.json(e.t)
	p1 := m["periodo1"].(map[string]any)
	p2 := m["periodo2"].(map[string]any)
	// Cada chamada a criarVendaComItem cria 1 venda (com 1 item)
	if p1["vendas"].(float64) != 1 {
		t.Errorf("p1 vendas=%v (esperado 1)", p1["vendas"])
	}
	if p2["vendas"].(float64) != 1 {
		t.Errorf("p2 vendas=%v (esperado 1)", p2["vendas"])
	}
	if p1["receita_centavos"].(float64) != 10000 {
		t.Errorf("p1 receita=%v", p1["receita_centavos"])
	}
	if p2["receita_centavos"].(float64) != 30000 {
		t.Errorf("p2 receita=%v", p2["receita_centavos"])
	}
	top := m["top_crescimento"].([]any)
	if len(top) == 0 {
		t.Fatal("esperava top crescimento")
	}
	tc := top[0].(map[string]any)
	if tc["crescimento_pct"].(float64) < 100 {
		t.Errorf("crescimento=%.1f", tc["crescimento_pct"])
	}
}

func TestEI_ConfigGET_POST(t *testing.T) {
	e := novaAppTeste(t)

	// GET
	r := e.get("/api/estoque-inteligente/config")
	if r.status != 200 {
		t.Fatalf("GET config %d", r.status)
	}

	// POST novo valor
	body := map[string]any{
		"lead_dias": 10, "seguranca_dias": 5, "demanda_dias": 60,
		"parado_dias": 120, "abc_dias": 120,
	}
	r = e.post("/api/estoque-inteligente/config", body)
	if r.status != 200 {
		t.Fatalf("POST config %d: %s", r.status, string(r.corpo))
	}

	// Verificar se salvou
	r = e.get("/api/estoque-inteligente/config")
	if r.status != 200 {
		t.Fatalf("GET after POST %d", r.status)
	}
	m := r.json(e.t)
	if m["lead_dias"].(float64) != 10 {
		t.Errorf("lead_dias não salvou: %v", m["lead_dias"])
	}
	if m["seguranca_dias"].(float64) != 5 {
		t.Errorf("seguranca_dias não salvou: %v", m["seguranca_dias"])
	}
}

func TestEI_CSV_Sugestoes(t *testing.T) {
	e := novaAppTeste(t)
	e.app.db.SetSetting("ei_demanda_dias", "90")
	e.app.db.SetSetting("ei_lead_dias", "7")
	e.app.db.SetSetting("ei_seguranca_dias", "3")

	e.criarPeca("CSV001", "CSV Teste", 1000, 2000, 3)
	de := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	e.criarVendaComItem(1, 10, 2000, de)

	r := e.get("/api/estoque-inteligente/sugestoes.csv")
	if r.status != 200 {
		t.Fatalf("CSV %d: %s", r.status, string(r.corpo))
	}
	if !strings.Contains(r.headers.Get("Content-Disposition"), "sugestoes-compra") {
		t.Errorf("filename errado: %s", r.headers.Get("Content-Disposition"))
	}
	if !strings.Contains(string(r.corpo), "CSV001") {
		t.Errorf("CSV não contém peça")
	}
}

func TestEI_PDF_Sugestoes(t *testing.T) {
	e := novaAppTeste(t)
	e.criarPeca("PDF001", "PDF Teste", 1000, 2000, 3)

	r := e.get("/api/estoque-inteligente/sugestoes.pdf")
	if r.status != 200 {
		t.Fatalf("PDF %d: %s", r.status, string(r.corpo))
	}
	if r.headers.Get("Content-Type") != "application/pdf" {
		t.Errorf("content-type=%s", r.headers.Get("Content-Type"))
	}
	if len(r.corpo) < 100 {
		t.Errorf("PDF muito pequeno")
	}
	if !strings.HasPrefix(string(r.corpo), "%PDF") {
		t.Errorf("não é PDF válido")
	}
}

func TestEI_GerarPedido(t *testing.T) {
	e := novaAppTeste(t)
	forn1 := e.criarFornecedor("Fornecedor A")
	forn2 := e.criarFornecedor("Fornecedor B")
	pid1 := e.criarPecaComFornecedor("PED001", "Pedido 1", 1000, 2000, 5, forn1)
	pid2 := e.criarPecaComFornecedor("PED002", "Pedido 2", 1500, 3000, 3, forn2)

	body := map[string]any{
		"itens": []map[string]any{
			{"part_id": pid1, "quantidade": 10},
			{"part_id": pid2, "quantidade": 5},
		},
	}
	r := e.post("/api/estoque-inteligente/pedido", body)
	if r.status != 200 {
		t.Fatalf("GerarPedido %d: %s", r.status, string(r.corpo))
	}
	m := r.json(e.t)
	pedidos := m["pedidos"].([]any)
	if len(pedidos) != 2 {
		t.Errorf("esperava 2 pedidos (fornecedores diferentes), got %d", len(pedidos))
	}
	p1 := pedidos[0].(map[string]any)
	if p1["fornecedor"] == "(sem fornecedor)" {
		t.Errorf("fornecedor não associado")
	}
}

func TestEI_GerarPedidoAgrupaMesmoFornecedor(t *testing.T) {
	e := novaAppTeste(t)
	fornID := e.criarFornecedor("Fornecedor Comum")
	p1 := e.criarPecaComFornecedor("AGR001", "Agrupa 1", 1000, 2000, 5, fornID)
	p2 := e.criarPecaComFornecedor("AGR002", "Agrupa 2", 1500, 3000, 5, fornID)

	body := map[string]any{
		"itens": []map[string]any{
			{"part_id": p1, "quantidade": 10},
			{"part_id": p1, "quantidade": 5}, // mesma peça duas vezes -> deve somar
			{"part_id": p2, "quantidade": 8},
		},
	}
	r := e.post("/api/estoque-inteligente/pedido", body)
	if r.status != 200 {
		t.Fatalf("Agrupa %d: %s", r.status, string(r.corpo))
	}
	m := r.json(e.t)
	pedidos := m["pedidos"].([]any)
	if len(pedidos) != 1 {
		t.Errorf("esperava 1 pedido agrupado, got %d", len(pedidos))
	}
	p := pedidos[0].(map[string]any)
	if p["itens"].(float64) != 2 {
		t.Errorf("itens no pedido=%v (esperado 2 - AGR001 somado)", p["itens"])
	}
}

func TestEI_ResumoAPI(t *testing.T) {
	e := novaAppTeste(t)
	e.app.db.SetSetting("ei_demanda_dias", "90")
	e.app.db.SetSetting("ei_lead_dias", "7")
	e.app.db.SetSetting("ei_seguranca_dias", "3")

	e.criarPeca("RES001", "Resumo 1", 1000, 2000, 2)
	de := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	e.criarVendaComItem(1, 10, 2000, de)

	r := e.get("/api/estoque-inteligente/resumo")
	if r.status != 200 {
		t.Fatalf("Resumo %d: %s", r.status, string(r.corpo))
	}
	m := r.json(e.t)
	if m["sugestoes_count"].(float64) == 0 {
		t.Errorf("sugestoes_count=0")
	}
	if m["pecas_criticas"].(float64) == 0 && m["pecas_media"].(float64) == 0 {
		t.Errorf("nenhuma criticidade detectada")
	}
}

// Helpers extras para o ambiente
func (e *ambiente) criarVendaComItem(partID int64, qty float64, price int64, data string) {
	e.t.Helper()
	// Criar cliente
	cli := e.post("/api/clientes", map[string]any{"nome": "Cliente Teste"})
	cliID := int64(cli.json(e.t)["id"].(float64))

	// Criar venda (formato que o front envia)
	venda := map[string]any{
		"cliente_id":      cliID,
		"itens":           []map[string]any{{"peca_id": partID, "quantidade": qty, "preco_centavos": price}},
		"forma_pagamento": "dinheiro",
		"data":            data,
	}
	r := e.post("/api/vendas", venda)
	if r.status >= 400 {
		e.t.Fatalf("criar venda: %d %s", r.status, string(r.corpo))
	}
}

// hojeISO igual à do app (para testes)
func hojeISO() string {
	return time.Now().Format("2006-01-02")
}

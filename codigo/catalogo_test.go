package main

// Testes do catalogo do fornecedor: a peca que voce nao tem no estoque, mas
// precisa achar na hora em que o cliente esta no balcao (antes: catalogo de papel).

import (
	"fmt"
	"strings"
	"testing"
)

func catalogoSalvar(env *ambiente, corpo map[string]any) resposta {
	env.t.Helper()
	return env.post("/api/catalogo", corpo)
}

// TestCatalogoCadastroEBuscaPorAplicacao: cadastrar uma peca de catalogo com a
// aplicacao e achar buscando pelo carro do cliente ("gol 2012").
func TestCatalogoCadastroEBuscaPorAplicacao(t *testing.T) {
	env := novaAppTeste(t)
	r := catalogoSalvar(env, map[string]any{
		"codigo": "W712/90", "descricao": "FILTRO DE OLEO",
		"aplicacao": "VW GOL 1.0/1.6 08/16", "medida": "ROSCA 3/4-16",
		"marca": "MANN", "custo_centavos": 1850,
		"equivalentes": []string{"TECFIL PSL560", "FRAM PH2870A"},
	})
	if r.status != 200 {
		t.Fatalf("cadastro no catalogo: %d %s", r.status, r.corpo)
	}

	for _, busca := range []string{"gol 2012", "GOL", "filtro de oleo", "W712/90", "w71290", "TECFIL PSL560", "PSL560"} {
		lista := env.get("/api/catalogo?q=" + urlQuery(busca))
		if lista.status != 200 {
			t.Fatalf("busca %q: %d %s", busca, lista.status, lista.corpo)
		}
		itens, _ := lista.json(t)["itens"].([]any)
		if len(itens) != 1 {
			t.Errorf("busca %q devolveu %d itens, esperava 1", busca, len(itens))
			continue
		}
		item := itens[0].(map[string]any)
		if item["codigo"] != "W712/90" {
			t.Errorf("busca %q: item errado (%v)", busca, item["codigo"])
		}
	}
}

// TestCatalogoMostraSeTemNoEstoque: a mesma busca tem que dizer se a peca esta
// no estoque, com quantas unidades - e pelo codigo equivalente tambem.
func TestCatalogoMostraSeTemNoEstoque(t *testing.T) {
	env := novaAppTeste(t)
	catalogoSalvar(env, map[string]any{
		"codigo": "W712/90", "descricao": "FILTRO DE OLEO",
		"aplicacao": "VW GOL 1.0/1.6 08/16",
	})
	// a peca existe no estoque com OUTRO codigo proprio, ligada pelo equivalente
	peca := env.criarPeca("FIL-001", "FILTRO DE OLEO GOL", 1000, 2490, 3)
	if r := env.req("PUT", "/api/pecas/"+itoa(peca), map[string]any{
		"id": peca, "codigo": "FIL-001", "descricao": "FILTRO DE OLEO GOL",
		"custo_centavos": 1000, "preco_centavos": 2490, "minimo": 1, "estoque": 3,
		"equivalentes": []string{"W712/90"},
	}); r.status != 200 {
		t.Fatalf("ligando equivalente: %d %s", r.status, r.corpo)
	}

	lista := env.get("/api/catalogo?q=gol").json(t)
	itens, _ := lista["itens"].([]any)
	if len(itens) != 1 {
		t.Fatalf("esperava 1 item, veio %d", len(itens))
	}
	item := itens[0].(map[string]any)
	if item["tem_estoque"] != true {
		t.Fatalf("o catalogo deveria dizer que TEM no estoque: %v", item)
	}
	if int(item["estoque"].(float64)) != 3 {
		t.Fatalf("estoque informado %v, esperava 3", item["estoque"])
	}
	if item["peca_codigo"] != "FIL-001" {
		t.Fatalf("codigo da peca no estoque = %v, esperava FIL-001", item["peca_codigo"])
	}
}

// TestCatalogoImportarCSV: subir a planilha/lista do fornecedor de uma vez.
func TestCatalogoImportarCSV(t *testing.T) {
	env := novaAppTeste(t)
	csv := "codigo;descricao;aplicacao;medida;marca;custo;equivalentes\n" +
		"W712/90;FILTRO DE OLEO;VW GOL 1.0/1.6 08/16;ROSCA 3/4-16;MANN;18,50;TECFIL PSL560\n" +
		"AF-1020;FILTRO DE AR;VW GOL G5/G6 1.0 08/16;265X165X38;MANN;29,90;\n" +
		"PA-5010;PASTILHA DE FREIO DIANT;VW GOL 1.0 13/20;JOGO 4 PECAS;FREMEC;64,00;FRAS-LE PD/1001\n"
	r := env.enviarArquivo("/api/catalogo/importar", "arquivo", "catalogo.csv", []byte(csv))
	if r.status != 200 {
		t.Fatalf("importar catalogo: %d %s", r.status, r.corpo)
	}
	m := r.json(t)
	if int(m["importados"].(float64)) != 3 {
		t.Fatalf("esperava 3 importados: %s", r.corpo)
	}
	// o codigo equivale a busca por aplicacao e por equivalente
	for busca, esperado := range map[string]string{
		"gol 1.0 13/20": "PA-5010",
		"265X165X38":    "AF-1020",
		"PSL560":        "W712/90",
	} {
		itens, _ := env.get("/api/catalogo?q=" + urlQuery(busca)).json(t)["itens"].([]any)
		if len(itens) != 1 || itens[0].(map[string]any)["codigo"] != esperado {
			t.Errorf("busca %q nao achou %s (veio %v)", busca, esperado, itens)
		}
	}
	// importar o mesmo arquivo de novo nao duplica
	r2 := env.enviarArquivo("/api/catalogo/importar", "arquivo", "catalogo.csv", []byte(csv))
	if int(r2.json(t)["atualizados"].(float64)) != 3 {
		t.Fatalf("segunda importacao deveria atualizar, nao duplicar: %s", r2.corpo)
	}
	if n := env.contar(t, "catalog"); n != 3 {
		t.Fatalf("catalogo com %d linhas depois de importar duas vezes (esperava 3)", n)
	}
}

// TestCatalogoSemEstoqueGeraFicha: o ganho do balcao - achou no catalogo,
// nao tem no estoque, gera a ficha de falta ja com codigo e aplicacao.
func TestCatalogoSemEstoqueGeraFicha(t *testing.T) {
	env := novaAppTeste(t)
	catalogoSalvar(env, map[string]any{
		"codigo": "PA-5010", "descricao": "PASTILHA DE FREIO DIANTEIRA",
		"aplicacao": "VW GOL 1.0 13/20", "marca": "FREMEC", "custo_centavos": 6400,
	})
	id := env.qint(`SELECT id FROM catalog WHERE code_norm=?`, normCode("PA-5010"))

	r := env.post("/api/catalogo/"+itoa(id)+"/ficha", map[string]any{
		"quantidade": 1, "cliente_nome": "Sr. Carlos", "veiculo": "Gol 2013 placa ABC1234",
	})
	if r.status != 200 {
		t.Fatalf("gerar ficha pelo catalogo: %d %s", r.status, r.corpo)
	}
	var desc, veiculo, status string
	var qtd float64
	if err := env.app.db.QueryRow(`SELECT description,vehicle,status,qty FROM faltas ORDER BY id DESC LIMIT 1`).
		Scan(&desc, &veiculo, &status, &qtd); err != nil {
		t.Fatalf("ficha nao foi criada: %v", err)
	}
	if !strings.Contains(desc, "PA-5010") || !strings.Contains(strings.ToUpper(desc), "PASTILHA") {
		t.Errorf("descricao da ficha deveria citar codigo e peca: %q", desc)
	}
	if veiculo != "Gol 2013 placa ABC1234" {
		t.Errorf("veiculo da ficha = %q", veiculo)
	}
	if status != "aberta" || qtd != 1 {
		t.Errorf("ficha com status=%q qtd=%v", status, qtd)
	}
}

// TestCatalogoNaoPoluiEstoque: cadastrar no catalogo NAO cria peca nem estoque.
func TestCatalogoNaoPoluiEstoque(t *testing.T) {
	env := novaAppTeste(t)
	catalogoSalvar(env, map[string]any{"codigo": "X-1", "descricao": "PECA QUE NAO TENHO", "aplicacao": "FIAT UNO"})
	if n := env.contar(t, "parts"); n != 0 {
		t.Fatalf("o catalogo criou %d pecas no estoque", n)
	}
	if n := env.contar(t, "stock_moves"); n != 0 {
		t.Fatalf("o catalogo lancou %d movimentos de estoque", n)
	}
}

// TestCatalogoCodigoRepetidoAtualiza: digitar o mesmo codigo de novo atualiza,
// nao da erro nem duplica (e o uso real: pessoa corrigindo o que digitou).
func TestCatalogoCodigoRepetidoAtualiza(t *testing.T) {
	env := novaAppTeste(t)
	catalogoSalvar(env, map[string]any{"codigo": "W712/90", "descricao": "FILTRO", "aplicacao": "GOL"})
	r := catalogoSalvar(env, map[string]any{"codigo": "w712-90", "descricao": "FILTRO DE OLEO", "aplicacao": "VW GOL 08/16"})
	if r.status != 200 {
		t.Fatalf("segundo cadastro: %d %s", r.status, r.corpo)
	}
	if n := env.contar(t, "catalog"); n != 1 {
		t.Fatalf("catalogo com %d linhas (esperava 1)", n)
	}
	var desc string
	if err := env.app.db.QueryRow(`SELECT description FROM catalog`).Scan(&desc); err != nil {
		t.Fatal(err)
	}
	if desc != "FILTRO DE OLEO" {
		t.Fatalf("descricao nao foi atualizada: %q", desc)
	}
}

// TestImportacaoXMLGuardaAplicacao: a aplicacao que vem no XML era lida e
// JOGADA FORA. Agora fica na peca e na busca.
func TestImportacaoXMLGuardaAplicacao(t *testing.T) {
	env := novaAppTeste(t)
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<catalogo>
  <produto>
    <codigo>XML-APP-1</codigo>
    <descricao>FILTRO DE OLEO</descricao>
    <aplicacao>VW GOL 1.0 08/16</aplicacao>
    <preco>18,50</preco>
    <estoque>2</estoque>
  </produto>
</catalogo>`
	r := env.enviarArquivoOpcoes("/api/xml/importar", "x.xml", []byte(xml), `{"preco_xml_eh_custo":true}`)
	if r.status != 200 {
		t.Fatalf("importar XML: %d %s", r.status, r.corpo)
	}
	var aplic string
	if err := env.app.db.QueryRow(`SELECT application FROM parts WHERE code_norm=?`, normCode("XML-APP-1")).Scan(&aplic); err != nil {
		t.Fatalf("peca do XML nao encontrada: %v", err)
	}
	if normText(aplic) != normText("VW GOL 1.0 08/16") {
		t.Fatalf("aplicacao guardada = %q", aplic)
	}
	// e a busca do sistema acha a peca pelo carro
	itens, _ := env.get("/api/pecas?q=" + urlQuery("gol 08")).json(t)["itens"].([]any)
	if len(itens) != 1 {
		t.Fatalf("busca por aplicacao devolveu %d pecas (esperava 1)", len(itens))
	}
}

// TestBuscaRapidaJuntaCatalogoEEstoque: uma busca so, respondendo as duas
// perguntas do balcao: "eu tenho?" e "existe? qual o codigo?".
func TestBuscaRapidaJuntaCatalogoEEstoque(t *testing.T) {
	env := novaAppTeste(t)
	env.criarPeca("FIL-001", "FILTRO DE OLEO GOL", 1000, 2490, 3)
	catalogoSalvar(env, map[string]any{"codigo": "W712/91", "descricao": "FILTRO DE OLEO PALIO", "aplicacao": "FIAT PALIO 1.0 96/11"})

	itens, _ := env.get("/api/busca?q=filtro").json(t)["itens"].([]any)
	if len(itens) != 2 {
		t.Fatalf("busca rapida devolveu %d itens (esperava 2: 1 do estoque + 1 do catalogo)", len(itens))
	}
	var noEstoque, noCatalogo int
	for _, it := range itens {
		m := it.(map[string]any)
		switch m["origem"] {
		case "estoque":
			noEstoque++
		case "catalogo":
			noCatalogo++
			if m["tem_estoque"] != false {
				t.Errorf("item de catalogo sem estoque marcado como disponivel: %v", m)
			}
		default:
			t.Errorf("origem desconhecida: %v", m["origem"])
		}
	}
	if noEstoque != 1 || noCatalogo != 1 {
		t.Fatalf("estoque=%d catalogo=%d (esperava 1 e 1)", noEstoque, noCatalogo)
	}
}

// ---------- apoio ----------

func urlQuery(s string) string {
	s = strings.ReplaceAll(s, " ", "+")
	s = strings.ReplaceAll(s, "/", "%2F")
	return s
}

// TestCatalogoBuscaPorAnoNaFaixa: no catalogo a aplicacao vem em faixa
// ("08/16"). Quem digita "gol 2012" ou "gol 12" tem que achar a peca.
func TestCatalogoBuscaPorAnoNaFaixa(t *testing.T) {
	env := novaAppTeste(t)
	catalogoSalvar(env, map[string]any{
		"codigo": "AF-1020", "descricao": "FILTRO DE AR", "aplicacao": "VW GOL G5/G6 1.0 08/16",
	})
	catalogoSalvar(env, map[string]any{
		"codigo": "PA-5010", "descricao": "PASTILHA DE FREIO", "aplicacao": "VW GOL 1.0 13/20",
	})
	// 2012 e 2018 caem em faixas diferentes: 08/16 e 13/20
	for _, busca := range []string{"gol 2012", "gol 12", "gol 2018"} {
		itens, _ := env.get("/api/catalogo?q=" + urlQuery(busca)).json(t)["itens"].([]any)
		if len(itens) == 0 {
			t.Fatalf("busca %q nao achou nada", busca)
		}
		esperado := "AF-1020"
		if busca == "gol 2018" {
			esperado = "PA-5010"
		}
		if primeiro := itens[0].(map[string]any)["codigo"]; primeiro != esperado {
			t.Errorf("busca %q: primeiro item = %v, esperava %s (a ordem deve seguir o ano)", busca, primeiro, esperado)
		}
	}
	// e a busca de pecas tambem entende o ano digitado
	env.criarPeca("F-1", "FILTRO DE OLEO GOL", 1000, 2490, 1)
	if r := env.req("PUT", "/api/pecas/"+itoa(env.qint(`SELECT id FROM parts LIMIT 1`)), map[string]any{
		"codigo": "F-1", "descricao": "FILTRO DE OLEO GOL", "custo_centavos": 1000,
		"preco_centavos": 2490, "minimo": 1, "aplicacao": "VW GOL 1.0 08/16",
	}); r.status != 200 {
		t.Fatalf("gravando aplicacao na peca: %d %s", r.status, r.corpo)
	}
	pecas, _ := env.get("/api/pecas?q=" + urlQuery("gol 2012")).json(t)["itens"].([]any)
	if len(pecas) != 1 {
		t.Fatalf("busca de pecas por ano devolveu %d (esperava 1)", len(pecas))
	}
}

// TestCatalogoImportaCSVComAspas: planilha salva pelo Excel costuma vir com
// campo entre aspas contendo ";" - nao pode quebrar a coluna no meio.
func TestCatalogoImportaCSVComAspas(t *testing.T) {
	env := novaAppTeste(t)
	csv := "codigo;descricao;aplicacao;medida;marca;custo;equivalentes\n" +
		"W712/90;FILTRO DE OLEO;\"VW GOL 1.0/1.6 08/16; VOYAGE 09/16\";ROSCA 3/4-16;MANN;18,50;\"TECFIL PSL560; FRAM PH2870A\"\n"
	if r := env.enviarArquivo("/api/catalogo/importar", "arquivo", "lista.csv", []byte(csv)); r.status != 200 {
		t.Fatalf("importar: %d %s", r.status, r.corpo)
	}
	var aplic, medida string
	if err := env.app.db.QueryRow(`SELECT application,measure FROM catalog WHERE code_norm=?`, normCode("W712/90")).
		Scan(&aplic, &medida); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(aplic, "VOYAGE") || normText(medida) != "rosca 3 4 16" {
		t.Fatalf("colunas desalinhadas: aplicacao=%q medida=%q", aplic, medida)
	}
	// os dois equivalentes entraram separados
	if n := env.contar(t, "catalog_equivalents"); n != 2 {
		t.Fatalf("equivalentes importados = %d (esperava 2)", n)
	}
}

// TestCatalogoCriteriosTecnicos: os critérios técnicos (rosca, dentes, altura)
// entram na busca — é o que o balcão usa para cruzar a peça do fornecedor com
// outra marca, no mesmo espírito dos critérios do padrão TecDoc.
func TestCatalogoCriteriosTecnicos(t *testing.T) {
	env := novaAppTeste(t)
	catalogoSalvar(env, map[string]any{
		"codigo": "CF-3301", "descricao": "CORREIA DENTADA", "aplicacao": "FIAT PALIO 1.0 04/12",
		"medida": "120 DENTES X 20MM", "criterios": "120 DENTES; 20MM de largura; perfil HTD",
		"marca": "GATES",
	})
	for _, busca := range []string{"rosca", "120 dentes", "perfil htd", "20mm"} {
		itens, _ := env.get("/api/catalogo?q=" + urlQuery(busca)).json(t)["itens"].([]any)
		if busca == "rosca" {
			if len(itens) != 0 {
				t.Errorf("busca %q deveria devolver nada (nao ha rosca cadastrada)", busca)
			}
			continue
		}
		if len(itens) != 1 {
			t.Errorf("busca %q devolveu %d itens (esperava 1)", busca, len(itens))
			continue
		}
		if got := itens[0].(map[string]any)["criterios"]; got != "120 DENTES; 20MM de largura; perfil HTD" {
			t.Errorf("criterios devolvidos = %v", got)
		}
	}
	// a ficha de falta leva os critérios (é o que o fornecedor precisa para separar a peça)
	id := env.qint(`SELECT id FROM catalog WHERE code_norm=?`, normCode("CF-3301"))
	if r := env.post("/api/catalogo/"+itoa(id)+"/ficha", map[string]any{"quantidade": 1}); r.status != 200 {
		t.Fatalf("ficha pelo catalogo: %d %s", r.status, r.corpo)
	}
	notas := env.qstr(`SELECT notes FROM faltas ORDER BY id DESC LIMIT 1`)
	if !strings.Contains(strings.ToUpper(notas), "HTD") {
		t.Errorf("nota da ficha deveria citar os criterios tecnicos: %q", notas)
	}
}

// TestCatalogoImportaCSVComCriterios: a planilha do fornecedor pode trazer uma
// coluna de critérios técnicos com qualquer um dos nomes usuais.
func TestCatalogoImportaCSVComCriterios(t *testing.T) {
	env := novaAppTeste(t)
	csv := "codigo;descricao;aplicacao;medida;marca;custo;equivalentes;criterios tecnicos\n" +
		"W712/90;FILTRO DE OLEO;VW GOL 1.0 08/16;ROSCA 3/4-16;MANN;18,50;TECFIL PSL560;rosca 3/4-16; altura 78mm; valvula anti-retorno\n"
	r := env.enviarArquivo("/api/catalogo/importar", "arquivo", "lista.csv", []byte(csv))
	if r.status != 200 || int(r.json(t)["importados"].(float64)) != 1 {
		t.Fatalf("importar com criterios: %d %s", r.status, r.corpo)
	}
	var criterios string
	if err := env.app.db.QueryRow(`SELECT IFNULL(criteria,'') FROM catalog WHERE code_norm=?`, normCode("W712/90")).
		Scan(&criterios); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToUpper(criterios), "ALTURA 78MM") {
		t.Fatalf("criterios importados = %q", criterios)
	}
	// e a busca acha pelos critérios
	itens, _ := env.get("/api/catalogo?q=" + urlQuery("altura 78mm")).json(t)["itens"].([]any)
	if len(itens) != 1 {
		t.Fatalf("busca por criterio tecnico devolveu %d itens", len(itens))
	}
}

// TestCatalogoBuscaUsaIndiceEAcertaPedacoDeCodigo: o índice de busca (FTS5 com
// trigrama) precisa achar tanto a palavra inteira quanto um pedaço no meio do
// código — é o que o balcão faz quando só lembra de parte do número.
func TestCatalogoBuscaUsaIndiceEAcertaPedacoDeCodigo(t *testing.T) {
	env := novaAppTeste(t)
	catalogoSalvar(env, map[string]any{
		"codigo": "W712/90", "descricao": "FILTRO DE OLEO", "aplicacao": "VW GOL 1.0 08/16",
		"criterios": "rosca 3/4-16; altura 78mm",
	})
	catalogoSalvar(env, map[string]any{
		"codigo": "PA-5010", "descricao": "PASTILHA DE FREIO", "aplicacao": "VW GOL 1.0 13/20",
	})
	casos := map[string]string{
		"712":      "W712/90", // pedaço no meio do código
		"w71290":   "W712/90", // código digitado sem barra
		"78mm":     "W712/90", // critério técnico
		"pastilha": "PA-5010",
		"gol":      "", // os dois
	}
	for busca, esperado := range casos {
		itens, _ := env.get("/api/catalogo?q=" + urlQuery(busca)).json(t)["itens"].([]any)
		if len(itens) == 0 {
			t.Errorf("busca %q não achou nada", busca)
			continue
		}
		if esperado == "" {
			if len(itens) != 2 {
				t.Errorf("busca %q devolveu %d (esperava 2)", busca, len(itens))
			}
			continue
		}
		achou := false
		for _, it := range itens {
			if it.(map[string]any)["codigo"] == esperado {
				achou = true
			}
		}
		if !achou {
			t.Errorf("busca %q não achou %s", busca, esperado)
		}
	}
	// termo curto (2 letras) continua funcionando pelo caminho antigo
	if itens, _ := env.get("/api/catalogo?q=" + urlQuery("pa")).json(t)["itens"].([]any); len(itens) == 0 {
		t.Error("busca de 2 letras não devolveu nada")
	}
	// e o índice acompanha alteração e exclusão
	if r := env.post("/api/catalogo", map[string]any{"codigo": "W712/90", "descricao": "FILTRO DE OLEO GRANDE", "aplicacao": "VW GOL"}); r.status != 200 {
		t.Fatalf("atualizando: %d %s", r.status, r.corpo)
	}
	itens, _ := env.get("/api/catalogo?q=grande").json(t)["itens"].([]any)
	if len(itens) != 1 {
		t.Errorf("depois de editar, a busca por 'grande' devolveu %d itens", len(itens))
	}
	id := env.qint(`SELECT id FROM catalog WHERE code_norm=?`, normCode("PA-5010"))
	if r := env.req("DELETE", "/api/catalogo/"+itoa(id), nil); r.status != 200 {
		t.Fatalf("excluindo: %d %s", r.status, r.corpo)
	}
	if itens, _ := env.get("/api/catalogo?q=pastilha").json(t)["itens"].([]any); len(itens) != 0 {
		t.Errorf("peça excluída ainda aparece na busca (%d itens)", len(itens))
	}
}

// TestCatalogoImportaListaGrande: a lista inteira do fornecedor entra em uma
// transação só e informa quanto tempo levou (o balcão acompanha o progresso).
func TestCatalogoImportaListaGrande(t *testing.T) {
	if testing.Short() {
		t.Skip("lista grande demora alguns segundos")
	}
	env := novaAppTeste(t)
	const n = 2000
	var b strings.Builder
	b.WriteString("codigo;descricao;aplicacao;medida;marca;custo;equivalentes;criterios\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "IMP-%05d;PECA DE TESTE %d;VW GOL 1.0 %02d/16;10X20;MANN;1,50;EQ-%05d;rosca 3/4-16\n",
			i, i, (i%20)+1, i)
	}
	r := env.enviarArquivo("/api/catalogo/importar", "arquivo", "grande.csv", []byte(b.String()))
	if r.status != 200 {
		t.Fatalf("importar lista grande: %d %s", r.status, r.corpo)
	}
	m := r.json(t)
	if int(m["importados"].(float64)) != n {
		t.Fatalf("importados = %v (esperava %d)", m["importados"], n)
	}
	if m["total_no_catalogo"].(float64) != n {
		t.Fatalf("total_no_catalogo = %v", m["total_no_catalogo"])
	}
	if _, ok := m["por_segundo"]; !ok {
		t.Error("a resposta deveria informar a velocidade (por_segundo)")
	}
	// segunda importação do mesmo arquivo: tudo atualizado, nada duplicado
	r2 := env.enviarArquivo("/api/catalogo/importar", "arquivo", "grande.csv", []byte(b.String()))
	m2 := r2.json(t)
	if int(m2["atualizados"].(float64)) != n || int(m2["importados"].(float64)) != 0 {
		t.Fatalf("segunda importação: importados=%v atualizados=%v", m2["importados"], m2["atualizados"])
	}
	if got := env.qint(`SELECT COUNT(*) FROM catalog`); got != n {
		t.Fatalf("catálogo com %d linhas (esperava %d)", got, n)
	}
	// a busca continua encontrando no meio da lista grande
	if itens, _ := env.get("/api/catalogo?q=IMP-01999").json(t)["itens"].([]any); len(itens) != 1 {
		t.Fatalf("busca no meio da lista grande devolveu %d itens", len(itens))
	}
}

// TestCatalogoNasceDoQueALojaJaTem: para quem não tem planilha nenhuma, o
// catálogo se monta com o que já está dentro do sistema (peças + fichas).
func TestCatalogoNasceDoQueALojaJaTem(t *testing.T) {
	env := novaAppTeste(t)
	// duas peças no estoque, uma com aplicação e equivalente
	peca := env.criarPeca("FIL-001", "FILTRO DE OLEO GOL", 1000, 2490, 3)
	if r := env.req("PUT", "/api/pecas/"+itoa(peca), map[string]any{
		"codigo": "FIL-001", "descricao": "FILTRO DE OLEO GOL", "aplicacao": "VW GOL 1.0 08/16",
		"custo_centavos": 1000, "preco_centavos": 2490, "minimo": 1,
		"equivalentes": []string{"W712/90"},
	}); r.status != 200 {
		t.Fatalf("gravando peça: %d %s", r.status, r.corpo)
	}
	env.criarPeca("PA-777", "PASTILHA DE FREIO", 5000, 9900, 1)
	// uma ficha de falta antiga com código entre colchetes (formato do catálogo)
	if r := env.post("/api/fichas", map[string]any{
		"descricao":   "[CF-3301] CORREIA DENTADA 120 DENTES",
		"observacoes": "FIAT PALIO 04/12", "custo_centavos": 5800,
	}); r.status != 200 {
		t.Fatalf("criando ficha: %d %s", r.status, r.corpo)
	}

	r := env.post("/api/catalogo/semear", map[string]any{"pecas": true, "fichas": true})
	if r.status != 200 {
		t.Fatalf("semear: %d %s", r.status, r.corpo)
	}
	m := r.json(t)
	if int(m["novos"].(float64)) != 3 {
		t.Fatalf("esperava 3 itens novos no catálogo: %s", r.corpo)
	}
	// a peça virou item de catálogo COM aplicação e equivalente
	var aplic string
	if err := env.app.db.QueryRow(`SELECT application FROM catalog WHERE code_norm=?`, normCode("FIL-001")).Scan(&aplic); err != nil {
		t.Fatal(err)
	}
	if normText(aplic) != normText("VW GOL 1.0 08/16") {
		t.Errorf("aplicação semeada = %q", aplic)
	}
	// e a busca por aplicação já acha o que veio do estoque
	itens, _ := env.get("/api/catalogo?q=" + urlQuery("gol 2012")).json(t)["itens"].([]any)
	if len(itens) != 1 || itens[0].(map[string]any)["codigo"] != "FIL-001" {
		t.Fatalf("busca por carro depois de semear: %v", itens)
	}
	if itens[0].(map[string]any)["tem_estoque"] != true {
		t.Error("o item semeado do estoque deveria aparecer como 'tem no estoque'")
	}
	// a ficha veio com código e aplicação
	var desc string
	if err := env.app.db.QueryRow(`SELECT description FROM catalog WHERE code_norm=?`, normCode("CF-3301")).Scan(&desc); err != nil {
		t.Fatalf("ficha não virou item de catálogo: %v", err)
	}
	if !strings.Contains(strings.ToUpper(desc), "CORREIA") {
		t.Errorf("descrição vinda da ficha = %q", desc)
	}
	// rodar de novo não duplica (só atualiza)
	r2 := env.post("/api/catalogo/semear", map[string]any{"pecas": true, "fichas": true})
	m2 := r2.json(t)
	if int(m2["novos"].(float64)) != 0 || int(m2["atualizados"].(float64)) != 3 {
		t.Fatalf("segunda semeadura: novos=%v atualizados=%v", m2["novos"], m2["atualizados"])
	}
	if n := env.contar(t, "catalog"); n != 3 {
		t.Fatalf("catálogo com %d itens (esperava 3)", n)
	}
	// semear não mexe no estoque
	if n := env.contar(t, "stock_moves"); n != 2 { // só as duas entradas iniciais das peças
		t.Errorf("semear gerou movimento de estoque a mais (%d movimentos)", n)
	}
}

// TestSepararCodigoDaFicha cobre os formatos de texto que aparecem nas fichas.
func TestSepararCodigoDaFicha(t *testing.T) {
	casos := []struct {
		entrada       string
		codigo, texto string
		ok            bool
	}{
		{"[PA-5010] PASTILHA DE FREIO", "PA-5010", "PASTILHA DE FREIO", true},
		{"[ W712/90 ] FILTRO", "W712/90", "FILTRO", true},
		{"PASTILHA SEM CODIGO", "", "", false},
		{"[] vazio", "", "", false},
		{"[", "", "", false},
	}
	for _, c := range casos {
		codigo, texto, ok := separarCodigoDaFicha(c.entrada)
		if ok != c.ok || codigo != c.codigo || texto != c.texto {
			t.Errorf("separarCodigoDaFicha(%q) = (%q, %q, %v); esperava (%q, %q, %v)",
				c.entrada, codigo, texto, ok, c.codigo, c.texto, c.ok)
		}
	}
}

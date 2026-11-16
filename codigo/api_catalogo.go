package main

// Catalogo do fornecedor.
//
// O catalogo guarda a peca que a loja NAO tem no estoque: codigo, descricao,
// para quais veiculos serve (aplicacao), medida e codigos equivalentes de outras
// marcas. Serve para responder no balcao o que antes exigia folhear catalogo de
// papel:
//
//   - "tem para o Gol 2012?"      -> busca por aplicacao
//   - "qual o codigo dessa peca?" -> busca por descricao
//   - "a Mann W712/90 e a mesma?" -> busca por codigo equivalente
//   - "eu tenho?"                 -> cruza com o estoque e diz quantas unidades
//
// O catalogo NAO cria peca nem estoque: quem manda no estoque continua sendo a
// importacao de XML, a nota por foto e o cadastro de peca.

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type CatalogoItem struct {
	ID           int64    `json:"id"`
	Codigo       string   `json:"codigo"`
	Descricao    string   `json:"descricao"`
	Aplicacao    string   `json:"aplicacao"`
	Medida       string   `json:"medida"`
	Criterios    string   `json:"criterios"`
	Marca        string   `json:"marca"`
	Categoria    string   `json:"categoria"`
	FornecedorID *int64   `json:"fornecedor_id"`
	Custo        int64    `json:"custo_centavos"`
	Observacao   string   `json:"observacoes"`
	Equivalentes []string `json:"equivalentes"`
	// preenchidos na busca: relacao com o estoque
	TemEstoque bool    `json:"tem_estoque"`
	Estoque    float64 `json:"estoque"`
	PecaID     *int64  `json:"peca_id"`
	PecaCodigo string  `json:"peca_codigo"`
	PecaPreco  int64   `json:"peca_preco_centavos"`
}

// blobDoCatalogo monta o texto de busca do item de catalogo.
func blobDoCatalogo(codigo, descricao, aplicacao, medida, criterios, marca, categoria string, equivs []string) string {
	// criterios tecnicos entram no mesmo texto de busca da medida: quem digita
	// "rosca 3/4" ou "120 dentes" acha a peca sem saber o codigo
	return buildSearchBlobApp(codigo, "", descricao, marca, categoria, "", aplicacao+" "+medida+" "+criterios, equivs)
}

// normalizarCodigoCatalogo deixa "W712/90", "w712-90" e "W 712/90" iguais.
func normalizarCodigoCatalogo(c string) string { return normCode(c) }

func (a *App) salvarCatalogo(it CatalogoItem) (int64, error) {
	var id int64
	err := a.db.Tx(func(tx *sql.Tx) error {
		var err error
		id, err = salvarCatalogoTx(it, tx)
		return err
	})
	return id, err
}

// salvarCatalogoTx e o mesmo cadastro, usado dentro da transacao de quem chama
// (importacao de lista grande: 1 transacao para o arquivo inteiro).
func salvarCatalogoTx(it CatalogoItem, tx *sql.Tx) (int64, error) {
	var id int64
	err := salvarCatalogoSQL(it, tx, &id)
	return id, err
}

// salvarCatalogoSQL faz o trabalho e devolve o id (novo ou existente) por ponteiro.
func salvarCatalogoSQL(it CatalogoItem, tx *sql.Tx, idOut *int64) error {
	codigo := strings.TrimSpace(it.Codigo)
	norm := normalizarCodigoCatalogo(codigo)
	if norm == "" {
		return fmt.Errorf("informe o código da peça no catálogo")
	}
	if strings.TrimSpace(it.Descricao) == "" && strings.TrimSpace(it.Aplicacao) == "" {
		return fmt.Errorf("informe a descrição ou a aplicação (para qual veículo serve)")
	}
	equivs := limparEquivalentes(it.Equivalentes, codigo)
	blob := blobDoCatalogo(codigo, it.Descricao, it.Aplicacao, it.Medida, it.Criterios, it.Marca, it.Categoria, equivs)

	var id int64
	var existente int64
	err := tx.QueryRow(`SELECT id FROM catalog WHERE code_norm=?`, norm).Scan(&existente)
	switch {
	case err == nil && existente > 0:
		if _, err := tx.Exec(`UPDATE catalog SET code=?,description=?,application=?,measure=?,criteria=?,brand=?,category=?,
			supplier_id=?,cost_cents=?,notes=?,search_blob=?,updated_at=? WHERE id=?`,
			codigo, it.Descricao, it.Aplicacao, it.Medida, it.Criterios, it.Marca, it.Categoria,
			it.FornecedorID, it.Custo, it.Observacao, blob, nowISO(), existente); err != nil {
			return err
		}
		id = existente
	case err == sql.ErrNoRows:
		res, err := tx.Exec(`INSERT INTO catalog(code,code_norm,description,application,measure,criteria,brand,category,
			supplier_id,cost_cents,notes,search_blob,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			codigo, norm, it.Descricao, it.Aplicacao, it.Medida, it.Criterios, it.Marca, it.Categoria,
			it.FornecedorID, it.Custo, it.Observacao, blob, nowISO(), nowISO())
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
	default:
		return err
	}
	if _, err := tx.Exec(`DELETE FROM catalog_equivalents WHERE catalog_id=?`, id); err != nil {
		return err
	}
	for _, e := range equivs {
		if _, err := tx.Exec(`INSERT INTO catalog_equivalents(catalog_id,code,code_norm) VALUES(?,?,?)`,
			id, e, normCode(e)); err != nil {
			return err
		}
	}
	*idOut = id
	return nil
}

func limparEquivalentes(lista []string, codigoProprio string) []string {
	vistos := map[string]bool{normCode(codigoProprio): true}
	out := []string{}
	for _, bruto := range lista {
		for _, parte := range strings.FieldsFunc(bruto, func(r rune) bool { return r == ',' || r == ';' || r == '|' }) {
			e := strings.TrimSpace(parte)
			n := normCode(e)
			if n == "" || vistos[n] {
				continue
			}
			vistos[n] = true
			out = append(out, e)
		}
	}
	return out
}

// anosDaBusca separa o que o balcao digita: "gol 2012" tem uma palavra forte
// ("gol") e um ano. O ano nao pode eliminar o resultado - a aplicacao no
// catalogo costuma vir como faixa ("08/16") -, entao ele so serve para ordenar.
func separarAnos(termos []string) (fortes, anos []string) {
	for _, t := range termos {
		if ehAno(t) {
			anos = append(anos, t)
			continue
		}
		fortes = append(fortes, t)
	}
	return fortes, anos
}

// ehAno reconhece "2012", "12" e "08" (o balcao digita tanto 2012 quanto 12).
func ehAno(t string) bool {
	if len(t) == 4 {
		return strings.HasPrefix(t, "19") || strings.HasPrefix(t, "20")
	}
	return len(t) == 2 && t[0] >= '0' && t[0] <= '9' && t[1] >= '0' && t[1] <= '9'
}

// anosDaAplicacao devolve os anos e as faixas de ano de uma aplicacao:
// "VW GOL 1.0/1.6 08/16" -> anos {1,0,1,6,8,16} e faixa {0,8}-{1,6}. Serve
// para responder "tem para o Gol 2012?" quando a aplicacao diz "08/16".
func anosDaAplicacao(aplicacao string) []int {
	out := []int{}
	atual := ""
	empurra := func() {
		if atual != "" && len(atual) <= 4 {
			if n, err := strconv.Atoi(atual); err == nil {
				out = append(out, n)
			}
		}
		atual = ""
	}
	for _, r := range aplicacao {
		if r >= '0' && r <= '9' {
			atual += string(r)
			continue
		}
		empurra()
	}
	empurra()
	return out
}

// contaAcertosAno diz quantos dos anos buscados a aplicacao cobre.
func contaAcertosAno(aplicacao string, anos []string) int {
	semAspas := normText(aplicacao)
	numeros := anosDaAplicacao(semAspas)
	if len(numeros) == 0 {
		return 0
	}
	acertos := 0
	for _, bruto := range anos {
		y, err := strconv.Atoi(bruto)
		if err != nil {
			continue
		}
		curto := y % 100 // 2012 -> 12
		casou := false
		for _, n := range numeros {
			if n == y || n == curto || (len(bruto) == 2 && n%100 == y) {
				casou = true
				break
			}
		}
		// faixa de anos: 08/16 atende 2012, 13/20 atende 2015
		if !casou {
			for i := 0; i+1 < len(numeros); i++ {
				ini, fim := numeros[i]%100, numeros[i+1]%100
				if ini <= curto && curto <= fim && fim-ini <= 40 {
					casou = true
					break
				}
			}
		}
		if casou {
			acertos++
		}
	}
	return acertos
}

// buscarCatalogo devolve os itens que casam com o texto, ja com a situacao no estoque.
// A busca e tolerante com o ano: tenta primeiro com todas as palavras e, se nao
// achar nada, repete sem os anos e ordena pelo que mais combina com o ano dito
// (a aplicacao do catalogo costuma vir em faixa: "08/16").
func (a *App) buscarCatalogo(q string, limite int) ([]CatalogoItem, error) {
	if limite <= 0 || limite > 500 {
		limite = 200
	}
	termos := strings.Fields(normText(q))
	fortes, anos := separarAnos(termos)
	tentativas := [][]string{termos}
	if len(anos) > 0 && len(fortes) > 0 {
		tentativas = append(tentativas, fortes)
	}

	itens := []CatalogoItem{}
	for _, conjunto := range tentativas {
		consulta := `SELECT c.id,c.code,c.description,c.application,c.measure,IFNULL(c.criteria,''),c.brand,c.category,
			c.supplier_id,c.cost_cents,c.notes FROM catalog c`
		var args []any
		where := []string{}
		var noIndice, curtos []string
		for _, t := range conjunto {
			if len([]rune(t)) >= 3 { // o trigrama precisa de 3 letras
				noIndice = append(noIndice, t)
			} else {
				curtos = append(curtos, t)
			}
		}
		if len(noIndice) > 0 {
			// busca pelo indice FTS5: AND de todos os termos
			where = append(where, "c.id IN (SELECT rowid FROM catalog_fts WHERE catalog_fts MATCH ?)")
			expr := make([]string, len(noIndice))
			for i, t := range noIndice {
				expr[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
			}
			args = append(args, strings.Join(expr, " AND "))
		}
		for _, t := range curtos {
			where = append(where, "c.search_blob LIKE ?")
			args = append(args, "%"+t+"%")
		}
		if len(where) > 0 {
			consulta += " WHERE " + strings.Join(where, " AND ")
		}
		consulta += " ORDER BY c.code LIMIT ?"
		args = append(args, limite)

		rows, err := a.db.Query(consulta, args...)
		if err != nil {
			return nil, err
		}
		itens = []CatalogoItem{}
		for rows.Next() {
			var it CatalogoItem
			if err := rows.Scan(&it.ID, &it.Codigo, &it.Descricao, &it.Aplicacao, &it.Medida, &it.Criterios,
				&it.Marca, &it.Categoria, &it.FornecedorID, &it.Custo, &it.Observacao); err != nil {
				rows.Close()
				return nil, err
			}
			itens = append(itens, it)
		}
		rows.Close()
		if len(itens) > 0 {
			break
		}
	}

	acertosAno := map[int64]int{}
	for i := range itens {
		itens[i].Equivalentes = a.equivalentesDoCatalogo(itens[i].ID)
		a.marcarSituacaoEstoque(&itens[i])
		acertosAno[itens[i].ID] = contaAcertosAno(itens[i].Aplicacao, anos)
	}
	// o que a loja TEM primeiro; depois o que casa melhor com o ano; depois o codigo
	sort.SliceStable(itens, func(i, j int) bool {
		if itens[i].TemEstoque != itens[j].TemEstoque {
			return itens[i].TemEstoque
		}
		if acertosAno[itens[i].ID] != acertosAno[itens[j].ID] {
			return acertosAno[itens[i].ID] > acertosAno[itens[j].ID]
		}
		return itens[i].Codigo < itens[j].Codigo
	})
	return itens, nil
}

func (a *App) equivalentesDoCatalogo(id int64) []string {
	out := []string{}
	rows, err := a.db.Query(`SELECT code FROM catalog_equivalents WHERE catalog_id=? ORDER BY id`, id)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil {
			out = append(out, c)
		}
	}
	return out
}

// marcarSituacaoEstoque diz se a peca do catalogo existe no estoque - pelo
// codigo do catalogo, pelo codigo de fabricante ou por um equivalente.
func (a *App) marcarSituacaoEstoque(it *CatalogoItem) {
	candidatos := append([]string{it.Codigo}, it.Equivalentes...)
	for _, c := range candidatos {
		n := normCode(c)
		if n == "" {
			continue
		}
		var id int64
		err := a.db.QueryRow(`SELECT id FROM parts WHERE active=1 AND (code_norm=? OR mfr_norm=?) LIMIT 1`, n, n).Scan(&id)
		if err != nil {
			err = a.db.QueryRow(`SELECT part_id FROM equivalents WHERE code_norm=? LIMIT 1`, n).Scan(&id)
		}
		if err != nil || id == 0 {
			continue
		}
		var estoque float64
		var codigo string
		var preco int64
		if a.db.QueryRow(`SELECT stock,code,price_cents FROM parts WHERE id=?`, id).Scan(&estoque, &codigo, &preco) == nil {
			pegaID := id
			it.TemEstoque = true
			it.Estoque = estoque
			it.PecaID = &pegaID
			it.PecaCodigo = codigo
			it.PecaPreco = preco
			return
		}
	}
}

// handleCatalogo lista/busca (GET) ou cadastra/atualiza (POST).
func (a *App) handleCatalogo(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		limite := 200
		if l := r.URL.Query().Get("limite"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				limite = n
			}
		}
		itens, err := a.buscarCatalogo(q, limite)
		if err != nil {
			fail(w, 500, "não consegui consultar o catálogo: "+err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"itens": itens, "total": len(itens), "busca": q})
		return
	}
	var in CatalogoItem
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados inválidos")
		return
	}
	id, err := a.salvarCatalogo(in)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "id": id})
}

// handleCatalogoItem atende PUT e DELETE em /api/catalogo/{id}.
func (a *App) handleCatalogoItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "item de catálogo inválido")
		return
	}
	if r.Method == http.MethodDelete {
		if err := a.db.Tx(func(tx *sql.Tx) error {
			if _, err := tx.Exec(`DELETE FROM catalog_equivalents WHERE catalog_id=?`, id); err != nil {
				return err
			}
			_, err := tx.Exec(`DELETE FROM catalog WHERE id=?`, id)
			return err
		}); err != nil {
			fail(w, 500, "não consegui excluir: "+err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	var in CatalogoItem
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados inválidos")
		return
	}
	novoID, err := a.salvarCatalogo(in)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "id": novoID})
}

// handleCatalogoImportar aceita CSV/TXT com ; ou , como separador. Reconhece o
// cabecalho (codigo, descricao, aplicacao, medida, marca, custo, equivalentes,
// criterios) e tambem arquivos sem cabecalho, na ordem padrao.
//
// O arquivo e lido em FLUXO (streaming): a lista inteira de um fornecedor
// (centenas de milhares de linhas, 100 MB+) entra sem carregar tudo na memoria.
// A gravacao vai em blocos de 10 mil linhas, dentro de transacao.
func (a *App) handleCatalogoImportar(w http.ResponseWriter, r *http.Request) {
	leitor, nome, err := abrirUploadEmFluxo(r)
	if err != nil {
		fail(w, 400, "não consegui ler o arquivo: "+err.Error())
		return
	}
	defer leitor.Close()

	comecou := time.Now()
	importados, atualizados, ignorados, lidas := 0, 0, 0, 0
	var problemas []string
	existentes := map[string]bool{}
	if rows, err := a.db.Query(`SELECT code_norm FROM catalog`); err == nil {
		for rows.Next() {
			var c string
			if rows.Scan(&c) == nil {
				existentes[c] = true
			}
		}
		rows.Close()
	}

	scanner := bufio.NewScanner(leitor)
	scanner.Buffer(make([]byte, 0, 1<<20), 4<<20) // linha de ate 4 MB
	colunas := map[string]int{}
	cabecalho := false
	primeira := true

	// grava em blocos: transacao curta, memoria baixa e progresso aproveitável
	const bloco = 10000
	var pendentes []CatalogoItem
	var normas []string
	gravar := func() error {
		if len(pendentes) == 0 {
			return nil
		}
		itens, ns := pendentes, normas
		pendentes, normas = nil, nil
		return a.db.Tx(func(tx *sql.Tx) error {
			for i, it := range itens {
				if _, err := salvarCatalogoTx(it, tx); err != nil {
					ignorados++
					if len(problemas) < 20 {
						problemas = append(problemas, fmt.Sprintf("%s: %v", it.Codigo, err))
					}
					continue
				}
				if existentes[ns[i]] {
					atualizados++
				} else {
					importados++
					existentes[ns[i]] = true
				}
			}
			return nil
		})
	}

	for scanner.Scan() {
		linha := strings.TrimSpace(scanner.Text())
		if linha == "" {
			continue
		}
		if primeira {
			primeira = false
			if idx := indexarCabecalho(linha); idx != nil {
				colunas = idx
				cabecalho = true
				continue
			}
		}
		lidas++
		campos := camposCSV(linha)
		if len(campos) == 0 || strings.TrimSpace(strings.Join(campos, "")) == "" {
			continue
		}
		// criterios tecnicos e aplicacao costumam vir com ";" dentro, sem aspas
		campos = juntarCamposExtras(campos, colunas, cabecalho)
		pegar := func(nome string, padrao int) string {
			if pos, ok := colunas[nome]; ok && pos < len(campos) {
				return campos[pos]
			}
			if !cabecalho && padrao < len(campos) {
				return campos[padrao]
			}
			return ""
		}
		it := CatalogoItem{
			Codigo:       pegar("codigo", 0),
			Descricao:    pegar("descricao", 1),
			Aplicacao:    pegar("aplicacao", 2),
			Medida:       pegar("medida", 3),
			Criterios:    pegar("criterios", 8),
			Marca:        pegar("marca", 4),
			Categoria:    pegar("categoria", 6),
			Custo:        parseMoneyBR(pegar("custo", 5)),
			Observacao:   pegar("observacoes", 9),
			Equivalentes: []string{pegar("equivalentes", 7)},
		}
		norm := normalizarCodigoCatalogo(it.Codigo)
		if norm == "" || (strings.TrimSpace(it.Descricao) == "" && strings.TrimSpace(it.Aplicacao) == "") {
			// linha sem codigo ou sem descricao/aplicacao: conta como ignorada e segue
			ignorados++
			if len(problemas) < 20 {
				problemas = append(problemas, fmt.Sprintf("linha %d: sem código ou sem descrição/aplicação", lidas+1))
			}
			continue
		}
		pendentes = append(pendentes, it)
		normas = append(normas, norm)
		if len(pendentes) >= bloco {
			if err := gravar(); err != nil {
				fail(w, 500, "não consegui gravar a lista: "+err.Error())
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fail(w, 400, "erro ao ler o arquivo: "+err.Error())
		return
	}
	if err := gravar(); err != nil {
		fail(w, 500, "não consegui gravar a lista: "+err.Error())
		return
	}
	if lidas == 0 {
		fail(w, 400, "o arquivo está vazio")
		return
	}

	segundos := time.Since(comecou).Seconds()
	writeJSON(w, 200, map[string]any{
		"arquivo": nome, "importados": importados, "atualizados": atualizados,
		"ignorados": ignorados, "problemas": problemas,
		"segundos":          math.Round(segundos*10) / 10,
		"por_segundo":       int(float64(importados+atualizados) / math.Max(segundos, 0.001)),
		"total_no_catalogo": a.contarCatalogo(),
	})
}

// abrirUploadEmFluxo devolve o arquivo do formulário sem carregá-lo na memória.
func abrirUploadEmFluxo(r *http.Request) (io.ReadCloser, string, error) {
	leitor, err := r.MultipartReader()
	if err != nil {
		return nil, "", err
	}
	for {
		parte, err := leitor.NextPart()
		if err == io.EOF {
			return nil, "", fmt.Errorf("nenhum arquivo enviado")
		}
		if err != nil {
			return nil, "", err
		}
		if parte.FormName() == "arquivo" && parte.FileName() != "" {
			return parte, parte.FileName(), nil
		}
		parte.Close()
	}
}

// contarCatalogo devolve quantas pecas existem no catalogo agora.
func (a *App) contarCatalogo() int64 {
	var n int64
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM catalog`).Scan(&n)
	return n
}

// juntarCamposExtras devolve a linha do jeito que o cabecalho espera: quando a
// ultima coluna mapeada e texto livre (criterios, aplicacao, observacoes), os
// campos que sobraram sao reunidos nela de novo, com ";".
func juntarCamposExtras(campos []string, colunas map[string]int, cabecalho bool) []string {
	if !cabecalho || len(colunas) == 0 {
		return campos
	}
	ultimaPos, ultimoNome := -1, ""
	for nome, pos := range colunas {
		if pos > ultimaPos {
			ultimaPos, ultimoNome = pos, nome
		}
	}
	switch ultimoNome {
	case "criterios", "aplicacao", "observacoes":
	default:
		return campos
	}
	if ultimaPos >= len(campos)-1 || ultimaPos < 0 {
		return campos // nao sobrou nada para juntar
	}
	junta := strings.Join(campos[ultimaPos:], "; ")
	saida := append([]string{}, campos[:ultimaPos]...)
	return append(saida, junta)
}

// camposCSV separa uma linha de planilha respeitando aspas: "GOL 1.0; VOYAGE"
// e um campo so. Aceita ; , ou tabulacao como separador (o mais frequente vence).
func camposCSV(linha string) []string {
	sep := detectarSeparador(linha)
	campos := []string{}
	var atual strings.Builder
	dentro := false
	for i := 0; i < len(linha); i++ {
		c := linha[i]
		if c == '"' {
			if dentro && i+1 < len(linha) && linha[i+1] == '"' { // aspas escapadas
				atual.WriteByte('"')
				i++
				continue
			}
			dentro = !dentro
			continue
		}
		if c == sep && !dentro {
			campos = append(campos, strings.TrimSpace(atual.String()))
			atual.Reset()
			continue
		}
		atual.WriteByte(c)
	}
	campos = append(campos, strings.TrimSpace(atual.String()))
	return campos
}

// detectarSeparador escolhe ; , ou tabulacao contando apenas o que esta fora
// das aspas (um campo entre aspas pode ter virgulas e ponto e virgula).
func detectarSeparador(linha string) byte {
	contagem := map[byte]int{';': 0, ',': 0, '\t': 0}
	dentro := false
	for i := 0; i < len(linha); i++ {
		c := linha[i]
		if c == '"' {
			dentro = !dentro
			continue
		}
		if dentro {
			continue
		}
		if _, ok := contagem[c]; ok {
			contagem[c]++
		}
	}
	melhor := byte(';')
	for _, cand := range []byte{',', '\t'} {
		if contagem[cand] > contagem[melhor] {
			melhor = cand
		}
	}
	return melhor
}

// indexarCabecalho descobre a posicao de cada coluna pelo titulo.
func indexarCabecalho(linha string) map[string]int {
	campos := camposCSV(linha)
	idx := map[string]int{}
	for i, bruto := range campos {
		c := normText(bruto)
		switch {
		case c == "":
			continue
		case strings.Contains(c, "codigo") || c == "cod" || strings.Contains(c, "referencia") || strings.Contains(c, "sku"):
			idx["codigo"] = i
		case strings.Contains(c, "descr") || strings.Contains(c, "produto") || strings.Contains(c, "nome"):
			idx["descricao"] = i
		case strings.Contains(c, "aplic") || strings.Contains(c, "veiculo") || strings.Contains(c, "carro") || strings.Contains(c, "modelo"):
			idx["aplicacao"] = i
		case strings.Contains(c, "medida") || strings.Contains(c, "dimens") || strings.Contains(c, "tamanho"):
			idx["medida"] = i
		case strings.Contains(c, "criterio") || strings.Contains(c, "especific") || strings.Contains(c, "atributo") ||
			strings.Contains(c, "tecnica") || strings.Contains(c, "tecnico") || strings.Contains(c, "rosca") ||
			strings.Contains(c, "dentes"):
			idx["criterios"] = i
		case strings.Contains(c, "marca") || strings.Contains(c, "fabricante"):
			idx["marca"] = i
		case strings.Contains(c, "custo") || strings.Contains(c, "preco") || strings.Contains(c, "valor"):
			idx["custo"] = i
		case strings.Contains(c, "categoria") || strings.Contains(c, "grupo") || strings.Contains(c, "familia"):
			idx["categoria"] = i
		case strings.Contains(c, "equival") || strings.Contains(c, "similar") || strings.Contains(c, "cruzad") || strings.Contains(c, "referencia cruzada") || strings.Contains(c, "codigos"):
			idx["equivalentes"] = i
		case strings.Contains(c, "observ") || strings.Contains(c, "obs"):
			idx["observacoes"] = i
		}
	}
	// cabecalho de verdade precisa ter codigo e (descricao ou aplicacao)
	if _, ok := idx["codigo"]; !ok {
		return nil
	}
	if _, ok := idx["descricao"]; !ok {
		if _, ok := idx["aplicacao"]; !ok {
			return nil
		}
	}
	return idx
}

// handleCatalogoFicha cria uma ficha de falta a partir de um item do catalogo.
// E o caminho do balcao: achou no catalogo, nao tem no estoque, registra o
// pedido do cliente antes que ele va embora.
func (a *App) handleCatalogoFicha(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "item de catálogo inválido")
		return
	}
	var it CatalogoItem
	err = a.db.QueryRow(`SELECT id,code,description,IFNULL(application,''),IFNULL(measure,''),IFNULL(criteria,''),brand,cost_cents,supplier_id FROM catalog WHERE id=?`, id).
		Scan(&it.ID, &it.Codigo, &it.Descricao, &it.Aplicacao, &it.Medida, &it.Criterios, &it.Marca, &it.Custo, &it.FornecedorID)
	if err == sql.ErrNoRows {
		fail(w, 404, "item de catálogo não encontrado")
		return
	}
	if err != nil {
		fail(w, 500, "não consegui ler o item do catálogo: "+err.Error())
		return
	}
	var in struct {
		Quantidade   float64 `json:"quantidade"`
		ClienteID    *int64  `json:"cliente_id"`
		ClienteNome  string  `json:"cliente_nome"`
		ClienteFone  string  `json:"cliente_telefone"`
		Veiculo      string  `json:"veiculo"`
		Observacoes  string  `json:"observacoes"`
		FornecedorID *int64  `json:"fornecedor_id"`
	}
	_ = readJSON(r, &in)
	if in.Quantidade <= 0 {
		in.Quantidade = 1
	}
	desc := strings.TrimSpace(it.Descricao)
	if desc == "" {
		desc = "peça do catálogo"
	}
	if it.Codigo != "" {
		desc = "[" + it.Codigo + "] " + desc
	}
	notas := strings.TrimSpace(it.Aplicacao)
	if it.Medida != "" {
		notas = strings.TrimSpace(notas + " " + it.Medida)
	}
	if it.Criterios != "" {
		notas = strings.TrimSpace(notas + " " + it.Criterios)
	}
	if it.Marca != "" {
		notas = strings.TrimSpace(notas + " (" + it.Marca + ")")
	}
	if in.Observacoes != "" {
		notas = strings.TrimSpace(notas + " - " + in.Observacoes)
	}
	nome := strings.TrimSpace(in.ClienteNome)
	if nome == "" && in.ClienteID != nil && *in.ClienteID > 0 {
		_ = a.db.QueryRow(`SELECT name FROM customers WHERE id=?`, *in.ClienteID).Scan(&nome)
	}
	fornecedor := it.FornecedorID
	if in.FornecedorID != nil && *in.FornecedorID > 0 {
		fornecedor = in.FornecedorID
	}
	// ficha SEM peca_id: e justamente a peca que a loja nao tem no estoque
	res, err := a.db.Exec(`INSERT INTO faltas(created_at,part_id,description,qty,customer_id,customer_name,customer_phone,vehicle,supplier_id,status,cost_cents,notes)
		VALUES(?,NULL,?,?,?,?,?,?,?,?,?,?)`,
		nowISO(), desc, in.Quantidade, in.ClienteID, nome, in.ClienteFone, in.Veiculo,
		fornecedor, "aberta", it.Custo, notas)
	if err != nil {
		fail(w, 500, "não consegui criar a ficha: "+err.Error())
		return
	}
	faltaID, _ := res.LastInsertId()
	writeJSON(w, 200, map[string]any{"ok": true, "ficha_id": faltaID, "descricao": desc})
}

// achadoBusca e uma linha da busca rapida, venha ela do estoque ou do catalogo.
type achadoBusca struct {
	Origem       string   `json:"origem"` // estoque | catalogo
	ID           int64    `json:"id"`
	Codigo       string   `json:"codigo"`
	Descricao    string   `json:"descricao"`
	Aplicacao    string   `json:"aplicacao"`
	Estoque      float64  `json:"estoque"`
	Preco        int64    `json:"preco_centavos"`
	TemEstoque   bool     `json:"tem_estoque"`
	Equivalentes []string `json:"equivalentes,omitempty"`
}

// handleBuscaRapida responde numa consulta so: "eu tenho no estoque?" e
// "existe no catalogo? qual o codigo?".
func (a *App) handleBuscaRapida(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	out := []achadoBusca{}
	if q != "" {
		// 1) o que a loja tem
		pecas, err := a.buscarPecas(q, 50)
		if err != nil {
			fail(w, 500, "não consegui buscar nas peças: "+err.Error())
			return
		}
		noEstoque := map[int64]bool{}
		for _, p := range pecas {
			noEstoque[p.ID] = true
			out = append(out, achadoBusca{Origem: "estoque", ID: p.ID, Codigo: p.Codigo,
				Descricao: p.Descricao, Aplicacao: p.Aplicacao, Estoque: p.Estoque,
				Preco: p.Preco, TemEstoque: true})
		}
		// 2) o que existe no catalogo (mesmo sem estoque)
		if catalogo, err := a.buscarCatalogo(q, 50); err == nil {
			for _, c := range catalogo {
				// nao repete o que ja veio do estoque
				if c.PecaID != nil && noEstoque[*c.PecaID] {
					continue
				}
				out = append(out, achadoBusca{Origem: "catalogo", ID: c.ID, Codigo: c.Codigo,
					Descricao: c.Descricao, Aplicacao: c.Aplicacao, Estoque: c.Estoque,
					Preco: c.PecaPreco, TemEstoque: c.TemEstoque, Equivalentes: c.Equivalentes})
			}
		}
	}
	writeJSON(w, 200, map[string]any{"itens": out, "total": len(out), "busca": q})
}

// buscarPecas faz a mesma busca da aba Pecas, resumida, para a busca rapida.
func (a *App) buscarPecas(q string, limite int) ([]Part, error) {
	if limite <= 0 || limite > 500 {
		limite = 50
	}
	termos := strings.Fields(normText(q))
	fortes, anos := separarAnos(termos)
	tentativas := [][]string{termos}
	if len(anos) > 0 && len(fortes) > 0 {
		tentativas = append(tentativas, fortes)
	}
	out := []Part{}
	for _, conjunto := range tentativas {
		var where []string
		var args []any
		where = append(where, "p.active=1")
		for _, t := range conjunto {
			where = append(where, "(p.search_blob LIKE ? OR p.code_norm LIKE ?)")
			args = append(args, "%"+t+"%", "%"+strings.ReplaceAll(t, " ", "")+"%")
		}
		sqlText := `SELECT ` + partCols + ` FROM parts p LEFT JOIN suppliers s ON s.id=p.supplier_id WHERE ` +
			strings.Join(where, " AND ") + ` ORDER BY p.description COLLATE NOCASE LIMIT ?`
		args = append(args, limite)
		rows, err := a.db.Query(sqlText, args...)
		if err != nil {
			return nil, err
		}
		out = []Part{}
		for rows.Next() {
			p, err := scanPart(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, p)
		}
		rows.Close()
		if len(out) > 0 {
			break
		}
	}
	// ordena pelo ano quando o ano foi digitado (o resto fica como veio)
	sort.SliceStable(out, func(i, j int) bool {
		return contaAcertosAno(out[i].Aplicacao, anos) > contaAcertosAno(out[j].Aplicacao, anos)
	})
	return out, nil
}

// handleCatalogoSemear monta o catálogo com o que a loja JÁ tem dentro do
// sistema, sem depender de lista do fornecedor:
//
//   - peças do estoque viram item de catálogo (com aplicação e equivalentes);
//   - fichas de falta antigas cujo texto começa com "[CODIGO]" (as que foram
//     geradas pelo próprio catálogo) voltam para o catálogo.
//
// É o começo honesto para quem não tem planilha nenhuma: em vez de digitar
// tudo de novo, o catálogo nasce do histórico da loja e cresce a cada ficha.
func (a *App) handleCatalogoSemear(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Pecas  bool `json:"pecas"`
		Fichas bool `json:"fichas"`
	}
	if err := readJSON(r, &in); err != nil {
		in.Pecas, in.Fichas = true, true // padrão: as duas fontes
	}
	if !in.Pecas && !in.Fichas {
		in.Pecas, in.Fichas = true, true
	}

	novos, atualizados, ignorados := 0, 0, 0
	var problemas []string

	// código que já está no catálogo (para não contar duas vezes)
	existentes := map[string]bool{}
	if rows, err := a.db.Query(`SELECT code_norm FROM catalog`); err == nil {
		for rows.Next() {
			var c string
			if rows.Scan(&c) == nil {
				existentes[c] = true
			}
		}
		rows.Close()
	}

	// grava em blocos de 500: uma transacao por bloco, nunca aninhada
	var lote []CatalogoItem
	var loteNorm []string
	gravarLote := func() {
		if len(lote) == 0 {
			return
		}
		itens, norms := lote, loteNorm
		lote, loteNorm = nil, nil
		_ = a.db.Tx(func(tx *sql.Tx) error {
			for i, it := range itens {
				var id int64
				if err := salvarCatalogoSQL(it, tx, &id); err != nil {
					ignorados++
					if len(problemas) < 20 {
						problemas = append(problemas, fmt.Sprintf("%s: %v", it.Codigo, err))
					}
					continue
				}
				if existentes[norms[i]] {
					atualizados++
				} else {
					novos++
					existentes[norms[i]] = true
				}
			}
			return nil
		})
	}
	gravar := func(it CatalogoItem) {
		norm := normalizarCodigoCatalogo(it.Codigo)
		if norm == "" {
			ignorados++
			return
		}
		lote = append(lote, it)
		loteNorm = append(loteNorm, norm)
		if len(lote) >= 500 {
			gravarLote()
		}
	}

	if in.Pecas {
		type pecaSemear struct {
			id                                                    int64
			codigo, codfab, desc, marca, categoria, aplic, medida string
			custo                                                 int64
		}
		pecas := []pecaSemear{}
		rows, err := a.db.Query(`SELECT id,code,IFNULL(mfr_code,''),description,brand,IFNULL(category,''),
			IFNULL(application,''),IFNULL(location,''),cost_cents FROM parts WHERE active=1 ORDER BY id`)
		if err == nil {
			for rows.Next() {
				var p pecaSemear
				if rows.Scan(&p.id, &p.codigo, &p.codfab, &p.desc, &p.marca, &p.categoria,
					&p.aplic, &p.medida, &p.custo) == nil {
					pecas = append(pecas, p)
				}
			}
			rows.Close()
		}
		for _, p := range pecas {
			var equivs []string
			if er, err := a.db.Query(`SELECT code FROM equivalents WHERE part_id=? ORDER BY id`, p.id); err == nil {
				for er.Next() {
					var c string
					if er.Scan(&c) == nil {
						equivs = append(equivs, c)
					}
				}
				er.Close()
			}
			gravar(CatalogoItem{
				Codigo: p.codigo, Descricao: p.desc, Aplicacao: p.aplic,
				Medida: p.medida, Marca: p.marca, Categoria: p.categoria,
				Custo: p.custo, Equivalentes: equivs,
				Observacao: "veio do cadastro de peças da loja",
			})
		}
	}

	if in.Fichas {
		// fichas com "[CODIGO] descricao" (as geradas pelo catálogo)
		rows, err := a.db.Query(`SELECT description, IFNULL(vehicle,''), IFNULL(notes,''), IFNULL(cost_cents,0)
			FROM faltas ORDER BY id`)
		if err == nil {
			for rows.Next() {
				var desc, veiculo, notas string
				var custo int64
				if rows.Scan(&desc, &veiculo, &notas, &custo) != nil {
					continue
				}
				codigo, texto, ok := separarCodigoDaFicha(desc)
				if !ok {
					continue
				}
				gravar(CatalogoItem{
					Codigo: codigo, Descricao: texto, Aplicacao: notas,
					Custo: custo, Observacao: "veio de uma ficha de falta da loja",
				})
			}
			rows.Close()
		}
	}

	gravarLote()
	writeJSON(w, 200, map[string]any{
		"novos": novos, "atualizados": atualizados, "ignorados": ignorados,
		"problemas": problemas, "total_no_catalogo": a.contarCatalogo(),
	})
}

// separarCodigoDaFicha entende o texto que o próprio catálogo grava na ficha:
// "[PA-5010] PASTILHA DE FREIO DIANTEIRA".
func separarCodigoDaFicha(desc string) (codigo, texto string, ok bool) {
	d := strings.TrimSpace(desc)
	if !strings.HasPrefix(d, "[") {
		return "", "", false
	}
	fim := strings.Index(d, "]")
	if fim <= 1 {
		return "", "", false
	}
	codigo = strings.TrimSpace(d[1:fim])
	texto = strings.TrimSpace(d[fim+1:])
	if codigo == "" {
		return "", "", false
	}
	return codigo, texto, true
}

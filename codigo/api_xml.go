package main

// Leitor de XML de catalogo/cotacao de fornecedor.
// O leitor e flexivel: identifica automaticamente os itens (pecas) do arquivo,
// independente dos nomes das tags, usando uma lista de sinonimos comuns
// (NF-e, planilhas de fornecedor, catalogos, etc).

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type xmlNode struct {
	Name     string
	Attrs    map[string]string
	Text     string
	Children []*xmlNode
}

func parseXMLTree(data []byte) (*xmlNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	root := &xmlNode{Name: "#root", Attrs: map[string]string{}}
	stack := []*xmlNode{root}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if len(root.Children) > 0 {
				break // aceita XML parcialmente malformado
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xmlNode{Name: t.Name.Local, Attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.Attrs[a.Name.Local] = strings.TrimSpace(a.Value)
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, n)
			stack = append(stack, n)
		case xml.CharData:
			s := strings.TrimSpace(string(t))
			if s != "" {
				cur := stack[len(stack)-1]
				if cur.Text != "" {
					cur.Text += " "
				}
				cur.Text += s
			}
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return root, nil
}

// achata um no guardando tambem o caminho das tags (para distinguir campos do item
// de campos que estao dentro de listas como <equivalentes>)
type xmlVal struct {
	Val  string
	Path []string
}

func flattenVals(n *xmlNode, prefix []string, out map[string][]xmlVal) {
	path := append(append([]string{}, prefix...), n.Name)
	if n.Text != "" {
		k := normKey(n.Name)
		out[k] = append(out[k], xmlVal{n.Text, path})
	}
	for k, v := range n.Attrs {
		if v == "" {
			continue
		}
		out[normKey(k)] = append(out[normKey(k)], xmlVal{v, path})
	}
	for _, c := range n.Children {
		flattenVals(c, path, out)
	}
}

var equivContainers = map[string]bool{
	"equivalentes": true, "equivalente": true, "equivalent": true, "similares": true,
	"similar": true, "cross": true, "crossreference": true, "referenciascruzadas": true,
	"codigosequivalentes": true, "codigosalternativos": true, "aplicacoes": true,
	"codigosimilar": true, "codigosimilares": true, "outroscodigos": true,
}

func normKey(s string) string {
	var b strings.Builder
	for _, r := range normText(s) {
		if r != ' ' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasEquivAncestor(path []string) bool {
	for i := 0; i < len(path)-1; i++ {
		if equivContainers[normKey(path[i])] {
			return true
		}
	}
	return false
}

// pick devolve o valor do campo priorizando as tags mais rasas (mais proximas do item)
func pick(m map[string][]xmlVal, field string) string {
	best := ""
	bestDepth := 99
	for _, alias := range fieldAliases[field] {
		for _, e := range m[alias] {
			v := strings.TrimSpace(e.Val)
			if v == "" || hasEquivAncestor(e.Path) {
				continue
			}
			if len(e.Path) < bestDepth {
				best = v
				bestDepth = len(e.Path)
			}
		}
	}
	return best
}

// pickAll busca todos os valores (usado para a lista de equivalentes)
func pickAll(m map[string][]xmlVal, field string) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		n := normCode(v)
		if v == "" || n == "" || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, v)
	}
	for _, alias := range fieldAliases[field] {
		for _, e := range m[alias] {
			if hasEquivAncestor(e.Path) {
				add(e.Val)
			}
		}
	}
	// codigos declarados como atributo dentro de listas de equivalentes
	for _, alias := range []string{"codigo", "cod", "codigofabricante", "referencia", "ean"} {
		for _, e := range m[alias] {
			if hasEquivAncestor(e.Path) {
				add(e.Val)
			}
		}
	}
	return out
}

var fieldAliases = map[string][]string{
	"descricao": {
		"descricao", "descricaoproduto", "descricaodoproduto", "desc", "xprod", "nome", "nomeproduto",
		"produto", "descricaoitem", "descricaopeca", "dsproduto", "descricaocurta", "description",
		"descricaocompleta", "nomepeca", "peca", "item.descricao", "det.prod.xprod",
	},
	"codfab": {
		"codigofabricante", "codfabricante", "codigodofabricante", "cod_fabricante", "referencia",
		"ref", "codigooriginal", "codoriginal", "codigoreferencia", "partnumber", "mpn",
		"codigomontadora", "codmontadora", "referenciafabricante", "codigoOEM", "oem", "codigopeca",
	},
	"codigo": {
		"codigo", "cod", "codproduto", "codigoproduto", "codigofornecedor", "coditem", "sku",
		"codigointerno", "codigocatalogo", "idproduto", "codigocomercial", "cprod", "cEAN",
	},
	"marca": {
		"marca", "brand", "fabricante", "marcafabricante", "montadora", "nomefabricante", "marcapeca",
	},
	"preco": {
		"preco", "precovenda", "precodevenda", "valor", "valorvenda", "precounitario", "precounidade",
		"price", "valorfinal", "precoconsumidor", "precoavista", "vlrvenda", "vunit", "valorunitario",
		// tags padrao da NF-e (item 1.0.2): valor unitario de comercializacao/tributavel
		"vuncom", "vuntrib", "vunitcom", "valorunitariocomercial", "precounitariocomercial",
	},
	"custo": {
		"custo", "precocusto", "precodecusto", "valorcusto", "precocompra", "precodedistribuidor",
		"vlrcusto", "cost", "precodedist", "custounitario",
	},
	"precototal": {
		"vprod", "valortotal", "precototal", "total", "valortotalbruto", "vtotal",
	},
	"qtdnota": {"qcom", "quantidadecomercial", "qtdcomercial", "quantidadecom", "qtde"},
	"estoque": {
		"estoque", "qtdestoque", "quantidadeestoque", "saldo", "saldos", "quantidade", "qtd",
		"disponivel", "stock", "qtestoque", "estoqueatual", "saldoestoque", "qtdisponivel", "qcom",
	},
	"ean":        {"ean", "codigobarras", "codigodebarras", "codbarra", "barras", "gtin", "ean13", "cean"},
	"categoria":  {"categoria", "grupo", "familia", "secao", "linha", "segmento", "tipoproduto", "classe"},
	"local":      {"local", "localizacao", "prateleira", "enderecoestoque", "corredor", "box", "deposito"},
	"unidade":    {"unidade", "un", "ucom", "unidademedida", "umed"},
	"aplicacao":  {"aplicacao", "aplicacoes", "veiculos", "carros", "compatibilidade", "modelo"},
	"observacao": {"observacao", "observacoes", "obs", "complemento", "infoadicional", "infadprod"},
	"equivalentes": {"equivalentes", "codigosequivalentes", "codigosalternativos", "similar", "similares",
		"crossreference", "cross", "referenciascruzadas", "codigos", "referencias", "aplicacoes", "equivalente"},
}

// nos candidatos a item: nos que tem folhas com "descricao" (ou codigo+preco) dentro deles
type xmlItem struct {
	Descricao    string
	Codigo       string
	CodFab       string
	Marca        string
	Categoria    string
	Local        string
	EAN          string
	Aplicacao    string
	PrecoTxt     string
	CustoTxt     string
	TotalTxt     string
	QtdNotaTxt   string
	EstoqueTxt   string
	Equivalentes []string
	No           string
}

func itemFromNode(n *xmlNode) xmlItem {
	m := map[string][]xmlVal{}
	flattenVals(n, nil, m)
	it := xmlItem{
		Descricao:    pick(m, "descricao"),
		Codigo:       pick(m, "codigo"),
		CodFab:       pick(m, "codfab"),
		Marca:        pick(m, "marca"),
		Categoria:    pick(m, "categoria"),
		Local:        pick(m, "local"),
		EAN:          pick(m, "ean"),
		Aplicacao:    pick(m, "aplicacao"),
		PrecoTxt:     pick(m, "preco"),
		CustoTxt:     pick(m, "custo"),
		TotalTxt:     pick(m, "precototal"),
		QtdNotaTxt:   pick(m, "qtdnota"),
		EstoqueTxt:   pick(m, "estoque"),
		Equivalentes: pickAll(m, "equivalentes"),
		No:           n.Name,
	}
	if it.Codigo == "" && it.CodFab == "" && it.EAN != "" {
		it.Codigo = it.EAN
	}
	// nota fiscal: preco unitario = valor total / quantidade
	if it.PrecoTxt == "" && it.TotalTxt != "" {
		q := parseQty(it.QtdNotaTxt)
		total := parseMoneyBR(it.TotalTxt)
		if q > 0 && total > 0 {
			it.PrecoTxt = fmt.Sprintf("%.2f", float64(total)/q/100)
		}
	}
	// se o codigo do fornecedor veio vazio mas existe codigo de fabricante, usa como referencia
	return it
}

func candidateScore(n *xmlNode) bool {
	if len(n.Children) == 0 && len(n.Attrs) == 0 {
		return false
	}
	it := itemFromNode(n)
	return it.Descricao != "" && (it.Codigo != "" || it.CodFab != "" || it.PrecoTxt != "" || it.CustoTxt != "" || it.TotalTxt != "")
}

func collectItems(n *xmlNode, depth int, out *[]xmlItem, depthLimit int) {
	if depth > depthLimit {
		return
	}
	hasChildCandidates := false
	for _, c := range n.Children {
		if candidateScore(c) || hasCandidateBelow(c, 2) {
			hasChildCandidates = true
			break
		}
	}
	if candidateScore(n) && !hasChildCandidates {
		*out = append(*out, itemFromNode(n))
		return
	}
	for _, c := range n.Children {
		collectItems(c, depth+1, out, depthLimit)
	}
}

// hasCandidateBelow ajuda a nao parar em nos "guarda-chuva" (ex.: <produtos><produto>)
func hasCandidateBelow(n *xmlNode, nivel int) bool {
	if nivel == 0 {
		return false
	}
	if candidateScore(n) {
		return true
	}
	for _, c := range n.Children {
		if hasCandidateBelow(c, nivel-1) {
			return true
		}
	}
	return false
}

func parseCatalogXML(data []byte) ([]xmlItem, error) {
	root, err := parseXMLTree(data)
	if err != nil {
		return nil, err
	}
	var items []xmlItem
	collectItems(root, 0, &items, 8)
	// conta tambem os registros vazios do arquivo (ex.: <produto></produto>)
	var vazios int
	contarVazios(root, &vazios, 0)
	for i := 0; i < vazios; i++ {
		items = append(items, xmlItem{})
	}
	return items, nil
}

var nomesDeItem = map[string]bool{
	"produto": true, "item": true, "peca": true, "product": true, "det": true,
	"produtocatalogo": true, "itemcatalogo": true,
}

func contarVazios(n *xmlNode, acc *int, depth int) {
	if depth > 8 {
		return
	}
	if nomesDeItem[normKey(n.Name)] && n.Text == "" {
		semFilhos := true
		for _, c := range n.Children {
			if strings.TrimSpace(c.Text) != "" {
				semFilhos = false
				break
			}
		}
		if semFilhos && len(n.Attrs) == 0 {
			*acc++
			return
		}
	}
	for _, c := range n.Children {
		contarVazios(c, acc, depth+1)
	}
}

// charsetReader converte arquivos em ISO-8859-1 / Windows-1252 para UTF-8
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	cs := strings.ToLower(strings.TrimSpace(charset))
	if cs == "iso-8859-1" || cs == "latin1" || cs == "windows-1252" || cs == "cp1252" || cs == "iso8859-1" {
		data, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		var b bytes.Buffer
		for _, c := range data {
			b.WriteRune(rune(c))
		}
		return bytes.NewReader(b.Bytes()), nil
	}
	return input, nil
}

// toUTF8 detecta arquivos em latin1 e ja converte os bytes, reescrevendo a declaracao
func toUTF8(data []byte) []byte {
	head := strings.ToLower(string(data[:min(len(data), 300)]))
	if strings.Contains(head, "iso-8859-1") || strings.Contains(head, "latin1") || strings.Contains(head, "windows-1252") {
		var b bytes.Buffer
		for _, c := range data {
			b.WriteRune(rune(c))
		}
		out := b.Bytes()
		// a declaracao original diz latin1, mas os bytes ja estao em UTF-8
		out = reEncodingDecl.ReplaceAll(out, []byte(`encoding="UTF-8"`))
		return out
	}
	return data
}

var reEncodingDecl = regexp.MustCompile(`encoding\s*=\s*["'][^"']*["']`)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- API ----------

type importOptions struct {
	Origem              string  `json:"origem"`             // vazio = importacao XML; "nota fotografada" etc.
	Margem              float64 `json:"margem_percentual"`  // markup sobre o custo quando o XML nao traz preco de venda
	PrecoXMLCusto       bool    `json:"preco_xml_eh_custo"` // true = coluna preco do XML e custo
	AtualizarEstoque    bool    `json:"atualizar_estoque"`  // lancar estoque informado como entrada
	ReceberFichas       bool    `json:"receber_fichas"`     // fechar fichas de falta que batem
	AtualizarCusto      bool    `json:"atualizar_custo"`
	AtualizarPreco      bool    `json:"atualizar_preco"`
	AtualizarDescricao  bool    `json:"atualizar_descricao"`
	CriarNovas          bool    `json:"criar_novas"`
	SobrescreverEstoque bool    `json:"sobrescrever_estoque"`
	FornecedorID        *int64  `json:"fornecedor_id"`

	// --- nota de entrada / contas a pagar (1.0.8) ---
	LancarContaPagar    *bool  `json:"lancar_conta_pagar"`   // nil = automatico (true quando o XML traz o total)
	RatearExtras        *bool  `json:"ratear_extras"`        // distribui frete/IPI/ST no custo das pecas
	CadastrarFornecedor *bool  `json:"cadastrar_fornecedor"` // cria o fornecedor do XML quando nao existe
	Vencimento          string `json:"vencimento"`           // AAAA-MM-DD; vazio = emissao + prazo
	PrazoDias           int    `json:"prazo_dias"`           // prazo quando a nota nao traz vencimento
}

// rotuloEntrada devolve o texto gravado no historico de estoque.
// Sem Origem o texto continua identico ao da importacao de XML.
func (o importOptions) rotuloEntrada() string {
	if strings.TrimSpace(o.Origem) == "" {
		return "importacao XML"
	}
	return "entrada pela " + o.Origem
}

func (o importOptions) refTipo() string {
	if strings.TrimSpace(o.Origem) == "" {
		return "xml"
	}
	return "nota_foto"
}

func defaultImportOptions() importOptions {
	return importOptions{
		Margem: 60, PrecoXMLCusto: true, AtualizarEstoque: false, ReceberFichas: true,
		AtualizarCusto: true, AtualizarPreco: true, AtualizarDescricao: false,
		CriarNovas: true, SobrescreverEstoque: false,
		PrazoDias: 30,
	}
}

func readUpload(r *http.Request) ([]byte, string, error) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return nil, "", err
	}
	f, hdr, err := r.FormFile("arquivo")
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		return nil, "", err
	}
	return toUTF8(data), hdr.Filename, nil
}

func (a *App) handleAnalyzeXML(w http.ResponseWriter, r *http.Request) {
	data, name, err := readUpload(r)
	if err != nil {
		fail(w, 400, "nao consegui ler o arquivo: "+err.Error())
		return
	}
	items, err := parseCatalogXML(data)
	if err != nil {
		fail(w, 400, "XML invalido: "+err.Error())
		return
	}
	// cabecalho da nota (fornecedor, valor total, boletos) - se o arquivo for NF-e
	var nota NotaInfo
	if root, e := parseXMLTree(data); e == nil {
		nota = lerNotaDoXML(root)
	}
	// classifica
	novos, atualizacoes, ignorados := 0, 0, 0
	amostra := []map[string]any{}
	for _, it := range items {
		id, _ := a.matchPart(it)
		if it.Descricao == "" && it.CodFab == "" && it.Codigo == "" {
			ignorados++
			continue
		}
		if id > 0 {
			atualizacoes++
		} else {
			novos++
		}
		if len(amostra) < 30 {
			amostra = append(amostra, map[string]any{
				"descricao": it.Descricao, "codigo": it.Codigo, "codigo_fabricante": it.CodFab,
				"marca": it.Marca, "preco_texto": it.PrecoTxt, "custo_texto": it.CustoTxt,
				"estoque_texto": it.EstoqueTxt, "acao": map[bool]string{true: "atualizar", false: "cadastrar"}[id > 0],
			})
		}
	}
	resp := map[string]any{
		"arquivo": name, "total_itens": len(items), "novos": novos,
		"atualizacoes": atualizacoes, "ignorados": ignorados, "amostra": amostra,
	}
	if nota.TemTotal || nota.EmitenteNome != "" || len(nota.Duplicatas) > 0 {
		jaImportada := a.notaJaImportada(nota, nota.EmitenteDoc, nota.EmitenteNome)
		fornecedorID := a.acharOuCriarFornecedor(nota.EmitenteNome, nota.EmitenteDoc, false)
		// 1.0.10: avisa na ANALISE (antes de importar) se os boletos nao
		// fecham com o total da nota - o usuario confere antes, no portal.
		prev := []Duplicata{}
		for _, d := range nota.Duplicatas {
			if d.ValorCents > 0 {
				prev = append(prev, d)
			}
		}
		_, avisosBoletos := reconciliarBoletos(nota, prev)
		resp["nota"] = map[string]any{
			"numero": nota.Numero, "serie": nota.Serie, "chave": nota.Chave,
			"fornecedor": nota.EmitenteNome, "cnpj": nota.EmitenteDoc,
			"emitida_em":     nota.EmitidaEm,
			"total_centavos": nota.TotalCents, "produtos_centavos": nota.ProdutosCents,
			"extras_centavos": nota.ExtrasCents, "tem_total": nota.TemTotal,
			"duplicatas":    nota.Duplicatas,
			"fornecedor_id": fornecedorID, "fornecedor_cadastrado": fornecedorID > 0,
			"ja_importada":        jaImportada > 0,
			"vencimento_sugerido": vencimentoPadrao(nota.EmitidaEm, 30),
			"avisos":              avisosBoletos,
		}
	}
	writeJSON(w, 200, resp)
}

// matchPart tenta achar a peca ja cadastrada. Retorna 0 se nao existe.
func (a *App) matchPart(it xmlItem) (int64, string) {
	cands := []struct{ kind, val string }{}
	if it.Codigo != "" {
		cands = append(cands, struct{ kind, val string }{"code", normCode(it.Codigo)})
	}
	if it.CodFab != "" {
		cands = append(cands, struct{ kind, val string }{"mfr", normCode(it.CodFab)})
	}
	if it.EAN != "" {
		cands = append(cands, struct{ kind, val string }{"ean", normCode(it.EAN)})
	}
	for _, c := range cands {
		var id int64
		var err error
		switch c.kind {
		case "code":
			err = a.db.QueryRow(`SELECT id FROM parts WHERE code_norm=?`, c.val).Scan(&id)
		case "mfr":
			err = a.db.QueryRow(`SELECT id FROM parts WHERE mfr_norm=? LIMIT 1`, c.val).Scan(&id)
		case "ean":
			err = a.db.QueryRow(`SELECT part_id FROM equivalents WHERE code_norm=? LIMIT 1`, c.val).Scan(&id)
		}
		if err == nil && id > 0 {
			return id, c.kind
		}
	}
	// cruzamento por equivalentes (codigo ou codigo de fabricante informado)
	for _, v := range append([]string{it.CodFab, it.Codigo, it.EAN}, it.Equivalentes...) {
		n := normCode(v)
		if n == "" {
			continue
		}
		var id int64
		if err := a.db.QueryRow(`SELECT part_id FROM equivalents WHERE code_norm=? LIMIT 1`, n).Scan(&id); err == nil && id > 0 {
			return id, "equivalente"
		}
	}
	return 0, ""
}

type ImportResult struct {
	Arquivo        string        `json:"arquivo"`
	TotalItens     int           `json:"total_itens"`
	Cadastradas    int           `json:"cadastradas"`
	Atualizadas    int           `json:"atualizadas"`
	Ignoradas      int           `json:"ignoradas"`
	FichasFechadas int           `json:"fichas_fechadas"`
	ListaIgnoradas []IgnoredItem `json:"ignoradas_lista"`
	ListaNovas     []string      `json:"novas_lista"`

	// nota de entrada (1.0.8)
	NotaNumero     string      `json:"nota_numero,omitempty"`
	NotaChave      string      `json:"nota_chave,omitempty"`
	FornecedorID   int64       `json:"fornecedor_id,omitempty"`
	FornecedorNome string      `json:"fornecedor_nome,omitempty"`
	FornecedorNovo bool        `json:"fornecedor_novo,omitempty"`
	TotalNota      int64       `json:"total_nota_centavos,omitempty"`
	RateadoCents   int64       `json:"rateado_centavos,omitempty"`
	ContasCriadas  int         `json:"contas_criadas,omitempty"`
	ContasTotal    int64       `json:"contas_total_centavos,omitempty"`
	Contas         []Duplicata `json:"contas,omitempty"`
	NotaJaExistia  bool        `json:"nota_ja_existia,omitempty"`
	Avisos         []string    `json:"avisos,omitempty"`
}

type IgnoredItem struct {
	Descricao string `json:"descricao"`
	Codigo    string `json:"codigo"`
	Motivo    string `json:"motivo"`
}

func (a *App) handleImportXML(w http.ResponseWriter, r *http.Request) {
	data, name, err := readUpload(r)
	if err != nil {
		fail(w, 400, "nao consegui ler o arquivo: "+err.Error())
		return
	}
	opts := defaultImportOptions()
	if s := r.FormValue("opcoes"); s != "" {
		_ = jsonUnmarshal([]byte(s), &opts)
	}
	items, err := parseCatalogXML(data)
	if err != nil {
		fail(w, 400, "XML invalido: "+err.Error())
		return
	}
	res := ImportResult{Arquivo: name, TotalItens: len(items), ListaIgnoradas: []IgnoredItem{}, ListaNovas: []string{}, Avisos: []string{}}
	seenFile := map[string]bool{}

	// ---- cabecalho da nota (1.0.8): fornecedor, valor total e boletos ----
	var nota NotaInfo
	if root, e := parseXMLTree(data); e == nil {
		nota = lerNotaDoXML(root)
	}
	prazo := opts.PrazoDias
	if prazo <= 0 {
		prazo = 30
	}
	cadastrarFornecedor := opts.CadastrarFornecedor == nil || *opts.CadastrarFornecedor
	fornecedorID := int64(0)
	if opts.FornecedorID != nil && *opts.FornecedorID > 0 {
		fornecedorID = *opts.FornecedorID
	} else if nota.EmitenteNome != "" {
		if id := a.acharOuCriarFornecedor(nota.EmitenteNome, nota.EmitenteDoc, false); id > 0 {
			fornecedorID = id
		} else if cadastrarFornecedor {
			fornecedorID = a.acharOuCriarFornecedor(nota.EmitenteNome, nota.EmitenteDoc, true)
			res.FornecedorNovo = fornecedorID > 0
		}
	}
	res.FornecedorID = fornecedorID
	res.FornecedorNome = nota.EmitenteNome
	res.NotaNumero = nota.Numero
	res.NotaChave = nota.Chave
	res.TotalNota = nota.TotalCents
	if fornecedorID > 0 && opts.FornecedorID == nil {
		opts.FornecedorID = &fornecedorID
	}

	// rateio do que nao e produto (frete, IPI, ST...) no custo unitario
	ratear := nota.TemTotal && nota.ExtrasCents > 0 && (opts.RatearExtras == nil || *opts.RatearExtras)
	var rateio []int64
	if ratear {
		rateio = calcularRateio(items, nota.ExtrasCents)
	}

	for idx, it := range items {
		desc := strings.TrimSpace(it.Descricao)
		if desc == "" {
			res.Ignoradas++
			res.ListaIgnoradas = append(res.ListaIgnoradas, IgnoredItem{Descricao: "(sem descricao)", Codigo: it.Codigo + it.CodFab, Motivo: "Item sem descricao no XML"})
			continue
		}
		key := normCode(it.CodFab)
		if key == "" {
			key = normCode(it.Codigo)
		}
		if key == "" {
			key = normText(desc)
		}
		if seenFile[key] {
			res.Ignoradas++
			res.ListaIgnoradas = append(res.ListaIgnoradas, IgnoredItem{Descricao: desc, Codigo: it.CodFab + it.Codigo, Motivo: "Codigo repetido dentro do proprio arquivo XML"})
			continue
		}
		seenFile[key] = true

		custo := parseMoneyBR(it.CustoTxt)
		preco := parseMoneyBR(it.PrecoTxt)
		if opts.PrecoXMLCusto {
			if custo == 0 && preco > 0 {
				custo = preco
			}
		} else {
			if preco > 0 && custo == 0 {
				custo = int64(float64(preco) / (1 + opts.Margem/100.0))
			}
		}
		// frete/IPI/ST da nota entram no custo da peca: sem isso o lucro que o
		// sistema mostra seria maior do que o que sobra no caixa.
		if ratear && idx < len(rateio) && rateio[idx] > 0 {
			custo += rateio[idx]
			res.RateadoCents += rateio[idx] * int64(parseQty(it.EstoqueTxt))
			if res.RateadoCents > nota.ExtrasCents {
				res.RateadoCents = nota.ExtrasCents
			}
		}
		if opts.PrecoXMLCusto {
			if custo > 0 {
				preco = int64(float64(custo) * (1 + opts.Margem/100.0))
			}
		} else {
			if preco == 0 && custo > 0 {
				preco = int64(float64(custo) * (1 + opts.Margem/100.0))
			}
		}
		qtdXML := parseQty(it.EstoqueTxt)
		if custo < 0 || preco < 0 || qtdXML < 0 {
			res.Ignoradas++
			res.ListaIgnoradas = append(res.ListaIgnoradas, IgnoredItem{Descricao: desc, Codigo: it.CodFab + it.Codigo, Motivo: "Preco ou estoque negativo no XML"})
			continue
		}

		id, _ := a.matchPart(it)
		if id > 0 {
			if err := a.updatePartFromXML(id, it, custo, preco, qtdXML, opts); err != nil {
				res.Ignoradas++
				res.ListaIgnoradas = append(res.ListaIgnoradas, IgnoredItem{Descricao: desc, Codigo: it.CodFab + it.Codigo, Motivo: "Erro ao atualizar: " + err.Error()})
				continue
			}
			res.Atualizadas++
			if opts.ReceberFichas {
				n, _ := a.receiveFaltasForPart(id, desc)
				res.FichasFechadas += n
			}
			continue
		}

		if !opts.CriarNovas {
			res.Ignoradas++
			res.ListaIgnoradas = append(res.ListaIgnoradas, IgnoredItem{Descricao: desc, Codigo: it.CodFab + it.Codigo, Motivo: "Peca nao cadastrada e a opcao de criar novas esta desligada"})
			continue
		}
		newID, err := a.createPartFromXML(it, custo, preco, qtdXML, opts)
		if err != nil {
			res.Ignoradas++
			res.ListaIgnoradas = append(res.ListaIgnoradas, IgnoredItem{Descricao: desc, Codigo: it.CodFab + it.Codigo, Motivo: "Erro ao cadastrar: " + err.Error()})
			continue
		}
		res.Cadastradas++
		res.ListaNovas = append(res.ListaNovas, desc)
		if opts.ReceberFichas {
			n, _ := a.receiveFaltasForPart(newID, desc)
			res.FichasFechadas += n
		}
	}
	// 1.0.10: o rateio por item arredonda; se sobrou uma diferenca de centavos,
	// o usuario fica sabendo em vez de o custo sair silenciosamente menor.
	if ratear && nota.ExtrasCents > 0 && res.RateadoCents < nota.ExtrasCents {
		if dif := nota.ExtrasCents - res.RateadoCents; dif > 0 {
			res.Avisos = append(res.Avisos,
				fmt.Sprintf("Frete/IPI/ST da nota: %s. Distribuí %s no custo das peças; sobraram %s (arredondamento de menos de um centavo por item).",
					fmtMoney(nota.ExtrasCents), fmtMoney(res.RateadoCents), fmtMoney(dif)))
		}
	}
	// ---- conta a pagar da nota (1.0.8) ----
	if nota.TemTotal || len(nota.Duplicatas) > 0 {
		// o aviso de "nota ja lancada" vale mesmo quando o usuario desmarca a
		// conta a pagar: senao ele reimporta, nada aparece e ele fica na duvida
		res.NotaJaExistia = a.notaJaImportada(nota, nota.EmitenteDoc, nota.EmitenteNome) > 0
		lancar := nota.TemTotal
		if opts.LancarContaPagar != nil {
			lancar = *opts.LancarContaPagar
		}
		if lancar {
			contas, jaExistia, err := a.registrarNotaEmitirContas(nota, &res, opts, fornecedorID, name, prazo)
			if err != nil {
				res.ListaIgnoradas = append(res.ListaIgnoradas, IgnoredItem{
					Descricao: "conta a pagar da nota " + nota.Numero, Codigo: nota.Chave,
					Motivo: "Não consegui lançar a conta a pagar: " + err.Error(),
				})
			} else {
				res.NotaJaExistia = jaExistia
				res.Contas = contas
				for _, c := range contas {
					res.ContasCriadas++
					res.ContasTotal += c.ValorCents
				}
			}
		}
	}

	writeJSON(w, 200, res)
}

// calcularRateio divide o que nao e produto (frete, IPI, ST, outras despesas)
// pelo custo unitario de cada item, proporcional ao peso do item na nota.
// Devolve, por item, quantos centavos somar no custo de UMA unidade.
func calcularRateio(items []xmlItem, extras int64) []int64 {
	out := make([]int64, len(items))
	if extras <= 0 || len(items) == 0 {
		return out
	}
	peso := make([]int64, len(items))
	var total int64
	for i, it := range items {
		qtd := parseQty(it.EstoqueTxt)
		if qtd <= 0 {
			qtd = 1
		}
		tot := parseMoneyBR(it.TotalTxt)
		if tot <= 0 {
			tot = int64(float64(parseMoneyBR(it.PrecoTxt)) * qtd)
		}
		if tot <= 0 {
			tot = int64(float64(parseMoneyBR(it.CustoTxt)) * qtd)
		}
		peso[i] = tot
		total += tot
	}
	if total <= 0 {
		return out
	}
	var aplicado int64
	for i := range items {
		qtd := parseQty(items[i].EstoqueTxt)
		if qtd <= 0 {
			qtd = 1
		}
		linha := int64(float64(extras) * float64(peso[i]) / float64(total))
		unidade := int64(math.Round(float64(linha) / qtd))
		if unidade <= 0 && linha > 0 {
			unidade = 1
		}
		out[i] = unidade
		aplicado += unidade * int64(qtd)
	}
	// sobra de arredondamento vai para o item de maior peso
	if sobra := extras - aplicado; sobra > 0 {
		maior := 0
		for i := range peso {
			if peso[i] > peso[maior] {
				maior = i
			}
		}
		qtd := parseQty(items[maior].EstoqueTxt)
		if qtd <= 0 {
			qtd = 1
		}
		out[maior] += int64(math.Round(float64(sobra) / qtd))
	}
	return out
}

func xmlCodes(it xmlItem) []string {
	codes := []string{}
	seen := map[string]bool{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		n := normCode(v)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		codes = append(codes, strings.ToUpper(v))
	}
	add(it.CodFab)
	add(it.Codigo)
	add(it.EAN)
	for _, e := range it.Equivalentes {
		add(e)
	}
	return codes
}

func origemTexto(o importOptions) string {
	if strings.TrimSpace(o.Origem) == "" {
		return "XML"
	}
	return o.Origem
}

func (a *App) createPartFromXML(it xmlItem, custo, preco int64, qtd float64, opts importOptions) (int64, error) {
	code := strings.TrimSpace(it.CodFab)
	if code == "" {
		code = strings.TrimSpace(it.Codigo)
	}
	if code == "" {
		code = strings.TrimSpace(it.EAN)
	}
	// evita conflito de codigo proprio
	var exists int64
	if code != "" {
		_ = a.db.QueryRow(`SELECT id FROM parts WHERE code_norm=?`, normCode(code)).Scan(&exists)
		if exists > 0 {
			code = ""
		}
	}
	var newID int64
	err := a.db.Tx(func(tx *sql.Tx) error {
		c := code
		if c == "" {
			var n int64
			if err := tx.QueryRow(`SELECT IFNULL(MAX(id),0)+1 FROM parts`).Scan(&n); err != nil {
				return err
			}
			c = fmt.Sprintf("INT-%05d", n)
		}
		equivs := xmlCodes(it)
		// a aplicacao (veiculos compativeis) do arquivo vai para a peca e para a busca
		blob := buildSearchBlobApp(c, it.CodFab, it.Descricao, it.Marca, it.Categoria, it.Local, it.Aplicacao, equivs)
		// peca nova: o estoque informado no arquivo vira o estoque inicial
		stock := qtd
		if stock < 0 {
			stock = 0
		}
		res, err := tx.Exec(`INSERT INTO parts(code,code_norm,mfr_code,mfr_norm,description,brand,category,application,cost_cents,price_cents,min_stock,stock,location,supplier_id,active,search_blob,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,?,?,?)`,
			c, normCode(c), it.CodFab, normCode(it.CodFab), it.Descricao, it.Marca, it.Categoria, it.Aplicacao, custo, preco, 1, stock, it.Local, opts.FornecedorID, blob, nowISO(), nowISO())
		if err != nil {
			return err
		}
		newID, _ = res.LastInsertId()
		if stock != 0 {
			if _, err := tx.Exec(`INSERT INTO stock_moves(part_id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,ref_id,created_at)
				VALUES(?,?,?,?,?,?,?,?,?)`, newID, "entrada", stock, stock, custo, opts.rotuloEntrada(), opts.refTipo(), newID, nowISO()); err != nil {
				return err
			}
		}
		for _, e := range equivs {
			if _, err := tx.Exec(`INSERT INTO equivalents(part_id,code,code_norm) VALUES(?,?,?)`, newID, e, normCode(e)); err != nil {
				return err
			}
		}
		return nil
	})
	return newID, err
}

func (a *App) updatePartFromXML(id int64, it xmlItem, custo, preco int64, qtd float64, opts importOptions) error {
	return a.db.Tx(func(tx *sql.Tx) error {
		var curCusto, curPreco int64
		var curDesc, curMfr, curBrand, curCat, curLoc, curCode, blobAtual, curApp string
		var curStock float64
		if err := tx.QueryRow(`SELECT cost_cents,price_cents,description,IFNULL(mfr_code,''),brand,category,location,stock,code,search_blob,IFNULL(application,'') FROM parts WHERE id=?`, id).
			Scan(&curCusto, &curPreco, &curDesc, &curMfr, &curBrand, &curCat, &curLoc, &curStock, &curCode, &blobAtual, &curApp); err != nil {
			return err
		}
		if opts.AtualizarCusto && custo > 0 {
			curCusto = custo
		}
		if opts.AtualizarPreco && preco > 0 {
			curPreco = preco
		}
		if curMfr == "" && it.CodFab != "" {
			curMfr = it.CodFab
		}
		if curBrand == "" && it.Marca != "" {
			curBrand = it.Marca
		}
		if curCat == "" && it.Categoria != "" {
			curCat = it.Categoria
		}
		if curLoc == "" && it.Local != "" {
			curLoc = it.Local
		}
		// aplicacao: completa se faltava, atualiza se o arquivo trouxe outra
		if it.Aplicacao != "" && (curApp == "" || opts.AtualizarDescricao) {
			curApp = it.Aplicacao
		}
		if opts.AtualizarDescricao && it.Descricao != "" {
			curDesc = it.Descricao
		}
		// equivalentes: adiciona os novos sem remover os existentes
		novos := xmlCodes(it)
		for _, e := range novos {
			if _, err := tx.Exec(`INSERT INTO equivalents(part_id,code,code_norm) VALUES(?,?,?)
				ON CONFLICT DO NOTHING`, id, e, normCode(e)); err != nil {
				return err
			}
		}
		// evita duplicar equivalentes ja existentes
		if _, err := tx.Exec(`DELETE FROM equivalents WHERE part_id=? AND id NOT IN (SELECT MIN(id) FROM equivalents WHERE part_id=? GROUP BY code_norm)`, id, id); err != nil {
			return err
		}
		var eqs []string
		rows, err := tx.Query(`SELECT code FROM equivalents WHERE part_id=?`, id)
		if err == nil {
			for rows.Next() {
				var c string
				if rows.Scan(&c) == nil {
					eqs = append(eqs, c)
				}
			}
			rows.Close()
		}
		blob := buildSearchBlobApp(curCode, curMfr, curDesc, curBrand, curCat, curLoc, curApp, eqs)
		if _, err := tx.Exec(`UPDATE parts SET cost_cents=?,price_cents=?,mfr_code=?,mfr_norm=?,brand=?,category=?,location=?,description=?,application=?,search_blob=?,updated_at=? WHERE id=?`,
			curCusto, curPreco, curMfr, normCode(curMfr), curBrand, curCat, curLoc, curDesc, curApp, blob, nowISO(), id); err != nil {
			return err
		}
		if opts.AtualizarEstoque {
			if opts.SobrescreverEstoque {
				if _, err := tx.Exec(`UPDATE parts SET stock=? WHERE id=?`, qtd, id); err != nil {
					return err
				}
				if _, err := tx.Exec(`INSERT INTO stock_moves(part_id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,ref_id,created_at)
					VALUES(?,?,?,?,?,?,?,?,?)`, id, "ajuste", qtd-curStock, qtd, curCusto, "estoque sincronizado pela "+origemTexto(opts), opts.refTipo(), id, nowISO()); err != nil {
					return err
				}
			} else if qtd > 0 {
				if _, err := tx.Exec(`UPDATE parts SET stock=stock+? WHERE id=?`, qtd, id); err != nil {
					return err
				}
				if _, err := tx.Exec(`INSERT INTO stock_moves(part_id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,ref_id,created_at)
					VALUES(?,?,?,?,?,?,?,?,?)`, id, "entrada", qtd, curStock+qtd, curCusto, opts.rotuloEntrada(), opts.refTipo(), id, nowISO()); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// receiveFaltasForPart fecha fichas de falta abertas da peca e da entrada no estoque.
func (a *App) receiveFaltasForPart(partID int64, descricao string) (int, error) {
	rows, err := a.db.Query(`SELECT id,qty,cost_cents FROM faltas WHERE status IN ('aberta','pedida') AND part_id=?`, partID)
	if err != nil {
		return 0, err
	}
	type f struct {
		id   int64
		qty  float64
		cost int64
	}
	list := []f{}
	for rows.Next() {
		var x f
		if rows.Scan(&x.id, &x.qty, &x.cost) == nil {
			list = append(list, x)
		}
	}
	rows.Close()
	// fichas sem peca vinculada: compara descricao
	if descricao != "" {
		nd := normText(descricao)
		rows2, err := a.db.Query(`SELECT id,qty,cost_cents,description FROM faltas WHERE status IN ('aberta','pedida') AND part_id IS NULL`)
		if err == nil {
			for rows2.Next() {
				var x f
				var d string
				if rows2.Scan(&x.id, &x.qty, &x.cost, &d) == nil {
					nfd := normText(d)
					if nfd != "" && (strings.Contains(nd, nfd) || strings.Contains(nfd, nd)) {
						list = append(list, x)
					}
				}
			}
			rows2.Close()
		}
	}
	count := 0
	for _, x := range list {
		if x.qty <= 0 {
			x.qty = 1
		}
		if err := a.receiveFalta(x.id, x.qty, partID); err == nil {
			count++
		}
	}
	return count, nil
}

// erroFichaJaRecebida indica que a ficha ja tinha sido recebida (nao pode
// entrar no estoque de novo).
type erroFichaJaRecebida struct{ faltaID int64 }

func (e erroFichaJaRecebida) Error() string {
	return fmt.Sprintf("a ficha %d já foi recebida; o estoque não foi lançado de novo", e.faltaID)
}

func (a *App) receiveFalta(faltaID int64, qty float64, partID int64) error {
	return a.db.Tx(func(tx *sql.Tx) error {
		var pid sql.NullInt64
		var statusAtual string
		if err := tx.QueryRow(`SELECT part_id,status FROM faltas WHERE id=?`, faltaID).Scan(&pid, &statusAtual); err != nil {
			return err
		}
		// trava contra duplo clique / outra aba / pedido ja recebido
		if statusAtual == "recebida" {
			return erroFichaJaRecebida{faltaID: faltaID}
		}
		target := partID
		if pid.Valid && pid.Int64 > 0 {
			target = pid.Int64
		}
		if target == 0 {
			return fmt.Errorf("ficha sem peca vinculada")
		}
		var bal float64
		var cost int64
		if err := tx.QueryRow(`SELECT stock,cost_cents FROM parts WHERE id=?`, target).Scan(&bal, &cost); err != nil {
			return err
		}
		nb := bal + qty
		if _, err := tx.Exec(`UPDATE parts SET stock=?, updated_at=? WHERE id=?`, nb, nowISO(), target); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO stock_moves(part_id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,ref_id,created_at)
			VALUES(?,?,?,?,?,?,?,?,?)`, target, "entrada", qty, nb, cost, "recebimento de ficha de falta", "falta", faltaID, nowISO()); err != nil {
			return err
		}
		res, err := tx.Exec(`UPDATE faltas SET status='recebida', part_id=?, received_at=? WHERE id=? AND status IN ('aberta','pedida')`, target, nowISO(), faltaID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return erroFichaJaRecebida{faltaID: faltaID}
		}
		return nil
	})
}

func jsonUnmarshal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

var _ = strconv.Itoa

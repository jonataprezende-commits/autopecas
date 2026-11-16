package main

// Leitura do "cabecalho" da nota no XML: quem emitiu, numero, valor total e as
// duplicatas (boletos). E o que permite lancar a conta a pagar sozinho quando a
// nota entra para dar entrada nas pecas.
//
// Regra de ouro: se o arquivo nao for uma NF-e (for so uma lista de produtos),
// NotaInfo volta vazio e nada muda no comportamento antigo.

import (
	"strconv"
	"strings"
)

type Duplicata struct {
	Numero     string `json:"numero"`
	Vencimento string `json:"vencimento"` // AAAA-MM-DD
	ValorCents int64  `json:"valor_centavos"`
}

type NotaInfo struct {
	Chave         string // 44 digitos, sem espacos
	Numero        string
	Serie         string
	EmitenteNome  string
	EmitenteDoc   string // CNPJ/CPF so digitos
	EmitidaEm     string // AAAA-MM-DD
	TotalCents    int64  // vNF (valor total da nota)
	ProdutosCents int64  // soma de vProd dos itens
	FreteCents    int64
	ExtrasCents   int64 // total - produtos (frete, IPI, ST, outras despesas)
	Duplicatas    []Duplicata
	TemTotal      bool
}

// nomes aceitos para o emitente/fornecedor (fora do bloco de itens)
var aliasesEmitenteNome = []string{
	"xnome", "razaosocial", "nomeemitente", "nomefornecedor", "emitente", "fornecedor", "nomefantasia",
}
var aliasesEmitenteDoc = []string{"cnpj", "cpf", "cnpjcpf", "cnpjemitente", "documento", "cgc"}
var aliasesTotal = []string{"vnf", "vtotal", "totalnota", "valortotalnota", "valortotal", "totalnf", "valornota"}
var aliasesProdutos = []string{"vprod", "totalprodutos", "valortotalprodutos", "produtos"}
var aliasesFrete = []string{"vfrete", "frete", "valorfrete"}

func apenasDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// dataISO converte o que aparece em XML (2026-10-29, 2026-10-29T10:00:00-03:00,
// 29/10/2026) para AAAA-MM-DD. Devolve "" se nao reconhecer.
func dataISO(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		if _, err := strconv.Atoi(s[0:4]); err == nil {
			return s[0:10]
		}
	}
	if len(s) == 10 && s[2] == '/' && s[5] == '/' {
		if _, err := strconv.Atoi(s[6:10]); err == nil {
			return s[6:10] + "-" + s[3:5] + "-" + s[0:2]
		}
	}
	return ""
}

// procuraNoDesce acha o primeiro no (em qualquer profundidade) cujo nome bata
// com um dos nomes e devolve os filhos diretos com o texto.
func nomeDoNo(n *xmlNode) string { return normKey(n.Name) }

func primeiroFilho(n *xmlNode, nomes ...string) *xmlNode {
	quer := map[string]bool{}
	for _, x := range nomes {
		quer[x] = true
	}
	var achado *xmlNode
	var anda func(*xmlNode)
	anda = func(cur *xmlNode) {
		if achado != nil {
			return
		}
		for _, c := range cur.Children {
			if quer[nomeDoNo(c)] && c.Text != "" {
				achado = c
				return
			}
			anda(c)
		}
	}
	anda(n)
	return achado
}

// primeiroFilhoRaso so aceita nos com profundidade <= max (evita pegar um campo
// "fornecedor" que esteja dentro de um item da lista).
func primeiroFilhoRaso(n *xmlNode, max int, nomes ...string) *xmlNode {
	quer := map[string]bool{}
	for _, x := range nomes {
		quer[x] = true
	}
	var achado *xmlNode
	var anda func(*xmlNode, int)
	anda = func(cur *xmlNode, nivel int) {
		if achado != nil || nivel > max {
			return
		}
		for _, c := range cur.Children {
			if quer[nomeDoNo(c)] && c.Text != "" {
				achado = c
				return
			}
			anda(c, nivel+1)
		}
	}
	anda(n, 0)
	return achado
}

func textoDe(n *xmlNode, nomes ...string) string {
	if c := primeiroFilho(n, nomes...); c != nil {
		return strings.TrimSpace(c.Text)
	}
	return ""
}

// acharBloco acha o no cujo nome bate (ex: "emit", "total", "icmstot", "cobr").
func acharBloco(n *xmlNode, nomes ...string) *xmlNode {
	quer := map[string]bool{}
	for _, x := range nomes {
		quer[x] = true
	}
	var achado *xmlNode
	var anda func(*xmlNode)
	anda = func(cur *xmlNode) {
		if achado != nil {
			return
		}
		for _, c := range cur.Children {
			if quer[nomeDoNo(c)] {
				achado = c
				return
			}
			anda(c)
		}
	}
	anda(n)
	return achado
}

// lerNotaDoXML extrai o cabecalho da nota. Vale para NF-e (padrao nacional) e,
// com tolerancia, para arquivos que tragam apenas "fornecedor" e "total".
func lerNotaDoXML(root *xmlNode) NotaInfo {
	var info NotaInfo

	// chave de acesso: atributo Id="NFe3526..." do no infNFe
	if no := acharBloco(root, "infnfe", "infcpl"); no != nil {
		for k, v := range no.Attrs {
			switch normKey(k) {
			case "id", "idnfe", "chave", "chaveacesso":
				if d := apenasDigitos(v); len(d) == 44 {
					info.Chave = d
				}
			}
			if info.Chave != "" {
				break
			}
		}
	}
	if info.Chave == "" {
		for _, k := range []string{"chave", "chaveacesso", "chavenfe", "chavedanfe"} {
			if no := primeiroFilho(root, k); no != nil {
				if v := apenasDigitos(no.Text); len(v) == 44 {
					info.Chave = v
					break
				}
			}
		}
	}

	// identificacao
	if ide := acharBloco(root, "ide"); ide != nil {
		info.Numero = textoDe(ide, "nnf", "numero", "numeronota")
		info.Serie = textoDe(ide, "serie")
		info.EmitidaEm = dataISO(textoDe(ide, "dhemi", "demi", "dataemissao", "data"))
	}
	if info.Numero == "" {
		if no := primeiroFilhoRaso(root, 6, "nnf", "numeronf", "numeronota", "nfdocumento"); no != nil {
			info.Numero = strings.TrimSpace(no.Text)
		}
	}
	if info.Serie == "" {
		if no := primeiroFilho(root, "serie"); no != nil {
			info.Serie = strings.TrimSpace(no.Text)
		}
	}
	if info.EmitidaEm == "" {
		for _, k := range []string{"dhemi", "demi", "dataemissao", "emitidaem"} {
			if no := primeiroFilho(root, k); no != nil {
				if d := dataISO(no.Text); d != "" {
					info.EmitidaEm = d
					break
				}
			}
		}
	}

	// emitente
	if emit := acharBloco(root, "emit", "emitente", "fornecedor"); emit != nil {
		info.EmitenteNome = textoDe(emit, aliasesEmitenteNome...)
		info.EmitenteDoc = apenasDigitos(textoDe(emit, aliasesEmitenteDoc...))
	}
	if info.EmitenteNome == "" {
		if no := primeiroFilhoRaso(root, 6, aliasesEmitenteNome...); no != nil {
			info.EmitenteNome = strings.TrimSpace(no.Text)
		}
	}
	if info.EmitenteDoc == "" {
		for _, k := range []string{"cnpj", "cnpjemitente", "cpf"} {
			if no := primeiroFilhoRaso(root, 6, k); no != nil {
				if d := apenasDigitos(no.Text); len(d) == 14 || len(d) == 11 {
					info.EmitenteDoc = d
					break
				}
			}
		}
	}

	// valores: dentro de <total><ICMSTot> na NF-e
	if tot := acharBloco(root, "icmstot", "total", "totais", "totalnota"); tot != nil {
		info.TotalCents = parseMoneyBR(textoDe(tot, aliasesTotal...))
		info.ProdutosCents = parseMoneyBR(textoDe(tot, aliasesProdutos...))
		info.FreteCents = parseMoneyBR(textoDe(tot, aliasesFrete...))
	}
	if info.TotalCents == 0 {
		for _, k := range aliasesTotal {
			if no := primeiroFilhoRaso(root, 6, k); no != nil {
				if v := parseMoneyBR(no.Text); v > 0 {
					info.TotalCents = v
					break
				}
			}
		}
	}
	info.TemTotal = info.TotalCents > 0

	// duplicatas: <cobr><dup><nDup>01</nDup><dVenc>2026-11-28</dVenc><vDup>1616.66</vDup></dup>
	var dupNo func(*xmlNode)
	dupNo = func(cur *xmlNode) {
		for _, c := range cur.Children {
			if nomeDoNo(c) == "dup" || nomeDoNo(c) == "duplicata" || nomeDoNo(c) == "parcela" || nomeDoNo(c) == "boleto" {
				d := Duplicata{
					Numero:     textoDe(c, "ndup", "numero", "numeroparcela", "parcela"),
					Vencimento: dataISO(textoDe(c, "dvenc", "vencimento", "datavencimento", "datadevencimento")),
					ValorCents: parseMoneyBR(textoDe(c, "vdup", "valor", "valordocumento", "valordaparcela")),
				}
				if d.Vencimento != "" && d.ValorCents > 0 {
					info.Duplicatas = append(info.Duplicatas, d)
				}
			}
			dupNo(c)
		}
	}
	dupNo(root)

	// sem duplicata declarada mas com valor total: uma conta unica
	if len(info.Duplicatas) == 0 && info.TotalCents > 0 {
		info.Duplicatas = []Duplicata{{Numero: "1", Vencimento: "", ValorCents: info.TotalCents}}
	}

	// quanto do total nao e produto (frete, IPI, ST...)
	if info.ProdutosCents > 0 && info.TotalCents > info.ProdutosCents {
		info.ExtrasCents = info.TotalCents - info.ProdutosCents
	}
	return info
}

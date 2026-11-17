package main

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- Estoque Inteligente: API ----------

func (a *App) handleEIResumo(w http.ResponseWriter, r *http.Request) {
	cfg := a.loadConfigEI()
	resumo, err := a.calcularResumo(cfg)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, resumo)
}

func (a *App) handleEISugestoes(w http.ResponseWriter, r *http.Request) {
	cfg := a.loadConfigEI()
	sug, err := a.calcularSugestoes(cfg)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"itens": sug})
}

func (a *App) handleEIParados(w http.ResponseWriter, r *http.Request) {
	cfg := a.loadConfigEI()
	par, err := a.calcularParados(cfg)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"itens": par})
}

func (a *App) handleEIABC(w http.ResponseWriter, r *http.Request) {
	cfg := a.loadConfigEI()
	abc, err := a.calcularABC(cfg)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"itens": abc})
}

func (a *App) handleEIComparar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	de1 := q.Get("de1")
	ate1 := q.Get("ate1")
	de2 := q.Get("de2")
	ate2 := q.Get("ate2")
	if de1 == "" || ate1 == "" || de2 == "" || ate2 == "" {
		// defaults: últimos 30 dias vs 30 dias anteriores
		hoje := todayISO()
		ate1 = hoje
		de1 = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
		ate2 = time.Now().AddDate(0, 0, -31).Format("2006-01-02")
		de2 = time.Now().AddDate(0, 0, -61).Format("2006-01-02")
	}
	cmp, err := a.calcularComparacao(de1, ate1, de2, ate2)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, cmp)
}

func (a *App) handleEIConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		cfg := a.loadConfigEI()
		writeJSON(w, 200, cfg)
		return
	}
	// POST: salvar config
	var in struct {
		LeadDias      int `json:"lead_dias"`
		SegurancaDias int `json:"seguranca_dias"`
		DemandaDias   int `json:"demanda_dias"`
		ParadoDias    int `json:"parado_dias"`
		ABCDias       int `json:"abc_dias"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	// validar mínimos
	if in.LeadDias < 0 {
		in.LeadDias = 7
	}
	if in.SegurancaDias < 0 {
		in.SegurancaDias = 3
	}
	if in.DemandaDias < 1 {
		in.DemandaDias = 90
	}
	if in.ParadoDias < 1 {
		in.ParadoDias = 90
	}
	if in.ABCDias < 1 {
		in.ABCDias = 90
	}
	a.db.SetSetting("ei_lead_dias", strconv.Itoa(in.LeadDias))
	a.db.SetSetting("ei_seguranca_dias", strconv.Itoa(in.SegurancaDias))
	a.db.SetSetting("ei_demanda_dias", strconv.Itoa(in.DemandaDias))
	a.db.SetSetting("ei_parado_dias", strconv.Itoa(in.ParadoDias))
	a.db.SetSetting("ei_abc_dias", strconv.Itoa(in.ABCDias))
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- CSV export ----------

func (a *App) handleEICSV(w http.ResponseWriter, r *http.Request) {
	// extrai o nome da lista do path: /api/estoque-inteligente/sugestoes.csv -> "sugestoes"
	path := r.URL.Path
	lista := ""
	for _, name := range []string{"sugestoes", "parados", "abc"} {
		if strings.HasSuffix(path, name+".csv") {
			lista = name
			break
		}
	}
	if lista == "" {
		fail(w, 400, "lista inválida: use sugestoes, parados ou abc")
		return
	}
	cfg := a.loadConfigEI()

	var rows [][]string
	var headers []string
	var filename string

	switch lista {
	case "sugestoes":
		sug, _ := a.calcularSugestoes(cfg)
		headers = []string{"Código", "Descrição", "Marca", "Estoque", "Mínimo", "Demanda/dia", "Lead (dias)",
			"Ponto Reposição", "Est. Segurança", "Dias p/ Faltar", "Criticidade", "Qtd Sugerida",
			"Custo Unit. (R$)", "Fornecedor", "Já em Pedido Aberto"}
		for _, s := range sug {
			dias := ""
			if s.DiasAteFaltar != nil {
				dias = fmt.Sprintf("%.1f", *s.DiasAteFaltar)
			}
			forn := s.FornecedorNome
			if forn == "" {
				forn = "(sem)"
			}
			ja := "Não"
			if s.JaEmPedidoAberto {
				ja = "Sim"
			}
			rows = append(rows, []string{
				s.Codigo, s.Descricao, s.Marca,
				fmtQty(s.Estoque), fmtQty(s.MinStock),
				fmt.Sprintf("%.2f", s.DemandaDiaria),
				strconv.Itoa(s.LeadDias),
				fmt.Sprintf("%.1f", s.PontoReposicao),
				fmt.Sprintf("%.1f", s.EstoqueSeguranca),
				dias, s.Criticidade, fmtQty(s.QtdSugerida),
				fmtMoneyBR(s.CustoUnit), forn, ja,
			})
		}
		filename = "sugestoes-compra-" + todayISO() + ".csv"

	case "parados":
		par, _ := a.calcularParados(cfg)
		headers = []string{"Código", "Descrição", "Marca", "Estoque", "Custo Unit. (R$)",
			"Valor Parado (R$)", "Última Saída", "Dias Parado"}
		for _, p := range par {
			ult := ""
			if p.UltimaSaida != nil {
				ult = *p.UltimaSaida
			}
			rows = append(rows, []string{
				p.Codigo, p.Descricao, p.Marca,
				fmtQty(p.Estoque), fmtMoneyBR(p.CustoUnit),
				fmtMoneyBR(p.ValorParado), ult, strconv.Itoa(p.DiasParado),
			})
		}
		filename = "estoque-parado-" + todayISO() + ".csv"

	case "abc":
		abc, _ := a.calcularABC(cfg)
		headers = []string{"Código", "Descrição", "Marca", "Receita (R$)", "% Acumulado", "Classe"}
		for _, x := range abc {
			rows = append(rows, []string{
				x.Codigo, x.Descricao, x.Marca,
				fmtMoneyBR(x.Receita),
				fmt.Sprintf("%.1f", x.PercAcum), x.Classe,
			})
		}
		filename = "curva-abc-" + todayISO() + ".csv"

	default:
		fail(w, 400, "lista inválida: use sugestoes, parados ou abc")
		return
	}

	var b bytes.Buffer
	b.WriteString("\xEF\xBB\xBF") // BOM UTF-8
	cw := csv.NewWriter(&b)
	cw.Comma = ';'
	_ = cw.Write(headers)
	for _, r := range rows {
		row := make([]string, len(r))
		copy(row, r)
		_ = cw.Write(row)
	}
	cw.Flush()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	_, _ = w.Write(b.Bytes())
}

// ---------- PDF export (sugestões) ----------

func (a *App) handleEIPDF(w http.ResponseWriter, r *http.Request) {
	cfg := a.loadConfigEI()
	sug, _ := a.calcularSugestoes(cfg)

	t := pdfTable{
		Titulo:      "Sugestões de Compra — Estoque Inteligente",
		Subtitulo:   fmt.Sprintf("Gerado em %s — Período de demanda: %d dias", time.Now().Format("02/01/2006 15:04"), cfg.DemandaDias),
		Empresa:     a.db.GetSetting("loja_nome"),
		Rodape:      fmt.Sprintf("Gerado em %s - %s", time.Now().Format("02/01/2006 15:04"), appName),
		Colunas:     []string{"Código", "Descrição", "Estoque", "Mín", "Demanda/dia", "Lead", "Ponto Repos.", "Crítico", "Qtd", "Custo", "Fornecedor"},
		Larguras:    []float64{55, 180, 40, 35, 50, 35, 55, 40, 35, 55, 90},
		Alinhamento: []string{"L", "L", "R", "R", "R", "R", "R", "L", "R", "R", "L"},
	}
	for _, s := range sug {
		critLabel := map[string]string{"critica": "Crítica", "media": "Média", "baixa": "Baixa"}[s.Criticidade]
		if critLabel == "" {
			critLabel = "—"
		}
		forn := s.FornecedorNome
		if forn == "" {
			forn = "(sem)"
		}
		t.Linhas = append(t.Linhas, []string{
			s.Codigo, s.Descricao, fmtQty(s.Estoque), fmtQty(s.MinStock),
			fmt.Sprintf("%.2f", s.DemandaDiaria), strconv.Itoa(s.LeadDias),
			fmt.Sprintf("%.1f", s.PontoReposicao), critLabel,
			fmtQty(s.QtdSugerida), fmtMoneyBR(s.CustoUnit), forn,
		})
	}
	t.Resumo = []string{
		fmt.Sprintf("Itens com sugestão: %d", len(sug)),
		fmt.Sprintf("Custo estimado da reposição: %s", fmtMoneyBR(somaCustoSugestoes(sug))),
	}
	data := t.Build()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=sugestoes-"+todayISO()+".pdf")
	_, _ = w.Write(data)
}

// fmtMoneyBR formata centavos para "1.234,56" (sem R$) - usado em CSV
func fmtMoneyBR(c int64) string {
	sign := ""
	if c < 0 {
		sign = "-"
		c = -c
	}
	return fmt.Sprintf("%s%d,%02d", sign, c/100, c%100)
}

func somaCustoSugestoes(sug []SugestaoCompra) int64 {
	var total int64
	for _, s := range sug {
		total += int64(s.QtdSugerida * float64(s.CustoUnit))
	}
	return total
}

// ---------- Gerar Pedido de Compra ----------

type PedidoItemInput struct {
	PartID     int64   `json:"part_id"`
	Quantidade float64 `json:"quantidade"`
}

type GerarPedidoInput struct {
	Itens []PedidoItemInput `json:"itens"`
}

func (a *App) handleEIGerarPedido(w http.ResponseWriter, r *http.Request) {
	var in GerarPedidoInput
	if err := readJSON(r, &in); err != nil || len(in.Itens) == 0 {
		fail(w, 400, "informe ao menos um item")
		return
	}

	// Agrupar por fornecedor
	type itemAgg struct {
		partID  int64
		desc    string
		qty     float64
		cost    int64
		supID   *int64
		supNome string
	}
	items := []itemAgg{}
	for _, it := range in.Itens {
		var desc string
		var cost int64
		var supID sql.NullInt64
		err := a.db.QueryRow(`SELECT description, COALESCE(NULLIF(avg_cost_cents,0),cost_cents), supplier_id
			FROM parts WHERE id=?`, it.PartID).Scan(&desc, &cost, &supID)
		if err != nil {
			continue
		}
		var fornNome string
		if supID.Valid {
			a.db.QueryRow(`SELECT name FROM suppliers WHERE id=?`, supID.Int64).Scan(&fornNome)
		}
		items = append(items, itemAgg{
			partID: it.PartID, desc: desc, qty: it.Quantidade, cost: cost,
			supID: idOuNulo(supID), supNome: fornNome,
		})
	}
	if len(items) == 0 {
		fail(w, 400, "nenhum item válido")
		return
	}

	// Agrupar por fornecedor (nil = sem fornecedor)
	grupos := map[string][]itemAgg{} // key = "supID|supNome"
	for _, it := range items {
		key := "0|(sem fornecedor)"
		if it.supID != nil {
			key = fmt.Sprintf("%d|%s", *it.supID, it.supNome)
		}
		grupos[key] = append(grupos[key], it)
	}

	type pedidoCriado struct {
		ID            int64  `json:"id"`
		Fornecedor    string `json:"fornecedor"`
		Itens         int    `json:"itens"`
		TotalCentavos int64  `json:"total_centavos"`
	}
	var criados []pedidoCriado

	err := a.db.Tx(func(tx *sql.Tx) error {
		for _, gitems := range grupos {
			if len(gitems) == 0 {
				continue
			}
			// Fornecedor do grupo
			var supID *int64
			var supNome string
			if gitems[0].supID != nil {
				supID = gitems[0].supID
				supNome = gitems[0].supNome
			}

			// Somar qtd por peça (evitar duplicatas)
			somados := map[int64]*itemAgg{}
			for _, it := range gitems {
				if ex, ok := somados[it.partID]; ok {
					ex.qty += it.qty
				} else {
					copia := it
					somados[it.partID] = &copia
				}
			}

			// Criar pedido
			res, err := tx.Exec(`INSERT INTO orders(created_at,supplier_id,supplier_name,status,total_cents) VALUES(?,?,?,'aberto',0)`,
				nowISO(), supID, supNome)
			if err != nil {
				return err
			}
			orderID, _ := res.LastInsertId()
			var total int64
			for _, it := range somados {
				sub := int64(it.qty * float64(it.cost))
				total += sub
				_, err := tx.Exec(`INSERT INTO order_items(order_id,part_id,description,qty,cost_cents,falta_id)
					VALUES(?,?,?,?,?,NULL)`,
					orderID, it.partID, it.desc, it.qty, it.cost)
				if err != nil {
					return err
				}
			}
			// Atualizar total
			_, err = tx.Exec(`UPDATE orders SET total_cents=? WHERE id=?`, total, orderID)
			if err != nil {
				return err
			}
			criados = append(criados, pedidoCriado{
				ID: orderID, Fornecedor: supNome, Itens: len(somados), TotalCentavos: total,
			})
		}
		return nil
	})
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "pedidos": criados})
}

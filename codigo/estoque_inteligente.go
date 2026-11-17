package main

import (
	"database/sql"
	"math"
	"sort"
	"strconv"
	"time"
)

// ---------- Estoque Inteligente: cálculos puros (sem HTTP) ----------

// ConfigEI guarda as configurações do módulo (lidas de settings).
type ConfigEI struct {
	LeadDias      int `json:"lead_dias"`      // ei_lead_dias      default 7
	SegurancaDias int `json:"seguranca_dias"` // ei_seguranca_dias default 3
	DemandaDias   int `json:"demanda_dias"`   // ei_demanda_dias   default 90
	ParadoDias    int `json:"parado_dias"`    // ei_parado_dias    default 90
	ABCDias       int `json:"abc_dias"`       // ei_abc_dias       default 90
}

func (a *App) loadConfigEI() ConfigEI {
	get := func(k string, def int) int {
		v := a.db.GetSetting(k)
		if v == "" {
			return def
		}
		n, _ := strconv.Atoi(v)
		if n <= 0 {
			return def
		}
		return n
	}
	return ConfigEI{
		LeadDias:      get("ei_lead_dias", 7),
		SegurancaDias: get("ei_seguranca_dias", 3),
		DemandaDias:   get("ei_demanda_dias", 90),
		ParadoDias:    get("ei_parado_dias", 90),
		ABCDias:       get("ei_abc_dias", 90),
	}
}

// DemandaDiaria calcula a demanda média diária de uma peça nos últimos N dias.
// Retorna 0 se não houver vendas no período (NUNCA inventa demanda).
func (a *App) DemandaDiaria(partID int64, dias int) (float64, error) {
	if dias <= 0 {
		dias = 90
	}
	de := time.Now().AddDate(0, 0, -dias).Format("2006-01-02")
	q := `SELECT COALESCE(SUM(si.qty),0)
		FROM sale_items si
		JOIN sales s ON s.id = si.sale_id
		WHERE si.part_id = ? AND s.status = 'concluida' AND DATE(s.created_at) >= ?`
	var total float64
	err := a.db.QueryRow(q, partID, de).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total / float64(dias), nil
}

// LeadDias busca o prazo de entrega do fornecedor da peça.
// Se não houver fornecedor ou prazo=0, usa o default global.
func (a *App) LeadDias(partID int64, cfg ConfigEI) (int, error) {
	var supplierID sql.NullInt64
	err := a.db.QueryRow(`SELECT supplier_id FROM parts WHERE id=?`, partID).Scan(&supplierID)
	if err != nil || !supplierID.Valid {
		return cfg.LeadDias, err
	}
	var prazo int
	err = a.db.QueryRow(`SELECT COALESCE(prazo_entrega_dias,0) FROM suppliers WHERE id=?`, supplierID.Int64).Scan(&prazo)
	if err != nil || prazo <= 0 {
		return cfg.LeadDias, err
	}
	return prazo, nil
}

// CalculaPontoReposicao devolve ponto de reposição, estoque de segurança e dias até faltar.
func (a *App) CalculaPontoReposicao(partID int64, estoqueAtual float64, cfg ConfigEI) (pontoReposicao, estoqueSeguranca float64, diasAteFaltar *float64, leadDias int, demandaDiaria float64, err error) {
	demandaDiaria, err = a.DemandaDiaria(partID, cfg.DemandaDias)
	if err != nil {
		return 0, 0, nil, 0, 0, err
	}
	leadDias, err = a.LeadDias(partID, cfg)
	if err != nil {
		return 0, 0, nil, 0, 0, err
	}
	estoqueSeguranca = demandaDiaria * float64(cfg.SegurancaDias)
	pontoReposicao = demandaDiaria * float64(leadDias+cfg.SegurancaDias)
	if demandaDiaria > 0 {
		d := estoqueAtual / demandaDiaria
		diasAteFaltar = &d
	}
	return
}

// Criticidade classifica a peça: "critica", "media", "baixa" ou "".
func Criticidade(estoqueAtual, minStock, pontoReposicao float64, diasAteFaltar *float64, leadDias int, demandaDiaria float64) string {
	if estoqueAtual <= 0 {
		return "critica"
	}
	if demandaDiaria > 0 && diasAteFaltar != nil && *diasAteFaltar <= float64(leadDias) {
		return "critica"
	}
	if demandaDiaria > 0 && estoqueAtual <= pontoReposicao {
		return "media"
	}
	if estoqueAtual < minStock {
		return "baixa"
	}
	return ""
}

// QtdSugerida calcula a quantidade sugerida de compra.
func QtdSugerida(estoqueAtual, minStock, pontoReposicao float64, demandaDiaria float64) float64 {
	if demandaDiaria > 0 {
		alvo := math.Max(pontoReposicao, minStock)
		q := math.Ceil(alvo - estoqueAtual)
		if q < 1 {
			q = 1
		}
		return q
	}
	// Sem histórico: só regra do mínimo
	q := minStock - estoqueAtual
	if q < 1 {
		q = 1
	}
	return q
}

// ---------- Sugestões de compra ----------

type SugestaoCompra struct {
	PartID           int64    `json:"part_id"`
	Codigo           string   `json:"codigo"`
	Descricao        string   `json:"descricao"`
	Marca            string   `json:"marca"`
	Estoque          float64  `json:"estoque"`
	MinStock         float64  `json:"min_stock"`
	DemandaDiaria    float64  `json:"demanda_diaria"`
	LeadDias         int      `json:"lead_dias"`
	PontoReposicao   float64  `json:"ponto_reposicao"`
	EstoqueSeguranca float64  `json:"estoque_seguranca"`
	DiasAteFaltar    *float64 `json:"dias_ate_faltar,omitempty"`
	Criticidade      string   `json:"criticidade"`
	QtdSugerida      float64  `json:"qtd_sugerida"`
	CustoUnit        int64    `json:"custo_unit_centavos"`
	FornecedorID     *int64   `json:"fornecedor_id,omitempty"`
	FornecedorNome   string   `json:"fornecedor_nome"`
	JaEmPedidoAberto bool     `json:"ja_em_pedido_aberto"`
}

func (a *App) calcularSugestoes(cfg ConfigEI) ([]SugestaoCompra, error) {
	rows, err := a.db.Query(`SELECT id,code,description,brand,stock,min_stock,
		COALESCE(NULLIF(avg_cost_cents,0),cost_cents),supplier_id
		FROM parts WHERE active=1 AND (stock<=min_stock OR stock>0)
		ORDER BY (min_stock-stock) DESC, description`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Buscar quais peças já estão em pedido aberto (para badge)
	emPedido := map[int64]bool{}
	pr, err := a.db.Query(`SELECT DISTINCT oi.part_id FROM order_items oi
		JOIN orders o ON o.id=oi.order_id WHERE o.status='aberto' AND oi.part_id IS NOT NULL`)
	if err == nil {
		for pr.Next() {
			var pid int64
			if pr.Scan(&pid) == nil {
				emPedido[pid] = true
			}
		}
		pr.Close()
	}

	var out []SugestaoCompra
	for rows.Next() {
		var id int64
		var code, desc, brand string
		var stock, minStock float64
		var cost int64
		var supID sql.NullInt64
		if rows.Scan(&id, &code, &desc, &brand, &stock, &minStock, &cost, &supID) != nil {
			continue
		}

		ponto, seg, dias, lead, demanda, err := a.CalculaPontoReposicao(id, stock, cfg)
		if err != nil {
			continue
		}
		crit := Criticidade(stock, minStock, ponto, dias, lead, demanda)
		if crit == "" && stock > minStock {
			continue // só lista se critica/media/baixa
		}
		qtd := QtdSugerida(stock, minStock, ponto, demanda)

		var fornNome string
		if supID.Valid {
			a.db.QueryRow(`SELECT name FROM suppliers WHERE id=?`, supID.Int64).Scan(&fornNome)
		}

		out = append(out, SugestaoCompra{
			PartID:           id,
			Codigo:           code,
			Descricao:        desc,
			Marca:            brand,
			Estoque:          stock,
			MinStock:         minStock,
			DemandaDiaria:    demanda,
			LeadDias:         lead,
			PontoReposicao:   ponto,
			EstoqueSeguranca: seg,
			DiasAteFaltar:    dias,
			Criticidade:      crit,
			QtdSugerida:      qtd,
			CustoUnit:        cost,
			FornecedorID:     idOuNulo(supID),
			FornecedorNome:   fornNome,
			JaEmPedidoAberto: emPedido[id],
		})
	}
	// Ordenar: critica > media > baixa, depois por estoque mais crítico
	sort.Slice(out, func(i, j int) bool {
		ord := map[string]int{"critica": 0, "media": 1, "baixa": 2}
		oi, oj := ord[out[i].Criticidade], ord[out[j].Criticidade]
		if oi != oj {
			return oi < oj
		}
		// mais crítico primeiro (menor estoque relativo ao ponto)
		ri := out[i].Estoque / math.Max(1, out[i].PontoReposicao)
		rj := out[j].Estoque / math.Max(1, out[j].PontoReposicao)
		return ri < rj
	})
	return out, nil
}

// ---------- Estoque Parado ----------

type ItemParado struct {
	PartID      int64   `json:"part_id"`
	Codigo      string  `json:"codigo"`
	Descricao   string  `json:"descricao"`
	Marca       string  `json:"marca"`
	Estoque     float64 `json:"estoque"`
	CustoUnit   int64   `json:"custo_unit_centavos"`
	ValorParado int64   `json:"valor_parado_centavos"`
	UltimaSaida *string `json:"ultima_saida,omitempty"`
	DiasParado  int     `json:"dias_parado"`
}

func (a *App) calcularParados(cfg ConfigEI) ([]ItemParado, error) {
	_ = time.Now().AddDate(0, 0, -cfg.ParadoDias).Format("2006-01-02 15:04:05") // threshold (unused directly)
	// Peças com estoque > 0
	rows, err := a.db.Query(`SELECT id,code,description,brand,stock,
		COALESCE(NULLIF(avg_cost_cents,0),cost_cents),created_at
		FROM parts WHERE active=1 AND stock>0 ORDER BY description`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ItemParado
	for rows.Next() {
		var id int64
		var code, desc, brand, created string
		var stock float64
		var cost int64
		if rows.Scan(&id, &code, &desc, &brand, &stock, &cost, &created) != nil {
			continue
		}
		// Última saída (stock_moves kind != 'entrada' OU venda via sale_items)
		var ultimaSaida sql.NullString
		// stock_moves
		a.db.QueryRow(`SELECT MAX(created_at) FROM stock_moves
			WHERE part_id=? AND kind!='entrada'`, id).Scan(&ultimaSaida)
		// vendas (sale_items de vendas concluídas)
		var ultVenda sql.NullString
		a.db.QueryRow(`SELECT MAX(s.created_at) FROM sale_items si
			JOIN sales s ON s.id=si.sale_id
			WHERE si.part_id=? AND s.status='concluida'`, id).Scan(&ultVenda)
		// escolher a mais recente
		var ultima time.Time
		has := false
		if ultimaSaida.Valid {
			if t, ok := dataLocal(ultimaSaida.String); ok {
				ultima = t
				has = true
			}
		}
		if ultVenda.Valid {
			if t, ok := dataLocal(ultVenda.String); ok {
				if !has || t.After(ultima) {
					ultima = t
					has = true
				}
			}
		}
		diasParado := 0
		if has {
			diasParado = int(time.Since(ultima).Hours() / 24)
		} else {
			// nunca teve saída: usar created_at
			if t, ok := dataLocal(created); ok {
				diasParado = int(time.Since(t).Hours() / 24)
			}
		}
		if diasParado >= cfg.ParadoDias {
			valor := int64(stock * float64(cost))
			var us *string
			if has {
				s := ultima.Format("2006-01-02")
				us = &s
			}
			out = append(out, ItemParado{
				PartID:      id,
				Codigo:      code,
				Descricao:   desc,
				Marca:       brand,
				Estoque:     stock,
				CustoUnit:   cost,
				ValorParado: valor,
				UltimaSaida: us,
				DiasParado:  diasParado,
			})
		}
	}
	// Ordenar por valor parado desc
	sort.Slice(out, func(i, j int) bool { return out[i].ValorParado > out[j].ValorParado })
	return out, nil
}

// ---------- Curva ABC ----------

type ItemABC struct {
	PartID    int64   `json:"part_id"`
	Codigo    string  `json:"codigo"`
	Descricao string  `json:"descricao"`
	Marca     string  `json:"marca"`
	Receita   int64   `json:"receita_centavos"`
	PercAcum  float64 `json:"perc_acumulado"`
	Classe    string  `json:"classe"` // A, B, C
}

func (a *App) calcularABC(cfg ConfigEI) ([]ItemABC, error) {
	de := time.Now().AddDate(0, 0, -cfg.ABCDias).Format("2006-01-02")
	rows, err := a.db.Query(`SELECT si.part_id, p.code, p.description, p.brand,
		CAST(SUM(si.total_cents) AS INTEGER) as receita
		FROM sale_items si
		JOIN sales s ON s.id=si.sale_id
		JOIN parts p ON p.id=si.part_id
		WHERE s.status='concluida' AND DATE(s.created_at) >= ?
		GROUP BY si.part_id, p.code, p.description, p.brand
		HAVING receita > 0
		ORDER BY receita DESC`, de)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type raw struct {
		id, receita       int64
		code, desc, brand string
	}
	var list []raw
	var totalReceita int64
	for rows.Next() {
		var r raw
		if rows.Scan(&r.id, &r.code, &r.desc, &r.brand, &r.receita) == nil {
			list = append(list, r)
			totalReceita += r.receita
		}
	}
	if totalReceita == 0 {
		return []ItemABC{}, nil
	}
	var out []ItemABC
	acum := int64(0)
	for _, r := range list {
		percAntes := float64(acum) * 100.0 / float64(totalReceita)
		acum += r.receita
		perc := float64(acum) * 100.0 / float64(totalReceita)
		classe := "C"
		if percAntes < 80 {
			classe = "A"
		} else if percAntes < 95 {
			classe = "B"
		}
		out = append(out, ItemABC{
			PartID:    r.id,
			Codigo:    r.code,
			Descricao: r.desc,
			Marca:     r.brand,
			Receita:   r.receita,
			PercAcum:  perc,
			Classe:    classe,
		})
	}
	return out, nil
}

// ---------- Comparar Períodos ----------

type PeriodoComparado struct {
	De          string `json:"de"`
	Ate         string `json:"ate"`
	Receita     int64  `json:"receita_centavos"`
	Custo       int64  `json:"custo_centavos"`
	Margem      int64  `json:"margem_centavos"`
	Vendas      int    `json:"vendas"`
	Itens       int    `json:"itens_vendidos"`
	TicketMedio int64  `json:"ticket_medio_centavos"`
}

type TopCrescimento struct {
	PartID      int64   `json:"part_id"`
	Codigo      string  `json:"codigo"`
	Descricao   string  `json:"descricao"`
	Receita1    int64   `json:"receita_p1_centavos"`
	Receita2    int64   `json:"receita_p2_centavos"`
	Crescimento float64 `json:"crescimento_pct"`
}

type ComparacaoPeriodos struct {
	Periodo1       PeriodoComparado `json:"periodo1"`
	Periodo2       PeriodoComparado `json:"periodo2"`
	TopCrescimento []TopCrescimento `json:"top_crescimento"`
}

func (a *App) calcularComparacao(de1, ate1, de2, ate2 string) (ComparacaoPeriodos, error) {
	p1 := a.calcularPeriodo(de1, ate1)
	p2 := a.calcularPeriodo(de2, ate2)

	// Top 10 crescimento de receita (peças que mais cresceram de p1 para p2)
	// Pegar receita por peça em cada período
	receitas := func(de, ate string) map[int64]struct {
		code, desc string
		receita    int64
	} { m := map[int64]struct {
		code, desc string
		receita    int64
	}{}; rows, err := a.db.Query(`SELECT si.part_id, p.code, p.description,
			CAST(SUM(si.total_cents) AS INTEGER)
			FROM sale_items si
			JOIN sales s ON s.id=si.sale_id
			JOIN parts p ON p.id=si.part_id
			WHERE s.status='concluida' AND DATE(s.created_at) BETWEEN ? AND ?
			GROUP BY si.part_id, p.code, p.description`, de, ate); if err != nil {
		return m
	}; defer rows.Close(); for rows.Next() {
		var id int64
		var code, desc string
		var r int64
		if rows.Scan(&id, &code, &desc, &r) == nil {
			m[id] = struct {
				code, desc string
				receita    int64
			}{code, desc, r}
		}
	}; return m }
	r1 := receitas(de1, ate1)
	r2 := receitas(de2, ate2)

	type cresc struct {
		id         int64
		code, desc string
		r1, r2     int64
		pct        float64
	}
	var lista []cresc
	for id, v2 := range r2 {
		v1 := r1[id]
		if v1.receita == 0 && v2.receita == 0 {
			continue
		}
		var pct float64
		if v1.receita > 0 {
			pct = float64(v2.receita-v1.receita) * 100.0 / float64(v1.receita)
		} else {
			pct = 100.0 // nova peça
		}
		lista = append(lista, cresc{id, v2.code, v2.desc, v1.receita, v2.receita, pct})
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].pct > lista[j].pct })
	if len(lista) > 10 {
		lista = lista[:10]
	}
	top := make([]TopCrescimento, len(lista))
	for i, c := range lista {
		top[i] = TopCrescimento{
			PartID: c.id, Codigo: c.code, Descricao: c.desc,
			Receita1: c.r1, Receita2: c.r2, Crescimento: c.pct,
		}
	}
	return ComparacaoPeriodos{Periodo1: p1, Periodo2: p2, TopCrescimento: top}, nil
}

func (a *App) calcularPeriodo(de, ate string) PeriodoComparado {
	rows, err := a.db.Query(`SELECT COUNT(DISTINCT s.id), SUM(s.total_cents), SUM(s.cost_cents), COUNT(si.id)
		FROM sales s
		LEFT JOIN sale_items si ON si.sale_id = s.id
		WHERE s.status='concluida' AND DATE(s.created_at) BETWEEN ? AND ?`, de, ate)
	var vendas, itens int
	var receita, custo int64
	if err == nil {
		defer rows.Close()
		if rows.Next() {
			rows.Scan(&vendas, &receita, &custo, &itens)
		}
	}
	margem := receita - custo
	var ticket int64
	if vendas > 0 {
		ticket = receita / int64(vendas)
	}
	return PeriodoComparado{
		De: de, Ate: ate, Receita: receita, Custo: custo,
		Margem: margem, Vendas: vendas, Itens: itens, TicketMedio: ticket,
	}
}

// ---------- Resumo (cards do dashboard) ----------

type ResumoEI struct {
	TotalPecas        int            `json:"total_pecas"`
	PecasCriticas     int            `json:"pecas_criticas"`
	PecasMedia        int            `json:"pecas_media"`
	PecasBaixa        int            `json:"pecas_baixa"`
	ValorParado       int64          `json:"valor_parado_centavos"`
	ItensParados      int            `json:"itens_parados"`
	ClassesABC        map[string]int `json:"classes_abc"`
	SugestoesCount    int            `json:"sugestoes_count"`
	CustoEstimadoRepo int64          `json:"custo_estimado_reposicao_centavos"`
}

func (a *App) calcularResumo(cfg ConfigEI) (ResumoEI, error) {
	sug, _ := a.calcularSugestoes(cfg)
	par, _ := a.calcularParados(cfg)
	abc, _ := a.calcularABC(cfg)

	classes := map[string]int{"A": 0, "B": 0, "C": 0}
	for _, x := range abc {
		classes[x.Classe]++
	}
	var valorParado int64
	for _, p := range par {
		valorParado += p.ValorParado
	}
	var custoRepo int64
	var crit, med, baixa int
	for _, s := range sug {
		custoRepo += int64(s.QtdSugerida * float64(s.CustoUnit))
		switch s.Criticidade {
		case "critica":
			crit++
		case "media":
			med++
		case "baixa":
			baixa++
		}
	}
	return ResumoEI{
		TotalPecas:        len(sug) + 1, // placeholder
		PecasCriticas:     crit,
		PecasMedia:        med,
		PecasBaixa:        baixa,
		ValorParado:       valorParado,
		ItensParados:      len(par),
		ClassesABC:        classes,
		SugestoesCount:    len(sug),
		CustoEstimadoRepo: custoRepo,
	}, nil
}

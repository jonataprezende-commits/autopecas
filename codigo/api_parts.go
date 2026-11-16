package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Equiv struct {
	ID     int64  `json:"id"`
	Codigo string `json:"codigo"`
}

type Part struct {
	ID           int64   `json:"id"`
	Codigo       string  `json:"codigo"`
	CodFab       string  `json:"codigo_fabricante"`
	Descricao    string  `json:"descricao"`
	Marca        string  `json:"marca"`
	Categoria    string  `json:"categoria"`
	Custo        int64   `json:"custo_centavos"`
	CustoMedio   int64   `json:"custo_medio_centavos"`
	Preco        int64   `json:"preco_centavos"`
	Minimo       float64 `json:"minimo"`
	Estoque      float64 `json:"estoque"`
	Local        string  `json:"local"`
	FornecedorID *int64  `json:"fornecedor_id"`
	Fornecedor   string  `json:"fornecedor_nome"`
	Ativo        bool    `json:"ativo"`
	Criado       string  `json:"criado_em"`
	Atualizado   string  `json:"atualizado_em"`
	Aplicacao    string  `json:"aplicacao"`
	Equivalentes []Equiv `json:"equivalentes"`
	AbaixoMinimo bool    `json:"abaixo_minimo"`
}

const partCols = `p.id,p.code,p.mfr_code,p.description,p.brand,p.category,IFNULL(p.application,''),
p.cost_cents,p.avg_cost_cents,p.price_cents,p.min_stock,p.stock,p.location,p.supplier_id,IFNULL(s.name,''),p.active,
p.created_at,p.updated_at`

func scanPart(rows interface{ Scan(...any) error }) (Part, error) {
	var p Part
	var ativo int
	err := rows.Scan(&p.ID, &p.Codigo, &p.CodFab, &p.Descricao, &p.Marca, &p.Categoria, &p.Aplicacao,
		&p.Custo, &p.CustoMedio, &p.Preco, &p.Minimo, &p.Estoque, &p.Local, &p.FornecedorID, &p.Fornecedor, &ativo, &p.Criado, &p.Atualizado)
	p.Ativo = ativo == 1
	p.AbaixoMinimo = p.Estoque <= p.Minimo
	p.Equivalentes = []Equiv{}
	return p, err
}

func (a *App) loadEquivs(partIDs []int64) map[int64][]Equiv {
	out := map[int64][]Equiv{}
	if len(partIDs) == 0 {
		return out
	}
	marks := make([]string, len(partIDs))
	args := make([]any, len(partIDs))
	for i, id := range partIDs {
		marks[i] = "?"
		args[i] = id
	}
	rows, err := a.db.Query(`SELECT id,part_id,code FROM equivalents WHERE part_id IN (`+strings.Join(marks, ",")+`) ORDER BY code`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, pid int64
		var code string
		if err := rows.Scan(&id, &pid, &code); err == nil {
			out[pid] = append(out[pid], Equiv{ID: id, Codigo: code})
		}
	}
	return out
}

// montarBuscaPartes monta o SELECT da aba Pecas para um conjunto de termos.
func montarBuscaPartes(q, cat string, onlyLow bool, limit int, termos []string) (string, []any) {
	var where []string
	var args []any
	if !onlyLow {
		where = append(where, "p.active=1")
	}
	if cat != "" {
		where = append(where, "p.category=?")
		args = append(args, cat)
	}
	if onlyLow {
		where = append(where, "p.active=1 AND p.stock<=p.min_stock")
	}
	order := "p.description COLLATE NOCASE"
	rankSelect := "0 AS rank"
	for _, t := range termos {
		where = append(where, "(p.search_blob LIKE ? OR p.code_norm LIKE ?)")
		args = append(args, "%"+t+"%", "%"+strings.ReplaceAll(t, " ", "")+"%")
	}
	if q != "" {
		nq := normCode(q)
		if nq != "" {
			rankSelect = (`CASE
			  WHEN UPPER(REPLACE(p.code,' ',''))=? THEN 0
			  WHEN UPPER(REPLACE(IFNULL(p.mfr_code,''),' ',''))=? THEN 1
			  WHEN EXISTS(SELECT 1 FROM equivalents e WHERE e.part_id=p.id AND e.code_norm=?) THEN 2
			  WHEN p.code LIKE ? THEN 3
			  WHEN IFNULL(p.mfr_code,'') LIKE ? THEN 4
			  ELSE 5 END AS rank`)
			args = append([]any{nq, nq, nq, nq + "%", nq + "%"}, args...)
		}
		order = "rank, p.description COLLATE NOCASE"
	}
	sqlText := `SELECT ` + partCols + `, ` + rankSelect + ` FROM parts p LEFT JOIN suppliers s ON s.id=p.supplier_id`
	if len(where) > 0 {
		sqlText += " WHERE " + strings.Join(where, " AND ")
	}
	sqlText += " ORDER BY " + order + " LIMIT ?"
	args = append(args, limit)
	return sqlText, args
}

func (a *App) handleListParts(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	cat := strings.TrimSpace(r.URL.Query().Get("categoria"))
	onlyLow := r.URL.Query().Get("abaixo_minimo") == "1"
	limit := 200
	if l := r.URL.Query().Get("limite"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}

	termos := strings.Fields(normText(q))
	// 1a tentativa: todos os termos. Se nao achar nada e a busca tinha ano
	// ("gol 2012"), repete sem o ano - a aplicacao pode estar em faixa (08/16).
	tentativas := [][]string{termos}
	if fortes, anos := separarAnos(termos); len(anos) > 0 && len(fortes) > 0 {
		tentativas = append(tentativas, fortes)
	}

	list := []Part{}
	ids := []int64{}
	var errBusca error
	for _, conjunto := range tentativas {
		sqlText, args := montarBuscaPartes(q, cat, onlyLow, limit, conjunto)
		rows, err := a.db.Query(sqlText, args...)
		if err != nil {
			fail(w, 500, "erro na busca: "+err.Error())
			return
		}
		list = []Part{}
		ids = []int64{}
		for rows.Next() {
			var p Part
			var ativo int
			var rank int
			if err := rows.Scan(&p.ID, &p.Codigo, &p.CodFab, &p.Descricao, &p.Marca, &p.Categoria, &p.Aplicacao,
				&p.Custo, &p.CustoMedio, &p.Preco, &p.Minimo, &p.Estoque, &p.Local, &p.FornecedorID, &p.Fornecedor, &ativo, &p.Criado, &p.Atualizado, &rank); err != nil {
				errBusca = err
				break
			}
			p.Ativo = ativo == 1
			p.AbaixoMinimo = p.Estoque <= p.Minimo
			p.Equivalentes = []Equiv{}
			list = append(list, p)
			ids = append(ids, p.ID)
		}
		rows.Close()
		if errBusca != nil {
			fail(w, 500, errBusca.Error())
			return
		}
		if len(list) > 0 {
			break
		}
	}

	eq := a.loadEquivs(ids)
	for i := range list {
		if e, ok := eq[list[i].ID]; ok {
			list[i].Equivalentes = filtrarEquivalentes(e, list[i].Codigo, list[i].CodFab)
		}
	}
	writeJSON(w, 200, map[string]any{"itens": list, "total": len(list)})
}

func (a *App) getPart(id int64) (*Part, error) {
	row := a.db.QueryRow(`SELECT `+partCols+` FROM parts p LEFT JOIN suppliers s ON s.id=p.supplier_id WHERE p.id=?`, id)
	p, err := scanPart(row)
	if err != nil {
		return nil, err
	}
	p.Equivalentes = filtrarEquivalentes(a.loadEquivs([]int64{id})[id], p.Codigo, p.CodFab)
	if p.Equivalentes == nil {
		p.Equivalentes = []Equiv{}
	}
	return &p, nil
}

func (a *App) handleGetPart(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, err := a.getPart(id)
	if err != nil {
		fail(w, 404, "peca nao encontrada")
		return
	}
	writeJSON(w, 200, p)
}

type PartInput struct {
	Codigo       string   `json:"codigo"`
	CodFab       string   `json:"codigo_fabricante"`
	Descricao    string   `json:"descricao"`
	Marca        string   `json:"marca"`
	Categoria    string   `json:"categoria"`
	Aplicacao    string   `json:"aplicacao"`
	Custo        int64    `json:"custo_centavos"`
	Preco        int64    `json:"preco_centavos"`
	Minimo       float64  `json:"minimo"`
	Estoque      float64  `json:"estoque"`
	Local        string   `json:"local"`
	FornecedorID *int64   `json:"fornecedor_id"`
	Ativo        *bool    `json:"ativo"`
	Equivalentes []string `json:"equivalentes"`
}

func (a *App) handleSavePart(w http.ResponseWriter, r *http.Request) {
	var in PartInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	in.Descricao = strings.TrimSpace(in.Descricao)
	in.Codigo = strings.TrimSpace(in.Codigo)
	in.CodFab = strings.TrimSpace(in.CodFab)
	if in.Descricao == "" {
		fail(w, 400, "informe a descricao da peca")
		return
	}
	// 1.0.10: valores impossiveis nao entram no banco (preco/custo/estoque
	// negativos eram aceitos pela rota e apareciam no estoque e no lucro).
	if in.Preco < 0 {
		fail(w, 400, "o preço de venda não pode ser negativo")
		return
	}
	if in.Custo < 0 {
		fail(w, 400, "o custo da peça não pode ser negativo")
		return
	}
	if in.Minimo < 0 {
		fail(w, 400, "o estoque mínimo não pode ser negativo")
		return
	}
	if in.Estoque < 0 {
		fail(w, 400, "o estoque não pode ser negativo (use o ajuste de estoque para corrigir o saldo)")
		return
	}
	if in.Preco > maxPrecoCentavos || in.Custo > maxPrecoCentavos {
		fail(w, 400, "valor acima do limite de R$ 10.000.000,00 — confira se não faltou a vírgula")
		return
	}
	if in.Estoque > maxQtdItem || in.Minimo > maxQtdItem {
		fail(w, 400, "quantidade acima do limite de 1.000.000")
		return
	}
	if in.Codigo == "" {
		// gera codigo interno automatico
		var n int64
		_ = a.db.QueryRow(`SELECT IFNULL(MAX(id),0)+1 FROM parts`).Scan(&n)
		in.Codigo = fmt.Sprintf("INT-%05d", n)
	}

	id := int64(0)
	if s := r.PathValue("id"); s != "" {
		id, _ = strconv.ParseInt(s, 10, 64)
	}
	// 1.0.10: editar uma peca que nao existe mais respondia 200 com corpo
	// vazio (parecia salvo). Agora responde 404.
	if id != 0 {
		var existe int64
		if err := a.db.QueryRow(`SELECT id FROM parts WHERE id=?`, id).Scan(&existe); err != nil || existe == 0 {
			fail(w, 404, "peça não encontrada (talvez tenha sido excluída em outra aba)")
			return
		}
	}

	var existingID int64
	err := a.db.QueryRow(`SELECT id FROM parts WHERE code_norm=?`, normCode(in.Codigo)).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		fail(w, 500, err.Error())
		return
	}
	if existingID != 0 && existingID != id {
		fail(w, 409, "ja existe outra peca com o codigo "+in.Codigo)
		return
	}

	ativo := 1
	if in.Ativo != nil && !*in.Ativo {
		ativo = 0
	}
	equivs := cleanEquivs(in.Equivalentes)
	blob := buildSearchBlobApp(in.Codigo, in.CodFab, in.Descricao, in.Marca, in.Categoria, in.Local, in.Aplicacao, equivs)

	err = a.db.Tx(func(tx *sql.Tx) error {
		if id == 0 {
			res, err := tx.Exec(`INSERT INTO parts(code,code_norm,mfr_code,mfr_norm,description,brand,category,application,cost_cents,avg_cost_cents,price_cents,min_stock,stock,location,supplier_id,active,search_blob,created_at,updated_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, in.Codigo, normCode(in.Codigo), in.CodFab, normCode(in.CodFab), in.Descricao, in.Marca, in.Categoria,
				in.Aplicacao, in.Custo, in.Custo, in.Preco, in.Minimo, in.Estoque, in.Local, in.FornecedorID, ativo, blob, nowISO(), nowISO())
			if err != nil {
				return err
			}
			id, _ = res.LastInsertId()
			if in.Estoque != 0 {
				if _, err := tx.Exec(`INSERT INTO stock_moves(part_id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,ref_id,created_at)
					VALUES(?,?,?,?,?,?,?,?,?)`, id, "entrada", in.Estoque, in.Estoque, in.Custo, "estoque inicial", "cadastro", id, nowISO()); err != nil {
					return err
				}
			}
		} else {
			// mantem o estoque atual; ajuste de estoque e feito pelo modulo de estoque
			var atual float64
			var avg int64
			if err := tx.QueryRow(`SELECT stock,avg_cost_cents FROM parts WHERE id=?`, id).Scan(&atual, &avg); err != nil {
				return err
			}
			// edicao manual nao recalcula a media (nao houve compra). So preenche
			// quando a peca ainda nao tem media registrada.
			if avg == 0 && in.Custo > 0 {
				avg = in.Custo
			}
			if _, err := tx.Exec(`UPDATE parts SET code=?,code_norm=?,mfr_code=?,mfr_norm=?,description=?,brand=?,category=?,application=?,cost_cents=?,avg_cost_cents=?,price_cents=?,min_stock=?,location=?,supplier_id=?,active=?,search_blob=?,updated_at=? WHERE id=?`,
				in.Codigo, normCode(in.Codigo), in.CodFab, normCode(in.CodFab), in.Descricao, in.Marca, in.Categoria, in.Aplicacao, in.Custo, avg, in.Preco, in.Minimo, in.Local, in.FornecedorID, ativo, blob, nowISO(), id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`DELETE FROM equivalents WHERE part_id=?`, id); err != nil {
			return err
		}
		for _, e := range equivs {
			if _, err := tx.Exec(`INSERT INTO equivalents(part_id,code,code_norm) VALUES(?,?,?)`, id, e, normCode(e)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	p, _ := a.getPart(id)
	writeJSON(w, 200, p)
}

// filtrarEquivalentes remove da lista o proprio codigo da peca e o codigo do fabricante
func filtrarEquivalentes(eq []Equiv, codigo, codFab string) []Equiv {
	proibidos := map[string]bool{normCode(codigo): true, normCode(codFab): true}
	out := []Equiv{}
	for _, e := range eq {
		if proibidos[normCode(e.Codigo)] {
			continue
		}
		out = append(out, e)
	}
	return out
}

func cleanEquivs(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		n := normCode(e)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, strings.ToUpper(e))
	}
	return out
}

func (a *App) handleDeletePart(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	// desativa em vez de apagar (mantem historico de vendas)
	if _, err := a.db.Exec(`UPDATE parts SET active=0, updated_at=? WHERE id=?`, nowISO(), id); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handlePartHistory(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	rows, err := a.db.Query(`SELECT id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,IFNULL(ref_id,0),created_at FROM stock_moves WHERE part_id=? ORDER BY id DESC LIMIT 200`, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, uc, refid int64
		var kind, reason, refType, created string
		var delta, bal float64
		if err := rows.Scan(&id, &kind, &delta, &bal, &uc, &reason, &refType, &refid, &created); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "tipo": kind, "quantidade": delta, "saldo": bal,
			"custo_centavos": uc, "motivo": reason, "origem": refType, "origem_id": refid, "data": created,
		})
	}
	writeJSON(w, 200, map[string]any{"itens": out})
}

// ---------- dashboard ----------

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	today := todayISO()
	month := time.Now().Format("2006-01")

	var vendasHoje, qtdVendasHoje int64
	var totalVendasHoje int64
	_ = a.db.QueryRow(`SELECT IFNULL(SUM(total_cents),0), COUNT(*) FROM sales WHERE status='concluida' AND DATE(created_at)=?`, today).Scan(&totalVendasHoje, &qtdVendasHoje)
	vendasHoje = totalVendasHoje

	var totalMes int64
	_ = a.db.QueryRow(`SELECT IFNULL(SUM(total_cents),0) FROM sales WHERE status='concluida' AND substr(created_at,1,7)=?`, month).Scan(&totalMes)

	var lucroMes int64
	_ = a.db.QueryRow(`SELECT IFNULL(SUM(total_cents-cost_cents),0) FROM sales WHERE status='concluida' AND substr(created_at,1,7)=?`, month).Scan(&lucroMes)

	// alertas de estoque minimo
	rows, err := a.db.Query(`SELECT ` + partCols + ` FROM parts p LEFT JOIN suppliers s ON s.id=p.supplier_id
		WHERE p.active=1 AND p.stock<=p.min_stock ORDER BY (p.min_stock-p.stock) DESC, p.description LIMIT 200`)
	baixo := []Part{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			p, err := scanPart(rows)
			if err != nil {
				break
			}
			baixo = append(baixo, p)
		}
	}
	var qtdBaixo int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM parts WHERE active=1 AND stock<=min_stock`).Scan(&qtdBaixo)

	var fichasAbertas int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM faltas WHERE status IN ('aberta','pedida')`).Scan(&fichasAbertas)
	var fichasAbertasAntigas int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM faltas WHERE status IN ('aberta','pedida') AND DATE(created_at) <= DATE('now','localtime','-7 days')`).Scan(&fichasAbertasAntigas)
	var pedidosAbertos int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM orders WHERE status='aberto'`).Scan(&pedidosAbertos)
	var recebVencidas int64
	var qtdVencidas int64
	_ = a.db.QueryRow(`SELECT IFNULL(SUM(amount_cents),0), COUNT(*) FROM receivables WHERE status='aberto' AND due_date < ?`, today).Scan(&recebVencidas, &qtdVencidas)
	var recebAberto int64
	_ = a.db.QueryRow(`SELECT IFNULL(SUM(amount_cents),0) FROM receivables WHERE status='aberto'`).Scan(&recebAberto)

	// contas a pagar (fornecedor)
	var pagasVencidas, qtdPagVencidas, pagasAberto int64
	_ = a.db.QueryRow(`SELECT IFNULL(SUM(amount_cents),0), COUNT(*) FROM payables WHERE status='aberto' AND due_date < ?`, today).Scan(&pagasVencidas, &qtdPagVencidas)
	_ = a.db.QueryRow(`SELECT IFNULL(SUM(amount_cents),0) FROM payables WHERE status='aberto'`).Scan(&pagasAberto)

	var estoqueValor int64
	_ = a.db.QueryRow(`SELECT CAST(ROUND(IFNULL(SUM(stock*COALESCE(NULLIF(avg_cost_cents,0),cost_cents)),0)) AS INTEGER) FROM parts WHERE active=1`).Scan(&estoqueValor)
	var totalPecas int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM parts WHERE active=1`).Scan(&totalPecas)

	lastBackup := a.db.GetSetting("ultimo_backup")

	// ultimas vendas
	vrows, err := a.db.Query(`SELECT id,total_cents,payment,customer_name,created_at,status FROM sales ORDER BY id DESC LIMIT 8`)
	ultimas := []map[string]any{}
	if err == nil {
		defer vrows.Close()
		for vrows.Next() {
			var id, total int64
			var pay, cust, created, status string
			if vrows.Scan(&id, &total, &pay, &cust, &created, &status) == nil {
				ultimas = append(ultimas, map[string]any{"id": id, "total_centavos": total, "forma_pagamento": pay, "cliente": cust, "data": created, "status": status})
			}
		}
	}

	writeJSON(w, 200, map[string]any{
		"vendas_hoje_centavos":         vendasHoje,
		"qtd_vendas_hoje":              qtdVendasHoje,
		"vendas_mes_centavos":          totalMes,
		"lucro_mes_centavos":           lucroMes,
		"abaixo_minimo":                baixo,
		"qtd_abaixo_minimo":            qtdBaixo,
		"fichas_abertas":               fichasAbertas,
		"fichas_antigas":               fichasAbertasAntigas,
		"pedidos_abertos":              pedidosAbertos,
		"recebiveis_vencidos_centavos": recebVencidas,
		"recebiveis_vencidos_qtd":      qtdVencidas,
		"recebiveis_aberto_centavos":   recebAberto,
		"pagaveis_vencidos_centavos":   pagasVencidas,
		"pagaveis_vencidos_qtd":        qtdPagVencidas,
		"pagaveis_aberto_centavos":     pagasAberto,
		"estoque_valor_centavos":       estoqueValor,
		"total_pecas":                  totalPecas,
		"ultimo_backup":                lastBackup,
		"ultimas_vendas":               ultimas,
		"hoje":                         today,
	})
}

var _ = time.Now

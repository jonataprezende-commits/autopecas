package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Column struct {
	Chave   string  `json:"chave"`
	Titulo  string  `json:"titulo"`
	Align   string  `json:"align"`
	Largura float64 `json:"largura"`
}

type Report struct {
	Tipo      string              `json:"tipo"`
	Titulo    string              `json:"titulo"`
	Subtitulo string              `json:"subtitulo"`
	Colunas   []Column            `json:"colunas"`
	Linhas    [][]string          `json:"linhas"`
	Resumo    []map[string]string `json:"resumo"`
}

func (a *App) buildReport(tipo string, q url.Values) (*Report, error) {
	de := q.Get("de")
	ate := q.Get("ate")
	hoje := todayISO()
	if ate == "" {
		ate = hoje
	}
	if de == "" {
		de = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	empresa := a.db.GetSetting("loja_nome")
	r := &Report{Tipo: tipo, Resumo: []map[string]string{}, Linhas: [][]string{}}
	periodo := fmt.Sprintf("Periodo: %s a %s", brDate(de), brDate(ate))

	switch tipo {
	case "vendas":
		r.Titulo = "Relatorio de Vendas"
		r.Subtitulo = periodo
		r.Colunas = []Column{{"data", "Data/hora", "L", 88}, {"cliente", "Cliente", "L", 120}, {"veiculo", "Veiculo", "L", 90},
			{"forma", "Pagamento", "L", 65}, {"total", "Total", "R", 62}, {"lucro", "Lucro", "R", 62}}
		rows, err := a.db.Query(`SELECT created_at,IFNULL(customer_name,''),TRIM(IFNULL(vehicle_model,'')||' '||IFNULL(vehicle_plate,'')),
			payment,total_cents,cost_cents,status FROM sales WHERE DATE(created_at) BETWEEN ? AND ? ORDER BY id`, de, ate)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var total, lucro int64
		var count int
		porForma := map[string]int64{}
		for rows.Next() {
			var created, cust, veh, pay, status string
			var t, c int64
			if rows.Scan(&created, &cust, &veh, &pay, &t, &c, &status) != nil {
				continue
			}
			if status == "cancelada" {
				continue
			}
			count++
			total += t
			lucro += t - c
			porForma[pay] += t
			r.Linhas = append(r.Linhas, []string{created, cust, veh, pagamentos[pay], fmtMoney(t), fmtMoney(t - c)})
		}
		r.Resumo = append(r.Resumo,
			map[string]string{"label": "Vendas no periodo", "valor": fmt.Sprintf("%d", count)},
			map[string]string{"label": "Total vendido", "valor": fmtMoney(total)},
			map[string]string{"label": "Lucro estimado", "valor": fmtMoney(lucro)},
		)
		if count > 0 {
			r.Resumo = append(r.Resumo, map[string]string{"label": "Ticket medio", "valor": fmtMoney(total / int64(count))})
		}
		for _, k := range []string{"dinheiro", "pix", "cartao", "transferencia", "faturado"} {
			if v, ok := porForma[k]; ok {
				r.Resumo = append(r.Resumo, map[string]string{"label": "Recebido em " + pagamentos[k], "valor": fmtMoney(v)})
			}
		}

	case "lucro":
		r.Titulo = "Lucro estimado por dia"
		r.Subtitulo = periodo
		r.Colunas = []Column{{"data", "Dia", "L", 100}, {"vendas", "Vendas", "R", 70}, {"total", "Total vendido", "R", 110},
			{"custo", "Custo", "R", 110}, {"lucro", "Lucro", "R", 110}}
		rows, err := a.db.Query(`SELECT DATE(created_at),COUNT(*),SUM(total_cents),SUM(cost_cents)
			FROM sales WHERE status='concluida' AND DATE(created_at) BETWEEN ? AND ? GROUP BY DATE(created_at) ORDER BY DATE(created_at) DESC`, de, ate)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var tv, tc int64
		for rows.Next() {
			var d string
			var n int
			var t, c int64
			if rows.Scan(&d, &n, &t, &c) != nil {
				continue
			}
			tv += t
			tc += c
			r.Linhas = append(r.Linhas, []string{brDate(d), fmt.Sprintf("%d", n), fmtMoney(t), fmtMoney(c), fmtMoney(t - c)})
		}
		r.Resumo = append(r.Resumo, map[string]string{"label": "Total vendido", "valor": fmtMoney(tv)},
			map[string]string{"label": "Custo das pecas", "valor": fmtMoney(tc)},
			map[string]string{"label": "Lucro estimado", "valor": fmtMoney(tv - tc)})

	case "mais_vendidos":
		r.Titulo = "Produtos mais vendidos"
		r.Subtitulo = periodo
		r.Colunas = []Column{{"codigo", "Codigo", "L", 70}, {"descricao", "Descricao", "L", 200},
			{"qtd", "Qtd", "R", 55}, {"total", "Total vendido", "R", 90}, {"lucro", "Lucro", "R", 90}}
		rows, err := a.db.Query(`SELECT si.code,si.description,SUM(si.qty),SUM(si.total_cents),CAST(SUM(si.total_cents-si.unit_cost_cents*si.qty) AS INTEGER)
			FROM sale_items si JOIN sales s ON s.id=si.sale_id
			WHERE s.status='concluida' AND DATE(s.created_at) BETWEEN ? AND ?
			GROUP BY si.code,si.description ORDER BY SUM(si.qty) DESC LIMIT 300`, de, ate)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var code, desc string
			var qty float64
			var t, l int64
			if rows.Scan(&code, &desc, &qty, &t, &l) == nil {
				r.Linhas = append(r.Linhas, []string{code, desc, fmtQty(qty), fmtMoney(t), fmtMoney(l)})
			}
		}

	case "estoque_minimo":
		r.Titulo = "Pecas abaixo do estoque minimo"
		r.Subtitulo = "Sugestao de compra"
		r.Colunas = []Column{{"codigo", "Codigo", "L", 70}, {"descricao", "Descricao", "L", 175}, {"marca", "Marca", "L", 70},
			{"estoque", "Estoque", "R", 55}, {"minimo", "Minimo", "R", 55}, {"comprar", "Comprar", "R", 55},
			{"custo", "Custo un.", "R", 70}}
		rows, err := a.db.Query(`SELECT p.code,p.description,p.brand,p.stock,p.min_stock,COALESCE(NULLIF(p.avg_cost_cents,0),p.cost_cents)
			FROM parts p WHERE p.active=1 AND p.stock<=p.min_stock ORDER BY (p.min_stock-p.stock) DESC, p.description`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var totalCompra int64
		for rows.Next() {
			var code, desc, brand string
			var stock, minimo float64
			var cost int64
			if rows.Scan(&code, &desc, &brand, &stock, &minimo, &cost) == nil {
				comprar := minimo - stock
				if comprar < 1 {
					comprar = 1
				}
				totalCompra += int64(comprar * float64(cost))
				r.Linhas = append(r.Linhas, []string{code, desc, brand, fmtQty(stock), fmtQty(minimo), fmtQty(comprar), fmtMoney(cost)})
			}
		}
		r.Resumo = append(r.Resumo, map[string]string{"label": "Itens abaixo do minimo", "valor": fmt.Sprintf("%d", len(r.Linhas))},
			map[string]string{"label": "Custo estimado da reposicao", "valor": fmtMoney(totalCompra)})

	case "pedidos_abertos":
		r.Titulo = "Pedidos de compra abertos"
		r.Subtitulo = "Fichas de falta agrupadas por fornecedor"
		r.Colunas = []Column{{"id", "Pedido", "L", 60}, {"data", "Data", "L", 80}, {"fornecedor", "Fornecedor", "L", 190},
			{"itens", "Itens", "R", 60}, {"total", "Total estimado", "R", 100}}
		rows, err := a.db.Query(`SELECT o.id,o.created_at,IFNULL(o.supplier_name,'(sem fornecedor)'),o.total_cents,
			(SELECT COUNT(*) FROM order_items oi WHERE oi.order_id=o.id) FROM orders o WHERE o.status='aberto' ORDER BY o.id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var total int64
		for rows.Next() {
			var id, t int64
			var dt, sup string
			var n int
			if rows.Scan(&id, &dt, &sup, &t, &n) == nil {
				total += t
				r.Linhas = append(r.Linhas, []string{fmt.Sprintf("#%d", id), brDate(dt), sup, fmt.Sprintf("%d", n), fmtMoney(t)})
			}
		}
		r.Resumo = append(r.Resumo, map[string]string{"label": "Pedidos abertos", "valor": fmt.Sprintf("%d", len(r.Linhas))},
			map[string]string{"label": "Total estimado", "valor": fmtMoney(total)})

	case "recebiveis":
		r.Titulo = "Contas a receber"
		r.Subtitulo = "Vendas faturadas"
		r.Colunas = []Column{{"cliente", "Cliente", "L", 150}, {"venda", "Venda", "L", 60}, {"valor", "Valor", "R", 85},
			{"venc", "Vencimento", "L", 85}, {"atraso", "Atraso", "R", 60}, {"status", "Situacao", "L", 100}}
		rows, err := a.db.Query(`SELECT IFNULL(customer_name,''),IFNULL(sale_id,0),amount_cents,due_date,status
			FROM receivables ORDER BY (status='aberto') DESC, due_date`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var aberto, vencido int64
		for rows.Next() {
			var cust, due, st string
			var saleID, amount int64
			if rows.Scan(&cust, &saleID, &amount, &due, &st) != nil {
				continue
			}
			atraso := ""
			// item 6: cancelada/estornada nunca contam como paga nem como aberta
			sit := "Pago"
			switch st {
			case "aberto":
				sit = "Em aberto"
			case "pago":
				sit = "Pago"
			case "cancelado":
				sit = "Cancelada"
			case "estornado":
				sit = "Estornada"
			}
			if st == "aberto" {
				aberto += amount
				sit = "Em aberto"
				if due < hoje {
					sit = "VENCIDO"
					vencido += amount
					if t, ok := dataLocal(due); ok {
						atraso = fmt.Sprintf("%d dias", diasDeCalendario(t, time.Now()))
					}
				}
			}
			r.Linhas = append(r.Linhas, []string{cust, fmt.Sprintf("#%d", saleID), fmtMoney(amount), brDate(due), atraso, sit})
		}
		r.Resumo = append(r.Resumo, map[string]string{"label": "Total em aberto", "valor": fmtMoney(aberto)},
			map[string]string{"label": "Total vencido", "valor": fmtMoney(vencido)})

	case "pagaveis":
		r.Titulo = "Contas a pagar"
		r.Subtitulo = "Boletos e despesas de fornecedor"
		r.Colunas = []Column{{"fornecedor", "Fornecedor", "L", 150}, {"nota", "Nota", "L", 70}, {"parcela", "Parc.", "L", 45},
			{"valor", "Valor", "R", 85}, {"venc", "Vencimento", "L", 85}, {"atraso", "Atraso", "R", 60}, {"status", "Situacao", "L", 95}}
		rows, err := a.db.Query(`SELECT IFNULL(supplier_name,''),IFNULL(doc_number,''),IFNULL(parcela,''),amount_cents,
			due_date,status FROM payables ORDER BY (status='aberto') DESC, due_date`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var aberto, vencido int64
		for rows.Next() {
			var nome, doc, parcela, due, st string
			var amount int64
			if rows.Scan(&nome, &doc, &parcela, &amount, &due, &st) != nil {
				continue
			}
			atraso := ""
			sit := "Pago"
			switch st {
			case "aberto":
				sit = "Em aberto"
				aberto += amount
				if due < hoje {
					sit = "VENCIDO"
					vencido += amount
					if t, ok := dataLocal(due); ok {
						atraso = fmt.Sprintf("%d dias", diasDeCalendario(t, time.Now()))
					}
				}
			case "cancelado":
				sit = "Cancelada"
			}
			r.Linhas = append(r.Linhas, []string{nome, doc, parcela, fmtMoney(amount), brDate(due), atraso, sit})
		}
		r.Resumo = append(r.Resumo, map[string]string{"label": "Total em aberto", "valor": fmtMoney(aberto)},
			map[string]string{"label": "Total vencido", "valor": fmtMoney(vencido)})

	case "estoque_valor":
		r.Titulo = "Saldo do estoque em dinheiro"
		r.Subtitulo = "Valores a preco de custo e de venda"
		r.Colunas = []Column{{"codigo", "Codigo", "L", 70}, {"descricao", "Descricao", "L", 165}, {"estoque", "Estoque", "R", 55},
			{"custo", "Custo un.", "R", 70}, {"total_custo", "Total custo", "R", 85}, {"venda", "Venda un.", "R", 70}, {"total_venda", "Total venda", "R", 85}}
		rows, err := a.db.Query(`SELECT code,description,stock,COALESCE(NULLIF(avg_cost_cents,0),cost_cents),price_cents FROM parts WHERE active=1 AND stock>0 ORDER BY description`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var tc, tv int64
		for rows.Next() {
			var code, desc string
			var stock float64
			var cost, price int64
			if rows.Scan(&code, &desc, &stock, &cost, &price) == nil {
				ct := int64(stock * float64(cost))
				vt := int64(stock * float64(price))
				tc += ct
				tv += vt
				r.Linhas = append(r.Linhas, []string{code, desc, fmtQty(stock), fmtMoney(cost), fmtMoney(ct), fmtMoney(price), fmtMoney(vt)})
			}
		}
		r.Resumo = append(r.Resumo, map[string]string{"label": "Estoque a preco de custo", "valor": fmtMoney(tc)},
			map[string]string{"label": "Estoque a preco de venda", "valor": fmtMoney(tv)},
			map[string]string{"label": "Lucro potencial", "valor": fmtMoney(tv - tc)})

	case "fichas_abertas":
		r.Titulo = "Fichas de falta em aberto"
		r.Subtitulo = "Pedidos de clientes aguardando peca"
		r.Colunas = []Column{{"id", "Ficha", "L", 55}, {"data", "Data", "L", 70}, {"peca", "Peca", "L", 175},
			{"qtd", "Qtd", "R", 45}, {"cliente", "Cliente", "L", 105}, {"veiculo", "Veiculo", "L", 85},
			{"status", "Status", "L", 55}, {"dias", "Dias", "R", 45}}
		rows, err := a.db.Query(`SELECT f.id,f.created_at,IFNULL(p.code,''),f.description,f.qty,IFNULL(f.customer_name,''),
			IFNULL(f.vehicle,''),f.status FROM faltas f LEFT JOIN parts p ON p.id=f.part_id
			WHERE f.status IN ('aberta','pedida') ORDER BY f.created_at`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		antigas := 0
		for rows.Next() {
			var id int64
			var dt, code, desc, cust, veh, st string
			var qty float64
			if rows.Scan(&id, &dt, &code, &desc, &qty, &cust, &veh, &st) == nil {
				dias := 0
				if t, err := time.Parse("2006-01-02 15:04:05", dt); err == nil {
					dias = int(time.Since(t).Hours() / 24)
				}
				if dias >= 7 {
					antigas++
				}
				peca := desc
				if code != "" {
					peca = code + " - " + desc
				}
				r.Linhas = append(r.Linhas, []string{fmt.Sprintf("#%d", id), brDate(dt), peca, fmtQty(qty), cust, veh, st, fmt.Sprintf("%d", dias)})
			}
		}
		r.Resumo = append(r.Resumo, map[string]string{"label": "Fichas em aberto", "valor": fmt.Sprintf("%d", len(r.Linhas))},
			map[string]string{"label": "Abertas ha 7 dias ou mais", "valor": fmt.Sprintf("%d", antigas)})

	case "clientes":
		r.Titulo = "Clientes e historico de compras"
		r.Subtitulo = "Total comprado por cliente"
		r.Colunas = []Column{{"nome", "Cliente", "L", 160}, {"telefone", "Telefone", "L", 90}, {"compras", "Compras", "R", 65},
			{"total", "Total comprado", "R", 110}, {"ultima", "Ultima compra", "L", 90}}
		rows, err := a.db.Query(`SELECT c.name,IFNULL(c.phone,''),
			(SELECT COUNT(*) FROM sales s WHERE s.customer_id=c.id AND s.status='concluida'),
			(SELECT CAST(IFNULL(SUM(total_cents),0) AS INTEGER) FROM sales s WHERE s.customer_id=c.id AND s.status='concluida'),
			(SELECT IFNULL(MAX(DATE(created_at)),'') FROM sales s WHERE s.customer_id=c.id AND s.status='concluida')
			FROM customers c ORDER BY c.name COLLATE NOCASE`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var name, phone, ult string
			var n int
			var total int64
			if rows.Scan(&name, &phone, &n, &total, &ult) == nil {
				r.Linhas = append(r.Linhas, []string{name, phone, fmt.Sprintf("%d", n), fmtMoney(total), brDate(ult)})
			}
		}
	default:
		return nil, fmt.Errorf("relatorio desconhecido")
	}
	r.Subtitulo = strings.TrimSpace(empresa + " - " + r.Subtitulo)
	return r, nil
}

func fmtQty(q float64) string {
	if q == float64(int64(q)) {
		return fmt.Sprintf("%d", int64(q))
	}
	return strings.ReplaceAll(fmt.Sprintf("%.2f", q), ".", ",")
}

func brDate(s string) string {
	if len(s) >= 10 {
		return s[8:10] + "/" + s[5:7] + "/" + s[0:4]
	}
	return s
}

func (a *App) handleReport(w http.ResponseWriter, r *http.Request) {
	tipo := r.PathValue("tipo")
	rep, err := a.buildReport(tipo, r.URL.Query())
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, rep)
}

func (a *App) handleReportCSV(w http.ResponseWriter, r *http.Request) {
	tipo := r.PathValue("tipo")
	rep, err := a.buildReport(tipo, r.URL.Query())
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	var b bytes.Buffer
	b.WriteString("\xEF\xBB\xBF") // BOM para o Excel abrir com acentos
	cw := csv.NewWriter(&b)
	cw.Comma = ';'
	_ = cw.Write([]string{rep.Titulo})
	_ = cw.Write([]string{rep.Subtitulo})
	_ = cw.Write([]string{})
	hdr := []string{}
	for _, c := range rep.Colunas {
		hdr = append(hdr, c.Titulo)
	}
	_ = cw.Write(hdr)
	for _, l := range rep.Linhas {
		row := make([]string, len(l))
		for i, v := range l {
			row[i] = strings.ReplaceAll(v, ".", ",") //csv pt-br friendly
			if i < len(rep.Colunas) {
				row[i] = csvField(rep, i, v)
			}
		}
		_ = cw.Write(row)
	}
	_ = cw.Write([]string{})
	for _, kv := range rep.Resumo {
		_ = csvWriterWrite(cw, []string{kv["label"], kv["valor"]})
	}
	cw.Flush()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+tipo+"-"+todayISO()+".csv")
	_, _ = w.Write(b.Bytes())
}

func csvField(rep *Report, i int, v string) string {
	if i < len(rep.Colunas) {
		switch rep.Colunas[i].Align {
		case "R":
			// mantem formato brasileiro: R$ 1.234,56 -> 1234,56 (numero puro para Excel)
			clean := strings.ReplaceAll(strings.ReplaceAll(v, "R$", ""), " ", "")
			clean = strings.ReplaceAll(clean, ".", "")
			return clean
		}
	}
	return v
}

func csvWriterWrite(cw *csv.Writer, row []string) error { return cw.Write(row) }

func (a *App) handleReportPDF(w http.ResponseWriter, r *http.Request) {
	tipo := r.PathValue("tipo")
	rep, err := a.buildReport(tipo, r.URL.Query())
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	t := pdfTable{
		Titulo: rep.Titulo, Subtitulo: rep.Subtitulo,
		Empresa: a.db.GetSetting("loja_nome"),
		Rodape:  fmt.Sprintf("Gerado em %s - %s", time.Now().Format("02/01/2006 15:04"), appName),
	}
	for _, c := range rep.Colunas {
		t.Colunas = append(t.Colunas, c.Titulo)
		t.Larguras = append(t.Larguras, c.Largura)
		t.Alinhamento = append(t.Alinhamento, c.Align)
	}
	t.Linhas = rep.Linhas
	for _, kv := range rep.Resumo {
		t.Resumo = append(t.Resumo, kv["label"]+": "+kv["valor"])
	}
	data := t.Build()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename="+tipo+"-"+todayISO()+".pdf")
	_, _ = w.Write(data)
}

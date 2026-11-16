package main

import (
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type SaleItemInput struct {
	PartID    *int64  `json:"peca_id"`
	Codigo    string  `json:"codigo"`
	Descricao string  `json:"descricao"`
	Qtd       float64 `json:"quantidade"`
	Preco     int64   `json:"preco_centavos"`
	Desconto  int64   `json:"desconto_centavos"`
}

type SaleInput struct {
	CustomerID   *int64          `json:"cliente_id"`
	CustomerName string          `json:"cliente_nome"`
	VehiclePlate string          `json:"veiculo_placa"`
	VehicleModel string          `json:"veiculo_modelo"`
	VehicleYear  string          `json:"veiculo_ano"`
	Items        []SaleItemInput `json:"itens"`
	Discount     int64           `json:"desconto_centavos"`
	Payment      string          `json:"forma_pagamento"`
	Received     int64           `json:"recebido_centavos"`
	DueDays      int             `json:"vencimento_dias"`
	Note         string          `json:"observacao"`
	CreatedAt    string          `json:"data"` // opcional (data/hora manual)
}

var pagamentos = map[string]string{
	"dinheiro": "Dinheiro", "pix": "PIX", "cartao": "Cartao",
	"faturado": "Faturado", "transferencia": "Transferencia",
}

func normPayment(s string) string {
	s = normText(s)
	switch {
	case strings.Contains(s, "pix"):
		return "pix"
	case strings.Contains(s, "cartao") || strings.Contains(s, "debito") || strings.Contains(s, "credito"):
		return "cartao"
	case strings.Contains(s, "faturado") || strings.Contains(s, "prazo") || strings.Contains(s, "fiado"):
		return "faturado"
	case strings.Contains(s, "transf"):
		return "transferencia"
	default:
		return "dinheiro"
	}
}

// limite de quantidade por item (protege contra digitacao errada)
const maxQtdItem = 1000000

// maxPrecoCentavos = R$ 10.000.000,00 por unidade. Acima disso e' erro de
// digitacao (virgula esquecida) e o valor bruto estourava o calculo.
const maxPrecoCentavos = 1000000000

// validarDataVenda aceita "AAAA-MM-DD HH:MM:SS" ou "AAAA-MM-DD" e devolve
// sempre o formato completo. Texto vazio = agora.
func validarDataVenda(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nowISO(), nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.Format("2006-01-02 15:04:05"), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2006-01-02 15:04:05"), nil
	}
	return "", fmt.Errorf("data inválida: use AAAA-MM-DD ou AAAA-MM-DD HH:MM:SS")
}

// prazoFaturadoPadrao le o prazo configurado. Precisa ser chamado FORA de
// qualquer transacao: o banco usa uma unica conexao (SetMaxOpenConns(1)).
func (a *App) prazoFaturadoPadrao() int {
	if n, err := strconv.Atoi(strings.TrimSpace(a.db.GetSetting("prazo_faturado_dias"))); err == nil && n > 0 {
		return n
	}
	return 30
}

func (a *App) handleCreateSale(w http.ResponseWriter, r *http.Request) {
	var in SaleInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	if len(in.Items) == 0 {
		fail(w, 400, "inclua pelo menos uma peca na venda")
		return
	}
	pay := normPayment(in.Payment)
	created, err := validarDataVenda(in.CreatedAt)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if in.Discount < 0 {
		fail(w, 400, "o desconto da venda não pode ser negativo")
		return
	}
	if in.Received < 0 {
		fail(w, 400, "o valor recebido não pode ser negativo")
		return
	}
	// item 2: tudo que vem do banco e lido antes da transacao
	prazoPadrao := a.prazoFaturadoPadrao()
	permitirNeg := a.db.GetSetting("permitir_estoque_negativo") != "0"

	type itemCalc struct {
		in    SaleItemInput
		code  string
		desc  string
		cost  int64
		bruto int64
		total int64
	}
	calc := []itemCalc{}
	var subtotal, descontoItens, costTotal int64
	for _, it := range in.Items {
		if it.Qtd <= 0 {
			fail(w, 400, "quantidade inválida em um dos itens")
			return
		}
		if it.Qtd > maxQtdItem {
			fail(w, 400, "quantidade acima do limite de 1.000.000 em um dos itens")
			return
		}
		if it.Preco < 0 {
			fail(w, 400, "o preço do item não pode ser negativo")
			return
		}
		if it.Preco > maxPrecoCentavos {
			fail(w, 400, "preço acima do limite de R$ 10.000.000,00 por unidade — confira se não faltou a vírgula")
			return
		}
		if float64(it.Preco)*it.Qtd > 9e16 {
			fail(w, 400, "valor total do item acima do limite — confira a quantidade e o preço")
			return
		}
		if it.Desconto < 0 {
			fail(w, 400, "o desconto do item não pode ser negativo")
			return
		}
		c := itemCalc{in: it, code: it.Codigo, desc: it.Descricao}
		if it.PartID != nil && *it.PartID > 0 {
			var cost int64
			var code, desc string
			if err := a.db.QueryRow(`SELECT code,description,cost_cents FROM parts WHERE id=?`, *it.PartID).Scan(&code, &desc, &cost); err != nil {
				fail(w, 400, "peca nao encontrada (id "+fmt.Sprint(*it.PartID)+")")
				return
			}
			c.code, c.desc, c.cost = code, desc, cost
		} else if strings.TrimSpace(c.desc) == "" {
			fail(w, 400, "item sem descricao")
			return
		}
		// item 1: valor bruto arredondado e desconto do item respeitado
		c.bruto = int64(math.Round(float64(it.Preco) * it.Qtd))
		if it.Desconto > c.bruto {
			fail(w, 400, "o desconto do item não pode ser maior que o valor do item")
			return
		}
		c.total = c.bruto - it.Desconto
		subtotal += c.bruto
		descontoItens += it.Desconto
		costTotal += int64(math.Round(float64(c.cost) * it.Qtd))
		calc = append(calc, c)
	}
	// total = bruto - descontos dos itens - desconto da venda (nunca negativo)
	total := subtotal - descontoItens - in.Discount
	descontoTotal := descontoItens + in.Discount
	if total < 0 {
		total = 0
		descontoTotal = subtotal // mantem subtotal - desconto == total
	}

	change := int64(0)
	if pay != "faturado" {
		if in.Received > 0 && in.Received < total {
			fail(w, 400, "Valor recebido é menor que o total.")
			return
		}
		if in.Received == 0 {
			in.Received = total
		}
		if in.Received > total {
			change = in.Received - total
		}
	}

	var saleID int64
	err = a.db.Tx(func(tx *sql.Tx) error {
		custName := strings.TrimSpace(in.CustomerName)
		if in.CustomerID != nil && *in.CustomerID > 0 && custName == "" {
			_ = tx.QueryRow(`SELECT name FROM customers WHERE id=?`, *in.CustomerID).Scan(&custName)
		}
		res, err := tx.Exec(`INSERT INTO sales(created_at,customer_id,customer_name,vehicle_plate,vehicle_model,vehicle_year,
			subtotal_cents,discount_cents,total_cents,cost_cents,payment,received_cents,change_cents,due_date,status,note)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'concluida',?)`,
			created, in.CustomerID, custName, in.VehiclePlate, in.VehicleModel, in.VehicleYear,
			subtotal, descontoTotal, total, costTotal, pay, in.Received, change, "", in.Note)
		if err != nil {
			return err
		}
		saleID, _ = res.LastInsertId()
		for _, c := range calc {
			if _, err := tx.Exec(`INSERT INTO sale_items(sale_id,part_id,code,description,qty,unit_price_cents,unit_cost_cents,discount_cents,total_cents)
				VALUES(?,?,?,?,?,?,?,?,?)`, saleID, c.in.PartID, c.code, c.desc, c.in.Qtd, c.in.Preco, c.cost, c.in.Desconto, c.total); err != nil {
				return err
			}
			if c.in.PartID != nil && *c.in.PartID > 0 {
				var bal float64
				if err := tx.QueryRow(`SELECT stock FROM parts WHERE id=?`, *c.in.PartID).Scan(&bal); err != nil {
					return err
				}
				nb := bal - c.in.Qtd
				if nb < 0 && !permitirNeg {
					return fmt.Errorf("estoque insuficiente para %s (saldo %v)", c.desc, bal)
				}
				if _, err := tx.Exec(`UPDATE parts SET stock=?, updated_at=? WHERE id=?`, nb, nowISO(), *c.in.PartID); err != nil {
					return err
				}
				if _, err := tx.Exec(`INSERT INTO stock_moves(part_id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,ref_id,created_at)
					VALUES(?,?,?,?,?,?,?,?,?)`, *c.in.PartID, "saida", -c.in.Qtd, nb, c.cost, "venda", "venda", saleID, created); err != nil {
					return err
				}
			}
		}
		if pay == "faturado" {
			dias := in.DueDays
			if dias <= 0 {
				dias = prazoPadrao
			}
			// 1.0.10: o prazo conta a partir da DATA DA VENDA. Antes usava
			// sempre "hoje", e uma venda lancada com data retroativa (ontem,
			// semana passada) vencia um dia/semana depois do que devia.
			base := time.Now()
			if t, ok := dataLocal(created); ok {
				base = t
			}
			due := base.AddDate(0, 0, dias).Format("2006-01-02")
			if _, err := tx.Exec(`INSERT INTO receivables(sale_id,customer_id,customer_name,amount_cents,due_date,status,created_at)
				VALUES(?,?,?,?,?,'aberto',?)`, saleID, in.CustomerID, custName, total, due, created); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	sale, _ := a.getSale(saleID)
	// 1.0.10: quando o sistema esta configurado para vender sem estoque, a venda
	// entra - mas o usuario passa a ser avisado na hora de quais pecas ficaram
	// com saldo negativo (antes o aviso so existia no relatorio de minimo).
	if sale != nil {
		for _, c := range calc {
			if c.in.PartID == nil || *c.in.PartID <= 0 {
				continue
			}
			var saldo float64
			if err := a.db.QueryRow(`SELECT stock FROM parts WHERE id=?`, *c.in.PartID).Scan(&saldo); err == nil && saldo < 0 {
				lista, _ := sale["aviso_estoque"].([]map[string]any)
				sale["aviso_estoque"] = append(lista, map[string]any{
					"peca_id": *c.in.PartID, "descricao": c.desc, "saldo": saldo,
				})
			}
		}
	}
	writeJSON(w, 200, sale)
}

func (a *App) getSale(id int64) (map[string]any, error) {
	var s struct {
		id                                    int64
		created, custName, plate, model, year string
		subtotal, discount, total, cost       int64
		payment, status, note, due            string
		received, change                      int64
		custID                                sql.NullInt64
	}
	err := a.db.QueryRow(`SELECT id,created_at,customer_id,customer_name,IFNULL(vehicle_plate,''),IFNULL(vehicle_model,''),
		IFNULL(vehicle_year,''),subtotal_cents,discount_cents,total_cents,cost_cents,payment,received_cents,change_cents,
		IFNULL(due_date,''),status,IFNULL(note,'') FROM sales WHERE id=?`, id).
		Scan(&s.id, &s.created, &s.custID, &s.custName, &s.plate, &s.model, &s.year, &s.subtotal, &s.discount,
			&s.total, &s.cost, &s.payment, &s.received, &s.change, &s.due, &s.status, &s.note)
	if err != nil {
		return nil, err
	}
	rows, err := a.db.Query(`SELECT id,part_id,code,description,qty,unit_price_cents,unit_cost_cents,discount_cents,total_cents FROM sale_items WHERE sale_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var iid, price, cost, disc, tot int64
		var pid sql.NullInt64
		var code, desc string
		var qty float64
		if rows.Scan(&iid, &pid, &code, &desc, &qty, &price, &cost, &disc, &tot) == nil {
			items = append(items, map[string]any{
				"id": iid, "peca_id": idOuNulo(pid), "codigo": code, "descricao": desc, "quantidade": qty,
				"preco_centavos": price, "custo_centavos": cost, "desconto_centavos": disc, "total_centavos": tot,
			})
		}
	}
	return map[string]any{
		"id": s.id, "data": s.created, "cliente_id": idOuNulo(s.custID), "cliente": s.custName,
		"veiculo_placa": s.plate, "veiculo_modelo": s.model, "veiculo_ano": s.year,
		"subtotal_centavos": s.subtotal, "desconto_centavos": s.discount, "total_centavos": s.total,
		"custo_centavos": s.cost, "lucro_centavos": s.total - s.cost,
		"forma_pagamento": s.payment, "forma_pagamento_nome": pagamentos[s.payment],
		"recebido_centavos": s.received, "troco_centavos": s.change,
		"vencimento": s.due, "status": s.status, "observacao": s.note, "itens": items,
	}, nil
}

func (a *App) handleGetSale(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s, err := a.getSale(id)
	if err != nil {
		fail(w, 404, "venda nao encontrada")
		return
	}
	writeJSON(w, 200, s)
}

func (a *App) handleListSales(w http.ResponseWriter, r *http.Request) {
	ini := r.URL.Query().Get("de")
	fim := r.URL.Query().Get("ate")
	q := `SELECT id,created_at,IFNULL(customer_name,''),IFNULL(vehicle_plate,''),IFNULL(vehicle_model,''),
		total_cents,cost_cents,payment,status FROM sales WHERE 1=1`
	var args []any
	if ini != "" {
		q += " AND DATE(created_at)>=?"
		args = append(args, ini)
	}
	if fim != "" {
		q += " AND DATE(created_at)<=?"
		args = append(args, fim)
	}
	q += " ORDER BY id DESC LIMIT 500"
	rows, err := a.db.Query(q, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, total, cost int64
		var created, cust, plate, model, pay, status string
		if rows.Scan(&id, &created, &cust, &plate, &model, &total, &cost, &pay, &status) != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "data": created, "cliente": cust, "veiculo_placa": plate, "veiculo_modelo": model,
			"total_centavos": total, "lucro_centavos": total - cost,
			"forma_pagamento": pay, "forma_pagamento_nome": pagamentos[pay], "status": status,
		})
	}
	writeJSON(w, 200, map[string]any{"itens": out})
}

func (a *App) handleCancelSale(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	err := a.db.Tx(func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRow(`SELECT status FROM sales WHERE id=?`, id).Scan(&status); err != nil {
			return fmt.Errorf("venda nao encontrada")
		}
		if status == "cancelada" {
			return nil
		}
		rows, err := tx.Query(`SELECT part_id,qty,description FROM sale_items WHERE sale_id=?`, id)
		if err != nil {
			return err
		}
		type ri struct {
			pid  sql.NullInt64
			qty  float64
			desc string
		}
		list := []ri{}
		for rows.Next() {
			var x ri
			if rows.Scan(&x.pid, &x.qty, &x.desc) == nil {
				list = append(list, x)
			}
		}
		rows.Close()
		for _, x := range list {
			if !x.pid.Valid || x.pid.Int64 == 0 {
				continue
			}
			var bal float64
			var cost int64
			if err := tx.QueryRow(`SELECT stock,cost_cents FROM parts WHERE id=?`, x.pid.Int64).Scan(&bal, &cost); err != nil {
				continue
			}
			nb := bal + x.qty
			if _, err := tx.Exec(`UPDATE parts SET stock=? WHERE id=?`, nb, x.pid.Int64); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO stock_moves(part_id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,ref_id,created_at)
				VALUES(?,?,?,?,?,?,?,?,?)`, x.pid.Int64, "entrada", x.qty, nb, cost, "cancelamento de venda", "venda", id, nowISO()); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE sales SET status='cancelada' WHERE id=?`, id); err != nil {
			return err
		}
		// item 6: conta ja paga nao pode continuar como "pago" (virou estorno)
		_, err = tx.Exec(`UPDATE receivables
			SET status=CASE WHEN status='pago' THEN 'estornado' ELSE 'cancelado' END
			WHERE sale_id=? AND status IN ('aberto','pago')`, id)
		return err
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleDailyClose(w http.ResponseWriter, r *http.Request) {
	data := r.URL.Query().Get("data")
	if data == "" {
		data = todayISO()
	}
	var total, cost, qtd int64
	_ = a.db.QueryRow(`SELECT IFNULL(SUM(total_cents),0),IFNULL(SUM(cost_cents),0),COUNT(*) FROM sales WHERE status='concluida' AND DATE(created_at)=?`, data).Scan(&total, &cost, &qtd)

	rows, err := a.db.Query(`SELECT payment,IFNULL(SUM(total_cents),0),COUNT(*) FROM sales WHERE status='concluida' AND DATE(created_at)=? GROUP BY payment`, data)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	porForma := []map[string]any{}
	for rows.Next() {
		var pay string
		var t, c int64
		if rows.Scan(&pay, &t, &c) == nil {
			porForma = append(porForma, map[string]any{"forma": pay, "nome": pagamentos[pay], "total_centavos": t, "qtd": c})
		}
	}
	rows.Close()

	irows, err := a.db.Query(`SELECT si.code,si.description,SUM(si.qty),SUM(si.total_cents) FROM sale_items si
		JOIN sales s ON s.id=si.sale_id WHERE s.status='concluida' AND DATE(s.created_at)=? GROUP BY si.code,si.description ORDER BY SUM(si.qty) DESC`, data)
	itens := []map[string]any{}
	if err == nil {
		for irows.Next() {
			var code, desc string
			var q float64
			var t int64
			if irows.Scan(&code, &desc, &q, &t) == nil {
				itens = append(itens, map[string]any{"codigo": code, "descricao": desc, "quantidade": q, "total_centavos": t})
			}
		}
		irows.Close()
	}
	writeJSON(w, 200, map[string]any{
		"data": data, "total_centavos": total, "custo_centavos": cost,
		"lucro_centavos": total - cost, "qtd_vendas": qtd,
		"por_forma": porForma, "itens": itens,
	})
}

func (a *App) handleListReceivables(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	q := `SELECT r.id,IFNULL(r.sale_id,0),IFNULL(r.customer_name,''),r.amount_cents,IFNULL(r.due_date,''),r.status,
		IFNULL(r.created_at,''),IFNULL(r.paid_at,'')
		FROM receivables r WHERE 1=1`
	var args []any
	if status == "" {
		q += " AND r.status='aberto'"
	} else if status != "todos" {
		q += " AND r.status=?"
		args = append(args, status)
	}
	q += " ORDER BY r.due_date"
	rows, err := a.db.Query(q, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	today := todayISO()
	out := []map[string]any{}
	var total int64
	for rows.Next() {
		var id, saleID, amount int64
		var cust, due, st, created, paid string
		if rows.Scan(&id, &saleID, &cust, &amount, &due, &st, &created, &paid) != nil {
			continue
		}
		diasAtraso := 0
		if st == "aberto" && due < today {
			if t, ok := dataLocal(due); ok {
				diasAtraso = diasDeCalendario(t, time.Now())
			}
		}
		if st == "aberto" {
			total += amount
		}
		out = append(out, map[string]any{
			"id": id, "venda_id": saleID, "cliente": cust, "valor_centavos": amount,
			"vencimento": due, "status": st, "criado": created, "pago_em": paid,
			"vencido": st == "aberto" && due < today, "dias_atraso": diasAtraso,
		})
	}
	writeJSON(w, 200, map[string]any{"itens": out, "total_aberto_centavos": total})
}

func (a *App) handlePayReceivable(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	res, err := a.db.Exec(`UPDATE receivables SET status='pago', paid_at=? WHERE id=? AND status='aberto'`, nowISO(), id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 400, "conta não encontrada ou já encerrada")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

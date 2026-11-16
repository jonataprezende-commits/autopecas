package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Falta struct {
	ID            int64   `json:"id"`
	Criada        string  `json:"criada_em"`
	PartID        *int64  `json:"peca_id"`
	PecaCodigo    string  `json:"peca_codigo"`
	Descricao     string  `json:"descricao"`
	Qtd           float64 `json:"quantidade"`
	CustomerID    *int64  `json:"cliente_id"`
	CustomerName  string  `json:"cliente_nome"`
	CustomerPhone string  `json:"cliente_telefone"`
	Veiculo       string  `json:"veiculo"`
	SupplierID    *int64  `json:"fornecedor_id"`
	SupplierName  string  `json:"fornecedor_nome"`
	Status        string  `json:"status"`
	Custo         int64   `json:"custo_centavos"`
	Notas         string  `json:"observacoes"`
	OrderID       *int64  `json:"pedido_id"`
	Recebida      string  `json:"recebida_em"`
	DiasAberta    int     `json:"dias_aberta"`
}

func (a *App) handleListFaltas(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	q := `SELECT f.id,f.created_at,f.part_id,IFNULL(p.code,''),f.description,f.qty,f.customer_id,
		IFNULL(f.customer_name,''),IFNULL(f.customer_phone,''),IFNULL(f.vehicle,''),f.supplier_id,
		IFNULL(s.name,''),f.status,f.cost_cents,IFNULL(f.notes,''),f.order_id,IFNULL(f.received_at,'')
		FROM faltas f LEFT JOIN parts p ON p.id=f.part_id LEFT JOIN suppliers s ON s.id=f.supplier_id WHERE 1=1`
	var args []any
	switch status {
	case "", "ativas":
		q += " AND f.status IN ('aberta','pedida')"
	case "todas":
	default:
		q += " AND f.status=?"
		args = append(args, status)
	}
	if d := r.URL.Query().Get("dias"); d != "" {
		q += " AND f.status IN ('aberta','pedida') AND DATE(f.created_at) <= DATE('now','localtime','-' || ? || ' days')"
		args = append(args, d)
	}
	q += " ORDER BY f.status, f.created_at"
	rows, err := a.db.Query(q, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []Falta{}
	hoje := time.Now()
	for rows.Next() {
		var f Falta
		if rows.Scan(&f.ID, &f.Criada, &f.PartID, &f.PecaCodigo, &f.Descricao, &f.Qtd, &f.CustomerID,
			&f.CustomerName, &f.CustomerPhone, &f.Veiculo, &f.SupplierID, &f.SupplierName, &f.Status,
			&f.Custo, &f.Notas, &f.OrderID, &f.Recebida) != nil {
			continue
		}
		if t, ok := dataLocal(f.Criada); ok {
			f.DiasAberta = diasDeCalendario(t, hoje)
		}
		out = append(out, f)
	}
	writeJSON(w, 200, map[string]any{"itens": out})
}

type FaltaInput struct {
	PartID        *int64  `json:"peca_id"`
	Descricao     string  `json:"descricao"`
	Qtd           float64 `json:"quantidade"`
	CustomerID    *int64  `json:"cliente_id"`
	CustomerName  string  `json:"cliente_nome"`
	CustomerPhone string  `json:"cliente_telefone"`
	Veiculo       string  `json:"veiculo"`
	SupplierID    *int64  `json:"fornecedor_id"`
	Custo         int64   `json:"custo_centavos"`
	Notas         string  `json:"observacoes"`
	Status        string  `json:"status"`
}

func (a *App) handleSaveFalta(w http.ResponseWriter, r *http.Request) {
	var in FaltaInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	if in.Qtd <= 0 {
		in.Qtd = 1
	}
	desc := strings.TrimSpace(in.Descricao)
	cost := in.Custo
	if in.PartID != nil && *in.PartID > 0 {
		var d string
		var c int64
		if err := a.db.QueryRow(`SELECT description,cost_cents FROM parts WHERE id=?`, *in.PartID).Scan(&d, &c); err == nil {
			if desc == "" {
				desc = d
			}
			if cost == 0 {
				cost = c
			}
		}
	}
	if desc == "" {
		fail(w, 400, "informe a peca ou uma descricao")
		return
	}
	custName := strings.TrimSpace(in.CustomerName)
	if in.CustomerID != nil && *in.CustomerID > 0 && custName == "" {
		_ = a.db.QueryRow(`SELECT name FROM customers WHERE id=?`, *in.CustomerID).Scan(&custName)
	}
	status := in.Status
	if status == "" {
		status = "aberta"
	}
	id := int64(0)
	if s := r.PathValue("id"); s != "" {
		id, _ = strconv.ParseInt(s, 10, 64)
	}
	if id == 0 {
		res, err := a.db.Exec(`INSERT INTO faltas(created_at,part_id,description,qty,customer_id,customer_name,customer_phone,vehicle,supplier_id,status,cost_cents,notes)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, nowISO(), in.PartID, desc, in.Qtd, in.CustomerID, custName,
			in.CustomerPhone, in.Veiculo, in.SupplierID, status, cost, in.Notas)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		id, _ = res.LastInsertId()
	} else {
		if _, err := a.db.Exec(`UPDATE faltas SET part_id=?,description=?,qty=?,customer_id=?,customer_name=?,customer_phone=?,
			vehicle=?,supplier_id=?,cost_cents=?,notes=? WHERE id=?`, in.PartID, desc, in.Qtd, in.CustomerID, custName,
			in.CustomerPhone, in.Veiculo, in.SupplierID, cost, in.Notas, id); err != nil {
			fail(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "id": id})
}

func (a *App) handleFaltaStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		Status string  `json:"status"`
		PartID *int64  `json:"peca_id"`
		Qtd    float64 `json:"quantidade"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	switch in.Status {
	case "aberta", "pedida":
		if _, err := a.db.Exec(`UPDATE faltas SET status=? WHERE id=?`, in.Status, id); err != nil {
			fail(w, 500, err.Error())
			return
		}
	case "recebida":
		qtd := in.Qtd
		if qtd <= 0 {
			_ = a.db.QueryRow(`SELECT qty FROM faltas WHERE id=?`, id).Scan(&qtd)
			if qtd <= 0 {
				qtd = 1
			}
		}
		var pid int64
		if in.PartID != nil && *in.PartID > 0 {
			pid = *in.PartID
			_, _ = a.db.Exec(`UPDATE faltas SET part_id=? WHERE id=?`, pid, id)
		} else {
			_ = a.db.QueryRow(`SELECT IFNULL(part_id,0) FROM faltas WHERE id=?`, id).Scan(&pid)
		}
		if pid == 0 {
			fail(w, 400, "vincule uma peca cadastrada antes de receber (aba Pecas)")
			return
		}
		if err := a.receiveFalta(id, qtd, pid); err != nil {
			var jaRecebida erroFichaJaRecebida
			if errors.As(err, &jaRecebida) {
				fail(w, 409, "Esta ficha já foi recebida. O estoque não foi lançado de novo.")
				return
			}
			fail(w, 400, err.Error())
			return
		}
	case "cancelada":
		if _, err := a.db.Exec(`UPDATE faltas SET status='cancelada' WHERE id=?`, id); err != nil {
			fail(w, 500, err.Error())
			return
		}
	default:
		fail(w, 400, "status invalido")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleDeleteFalta(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	// 1.0.10: ficha ja recebida ja deu entrada no estoque. Apagar escondia de
	// onde veio a peça; agora ela so pode ser cancelada (fica o historico).
	var status string
	if err := a.db.QueryRow(`SELECT status FROM faltas WHERE id=?`, id).Scan(&status); err != nil {
		fail(w, 404, "ficha não encontrada")
		return
	}
	if status == "recebida" {
		fail(w, 409, "esta ficha já foi recebida e deu entrada no estoque: ela não pode ser apagada (o histórico de entrada sumiria). Use Cancelar se quiser tirá-la da lista.")
		return
	}
	if _, err := a.db.Exec(`DELETE FROM faltas WHERE id=?`, id); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- pedidos de compra ----------

func (a *App) handleGenerateOrders(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FaltaIDs   []int64 `json:"ficha_ids"`
		SupplierID *int64  `json:"fornecedor_id"`
	}
	_ = readJSON(r, &in)
	q := `SELECT f.id,IFNULL(f.supplier_id,0),f.description,f.qty,f.part_id,f.cost_cents,IFNULL(p.cost_cents,0),IFNULL(p.code,'')
		FROM faltas f LEFT JOIN parts p ON p.id=f.part_id
		WHERE f.status='aberta'`
	var args []any
	if len(in.FaltaIDs) > 0 {
		marks := make([]string, len(in.FaltaIDs))
		for i, id := range in.FaltaIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		q += " AND f.id IN (" + strings.Join(marks, ",") + ")"
	}
	rows, err := a.db.Query(q, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	type rowData struct {
		id, supID, partID, costFalta, costPart int64
		desc, code                             string
		qty                                    float64
	}
	list := []rowData{}
	for rows.Next() {
		var x rowData
		var pid sql.NullInt64
		if rows.Scan(&x.id, &x.supID, &x.desc, &x.qty, &pid, &x.costFalta, &x.costPart, &x.code) == nil {
			if pid.Valid {
				x.partID = pid.Int64
			}
			list = append(list, x)
		}
	}
	rows.Close()
	if len(list) == 0 {
		fail(w, 400, "nao ha fichas de falta com status 'aberta' para gerar pedido")
		return
	}
	// agrupa por fornecedor
	groups := map[int64][]rowData{}
	for _, x := range list {
		sup := x.supID
		if in.SupplierID != nil && *in.SupplierID > 0 {
			sup = *in.SupplierID
		}
		groups[sup] = append(groups[sup], x)
	}
	criados := []map[string]any{}
	err = a.db.Tx(func(tx *sql.Tx) error {
		for sup, items := range groups {
			var total int64
			supName := ""
			var supPtr *int64
			if sup > 0 {
				_ = tx.QueryRow(`SELECT name FROM suppliers WHERE id=?`, sup).Scan(&supName)
				sp := sup
				supPtr = &sp
			}
			for _, it := range items {
				c := it.costFalta
				if c == 0 {
					c = it.costPart
				}
				total += int64(float64(c) * it.qty)
			}
			res, err := tx.Exec(`INSERT INTO orders(created_at,supplier_id,supplier_name,status,notes,total_cents)
				VALUES(?,?,?,'aberto','',?)`, nowISO(), supPtr, supName, total)
			if err != nil {
				return err
			}
			orderID, _ := res.LastInsertId()
			for _, it := range items {
				c := it.costFalta
				if c == 0 {
					c = it.costPart
				}
				if _, err := tx.Exec(`INSERT INTO order_items(order_id,falta_id,part_id,description,qty,cost_cents)
					VALUES(?,?,?,?,?,?)`, orderID, it.id, it.partID, it.desc, it.qty, c); err != nil {
					return err
				}
				if _, err := tx.Exec(`UPDATE faltas SET status='pedida', order_id=?, supplier_id=? WHERE id=?`, orderID, supPtr, it.id); err != nil {
					return err
				}
			}
			criados = append(criados, map[string]any{"id": orderID, "fornecedor": supName, "itens": len(items), "total_centavos": total})
		}
		return nil
	})
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "pedidos": criados})
}

func (a *App) handleListOrders(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	q := `SELECT o.id,o.created_at,IFNULL(o.supplier_name,''),o.status,o.total_cents,
		(SELECT COUNT(*) FROM order_items oi WHERE oi.order_id=o.id),IFNULL(o.received_at,'')
		FROM orders o WHERE 1=1`
	var args []any
	if status != "" && status != "todos" {
		q += " AND o.status=?"
		args = append(args, status)
	}
	q += " ORDER BY o.id DESC"
	rows, err := a.db.Query(q, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, total int64
		var created, sup, st, recv string
		var n int
		if rows.Scan(&id, &created, &sup, &st, &total, &n, &recv) == nil {
			out = append(out, map[string]any{"id": id, "criado": created, "fornecedor": sup, "status": st,
				"total_centavos": total, "itens": n, "recebido": recv})
		}
	}
	writeJSON(w, 200, map[string]any{"itens": out})
}

func (a *App) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var o struct {
		id, total     int64
		created, sup  string
		status, notes string
	}
	if err := a.db.QueryRow(`SELECT id,created_at,IFNULL(supplier_name,''),status,IFNULL(notes,''),total_cents FROM orders WHERE id=?`, id).
		Scan(&o.id, &o.created, &o.sup, &o.status, &o.notes, &o.total); err != nil {
		fail(w, 404, "pedido nao encontrado")
		return
	}
	rows, err := a.db.Query(`SELECT oi.id,oi.falta_id,oi.part_id,oi.description,oi.qty,oi.cost_cents,IFNULL(p.code,''),IFNULL(f.status,'')
		FROM order_items oi LEFT JOIN parts p ON p.id=oi.part_id LEFT JOIN faltas f ON f.id=oi.falta_id WHERE oi.order_id=?`, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var iid, cost int64
		var faltaID, partID sql.NullInt64
		var desc, code, fstatus string
		var qty float64
		if rows.Scan(&iid, &faltaID, &partID, &desc, &qty, &cost, &code, &fstatus) == nil {
			items = append(items, map[string]any{"id": iid, "ficha_id": idOuNulo(faltaID), "peca_id": idOuNulo(partID),
				"descricao": desc, "codigo": code, "quantidade": qty, "custo_centavos": cost, "ficha_status": fstatus})
		}
	}
	writeJSON(w, 200, map[string]any{"id": o.id, "criado": o.created, "fornecedor": o.sup, "status": o.status,
		"observacoes": o.notes, "total_centavos": o.total, "itens": items})
}

// erroRecebimento marca falhas causadas pelos dados enviados pelo usuario
// (mensagem vai direto para a tela, com status 400).
type erroRecebimento struct{ msg string }

func (e erroRecebimento) Error() string { return e.msg }

// handleReceiveOrder recebe os itens do pedido: entra no estoque, fecha as
// fichas e marca cada item como recebido. Tudo em uma unica transacao (item 4):
// se qualquer item falhar, nada e gravado.
func (a *App) handleReceiveOrder(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		ItemIDs []int64 `json:"item_ids"`
	}
	_ = readJSON(r, &in)
	only := map[int64]bool{}
	for _, i := range in.ItemIDs {
		only[i] = true
	}

	var status string
	if err := a.db.QueryRow(`SELECT status FROM orders WHERE id=?`, id).Scan(&status); err != nil {
		fail(w, 404, "pedido nao encontrado")
		return
	}
	if status != "aberto" {
		if status == "recebido" {
			fail(w, 409, "Este pedido já foi recebido. Nada foi alterado no estoque.")
		} else {
			fail(w, 409, "Este pedido está "+status+" e não pode ser recebido.")
		}
		return
	}

	rows, err := a.db.Query(`SELECT id,falta_id,part_id,IFNULL(description,''),qty,cost_cents,IFNULL(received_at,'')
		FROM order_items WHERE order_id=?`, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	type oi struct {
		id, faltaID, partID int64
		desc                string
		qty                 float64
		cost                int64
		recebido            string
	}
	list := []oi{}
	for rows.Next() {
		var x oi
		var fid, pid sql.NullInt64
		if rows.Scan(&x.id, &fid, &pid, &x.desc, &x.qty, &x.cost, &x.recebido) == nil {
			if fid.Valid {
				x.faltaID = fid.Int64
			}
			if pid.Valid {
				x.partID = pid.Int64
			}
			if len(only) == 0 || only[x.id] {
				// itens ja recebidos antes nao entram de novo no estoque
				if x.recebido == "" {
					list = append(list, x)
				}
			}
		}
	}
	rows.Close()
	if len(list) == 0 {
		fail(w, 400, "nenhum item pendente para receber neste pedido")
		return
	}

	agora := nowISO()
	recebidos := 0
	err = a.db.Tx(func(tx *sql.Tx) error {
		for _, x := range list {
			pid := x.partID
			if pid == 0 && x.faltaID > 0 {
				var p2 sql.NullInt64
				_ = tx.QueryRow(`SELECT part_id FROM faltas WHERE id=?`, x.faltaID).Scan(&p2)
				if p2.Valid {
					pid = p2.Int64
				}
			}
			if pid > 0 {
				var bal float64
				var cost int64
				if err := tx.QueryRow(`SELECT stock,cost_cents FROM parts WHERE id=?`, pid).Scan(&bal, &cost); err != nil {
					return erroRecebimento{fmt.Sprintf("a peça %d do item \"%s\" não existe mais no cadastro. Nada foi recebido — corrija o pedido e tente de novo.", pid, x.desc)}
				}
				if x.cost > 0 {
					cost = x.cost
					if _, err := tx.Exec(`UPDATE parts SET cost_cents=? WHERE id=?`, x.cost, pid); err != nil {
						return err
					}
				}
				nb := bal + x.qty
				if _, err := tx.Exec(`UPDATE parts SET stock=?, updated_at=? WHERE id=?`, nb, agora, pid); err != nil {
					return err
				}
				if _, err := tx.Exec(`INSERT INTO stock_moves(part_id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,ref_id,created_at)
					VALUES(?,?,?,?,?,?,?,?,?)`, pid, "entrada", x.qty, nb, cost, "recebimento de pedido de compra", "pedido", id, agora); err != nil {
					return err
				}
			}
			// marca o item como recebido (impede entrada duplicada no estoque)
			if _, err := tx.Exec(`UPDATE order_items SET received_at=? WHERE id=?`, agora, x.id); err != nil {
				return err
			}
			if x.faltaID > 0 {
				if _, err := tx.Exec(`UPDATE faltas SET status='recebida', received_at=?, part_id=? WHERE id=?`, agora, pid, x.faltaID); err != nil {
					return err
				}
			}
			recebidos++
		}
		// fecha o pedido quando todos os itens estiverem recebidos (com ou sem ficha)
		var pendentes int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM order_items WHERE order_id=? AND IFNULL(received_at,'')=''`, id).Scan(&pendentes); err != nil {
			return err
		}
		if pendentes == 0 {
			if _, err := tx.Exec(`UPDATE orders SET status='recebido', received_at=? WHERE id=?`, agora, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		var eu erroRecebimento
		if errors.As(err, &eu) {
			fail(w, 400, eu.msg)
			return
		}
		fail(w, 500, "erro ao receber o pedido: nada foi gravado. "+err.Error())
		return
	}

	var pendentes int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM order_items WHERE order_id=? AND IFNULL(received_at,'')=''`, id).Scan(&pendentes)
	writeJSON(w, 200, map[string]any{"ok": true, "recebidos": recebidos, "pendentes": pendentes})
}

func (a *App) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	err := a.db.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE faltas SET status='aberta', order_id=NULL WHERE order_id=? AND status='pedida'`, id); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE orders SET status='cancelado' WHERE id=?`, id)
		return err
	})
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

var _ = fmt.Sprint

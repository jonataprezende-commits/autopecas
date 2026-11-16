package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type StockMoveInput struct {
	PartID  int64   `json:"peca_id"`
	Tipo    string  `json:"tipo"` // entrada | saida | ajuste | baixa
	Qtd     float64 `json:"quantidade"`
	Custo   int64   `json:"custo_centavos"`
	Motivo  string  `json:"motivo"`
	Cliente string  `json:"cliente"`
}

func (a *App) handleStockMove(w http.ResponseWriter, r *http.Request) {
	var in StockMoveInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	if in.PartID == 0 {
		fail(w, 400, "selecione a peca")
		return
	}
	if in.Qtd <= 0 && in.Tipo != "ajuste" {
		fail(w, 400, "informe uma quantidade maior que zero")
		return
	}
	// 1.0.10: cifra digitada com a virgula no lugar errado virava um estoque
	// absurdo. Mesmo limite da venda.
	if in.Qtd > maxQtdItem {
		fail(w, 400, "quantidade acima do limite de 1.000.000 — confira o valor digitado")
		return
	}
	if in.Custo < 0 || in.Custo > maxPrecoCentavos {
		fail(w, 400, "custo inválido (fora da faixa de R$ 0,00 a R$ 10.000.000,00)")
		return
	}
	if in.Tipo == "ajuste" && in.Qtd < 0 {
		fail(w, 400, "o saldo do ajuste nao pode ser negativo")
		return
	}
	if (in.Tipo == "baixa" || in.Tipo == "ajuste") && strings.TrimSpace(in.Motivo) == "" {
		fail(w, 400, "informe o motivo")
		return
	}
	permitirNeg := a.db.GetSetting("permitir_estoque_negativo") != "0"

	err := a.db.Tx(func(tx *sql.Tx) error {
		var bal float64
		var cost int64
		var desc string
		if err := tx.QueryRow(`SELECT stock,cost_cents,description FROM parts WHERE id=?`, in.PartID).Scan(&bal, &cost, &desc); err != nil {
			return fmt.Errorf("peca nao encontrada")
		}
		delta := 0.0
		switch in.Tipo {
		case "entrada":
			delta = in.Qtd
		case "saida", "baixa":
			delta = -in.Qtd
		case "ajuste":
			delta = in.Qtd - bal
		default:
			return fmt.Errorf("tipo de movimento invalido")
		}
		nb := bal + delta
		if nb < 0 && !permitirNeg {
			return fmt.Errorf("estoque insuficiente (saldo atual: %v)", bal)
		}
		if in.Custo > 0 {
			cost = in.Custo
			if _, err := tx.Exec(`UPDATE parts SET cost_cents=? WHERE id=?`, in.Custo, in.PartID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE parts SET stock=?, updated_at=? WHERE id=?`, nb, nowISO(), in.PartID); err != nil {
			return err
		}
		reason := in.Motivo
		if in.Tipo == "entrada" && reason == "" {
			reason = "entrada de compra"
		}
		if in.Tipo == "saida" && reason == "" {
			reason = "saida"
		}
		_, err := tx.Exec(`INSERT INTO stock_moves(part_id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,ref_id,created_at)
			VALUES(?,?,?,?,?,?,?,?,?)`, in.PartID, in.Tipo, delta, nb, cost, reason, "manual", nil, nowISO())
		return err
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	p, _ := a.getPart(in.PartID)
	writeJSON(w, 200, map[string]any{"ok": true, "peca": p})
}

func (a *App) handleStockMoves(w http.ResponseWriter, r *http.Request) {
	limit := 300
	if l := r.URL.Query().Get("limite"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	q := "SELECT m.id,m.part_id,p.code,p.description,p.mfr_code,m.kind,m.qty_delta,m.balance_after,m.unit_cost_cents,m.reason,m.ref_type,m.created_at FROM stock_moves m JOIN parts p ON p.id=m.part_id"
	var args []any
	if pid := r.URL.Query().Get("peca_id"); pid != "" {
		q += " WHERE m.part_id=?"
		args = append(args, pid)
	}
	if d := r.URL.Query().Get("data"); d != "" {
		if len(args) > 0 {
			q += " AND"
		} else {
			q += " WHERE"
		}
		q += " DATE(m.created_at)=?"
		args = append(args, d)
	}
	q += " ORDER BY m.id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := a.db.Query(q, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, pid, cost int64
		var code, desc, mfr, kind, reason, ref, created string
		var delta, bal float64
		if rows.Scan(&id, &pid, &code, &desc, &mfr, &kind, &delta, &bal, &cost, &reason, &ref, &created) != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "peca_id": pid, "codigo": code, "descricao": desc, "codigo_fabricante": mfr,
			"tipo": kind, "quantidade": delta, "saldo": bal, "custo_centavos": cost,
			"motivo": reason, "origem": ref, "data": created,
		})
	}
	writeJSON(w, 200, map[string]any{"itens": out})
}

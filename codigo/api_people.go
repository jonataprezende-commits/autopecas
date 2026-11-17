package main

import (
	"net/http"
	"strconv"
	"strings"
)

// ---------- fornecedores ----------

func (a *App) handleListSuppliers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sqlText := `SELECT s.id,s.name,IFNULL(s.contact,''),IFNULL(s.phone,''),IFNULL(s.email,''),IFNULL(s.notes,''),
		IFNULL(s.prazo_entrega_dias,0),
		(SELECT COUNT(*) FROM faltas f WHERE f.supplier_id=s.id AND f.status IN ('aberta','pedida')),
		(SELECT COUNT(*) FROM parts p WHERE p.supplier_id=s.id AND p.active=1)
		FROM suppliers s`
	var args []any
	if q != "" {
		sqlText += " WHERE lower(s.name) LIKE ? OR lower(IFNULL(s.contact,'')) LIKE ? OR IFNULL(s.phone,'') LIKE ?"
		lq := "%" + strings.ToLower(q) + "%"
		args = append(args, lq, lq, lq)
	}
	sqlText += " ORDER BY s.name COLLATE NOCASE"
	rows, err := a.db.Query(sqlText, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, contact, phone, email, notes string
		var prazo int
		var fichas, pecas int
		if rows.Scan(&id, &name, &contact, &phone, &email, &notes, &prazo, &fichas, &pecas) == nil {
			out = append(out, map[string]any{"id": id, "nome": name, "contato": contact, "telefone": phone,
				"email": email, "observacoes": notes, "prazo_entrega_dias": prazo,
				"fichas_abertas": fichas, "pecas": pecas})
		}
	}
	writeJSON(w, 200, map[string]any{"itens": out})
}

type SupplierInput struct {
	Nome             string `json:"nome"`
	Contato          string `json:"contato"`
	Telefone         string `json:"telefone"`
	Email            string `json:"email"`
	Observacoes      string `json:"observacoes"`
	PrazoEntregaDias int    `json:"prazo_entrega_dias"`
}

func (a *App) handleSaveSupplier(w http.ResponseWriter, r *http.Request) {
	var in SupplierInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	in.Nome = strings.TrimSpace(in.Nome)
	if in.Nome == "" {
		fail(w, 400, "informe o nome do fornecedor")
		return
	}
	if in.PrazoEntregaDias < 0 {
		in.PrazoEntregaDias = 0
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id == 0 {
		res, err := a.db.Exec(`INSERT INTO suppliers(name,contact,phone,email,notes,prazo_entrega_dias,created_at) VALUES(?,?,?,?,?,?,?)`,
			in.Nome, in.Contato, in.Telefone, in.Email, in.Observacoes, in.PrazoEntregaDias, nowISO())
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		id, _ = res.LastInsertId()
	} else {
		if _, err := a.db.Exec(`UPDATE suppliers SET name=?,contact=?,phone=?,email=?,notes=?,prazo_entrega_dias=? WHERE id=?`,
			in.Nome, in.Contato, in.Telefone, in.Email, in.Observacoes, in.PrazoEntregaDias, id); err != nil {
			fail(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "id": id})
}

func (a *App) handleDeleteSupplier(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	_, _ = a.db.Exec(`UPDATE parts SET supplier_id=NULL WHERE supplier_id=?`, id)
	_, _ = a.db.Exec(`UPDATE faltas SET supplier_id=NULL WHERE supplier_id=?`, id)
	if _, err := a.db.Exec(`DELETE FROM suppliers WHERE id=?`, id); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- clientes ----------

func (a *App) handleListCustomers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 300
	sqlText := `SELECT c.id,c.name,IFNULL(c.phone,''),IFNULL(c.doc,''),IFNULL(c.address,''),IFNULL(c.notes,''),
		(SELECT COUNT(*) FROM vehicles v WHERE v.customer_id=c.id)
		FROM customers c`
	var args []any
	if q != "" {
		lq := "%" + strings.ToLower(q) + "%"
		sqlText += ` WHERE lower(c.name) LIKE ? OR IFNULL(c.phone,'') LIKE ? OR IFNULL(c.doc,'') LIKE ?
			OR EXISTS(SELECT 1 FROM vehicles v WHERE v.customer_id=c.id AND (lower(v.plate) LIKE ? OR lower(v.model) LIKE ?))`
		args = append(args, lq, lq, lq, lq, lq)
	}
	sqlText += " ORDER BY c.name COLLATE NOCASE LIMIT ?"
	args = append(args, limit)
	rows, err := a.db.Query(sqlText, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, phone, doc, addr, notes string
		var nveh int
		if rows.Scan(&id, &name, &phone, &doc, &addr, &notes, &nveh) == nil {
			out = append(out, map[string]any{"id": id, "nome": name, "telefone": phone, "documento": doc,
				"endereco": addr, "observacoes": notes, "qtd_veiculos": nveh})
		}
	}
	writeJSON(w, 200, map[string]any{"itens": out})
}

type CustomerInput struct {
	Nome        string `json:"nome"`
	Telefone    string `json:"telefone"`
	Documento   string `json:"documento"`
	Endereco    string `json:"endereco"`
	Observacoes string `json:"observacoes"`
}

func (a *App) handleSaveCustomer(w http.ResponseWriter, r *http.Request) {
	var in CustomerInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	in.Nome = strings.TrimSpace(in.Nome)
	if in.Nome == "" {
		fail(w, 400, "informe o nome do cliente")
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id == 0 {
		res, err := a.db.Exec(`INSERT INTO customers(name,phone,doc,address,notes,created_at) VALUES(?,?,?,?,?,?)`,
			in.Nome, in.Telefone, in.Documento, in.Endereco, in.Observacoes, nowISO())
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		id, _ = res.LastInsertId()
	} else {
		if _, err := a.db.Exec(`UPDATE customers SET name=?,phone=?,doc=?,address=?,notes=? WHERE id=?`,
			in.Nome, in.Telefone, in.Documento, in.Endereco, in.Observacoes, id); err != nil {
			fail(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "id": id})
}

func (a *App) handleDeleteCustomer(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := a.db.Exec(`DELETE FROM customers WHERE id=?`, id); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleGetCustomer(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var name, phone, doc, addr, notes, created string
	if err := a.db.QueryRow(`SELECT name,IFNULL(phone,''),IFNULL(doc,''),IFNULL(address,''),IFNULL(notes,''),created_at
		FROM customers WHERE id=?`, id).Scan(&name, &phone, &doc, &addr, &notes, &created); err != nil {
		fail(w, 404, "cliente nao encontrado")
		return
	}
	// veiculos
	vehicles := []map[string]any{}
	vrows, err := a.db.Query(`SELECT id,IFNULL(plate,''),IFNULL(model,''),IFNULL(year,''),IFNULL(notes,'') FROM vehicles WHERE customer_id=? ORDER BY id`, id)
	if err == nil {
		for vrows.Next() {
			var vid int64
			var plate, model, year, vnotes string
			if vrows.Scan(&vid, &plate, &model, &year, &vnotes) == nil {
				vehicles = append(vehicles, map[string]any{"id": vid, "placa": plate, "modelo": model, "ano": year, "observacoes": vnotes})
			}
		}
		vrows.Close()
	}
	// compras
	sales := []map[string]any{}
	srows, err := a.db.Query(`SELECT id,created_at,total_cents,payment,IFNULL(vehicle_plate,''),IFNULL(vehicle_model,''),status
		FROM sales WHERE customer_id=? ORDER BY id DESC LIMIT 100`, id)
	if err == nil {
		for srows.Next() {
			var sid, total int64
			var dt, pay, plate, model, st string
			if srows.Scan(&sid, &dt, &total, &pay, &plate, &model, &st) == nil {
				sales = append(sales, map[string]any{"id": sid, "data": dt, "total_centavos": total,
					"forma_pagamento": pay, "forma_pagamento_nome": pagamentos[pay], "veiculo_placa": plate,
					"veiculo_modelo": model, "status": st})
			}
		}
		srows.Close()
	}
	// fichas
	faltas := []map[string]any{}
	frows, err := a.db.Query(`SELECT id,created_at,description,qty,status,IFNULL(vehicle,'') FROM faltas WHERE customer_id=? ORDER BY id DESC LIMIT 100`, id)
	if err == nil {
		for frows.Next() {
			var fid int64
			var dt, desc, st, veh string
			var qty float64
			if frows.Scan(&fid, &dt, &desc, &qty, &st, &veh) == nil {
				faltas = append(faltas, map[string]any{"id": fid, "data": dt, "descricao": desc, "quantidade": qty, "status": st, "veiculo": veh})
			}
		}
		frows.Close()
	}
	writeJSON(w, 200, map[string]any{"id": id, "nome": name, "telefone": phone, "documento": doc,
		"endereco": addr, "observacoes": notes, "criado": created,
		"veiculos": vehicles, "compras": sales, "fichas": faltas})
}

type VehicleInput struct {
	Placa       string `json:"placa"`
	Modelo      string `json:"modelo"`
	Ano         string `json:"ano"`
	Observacoes string `json:"observacoes"`
}

func (a *App) handleSaveVehicle(w http.ResponseWriter, r *http.Request) {
	custID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in VehicleInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	if strings.TrimSpace(in.Modelo) == "" && strings.TrimSpace(in.Placa) == "" {
		fail(w, 400, "informe pelo menos a placa ou o modelo")
		return
	}
	res, err := a.db.Exec(`INSERT INTO vehicles(customer_id,plate,model,year,notes,created_at) VALUES(?,?,?,?,?,?)`,
		custID, strings.ToUpper(strings.TrimSpace(in.Placa)), in.Modelo, in.Ano, in.Observacoes, nowISO())
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 200, map[string]any{"ok": true, "id": id})
}

func (a *App) handleDeleteVehicle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := a.db.Exec(`DELETE FROM vehicles WHERE id=?`, id); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

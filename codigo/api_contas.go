package main

// Contas a pagar (o dinheiro que sai) + historico das notas de entrada.
//
// Nasce da importacao da nota do fornecedor: o XML traz o emitente, o numero, o
// valor total (vNF) e as duplicatas (boletos). Aqui ficam as rotas que listam,
// baixam (pagam), editam e cancelam essas contas - e tambem o lancamento manual,
// para a despesa que nao vem de nota (aluguel, luz, salario...).

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ContaPagar struct {
	ID           int64  `json:"id"`
	FornecedorID int64  `json:"fornecedor_id"`
	Fornecedor   string `json:"fornecedor"`
	Cnpj         string `json:"cnpj"`
	Documento    string `json:"documento"`
	Parcela      string `json:"parcela"`
	Observacao   string `json:"observacao"`
	ValorCents   int64  `json:"valor_centavos"`
	PagoCents    int64  `json:"pago_centavos"`
	Vencimento   string `json:"vencimento"`
	Status       string `json:"status"`
	Origem       string `json:"origem"`
	Criado       string `json:"criado"`
	PagoEm       string `json:"pago_em"`
}

type contaInput struct {
	FornecedorID *int64 `json:"fornecedor_id"`
	Fornecedor   string `json:"fornecedor"`
	Documento    string `json:"documento"`
	Parcela      string `json:"parcela"`
	Observacao   string `json:"observacao"`
	Valor        int64  `json:"valor_centavos"`
	Vencimento   string `json:"vencimento"` // AAAA-MM-DD
	ValorPago    int64  `json:"valor_pago_centavos"`
}

const contaCols = `p.id,IFNULL(p.supplier_id,0),IFNULL(p.supplier_name,''),IFNULL(p.supplier_doc,''),
	IFNULL(p.doc_number,''),IFNULL(p.parcela,''),IFNULL(p.note,''),p.amount_cents,
	IFNULL(p.paid_cents,0),p.due_date,p.status,IFNULL(p.origin,''),IFNULL(p.created_at,''),IFNULL(p.paid_at,'')`

func scanConta(rows *sql.Rows) (ContaPagar, error) {
	var c ContaPagar
	err := rows.Scan(&c.ID, &c.FornecedorID, &c.Fornecedor, &c.Cnpj, &c.Documento, &c.Parcela,
		&c.Observacao, &c.ValorCents, &c.PagoCents, &c.Vencimento, &c.Status, &c.Origem, &c.Criado, &c.PagoEm)
	return c, err
}

// handleListPayables lista as contas. status: aberto (padrao) | pago | vencidos | todos
func (a *App) handleListPayables(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	q := `SELECT ` + contaCols + ` FROM payables p WHERE 1=1`
	var args []any
	switch status {
	case "", "aberto":
		q += " AND p.status='aberto'"
	case "todos":
		// tudo
	case "vencidos":
		q += " AND p.status='aberto' AND p.due_date < ?"
		args = append(args, todayISO())
	default:
		q += " AND p.status=?"
		args = append(args, status)
	}
	q += " ORDER BY (p.status='aberto') DESC, p.due_date, p.id"
	rows, err := a.db.Query(q, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	hoje := todayISO()
	out := []ContaPagar{}
	var totalAberto, totalVencido int64
	var qtdVencidas int
	for rows.Next() {
		c, err := scanConta(rows)
		if err != nil {
			continue
		}
		if c.Status == "aberto" {
			totalAberto += c.ValorCents
			if c.Vencimento < hoje {
				totalVencido += c.ValorCents
				qtdVencidas++
			}
		}
		out = append(out, c)
	}
	writeJSON(w, 200, map[string]any{
		"itens": out, "total_aberto_centavos": totalAberto,
		"total_vencido_centavos": totalVencido, "qtd_vencidas": qtdVencidas, "hoje": hoje,
	})
}

// handleSavePayable cria (sem id) ou edita (com id) uma conta a pagar manual.
func (a *App) handleSavePayable(w http.ResponseWriter, r *http.Request) {
	var in contaInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	in.Fornecedor = strings.TrimSpace(in.Fornecedor)
	if in.Fornecedor == "" && in.FornecedorID != nil && *in.FornecedorID > 0 {
		_ = a.db.QueryRow(`SELECT name FROM suppliers WHERE id=?`, *in.FornecedorID).Scan(&in.Fornecedor)
	}
	if in.Fornecedor == "" {
		fail(w, 400, "informe o fornecedor (ou a descrição da despesa)")
		return
	}
	if in.Valor <= 0 {
		fail(w, 400, "o valor da conta tem que ser maior que zero")
		return
	}
	venc := strings.TrimSpace(in.Vencimento)
	if v := dataISO(venc); v != "" {
		venc = v
	}
	if _, err := time.ParseInLocation("2006-01-02", venc, time.Local); err != nil {
		fail(w, 400, "vencimento inválido (use AAAA-MM-DD)")
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id == 0 {
		res, err := a.db.Exec(`INSERT INTO payables(supplier_id,supplier_name,supplier_doc,doc_number,parcela,note,
			amount_cents,due_date,status,origin,created_at) VALUES(?,?,?,?,?,?,?,?,'aberto','manual',?)`,
			in.FornecedorID, in.Fornecedor, apenasDigitos(in.Documento), "",
			in.Parcela, in.Observacao, in.Valor, venc, nowISO())
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		id, _ = res.LastInsertId()
		writeJSON(w, 200, map[string]any{"ok": true, "id": id})
		return
	}
	// editar: so conta em aberto (conta paga nao muda de valor)
	res, err := a.db.Exec(`UPDATE payables SET supplier_name=?, supplier_doc=?, note=?, amount_cents=?, due_date=?
		WHERE id=? AND status='aberto'`, in.Fornecedor, apenasDigitos(in.Documento), in.Observacao, in.Valor, venc, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 400, "conta não encontrada ou já paga/cancelada")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "id": id})
}

// handlePayPayable baixa a conta (paga). Aceita valor pago diferente do valor
// da conta (juros/desconto) - o valor do titulo nao muda, o pago fica gravado.
func (a *App) handlePayPayable(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		ValorPago int64  `json:"valor_pago_centavos"`
		PagoEm    string `json:"pago_em"`
	}
	_ = readJSON(r, &in)
	var valor int64
	if err := a.db.QueryRow(`SELECT amount_cents FROM payables WHERE id=? AND status='aberto'`, id).Scan(&valor); err != nil {
		fail(w, 400, "conta não encontrada ou já encerrada")
		return
	}
	pago := in.ValorPago
	if pago <= 0 {
		pago = valor
	}
	quando := nowISO()
	if d := dataISO(in.PagoEm); d != "" {
		quando = d + " " + time.Now().Format("15:04:05")
	}
	res, err := a.db.Exec(`UPDATE payables SET status='pago', paid_at=?, paid_cents=? WHERE id=? AND status='aberto'`,
		quando, pago, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 400, "conta não encontrada ou já encerrada")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "pago_centavos": pago})
}

// handleCancelPayable cancela uma conta (nao apaga: fica o historico)
func (a *App) handleCancelPayable(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	res, err := a.db.Exec(`UPDATE payables SET status='cancelado' WHERE id=? AND status='aberto'`, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 400, "conta não encontrada ou já paga")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleListNotasEntrada: historico das notas que entraram no estoque.
func (a *App) handleListNotasEntrada(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT n.id,IFNULL(n.chave,''),IFNULL(n.numero,''),IFNULL(n.serie,''),
		IFNULL(n.supplier_id,0),IFNULL(n.supplier_name,''),IFNULL(n.supplier_doc,''),IFNULL(n.emitida_em,''),
		n.produtos_cents,n.extras_cents,n.total_cents,n.rateado_cents,n.itens,IFNULL(n.arquivo,''),n.created_at,
		(SELECT COUNT(*) FROM payables p WHERE p.import_id=n.id),
		(SELECT IFNULL(SUM(p.amount_cents),0) FROM payables p WHERE p.import_id=n.id)
		FROM notas_entrada n ORDER BY n.id DESC LIMIT 200`)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, supID, produtos, extras, total, rateado, qtdContas, totalContas int64
		var chave, numero, serie, nome, doc, emitida, arquivo, criado string
		var itens int64
		if rows.Scan(&id, &chave, &numero, &serie, &supID, &nome, &doc, &emitida,
			&produtos, &extras, &total, &rateado, &itens, &arquivo, &criado,
			&qtdContas, &totalContas) != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "chave": chave, "numero": numero, "serie": serie,
			"fornecedor_id": supID, "fornecedor": nome, "cnpj": doc, "emitida_em": emitida,
			"produtos_centavos": produtos, "extras_centavos": extras, "total_centavos": total,
			"rateado_centavos": rateado, "itens": itens, "arquivo": arquivo, "criado": criado,
			"contas": qtdContas, "contas_centavos": totalContas,
		})
	}
	writeJSON(w, 200, map[string]any{"itens": out})
}

// ---------- o que a importacao usa ----------

// acharOuCriarFornecedor casa pelo CNPJ (ou pelo nome, sem duplicar) e devolve o id.
func (a *App) acharOuCriarFornecedor(nome, doc string, criar bool) int64 {
	nome = strings.TrimSpace(nome)
	doc = apenasDigitos(doc)
	if doc != "" {
		var id int64
		// compara so os digitos, porque o cadastro pode ter ponto/barra/traco
		rows, err := a.db.Query(`SELECT id, IFNULL(doc,'') FROM suppliers`)
		if err == nil {
			for rows.Next() {
				var i int64
				var d string
				if rows.Scan(&i, &d) == nil && apenasDigitos(d) == doc {
					id = i
					break
				}
			}
			rows.Close()
		}
		if id > 0 {
			// completo o CNPJ se estava vazio
			if _, err := a.db.Exec(`UPDATE suppliers SET doc=? WHERE id=? AND IFNULL(doc,'')=''`, doc, id); err == nil {
				return id
			}
			return id
		}
	}
	if nome != "" {
		var id int64
		if err := a.db.QueryRow(`SELECT id FROM suppliers WHERE lower(trim(name))=lower(trim(?))`, nome).Scan(&id); err == nil && id > 0 {
			if doc != "" {
				_, _ = a.db.Exec(`UPDATE suppliers SET doc=? WHERE id=? AND IFNULL(doc,'')=''`, doc, id)
			}
			return id
		}
	}
	if !criar || nome == "" {
		return 0
	}
	res, err := a.db.Exec(`INSERT INTO suppliers(name,contact,phone,email,notes,doc,created_at) VALUES(?,'','','','',?,?)`,
		nome, doc, nowISO())
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	return id
}

// notaJaImportada procura uma nota igual: pela chave de acesso (certeza) ou,
// sem chave, pelo fornecedor + numero + total.
func (a *App) notaJaImportada(n NotaInfo, fornecedorDoc, fornecedorNome string) int64 {
	if n.Chave != "" {
		var id int64
		if err := a.db.QueryRow(`SELECT id FROM notas_entrada WHERE chave=?`, n.Chave).Scan(&id); err == nil {
			return id
		}
		return 0
	}
	if n.TemTotal && (fornecedorDoc != "" || fornecedorNome != "") {
		q := `SELECT id FROM notas_entrada WHERE total_cents=? AND IFNULL(numero,'')=? AND (IFNULL(supplier_doc,'')=? OR lower(IFNULL(supplier_name,''))=lower(?))`
		var id int64
		if err := a.db.QueryRow(q, n.TotalCents, n.Numero, apenasDigitos(fornecedorDoc), strings.TrimSpace(fornecedorNome)).Scan(&id); err == nil {
			return id
		}
	}
	return 0
}

// vencimentoPadrao devolve a data (AAAA-MM-DD) de emitidaEm + prazoDias.
func vencimentoPadrao(emitidaEm string, prazoDias int) string {
	base := time.Now()
	if t, ok := dataLocal(emitidaEm); ok {
		base = t
	}
	return base.AddDate(0, 0, prazoDias).Format("2006-01-02")
}

// reconciliarBoletos confere a soma das duplicatas contra o total da nota.
//
// Decisao (1.0.10): a diferenca de centavos (arredondamento do proprio XML)
// e' acertada na ultima parcela; diferenca maior NAO e' inventada - o sistema
// lanca exatamente os boletos do arquivo e avisa, porque criar um boleto que
// o fornecedor nao emitiu (ou engordar um existente) seria pior que avisar.
func reconciliarBoletos(nota NotaInfo, contas []Duplicata) ([]Duplicata, []string) {
	avisos := []string{}
	if !nota.TemTotal || nota.TotalCents <= 0 || len(contas) == 0 {
		return contas, avisos
	}
	var soma int64
	for _, c := range contas {
		soma += c.ValorCents
	}
	dif := nota.TotalCents - soma
	switch {
	case dif == 0:
	case dif > -5 && dif < 5:
		ultimo := len(contas) - 1
		contas[ultimo].ValorCents += dif
		avisos = append(avisos, fmt.Sprintf(
			"Os boletos do XML somavam %s e o total da nota é %s: ajustei %s na última parcela (arredondamento do arquivo).",
			fmtMoney(soma), fmtMoney(nota.TotalCents), fmtMoney(dif)))
	case dif > 0:
		avisos = append(avisos, fmt.Sprintf(
			"ATENÇÃO: os boletos do XML somam %s, mas a nota é de %s — faltam %s. Lancei só os boletos que vieram no arquivo; confira no portal do fornecedor antes de pagar.",
			fmtMoney(soma), fmtMoney(nota.TotalCents), fmtMoney(dif)))
	default:
		avisos = append(avisos, fmt.Sprintf(
			"ATENÇÃO: os boletos do XML somam %s, mais que o total da nota (%s) — diferença de %s. Lancei exatamente o que veio no arquivo; confira antes de pagar.",
			fmtMoney(soma), fmtMoney(nota.TotalCents), fmtMoney(-dif)))
	}
	return contas, avisos
}

// registrarNotaEmitirContas grava o cabecalho da nota e cria as contas a pagar
// (uma por boleto). Se a nota ja entrou antes, nao cria nada de novo: devolve
// jaExistia=true para o usuario nao lancar a despesa duas vezes.
func (a *App) registrarNotaEmitirContas(nota NotaInfo, res *ImportResult, opts importOptions, fornecedorID int64, arquivo string, prazo int) ([]Duplicata, bool, error) {
	if jaID := a.notaJaImportada(nota, nota.EmitenteDoc, nota.EmitenteNome); jaID > 0 {
		return nil, true, nil
	}

	contas := []Duplicata{}
	for i, d := range nota.Duplicatas {
		if d.ValorCents <= 0 {
			continue
		}
		venc := d.Vencimento
		if venc == "" {
			if v := dataISO(opts.Vencimento); v != "" {
				venc = v
			} else {
				venc = vencimentoPadrao(nota.EmitidaEm, prazo)
			}
		}
		num := d.Numero
		if num == "" {
			num = strconv.Itoa(i + 1)
		}
		contas = append(contas, Duplicata{Numero: num, Vencimento: venc, ValorCents: d.ValorCents})
	}
	if len(contas) == 0 {
		return nil, false, nil
	}
	contas, avisos := reconciliarBoletos(nota, contas)
	if res != nil {
		res.Avisos = append(res.Avisos, avisos...)
	}

	var fornecedorNome string
	if fornecedorID > 0 {
		_ = a.db.QueryRow(`SELECT name FROM suppliers WHERE id=?`, fornecedorID).Scan(&fornecedorNome)
	}
	if fornecedorNome == "" {
		fornecedorNome = nota.EmitenteNome
	}

	err := a.db.Tx(func(tx *sql.Tx) error {
		idNota64, err := tx.Exec(`INSERT INTO notas_entrada(chave,numero,serie,supplier_id,supplier_name,supplier_doc,
			emitida_em,produtos_cents,extras_cents,total_cents,rateado_cents,itens,arquivo,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			nota.Chave, nota.Numero, nota.Serie, fornecedorID, fornecedorNome, apenasDigitos(nota.EmitenteDoc),
			nota.EmitidaEm, nota.ProdutosCents, nota.ExtrasCents, nota.TotalCents, res.RateadoCents,
			res.Cadastradas+res.Atualizadas, arquivo, nowISO())
		if err != nil {
			return err
		}
		notaID, _ := idNota64.LastInsertId()
		for _, c := range contas {
			parcela := ""
			if len(contas) > 1 {
				parcela = c.Numero + "/" + strconv.Itoa(len(contas))
			}
			obs := "Nota " + strings.TrimSpace(nota.Numero)
			if nota.Serie != "" {
				obs += " série " + nota.Serie
			}
			if _, err := tx.Exec(`INSERT INTO payables(supplier_id,supplier_name,supplier_doc,doc_number,doc_serie,
				doc_key,parcela,note,amount_cents,due_date,status,origin,import_id,created_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,'aberto','nota',?,?)`,
				fornecedorID, fornecedorNome, apenasDigitos(nota.EmitenteDoc), nota.Numero, nota.Serie,
				nota.Chave, parcela, obs, c.ValorCents, c.Vencimento, notaID, nowISO()); err != nil {
				return err
			}
		}
		return nil
	})
	return contas, false, err
}

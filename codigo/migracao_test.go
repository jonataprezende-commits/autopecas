package main

// Teste do item 10: versionamento de esquema com PRAGMA user_version.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// criarBanco100 gera em disco um banco exatamente como o AutoPecas 1.0.0 criava
// (esquema + as duas colunas adicionadas por ALTER TABLE), sem user_version
// e ja com dados dentro.
func criarBanco100(t *testing.T, caminho string) {
	t.Helper()
	db, err := OpenDB(caminho)
	if err != nil {
		t.Fatalf("abrindo banco 1.0.0: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("criando esquema 1.0.0: %v", err)
	}
	// o migrate() do 1.0.0 rodava estes dois ALTER TABLE ignorando o erro
	db.Exec(`ALTER TABLE parts ADD COLUMN code_norm TEXT DEFAULT ''`)
	db.Exec(`ALTER TABLE parts ADD COLUMN mfr_norm TEXT DEFAULT ''`)

	passos := []string{
		`INSERT INTO parts(id,code,code_norm,mfr_code,mfr_norm,description,cost_cents,price_cents,min_stock,stock,created_at,updated_at)
		 VALUES(1,'OLD-1','OLD1','','','Peca antiga',1000,2500,1,8,'2025-01-01 10:00:00','2025-01-01 10:00:00')`,
		`INSERT INTO customers(id,name,created_at) VALUES(1,'Cliente antigo','2025-01-01 10:00:00')`,
		`INSERT INTO suppliers(id,name,created_at) VALUES(1,'Fornecedor antigo','2025-01-01 10:00:00')`,
		`INSERT INTO sales(id,created_at,customer_id,customer_name,subtotal_cents,discount_cents,total_cents,cost_cents,payment,received_cents,status)
		 VALUES(1,'2025-01-02 09:00:00',1,'Cliente antigo',2500,0,2500,1000,'dinheiro',2500,'concluida')`,
		`INSERT INTO sale_items(sale_id,part_id,code,description,qty,unit_price_cents,unit_cost_cents,discount_cents,total_cents)
		 VALUES(1,1,'OLD-1','Peca antiga',1,2500,1000,0,2500)`,
		`INSERT INTO stock_moves(part_id,kind,qty_delta,balance_after,unit_cost_cents,reason,ref_type,ref_id,created_at)
		 VALUES(1,'entrada',8,8,1000,'estoque inicial','cadastro',1,'2025-01-01 10:00:00')`,
		`INSERT INTO faltas(id,created_at,part_id,description,qty,customer_name,status,cost_cents)
		 VALUES(1,'2025-01-03 08:00:00',1,'Peca antiga',2,'Cliente antigo','pedida',1000)`,
		`INSERT INTO orders(id,created_at,supplier_id,supplier_name,status,notes,total_cents)
		 VALUES(1,'2025-01-03 08:00:00',1,'Fornecedor antigo','aberto','',2000)`,
		`INSERT INTO order_items(id,order_id,falta_id,part_id,description,qty,cost_cents)
		 VALUES(1,1,1,1,'Peca antiga',2,1000)`,
		`INSERT INTO receivables(id,sale_id,customer_id,customer_name,amount_cents,due_date,status,created_at)
		 VALUES(1,1,1,'Cliente antigo',2500,'2025-02-01','aberto','2025-01-02 09:00:00')`,
		`INSERT INTO settings(key,value) VALUES('loja_nome','Loja Antiga')`,
	}
	for _, q := range passos {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("populando banco 1.0.0 (%s): %v", q[:40], err)
		}
	}
}

func versaoBanco(t *testing.T, env *ambiente) int64 {
	t.Helper()
	return env.qint(`PRAGMA user_version`)
}

func temTabela(t *testing.T, env *ambiente, tabela string) bool {
	t.Helper()
	var n int64
	if err := env.app.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tabela).Scan(&n); err != nil {
		t.Fatalf("consultando sqlite_master: %v", err)
	}
	return n > 0
}

func temColuna(t *testing.T, env *ambiente, tabela, coluna string) bool {
	t.Helper()
	rows, err := env.app.db.Query(`PRAGMA table_info(` + tabela + `)`)
	if err != nil {
		t.Fatalf("table_info(%s): %v", tabela, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var nome, tipo string
		var dflt any
		if rows.Scan(&cid, &nome, &tipo, &notnull, &dflt, &pk) == nil && nome == coluna {
			return true
		}
	}
	return false
}

func TestItem10Banco100MigraSemPerderDados(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0o755); err != nil {
		t.Fatalf("preparando pasta: %v", err)
	}
	banco := filepath.Join(dir, "autopecas.db")
	criarBanco100(t, banco)

	db, err := OpenDB(banco)
	if err != nil {
		t.Fatalf("abrindo banco para migrar: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := &App{db: db, dir: dir, hostEscuta: "127.0.0.1"}

	if err := app.migrate(); err != nil {
		t.Fatalf("migracao de banco 1.0.0 falhou: %v", err)
	}
	env := &ambiente{t: t, app: app, dir: dir}

	if v := versaoBanco(t, env); v != versaoEsquema {
		t.Errorf("user_version = %d, esperado %d", v, versaoEsquema)
	}
	if !temColuna(t, env, "parts", "avg_cost_cents") {
		t.Errorf("coluna parts.avg_cost_cents (custo medio) nao foi criada na migracao 7")
	}
	if !temColuna(t, env, "order_items", "received_at") {
		t.Errorf("coluna order_items.received_at nao foi criada")
	}
	if !temColuna(t, env, "parts", "application") {
		t.Errorf("coluna parts.application (aplicacao) nao foi criada na migracao 3")
	}
	if !temTabela(t, env, "catalog_fts") {
		t.Errorf("tabela catalog_fts (indice de busca do catalogo) nao foi criada na migracao 5")
	}
	if !temColuna(t, env, "catalog", "criteria") {
		t.Errorf("coluna catalog.criteria (criterios tecnicos) nao foi criada na migracao 4")
	}
	if !temTabela(t, env, "catalog") {
		t.Errorf("tabela catalog (catalogo do fornecedor) nao foi criada na migracao 3")
	}
	// nenhum dado perdido
	conferencias := map[string]struct {
		consulta string
		esperado int64
	}{
		"pecas":      {`SELECT COUNT(*) FROM parts`, 1},
		"clientes":   {`SELECT COUNT(*) FROM customers`, 1},
		"vendas":     {`SELECT COUNT(*) FROM sales`, 1},
		"itens":      {`SELECT COUNT(*) FROM sale_items`, 1},
		"fichas":     {`SELECT COUNT(*) FROM faltas`, 1},
		"pedidos":    {`SELECT COUNT(*) FROM orders`, 1},
		"itens_ped":  {`SELECT COUNT(*) FROM order_items`, 1},
		"recebiveis": {`SELECT COUNT(*) FROM receivables`, 1},
		"estoque":    {`SELECT CAST(stock AS INTEGER) FROM parts WHERE id=1`, 8},
		"config":     {`SELECT COUNT(*) FROM settings WHERE key='loja_nome' AND value='Loja Antiga'`, 1},
	}
	for nome, c := range conferencias {
		if got := env.qint(c.consulta); got != c.esperado {
			t.Errorf("%s: %d, esperado %d", nome, got, c.esperado)
		}
	}

	// backup de seguranca antes de migrar
	ents, err := os.ReadDir(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatalf("lendo backups: %v", err)
	}
	achou := false
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "antes-de-migrar") && strings.HasSuffix(e.Name(), ".db") {
			achou = true
		}
	}
	if !achou {
		t.Errorf("nenhum backup 'antes-de-migrar-*.db' criado antes da migracao")
	}

	// idempotente: rodar de novo nao muda nada
	if err := app.migrate(); err != nil {
		t.Fatalf("segunda migracao falhou: %v", err)
	}
	if v := versaoBanco(t, env); v != versaoEsquema {
		t.Errorf("user_version depois da segunda migracao = %d, esperado %d", v, versaoEsquema)
	}
	if got := env.qint(`SELECT COUNT(*) FROM parts`); got != 1 {
		t.Errorf("dados alterados na segunda migracao: pecas=%d", got)
	}
	if n := env.qint(`SELECT COUNT(*) FROM order_items WHERE IFNULL(received_at,'x')='x'`); n != 0 {
		t.Errorf("coluna received_at nao tem valor padrao vazio em %d linhas", n)
	}
}

func TestItem10BancoNovoJaNasceNaVersaoAtual(t *testing.T) {
	env := novaAppTeste(t)
	if v := versaoBanco(t, env); v != versaoEsquema {
		t.Errorf("banco novo com user_version %d, esperado %d", v, versaoEsquema)
	}
	if !temColuna(t, env, "parts", "avg_cost_cents") {
		t.Errorf("banco novo sem a coluna parts.avg_cost_cents")
	}
	if !temColuna(t, env, "order_items", "received_at") {
		t.Errorf("banco novo sem a coluna order_items.received_at")
	}
	if !temColuna(t, env, "parts", "application") {
		t.Errorf("banco novo sem a coluna parts.application")
	}
	if !temTabela(t, env, "catalog") {
		t.Errorf("banco novo sem a tabela catalog")
	}
}

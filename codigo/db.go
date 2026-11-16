package main

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

func OpenDB(path string) (*DB, error) {
	dsn := path + "?_pragma=busy_timeout(8000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)"
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sdb.SetMaxOpenConns(1)
	if err := sdb.Ping(); err != nil {
		return nil, err
	}
	return &DB{sql: sdb}, nil
}

func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) Exec(q string, args ...any) (sql.Result, error) { return d.sql.Exec(q, args...) }
func (d *DB) Query(q string, args ...any) (*sql.Rows, error) { return d.sql.Query(q, args...) }
func (d *DB) QueryRow(q string, args ...any) *sql.Row        { return d.sql.QueryRow(q, args...) }

func (d *DB) Tx(fn func(tx *sql.Tx) error) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

const schema = `
CREATE TABLE IF NOT EXISTS parts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  code TEXT NOT NULL UNIQUE,
  code_norm TEXT DEFAULT '',
  mfr_code TEXT DEFAULT '',
  mfr_norm TEXT DEFAULT '',
  description TEXT NOT NULL,
  brand TEXT DEFAULT '',
  category TEXT DEFAULT '',
  cost_cents INTEGER NOT NULL DEFAULT 0,
  price_cents INTEGER NOT NULL DEFAULT 0,
  min_stock REAL NOT NULL DEFAULT 0,
  stock REAL NOT NULL DEFAULT 0,
  location TEXT DEFAULT '',
  supplier_id INTEGER,
  active INTEGER NOT NULL DEFAULT 1,
  search_blob TEXT DEFAULT '',
  application TEXT DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_parts_code ON parts(code);
CREATE INDEX IF NOT EXISTS idx_parts_mfr ON parts(mfr_code);
CREATE INDEX IF NOT EXISTS idx_parts_code_norm ON parts(code_norm);
CREATE INDEX IF NOT EXISTS idx_parts_mfr_norm ON parts(mfr_norm);
CREATE INDEX IF NOT EXISTS idx_parts_cat ON parts(category);
CREATE INDEX IF NOT EXISTS idx_parts_blob ON parts(search_blob);

CREATE TABLE IF NOT EXISTS equivalents (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  part_id INTEGER NOT NULL REFERENCES parts(id) ON DELETE CASCADE,
  code TEXT NOT NULL,
  code_norm TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_equiv_norm ON equivalents(code_norm);
CREATE INDEX IF NOT EXISTS idx_equiv_part ON equivalents(part_id);

CREATE TABLE IF NOT EXISTS suppliers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  contact TEXT DEFAULT '',
  phone TEXT DEFAULT '',
  email TEXT DEFAULT '',
  notes TEXT DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS customers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  phone TEXT DEFAULT '',
  doc TEXT DEFAULT '',
  address TEXT DEFAULT '',
  notes TEXT DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS vehicles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  plate TEXT DEFAULT '',
  model TEXT DEFAULT '',
  year TEXT DEFAULT '',
  notes TEXT DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_veh_cust ON vehicles(customer_id);

CREATE TABLE IF NOT EXISTS sales (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  customer_id INTEGER,
  customer_name TEXT DEFAULT '',
  vehicle_plate TEXT DEFAULT '',
  vehicle_model TEXT DEFAULT '',
  vehicle_year TEXT DEFAULT '',
  subtotal_cents INTEGER NOT NULL DEFAULT 0,
  discount_cents INTEGER NOT NULL DEFAULT 0,
  total_cents INTEGER NOT NULL DEFAULT 0,
  cost_cents INTEGER NOT NULL DEFAULT 0,
  payment TEXT NOT NULL DEFAULT 'dinheiro',
  received_cents INTEGER NOT NULL DEFAULT 0,
  change_cents INTEGER NOT NULL DEFAULT 0,
  due_date TEXT DEFAULT '',
  status TEXT NOT NULL DEFAULT 'concluida',
  note TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sales_created ON sales(created_at);

CREATE TABLE IF NOT EXISTS sale_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sale_id INTEGER NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
  part_id INTEGER,
  code TEXT DEFAULT '',
  description TEXT DEFAULT '',
  qty REAL NOT NULL DEFAULT 1,
  unit_price_cents INTEGER NOT NULL DEFAULT 0,
  unit_cost_cents INTEGER NOT NULL DEFAULT 0,
  discount_cents INTEGER NOT NULL DEFAULT 0,
  total_cents INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sale_items_sale ON sale_items(sale_id);
CREATE INDEX IF NOT EXISTS idx_sale_items_part ON sale_items(part_id);

CREATE TABLE IF NOT EXISTS stock_moves (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  part_id INTEGER NOT NULL REFERENCES parts(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  qty_delta REAL NOT NULL,
  balance_after REAL NOT NULL DEFAULT 0,
  unit_cost_cents INTEGER NOT NULL DEFAULT 0,
  reason TEXT DEFAULT '',
  ref_type TEXT DEFAULT '',
  ref_id INTEGER,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_moves_part ON stock_moves(part_id);
CREATE INDEX IF NOT EXISTS idx_moves_created ON stock_moves(created_at);

CREATE TABLE IF NOT EXISTS orders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  supplier_id INTEGER,
  supplier_name TEXT DEFAULT '',
  status TEXT NOT NULL DEFAULT 'aberto',
  notes TEXT DEFAULT '',
  total_cents INTEGER NOT NULL DEFAULT 0,
  received_at TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS order_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  order_id INTEGER NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  falta_id INTEGER,
  part_id INTEGER,
  description TEXT DEFAULT '',
  qty REAL NOT NULL DEFAULT 1,
  cost_cents INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_id);

CREATE TABLE IF NOT EXISTS faltas (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  part_id INTEGER,
  description TEXT NOT NULL DEFAULT '',
  qty REAL NOT NULL DEFAULT 1,
  customer_id INTEGER,
  customer_name TEXT DEFAULT '',
  customer_phone TEXT DEFAULT '',
  vehicle TEXT DEFAULT '',
  supplier_id INTEGER,
  status TEXT NOT NULL DEFAULT 'aberta',
  cost_cents INTEGER NOT NULL DEFAULT 0,
  notes TEXT DEFAULT '',
  order_id INTEGER,
  received_at TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_faltas_status ON faltas(status);
CREATE INDEX IF NOT EXISTS idx_faltas_part ON faltas(part_id);
CREATE INDEX IF NOT EXISTS idx_faltas_created ON faltas(created_at);

CREATE TABLE IF NOT EXISTS receivables (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sale_id INTEGER,
  customer_id INTEGER,
  customer_name TEXT DEFAULT '',
  amount_cents INTEGER NOT NULL DEFAULT 0,
  due_date TEXT NOT NULL,
  paid_at TEXT DEFAULT '',
  status TEXT NOT NULL DEFAULT 'aberto',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_receb_status ON receivables(status);
CREATE INDEX IF NOT EXISTS idx_receb_due ON receivables(due_date);

CREATE TABLE IF NOT EXISTS catalog (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  code TEXT NOT NULL DEFAULT '',
  code_norm TEXT NOT NULL,
  description TEXT DEFAULT '',
  application TEXT DEFAULT '',
  measure TEXT DEFAULT '',
  criteria TEXT DEFAULT '',
  brand TEXT DEFAULT '',
  category TEXT DEFAULT '',
  supplier_id INTEGER,
  cost_cents INTEGER NOT NULL DEFAULT 0,
  notes TEXT DEFAULT '',
  search_blob TEXT DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_code ON catalog(code_norm);
CREATE INDEX IF NOT EXISTS idx_catalog_app ON catalog(application);
CREATE INDEX IF NOT EXISTS idx_catalog_blob ON catalog(search_blob);

CREATE TABLE IF NOT EXISTS catalog_equivalents (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  catalog_id INTEGER NOT NULL REFERENCES catalog(id) ON DELETE CASCADE,
  code TEXT NOT NULL,
  code_norm TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cateq_norm ON catalog_equivalents(code_norm);
CREATE INDEX IF NOT EXISTS idx_cateq_catalog ON catalog_equivalents(catalog_id);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT DEFAULT ''
);
`

// schemaCatalogo guarda o catalogo do fornecedor: codigo, descricao, para quais
// veiculos serve (aplicacao) e medida. Serve para responder na hora o que antes
// exigia folhear catalogo de papel - inclusive de pecas que nao estao no estoque.
var schemaCatalogo = `
CREATE TABLE IF NOT EXISTS catalog (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  code TEXT NOT NULL DEFAULT '',
  code_norm TEXT NOT NULL,
  description TEXT DEFAULT '',
  application TEXT DEFAULT '',
  measure TEXT DEFAULT '',
  brand TEXT DEFAULT '',
  category TEXT DEFAULT '',
  supplier_id INTEGER,
  cost_cents INTEGER NOT NULL DEFAULT 0,
  notes TEXT DEFAULT '',
  search_blob TEXT DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_code ON catalog(code_norm);
CREATE INDEX IF NOT EXISTS idx_catalog_app ON catalog(application);
CREATE INDEX IF NOT EXISTS idx_catalog_blob ON catalog(search_blob);

CREATE TABLE IF NOT EXISTS catalog_equivalents (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  catalog_id INTEGER NOT NULL REFERENCES catalog(id) ON DELETE CASCADE,
  code TEXT NOT NULL,
  code_norm TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cateq_norm ON catalog_equivalents(code_norm);
CREATE INDEX IF NOT EXISTS idx_cateq_catalog ON catalog_equivalents(catalog_id);
`

// criarIndiceBuscaCatalogo cria o indice FTS5 (trigrama) do catalogo e o
// preenche com o que ja existe. E idempotente: pode rodar em banco novo ou antigo.
func criarIndiceBuscaCatalogo(tx *sql.Tx) error {
	ddl := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS catalog_fts USING fts5(
			search_blob,
			content='catalog',
			content_rowid='id',
			tokenize='trigram'
		)`,
		`CREATE TRIGGER IF NOT EXISTS catalog_fts_ai AFTER INSERT ON catalog BEGIN
			INSERT INTO catalog_fts(rowid, search_blob) VALUES (new.id, new.search_blob);
		END`,
		`CREATE TRIGGER IF NOT EXISTS catalog_fts_ad AFTER DELETE ON catalog BEGIN
			INSERT INTO catalog_fts(catalog_fts, rowid, search_blob) VALUES('delete', old.id, old.search_blob);
		END`,
		`CREATE TRIGGER IF NOT EXISTS catalog_fts_au AFTER UPDATE ON catalog BEGIN
			INSERT INTO catalog_fts(catalog_fts, rowid, search_blob) VALUES('delete', old.id, old.search_blob);
			INSERT INTO catalog_fts(rowid, search_blob) VALUES (new.id, new.search_blob);
		END`,
	}
	for _, q := range ddl {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	// preenche o indice com o catalogo que ja estava gravado (se houver)
	var total, noIndice int64
	if err := tx.QueryRow(`SELECT COUNT(*) FROM catalog`).Scan(&total); err != nil {
		return err
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM catalog_fts`).Scan(&noIndice); err != nil {
		return err
	}
	if total > 0 && noIndice == 0 {
		if _, err := tx.Exec(`INSERT INTO catalog_fts(rowid, search_blob)
			SELECT id, search_blob FROM catalog`); err != nil {
			return err
		}
	}
	return nil
}

// versaoEsquema e a versao atual do banco (PRAGMA user_version).
// 1 = esquema do AutoPecas 1.0.0
// 2 = coluna order_items.received_at (controle de recebimento por item)
// 3 = tabela catalog (catalogo do fornecedor) e parts.application
// 4 = coluna catalog.criteria (criterios tecnicos: rosca, dentes, altura...)
// 5 = indice catalog_fts (busca instantanea com catalogo grande)
// 6 = payables (contas a pagar), notas_entrada (nota importada) e suppliers.doc
const versaoEsquema = 6

func (a *App) userVersion() (int, error) {
	var v int
	if err := a.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

// bancoTemTabelas informa se o arquivo ja tem tabelas (banco antigo x banco novo)
func (a *App) bancoTemTabelas() bool {
	var n int64
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN
		('parts','sales','sale_items','customers','suppliers','stock_moves','settings','orders','faltas')`).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// bancoTemDados informa se ja existe algo cadastrado (para decidir o backup antes de migrar)
func (a *App) bancoTemDados() bool {
	var n int64
	err := a.db.QueryRow(`SELECT (SELECT COUNT(*) FROM parts)+(SELECT COUNT(*) FROM sales)+(SELECT COUNT(*) FROM faltas)`).Scan(&n)
	return err == nil && n > 0
}

// validarEsquemaV1 confere que o banco antigo (sem user_version) e mesmo do AutoPecas.
func (a *App) validarEsquemaV1() error {
	necessarias := map[string][]string{
		"parts":    {"id", "code", "description", "stock", "cost_cents", "price_cents"},
		"sales":    {"id", "created_at", "total_cents", "status"},
		"settings": {"key", "value"},
	}
	for tabela, colunas := range necessarias {
		cols, err := a.colunasTabela(tabela)
		if err != nil {
			return fmt.Errorf("não foi possível ler o banco de dados antigo (tabela %s): %w", tabela, err)
		}
		if len(cols) == 0 {
			return fmt.Errorf("o arquivo do banco não parece ser do AutoPecas (falta a tabela %s)", tabela)
		}
		set := map[string]bool{}
		for _, c := range cols {
			set[c] = true
		}
		for _, c := range colunas {
			if !set[c] {
				return fmt.Errorf("o banco antigo não parece ser do AutoPecas (falta a coluna %s.%s)", tabela, c)
			}
		}
	}
	return nil
}

func (a *App) colunasTabela(tabela string) ([]string, error) {
	rows, err := a.db.Query(`PRAGMA table_info(` + tabela + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var cid, notnull, pk int
		var nome, tipo string
		var dflt sql.NullString
		if rows.Scan(&cid, &nome, &tipo, &notnull, &dflt, &pk) == nil {
			out = append(out, nome)
		}
	}
	return out, nil
}

func colunasTx(tx *sql.Tx, tabela string) ([]string, error) {
	rows, err := tx.Query(`PRAGMA table_info(` + tabela + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var cid, notnull, pk int
		var nome, tipo string
		var dflt sql.NullString
		if rows.Scan(&cid, &nome, &tipo, &notnull, &dflt, &pk) == nil {
			out = append(out, nome)
		}
	}
	return out, nil
}

// colunaSeFaltar faz o ALTER TABLE somente quando a coluna ainda nao existe,
// para poder rodar de novo sem erro em bancos antigos.
func colunaSeFaltar(tx *sql.Tx, tabela, coluna, definicao string) error {
	cols, err := colunasTx(tx, tabela)
	if err != nil {
		return err
	}
	for _, c := range cols {
		if c == coluna {
			return nil
		}
	}
	_, err = tx.Exec(`ALTER TABLE ` + tabela + ` ADD COLUMN ` + coluna + ` ` + definicao)
	return err
}

func definirVersao(tx *sql.Tx, v int) error {
	_, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v))
	return err
}

// aplicarMigracao roda uma migracao numerada dentro de uma transacao.
func (a *App) aplicarMigracao(n int) error {
	return a.db.Tx(func(tx *sql.Tx) error {
		switch n {
		case 1:
			// esquema base do AutoPecas 1.0.0
			if _, err := tx.Exec(schema); err != nil {
				return err
			}
			if err := colunaSeFaltar(tx, "parts", "code_norm", `TEXT DEFAULT ''`); err != nil {
				return err
			}
			if err := colunaSeFaltar(tx, "parts", "mfr_norm", `TEXT DEFAULT ''`); err != nil {
				return err
			}
		case 2:
			if err := colunaSeFaltar(tx, "order_items", "received_at", `TEXT DEFAULT ''`); err != nil {
				return err
			}
		case 3:
			// catalogo do fornecedor: a peca que voce nao tem, mas sabe que existe
			if _, err := tx.Exec(schemaCatalogo); err != nil {
				return err
			}
			if err := criarIndiceBuscaCatalogo(tx); err != nil {
				return err
			}
			// aplicacao (para quais veiculos serve) passa a ser guardada na peca.
			// Antes ela era lida do XML e descartada.
			if err := colunaSeFaltar(tx, "parts", "application", `TEXT DEFAULT ''`); err != nil {
				return err
			}
		case 4:
			// criterios tecnicos do catalogo (rosca, dentes, altura, encaixe...) -
			// e o que o balcao usa para cruzar a peca do fornecedor com outra marca
			if err := colunaSeFaltar(tx, "catalog", "criteria", `TEXT DEFAULT ''`); err != nil {
				return err
			}
		case 5:
			// indice de busca do catalogo: com 100 mil itens a busca por LIKE
			// varreria a tabela inteira a cada tecla digitada. O FTS5 com
			// tokenizador trigrama acha pedaco de codigo ("712" dentro de
			// "w71290") em milissegundos.
			if err := criarIndiceBuscaCatalogo(tx); err != nil {
				return err
			}
		case 6:
			// contas a pagar nascem da nota do fornecedor: fornecedor, numero,
			// chave da NF-e e uma linha por boleto (duplicata)
			if _, err := tx.Exec(schemaContas); err != nil {
				return err
			}
			// CNPJ do fornecedor: e por ele que a nota encontra o fornecedor
			// certo sem duplicar cadastro
			if err := colunaSeFaltar(tx, "suppliers", "doc", `TEXT DEFAULT ''`); err != nil {
				return err
			}
		default:
			return fmt.Errorf("migração %d desconhecida", n)
		}
		return definirVersao(tx, n)
	})
}

// migrate leva o banco para a versao atual do esquema (item 10).
func (a *App) migrate() error {
	versao, err := a.userVersion()
	if err != nil {
		return err
	}
	if versao > versaoEsquema {
		return fmt.Errorf("este banco foi criado por uma versão mais nova do AutoPecas (versão de dados %d); atualize o programa", versao)
	}
	if versao == 0 {
		// banco novo (sem tabelas) ou banco antigo do AutoPecas 1.0.0 (que nao gravava user_version)
		if a.bancoTemTabelas() {
			if err := a.validarEsquemaV1(); err != nil {
				return err
			}
		}
		if a.bancoTemDados() {
			if _, _, err := a.criaArquivoBackup("antes-de-migrar"); err != nil {
				return fmt.Errorf("não foi possível criar o backup antes de atualizar o banco: %w", err)
			}
		}
		versao = 1
		if err := a.aplicarMigracao(1); err != nil {
			return fmt.Errorf("falha ao preparar o banco de dados (versão 1): %w", err)
		}
	}
	for versao < versaoEsquema {
		proxima := versao + 1
		if err := a.aplicarMigracao(proxima); err != nil {
			return fmt.Errorf("falha ao atualizar o banco de dados para a versão %d: %w", proxima, err)
		}
		versao = proxima
	}
	return a.recalcularCodigos()
}

// recalcularCodigos preenche/recalcula os codigos normalizados das pecas.
func (a *App) recalcularCodigos() error {
	rows, err := a.db.Query(`SELECT id,code,IFNULL(mfr_code,'') FROM parts WHERE code_norm IS NULL OR code_norm='' OR mfr_norm IS NULL OR mfr_norm=''`)
	if err != nil {
		return nil
	}
	type up struct {
		id      int64
		code, m string
	}
	list := []up{}
	for rows.Next() {
		var x up
		if rows.Scan(&x.id, &x.code, &x.m) == nil {
			list = append(list, x)
		}
	}
	rows.Close()
	for _, x := range list {
		_, _ = a.db.Exec(`UPDATE parts SET code_norm=?, mfr_norm=? WHERE id=?`, normCode(x.code), normCode(x.m), x.id)
	}
	return nil
}

func (a *App) firstRun() {
	defaults := map[string]string{
		"loja_nome":                 "Minha Loja de Autopecas",
		"loja_cnpj":                 "",
		"loja_telefone":             "",
		"loja_endereco":             "",
		"backup_auto":               "1",
		"backup_manter":             "30",
		"backup_intervalo_minutos":  "60",
		"backup_manter_horarios":    "24",
		"backup_manter_fechamentos": "60",
		"backup_manter_manuais":     "60",
		"backup_manter_seguranca":   "10",
		// teto de espaco da pasta de backups (MB). 0 = sem teto.
		// Com o teto, os mais antigos saem mesmo que cada tipo ainda tenha vaga.
		"backup_limite_mb":          "300",
		"permitir_estoque_negativo": "1",
		"prazo_faturado_dias":       "30",
	}
	for k, v := range defaults {
		if a.db.GetSetting(k) == "" {
			a.db.SetSetting(k, v)
		}
	}
}

func (d *DB) GetSetting(k string) string {
	var v string
	err := d.QueryRow(`SELECT value FROM settings WHERE key=?`, k).Scan(&v)
	if err != nil {
		return ""
	}
	return v
}

func (d *DB) SetSetting(k, v string) {
	_, _ = d.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v)
}

// diasDeCalendario devolve a diferenca em dias entre duas datas
// ("2006-01-02" ou "2006-01-02 15:04:05"). A conta e feita por DATA no fuso
// local: comparar com time.Parse (UTC) errava por um dia no fim do dia.
func diasDeCalendario(de, ate time.Time) int {
	d := time.Date(de.Year(), de.Month(), de.Day(), 0, 0, 0, 0, time.UTC)
	a := time.Date(ate.Year(), ate.Month(), ate.Day(), 0, 0, 0, 0, time.UTC)
	return int(a.Sub(d).Hours() / 24)
}

// dataLocal interpreta uma data do banco no fuso local.
func dataLocal(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// idOuNulo converte uma coluna anulavel em *int64 para o JSON sair como
// numero ou null (e nunca como {"Int64":0,"Valid":false}).
func idOuNulo(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

// ---------- helpers ----------

func nowISO() string   { return time.Now().Format("2006-01-02 15:04:05") }
func todayISO() string { return time.Now().Format("2006-01-02") }

var reAccents = map[rune]rune{
	'á': 'a', 'à': 'a', 'ã': 'a', 'â': 'a', 'ä': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'õ': 'o', 'ô': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

// normText deixa o texto minusculo, sem acento e com espacos simples.
func normText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if n, ok := reAccents[r]; ok {
			b.WriteRune(n)
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// normCode deixa o codigo em maiusculas e sem separadores (para cruzamento).
func normCode(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// reMilhar casa com numeros escritos com separador de milhar e sem centavos
// ("1.234", "10.000", "1.000.000"). Sem esta regra, "1.234" era lido como 1,234.
var reMilhar = regexp.MustCompile(`^[0-9]{1,3}(?:\.[0-9]{3})+$`)

// normalizarNumero transforma o numero digitado/exportado no formato interno
// (ponto como separador decimal).
func normalizarNumero(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	if strings.Contains(s, ",") {
		// 1.234,56 -> 1234.56
		return strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	}
	if reMilhar.MatchString(s) {
		// 1.234 -> 1234 (separador de milhar, nao decimal)
		return strings.ReplaceAll(s, ".", "")
	}
	return s
}

// parseMoneyBR aceita "12,50", "12.50", "R$ 12,50", "1.234,56" e "1.234"
func parseMoneyBR(s string) int64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, "R$", ""))
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	s = normalizarNumero(s)
	var re = regexp.MustCompile(`^[0-9]*\.?[0-9]*$`)
	if !re.MatchString(s) || s == "." || s == "" {
		return 0
	}
	v := parseFloatSafe(s)
	c := int64(v*100 + 0.5)
	if neg {
		c = -c
	}
	return c
}

func parseFloatSafe(s string) float64 {
	var f float64
	fmt.Sscanf(strings.TrimSpace(s), "%g", &f)
	return f
}

func parseQty(s any) float64 {
	switch v := s.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		t := normalizarNumero(strings.TrimSpace(v))
		if t == "" {
			return 0
		}
		return parseFloatSafe(t)
	}
	return 0
}

func fmtMoney(c int64) string {
	sign := ""
	if c < 0 {
		sign = "-"
		c = -c
	}
	return fmt.Sprintf("%sR$ %d,%02d", sign, c/100, c%100)
}

// buildSearchBlobApp inclui a aplicacao (veiculos compativeis) no texto de busca.
func buildSearchBlobApp(code, mfr, desc, brand, cat, loc, aplicacao string, equivs []string) string {
	parts := []string{
		normText(code), normText(mfr), normText(desc), normText(brand),
		normText(cat), normText(loc), normCode(code), normCode(mfr),
		normText(aplicacao),
	}
	for _, e := range equivs {
		parts = append(parts, normText(e), normCode(e))
	}
	// Remove vazios e duplicados
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// backup helpers live in api_backup.go

// OpenDBLeitura abre um arquivo .db somente para leitura (usado no teste de
// restauracao: conferir o conteudo do backup sem tocar no banco em uso).
func OpenDBLeitura(caminho string) (*sql.DB, error) {
	if _, err := os.Stat(caminho); err != nil {
		return nil, err
	}
	d, err := sql.Open("sqlite", caminho+"?mode=ro")
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// schemaContas guarda o dinheiro que sai da loja e de onde ele veio.
//
// payables  = contas a pagar (uma linha por boleto/parcela da nota)
// notas_entrada = cabecalho de cada nota importada (fornecedor, numero, chave
//
//	da NF-e, valor total) - serve de historico e de trava para
//	nao lancar a mesma nota duas vezes.
var schemaContas = `
CREATE TABLE IF NOT EXISTS payables (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  supplier_id INTEGER,
  supplier_name TEXT DEFAULT '',
  supplier_doc TEXT DEFAULT '',
  doc_number TEXT DEFAULT '',
  doc_serie TEXT DEFAULT '',
  doc_key TEXT DEFAULT '',
  parcela TEXT DEFAULT '',
  note TEXT DEFAULT '',
  amount_cents INTEGER NOT NULL DEFAULT 0,
  due_date TEXT NOT NULL,
  paid_at TEXT DEFAULT '',
  paid_cents INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'aberto',
  origin TEXT DEFAULT '',
  import_id INTEGER,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pay_status ON payables(status);
CREATE INDEX IF NOT EXISTS idx_pay_due ON payables(due_date);
CREATE INDEX IF NOT EXISTS idx_pay_doc_key ON payables(doc_key);
CREATE INDEX IF NOT EXISTS idx_pay_supplier ON payables(supplier_id);

CREATE TABLE IF NOT EXISTS notas_entrada (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chave TEXT DEFAULT '',
  numero TEXT DEFAULT '',
  serie TEXT DEFAULT '',
  supplier_id INTEGER,
  supplier_name TEXT DEFAULT '',
  supplier_doc TEXT DEFAULT '',
  emitida_em TEXT DEFAULT '',
  produtos_cents INTEGER NOT NULL DEFAULT 0,
  extras_cents INTEGER NOT NULL DEFAULT 0,
  total_cents INTEGER NOT NULL DEFAULT 0,
  rateado_cents INTEGER NOT NULL DEFAULT 0,
  itens INTEGER NOT NULL DEFAULT 0,
  arquivo TEXT DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_notas_chave ON notas_entrada(chave) WHERE chave <> '';
`

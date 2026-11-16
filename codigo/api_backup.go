package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) backupDir() string { return filepath.Join(a.dir, "backups") }

// copiarParaPastaExtra copia o backup recem gerado para a pasta configurada em
// "backup_pasta_extra" (pendrive, pasta do Google Drive etc). Uma falha aqui
// NUNCA invalida o backup principal: o problema fica registrado para a tela
// "Backup e Ajustes" mostrar o aviso.
func (a *App) copiarParaPastaExtra(arquivo string) error {
	destino := strings.TrimSpace(a.db.GetSetting("backup_pasta_extra"))
	if destino == "" {
		a.db.SetSetting("backup_pasta_extra_erro", "")
		return nil
	}
	st, err := os.Stat(destino)
	if err != nil || !st.IsDir() {
		msg := "a pasta extra de backup não existe ou não está acessível: " + destino
		a.db.SetSetting("backup_pasta_extra_erro", msg)
		log.Printf("aviso: %s", msg)
		return fmt.Errorf("%s", msg)
	}
	alvo := filepath.Join(destino, filepath.Base(arquivo))
	origem, err := os.Open(arquivo)
	if err != nil {
		msg := "não foi possível ler o backup para copiar: " + err.Error()
		a.db.SetSetting("backup_pasta_extra_erro", msg)
		return fmt.Errorf("%s", msg)
	}
	defer origem.Close()
	dest, err := os.Create(alvo)
	if err != nil {
		msg := "não foi possível gravar na pasta extra: " + err.Error()
		a.db.SetSetting("backup_pasta_extra_erro", msg)
		log.Printf("aviso: %s", msg)
		return fmt.Errorf("%s", msg)
	}
	if _, err := io.Copy(dest, origem); err != nil {
		dest.Close()
		_ = os.Remove(alvo)
		msg := "não foi possível copiar o backup para a pasta extra: " + err.Error()
		a.db.SetSetting("backup_pasta_extra_erro", msg)
		log.Printf("aviso: %s", msg)
		return fmt.Errorf("%s", msg)
	}
	if err := dest.Close(); err != nil {
		msg := "não foi possível concluir a cópia do backup: " + err.Error()
		a.db.SetSetting("backup_pasta_extra_erro", msg)
		return fmt.Errorf("%s", msg)
	}
	a.db.SetSetting("backup_pasta_extra_erro", "")
	return nil
}

// criaArquivoBackup gera um arquivo unico .db (copia consistente do banco)
func (a *App) criaArquivoBackup(prefixo string) (string, int64, error) {
	stamp := time.Now().Format("2006-01-02_1504")
	nome := fmt.Sprintf("%s-%s.db", prefixo, stamp)
	destino := filepath.Join(a.backupDir(), nome)
	_ = os.Remove(destino)
	q := fmt.Sprintf("VACUUM INTO '%s'", strings.ReplaceAll(destino, "'", "''"))
	if _, err := a.db.Exec(q); err != nil {
		return "", 0, err
	}
	st, err := os.Stat(destino)
	if err != nil {
		return "", 0, err
	}
	a.db.SetSetting("ultimo_backup", nowISO())
	a.db.SetSetting("ultimo_backup_arquivo", destino)
	a.limparBackupsAntigos()
	// item 9: copia opcional para uma segunda pasta; erro nao derruba o backup
	_ = a.copiarParaPastaExtra(destino)
	return destino, st.Size(), nil
}

func (a *App) ensureDailyBackup() (string, error) {
	ultimo := a.db.GetSetting("ultimo_backup")
	if strings.HasPrefix(ultimo, todayISO()) {
		if p := a.db.GetSetting("ultimo_backup_arquivo"); p != "" {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	p, _, err := a.criaArquivoBackup("backup-automatico")
	return p, err
}

func (a *App) handleDoBackup(w http.ResponseWriter, r *http.Request) {
	p, size, err := a.criaArquivoBackup("backup")
	if err != nil {
		fail(w, 500, "erro ao gerar backup: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "arquivo": filepath.Base(p), "caminho": p,
		"tamanho_bytes": size, "data": nowISO()})
}

func (a *App) handleListBackups(w http.ResponseWriter, r *http.Request) {
	ents, err := os.ReadDir(a.backupDir())
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	out := []map[string]any{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, map[string]any{
			"arquivo": e.Name(), "tamanho_bytes": info.Size(),
			"data":       info.ModTime().Format("2006-01-02 15:04"),
			"automatico": strings.HasPrefix(e.Name(), "backup-automatico"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["data"].(string) > out[j]["data"].(string) })
	writeJSON(w, 200, map[string]any{"itens": out, "pasta": a.backupDir(),
		"ultimo_backup":     a.db.GetSetting("ultimo_backup"),
		"backup_auto":       a.db.GetSetting("backup_auto"),
		"pasta_extra":       a.db.GetSetting("backup_pasta_extra"),
		"aviso_pasta_extra": a.db.GetSetting("backup_pasta_extra_erro")})
}

func (a *App) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	nome := filepath.Base(r.URL.Query().Get("arquivo"))
	p := filepath.Join(a.backupDir(), nome)
	if _, err := os.Stat(p); err != nil {
		fail(w, 404, "backup nao encontrado")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+nome)
	f, err := os.Open(p)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer f.Close()
	_, _ = io.Copy(w, f)
}

// handleRestore restaura o banco a partir de um backup enviado ou de um arquivo da pasta de backups.
func (a *App) handleRestore(w http.ResponseWriter, r *http.Request) {
	var origem string
	var tempPath string
	if err := r.ParseMultipartForm(64 << 20); err == nil {
		if f, hdr, err := r.FormFile("arquivo"); err == nil {
			defer f.Close()
			name := hdr.Filename
			if !strings.HasSuffix(strings.ToLower(name), ".db") && !strings.HasSuffix(strings.ToLower(name), ".sqlite") && !strings.HasSuffix(strings.ToLower(name), ".sqlite3") {
				fail(w, 400, "envie o arquivo de backup (.db) gerado pelo sistema")
				return
			}
			tmp, err := os.CreateTemp("", "restore-*.db")
			if err != nil {
				fail(w, 500, err.Error())
				return
			}
			tempPath = tmp.Name()
			if _, err := io.Copy(tmp, io.LimitReader(f, 512<<20)); err != nil {
				tmp.Close()
				fail(w, 500, err.Error())
				return
			}
			tmp.Close()
			origem = tempPath
			defer os.Remove(tempPath)
		}
	}
	if origem == "" {
		nome := filepath.Base(r.URL.Query().Get("arquivo"))
		if nome == "" || nome == "." {
			fail(w, 400, "envie um arquivo de backup ou informe o nome de um backup existente")
			return
		}
		origem = filepath.Join(a.backupDir(), nome)
	}

	// item 3: valida de verdade antes de tocar no banco em uso
	pecas, err := validarArquivoBackup(origem)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}

	// backup de seguranca antes de restaurar (1.0.10: se ele falhar, NAO
	// restauramos - sem rede de seguranca o risco de perder tudo e' real)
	seg, _, errSeg := a.criaArquivoBackup("antes-de-restaurar")
	if errSeg != nil {
		fail(w, 500, "não restaurei: não consegui guardar antes uma cópia do sistema atual ("+errSeg.Error()+
			"). Verifique o espaço em disco e tente de novo.")
		return
	}

	if err := a.restaurarDe(origem); err != nil {
		fail(w, 500, "erro ao restaurar: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "seguranca": filepath.Base(seg), "pecas": pecas})
}

// validarArquivoBackup confere a integridade do arquivo (PRAGMA quick_check) e
// se as tabelas principais do AutoPecas existem.
func validarArquivoBackup(caminho string) (int64, error) {
	if st, err := os.Stat(caminho); err != nil || st.IsDir() {
		return 0, fmt.Errorf("arquivo de backup não encontrado")
	}
	chk, err := sql.Open("sqlite", caminho+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("arquivo de backup inválido: %v", err)
	}
	defer chk.Close()

	rows, err := chk.Query(`PRAGMA quick_check`)
	if err != nil {
		return 0, fmt.Errorf("este arquivo não parece ser um backup do AutoPecas")
	}
	ok := false
	if rows.Next() {
		var res string
		if rows.Scan(&res) == nil && strings.EqualFold(strings.TrimSpace(res), "ok") {
			ok = true
		}
	}
	rows.Close()
	if !ok {
		return 0, fmt.Errorf("arquivo de backup corrompido (a verificação de integridade falhou)")
	}

	obrigatorias := []string{"parts", "sales", "sale_items", "customers", "suppliers", "stock_moves", "settings"}
	for _, t := range obrigatorias {
		var n int64
		if err := chk.QueryRow(`SELECT COUNT(*) FROM ` + t).Scan(&n); err != nil {
			return 0, fmt.Errorf("este arquivo não parece ser um backup do AutoPecas (falta a tabela %s)", t)
		}
	}
	var pecas int64
	_ = chk.QueryRow(`SELECT COUNT(*) FROM parts`).Scan(&pecas)
	return pecas, nil
}

// restaurarDe copia os dados do arquivo de backup para o banco em uso usando
// uma UNICA conexao dedicada (item 3). O ATTACH e o DETACH precisam acontecer
// na mesma conexao; com o pool de 1 conexao do programa isso so e seguro fora
// do pool comum.
func (a *App) restaurarDe(origem string) error {
	// 1.0.10: o backup trazia os "ajustes" da epoca dele, inclusive a senha do
	// celular e o acesso_rede. Restaurar um backup antigo APAGAVA a senha atual
	// - e se aquele backup dizia acesso_rede=1 sem senha, o programa podia se
	// recusar a abrir na proxima vez. Estes ajustes ficam como estao hoje.
	//
	// Eles sao lidos AQUI (antes) e regravados no fim, FORA da conexao
	// dedicada: o banco tem uma conexao so, entao chamar o pool enquanto ela
	// esta presa travaria o sistema.
	preservar := map[string]string{}
	for _, k := range []string{"senha_rede_salt", "senha_rede_hash", "acesso_rede"} {
		preservar[k] = a.db.GetSetting(k)
	}
	if err := a.restaurarComConexao(origem); err != nil {
		return err
	}
	for k, v := range preservar {
		if v == "" {
			if _, err := a.db.Exec(`DELETE FROM settings WHERE key=?`, k); err != nil {
				log.Printf("aviso: nao consegui limpar o ajuste %s depois de restaurar: %v", k, err)
			}
			continue
		}
		a.db.SetSetting(k, v)
	}
	return nil
}

// restaurarComConexao copia as tabelas do backup para o banco em uso usando uma
// conexao dedicada (ATTACH/DETACH precisam sair da mesma conexao).
func (a *App) restaurarComConexao(origem string) error {
	ctx := context.Background()
	conn, err := a.db.sql.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS bkp`, origem); err != nil {
		return fmt.Errorf("abrindo o arquivo de backup: %w", err)
	}
	// DETACH sempre, inclusive em caso de erro, e sempre nesta conexao
	defer func() {
		if _, err := conn.ExecContext(ctx, `DETACH DATABASE bkp`); err != nil {
			log.Printf("aviso: falha ao desanexar o backup: %v", err)
		}
	}()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	commit := false
	defer func() {
		if !commit {
			_ = tx.Rollback()
		}
	}()

	ordem := []string{"equivalents", "stock_moves", "sale_items", "order_items", "faltas",
		"receivables", "sales", "vehicles", "customers", "orders", "parts", "suppliers", "settings",
		"payables", "notas_entrada"}
	for _, t := range ordem {
		if _, err := tx.Exec(`DELETE FROM main.` + t); err != nil {
			return err
		}
	}
	pais := []string{"suppliers", "customers", "parts", "vehicles", "sales", "sale_items", "stock_moves",
		"orders", "order_items", "faltas", "receivables", "payables", "notas_entrada",
		"settings", "equivalents"}
	for _, t := range pais {
		cols, err := tableColumns(tx, "bkp", t)
		if err != nil {
			return err
		}
		if len(cols) == 0 {
			continue
		}
		liveCols, _ := tableColumns(tx, "main", t)
		use := intersect(cols, liveCols)
		if len(use) == 0 {
			continue
		}
		q := fmt.Sprintf("INSERT INTO main.%s (%s) SELECT %s FROM bkp.%s", t,
			strings.Join(use, ", "), strings.Join(use, ", "), t)
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("tabela %s: %w", t, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	commit = true
	return nil
}

func tableColumns(tx *sql.Tx, schema, table string) ([]string, error) {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA %s.table_info(%s)", schema, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk) == nil {
			cols = append(cols, name)
		}
	}
	return cols, nil
}

func intersect(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	out := []string{}
	for _, x := range a {
		if set[x] {
			out = append(out, x)
		}
	}
	return out
}

// ---------- configuracao ----------

func (a *App) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	keys := []string{"loja_nome", "loja_cnpj", "loja_telefone", "loja_endereco", "backup_auto",
		"backup_manter", "permitir_estoque_negativo", "prazo_faturado_dias", "ultimo_backup",
		"backup_pasta_extra", "backup_pasta_extra_erro", "acesso_rede",
		"backup_intervalo_minutos", "backup_manter_horarios", "backup_manter_fechamentos",
		"backup_manter_manuais", "backup_manter_seguranca", "backup_limite_mb", "ultimo_fechamento",
		"ultimo_backup_horario", "backup_ultimo_erro"}
	out := map[string]string{}
	for _, k := range keys {
		out[k] = a.db.GetSetting(k)
	}
	out["pasta_dados"] = a.dir
	out["pasta_backups"] = a.backupDir()
	out["versao"] = appVersion
	// nunca devolve a senha: so se ela existe
	out["senha_definida"] = fmt.Sprint(a.exigeSenha())
	out["modo_acesso"] = a.modoAcesso()
	writeJSON(w, 200, out)
}

func (a *App) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	permitido := map[string]bool{"loja_nome": true, "loja_cnpj": true, "loja_telefone": true,
		"loja_endereco": true, "backup_auto": true, "backup_manter": true,
		"permitir_estoque_negativo": true, "prazo_faturado_dias": true,
		"backup_pasta_extra": true, "acesso_rede": true,
		"backup_intervalo_minutos": true, "backup_manter_horarios": true,
		"backup_manter_fechamentos": true, "backup_manter_manuais": true,
		"backup_manter_seguranca": true, "backup_limite_mb": true}
	// 1.0.10: antes qualquer texto entrava no banco ("abc" em campo de numero,
	// quantidade negativa, teto de 0 MB...). Agora numero invalido e recusado
	// com mensagem, em vez de virar um ajuste quebrado em silencio.
	numericos := map[string][2]int{
		"backup_manter":             {1, 999},
		"backup_manter_horarios":    {1, 999},
		"backup_manter_fechamentos": {1, 999},
		"backup_manter_manuais":     {1, 999},
		"backup_manter_seguranca":   {1, 999},
		"backup_intervalo_minutos":  {15, 1440},
		"backup_limite_mb":          {0, 100000},
		"prazo_faturado_dias":       {1, 3650},
	}
	gravados := []string{}
	for k, v := range in {
		if !permitido[k] {
			continue
		}
		texto := strings.TrimSpace(fmt.Sprint(v))
		if faixa, ehNumero := numericos[k]; ehNumero {
			n, err := strconv.Atoi(texto)
			if err != nil {
				fail(w, 400, "o ajuste "+k+" precisa ser um número (recebi "+texto+")")
				return
			}
			if n < faixa[0] || n > faixa[1] {
				fail(w, 400, fmt.Sprintf("o ajuste %s precisa ficar entre %d e %d (recebi %d)", k, faixa[0], faixa[1], n))
				return
			}
			texto = strconv.Itoa(n)
		}
		if k == "backup_pasta_extra" && texto != "" && !filepath.IsAbs(texto) {
			fail(w, 400, "a pasta de backup extra precisa ser um caminho completo (ex.: C:\\Users\\Voce\\Dropbox\\Backup AutoPecas)")
			return
		}
		a.db.SetSetting(k, texto)
		gravados = append(gravados, k)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "gravados": len(gravados)})
}

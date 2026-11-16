package main

// Testes dos itens 3 (restaurar backup) e 9 (backups automaticos).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// backupAtual gera um backup pela API e devolve os bytes do arquivo.
func backupAtual(t *testing.T, env *ambiente) []byte {
	t.Helper()
	r := env.post("/api/backup", map[string]any{})
	if r.status != 200 {
		t.Fatalf("gerar backup: %d %s", r.status, r.corpo)
	}
	nome, _ := r.json(t)["arquivo"].(string)
	if nome == "" {
		t.Fatalf("backup sem nome de arquivo: %s", r.corpo)
	}
	return env.lerArquivo(filepath.Join(env.dir, "backups", nome))
}

// ---------- item 3: restaurar duas vezes na mesma sessao ----------

func TestItem3RestaurarDuasVezesNaMesmaSessao(t *testing.T) {
	env := novaAppTeste(t)
	peca := env.criarPeca("RST-001", "Peca restaurada", 1500, 3000, 5)
	env.post("/api/vendas", corpoVenda(peca, 1, 3000, 0, 0, "dinheiro", map[string]any{"recebido_centavos": 3000}))

	arquivo := backupAtual(t, env)
	pecasAntes := env.qint(`SELECT COUNT(*) FROM parts`)
	vendasAntes := env.qint(`SELECT COUNT(*) FROM sales`)
	estoqueAntes := env.estoque(peca)

	// altera os dados para provar que a restauracao realmente volta tudo
	env.criarPeca("RST-002", "Peca que sera descartada", 100, 200, 1)
	if env.qint(`SELECT COUNT(*) FROM parts`) != pecasAntes+1 {
		t.Fatalf("preparacao do teste falhou")
	}

	for i := 1; i <= 2; i++ {
		r := env.enviarArquivo("/api/restaurar", "arquivo", "meu-backup.db", arquivo)
		if r.status != 200 {
			t.Fatalf("restauracao %d falhou: status %d corpo %s", i, r.status, r.corpo)
		}
		if got := env.qint(`SELECT COUNT(*) FROM parts`); got != pecasAntes {
			t.Errorf("restauracao %d: pecas = %d, esperado %d", i, got, pecasAntes)
		}
		if got := env.qint(`SELECT COUNT(*) FROM sales`); got != vendasAntes {
			t.Errorf("restauracao %d: vendas = %d, esperado %d", i, got, vendasAntes)
		}
		if got := env.estoque(peca); got != estoqueAntes {
			t.Errorf("restauracao %d: estoque = %v, esperado %v", i, got, estoqueAntes)
		}
	}
	// o banco continua utilizavel depois das restauracoes
	if r := env.post("/api/pecas", map[string]any{"codigo": "NOVO-1", "descricao": "Depois da restauracao"}); r.status != 200 {
		t.Errorf("banco inutilizavel apos restaurar: %d %s", r.status, r.corpo)
	}
}

func TestItem3RestaurarArquivoInvalidoNaoAlteraBanco(t *testing.T) {
	env := novaAppTeste(t)
	peca := env.criarPeca("INV-001", "Peca original", 1000, 2000, 7)
	pecasAntes := env.qint(`SELECT COUNT(*) FROM parts`)

	casos := []struct {
		nome    string
		arquivo string
		bytes   []byte
	}{
		{"arquivo de texto", "lixo.db", []byte("isto nao e um banco de dados, apenas texto")},
		{"banco vazio sem tabelas", "vazio.db", nil}, // gerado abaixo
	}
	// banco sqlite valido, mas de outro programa (sem as tabelas do AutoPecas)
	outro := filepath.Join(t.TempDir(), "outro.db")
	odb, err := OpenDB(outro)
	if err != nil {
		t.Fatalf("criando banco de outro programa: %v", err)
	}
	if _, err := odb.Exec(`CREATE TABLE qualquer(id INTEGER PRIMARY KEY, x TEXT); INSERT INTO qualquer(x) VALUES('a')`); err != nil {
		t.Fatalf("populando banco de outro programa: %v", err)
	}
	_ = odb.Close()
	dadosOutro := env.lerArquivo(outro)
	casos[1].bytes = dadosOutro
	casos[1].arquivo = "outro-programa.db"

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r := env.enviarArquivo("/api/restaurar", "arquivo", c.arquivo, c.bytes)
			if r.status != 400 {
				t.Fatalf("esperado 400 para arquivo invalido, veio %d (%s)", r.status, r.corpo)
			}
			if got := env.qint(`SELECT COUNT(*) FROM parts`); got != pecasAntes {
				t.Errorf("banco foi alterado por restauracao invalida (pecas=%d, esperado %d)", got, pecasAntes)
			}
			if got := env.estoque(peca); got != 7 {
				t.Errorf("estoque alterado por restauracao invalida: %v", got)
			}
		})
	}
}

// ---------- item 9: backups automaticos ----------

func TestItem9BackupCopiaParaPastaExtra(t *testing.T) {
	env := novaAppTeste(t)
	env.criarPeca("BKP-001", "Peca backup", 1000, 2000, 3)
	extra := filepath.Join(env.dir, "pasta-extra")
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatalf("criando pasta extra: %v", err)
	}

	if r := env.post("/api/config", map[string]any{"backup_pasta_extra": extra}); r.status != 200 {
		t.Fatalf("salvar pasta extra: %d %s", r.status, r.corpo)
	}
	if r := env.post("/api/backup", map[string]any{}); r.status != 200 {
		t.Fatalf("gerar backup: %d %s", r.status, r.corpo)
	}

	ents, err := os.ReadDir(extra)
	if err != nil {
		t.Fatalf("lendo pasta extra: %v", err)
	}
	achou := false
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".db") {
			achou = true
			info, _ := e.Info()
			if info.Size() == 0 {
				t.Errorf("copia na pasta extra esta vazia")
			}
		}
	}
	if !achou {
		t.Errorf("nenhum backup copiado para a pasta extra %s", extra)
	}

	// configuracao aparece na tela de configuracao
	cfg := env.get("/api/config").json(t)
	if cfg["backup_pasta_extra"] != extra {
		t.Errorf("config nao devolve backup_pasta_extra: %v", cfg["backup_pasta_extra"])
	}
}

func TestItem9PastaExtraInexistenteNaoAtrapalhaBackup(t *testing.T) {
	env := novaAppTeste(t)
	env.criarPeca("BKP-002", "Peca backup 2", 1000, 2000, 3)
	fantasma := filepath.Join(env.dir, "nao-existe", "drive")
	if r := env.post("/api/config", map[string]any{"backup_pasta_extra": fantasma}); r.status != 200 {
		t.Fatalf("salvar pasta extra inexistente: %d %s", r.status, r.corpo)
	}

	r := env.post("/api/backup", map[string]any{})
	if r.status != 200 {
		t.Fatalf("backup principal deve funcionar mesmo com pasta extra inexistente: %d %s", r.status, r.corpo)
	}
	nome, _ := r.json(t)["arquivo"].(string)
	if _, err := os.Stat(filepath.Join(env.dir, "backups", nome)); err != nil {
		t.Fatalf("arquivo de backup principal nao existe: %v", err)
	}

	// a tela de backup precisa avisar o problema
	lista := env.get("/api/backup")
	if lista.status != 200 {
		t.Fatalf("listar backups: %d %s", lista.status, lista.corpo)
	}
	m := lista.json(t)
	aviso, _ := m["aviso_pasta_extra"].(string)
	if aviso == "" {
		t.Errorf("tela de backup deveria avisar que a pasta extra nao existe (aviso_pasta_extra vazio)")
	}
}

func TestItem9BackupDeHoraEmHoraConfigurado(t *testing.T) {
	// o backup diario e verificado uma vez por dia; a rotina de hora em hora
	// chama ensureDailyBackup e nao deve gerar arquivos duplicados no mesmo dia.
	env := novaAppTeste(t)
	env.criarPeca("BKP-003", "Peca backup 3", 1000, 2000, 3)

	p1, err := env.app.ensureDailyBackup()
	if err != nil {
		t.Fatalf("ensureDailyBackup: %v", err)
	}
	p2, err := env.app.ensureDailyBackup()
	if err != nil {
		t.Fatalf("segunda chamada de ensureDailyBackup: %v", err)
	}
	if p1 != p2 {
		t.Errorf("backup duplicado no mesmo dia: %s e %s", p1, p2)
	}
	ents, err := os.ReadDir(filepath.Join(env.dir, "backups"))
	if err != nil {
		t.Fatalf("lendo backups: %v", err)
	}
	n := 0
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "backup-automatico") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("esperado 1 backup automatico no dia, encontrei %d", n)
	}
}

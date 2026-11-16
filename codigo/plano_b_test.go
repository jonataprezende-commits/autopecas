package main

// Testes do Plano B (1.0.7): backup de hora em hora, backup ao fechar o caixa,
// painel de saude, teste de restauracao, kit de emergencia e pasta na nuvem.

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func arquivosComPrefixo(dir, prefixo string) []string {
	ents, _ := os.ReadDir(dir)
	out := []string{}
	for _, e := range ents {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefixo) && strings.HasSuffix(e.Name(), ".db") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestBackupDeHoraEmHoraCriaQuandoVence(t *testing.T) {
	env := novaAppTeste(t)
	// sem nenhum backup ainda: deve criar na hora
	p, criou := env.app.backupHorarioSePreciso()
	if !criou {
		t.Fatalf("o primeiro backup de hora em hora deveria ser criado na abertura")
	}
	if !strings.Contains(filepath.Base(p), "backup-horario") {
		t.Errorf("arquivo deveria ter o prefixo backup-horario, veio %s", filepath.Base(p))
	}
	if !arquivoExiste(p) {
		t.Fatalf("o arquivo do backup não existe: %s", p)
	}
	if env.qstr(`SELECT value FROM settings WHERE key='ultimo_backup_horario'`) == "" {
		t.Errorf("o horário do último backup deveria ficar gravado")
	}

	// dentro do intervalo: nao cria outro
	if _, deNovo := env.app.backupHorarioSePreciso(); deNovo {
		t.Errorf("não deveria criar outro backup antes do intervalo vencer")
	}

	// intervalo vencido (2 horas atrás): cria
	env.app.db.SetSetting("ultimo_backup_horario", time.Now().Add(-2*time.Hour).Format(time.RFC3339))
	if _, venceu := env.app.backupHorarioSePreciso(); !venceu {
		t.Errorf("com o intervalo vencido deveria criar um novo backup")
	}
	if n := len(arquivosComPrefixo(env.app.backupDir(), "backup-horario")); n < 1 {
		t.Errorf("esperava um backup de hora em hora na pasta, achei %d", n)
	}
	if !strings.HasPrefix(env.qstr(`SELECT value FROM settings WHERE key='ultimo_backup_horario'`), time.Now().Format("2006-01-02")) {
		t.Errorf("o horário do backup deveria ser de hoje")
	}
}

func TestBackupRespeitaDesligadoNaConfiguracao(t *testing.T) {
	env := novaAppTeste(t)
	env.app.db.SetSetting("backup_auto", "0")
	if _, criou := env.app.backupHorarioSePreciso(); criou {
		t.Errorf("com o automático desligado não deveria criar backup")
	}
	if n := len(arquivosComPrefixo(env.app.backupDir(), "backup-horario")); n != 0 {
		t.Errorf("não deveria existir arquivo de backup automático (%d)", n)
	}
}

func TestRetencaoGuardaCotaPorTipo(t *testing.T) {
	env := novaAppTeste(t)
	env.app.db.SetSetting("backup_manter_horarios", "3")
	env.app.db.SetSetting("backup_manter", "2")

	// cria 5 backups horários e 4 diários direto na pasta
	dir := env.app.backupDir()
	fazer := func(prefixo string, i int) {
		nome := prefixo + "-2026-11-1" + string(rune('0'+i)) + "_0" + string(rune('0'+i)) + "0.db"
		if err := os.WriteFile(filepath.Join(dir, nome), []byte("x"), 0o644); err != nil {
			t.Fatalf("criando arquivo de teste: %v", err)
		}
	}
	for i := 1; i <= 5; i++ {
		fazer("backup-horario", i)
	}
	for i := 1; i <= 4; i++ {
		fazer("backup-automatico", i)
	}
	env.app.limparBackupsAntigos()

	if n := len(arquivosComPrefixo(dir, "backup-horario")); n != 3 {
		t.Errorf("deveria guardar 3 backups de hora em hora, guardou %d", n)
	}
	if n := len(arquivosComPrefixo(dir, "backup-automatico")); n != 2 {
		t.Errorf("deveria guardar 2 backups diários, guardou %d", n)
	}
	// a limpeza tem que manter os mais novos
	for _, nome := range arquivosComPrefixo(dir, "backup-horario") {
		if strings.Contains(nome, "2026-11-11_010") || strings.Contains(nome, "2026-11-12_020") {
			t.Errorf("apagou um backup novo e manteve antigo: %s", nome)
		}
	}
}

func TestFecharCaixaGeraBackup(t *testing.T) {
	env := novaAppTeste(t)
	env.criarPeca("CX-1", "Peca do fechamento", 1000, 2000, 3)

	r := env.post("/api/fechamento/fechar", map[string]any{"data": todayISO()})
	if r.status != 200 {
		t.Fatalf("fechar o caixa deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	m := r.json(t)
	nome, _ := m["arquivo"].(string)
	if !strings.HasPrefix(nome, "backup-fechamento") {
		t.Errorf("o backup deveria ter o prefixo backup-fechamento, veio %q", nome)
	}
	if !arquivoExiste(filepath.Join(env.app.backupDir(), nome)) {
		t.Errorf("o arquivo do fechamento não foi criado")
	}
	if !strings.HasPrefix(env.qstr(`SELECT value FROM settings WHERE key='ultimo_fechamento'`), todayISO()) {
		t.Errorf("o dia do último fechamento deveria ficar gravado")
	}
	// o backup do fechamento tem os dados da loja
	if _, err := validarArquivoBackup(filepath.Join(env.app.backupDir(), nome)); err != nil {
		t.Errorf("o backup do fechamento deveria ser válido: %v", err)
	}
	// data invalida e recusada
	if r := env.post("/api/fechamento/fechar", map[string]any{"data": "31/02/2026"}); r.status != 400 {
		t.Errorf("data inválida deveria dar 400, veio %d", r.status)
	}
}

func TestSaudeDoBackupTemSemaforo(t *testing.T) {
	env := novaAppTeste(t)
	// sem backup: vermelho
	m := env.get("/api/saude-backup").json(t)
	if m["nivel"] != "vermelho" {
		t.Errorf("sem backup o nível deveria ser vermelho, veio %v", m["nivel"])
	}
	if m["minutos_desde_ultimo"].(float64) != -1 {
		t.Errorf("sem backup, minutos deveria ser -1, veio %v", m["minutos_desde_ultimo"])
	}

	// com backup agora: verde
	if r := env.post("/api/backup", nil); r.status != 200 {
		t.Fatalf("fazer backup deveria dar 200, veio %d", r.status)
	}
	m = env.get("/api/saude-backup").json(t)
	if m["nivel"] != "verde" {
		t.Errorf("com backup recente o nível deveria ser verde, veio %v", m["nivel"])
	}
	if m["minutos_desde_ultimo"].(float64) > 5 {
		t.Errorf("o backup acabou de ser feito, minutos deveria ser ~0: %v", m["minutos_desde_ultimo"])
	}
	if m["total_arquivos"].(float64) < 1 {
		t.Errorf("deveria contar pelo menos 1 arquivo")
	}
	nuvem, ok := m["nuvem"].(map[string]any)
	if !ok || nuvem["ok"] != false {
		t.Errorf("sem pasta na nuvem, o painel deveria dizer que não está configurada: %v", m["nuvem"])
	}
	if m["disco"] == nil {
		t.Errorf("o painel deveria informar o espaço em disco")
	}

	// backup de 3 horas atrás: amarelo
	env.app.db.SetSetting("ultimo_backup", time.Now().Add(-3*time.Hour).Format("2006-01-02 15:04:05"))
	if m := env.get("/api/saude-backup").json(t); m["nivel"] != "amarelo" {
		t.Errorf("backup de 3h atrás deveria ser amarelo, veio %v", m["nivel"])
	}
	// backup de 2 dias atrás: vermelho
	env.app.db.SetSetting("ultimo_backup", time.Now().Add(-48*time.Hour).Format("2006-01-02 15:04:05"))
	if m := env.get("/api/saude-backup").json(t); m["nivel"] != "vermelho" {
		t.Errorf("backup de 2 dias atrás deveria ser vermelho, veio %v", m["nivel"])
	}
}

func TestTestarRestauracaoNaoMexeNoBancoEmUso(t *testing.T) {
	env := novaAppTeste(t)
	env.criarPeca("TR-1", "Peca do teste", 1000, 2000, 1)
	if r := env.post("/api/backup", nil); r.status != 200 {
		t.Fatalf("backup falhou: %d", r.status)
	}
	// depois do backup entra outra peça (não pode aparecer no arquivo testado)
	env.criarPeca("TR-2", "Peca de depois do backup", 1000, 2000, 1)

	r := env.post("/api/backup/testar", nil)
	if r.status != 200 {
		t.Fatalf("testar restauração deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	m := r.json(t)
	if m["ok"] != true {
		t.Errorf("a resposta deveria dizer ok: %s", r.corpo)
	}
	if m["pecas"].(float64) != 1 {
		t.Errorf("o backup testado tem 1 peça, veio %v", m["pecas"])
	}
	// o banco em uso continua com as 2 peças
	if n := env.qint(`SELECT COUNT(*) FROM parts`); n != 2 {
		t.Errorf("o teste não pode alterar o banco em uso (esperava 2 peças, tem %d)", n)
	}
	// arquivo inexistente: 400 com mensagem clara
	if r := env.post("/api/backup/testar", map[string]any{"arquivo": "nao-existe.db"}); r.status != 400 {
		t.Errorf("testar arquivo inexistente deveria dar 400, veio %d", r.status)
	}
}

func TestKitEmergenciaTemProgramaBackupEInstrucao(t *testing.T) {
	env := novaAppTeste(t)
	// kit sem backup: recusa explicando
	if r := env.post("/api/kit-emergencia", nil); r.status != 500 {
		t.Errorf("sem backup o kit deveria ser recusado, veio %d (%s)", r.status, r.corpo)
	}
	env.criarPeca("KE-1", "Peca do kit", 1000, 2000, 1)
	if r := env.post("/api/backup", nil); r.status != 200 {
		t.Fatalf("backup falhou")
	}
	r := env.post("/api/kit-emergencia", nil)
	if r.status != 200 {
		t.Fatalf("gerar kit deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	m := r.json(t)
	caminho, _ := m["arquivo"].(string)
	if !arquivoExiste(caminho) {
		t.Fatalf("o kit não foi criado em %s", caminho)
	}
	if m["tamanho_bytes"].(float64) < 1000 {
		t.Errorf("o kit parece vazio: %v bytes", m["tamanho_bytes"])
	}
	// confere o conteudo do zip
	zr, err := zip.OpenReader(caminho)
	if err != nil {
		t.Fatalf("abrindo o kit: %v", err)
	}
	defer zr.Close()
	nomes := []string{}
	for _, f := range zr.File {
		nomes = append(nomes, f.Name)
	}
	juntos := strings.Join(nomes, " ")
	if !strings.Contains(juntos, "LEIA-ME-EMERGENCIA.txt") {
		t.Errorf("o kit deveria ter a instrução: %v", nomes)
	}
	if !strings.Contains(juntos, "backup") {
		t.Errorf("o kit deveria ter o backup: %v", nomes)
	}
	if !strings.Contains(juntos, ".exe") && !strings.Contains(juntos, "autopecas.test") {
		t.Errorf("o kit deveria ter o programa: %v", nomes)
	}
	// download
	d := env.get("/api/kit-emergencia/baixar")
	if d.status != 200 {
		t.Errorf("baixar o kit deveria dar 200, veio %d", d.status)
	}
	if !strings.Contains(d.headers.Get("Content-Type"), "zip") {
		t.Errorf("o download deveria vir como zip, veio %q", d.headers.Get("Content-Type"))
	}
	if len(d.corpo) < 1000 {
		t.Errorf("o download veio vazio (%d bytes)", len(d.corpo))
	}
}

func TestPastaNaNuvemEscolhidaRecebeCopia(t *testing.T) {
	env := novaAppTeste(t)
	nuvem := t.TempDir()

	// sem pasta escolhida
	m := env.get("/api/nuvem").json(t)
	if m["atual"] != "" {
		t.Errorf("no começo não deveria ter pasta escolhida: %v", m["atual"])
	}

	// pasta inexistente é recusada com mensagem clara
	if r := env.post("/api/nuvem/testar", map[string]any{"pasta": filepath.Join(nuvem, "nao-existe")}); r.status != 400 {
		t.Errorf("pasta inexistente deveria dar 400, veio %d", r.status)
	}
	// caminho relativo é recusado
	if r := env.post("/api/nuvem/testar", map[string]any{"pasta": "Dropbox/Backup"}); r.status != 400 {
		t.Errorf("caminho relativo deveria dar 400, veio %d", r.status)
	}
	// pasta boa: aceita
	if r := env.post("/api/nuvem/testar", map[string]any{"pasta": nuvem}); r.status != 200 {
		t.Errorf("pasta válida deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}

	// configura e faz backup: o arquivo tem que aparecer na pasta da nuvem
	if r := env.post("/api/config", map[string]any{"backup_pasta_extra": nuvem}); r.status != 200 {
		t.Fatalf("salvar config falhou: %d", r.status)
	}
	if r := env.post("/api/backup", nil); r.status != 200 {
		t.Fatalf("backup falhou: %d", r.status)
	}
	copias := arquivosComPrefixo(nuvem, "backup")
	if len(copias) == 0 {
		t.Errorf("o backup deveria ter sido copiado para a pasta da nuvem")
	}
	// e o painel mostra a nuvem como OK
	m = env.get("/api/saude-backup").json(t)
	nuvemInfo := m["nuvem"].(map[string]any)
	if nuvemInfo["ok"] != true {
		t.Errorf("o painel deveria mostrar a nuvem como acessível: %v", nuvemInfo)
	}
	if env.app.db.GetSetting("backup_pasta_extra_erro") != "" {
		t.Errorf("não deveria haver erro registrado: %s", env.app.db.GetSetting("backup_pasta_extra_erro"))
	}
}

func TestPastaNaNuvemComProblemaNaoDerrubaOBackup(t *testing.T) {
	env := novaAppTeste(t)
	env.app.db.SetSetting("backup_pasta_extra", filepath.Join(t.TempDir(), "sumiu"))
	r := env.post("/api/backup", nil)
	if r.status != 200 {
		t.Fatalf("o backup principal tem que funcionar mesmo com a nuvem quebrada, veio %d (%s)", r.status, r.corpo)
	}
	if env.app.db.GetSetting("backup_pasta_extra_erro") == "" {
		t.Errorf("o problema da pasta deveria ficar registrado para a tela mostrar")
	}
	if n := len(arquivosComPrefixo(env.app.backupDir(), "backup")); n == 0 {
		t.Errorf("o backup local deveria existir")
	}
}

func TestAbrirPastaSoAceitaPastasDoPrograma(t *testing.T) {
	env := novaAppTeste(t)
	r := env.post("/api/abrir-pasta", map[string]any{"qual": "C:\\Windows\\System32"})
	if r.status != 400 {
		t.Errorf("caminho arbitrário deveria ser recusado com 400, veio %d (%s)", r.status, r.corpo)
	}
	r = env.post("/api/abrir-pasta", map[string]any{"qual": "backup"})
	if r.status == 400 {
		t.Errorf("a pasta de backups deveria ser aceita, veio %d (%s)", r.status, r.corpo)
	}
}

func TestConferirBackupDeArquivoCorrompido(t *testing.T) {
	env := novaAppTeste(t)
	lixo := filepath.Join(env.app.backupDir(), "backup-quebrado-2026-01-01_0000.db")
	if err := os.WriteFile(lixo, []byte("isto nao e um banco de dados"), 0o644); err != nil {
		t.Fatalf("criando arquivo: %v", err)
	}
	if _, err := conferirBackup(lixo); err == nil {
		t.Errorf("um arquivo que não é banco deveria ser recusado")
	}
	if _, err := os.Stat(lixo); err != nil {
		t.Errorf("o arquivo não pode ser apagado pelo teste")
	}
}

func TestIntervaloDeBackupTemLimites(t *testing.T) {
	env := novaAppTeste(t)
	casos := []struct {
		minutos string
		espera  time.Duration
	}{
		{"", intervaloBackupPadrao},
		{"1", intervaloBackupPadrao},    // abaixo do mínimo
		{"60", time.Hour},               // aceito
		{"9999", intervaloBackupPadrao}, // acima do máximo
	}
	for _, c := range casos {
		env.app.db.SetSetting("backup_intervalo_minutos", c.minutos)
		if d := env.app.intervaloBackup(); d != c.espera {
			t.Errorf("intervalo %q deveria virar %v, veio %v", c.minutos, c.espera, d)
		}
	}
}

// garante que o registro do handler do kit nao ficou servindo arquivo errado
func TestBaixarKitSemGerar(t *testing.T) {
	env := novaAppTeste(t)
	r := env.get("/api/kit-emergencia/baixar")
	if r.status != 404 {
		t.Errorf("sem kit gerado o download deveria dar 404, veio %d", r.status)
	}
	var _ io.Reader = strings.NewReader("") // mantem o import io util se mudarmos o teste
	var _ = http.StatusOK
}

// ---------- 1.0.9: não encher o disco ----------

// arquivosDe cria arquivos de backup falsos (com tamanho) direto na pasta,
// para testar a retenção sem depender de gerar dezenas de bancos de verdade.
func arquivosDe(t *testing.T, dir, prefixo string, quantos int, tamanho int) {
	t.Helper()
	for i := 1; i <= quantos; i++ {
		nome := fmt.Sprintf("%s-2026-11-%02d_%02d00.db", prefixo, i, i)
		if err := os.WriteFile(filepath.Join(dir, nome), make([]byte, tamanho), 0o644); err != nil {
			t.Fatalf("criando arquivo de teste: %v", err)
		}
	}
}

func TestRetencaoPodeGuardarSoUmaCopia(t *testing.T) {
	env := novaAppTeste(t)
	dir := env.app.backupDir()
	env.app.db.SetSetting("backup_manter_horarios", "1")
	env.app.db.SetSetting("backup_manter_fechamentos", "1")
	env.app.db.SetSetting("backup_manter_manuais", "1")
	env.app.db.SetSetting("backup_manter", "1")
	env.app.db.SetSetting("backup_manter_seguranca", "1")

	arquivosDe(t, dir, "backup-horario", 6, 1024)
	arquivosDe(t, dir, "backup-fechamento", 4, 1024)
	arquivosDe(t, dir, "backup", 3, 1024)

	removidos, liberados, restante := env.app.limparBackups()

	if n := len(arquivosComPrefixo(dir, "backup-horario")); n != 1 {
		t.Errorf("com limite 1 deveria sobrar 1 backup de hora em hora, sobraram %d", n)
	}
	if n := len(arquivosComPrefixo(dir, "backup-fechamento")); n != 1 {
		t.Errorf("com limite 1 deveria sobrar 1 fechamento, sobraram %d", n)
	}
	if n := len(arquivosComPrefixo(dir, "backup-")); n < 1 {
		t.Errorf("não pode apagar tudo")
	}
	if removidos != 10 {
		t.Errorf("esperava apagar 10 arquivos (6+4+3-3), apagou %d", removidos)
	}
	if liberados != int64(10*1024) {
		t.Errorf("esperava liberar %d bytes, liberou %d", 10*1024, liberados)
	}
	if restante != int64(3*1024) {
		t.Errorf("esperava sobrar %d bytes, sobrou %d", 3*1024, restante)
	}
	// o que fica é sempre o MAIS NOVO
	sobraram := arquivosComPrefixo(dir, "backup-horario")
	if len(sobraram) == 1 && !strings.Contains(sobraram[0], "2026-11-06") {
		t.Errorf("deveria ter mantido a cópia mais nova, ficou %s", sobraram[0])
	}
}

func TestTetoDeEspacoApagaOsMaisAntigos(t *testing.T) {
	env := novaAppTeste(t)
	dir := env.app.backupDir()
	// 1 MB por arquivo, teto de 3 MB -> sobram no máximo 3 arquivos
	env.app.db.SetSetting("backup_limite_mb", "20") // conversão: 20 MB é o mínimo aceito
	env.app.db.SetSetting("backup_manter_horarios", "50")
	env.app.db.SetSetting("backup_manter", "50")
	arquivosDe(t, dir, "backup-horario", 5, 6*1024*1024) // 30 MB
	arquivosDe(t, dir, "backup-automatico", 2, 1*1024*1024)

	env.app.limparBackups()

	total := env.app.tamanhoBackups()
	if total > 20*1024*1024 {
		t.Errorf("a pasta ficou com %d bytes, acima do teto de 20 MB", total)
	}
	// nunca apaga a última de cada tipo
	if n := len(arquivosComPrefixo(dir, "backup-automatico")); n < 1 {
		t.Errorf("apagou todas as cópias diárias")
	}
	if n := len(arquivosComPrefixo(dir, "backup-horario")); n < 1 {
		t.Errorf("apagou todas as cópias de hora em hora")
	}
	// o mais novo continua lá
	sobraram := arquivosComPrefixo(dir, "backup-horario")
	maisNovo := false
	for _, n := range sobraram {
		if strings.Contains(n, "2026-11-05") {
			maisNovo = true
		}
	}
	if !maisNovo {
		t.Errorf("o backup mais novo foi apagado: %v", sobraram)
	}
}

func TestTetoZeroSignificaSemTeto(t *testing.T) {
	env := novaAppTeste(t)
	dir := env.app.backupDir()
	env.app.db.SetSetting("backup_limite_mb", "0")
	env.app.db.SetSetting("backup_manter_horarios", "50")
	arquivosDe(t, dir, "backup-horario", 5, 3*1024*1024)
	env.app.limparBackups()
	if n := len(arquivosComPrefixo(dir, "backup-horario")); n != 5 {
		t.Errorf("com teto 0 nada deveria sair (%d arquivos)", n)
	}
}

func TestRotaApagarAntigosRespondeOQueSaiu(t *testing.T) {
	env := novaAppTeste(t)
	dir := env.app.backupDir()
	env.app.db.SetSetting("backup_manter_horarios", "1")
	arquivosDe(t, dir, "backup-horario", 3, 2048)

	r := env.post("/api/backup/limpar", map[string]any{})
	if r.status != 200 {
		t.Fatalf("rota de limpeza: %d %s", r.status, r.corpo)
	}
	j := r.json(t)
	if v, _ := j["removidos"].(float64); int(v) != 2 {
		t.Errorf("deveria informar 2 removidos, veio %v", j["removidos"])
	}
	if v, _ := j["liberados_bytes"].(float64); int64(v) != 4096 {
		t.Errorf("deveria informar 4096 bytes liberados, veio %v", j["liberados_bytes"])
	}
	if txt, _ := j["total_texto"].(string); txt == "" {
		t.Errorf("deveria informar quanto sobrou em texto legível")
	}
}

func TestSaudeDoBackupInformaOTeto(t *testing.T) {
	env := novaAppTeste(t)
	s := env.get("/api/saude-backup").json(t)
	if v, ok := s["limite_mb"]; !ok || v == nil {
		t.Errorf("o painel deveria informar o teto de espaço configurado")
	}
	if _, ok := s["total_bytes"]; !ok {
		t.Errorf("o painel deveria informar quanto a pasta ocupa")
	}
}

package main

// Plano B (versao 1.0.7): backup de hora em hora, backup ao fechar o caixa,
// painel de saude do backup, teste de restauracao, kit de emergencia e escolha
// da pasta na nuvem (Dropbox / Google Drive / OneDrive / pendrive).
//
// Regras de seguranca que valem aqui:
//   - backup NUNCA trava a loja: qualquer falha vira aviso na tela, nao erro;
//   - um backup por hora (configuravel), e a limpeza guarda uma cota por tipo;
//   - a pasta na nuvem e uma PASTA LOCAL do programa de sincronizacao: o
//     AutoPecas nao fala com a internet em momento nenhum;
//   - testar restauracao abre uma copia em modo somente leitura.
//
// Cuidado com transacao: VACUUM INTO nao pode rodar dentro de a.db.Tx
// (o banco tem uma unica conexao). Todas as funcoes daqui rodam soltas.

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------- configuracoes e limites ----------------

const (
	intervaloBackupPadrao = 60 * time.Minute
	intervaloMinimo       = 15 * time.Minute
	intervaloMaximo       = 24 * time.Hour
	batidaBackup          = 5 * time.Minute // de quanto em quanto o relogio olha
)

// categoriaBackup junta o prefixo do arquivo com quantos devem ser guardados.
// A ordem importa: prefixos mais especificos vem antes de "backup-".
type categoriaBackup struct {
	prefixo      string
	chaveLimite  string
	limitePadrao int
	rotulo       string
}

var categoriasBackup = []categoriaBackup{
	{"backup-horario", "backup_manter_horarios", 24, "de hora em hora"},
	{"backup-automatico", "backup_manter", 30, "diário (ao abrir)"},
	{"backup-fechamento", "backup_manter_fechamentos", 60, "ao fechar o caixa"},
	{"backup", "backup_manter_manuais", 60, "manual"},
	{"antes-de-restaurar", "backup_manter_seguranca", 10, "segurança (antes de restaurar)"},
}

func categoriaDe(nome string) categoriaBackup {
	for _, c := range categoriasBackup {
		if strings.HasPrefix(nome, c.prefixo+"-") || nome == c.prefixo+".db" {
			return c
		}
	}
	return categoriaBackup{"outros", "backup_manter", 30, "outros"}
}

func (a *App) limiteDaCategoria(c categoriaBackup) int {
	n, _ := strconv.Atoi(a.db.GetSetting(c.chaveLimite))
	if n <= 0 {
		if c.chaveLimite == "backup_manter" {
			if n2, err := strconv.Atoi(a.db.GetSetting("backup_manter")); err == nil && n2 > 0 {
				return n2
			}
		}
		return c.limitePadrao
	}
	return n
}

func (a *App) intervaloBackup() time.Duration {
	min, _ := strconv.Atoi(a.db.GetSetting("backup_intervalo_minutos"))
	d := time.Duration(min) * time.Minute
	if d < intervaloMinimo || d > intervaloMaximo {
		return intervaloBackupPadrao
	}
	return d
}

// ---------------- criacao dos backups ----------------

// criarBackup cria um backup do tipo pedido e copia para a pasta da nuvem.
// Devolve o caminho e o tamanho; erro so quando o backup principal falhou.
func (a *App) criarBackup(prefixo string) (string, int64, error) {
	caminho, tamanho, err := a.criaArquivoBackup(prefixo)
	if err != nil {
		a.db.SetSetting("backup_ultimo_erro", nowISO()+" - "+err.Error())
		return "", 0, err
	}
	a.db.SetSetting("backup_ultimo_erro", "")
	switch {
	case strings.HasPrefix(prefixo, "backup-horario"):
		a.db.SetSetting("ultimo_backup_horario", time.Now().Format(time.RFC3339))
	case strings.HasPrefix(prefixo, "backup-fechamento"):
		a.db.SetSetting("ultimo_backup_fechamento", nowISO())
	}
	return caminho, tamanho, nil
}

// backupHorarioSePreciso cria o backup da hora quando o anterior ja venceu.
// Devolve true quando criou um arquivo novo.
func (a *App) backupHorarioSePreciso() (string, bool) {
	if a.db.GetSetting("backup_auto") == "0" {
		return "", false
	}
	intervalo := a.intervaloBackup()
	if t, ok := instanteDoBanco(a.db.GetSetting("ultimo_backup_horario")); ok {
		if time.Since(t) < intervalo {
			return "", false
		}
	}
	p, _, err := a.criarBackup("backup-horario")
	if err != nil {
		log.Printf("aviso: backup de hora em hora falhou: %v", err)
		return "", false
	}
	log.Printf("backup automatico criado: %s", filepath.Base(p))
	return p, true
}

// iniciarBackupDeHoraEmHora mantem o nome antigo (usado no main) e passa a
// criar backup de verdade a cada intervalo configurado.
func (a *App) iniciarBackupDeHoraEmHora() func() {
	parar := make(chan struct{})
	go func() {
		t := time.NewTicker(batidaBackup)
		defer t.Stop()
		// primeira chance logo que o programa abre
		a.backupHorarioSePreciso()
		for {
			select {
			case <-t.C:
				if a.db.GetSetting("backup_auto") == "0" {
					continue
				}
				if _, err := a.ensureDailyBackup(); err != nil {
					log.Printf("aviso: backup diario falhou: %v", err)
				}
				a.backupHorarioSePreciso()
			case <-parar:
				return
			}
		}
	}()
	var vez sync.Once
	return func() { vez.Do(func() { close(parar) }) }
}

// ---------------- limpeza por categoria ----------------

// limparBackups aplica a retencao e devolve quantos arquivos sairam, quantos
// bytes foram liberados e quanto sobrou na pasta.
//
// Duas regras:
//  1. por categoria: guarda as N copias mais novas (configuravel, minimo 1);
//  2. teto de espaco: se a pasta passar do limite configurado, apaga os mais
//     antigos (de qualquer tipo) ate caber - nunca o mais novo de cada tipo.
func (a *App) limparBackups() (removidos int, liberados, restante int64) {
	dir := a.backupDir()
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, 0
	}
	grupos := map[string][]string{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".db") {
			continue
		}
		c := categoriaDe(e.Name())
		grupos[c.prefixo] = append(grupos[c.prefixo], e.Name())
	}
	apagar := func(nome string) {
		caminho := filepath.Join(dir, nome)
		var tam int64
		if fi, err := os.Stat(caminho); err == nil {
			tam = fi.Size()
		}
		if os.Remove(caminho) == nil {
			removidos++
			liberados += tam
		}
	}
	for _, c := range categoriasBackup {
		nomes := grupos[c.prefixo]
		if len(nomes) == 0 {
			continue
		}
		manter := a.limiteDaCategoria(c)
		if manter < 1 {
			manter = 1
		}
		// o carimbo no nome e YYYY-MM-DD_HHMM, entao ordem alfabetica = ordem do tempo
		sort.Sort(sort.Reverse(sort.StringSlice(nomes)))
		for i, n := range nomes {
			if i >= manter {
				apagar(n)
			}
		}
		if len(nomes) > manter {
			grupos[c.prefixo] = nomes[:manter]
		}
	}
	// teto de espaco (depois da limpeza por categoria)
	limite := a.limiteBackupBytes()
	if limite > 0 {
		todos := []string{}
		for _, nomes := range grupos {
			todos = append(todos, nomes...)
		}
		total := a.tamanhoBackups()
		if total > limite {
			// 1.0.10: a ordem tem que ser por DATA. Antes ordenava pelo nome
			// completo, e "antes-de-restaurar-..." vinha antes de "backup-..."
			// no alfabeto - ou seja, o teto apagava backup novo e deixava o
			// velho. Agora o mais antigo (pelo carimbo do nome) sai primeiro.
			sort.Slice(todos, func(i, j int) bool {
				qi, qj := carimboDoBackup(todos[i]), carimboDoBackup(todos[j])
				if qi.Equal(qj) {
					return todos[i] < todos[j]
				}
				return qi.Before(qj)
			})
			porPrefixo := map[string]int{}
			for _, nomes := range grupos {
				for _, n := range nomes {
					porPrefixo[categoriaDe(n).prefixo]++
				}
			}
			for _, n := range todos {
				if total <= limite {
					break
				}
				pref := categoriaDe(n).prefixo
				if porPrefixo[pref] <= 1 {
					continue // nunca apaga a ultima (mais nova) de um tipo
				}
				var tam int64
				if fi, err := os.Stat(filepath.Join(dir, n)); err == nil {
					tam = fi.Size()
				}
				antes := removidos
				apagar(n)
				if removidos > antes {
					total -= tam
				}
				porPrefixo[pref]--
			}
		}
	}
	return removidos, liberados, a.tamanhoBackups()
}

// carimboDoBackup le a data do proprio nome do arquivo
// (prefixo-AAAA-MM-DD_HHMM.db). Se o nome estiver fora do padrao, usa a data
// de modificacao do arquivo; sem nenhuma das duas, trata como muito antigo.
func carimboDoBackup(nome string) time.Time {
	base := strings.TrimSuffix(filepath.Base(nome), filepath.Ext(nome))
	// o carimbo tem 15 caracteres (2026-03-04_1530) e fica no fim do nome
	if len(base) >= 15 {
		if t, err := time.ParseInLocation("2006-01-02_1504", base[len(base)-15:], time.Local); err == nil {
			return t
		}
	}
	if st, err := os.Stat(nome); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}

// limparBackupsAntigos mantem o nome antigo (chamado depois de cada backup).
func (a *App) limparBackupsAntigos() {
	a.limparBackups()
}

// tamanhoBackups soma o tamanho de todos os arquivos de backup.
func (a *App) tamanhoBackups() int64 {
	var total int64
	ents, err := os.ReadDir(a.backupDir())
	if err != nil {
		return 0
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".db") {
			continue
		}
		if fi, err := e.Info(); err == nil {
			total += fi.Size()
		}
	}
	return total
}

// limiteBackupBytes devolve o teto de espaco da pasta de backups (0 = sem teto).
func (a *App) limiteBackupBytes() int64 {
	mb, _ := strconv.Atoi(a.db.GetSetting("backup_limite_mb"))
	if mb <= 0 {
		return 0
	}
	if mb < 20 {
		mb = 20
	}
	return int64(mb) * 1024 * 1024
}

// handleLimparBackups: POST /api/backup/limpar - aplica a retencao na hora e
// responde o que foi apagado (botao "Apagar os antigos agora").
func (a *App) handleLimparBackups(w http.ResponseWriter, r *http.Request) {
	removidos, liberados, restante := a.limparBackups()
	writeJSON(w, 200, map[string]any{
		"ok": true, "removidos": removidos, "liberados_bytes": liberados,
		"total_bytes": restante, "total_texto": fmtBytes(uint64(restante)),
	})
}

// ---------------- backup ao fechar o caixa ----------------

// handleFecharCaixa: POST /api/fechamento/fechar
// O dono da loja clica em "Fechar o dia" e o sistema guarda o backup daquele
// momento (o dia inteiro fica garantido, sem depender de memoria).
func (a *App) handleFecharCaixa(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Data string `json:"data"`
	}
	_ = readJSON(r, &in)
	data := strings.TrimSpace(in.Data)
	if data == "" {
		data = todayISO()
	}
	if _, err := time.Parse("2006-01-02", data); err != nil {
		fail(w, 400, "data inválida")
		return
	}
	p, tamanho, err := a.criarBackup("backup-fechamento")
	if err != nil {
		fail(w, 500, "não consegui gerar o backup do fechamento: "+err.Error())
		return
	}
	a.db.SetSetting("ultimo_fechamento", data+" "+time.Now().Format("15:04"))
	aviso := a.db.GetSetting("backup_pasta_extra_erro")
	writeJSON(w, 200, map[string]any{
		"ok": true, "data": data, "arquivo": filepath.Base(p),
		"tamanho_bytes": tamanho, "aviso_nuvem": aviso,
	})
}

// ---------------- painel Plano B ----------------

// handleSaudeBackup: GET /api/saude-backup
func (a *App) handleSaudeBackup(w http.ResponseWriter, r *http.Request) {
	itens := a.listarBackups()
	porTipo := map[string]int{}
	var totalBytes int64
	for _, it := range itens {
		porTipo[categoriaDe(it.nome).prefixo]++
		totalBytes += it.tamanho
	}
	ultimo := a.db.GetSetting("ultimo_backup")
	minutos := -1
	if t, ok := instanteDoBanco(ultimo); ok {
		minutos = int(time.Since(t).Minutes())
	}

	// nivel do semaforo
	nivel := "vermelho"
	switch {
	case minutos < 0:
		nivel = "vermelho"
	case minutos <= 90:
		nivel = "verde"
	case minutos <= 24*60:
		nivel = "amarelo"
	}

	extra := strings.TrimSpace(a.db.GetSetting("backup_pasta_extra"))
	extraOK, extraDetalhe := a.estadoPastaNuvem(extra)

	intervalo := a.intervaloBackup()
	resp := map[string]any{
		"ultimo_backup":            ultimo,
		"minutos_desde_ultimo":     minutos,
		"nivel":                    nivel,
		"limite_mb":                a.db.GetSetting("backup_limite_mb"),
		"total_arquivos":           len(itens),
		"total_bytes":              totalBytes,
		"por_tipo":                 porTipo,
		"rotulos":                  rotulosPorTipo(),
		"intervalo_minutos":        int(intervalo.Minutes()),
		"limite_horarios":          a.limiteDaCategoria(categoriasBackup[0]),
		"limite_diarios":           a.limiteDaCategoria(categoriasBackup[1]),
		"ultimo_horario":           a.db.GetSetting("ultimo_backup_horario"),
		"ultimo_fechamento":        a.db.GetSetting("ultimo_fechamento"),
		"ultimo_backup_fechamento": a.db.GetSetting("ultimo_backup_fechamento"),
		"ultimo_erro":              a.db.GetSetting("backup_ultimo_erro"),
		"pasta":                    a.backupDir(),
		"nuvem": map[string]any{
			"pasta":   extra,
			"ok":      extraOK,
			"detalhe": extraDetalhe,
			"erro":    a.db.GetSetting("backup_pasta_extra_erro"),
		},
		"disco":      espacoEmDisco(a.dir),
		"categorias": a.contagemPorCategoria(),
	}
	writeJSON(w, 200, resp)
}

func rotulosPorTipo() map[string]string {
	out := map[string]string{}
	for _, c := range categoriasBackup {
		out[c.prefixo] = c.rotulo
	}
	return out
}

func (a *App) contagemPorCategoria() []map[string]any {
	itens := a.listarBackups()
	conta := map[string]int{}
	for _, it := range itens {
		conta[categoriaDe(it.nome).prefixo]++
	}
	out := []map[string]any{}
	vistos := map[string]bool{}
	for _, c := range categoriasBackup {
		if vistos[c.prefixo] {
			continue
		}
		vistos[c.prefixo] = true
		out = append(out, map[string]any{
			"tipo": c.prefixo, "rotulo": c.rotulo,
			"quantidade": conta[c.prefixo], "limite": a.limiteDaCategoria(c),
		})
	}
	return out
}

type backupArquivo struct {
	nome    string
	quando  time.Time
	tamanho int64
}

func (a *App) listarBackups() []backupArquivo {
	ents, err := os.ReadDir(a.backupDir())
	if err != nil {
		return nil
	}
	out := []backupArquivo{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, backupArquivo{nome: e.Name(), quando: info.ModTime(), tamanho: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].quando.After(out[j].quando) })
	return out
}

// backupMaisRecente devolve o caminho do backup mais novo ("" se nao houver).
func (a *App) backupMaisRecente() string {
	lista := a.listarBackups()
	if len(lista) == 0 {
		return ""
	}
	return filepath.Join(a.backupDir(), lista[0].nome)
}

// ---------------- testar restauracao ----------------

// handleTestarBackup: POST /api/backup/testar
// Abre o backup mais recente (ou o arquivo indicado) em modo somente leitura,
// confere a integridade e conta o que tem dentro. NAO toca no banco em uso.
func (a *App) handleTestarBackup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Arquivo string `json:"arquivo"`
	}
	_ = readJSON(r, &in)
	caminho := ""
	if nome := filepath.Base(strings.TrimSpace(in.Arquivo)); nome != "" && nome != "." {
		caminho = filepath.Join(a.backupDir(), nome)
	} else {
		caminho = a.backupMaisRecente()
	}
	if caminho == "" {
		fail(w, 404, "ainda não existe nenhum backup para testar")
		return
	}
	res, err := conferirBackup(caminho)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// conferirBackup abre o arquivo em modo leitura e devolve um resumo do conteudo.
func conferirBackup(caminho string) (map[string]any, error) {
	if _, err := os.Stat(caminho); err != nil {
		return nil, fmt.Errorf("backup não encontrado: %s", filepath.Base(caminho))
	}
	pecas, err := validarArquivoBackup(caminho)
	if err != nil {
		return nil, err
	}
	db, err := OpenDBLeitura(caminho)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	contar := func(sqlText string) int64 {
		var n int64
		if err := db.QueryRow(sqlText).Scan(&n); err != nil {
			return 0
		}
		return n
	}
	vendas := contar(`SELECT COUNT(*) FROM sales WHERE status='concluida'`)
	clientes := contar(`SELECT COUNT(*) FROM customers`)
	movimentos := contar(`SELECT COUNT(*) FROM stock_moves`)
	var ultimaVenda string
	_ = db.QueryRow(`SELECT IFNULL(MAX(created_at),'') FROM sales`).Scan(&ultimaVenda)
	st, _ := os.Stat(caminho)
	tamanho := int64(0)
	if st != nil {
		tamanho = st.Size()
	}
	return map[string]any{
		"arquivo":       filepath.Base(caminho),
		"tamanho_bytes": tamanho,
		"pecas":         pecas,
		"vendas":        vendas,
		"clientes":      clientes,
		"movimentos":    movimentos,
		"ultima_venda":  ultimaVenda,
		"ok":            true,
	}, nil
}

// ---------------- kit de emergencia ----------------

const nomeKit = "Kit-Emergencia-AutoPecas.zip"

// handleGerarKit: POST /api/kit-emergencia
func (a *App) handleGerarKit(w http.ResponseWriter, r *http.Request) {
	caminho, tamanho, aviso, err := a.gerarKitEmergencia()
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok": true, "arquivo": caminho, "nome": filepath.Base(caminho),
		"tamanho_bytes": tamanho, "aviso": aviso,
	})
}

// gerarKitEmergencia monta um .zip com o proprio programa, o backup mais novo e
// uma instrucao de uma pagina. Serve para o caso "o computador morreu".
func (a *App) gerarKitEmergencia() (string, int64, string, error) {
	backup := a.backupMaisRecente()
	if backup == "" {
		return "", 0, "", fmt.Errorf("não existe backup para colocar no kit. Clique em “Fazer backup agora” primeiro")
	}
	exe, err := os.Executable()
	if err != nil || !arquivoExiste(exe) {
		return "", 0, "", fmt.Errorf("não consegui localizar o programa para copiar")
	}
	dir := filepath.Join(a.dir, "kit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, "", err
	}
	destino := filepath.Join(dir, nomeKit)
	tmp := destino + ".parcial"
	f, err := os.Create(tmp)
	if err != nil {
		return "", 0, "", err
	}
	zw := zip.NewWriter(f)
	addArquivo := func(nome, origem string) error {
		src, err := os.Open(origem)
		if err != nil {
			return err
		}
		defer src.Close()
		w, err := zw.Create(nome)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, src)
		return err
	}
	if err := addArquivo(filepath.Base(exe), exe); err != nil {
		zw.Close()
		f.Close()
		_ = os.Remove(tmp)
		return "", 0, "", err
	}
	if err := addArquivo(filepath.Base(backup), backup); err != nil {
		zw.Close()
		f.Close()
		_ = os.Remove(tmp)
		return "", 0, "", err
	}
	instrucao := a.textoEmergencia(backup)
	w, err := zw.Create("LEIA-ME-EMERGENCIA.txt")
	if err == nil {
		_, _ = w.Write([]byte(instrucao))
	}
	if err := zw.Close(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return "", 0, "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", 0, "", err
	}
	if err := os.Rename(tmp, destino); err != nil {
		_ = os.Remove(tmp)
		return "", 0, "", err
	}
	st, err := os.Stat(destino)
	if err != nil {
		return "", 0, "", err
	}
	// o kit tambem vai para a nuvem/pendrive quando configurado
	aviso := ""
	if origem, err := os.Open(destino); err == nil {
		destinoExtra := strings.TrimSpace(a.db.GetSetting("backup_pasta_extra"))
		if destinoExtra != "" {
			if alvo, err := os.Create(filepath.Join(destinoExtra, nomeKit)); err == nil {
				if _, err := io.Copy(alvo, origem); err != nil {
					aviso = "o kit foi gerado, mas não consegui copiar para a nuvem: " + err.Error()
				}
				alvo.Close()
			} else {
				aviso = "o kit foi gerado, mas não consegui copiar para a nuvem (pasta inacessível)"
			}
		}
		origem.Close()
	}
	a.db.SetSetting("ultimo_kit", nowISO())
	return destino, st.Size(), aviso, nil
}

func (a *App) textoEmergencia(backup string) string {
	return fmt.Sprintf(`AUTOPECAS — KIT DE EMERGENCIA
Gerado em: %s

Este zip tem tudo o que voce precisa para continuar trabalhando em outro
computador, sem instalar nada.

COMO USAR (5 minutos)
1. Copie este zip para o outro computador (pen drive, e-mail, nuvem).
2. Descompacte (botao direito > Extrair tudo).
3. Abra o programa "AutoPecas.exe" que esta aqui dentro. Ele ja vem pronto
   para rodar sem instalar; os dados ficam numa pasta do usuario.
4. No programa, abra "Backup e Ajustes" (menu da esquerda).
5. Em "Restaurar backup de outro computador", escolha o arquivo:
   %s
6. Confirme. Pronto: o sistema volta com as pecas, vendas, clientes e contas.
7. Se quiser usar no celular tambem, va em "Inicio > Acessar pelo celular" e
   ligue de novo com uma senha.

O QUE TEM AQUI DENTRO
- AutoPecas.exe .................. o programa
- %s ... backup do dia (o mais recente)
- LEIA-ME-EMERGENCIA.txt ......... este texto

IMPORTANTE
- O backup guarda os dados ate o momento em que ele foi gerado.
  O que foi lancado depois disso precisa ser digitado de novo.
- Nao apague este zip depois de restaurar: ele e a sua copia de seguranca.
`, time.Now().Format("02/01/2006 15:04"), backup, filepath.Base(backup))
}

// handleBaixarKit: GET /api/kit-emergencia/baixar
func (a *App) handleBaixarKit(w http.ResponseWriter, r *http.Request) {
	p := filepath.Join(a.dir, "kit", nomeKit)
	f, err := os.Open(p)
	if err != nil {
		fail(w, 404, "o kit ainda não foi gerado. Clique em “Gerar kit de emergência”")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+nomeKit)
	_, _ = io.Copy(w, f)
}

// ---------------- pasta na nuvem ----------------

// candidatosNuvem lista as pastas de sincronizacao que existem nesta maquina.
// O AutoPecas nao conversa com a internet: ele so grava numa pasta e o
// programa da nuvem (Dropbox, Drive, OneDrive) faz o envio.
func candidatosNuvem() []map[string]string {
	casa, _ := os.UserHomeDir()
	cands := []struct{ nome, caminho string }{
		{"Dropbox", filepath.Join(casa, "Dropbox")},
		{"Dropbox (empresa)", filepath.Join(casa, "Dropbox (Business)")},
		{"Google Drive", filepath.Join(casa, "Google Drive")},
		{"Google Drive (Meu Drive)", filepath.Join(casa, "Meu Drive")},
		{"Google Drive (unidade G:)", "G:\\Meu Drive"},
		{"OneDrive", filepath.Join(casa, "OneDrive")},
		{"iCloud Drive", filepath.Join(casa, "iCloudDrive")},
	}
	// pastas "OneDrive - Nome da Empresa"
	if ents, err := os.ReadDir(casa); err == nil {
		for _, e := range ents {
			if e.IsDir() && strings.HasPrefix(strings.ToLower(e.Name()), "onedrive -") {
				cands = append(cands, struct{ nome, caminho string }{"OneDrive (empresa)", filepath.Join(casa, e.Name())})
			}
		}
	}
	out := []map[string]string{}
	vistos := map[string]bool{}
	for _, c := range cands {
		if vistos[c.caminho] {
			continue
		}
		st, err := os.Stat(c.caminho)
		if err != nil || !st.IsDir() {
			continue
		}
		vistos[c.caminho] = true
		out = append(out, map[string]string{
			"servico": c.nome,
			"pasta":   c.caminho,
			"estado":  "encontrada",
		})
	}
	return out
}

// estadoPastaNuvem diz se a pasta configurada existe e aceita escrita.
func (a *App) estadoPastaNuvem(pasta string) (bool, string) {
	if pasta == "" {
		return false, "nenhuma pasta escolhida"
	}
	st, err := os.Stat(pasta)
	if err != nil || !st.IsDir() {
		return false, "a pasta não existe ou não está acessível"
	}
	teste := filepath.Join(pasta, ".autopecas-teste")
	if err := os.WriteFile(teste, []byte("ok"), 0o644); err != nil {
		return false, "a pasta existe, mas o sistema não consegue gravar nela"
	}
	_ = os.Remove(teste)
	return true, "pasta acessível e gravando"
}

// handleNuvem: GET /api/nuvem  (candidatas + a pasta configurada hoje)
func (a *App) handleNuvem(w http.ResponseWriter, r *http.Request) {
	atual := strings.TrimSpace(a.db.GetSetting("backup_pasta_extra"))
	ok, detalhe := a.estadoPastaNuvem(atual)
	writeJSON(w, 200, map[string]any{
		"atual":      atual,
		"atual_ok":   ok,
		"detalhe":    detalhe,
		"candidatas": candidatosNuvem(),
		"erro":       a.db.GetSetting("backup_pasta_extra_erro"),
	})
}

// handleNuvemTestar: POST /api/nuvem/testar { pasta }
func (a *App) handleNuvemTestar(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Pasta string `json:"pasta"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados inválidos")
		return
	}
	pasta := strings.TrimSpace(in.Pasta)
	if pasta == "" {
		writeJSON(w, 200, map[string]any{"ok": true, "vazio": true, "detalhe": "nenhuma pasta escolhida"})
		return
	}
	if !filepath.IsAbs(pasta) {
		fail(w, 400, "informe o caminho completo da pasta (ex.: C:\\Users\\Voce\\Dropbox\\Backup AutoPecas)")
		return
	}
	if ok, detalhe := a.estadoPastaNuvem(pasta); !ok {
		fail(w, 400, detalhe)
		return
	}
	// grava e apaga um arquivo de verdade, para nao descobrir o problema depois
	teste := filepath.Join(pasta, ".autopecas-teste")
	if err := os.WriteFile(teste, []byte("teste do AutoPecas"), 0o644); err != nil {
		fail(w, 400, "não consegui gravar na pasta: "+err.Error())
		return
	}
	_ = os.Remove(teste)
	writeJSON(w, 200, map[string]any{"ok": true, "pasta": pasta, "detalhe": "pasta acessível e gravando"})
}

// ---------------- utilitarios ----------------

// instanteDoBanco entende os formatos de data que o sistema grava
// (RFC3339 com fuso, "2006-01-02 15:04:05" e so a data).
func instanteDoBanco(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// fmtBytes escreve o tamanho do jeito que o dono da loja entende.
func fmtBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

func arquivoExiste(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// handleAbrirPasta: POST /api/abrir-pasta { qual }
// Abre no Explorador apenas as pastas do proprio sistema (dados, backups, kit e
// a pasta da nuvem configurada) - nunca um caminho arbitrario.
func (a *App) handleAbrirPasta(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Qual string `json:"qual"`
	}
	_ = readJSON(r, &in)
	var pasta string
	switch strings.ToLower(strings.TrimSpace(in.Qual)) {
	case "backup", "backups":
		pasta = a.backupDir()
	case "dados", "dados-loja":
		pasta = a.dir
	case "kit":
		pasta = filepath.Join(a.dir, "kit")
	case "nuvem":
		pasta = strings.TrimSpace(a.db.GetSetting("backup_pasta_extra"))
	default:
		fail(w, 400, "pasta desconhecida")
		return
	}
	if pasta == "" {
		fail(w, 400, "nenhuma pasta configurada")
		return
	}
	if !arquivoExiste(pasta) {
		if _, err := os.Stat(pasta); err != nil {
			fail(w, 404, "a pasta não existe: "+pasta)
			return
		}
	}
	if err := abrirPasta(pasta); err != nil {
		fail(w, 500, "não consegui abrir a pasta: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "pasta": pasta})
}

func abrirPasta(pasta string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", pasta).Start()
	case "darwin":
		return exec.Command("open", pasta).Start()
	default:
		return exec.Command("xdg-open", pasta).Start()
	}
}

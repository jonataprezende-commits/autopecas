package main

// AutoPecas - Sistema de gestao para loja de autopecas
// Programa local (offline), banco SQLite, interface web embutida.

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var webFS embed.FS

const appName = "AutoPecas"
const appVersion = "1.1.0"

type App struct {
	db  *DB
	dir string

	// item 7: seguranca do servidor local
	hostEscuta    string // valor de escuta em uso (ex.: 127.0.0.1)
	hostExplicito string // o que veio em -host na linha de comando
	// 1.0.7 - acesso pelo celular
	senhaRede      string              // senha vinda da opcao -senha-rede
	senhaHash      string              // senha gravada pela tela (SHA-256)
	senhaSalt      string              // sal da senha gravada
	sessoes        *gerenciadorSessoes // sessoes abertas no navegador/celular
	loginMu        sync.Mutex          // protege o mapa de tentativas
	falhas         map[string]*falhasLogin
	hostsExtras    []string // nomes liberados alem do IP local (-hosts-permitidos)
	proxyConfiavel bool     // confia em X-Forwarded-* (-proxy)
	srv            *http.Server
	ln             *listenerTrocavel // endereco de escuta (pode mudar em tempo real)
	porta          int               // porta realmente em uso (0 = desconhecida)

	// item 9/11: encerramento limpo (para o backup de hora em hora)
	pararTarefas func()
}

// avisoErro mostra uma caixa de mensagem no Windows mesmo sem console
// (o programa e compilado como aplicacao grafica) e grava um log simples.
func avisoErro(msg string) {
	log.Printf("ERRO: %s", msg)
	base, _ := os.UserConfigDir()
	if base != "" {
		dir := filepath.Join(base, appName)
		_ = os.MkdirAll(dir, 0o755)
		f, err := os.OpenFile(filepath.Join(dir, "erro.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%s - %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
			f.Close()
		}
	}
	// item 8: caixa de mensagem nativa do Windows (MessageBoxW).
	// O antigo mshta/VBScript foi removido (o Windows esta bloqueando VBScript).
	mostrarCaixaErro(msg)
}

func main() {
	porta := flag.Int("porta", 8787, "porta local do sistema")
	host := flag.String("host", "127.0.0.1", "endereco de escuta (127.0.0.1 = so este computador)")
	dados := flag.String("dados", "", "pasta de dados (padrao: pasta do usuario)")
	semNavegador := flag.Bool("sem-navegador", false, "nao abrir o navegador automaticamente")
	senhaRede := flag.String("senha-rede", "", "senha de acesso quando o sistema e aberto na rede (-host diferente de 127.0.0.1)")
	hostsPermitidos := flag.String("hosts-permitidos", "", "nomes extras aceitos no navegador, separados por virgula (ex.: autopecas.minhaloja.com). Use com tunel/VPN")
	aguardarPorta := flag.Bool("aguardar-porta", false, "uso interno: espera a porta do reinicio ficar livre (nao use na mao)")
	atrasDeProxy := flag.Bool("proxy", false, "o programa esta atras de um tunel/proxy (Cloudflare, ngrok): confia em X-Forwarded-Proto e X-Forwarded-Host")
	encerrar := flag.Bool("encerrar", false, "fecha o AutoPecas que estiver aberto nesta maquina")
	flag.Parse()

	app := &App{hostEscuta: *host, hostExplicito: *host, porta: *porta, senhaRede: strings.TrimSpace(*senhaRede)}
	app.proxyConfiavel = *atrasDeProxy
	for _, h := range strings.Split(*hostsPermitidos, ",") {
		if h = strings.TrimSpace(h); h != "" {
			app.hostsExtras = append(app.hostsExtras, h)
		}
	}

	if *encerrar {
		if err := app.pedirEncerramento(); err != nil {
			fmt.Printf(" %s nao esta aberto (%v).\n", appName, err)
		} else {
			fmt.Printf(" %s encerrado.\n", appName)
		}
		return
	}

	dir := *dados
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil || base == "" {
			base = "."
		}
		dir = filepath.Join(base, appName)
	}
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0o755); err != nil {
		avisoErro("Nao foi possivel criar a pasta de dados " + dir + ": " + err.Error())
		return
	}

	db, err := OpenDB(filepath.Join(dir, "autopecas.db"))
	if err != nil {
		avisoErro("Erro ao abrir o banco de dados: " + err.Error())
		return
	}
	defer db.Close()

	app.db = db
	app.dir = dir
	if err := app.migrate(); err != nil {
		avisoErro("Erro ao preparar o banco de dados: " + err.Error())
		return
	}
	app.firstRun()

	// 1.0.7: senha e modo de acesso gravados pela tela "Acessar pelo celular"
	app.senhaSalt = app.db.GetSetting("senha_rede_salt")
	app.senhaHash = app.db.GetSetting("senha_rede_hash")
	if app.senhaRede == "" && app.senhaHash != "" && app.senhaSalt == "" {
		// senha gravada sem sal (nao deveria acontecer): descarta
		app.senhaHash = ""
	}
	if app.db.GetSetting("acesso_rede") == "1" {
		if !app.exigeSenha() {
			// Banco restaurado de outro computador (ou senha apagada) com o acesso
			// pelo celular marcado: volta para o modo local em vez de recusar abrir.
			app.db.SetSetting("acesso_rede", "0")
			avisoErro("O acesso pelo celular estava ligado, mas a senha não foi encontrada. " +
				"O sistema abriu só neste computador. Ligue de novo em Início → Acessar pelo celular.")
		} else if ehEnderecoLocal(*host) {
			*host = "0.0.0.0"
			app.hostEscuta = *host
		}
	}

	// item 7e: nao abre na rede sem senha
	if err := app.validarInicial(); err != nil {
		fmt.Printf(" %s: %v\n", appName, err)
		avisoErro(err.Error())
		return
	}

	// Backup automatico do dia (uma vez por dia, silencioso)
	if app.db.GetSetting("backup_auto") != "0" {
		if _, err := app.ensureDailyBackup(); err != nil {
			log.Printf("aviso: backup automatico falhou: %v", err)
		}
	}

	// Escolhe uma porta livre
	var ln net.Listener
	url := ""
	portaReal := 0
	start := *porta
	if *aguardarPorta {
		// reinicio: o processo anterior pode levar alguns segundos para soltar a
		// porta. Sem esta espera o sistema voltaria em 8841 e a pagina aberta no
		// navegador (8840) nunca responderia de novo.
		// 1.0.11: a espera era de 12 s e o teste era feito no endereco ANTIGO
		// (127.0.0.1), mas o processo novo sobe em 0.0.0.0 quando o acesso pelo
		// celular e ligado. Quando o processo velho demorava a soltar a porta,
		// este dava a porta por ocupada, concluia "ja esta aberto" e morria
		// calado — o sistema sumia da tela. Agora espera ate 90 s na MESMA porta.
		limite := time.Now().Add(90 * time.Second)
		for time.Now().Before(limite) {
			if l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *host, start)); err == nil {
				ln = l
				portaReal = start
				break
			}
			time.Sleep(120 * time.Millisecond)
		}
		if ln != nil {
			url = fmt.Sprintf("http://127.0.0.1:%d", start)
		} else {
			fmt.Printf("aviso: a porta %d nao ficou livre; abrindo em outra porta.\n", start)
		}
	}
	for p := start; p < start+30 && ln == nil; p++ {
		addr := fmt.Sprintf("%s:%d", *host, p)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			// "ja esta aberto" NAO vale durante um reinicio: aqui quem esta
			// respondendo e o processo velho que ainda esta fechando.
			if p == start && !*aguardarPorta && app.alreadyRunning(fmt.Sprintf("http://127.0.0.1:%d", p)) {
				fmt.Printf("%s ja esta aberto. Abrindo a janela...\n", appName)
				if !*semNavegador {
					openBrowser(fmt.Sprintf("http://127.0.0.1:%d", p))
				}
				return
			}
			continue
		}
		ln = l
		portaReal = p
		url = fmt.Sprintf("http://127.0.0.1:%d", p)
		break
	}
	if ln == nil {
		avisoErro("Nao encontrei porta livre para iniciar o sistema. Feche outros programas e tente novamente.")
		return
	}

	app.porta = portaReal // a partir daqui a trava de Host usa a porta real
	// 1.0.11: o endereco de escuta pode mudar com o programa aberto (ligar o
	// acesso pelo celular). O listener "trocavel" permite passar de
	// 127.0.0.1:8787 para 0.0.0.0:8787 sem fechar e reabrir o programa.
	app.ln = &listenerTrocavel{ln: ln}
	srv := &http.Server{Handler: app.routes(), ReadHeaderTimeout: 20 * time.Second}
	app.srv = srv

	fmt.Printf("==============================================\n")
	fmt.Printf(" %s %s - pronto para usar\n", appName, appVersion)
	fmt.Printf(" Pasta de dados: %s\n", dir)
	fmt.Printf(" Endereco: %s\n", url)
	fmt.Printf(" NAO FECHE ESTA JANELA enquanto estiver usando o sistema.\n")
	fmt.Printf("==============================================\n")

	// item 9: backup verificado de hora em hora (para no encerramento)
	app.pararTarefas = app.iniciarBackupDeHoraEmHora()
	defer func() {
		if app.pararTarefas != nil {
			app.pararTarefas()
		}
	}()

	if !*semNavegador {
		go func() {
			time.Sleep(400 * time.Millisecond)
			openBrowser(url)
		}()
	}
	if err := srv.Serve(app.ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		avisoErro("Erro no servidor: " + err.Error())
	}
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (a *App) alreadyRunning(base string) bool {
	c := http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get(base + "/api/ping")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return strings.Contains(string(b), appName)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func (a *App) routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"app": appName, "versao": appVersion, "pasta": a.dir})
	})

	// Acesso pelo celular / pela rede (1.0.7)
	m.HandleFunc("GET /api/sessao", a.handleSessao)
	m.HandleFunc("POST /api/entrar", a.handleEntrar)
	m.HandleFunc("POST /api/sair", a.handleSair)
	m.HandleFunc("GET /api/rede", a.handleRede)
	m.HandleFunc("POST /api/rede/aplicar", a.handleRedeAplicar)

	// Dashboard
	m.HandleFunc("GET /api/dashboard", a.handleDashboard)

	// Pecas
	m.HandleFunc("GET /api/pecas", a.handleListParts)
	m.HandleFunc("GET /api/pecas/{id}", a.handleGetPart)
	m.HandleFunc("POST /api/pecas", a.handleSavePart)
	m.HandleFunc("PUT /api/pecas/{id}", a.handleSavePart)
	m.HandleFunc("DELETE /api/pecas/{id}", a.handleDeletePart)
	m.HandleFunc("GET /api/pecas/{id}/historico", a.handlePartHistory)

	// XML
	m.HandleFunc("POST /api/xml/importar", a.handleImportXML)
	m.HandleFunc("POST /api/xml/analisar", a.handleAnalyzeXML)

	// catalogo do fornecedor (a peca que a loja nao tem) + busca rapida do balcao
	m.HandleFunc("GET /api/catalogo", a.handleCatalogo)
	m.HandleFunc("POST /api/catalogo", a.handleCatalogo)
	m.HandleFunc("PUT /api/catalogo/{id}", a.handleCatalogoItem)
	m.HandleFunc("DELETE /api/catalogo/{id}", a.handleCatalogoItem)
	m.HandleFunc("POST /api/catalogo/importar", a.handleCatalogoImportar)
	m.HandleFunc("POST /api/catalogo/semear", a.handleCatalogoSemear)
	m.HandleFunc("POST /api/catalogo/{id}/ficha", a.handleCatalogoFicha)
	m.HandleFunc("GET /api/busca", a.handleBuscaRapida)

	// nota de compra fotografada (OCR offline embutido)
	m.HandleFunc("GET /api/nota-foto/status", a.handleStatusNotaFoto)
	m.HandleFunc("POST /api/nota-foto/ler", a.handleLerNotaFoto)
	m.HandleFunc("POST /api/nota-foto/importar", a.handleImportarNotaFoto)

	// Estoque
	m.HandleFunc("POST /api/estoque/movimento", a.handleStockMove)
	m.HandleFunc("GET /api/estoque/movimentos", a.handleStockMoves)

	// Estoque Inteligente (v1.1)
	m.HandleFunc("GET /api/estoque-inteligente/resumo", a.handleEIResumo)
	m.HandleFunc("GET /api/estoque-inteligente/sugestoes", a.handleEISugestoes)
	m.HandleFunc("GET /api/estoque-inteligente/parados", a.handleEIParados)
	m.HandleFunc("GET /api/estoque-inteligente/abc", a.handleEIABC)
	m.HandleFunc("GET /api/estoque-inteligente/comparar", a.handleEIComparar)
	m.HandleFunc("GET /api/estoque-inteligente/config", a.handleEIConfig)
	m.HandleFunc("POST /api/estoque-inteligente/config", a.handleEIConfig)
	m.HandleFunc("POST /api/estoque-inteligente/pedido", a.handleEIGerarPedido)
	m.HandleFunc("GET /api/estoque-inteligente/sugestoes.csv", a.handleEICSV)
	m.HandleFunc("GET /api/estoque-inteligente/parados.csv", a.handleEICSV)
	m.HandleFunc("GET /api/estoque-inteligente/abc.csv", a.handleEICSV)
	m.HandleFunc("GET /api/estoque-inteligente/sugestoes.pdf", a.handleEIPDF)

	// Fichas de falta
	m.HandleFunc("GET /api/fichas", a.handleListFaltas)
	m.HandleFunc("POST /api/fichas", a.handleSaveFalta)
	m.HandleFunc("PUT /api/fichas/{id}", a.handleSaveFalta)
	m.HandleFunc("POST /api/fichas/{id}/status", a.handleFaltaStatus)
	m.HandleFunc("DELETE /api/fichas/{id}", a.handleDeleteFalta)

	// Pedidos de compra
	m.HandleFunc("GET /api/pedidos", a.handleListOrders)
	m.HandleFunc("GET /api/pedidos/{id}", a.handleGetOrder)
	m.HandleFunc("POST /api/pedidos/gerar", a.handleGenerateOrders)
	m.HandleFunc("POST /api/pedidos/{id}/receber", a.handleReceiveOrder)
	m.HandleFunc("POST /api/pedidos/{id}/cancelar", a.handleCancelOrder)

	// Vendas
	m.HandleFunc("GET /api/vendas", a.handleListSales)
	m.HandleFunc("GET /api/vendas/{id}", a.handleGetSale)
	m.HandleFunc("POST /api/vendas", a.handleCreateSale)
	m.HandleFunc("POST /api/vendas/{id}/cancelar", a.handleCancelSale)
	m.HandleFunc("GET /api/fechamento", a.handleDailyClose)
	m.HandleFunc("GET /api/recebiveis", a.handleListReceivables)
	m.HandleFunc("POST /api/recebiveis/{id}/baixar", a.handlePayReceivable)

	// contas a pagar (o dinheiro que sai) e notas de entrada
	m.HandleFunc("GET /api/pagaveis", a.handleListPayables)
	m.HandleFunc("POST /api/pagaveis", a.handleSavePayable)
	m.HandleFunc("PUT /api/pagaveis/{id}", a.handleSavePayable)
	m.HandleFunc("POST /api/pagaveis/{id}/baixar", a.handlePayPayable)
	m.HandleFunc("POST /api/pagaveis/{id}/cancelar", a.handleCancelPayable)
	m.HandleFunc("GET /api/notas-entrada", a.handleListNotasEntrada)

	// Clientes e veiculos
	m.HandleFunc("GET /api/clientes", a.handleListCustomers)
	m.HandleFunc("GET /api/clientes/{id}", a.handleGetCustomer)
	m.HandleFunc("POST /api/clientes", a.handleSaveCustomer)
	m.HandleFunc("PUT /api/clientes/{id}", a.handleSaveCustomer)
	m.HandleFunc("DELETE /api/clientes/{id}", a.handleDeleteCustomer)
	m.HandleFunc("POST /api/clientes/{id}/veiculos", a.handleSaveVehicle)
	m.HandleFunc("DELETE /api/veiculos/{id}", a.handleDeleteVehicle)

	// Fornecedores
	m.HandleFunc("GET /api/fornecedores", a.handleListSuppliers)
	m.HandleFunc("POST /api/fornecedores", a.handleSaveSupplier)
	m.HandleFunc("PUT /api/fornecedores/{id}", a.handleSaveSupplier)
	m.HandleFunc("DELETE /api/fornecedores/{id}", a.handleDeleteSupplier)

	// Relatorios
	m.HandleFunc("GET /api/relatorios/{tipo}", a.handleReport)
	m.HandleFunc("GET /api/relatorios/{tipo}/csv", a.handleReportCSV)
	m.HandleFunc("GET /api/relatorios/{tipo}/pdf", a.handleReportPDF)

	// Backup / configuracao
	m.HandleFunc("GET /api/backup", a.handleListBackups)
	// Plano B (1.0.7): saude do backup, teste de restauracao, kit e nuvem
	m.HandleFunc("GET /api/saude-backup", a.handleSaudeBackup)
	m.HandleFunc("POST /api/backup/testar", a.handleTestarBackup)
	m.HandleFunc("POST /api/kit-emergencia", a.handleGerarKit)
	m.HandleFunc("GET /api/kit-emergencia/baixar", a.handleBaixarKit)
	m.HandleFunc("GET /api/nuvem", a.handleNuvem)
	m.HandleFunc("POST /api/nuvem/testar", a.handleNuvemTestar)
	m.HandleFunc("POST /api/abrir-pasta", a.handleAbrirPasta)
	m.HandleFunc("POST /api/fechamento/fechar", a.handleFecharCaixa)
	m.HandleFunc("POST /api/backup/limpar", a.handleLimparBackups)
	m.HandleFunc("POST /api/backup", a.handleDoBackup)
	m.HandleFunc("POST /api/restaurar", a.handleRestore)
	m.HandleFunc("GET /api/backup/baixar", a.handleDownloadBackup)
	m.HandleFunc("GET /api/config", a.handleGetConfig)
	m.HandleFunc("POST /api/config", a.handleSaveConfig)
	m.HandleFunc("POST /api/encerrar", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
		go func() {
			time.Sleep(400 * time.Millisecond)
			if a.pararTarefas != nil {
				a.pararTarefas()
			}
			_ = a.db.Close()
			os.Exit(0)
		}()
	})

	// arquivos da interface (embutidos no executavel)
	sub, _ := fs.Sub(webFS, "web")
	m.Handle("GET /", noCache(http.FileServer(http.FS(sub))))

	return a.comSeguranca(m)
}

// listenerTrocavel e' um net.Listener cujo endereco pode ser trocado em tempo
// real: o http.Server continua sendo o mesmo e as conexoes abertas nao caem.
type listenerTrocavel struct {
	mu       sync.Mutex
	ln       net.Listener
	trocando bool // verdadeiro enquanto o endereco esta sendo trocado
}

func (l *listenerTrocavel) Accept() (net.Conn, error) {
	for {
		l.mu.Lock()
		atual := l.ln
		trocando := l.trocando
		l.mu.Unlock()
		if atual == nil {
			// endereco em troca: espera o novo entrar no lugar em vez de derrubar
			// o servidor (era isso que fazia o programa sair sozinho)
			if trocando {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			return nil, net.ErrClosed
		}
		c, err := atual.Accept()
		if err != nil {
			l.mu.Lock()
			mudou := l.ln != atual || l.trocando
			l.mu.Unlock()
			if mudou {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			return nil, err
		}
		return c, nil
	}
}

func (l *listenerTrocavel) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ln == nil {
		return nil
	}
	err := l.ln.Close()
	l.ln = nil
	return err
}

func (l *listenerTrocavel) Addr() net.Addr {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ln == nil {
		return &net.TCPAddr{}
	}
	return l.ln.Addr()
}

// trocarEndereco passa a atender em outro endereco (ou na mesma porta, em outra
// interface) sem reiniciar o programa. Se o endereco novo nao puder ser aberto,
// volta para o antigo e devolve o erro — assim a loja nunca fica sem sistema.
func (a *App) trocarEndereco(novoHost string) error {
	if novoHost == "" || novoHost == a.hostEscuta {
		return nil
	}
	porta := a.porta
	if porta == 0 {
		porta = 8787
	}
	endereco := fmt.Sprintf("%s:%d", novoHost, porta)
	// 1) as vezes o endereco novo nao conflita com o atual e abre na hora
	if novo, err := net.Listen("tcp", endereco); err == nil {
		a.trocarListener(novo, novoHost)
		return nil
	}
	// 2) o caso comum: solta o endereco atual e abre o novo. O sistema
	// operacional leva alguns instantes para liberar a porta (conexoes em uso),
	// por isso tenta de novo por ate 5 segundos antes de desistir.
	a.soltarListener()
	var ultimo error
	fim := time.Now().Add(5 * time.Second)
	for time.Now().Before(fim) {
		if novo, err := net.Listen("tcp", endereco); err == nil {
			a.trocarListener(novo, novoHost)
			return nil
		} else {
			ultimo = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	a.voltarAoEnderecoAntigo(porta)
	return fmt.Errorf("endereço %s não pôde ser aberto (%v)", endereco, ultimo)
}

// soltarListener fecha o endereco atual sem derrubar o programa. O Accept()
// fica esperando (marcado como "trocando") ate o endereco novo entrar no lugar.
func (a *App) soltarListener() {
	if a.ln == nil {
		return
	}
	a.ln.mu.Lock()
	a.ln.trocando = true
	atual := a.ln.ln
	a.ln.ln = nil
	a.ln.mu.Unlock()
	if atual != nil {
		_ = atual.Close()
	}
}

// trocarListener coloca o listener novo no lugar do antigo.
func (a *App) trocarListener(novo net.Listener, novoHost string) {
	a.ln.mu.Lock()
	antigo := a.ln.ln
	a.ln.ln = novo
	a.ln.trocando = false
	a.ln.mu.Unlock()
	if antigo != nil && antigo != novo {
		_ = antigo.Close()
	}
	a.hostEscuta = novoHost
	fmt.Printf(" Endereco de escuta agora: %s:%d\n", novoHost, a.porta)
}

// voltarAoEnderecoAntigo reabre o endereco que estava em uso (rollback).
func (a *App) voltarAoEnderecoAntigo(porta int) {
	antigo := a.hostEscuta
	for i := 0; i < 40; i++ {
		if l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", antigo, porta)); err == nil {
			a.trocarListener(l, antigo)
			return
		}
		if i == 20 {
			// metade do caminho: tenta reabrir pelo menos neste computador
			if l2, err2 := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", porta)); err2 == nil {
				a.trocarListener(l2, "127.0.0.1")
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	a.ln.mu.Lock()
	a.ln.trocando = false
	a.ln.mu.Unlock()
	avisoErro("O sistema parou de atender e não consegui reabrir o endereço. " +
		"Feche e abra o AutoPeças pelo atalho da área de trabalho.")
}

// hostDesejado decide onde o programa deve escutar de acordo com a configuracao
// salva (acesso pelo celular ligado = 0.0.0.0).
func (a *App) hostDesejado() string {
	if a.db != nil && a.db.GetSetting("acesso_rede") == "1" && a.exigeSenha() {
		return "0.0.0.0"
	}
	// sem acesso pelo celular: so este computador. Excecao: o dono pediu um
	// endereco proprio na linha de comando (-host 192.168.0.12).
	if a.hostExplicito != "" && !ehEnderecoLocal(a.hostExplicito) {
		return a.hostExplicito
	}
	return "127.0.0.1"
}

// pedirEncerramento fecha o AutoPecas que estiver aberto nesta maquina
// (usado pelo atalho "Encerrar AutoPecas" e pela opcao -encerrar).
func (a *App) pedirEncerramento() error {
	portas := []int{a.porta}
	if a.porta == 0 {
		portas = []int{8787}
	}
	cliente := &http.Client{Timeout: 3 * time.Second}
	var ultimo error
	for p := portas[0]; p < portas[0]+120; p++ {
		url := fmt.Sprintf("http://127.0.0.1:%d/api/encerrar", p)
		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-AutoPecas", "1")
		req.Header.Set("Origin", fmt.Sprintf("http://127.0.0.1:%d", p))
		resp, err := cliente.Do(req)
		if err != nil {
			ultimo = err
			continue
		}
		codigo := resp.StatusCode
		resp.Body.Close()
		if codigo == 200 {
			return nil
		}
		ultimo = fmt.Errorf("resposta %d", codigo)
	}
	if ultimo == nil {
		ultimo = fmt.Errorf("nenhum servidor respondeu")
	}
	return ultimo
}

// ---------- utilitarios HTTP ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"erro": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	// item 7: so aceita corpo JSON. Um site externo nao consegue mandar
	// Content-Type application/json sem passar por pre-verificacao (CORS),
	// o que dificulta chamadas forjadas com formulario simples.
	tipo := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if i := strings.Index(tipo, ";"); i >= 0 {
		tipo = strings.TrimSpace(tipo[:i])
	}
	if tipo != "" && tipo != "application/json" {
		return fmt.Errorf("envie os dados como JSON (Content-Type: application/json)")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 32<<20))
	return dec.Decode(v)
}

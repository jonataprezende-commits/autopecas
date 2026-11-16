package main

// Acesso pelo celular / pela rede (versao 1.0.7).
//
// Antes, quem abria o sistema em rede (-host 0.0.0.0 -senha-rede X) precisava
// digitar a senha naquela janelinha do navegador (HTTP Basic). No celular isso
// e ruim: a janelinha nao aparece, some sozinha e nao tem como sair do sistema.
//
// Agora existe login de verdade:
//   - POST /api/entrar   {senha}  -> grava um cookie de sessao (12 horas);
//   - POST /api/sair     apaga a sessao naquele aparelho;
//   - GET  /api/sessao   diz se ja esta logado e se o sistema pede senha;
//   - GET  /api/rede     mostra os enderecos para abrir no celular;
//   - POST /api/rede/aplicar  liga/desliga o acesso pelo celular.
//
// O HTTP Basic continua aceito (para scripts e para quem ja usa assim), mas o
// navegador passa a usar o cookie. A senha pode ser definida:
//   - pela opcao -senha-rede SUASENHA (linha de comando), ou
//   - pela tela "Acessar pelo celular", que grava a senha no banco (SHA-256).
// A senha nunca volta pela API: so o booleano "senha_definida".

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	nomeCookieSessao = "ap_sessao"
	duracaoSessao    = 12 * time.Hour
	maxSessoes       = 60
	// tentativas de senha: 8 erros em 5 minutos bloqueiam o aparelho
	maxFalhasLogin   = 8
	janelaFalhas     = 5 * time.Minute
	bloqueioLogin    = 5 * time.Minute
	tamanhoMinSenha  = 4
	chaveSessaoTexto = "X-AutoPecas-Sessao"
)

// ---------------- sessoes ----------------

type gerenciadorSessoes struct {
	mu      sync.Mutex
	tokens  map[string]time.Time
	duracao time.Duration
}

func novoGerenciadorSessoes() *gerenciadorSessoes {
	return &gerenciadorSessoes{tokens: map[string]time.Time{}, duracao: duracaoSessao}
}

func (g *gerenciadorSessoes) criar() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	agora := time.Now()
	for t, exp := range g.tokens {
		if agora.After(exp) {
			delete(g.tokens, t)
		}
	}
	for len(g.tokens) >= maxSessoes { // derruba a mais antiga
		var velha string
		var quando time.Time
		for t, exp := range g.tokens {
			if velha == "" || exp.Before(quando) {
				velha, quando = t, exp
			}
		}
		delete(g.tokens, velha)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil { // nunca acontece, mas nao deixa passar
		b = []byte(fmt.Sprintf("%d-%d", agora.UnixNano(), len(g.tokens)))
	}
	tk := base64.RawURLEncoding.EncodeToString(b)
	g.tokens[tk] = agora.Add(g.duracao)
	return tk
}

// valido confere o token e renova o prazo (quem esta usando nao cai).
func (g *gerenciadorSessoes) valido(tk string) bool {
	if tk == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	exp, ok := g.tokens[tk]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(g.tokens, tk)
		return false
	}
	g.tokens[tk] = time.Now().Add(g.duracao)
	return true
}

func (g *gerenciadorSessoes) remover(tk string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.tokens, tk)
}

// limparTodas derruba todas as sessoes (troca de senha). Sem isso, um celular
// que ja estava logado continuava entrando com a senha antiga.
func (g *gerenciadorSessoes) limparTodas() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := len(g.tokens)
	g.tokens = map[string]time.Time{}
	return n
}

func (g *gerenciadorSessoes) quantidade() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.tokens)
}

// ---------------- senha ----------------

func hashSenha(salt, senha string) string {
	h := sha256.Sum256([]byte(salt + "\x00" + senha))
	return hex.EncodeToString(h[:])
}

func novoSalt() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "autopecas"
	}
	return hex.EncodeToString(b)
}

// exigeSenha diz se o sistema esta pedindo senha.
func (a *App) exigeSenha() bool {
	return a.senhaRede != "" || a.senhaHash != ""
}

// confereSenha compara em tempo constante com o que estiver configurado.
func (a *App) confereSenha(senha string) bool {
	if senha == "" {
		return false
	}
	if a.senhaRede != "" {
		if subtle.ConstantTimeCompare([]byte(senha), []byte(a.senhaRede)) == 1 {
			return true
		}
	}
	if a.senhaHash != "" {
		if subtle.ConstantTimeCompare([]byte(hashSenha(a.senhaSalt, senha)), []byte(a.senhaHash)) == 1 {
			return true
		}
	}
	return false
}

func (a *App) sessoesProntas() *gerenciadorSessoes {
	if a.sessoes == nil {
		a.sessoes = novoGerenciadorSessoes()
	}
	return a.sessoes
}

// tokenDaRequisicao pega a sessao do cookie ou do cabecalho.
func tokenDaRequisicao(r *http.Request) string {
	if tk := strings.TrimSpace(r.Header.Get(chaveSessaoTexto)); tk != "" {
		return tk
	}
	if c, err := r.Cookie(nomeCookieSessao); err == nil {
		return c.Value
	}
	return ""
}

// clienteLocal diz se a requisicao veio do proprio computador (127.0.0.1).
func (a *App) clienteLocal(r *http.Request) bool {
	ip := net.ParseIP(ipDoCliente(r))
	return ip != nil && ip.IsLoopback()
}

// autenticado diz se a requisicao pode usar a API.
func (a *App) autenticado(r *http.Request) bool {
	if !a.exigeSenha() {
		return true // modo local: sem senha, como sempre foi
	}
	// Quem esta no proprio computador nao precisa digitar senha: a protecao e
	// para o celular e para qualquer outra maquina da rede. Atras de um tunel
	// isso NAO vale, porque o tunel conversa com o programa como 127.0.0.1.
	if !a.proxyConfiavel && a.clienteLocal(r) {
		return true
	}
	if a.sessoesProntas().valido(tokenDaRequisicao(r)) {
		return true
	}
	return a.senhaBasicaConfere(r)
}

// ---------------- limite de tentativas ----------------

type falhasLogin struct {
	quando []time.Time
	ate    time.Time
}

// podeTentar devolve quanto falta esperar (0 = pode tentar).
func (a *App) podeTentar(ip string) time.Duration {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if a.falhas == nil {
		a.falhas = map[string]*falhasLogin{}
	}
	f := a.falhas[ip]
	if f == nil {
		return 0
	}
	agora := time.Now()
	if !f.ate.IsZero() && agora.Before(f.ate) {
		return f.ate.Sub(agora)
	}
	return 0
}

func (a *App) registrarFalha(ip string) {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if a.falhas == nil {
		a.falhas = map[string]*falhasLogin{}
	}
	agora := time.Now()
	f := a.falhas[ip]
	if f == nil {
		f = &falhasLogin{}
		a.falhas[ip] = f
	}
	// guarda so as tentativas dentro da janela
	vivas := f.quando[:0]
	for _, t := range f.quando {
		if agora.Sub(t) < janelaFalhas {
			vivas = append(vivas, t)
		}
	}
	f.quando = append(vivas, agora)
	if len(f.quando) >= maxFalhasLogin {
		f.ate = agora.Add(bloqueioLogin)
		f.quando = nil
	}
	// nao deixa o mapa crescer sem limite
	if len(a.falhas) > 500 {
		for k, v := range a.falhas {
			if v.ate.IsZero() && len(v.quando) == 0 {
				delete(a.falhas, k)
			}
		}
	}
}

// limparFalhasTodas zera o contador de senhas erradas (troca de senha): quem
// estava bloqueado por errar a senha antiga nao fica preso.
// apagarSessoesDeOutrosAparelhos: quando o acesso pelo celular e' ligado ou
// desligado, quem estava conectado pelo celular/computador da rede sai e precisa
// entrar de novo. Este computador continua funcionando (o acesso local nao pede
// senha).
func (a *App) apagarSessoesDeOutrosAparelhos() {
	a.sessoesProntas().limparTodas()
	a.limparFalhasTodas()
}

func (a *App) limparFalhasTodas() {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	a.falhas = map[string]*falhasLogin{}
}

func (a *App) limparFalhas(ip string) {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	delete(a.falhas, ip)
}

func ipDoCliente(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

// ---------------- utilidades de resposta ----------------

func (a *App) modoAcesso() string {
	if ehEnderecoLocal(a.hostEscuta) {
		return "local"
	}
	return "rede"
}

// conexaoSegura diz se o navegador chegou por HTTPS (direto ou atras de proxy).
func (a *App) conexaoSegura(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if a.proxyConfiavel {
		return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
	}
	return false
}

func (a *App) gravarCookieSessao(w http.ResponseWriter, r *http.Request, tk string) {
	http.SetCookie(w, &http.Cookie{
		Name:     nomeCookieSessao,
		Value:    tk,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   a.conexaoSegura(r),
		MaxAge:   int(duracaoSessao / time.Second),
	})
}

func (a *App) apagarCookieSessao(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     nomeCookieSessao,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// ---------------- rotas ----------------

// handleSessao: GET /api/sessao (rota livre, usada pela tela de login)
func (a *App) handleSessao(w http.ResponseWriter, r *http.Request) {
	tk := tokenDaRequisicao(r)
	logado := a.autenticado(r)
	writeJSON(w, 200, map[string]any{
		"autenticado":   logado,
		"precisa_senha": a.exigeSenha(),
		"local":         !a.proxyConfiavel && a.clienteLocal(r),
		"modo":          a.modoAcesso(),
		"usuario":       "autopecas",
		"tem_sessao":    tk != "",
	})
}

// handleEntrar: POST /api/entrar { senha }
func (a *App) handleEntrar(w http.ResponseWriter, r *http.Request) {
	ip := ipDoCliente(r)
	if espera := a.podeTentar(ip); espera > 0 {
		min := int(espera.Minutes()) + 1
		fail(w, 429, fmt.Sprintf("senha errada demais. Tente de novo em %d minuto(s).", min))
		return
	}
	var in struct {
		Senha string `json:"senha"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	if !a.exigeSenha() {
		// sistema local, sem senha: entra direto
		writeJSON(w, 200, map[string]any{"ok": true, "modo": a.modoAcesso(), "senha_necessaria": false,
			"local": !a.proxyConfiavel && a.clienteLocal(r)})
		return
	}
	if !a.confereSenha(strings.TrimSpace(in.Senha)) {
		a.registrarFalha(ip)
		fail(w, 401, "senha incorreta")
		return
	}
	a.limparFalhas(ip)
	tk := a.sessoesProntas().criar()
	a.gravarCookieSessao(w, r, tk)
	writeJSON(w, 200, map[string]any{"ok": true, "modo": a.modoAcesso(), "senha_necessaria": true,
		"local": !a.proxyConfiavel && a.clienteLocal(r)})
}

// handleSair: POST /api/sair
func (a *App) handleSair(w http.ResponseWriter, r *http.Request) {
	if tk := tokenDaRequisicao(r); tk != "" {
		a.sessoesProntas().remover(tk)
	}
	a.apagarCookieSessao(w)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ipsDaLoja lista os IPv4 da maquina na rede local (para montar o endereco).
func ipsDaLoja() []string {
	var saida []string
	interfaces, err := net.Interfaces()
	if err != nil {
		return saida
	}
	for _, it := range interfaces {
		if it.Flags&net.FlagUp == 0 || it.Flags&net.FlagLoopback != 0 {
			continue
		}
		nome := strings.ToLower(it.Name)
		// placas virtuais do Windows (VirtualBox, VMware, Hyper-V, WSL) nao
		// interessam: o celular nao chega por elas.
		ignorar := strings.Contains(nome, "virtual") || strings.Contains(nome, "vmware") ||
			strings.Contains(nome, "vbox") || strings.Contains(nome, "vethernet") ||
			strings.Contains(nome, "hyper-v") || strings.Contains(nome, "wsl") ||
			strings.Contains(nome, "docker") || strings.Contains(nome, "loopback")
		addrs, err := it.Addrs()
		if err != nil {
			continue
		}
		for _, ad := range addrs {
			ipnet, ok := ad.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			if ignorar {
				continue
			}
			saida = append(saida, ip.String())
		}
	}
	sort.Strings(saida)
	return saida
}

// urlsRede monta os enderecos que o celular pode usar (um por IP da loja).
func (a *App) urlsRede() []map[string]string {
	porta := a.porta
	if porta == 0 {
		porta = 8787
	}
	ips := ipsDaLoja()
	urls := make([]map[string]string, 0, len(ips)+1)
	for _, ip := range ips {
		urls = append(urls, map[string]string{
			"rotulo": "Celular na mesma Wi-Fi",
			"url":    fmt.Sprintf("http://%s:%d", ip, porta),
		})
	}
	return urls
}

// handleRede: GET /api/rede
func (a *App) handleRede(w http.ResponseWriter, r *http.Request) {
	porta := a.porta
	if porta == 0 {
		porta = 8787
	}
	ips := ipsDaLoja()
	urls := a.urlsRede()
	extras := append([]string{}, a.hostsExtras...)
	writeJSON(w, 200, map[string]any{
		"modo":            a.modoAcesso(),
		"porta":           porta,
		"ips":             ips,
		"urls":            urls,
		"url_local":       fmt.Sprintf("http://127.0.0.1:%d", porta),
		"senha_definida":  a.exigeSenha(),
		"senha_por_flag":  a.senhaRede != "",
		"hosts_extras":    extras,
		"atras_de_proxy":  a.proxyConfiavel,
		"sessoes_abertas": a.sessoesProntas().quantidade(),
	})
}

// handleRedeAplicar: POST /api/rede/aplicar { ativo, senha, reiniciar }
//
// Liga ou desliga o acesso pelo celular. Quando liga, exige uma senha (nova ou
// a que ja existe). Devolve o que fazer: reiniciar o programa ou nao.
func (a *App) handleRedeAplicar(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ativo     *bool  `json:"ativo"`
		Senha     string `json:"senha"`
		Reiniciar bool   `json:"reiniciar"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados invalidos")
		return
	}
	if in.Ativo == nil {
		fail(w, 400, "informe se o acesso pelo celular fica ligado ou desligado")
		return
	}
	ligado := *in.Ativo

	if in.Senha != "" {
		if len([]rune(strings.TrimSpace(in.Senha))) < tamanhoMinSenha {
			fail(w, 400, fmt.Sprintf("a senha precisa de pelo menos %d caracteres", tamanhoMinSenha))
			return
		}
		if a.senhaRede != "" {
			fail(w, 400, "a senha está sendo definida pela opção -senha-rede; troque por lá ou abra sem essa opção")
			return
		}
		salt := novoSalt()
		a.db.SetSetting("senha_rede_salt", salt)
		a.db.SetSetting("senha_rede_hash", hashSenha(salt, strings.TrimSpace(in.Senha)))
		a.senhaSalt = salt
		a.senhaHash = hashSenha(salt, strings.TrimSpace(in.Senha))
		// 1.0.10: senha nova derruba as sessoes antigas (o celular que estava
		// logado com a senha velha cai). A sessao deste computador e' recriada
		// logo abaixo, entao ninguem perde o que esta fazendo.
		a.sessoesProntas().limparTodas()
		a.limparFalhasTodas()
	}
	if ligado && !a.exigeSenha() {
		fail(w, 400, "para abrir o sistema na rede é obrigatório definir uma senha")
		return
	}

	antes := a.db.GetSetting("acesso_rede") == "1"
	if ligado {
		a.db.SetSetting("acesso_rede", "1")
	} else {
		a.db.SetSetting("acesso_rede", "0")
	}
	if antes != ligado {
		a.apagarSessoesDeOutrosAparelhos()
	}

	// quem acabou de definir a senha continua entrando: cria a sessao deste
	// aparelho na hora, senao a tela que estava aberta passaria a dar 401.
	if a.exigeSenha() {
		if tk := tokenDaRequisicao(r); tk == "" || !a.sessoesProntas().valido(tk) {
			a.gravarCookieSessao(w, r, a.sessoesProntas().criar())
		}
	}

	// 1.0.11: o endereco de escuta muda na hora, DENTRO deste mesmo programa.
	// Antes era preciso fechar e abrir outro processo ("reiniciar"), e era aí
	// que morava o defeito: o processo novo as vezes nao conseguia a porta e
	// morria calado, deixando a loja sem sistema e sem aviso.
	trocou := false
	var erroTroca string
	if modoAtualDifere(a) {
		if err := a.trocarEndereco(a.hostDesejado()); err != nil {
			erroTroca = err.Error()
		} else {
			trocou = true
		}
	}

	resp := map[string]any{
		"ok":                true,
		"ativo":             ligado,
		"precisa_reiniciar": false,
		"senha_definida":    a.exigeSenha(),
		"endereco":          a.hostEscuta,
	}
	if trocou {
		resp["trocou_endereco"] = true
		resp["enderecos"] = a.urlsRede()
	}
	if erroTroca != "" {
		resp["aviso"] = "A senha foi salva, mas o programa não conseguiu passar a atender a rede: " + erroTroca
		avisoErro("Não consegui abrir o sistema na rede: " + erroTroca)
	}
	writeJSON(w, 200, resp)
}

// modoAtualDifere: a configuracao salva pede rede mas o programa esta local?
func modoAtualDifere(a *App) bool {
	querRede := a.db.GetSetting("acesso_rede") == "1"
	estaRede := !ehEnderecoLocal(a.hostEscuta)
	return querRede != estaRede
}

package main

// Testes do acesso pelo celular (versao 1.0.7): login por sessao, limite de
// tentativas, senha gravada pela tela, enderecos de rede e hostnames extras.
//
// Estes testes usam o handler direto (novaAppSemServidor) porque precisam
// controlar o IP de quem chama: no modo de rede, quem vem do proprio computador
// (127.0.0.1) entra sem senha - e um servidor httptest sempre aparece como
// 127.0.0.1. Com o handler direto conseguimos fingir um celular na rede.

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// entrarFazLogin faz o POST /api/entrar e devolve o cookie de sessao ("" se nao veio).
func entrarFazLogin(env *ambiente, senha string) (int, string) {
	env.t.Helper()
	r := env.post("/api/entrar", map[string]any{"senha": senha})
	cookie := ""
	for _, c := range r.headers.Values("Set-Cookie") {
		if strings.HasPrefix(c, nomeCookieSessao+"=") {
			parte := strings.SplitN(c, ";", 2)[0]
			cookie = strings.TrimPrefix(parte, nomeCookieSessao+"=")
		}
	}
	return r.status, cookie
}

// deFora simula um celular na rede (a requisicao deixa de vir do proprio PC,
// que por seguranca entra sem senha).
func deFora(req *http.Request) {
	req.RemoteAddr = "192.168.0.55:51234"
	if req.Header.Get("Origin") == "" {
		req.Header.Set("Origin", "http://"+req.Host)
	}
}

// doProprioPC simula o navegador do computador da loja (127.0.0.1).
// No handler direto o RemoteAddr vem vazio, e vazio nao conta como local.
func doProprioPC(req *http.Request) { req.RemoteAddr = "127.0.0.1:49876" }

// comCookie devolve um "mexer" que manda o cookie de sessao na requisicao.
// ambos junta dois "mexer" na mesma requisicao.
func ambos(a, b func(*http.Request)) func(*http.Request) {
	return func(req *http.Request) {
		a(req)
		b(req)
	}
}

func comCookie(valor string) func(*http.Request) {
	return func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: nomeCookieSessao, Value: valor})
	}
}

func TestAcessoLocalNaoPedeSenha(t *testing.T) {
	env := novaAppSemServidor(t)
	r := env.get("/api/sessao")
	if r.status != 200 {
		t.Fatalf("/api/sessao deveria responder 200, veio %d (%s)", r.status, r.corpo)
	}
	m := r.json(t)
	if m["autenticado"] != true {
		t.Errorf("modo local deveria estar autenticado")
	}
	if m["precisa_senha"] != false {
		t.Errorf("modo local nao deveria pedir senha")
	}
	if m["modo"] != "local" {
		t.Errorf("modo deveria ser local, veio %v", m["modo"])
	}
	// a API continua aberta, como sempre foi
	if r := env.get("/api/pecas"); r.status != 200 {
		t.Errorf("/api/pecas no modo local deveria dar 200, veio %d", r.status)
	}
}

func TestModoRedeSemSessaoDa401(t *testing.T) {
	env := novaAppSemServidor(t)
	env.app.hostEscuta = "0.0.0.0"
	env.app.senhaRede = "segredo123"

	if r := env.reqCom("GET", "/api/pecas", nil, deFora); r.status != 401 {
		t.Fatalf("sem senha a API deveria dar 401 para quem vem da rede, veio %d (%s)", r.status, r.corpo)
	}
	// nao manda mais WWW-Authenticate (era o que abria a janelinha do navegador)
	if h := env.reqCom("GET", "/api/pecas", nil, deFora).headers.Get("WWW-Authenticate"); h != "" {
		t.Errorf("nao deveria mandar WWW-Authenticate, veio %q", h)
	}
	// a tela de login precisa conseguir perguntar
	if r := env.get("/api/sessao"); r.status != 200 || r.json(t)["precisa_senha"] != true {
		t.Errorf("/api/sessao deveria dizer que precisa de senha: %d %s", r.status, r.corpo)
	}
	if r := env.reqCom("POST", "/api/entrar", map[string]any{"senha": "errada"}, deFora); r.status != 401 {
		t.Errorf("senha errada deveria dar 401, veio %d", r.status)
	}
}

func TestLoginAbreSessaoEDeixaUsarOCelular(t *testing.T) {
	env := novaAppSemServidor(t)
	env.app.hostEscuta = "0.0.0.0"
	env.app.senhaRede = "segredo123"

	status, cookie := entrarFazLogin(env, "segredo123")
	if status != 200 || cookie == "" {
		t.Fatalf("login deveria dar 200 e cookie, veio %d cookie=%q", status, cookie)
	}
	// com o cookie a API responde
	r := env.reqCom("GET", "/api/pecas", nil, ambos(deFora, comCookie(cookie)))
	if r.status != 200 {
		t.Fatalf("com cookie de sessao deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	// e o /api/sessao confirma
	if m := env.reqCom("GET", "/api/sessao", nil, ambos(deFora, comCookie(cookie))).json(t); m["autenticado"] != true {
		t.Errorf("sessao valida deveria aparecer como autenticado")
	}
	// gravar dados tambem funciona (POST com o cabecalho anti-CSRF)
	if r := env.reqCom("POST", "/api/pecas", map[string]any{"codigo": "CEL-1", "descricao": "Peca do celular"}, ambos(deFora, comCookie(cookie))); r.status != 200 {
		t.Errorf("POST com sessao deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	// sair derruba a sessao
	if r := env.reqCom("POST", "/api/sair", nil, ambos(deFora, comCookie(cookie))); r.status != 200 {
		t.Fatalf("sair deveria dar 200, veio %d", r.status)
	}
	if r := env.reqCom("GET", "/api/pecas", nil, ambos(deFora, comCookie(cookie))); r.status != 401 {
		t.Errorf("depois de sair a sessao nao pode mais valer, veio %d", r.status)
	}
}

func TestSenhaBasicaContinuaFuncionando(t *testing.T) {
	env := novaAppSemServidor(t)
	env.app.hostEscuta = "0.0.0.0"
	env.app.senhaRede = "segredo123"

	r := env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) {
		deFora(req)
		req.SetBasicAuth("autopecas", "segredo123")
	})
	if r.status != 200 {
		t.Errorf("HTTP Basic com a senha certa deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	r = env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) {
		deFora(req)
		req.SetBasicAuth("autopecas", "errada")
	})
	if r.status != 401 {
		t.Errorf("HTTP Basic com senha errada deveria dar 401, veio %d", r.status)
	}
}

func TestLoginBloqueiaDepoisDeMuitasTentativas(t *testing.T) {
	env := novaAppSemServidor(t)
	env.app.hostEscuta = "0.0.0.0"
	env.app.senhaRede = "segredo123"

	for i := 0; i < maxFalhasLogin; i++ {
		if r := env.post("/api/entrar", map[string]any{"senha": "chute"}); r.status != 401 {
			t.Fatalf("tentativa %d deveria dar 401, veio %d", i+1, r.status)
		}
	}
	r := env.post("/api/entrar", map[string]any{"senha": "segredo123"})
	if r.status != 429 {
		t.Fatalf("depois de %d erros deveria bloquear com 429, veio %d (%s)", maxFalhasLogin, r.status, r.corpo)
	}
	if !strings.Contains(strings.ToLower(string(r.corpo)), "minuto") {
		t.Errorf("a mensagem deveria dizer quanto tempo esperar: %s", r.corpo)
	}
	// a senha certa tambem fica bloqueada durante a espera (senao o limite nao serve)
	if _, cookie := entrarFazLogin(env, "segredo123"); cookie != "" {
		t.Errorf("nao deveria liberar sessao durante o bloqueio")
	}
}

func TestSenhaGravadaPelaTelaFunciona(t *testing.T) {
	env := novaAppSemServidor(t)
	env.app.hostEscuta = "0.0.0.0" // ja esta na rede...

	// ...mas ainda sem senha: aplicar sem senha precisa ser recusado
	r := env.post("/api/rede/aplicar", map[string]any{"ativo": true})
	if r.status != 400 {
		t.Fatalf("ligar o acesso pelo celular sem senha deveria dar 400, veio %d (%s)", r.status, r.corpo)
	}
	// senha curta tambem
	r = env.post("/api/rede/aplicar", map[string]any{"ativo": true, "senha": "12"})
	if r.status != 400 {
		t.Errorf("senha curta deveria ser recusada, veio %d", r.status)
	}
	// senha boa: grava e passa a valer na hora
	r = env.post("/api/rede/aplicar", map[string]any{"ativo": true, "senha": "loja2026"})
	if r.status != 200 {
		t.Fatalf("definir a senha deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	if env.app.senhaHash == "" || env.app.senhaSalt == "" {
		t.Fatalf("a senha deveria ter sido gravada com sal")
	}
	if env.qstr(`SELECT value FROM settings WHERE key='senha_rede_hash'`) == "" {
		t.Errorf("a senha deveria estar no banco")
	}
	// a senha nao pode voltar pela API (quem definiu continua logado)
	_, cookieAplicar := entrarFazLogin(env, "loja2026")
	cfg := string(env.reqCom("GET", "/api/config", nil, comCookie(cookieAplicar)).corpo)
	if strings.Contains(cfg, "loja2026") {
		t.Errorf("/api/config nao pode devolver a senha: %s", cfg)
	}
	if !strings.Contains(cfg, `"senha_definida":"true"`) {
		t.Errorf("/api/config deveria informar que existe senha: %s", cfg)
	}
	// e o login usa a senha gravada
	status, cookie := entrarFazLogin(env, "loja2026")
	if status != 200 || cookie == "" {
		t.Fatalf("login com a senha gravada deveria funcionar, veio %d", status)
	}
	if r := env.reqCom("GET", "/api/pecas", nil, comCookie(cookie)); r.status != 200 {
		t.Errorf("sessao criada com a senha gravada deveria abrir a API, veio %d", r.status)
	}
}

func TestModoLocalNaoDeixaUsuarioTrancado(t *testing.T) {
	// Programa no modo local (sem senha) nunca pode pedir senha, senao o dono
	// da loja se tranca para fora do proprio sistema.
	env := novaAppSemServidor(t)
	if env.app.exigeSenha() {
		t.Fatalf("modo local nao deveria exigir senha")
	}
	if r := env.post("/api/entrar", map[string]any{"senha": ""}); r.status != 200 {
		t.Errorf("entrar no modo local deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	if r := env.post("/api/entrar", map[string]any{"senha": "qualquer"}); r.status != 200 {
		t.Errorf("modo local nao deve validar senha, veio %d", r.status)
	}
	if m := env.get("/api/sessao").json(t); m["autenticado"] != true {
		t.Errorf("modo local deveria estar sempre autenticado")
	}
}

func TestRotaRedeMostraEnderecosParaOCelular(t *testing.T) {
	env := novaAppSemServidor(t)
	env.app.senhaRede = "segredo123"
	_, cookie := entrarFazLogin(env, "segredo123")
	r := env.reqCom("GET", "/api/rede", nil, comCookie(cookie))
	if r.status != 200 {
		t.Fatalf("/api/rede deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	m := r.json(t)
	if m["porta"] == nil || m["senha_definida"] != true {
		t.Errorf("resposta incompleta: %s", r.corpo)
	}
	if _, ok := m["urls"].([]any); !ok {
		t.Errorf("deveria trazer a lista de enderecos: %s", r.corpo)
	}
	if !strings.Contains(string(r.corpo), "127.0.0.1") {
		t.Errorf("deveria trazer tambem o endereco local: %s", r.corpo)
	}
	// /api/rede nao existe para quem nao esta logado
	env.app.hostEscuta = "0.0.0.0"
	if r := env.get("/api/rede"); r.status != 401 { // sem cookie nenhum
		t.Errorf("/api/rede sem sessao na rede deveria dar 401, veio %d", r.status)
	}
}

func TestHostExtraLiberadoParaTunelEVPN(t *testing.T) {
	env := novaAppSemServidor(t)
	env.app.hostEscuta = "0.0.0.0"
	env.app.senhaRede = "segredo123"

	pedido := func() resposta {
		return env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) {
			req.Host = "autopecas.minhaloja.com"
			req.Header.Set("Origin", "https://autopecas.minhaloja.com")
			req.SetBasicAuth("autopecas", "segredo123")
		})
	}
	// sem liberar o nome, bloqueia
	if r := pedido(); r.status != 403 {
		t.Errorf("nome nao liberado deveria dar 403, veio %d (%s)", r.status, r.corpo)
	}
	// liberando o nome e avisando que esta atras de tunel, funciona
	env.app.hostsExtras = []string{"autopecas.minhaloja.com"}
	env.app.proxyConfiavel = true
	if r := pedido(); r.status != 200 {
		t.Errorf("nome liberado com -proxy deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	// mesmo liberado, o nome tem que bater
	r := env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) {
		req.Host = "outro.nome.com"
		req.Header.Set("Origin", "https://outro.nome.com")
		req.SetBasicAuth("autopecas", "segredo123")
	})
	if r.status != 403 {
		t.Errorf("outro nome deveria continuar bloqueado, veio %d", r.status)
	}
}

func TestSenhaErradaNaoDerrubaSessaoBoa(t *testing.T) {
	// Um celular logado continua funcionando mesmo que alguem erre a senha
	// varias vezes em outro aparelho.
	env := novaAppSemServidor(t)
	env.app.hostEscuta = "0.0.0.0"
	env.app.senhaRede = "segredo123"

	_, cookie := entrarFazLogin(env, "segredo123")
	if cookie == "" {
		t.Fatalf("login inicial falhou")
	}
	env.app.falhas = map[string]*falhasLogin{
		"10.0.0.99": {quando: []time.Time{time.Now()}, ate: time.Now().Add(bloqueioLogin)},
	}
	if r := env.reqCom("GET", "/api/pecas", nil, comCookie(cookie)); r.status != 200 {
		t.Errorf("sessao valida nao pode ser afetada por erros de outro aparelho, veio %d", r.status)
	}
}

func TestSessoesTemPrazoETeto(t *testing.T) {
	g := novoGerenciadorSessoes()
	tk := g.criar()
	if !g.valido(tk) {
		t.Fatalf("token recem criado deveria valer")
	}
	if g.valido("inventado") {
		t.Errorf("token inventado nao pode valer")
	}
	g.remover(tk)
	if g.valido(tk) {
		t.Errorf("token removido nao pode valer")
	}
	// sessao vencida nao vale
	g.tokens["velho"] = time.Now().Add(-time.Minute)
	if g.valido("velho") {
		t.Errorf("sessao vencida nao pode valer")
	}
	// nunca passa do teto
	for i := 0; i < maxSessoes+10; i++ {
		g.criar()
	}
	if n := g.quantidade(); n > maxSessoes {
		t.Errorf("passou do teto de sessoes: %d", n)
	}
}

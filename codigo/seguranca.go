package main

// Item 7 - seguranca do servidor local.
//
// O AutoPecas roda um servidor HTTP em 127.0.0.1 e a interface e uma pagina web.
// Sem estas travas, qualquer site aberto no mesmo navegador (ou um programa
// malicioso na maquina) consegue chamar a API em http://127.0.0.1:8787.
//
// Regras aplicadas a TODAS as rotas /api/:
//   a) Host precisa ser o endereco local real (com a porta em uso);
//   b) POST/PUT/DELETE/PATCH exigem o cabecalho X-AutoPecas: 1;
//   c) se vier Origin, precisa ser o proprio sistema; Sec-Fetch-Site so
//      aceita same-origin/none;
//   d) quando o programa e aberto na rede (-host 0.0.0.0) e obrigatorio
//      usar -senha-rede, aplicada como HTTP Basic em toda a API.

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// ehEnderecoLocal indica se o endereco de escuta atende somente esta maquina.
func ehEnderecoLocal(host string) bool {
	h := strings.Trim(strings.TrimSpace(strings.ToLower(host)), "[]")
	switch h {
	case "", "127.0.0.1", "localhost", "::1":
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// validarInicial recusa abrir o sistema na rede sem senha (item 7e).
func (a *App) validarInicial() error {
	if !a.exigeSenha() && !ehEnderecoLocal(a.hostEscuta) {
		return fmt.Errorf("por segurança o AutoPeças não abre em rede (%s) sem senha. "+
			"Abra o AutoPeças e use Ajustes → Acessar pelo celular para definir a senha, "+
			"ou inicie com: AutoPecas.exe -host %s -senha-rede SUASENHA", a.hostEscuta, a.hostEscuta)
	}
	return nil
}

// hostPermitido confere o cabecalho Host contra a porta realmente em uso.
func (a *App) hostPermitido(host string) bool {
	if host == "" {
		return false
	}
	h, porta, err := net.SplitHostPort(host)
	if err != nil {
		h, porta = host, ""
	}
	h = strings.Trim(strings.ToLower(h), "[]")
	// 1.0.7: enderecos extras liberados na linha de comando
	// (-hosts-permitidos) para quem usa tunel/VPN com nome proprio.
	for _, extra := range a.hostsExtras {
		if h == strings.Trim(strings.ToLower(extra), "[]") {
			return true
		}
	}
	// atras de um tunel o Host chega sem porta; nesse caso o nome TEM que estar
	// na lista liberada (checada acima) - nao liberamos nome desconhecido.
	if a.porta > 0 && porta != strconv.Itoa(a.porta) {
		return false
	}
	escuta := strings.Trim(strings.ToLower(a.hostEscuta), "[]")
	if h == escuta || h == "127.0.0.1" || h == "localhost" || h == "::1" {
		return true
	}
	// escutando em todas as interfaces: o nome usado pelo cliente varia
	// (IP da maquina na rede), mas a senha continua obrigatoria.
	if escuta == "0.0.0.0" || escuta == "::" || escuta == "" {
		return escuta != "" || a.porta == 0
	}
	return false
}

// originDoServidorProxy: com -proxy, o esquema e o nome vem do proprio tunel.
func originDoServidorProxy(r *http.Request, confiarNoProxy bool) string {
	esquema := "http"
	if r.TLS != nil {
		esquema = "https"
	}
	host := r.Host
	if confiarNoProxy {
		if p := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); p != "" {
			esquema = strings.ToLower(p)
		}
		if h := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); h != "" {
			host = h
		}
	}
	return esquema + "://" + host
}

// senhaBasicaConfere e o HTTP Basic antigo: continua aceito para scripts e
// para quem abre o sistema com -senha-rede e um programa que nao guarda cookie.
func (a *App) senhaBasicaConfere(r *http.Request) bool {
	if !a.exigeSenha() {
		return true
	}
	cab := r.Header.Get("Authorization")
	if !strings.HasPrefix(strings.ToLower(cab), "basic ") {
		return false
	}
	bruto, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cab[6:]))
	if err != nil {
		return false
	}
	partes := strings.SplitN(string(bruto), ":", 2)
	if len(partes) != 2 {
		return false
	}
	if partes[0] != "autopecas" && partes[0] != "" {
		return false
	}
	return a.confereSenha(partes[1])
}

func exigeCabecalho(m string) bool {
	switch m {
	case "POST", "PUT", "DELETE", "PATCH":
		return true
	}
	return false
}

// comSeguranca aplica as travas do item 7 nas rotas /api/.
func (a *App) comSeguranca(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if !a.hostPermitido(r.Host) {
			fail(w, 403, "endereço não autorizado: abra o sistema por http://127.0.0.1:"+strconv.Itoa(a.porta))
			return
		}
		if rotaLivre(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !a.autenticado(r) {
			// sem cookie valido e sem Basic: o navegador mostra a tela de senha
			// (o sistema nao manda mais WWW-Authenticate, que abria a janelinha
			// do navegador e atrapalhava no celular)
			fail(w, 401, "informe a senha do sistema")
			return
		}
		if exigeCabecalho(r.Method) && r.Header.Get("X-AutoPecas") != "1" {
			fail(w, 403, "requisição bloqueada: origem não identificada como o próprio sistema")
			return
		}
		if origem := r.Header.Get("Origin"); origem != "" {
			if !origemCombina(origem, originDoServidorProxy(r, a.proxyConfiavel), a.proxyConfiavel) {
				fail(w, 403, "requisição bloqueada: origem não permitida")
				return
			}
		}
		if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" {
			if sfs != "same-origin" && sfs != "none" {
				fail(w, 403, "requisição bloqueada: chamada de outro site")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// rotaLivre: rotas que respondem antes de exigir senha.
//   - /api/ping   a propria trava de instancia duplicada e o instalador usam;
//   - /api/sessao a tela de login pergunta se ja esta logado;
//   - /api/entrar a propria tentativa de entrar (tem limite de tentativas).
func rotaLivre(caminho string) bool {
	switch caminho {
	case "/api/ping", "/api/sessao", "/api/entrar":
		return true
	}
	return false
}

// origemCombina compara a origem do navegador com o endereco do servidor.
// Atras de tunel (proxyConfiavel) o esquema nao conta: o cliente ve https e o
// programa ve http, mas o nome (host) continua tendo que ser o mesmo - e e o
// nome que separa o sistema de um site qualquer tentando chamar a API.
func origemCombina(origem, esperado string, ignorarEsquema bool) bool {
	o := strings.TrimSuffix(strings.TrimSpace(origem), "/")
	e := strings.TrimSuffix(strings.TrimSpace(esperado), "/")
	if strings.EqualFold(o, e) {
		return true
	}
	if !ignorarEsquema {
		return false
	}
	cortar := func(s string) string {
		if i := strings.Index(s, "://"); i >= 0 {
			return strings.ToLower(s[i+3:])
		}
		return strings.ToLower(s)
	}
	return cortar(o) == cortar(e)
}

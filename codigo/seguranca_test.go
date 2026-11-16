package main

// Testes do item 7: seguranca do servidor local (Host, CSRF, Origin, senha de rede).

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestItem7HostDesconhecidoRetorna403(t *testing.T) {
	env := novaAppTeste(t)
	casos := []string{"evil.example.com", "127.0.0.1:1", "atacante.local:8787", "10.0.0.5:8787"}
	for _, host := range casos {
		r := env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) {
			req.Host = host
			req.Header.Set("Origin", "http://"+host)
		})
		if r.status != 403 {
			t.Errorf("Host %q deveria ser bloqueado com 403, veio %d (%s)", host, r.status, r.corpo)
		}
	}
	// hosts legitimos continuam funcionando
	for _, host := range []string{"127.0.0.1:8787", "localhost:8787"} {
		if env.app.porta == 0 {
			continue
		}
		host = strings.Replace(host, "8787", itoa(int64(env.app.porta)), 1)
		r := env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) {
			req.Host = host
			req.Header.Set("Origin", "http://"+host)
		})
		if r.status != 200 {
			t.Errorf("Host %q deveria funcionar, veio %d (%s)", host, r.status, r.corpo)
		}
	}
	// o ping usado para detectar "ja esta aberto" continua respondendo
	if r := env.get("/api/ping"); r.status != 200 {
		t.Errorf("/api/ping deveria responder 200, veio %d", r.status)
	}
	if !strings.Contains(string(env.get("/api/ping").corpo), appName) {
		t.Errorf("/api/ping deveria conter o nome do programa")
	}
}

func TestItem7ExigeCabecalhoXAutoPecas(t *testing.T) {
	env := novaAppTeste(t)
	for i, metodo := range []string{"POST", "PUT", "DELETE"} {
		corpo := map[string]any{"codigo": "SEG-" + itoa(int64(i)), "descricao": "Peca de seguranca"}
		r := env.reqCom(metodo, "/api/pecas", corpo, func(req *http.Request) {
			req.Header.Del("X-AutoPecas")
		})
		if r.status != 403 {
			t.Errorf("%s sem X-AutoPecas deveria dar 403, veio %d", metodo, r.status)
		}
	}
	// com o cabecalho (como o front faz) funciona
	if r := env.post("/api/pecas", map[string]any{"codigo": "SEG-OK", "descricao": "Peca de seguranca"}); r.status != 200 {
		t.Errorf("POST com X-AutoPecas deveria funcionar: %d %s", r.status, r.corpo)
	}
	// GET nao exige o cabecalho
	if r := env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) { req.Header.Del("X-AutoPecas") }); r.status != 200 {
		t.Errorf("GET nao deveria exigir X-AutoPecas: %d", r.status)
	}
}

func TestItem7OriginDeOutroSiteEhBloqueada(t *testing.T) {
	env := novaAppTeste(t)
	r := env.reqCom("POST", "/api/pecas", map[string]any{"codigo": "SEG-2", "descricao": "x"}, func(req *http.Request) {
		req.Header.Set("Origin", "http://site-malicioso.com")
	})
	if r.status != 403 {
		t.Errorf("Origin de outro site deveria dar 403, veio %d (%s)", r.status, r.corpo)
	}
	if n := env.qint(`SELECT COUNT(*) FROM parts`); n != 0 {
		t.Errorf("requisicao de outro site gravou dados (%d pecas)", n)
	}
	// sem Origin (requisicao direta, ex.: curl local) continua permitido
	if r := env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) { req.Header.Del("Origin") }); r.status != 200 {
		t.Errorf("GET sem Origin deveria funcionar: %d", r.status)
	}
}

func TestItem7SecFetchSite(t *testing.T) {
	env := novaAppTeste(t)
	casos := []struct {
		valor string
		ok    bool
	}{
		{"same-origin", true},
		{"none", true},
		{"cross-site", false},
		{"same-site", false},
	}
	for _, c := range casos {
		r := env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) { req.Header.Set("Sec-Fetch-Site", c.valor) })
		if c.ok && r.status != 200 {
			t.Errorf("Sec-Fetch-Site %q deveria funcionar, veio %d", c.valor, r.status)
		}
		if !c.ok && r.status != 403 {
			t.Errorf("Sec-Fetch-Site %q deveria dar 403, veio %d", c.valor, r.status)
		}
	}
}

func TestItem7FluxoNormalDaTelaFunciona(t *testing.T) {
	env := novaAppTeste(t)
	peca := env.criarPeca("FLU-001", "Peca fluxo normal", 1000, 2000, 5)
	if r := env.get("/api/pecas?q=FLU"); r.status != 200 {
		t.Fatalf("listar pecas: %d %s", r.status, r.corpo)
	}
	if r := env.post("/api/vendas", corpoVenda(peca, 1, 2000, 0, 0, "dinheiro", map[string]any{"recebido_centavos": 2000})); r.status != 200 {
		t.Fatalf("venda pelo fluxo normal: %d %s", r.status, r.corpo)
	}
	if r := env.get("/api/dashboard"); r.status != 200 {
		t.Fatalf("dashboard: %d %s", r.status, r.corpo)
	}

	// download de backup pelo link da tela (GET, sem cabecalho extra)
	if r := env.post("/api/backup", map[string]any{}); r.status != 200 {
		t.Fatalf("gerar backup: %d %s", r.status, r.corpo)
	}
	lista := env.get("/api/backup")
	itens := env.jsonLista(lista, "itens")
	if len(itens) == 0 {
		t.Fatalf("nenhum backup na lista: %s", lista.corpo)
	}
	nome, _ := itens[0]["arquivo"].(string)
	d := env.reqCom("GET", "/api/backup/baixar?arquivo="+nome, nil, func(req *http.Request) {
		req.Header.Del("X-AutoPecas")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	})
	if d.status != 200 {
		t.Fatalf("download de backup pela tela: %d %s", d.status, d.corpo)
	}
	if len(d.corpo) < 1000 {
		t.Errorf("arquivo baixado parece vazio (%d bytes)", len(d.corpo))
	}
}

func TestItem7ContentTypePrecisaSerJSON(t *testing.T) {
	env := novaAppTeste(t)
	// corpo JSON valido, mas anunciado como texto (como faria um formulario externo)
	corpo := `{"codigo":"CT-1","descricao":"peca com content-type errado"}`
	r := env.reqCom("POST", "/api/pecas", nil, func(req *http.Request) {
		req.Body = io.NopCloser(strings.NewReader(corpo))
		req.ContentLength = int64(len(corpo))
		req.Header.Set("Content-Type", "text/plain")
	})
	if r.status != 400 {
		t.Errorf("POST com Content-Type text/plain deveria ser recusado (400), veio %d (%s)", r.status, r.corpo)
	}
	if n := env.qint(`SELECT COUNT(*) FROM parts`); n != 0 {
		t.Errorf("peca gravada mesmo com Content-Type invalido (%d)", n)
	}
}

func TestItem7SenhaObrigatoriaForaDoLocalhost(t *testing.T) {
	// handler direto para poder fingir que a requisicao vem da rede (1.0.7:
	// quem esta no proprio computador continua entrando sem senha)
	env := novaAppSemServidor(t)
	app := env.app

	app.hostEscuta = "0.0.0.0"
	app.senhaRede = ""
	if err := app.validarInicial(); err == nil {
		t.Errorf("iniciar em 0.0.0.0 sem senha deveria ser recusado")
	}
	app.senhaRede = "senha-secreta"
	if err := app.validarInicial(); err != nil {
		t.Errorf("iniciar em 0.0.0.0 com senha deveria ser permitido: %v", err)
	}

	// com senha definida, a API exige autenticacao para quem vem da rede
	if r := env.reqCom("GET", "/api/pecas", nil, deFora); r.status != 401 {
		t.Errorf("sem senha deveria dar 401, veio %d", r.status)
	}
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("autopecas:senha-secreta"))
	if r := env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) {
		deFora(req)
		req.Header.Set("Authorization", auth)
	}); r.status != 200 {
		t.Errorf("com a senha correta deveria dar 200, veio %d (%s)", r.status, r.corpo)
	}
	errada := "Basic " + base64.StdEncoding.EncodeToString([]byte("autopecas:errada"))
	if r := env.reqCom("GET", "/api/pecas", nil, func(req *http.Request) {
		deFora(req)
		req.Header.Set("Authorization", errada)
	}); r.status != 401 {
		t.Errorf("com senha errada deveria dar 401, veio %d", r.status)
	}
	// o proprio computador (127.0.0.1) entra sem senha, para nao atrapalhar a loja
	if r := env.reqCom("GET", "/api/pecas", nil, doProprioPC); r.status != 200 {
		t.Errorf("do proprio computador deveria dar 200, veio %d", r.status)
	}

	// host local nao exige senha mesmo que configurada
	app.hostEscuta = "127.0.0.1"
	app.senhaRede = ""
	if err := app.validarInicial(); err != nil {
		t.Errorf("iniciar em 127.0.0.1 sem senha deveria ser permitido: %v", err)
	}
	if r := env.get("/api/pecas"); r.status != 200 {
		t.Errorf("depois de remover a senha deveria dar 200, veio %d", r.status)
	}
}

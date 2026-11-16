package main

// Testes da 1.0.11: "Ligar o acesso pelo celular" sem reiniciar o programa.
//
// O defeito original tinha três camadas, e cada uma tem teste aqui:
//   1. o botão mandava GET numa rota que só aceita POST (404) — coberto pelo
//      teste de tela qa/teste_ui_botao_celular.py, que clica no botão de verdade;
//   2. o programa reiniciava (matava um processo e abria outro) e o processo novo
//      às vezes não conseguia a porta e morria calado;
//   3. ao soltar o endereço antigo, o `Serve` recebia "use of closed network
//      connection", o programa tratava como erro grave e saía.

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

// portaLocalLivre devolve uma porta livre (fecha em seguida para poder ser usada).
func portaLocalLivre(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("não consegui abrir porta livre: %v", err)
	}
	porta := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return porta
}

func pegar(t *testing.T, url string) (string, error) {
	t.Helper()
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func esperarServir(t *testing.T, url string) string {
	t.Helper()
	var ultimo error
	for i := 0; i < 60; i++ {
		corpo, err := pegar(t, url)
		if err == nil {
			return corpo
		}
		ultimo = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s não respondeu: %v", url, ultimo)
	return ""
}

// TestTrocaDeEnderecoMantemProgramaNoAr é o coração da correção: o servidor
// continua atendendo antes, durante e depois da troca de endereço.
func TestTrocaDeEnderecoMantemProgramaNoAr(t *testing.T) {
	porta := portaLocalLivre(t)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", porta))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	app := &App{hostEscuta: "127.0.0.1", porta: porta, ln: &listenerTrocavel{ln: ln}}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "vivo")
	})}
	fimServidor := make(chan error, 1)
	go func() { fimServidor <- srv.Serve(app.ln) }()

	local := fmt.Sprintf("http://127.0.0.1:%d/", porta)
	if corpo := esperarServir(t, local); corpo != "vivo" {
		t.Fatalf("resposta inesperada antes da troca: %q", corpo)
	}

	// liga o acesso pelo celular (127.0.0.1 -> 0.0.0.0)
	if err := app.trocarEndereco("0.0.0.0"); err != nil {
		t.Fatalf("trocar para 0.0.0.0 falhou: %v", err)
	}
	if app.hostEscuta != "0.0.0.0" {
		t.Fatalf("endereco em uso = %q (esperado 0.0.0.0)", app.hostEscuta)
	}
	if corpo := esperarServir(t, local); corpo != "vivo" {
		t.Fatalf("o programa parou de atender depois de ligar o celular: %q", corpo)
	}

	// desliga (0.0.0.0 -> 127.0.0.1)
	if err := app.trocarEndereco("127.0.0.1"); err != nil {
		t.Fatalf("trocar de volta para 127.0.0.1 falhou: %v", err)
	}
	if corpo := esperarServir(t, local); corpo != "vivo" {
		t.Fatalf("o programa parou de atender depois de desligar o celular: %q", corpo)
	}

	// o servidor não pode ter morrido no meio do caminho
	select {
	case err := <-fimServidor:
		t.Fatalf("o servidor saiu sozinho durante a troca de endereço (%v)", err)
	case <-time.After(300 * time.Millisecond):
	}

	_ = app.ln.Close()
	_ = srv.Close()
}

// TestSoltarListenerNaoDerrubaOServidor cobre o caminho de erro: se o endereço
// novo não puder ser aberto, o programa volta ao endereço antigo e continua no
// ar (antes ele simplesmente morria).
func TestSoltarListenerNaoDerrubaOServidor(t *testing.T) {
	if runtime.GOOS == "windows" {
		// No Windows um bind para 0.0.0.0:porta continua funcionando mesmo com
		// 127.0.0.2:porta ocupado (o SO nao recusa o wildcard), entao o cenario
		// de "porta ocupada" deste teste nao existe aqui. No Linux/Mac (CI) ele roda.
		t.Skip("Windows nao recusa bind em 0.0.0.0 com outro IP da mesma porta ocupado")
	}
	porta := portaLocalLivre(t)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", porta))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	app := &App{hostEscuta: "127.0.0.1", porta: porta, ln: &listenerTrocavel{ln: ln}}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "vivo")
	})}
	go func() { _ = srv.Serve(app.ln) }()
	local := fmt.Sprintf("http://127.0.0.1:%d/", porta)
	esperarServir(t, local)

	// ocupar a porta em TODAS as interfaces: a troca tem de falhar e voltar
	ladrao, err := net.Listen("tcp", fmt.Sprintf("127.0.0.2:%d", porta))
	if err != nil {
		t.Skipf("este sistema não tem 127.0.0.2 para simular a porta ocupada: %v", err)
	}
	defer ladrao.Close()

	if err := app.trocarEndereco("0.0.0.0"); err == nil {
		t.Fatalf("esperava erro ao abrir 0.0.0.0 com a porta ocupada")
	}
	if app.hostEscuta != "127.0.0.1" {
		t.Fatalf("deveria ter voltado para 127.0.0.1 (ficou %q)", app.hostEscuta)
	}
	if corpo := esperarServir(t, local); corpo != "vivo" {
		t.Fatalf("o programa parou de atender depois de uma troca que falhou: %q", corpo)
	}
	_ = app.ln.Close()
	_ = srv.Close()
}

// TestAcceptDuranteTrocaNaoRetornaErro garante que a troca no meio do Accept
// (o caso que gerava "use of closed network connection") não devolve erro.
func TestAcceptDuranteTrocaNaoRetornaErro(t *testing.T) {
	porta := portaLocalLivre(t)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", porta))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	trocavel := &listenerTrocavel{ln: ln}
	erros := make(chan error, 1)
	go func() {
		for {
			c, err := trocavel.Accept()
			if err != nil {
				if strings.Contains(err.Error(), "closed") {
					erros <- err
				}
				return
			}
			_ = c.Close()
		}
	}()
	time.Sleep(100 * time.Millisecond)
	trocavel.mu.Lock()
	trocavel.trocando = true
	antigo := trocavel.ln
	trocavel.ln = nil
	trocavel.mu.Unlock()
	_ = antigo.Close() // Accept recebe o erro aqui, mas não pode desistir

	novo, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", porta))
	if err != nil {
		t.Fatalf("não consegui reabrir a porta: %v", err)
	}
	trocavel.mu.Lock()
	trocavel.ln = novo
	trocavel.trocando = false
	trocavel.mu.Unlock()

	// uma conexão nova tem de ser aceita (e fechada) sem derrubar o laço
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", porta), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = c.Close()
	select {
	case err := <-erros:
		t.Fatalf("Accept devolveu erro durante a troca: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	_ = trocavel.Close()
}

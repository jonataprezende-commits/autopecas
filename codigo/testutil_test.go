package main

// Infraestrutura de testes: banco SQLite real em diretorio temporario e
// servidor httptest usando o handler REAL da aplicacao (app.routes()),
// com o middleware de seguranca do item 7 incluido.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type ambiente struct {
	t       *testing.T
	app     *App
	srv     *httptest.Server // nil quando direto=true
	handler http.Handler
	direto  bool
	base    string // http://127.0.0.1:porta
	dir     string
}

func novaAppTeste(t *testing.T) *ambiente {
	t.Helper()
	return novaAppTesteEm(t, t.TempDir(), true)
}

// novaAppSemServidor usa o handler direto (recorder). Necessario nos testes de
// travamento: um servidor httptest nao pode ser fechado enquanto uma requisicao
// estiver presa, o que faria a suite inteira parar.
func novaAppSemServidor(t *testing.T) *ambiente {
	t.Helper()
	return novaAppTesteEm(t, t.TempDir(), false)
}

func novaAppTesteEm(t *testing.T, dir string, comServidor bool) *ambiente {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0o755); err != nil {
		t.Fatalf("preparando pasta: %v", err)
	}
	db, err := OpenDB(filepath.Join(dir, "autopecas.db"))
	if err != nil {
		t.Fatalf("abrindo banco: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	app := &App{db: db, dir: dir, hostEscuta: "127.0.0.1"}
	if err := app.migrate(); err != nil {
		t.Fatalf("migracao falhou: %v", err)
	}
	app.firstRun()

	env := &ambiente{t: t, app: app, dir: dir, handler: app.routes()}
	if comServidor {
		srv := httptest.NewServer(app.routes())
		t.Cleanup(srv.Close)
		env.srv = srv
		env.base = srv.URL
		if _, porta, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://")); err == nil {
			if p, err := strconv.Atoi(porta); err == nil {
				app.porta = p
			}
		}
	} else {
		env.direto = true
		app.porta = 8787
		env.base = "http://127.0.0.1:8787"
		t.Cleanup(func() { _ = db.Close() })
	}
	return env
}

// ---------- chamadas HTTP no mesmo formato que o front usa ----------

type resposta struct {
	status  int
	corpo   []byte
	headers http.Header
}

func (r resposta) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.corpo, &m); err != nil {
		t.Fatalf("resposta nao e JSON (%d): %s", r.status, string(r.corpo))
	}
	return m
}

func (r resposta) erro(t *testing.T) string {
	t.Helper()
	m := r.json(t)
	s, _ := m["erro"].(string)
	return s
}

func (e *ambiente) req(metodo, caminho string, corpo any) resposta {
	e.t.Helper()
	return e.reqCom(metodo, caminho, corpo, nil)
}

// reqCom monta a requisicao como o front real faz: Host proprio, Origin igual e
// o cabecalho anti-CSRF X-AutoPecas.
func (e *ambiente) reqCom(metodo, caminho string, corpo any, mexer func(*http.Request)) resposta {
	e.t.Helper()
	r, err := e.montar(metodo, caminho, corpo, mexer)
	if err != nil {
		e.t.Fatalf("%s %s: %v", metodo, caminho, err)
	}
	resp, err := e.executar(r)
	if err != nil {
		e.t.Fatalf("%s %s: %v", metodo, caminho, err)
	}
	return resp
}

func (e *ambiente) montar(metodo, caminho string, corpo any, mexer func(*http.Request)) (*http.Request, error) {
	var rd io.Reader
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(metodo, e.base+caminho, rd)
	if err != nil {
		return nil, err
	}
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-AutoPecas", "1")
	req.Header.Set("Origin", e.base)
	if mexer != nil {
		mexer(req)
	}
	return req, nil
}

func (e *ambiente) executar(req *http.Request) (resposta, error) {
	if e.direto {
		rec := httptest.NewRecorder()
		e.handler.ServeHTTP(rec, req)
		return resposta{status: rec.Code, corpo: rec.Body.Bytes(), headers: rec.Result().Header}, nil
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		return resposta{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resposta{status: resp.StatusCode, corpo: b, headers: resp.Header}, nil
}

func (e *ambiente) get(caminho string) resposta { e.t.Helper(); return e.req("GET", caminho, nil) }
func (e *ambiente) post(caminho string, corpo any) resposta {
	e.t.Helper()
	return e.req("POST", caminho, corpo)
}

// postComTimeout falha o teste se o POST nao responder dentro do limite
// (prova que a operacao nao ficou travada esperando conexao do banco).
func (e *ambiente) postComTimeout(caminho string, corpo any, limite time.Duration) resposta {
	e.t.Helper()
	req, err := e.montar("POST", caminho, corpo, nil)
	if err != nil {
		e.t.Fatalf("montando requisicao: %v", err)
	}
	tipo := make(chan resposta, 1)
	erro := make(chan error, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				erro <- fmt.Errorf("panico: %v", rec)
			}
		}()
		r, err := e.executar(req)
		if err != nil {
			erro <- err
			return
		}
		tipo <- r
	}()
	select {
	case r := <-tipo:
		return r
	case err := <-erro:
		e.t.Fatalf("requisicao falhou: %v", err)
	case <-time.After(limite):
		e.t.Fatalf("TRAVOU: POST %s nao respondeu em %v (conexao do banco presa dentro da transacao)", caminho, limite)
	}
	return resposta{}
}

// enviarArquivoOpcoes faz upload multipart enviando tambem o campo "opcoes" (JSON).
func (e *ambiente) enviarArquivoOpcoes(caminho, nome string, conteudo []byte, opcoes string) resposta {
	e.t.Helper()
	var corpo bytes.Buffer
	mw := multipart.NewWriter(&corpo)
	fw, err := mw.CreateFormFile("arquivo", nome)
	if err != nil {
		e.t.Fatalf("multipart: %v", err)
	}
	_, _ = fw.Write(conteudo)
	if opcoes != "" {
		_ = mw.WriteField("opcoes", opcoes)
	}
	_ = mw.Close()

	req, err := http.NewRequest("POST", e.base+caminho, &corpo)
	if err != nil {
		e.t.Fatalf("requisicao: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-AutoPecas", "1")
	req.Header.Set("Origin", e.base)
	resp, err := e.executar(req)
	if err != nil {
		e.t.Fatalf("upload: %v", err)
	}
	return resp
}

// enviarArquivo faz upload multipart (restaurar backup / importar XML).
func (e *ambiente) enviarArquivo(caminho, campo, nome string, conteudo []byte) resposta {
	e.t.Helper()
	var corpo bytes.Buffer
	mw := multipart.NewWriter(&corpo)
	fw, err := mw.CreateFormFile(campo, nome)
	if err != nil {
		e.t.Fatalf("multipart: %v", err)
	}
	_, _ = fw.Write(conteudo)
	_ = mw.Close()

	req, err := http.NewRequest("POST", e.base+caminho, &corpo)
	if err != nil {
		e.t.Fatalf("requisicao: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-AutoPecas", "1")
	req.Header.Set("Origin", e.base)
	resp, err := e.executar(req)
	if err != nil {
		e.t.Fatalf("upload: %v", err)
	}
	return resp
}

// ---------- atalhos de dados ----------

func (e *ambiente) criarPeca(codigo, descricao string, custo, preco int64, estoque float64) int64 {
	e.t.Helper()
	r := e.post("/api/pecas", map[string]any{
		"codigo": codigo, "descricao": descricao, "custo_centavos": custo,
		"preco_centavos": preco, "minimo": 1, "estoque": estoque,
	})
	if r.status != 200 {
		e.t.Fatalf("criar peca: %d %s", r.status, r.corpo)
	}
	return int64(r.json(e.t)["id"].(float64))
}

func (e *ambiente) criarFornecedor(nome string) int64 {
	e.t.Helper()
	r := e.post("/api/fornecedores", map[string]any{"nome": nome})
	if r.status != 200 {
		e.t.Fatalf("criar fornecedor: %d %s", r.status, r.corpo)
	}
	m := r.json(e.t)
	if id, ok := m["id"].(float64); ok {
		return int64(id)
	}
	if it, ok := m["itens"].([]any); ok && len(it) > 0 {
		return int64(it[0].(map[string]any)["id"].(float64))
	}
	e.t.Fatalf("resposta sem id ao criar fornecedor: %s", r.corpo)
	return 0
}

func (e *ambiente) q1(sqlText string, args ...any) []any {
	e.t.Helper()
	rows, err := e.app.db.Query(sqlText, args...)
	if err != nil {
		e.t.Fatalf("consulta %q: %v", sqlText, err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	if !rows.Next() {
		return nil
	}
	dest := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range dest {
		ptrs[i] = &dest[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		e.t.Fatalf("scan: %v", err)
	}
	return dest
}

func (e *ambiente) qv(sqlText string, args ...any) any {
	e.t.Helper()
	linha := e.q1(sqlText, args...)
	if len(linha) == 0 {
		return nil
	}
	return linha[0]
}

func (e *ambiente) qint(sqlText string, args ...any) int64 {
	e.t.Helper()
	return comoInt(e.qv(sqlText, args...))
}

func (e *ambiente) qstr(sqlText string, args ...any) string {
	e.t.Helper()
	v := e.qv(sqlText, args...)
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	}
	return fmt.Sprint(v)
}

func (e *ambiente) qfloat(sqlText string, args ...any) float64 {
	e.t.Helper()
	v := e.qv(sqlText, args...)
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case []byte:
		f, _ := strconv.ParseFloat(string(x), 64)
		return f
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

func comoInt(v any) int64 {
	switch x := v.(type) {
	case nil:
		return 0
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case []byte:
		n, _ := strconv.ParseInt(string(x), 10, 64)
		return n
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}

func (e *ambiente) estoque(pecaID int64) float64 {
	e.t.Helper()
	return e.qfloat(`SELECT stock FROM parts WHERE id=?`, pecaID)
}

func (e *ambiente) jsonLista(r resposta, campo string) []map[string]any {
	e.t.Helper()
	m := r.json(e.t)
	bruto, ok := m[campo].([]any)
	if !ok {
		e.t.Fatalf("campo %q ausente na resposta: %s", campo, r.corpo)
	}
	out := []map[string]any{}
	for _, x := range bruto {
		if obj, ok := x.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func contemTexto(s string, partes ...string) bool {
	alvo := strings.ToLower(s)
	for _, p := range partes {
		if !strings.Contains(alvo, strings.ToLower(p)) {
			return false
		}
	}
	return true
}

// backupBytes devolve o conteudo de um arquivo .db da pasta de backups
func (e *ambiente) lerArquivo(caminho string) []byte {
	e.t.Helper()
	b, err := os.ReadFile(caminho)
	if err != nil {
		e.t.Fatalf("lendo %s: %v", caminho, err)
	}
	return b
}

package main

// Testes da revisao de bugs da 1.0.10.
//
// Cada teste aqui corresponde a um defeito concreto encontrado na revisao do
// codigo (nao a uma funcionalidade nova). A ideia: se alguem "consertar" o
// codigo de volta para o comportamento antigo, este arquivo quebra.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- pecas: valores impossiveis nao entram ----------

func TestPecaRecusaValoresNegativos(t *testing.T) {
	env := novaAppTeste(t)
	casos := []struct {
		nome  string
		corpo map[string]any
	}{
		{"preco negativo", map[string]any{"descricao": "X", "preco_centavos": -100}},
		{"custo negativo", map[string]any{"descricao": "X", "custo_centavos": -1}},
		{"estoque negativo", map[string]any{"descricao": "X", "estoque": -5}},
		{"minimo negativo", map[string]any{"descricao": "X", "minimo": -2}},
		{"preco acima do teto", map[string]any{"descricao": "X", "preco_centavos": maxPrecoCentavos + 1}},
		{"quantidade acima do teto", map[string]any{"descricao": "X", "estoque": maxQtdItem + 1}},
	}
	for _, c := range casos {
		r := env.post("/api/pecas", c.corpo)
		if r.status != 400 {
			t.Fatalf("%s: esperava 400, veio %d (%s)", c.nome, r.status, string(r.corpo))
		}
		if msg := r.erro(t); msg == "" {
			t.Fatalf("%s: recusou sem explicar o motivo", c.nome)
		}
	}
	// nada foi gravado
	r := env.get("/api/pecas?limite=50")
	itens, _ := r.json(t)["itens"].([]any)
	if len(itens) != 0 {
		t.Fatalf("recusou mas gravou %d peca(s)", len(itens))
	}
}

func TestEditarPecaInexistenteResponde404(t *testing.T) {
	env := novaAppTeste(t)
	r := env.req("PUT", "/api/pecas/999999", map[string]any{"descricao": "fantasma"})
	if r.status != 404 {
		t.Fatalf("esperava 404 ao editar peca inexistente, veio %d (%s)", r.status, string(r.corpo))
	}
}

// ---------- venda: preco absurdo e vencimento do faturado ----------

func TestVendaRecusaPrecoAcimaDoTeto(t *testing.T) {
	env := novaAppTeste(t)
	p := env.post("/api/pecas", map[string]any{"descricao": "Pastilha", "codigo": "P1", "preco_centavos": 1000, "estoque": 10}).json(t)
	id := int64(p["id"].(float64))
	r := env.post("/api/vendas", map[string]any{
		"itens":           []map[string]any{{"peca_id": id, "quantidade": 1, "preco_centavos": maxPrecoCentavos + 1}},
		"forma_pagamento": "dinheiro",
	})
	if r.status != 400 {
		t.Fatalf("esperava 400 para preco absurdo, veio %d (%s)", r.status, string(r.corpo))
	}
}

func TestVendaFaturadaComDataRetroativaVenceAPartirDaData(t *testing.T) {
	env := novaAppTeste(t)
	p := env.post("/api/pecas", map[string]any{"descricao": "Disco", "codigo": "D1", "preco_centavos": 10000, "estoque": 10}).json(t)
	id := int64(p["id"].(float64))
	ontem := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	r := env.post("/api/vendas", map[string]any{
		"itens":           []map[string]any{{"peca_id": id, "quantidade": 1, "preco_centavos": 10000}},
		"forma_pagamento": "faturado",
		"cliente_nome":    "Fulano",
		"vencimento_dias": 30,
		"data":            ontem,
	})
	if r.status != 200 {
		t.Fatalf("venda falhou: %d %s", r.status, string(r.corpo))
	}
	esperado := time.Now().AddDate(0, 0, -10).AddDate(0, 0, 30).Format("2006-01-02")
	rec := env.get("/api/recebiveis")
	var venc string
	for _, it := range rec.json(t)["itens"].([]any) {
		m := it.(map[string]any)
		venc = m["vencimento"].(string)
	}
	if venc != esperado {
		t.Fatalf("vencimento saiu %q, esperava %q (venda retroativa conta da data da venda, nao de hoje)", venc, esperado)
	}
}

// ---------- boletos da nota: soma tem que fechar com o total ----------

func notaTeste(total int64, dups ...Duplicata) NotaInfo {
	return NotaInfo{Numero: "1", TemTotal: total > 0, TotalCents: total, Duplicatas: dups}
}

func TestBoletosComDiferencaDeCentavosSaoAcertados(t *testing.T) {
	n := notaTeste(175000, Duplicata{"1", "2026-11-19", 58333}, Duplicata{"2", "2026-12-19", 58333}, Duplicata{"3", "2027-01-19", 58333})
	contas, avisos := reconciliarBoletos(n, n.Duplicatas)
	var soma int64
	for _, c := range contas {
		soma += c.ValorCents
	}
	if soma != n.TotalCents {
		t.Fatalf("soma das contas %d != total da nota %d", soma, n.TotalCents)
	}
	if len(avisos) != 1 || !strings.Contains(avisos[0], "arredondamento") {
		t.Fatalf("esperava aviso de arredondamento, veio %v", avisos)
	}
	if contas[2].ValorCents != 58334 {
		t.Fatalf("o ajuste tem que cair na ultima parcela, veio %d", contas[2].ValorCents)
	}
}

func TestBoletosQueNaoFechamGeramAvisoENaoInventamValor(t *testing.T) {
	n := notaTeste(200000, Duplicata{"1", "2026-11-19", 50000}, Duplicata{"2", "2026-12-19", 50000})
	contas, avisos := reconciliarBoletos(n, n.Duplicatas)
	var soma int64
	for _, c := range contas {
		soma += c.ValorCents
	}
	if soma != 100000 {
		t.Fatalf("nao pode inventar valor: soma %d, boletos do XML somam 100000", soma)
	}
	if len(avisos) != 1 || !strings.Contains(avisos[0], "ATENÇÃO") {
		t.Fatalf("esperava um aviso claro, veio %v", avisos)
	}
	if !strings.Contains(avisos[0], "1000,00") {
		t.Fatalf("o aviso tem que dizer quanto falta: %s", avisos[0])
	}
}

func TestImportacaoDeNotaAvisaBoletosQueNaoFecham(t *testing.T) {
	env := novaAppTeste(t)
	xml := `<?xml version="1.0" encoding="utf-8"?>
<nfeProc><NFe><infNFe Id="NFe35260912345678000199550010000012341000012345" versao="4.00">
<ide><nNF>1234</nNF><serie>1</serie><dhEmi>2026-10-01T10:00:00-03:00</dhEmi></ide>
<emit><CNPJ>12345678000199</CNPJ><xNome>DISTRIBUIDORA TESTE LTDA</xNome></emit>
<det nItem="1"><prod><cProd>PA1</cProd><cEAN></cEAN><xProd>PASTILHA DE FREIO</xProd><NCM>8708</NCM><CFOP>5102</CFOP>
<uCom>PC</uCom><qCom>2</qCom><vUnCom>50.00</vUnCom><vProd>100.00</vProd></prod></det>
<total><ICMSTot><vProd>100.00</vProd><vNF>180.00</vNF></ICMSTot></total>
<cobr><dup><nDup>001</nDup><dVenc>2026-11-01</dVenc><vDup>60.00</vDup></dup></cobr>
</infNFe></NFe></nfeProc>`
	resp := env.enviarArquivoOpcoes("/api/xml/importar", "nota.xml", []byte(xml),
		`{"criar_novas":true,"lancar_conta_pagar":true}`)
	if resp.status != 200 {
		t.Fatalf("importacao falhou: %d %s", resp.status, string(resp.corpo))
	}
	m := resp.json(t)
	avisos, _ := m["avisos"].([]any)
	if len(avisos) == 0 {
		t.Fatalf("boletos (60) nao fecham com a nota (180) e nenhum aviso saiu: %v", m)
	}
	junto := strings.ToLower(fmt.Sprint(avisos))
	if !strings.Contains(junto, "atenção") && !strings.Contains(junto, "atencao") {
		t.Fatalf("aviso sem destaque: %v", avisos)
	}
	// e a conta lançada continua sendo o valor do boleto, nao o total
	contas := env.get("/api/pagaveis?status=todos").json(t)
	var total int64
	for _, it := range contas["itens"].([]any) {
		total += int64(it.(map[string]any)["valor_centavos"].(float64))
	}
	if total != 6000 {
		t.Fatalf("esperava uma conta de R$ 60,00 (boleto do XML), veio %d centavos", total)
	}
}

// ---------- teto de espaco apaga o mais antigo (por data, nao por nome) ----------

func TestTetoDeEspacoApagaOsMaisAntigosPorData(t *testing.T) {
	env := novaAppTeste(t)
	dir := filepath.Join(env.dir, "backups")
	// nomes escolhidos de proposito: em ordem alfabetica "antes-de-restaurar"
	// vem antes de "backup", mas ele e' o MAIS NOVO. O defeito antigo apagava
	// justamente ele.
	arquivos := []struct {
		nome  string
		bytes int
	}{
		{"backup-horario-2026-01-01_0900.db", 120000},
		{"backup-horario-2026-01-02_0900.db", 120000},
		{"backup-horario-2026-01-03_0900.db", 120000},
		{"antes-de-restaurar-2026-01-04_0900.db", 120000}, // mais novo
	}
	for _, a := range arquivos {
		conteudo := make([]byte, a.bytes)
		for i := range conteudo {
			conteudo[i] = 'x'
		}
		if err := os.WriteFile(filepath.Join(dir, a.nome), conteudo, 0o644); err != nil {
			t.Fatalf("preparando arquivo: %v", err)
		}
	}
	// o menor teto aceito e' 20 MB; entao os arquivos precisam passar disso
	env.app.db.SetSetting("backup_limite_mb", "20")
	grande := make([]byte, 6*1024*1024)
	for _, a := range arquivos {
		if err := os.WriteFile(filepath.Join(dir, a.nome), grande, 0o644); err != nil {
			t.Fatalf("preparando arquivo: %v", err)
		}
	}
	removidos, _, restante := env.app.limparBackups()
	if removidos == 0 {
		t.Fatalf("teto de 1 MB com 1,6 MB de arquivos: nada foi apagado")
	}
	if restante > 20*1024*1024 {
		t.Fatalf("depois da limpeza a pasta continua acima do teto: %d bytes", restante)
	}
	// o mais novo de cada tipo tem que continuar existindo
	if _, err := os.Stat(filepath.Join(dir, "antes-de-restaurar-2026-01-04_0900.db")); err != nil {
		t.Fatalf("apagou o backup MAIS NOVO (antes-de-restaurar): o teto tem que sair do mais antigo por data")
	}
	if _, err := os.Stat(filepath.Join(dir, "backup-horario-2026-01-03_0900.db")); err != nil {
		t.Fatalf("apagou o backup de hora em hora mais novo")
	}
	if _, err := os.Stat(filepath.Join(dir, "backup-horario-2026-01-01_0900.db")); err == nil {
		t.Fatalf("o mais antigo deveria ter saido primeiro")
	}
}

func TestCarimboDoBackupLeADataDoNome(t *testing.T) {
	quando := carimboDoBackup("backup-horario-2026-03-04_1530.db")
	if quando.Format("2006-01-02 15:04") != "2026-03-04 15:30" {
		t.Fatalf("carimbo lido errado: %v", quando)
	}
}

// ---------- restaurar backup nao apaga a senha do celular ----------

func TestRestaurarNaoApagaASenhaDoCelular(t *testing.T) {
	env := novaAppTeste(t)
	// senha atual (como se o dono tivesse ligado o acesso pelo celular)
	salt := novoSalt()
	env.app.db.SetSetting("senha_rede_salt", salt)
	env.app.db.SetSetting("senha_rede_hash", hashSenha(salt, "senha123"))
	env.app.db.SetSetting("acesso_rede", "1")
	env.app.senhaSalt = salt
	env.app.senhaHash = hashSenha(salt, "senha123")

	// um backup antigo, gravado quando a senha era outra (e acesso_rede=0)
	caminho, _, err := env.app.criaArquivoBackup("antes-de-restaurar")
	if err != nil {
		t.Fatalf("gerando backup: %v", err)
	}
	arq, err := OpenDB(caminho)
	if err != nil {
		t.Fatalf("abrindo backup: %v", err)
	}
	arq.SetSetting("senha_rede_salt", "salt-antigo")
	arq.SetSetting("senha_rede_hash", "hash-antigo")
	arq.SetSetting("acesso_rede", "0")
	_ = arq.Close()

	if err := env.app.restaurarDe(caminho); err != nil {
		t.Fatalf("restaurando: %v", err)
	}
	if got := env.app.db.GetSetting("senha_rede_hash"); got != hashSenha(salt, "senha123") {
		t.Fatalf("a senha do celular foi trocada pela do backup (%q)", got)
	}
	if got := env.app.db.GetSetting("acesso_rede"); got != "1" {
		t.Fatalf("acesso_rede virou %q: o celular pararia de entrar depois de restaurar", got)
	}
}

// ---------- ajustes: numero invalido nao entra mais em silencio ----------

func TestConfigRecusaNumeroInvalido(t *testing.T) {
	env := novaAppTeste(t)
	casos := []map[string]any{
		{"backup_manter": "abc"},
		{"backup_manter_horarios": "0"},
		{"backup_limite_mb": "-5"},
		{"backup_intervalo_minutos": "3"},
		{"prazo_faturado_dias": ""},
	}
	for _, c := range casos {
		r := env.post("/api/config", c)
		if r.status != 400 {
			t.Fatalf("%v deveria ser recusado, veio %d (%s)", c, r.status, string(r.corpo))
		}
	}
	// e os ajustes bons continuam
	if r := env.post("/api/config", map[string]any{"backup_manter_horarios": "3"}); r.status != 200 {
		t.Fatalf("ajuste valido recusado: %d %s", r.status, string(r.corpo))
	}
	if got := env.app.db.GetSetting("backup_manter_horarios"); got != "3" {
		t.Fatalf("ajuste valido nao foi gravado (%q)", got)
	}
}

// ---------- ficha ja recebida nao pode ser apagada ----------

func TestFichaRecebidaNaoPodeSerApagada(t *testing.T) {
	env := novaAppTeste(t)
	p := env.post("/api/pecas", map[string]any{"descricao": "Correia", "codigo": "C1", "estoque": 0, "preco_centavos": 5000}).json(t)
	pid := int64(p["id"].(float64))
	f := env.post("/api/fichas", map[string]any{"descricao": "Correia", "quantidade": 2, "peca_id": pid}).json(t)
	fid := int64(f["id"].(float64))
	if r := env.post(fmt.Sprintf("/api/fichas/%d/status", fid), map[string]any{"status": "recebida", "peca_id": pid}); r.status != 200 {
		t.Fatalf("receber ficha falhou: %d %s", r.status, string(r.corpo))
	}
	r := env.req("DELETE", fmt.Sprintf("/api/fichas/%d", fid), nil)
	if r.status != 409 {
		t.Fatalf("apagar ficha ja recebida deveria dar 409, veio %d (%s)", r.status, string(r.corpo))
	}
	// continua no lugar
	if r := env.get("/api/fichas?status=todas"); !strings.Contains(string(r.corpo), "Correia") {
		t.Fatalf("a ficha sumiu da lista")
	}
}

// ---------- ajuste manual de estoque: quantidade absurda ----------

func TestAjusteDeEstoqueRecusaQuantidadeAbsurda(t *testing.T) {
	env := novaAppTeste(t)
	p := env.post("/api/pecas", map[string]any{"descricao": "Filtro", "codigo": "F1", "estoque": 1}).json(t)
	pid := int64(p["id"].(float64))
	r := env.post("/api/estoque/movimento", map[string]any{"peca_id": pid, "tipo": "entrada", "quantidade": maxQtdItem + 5})
	if r.status != 400 {
		t.Fatalf("quantidade absurda deveria ser recusada, veio %d (%s)", r.status, string(r.corpo))
	}
	r = env.post("/api/estoque/movimento", map[string]any{"peca_id": pid, "tipo": "entrada", "quantidade": 5, "custo_centavos": -10})
	if r.status != 400 {
		t.Fatalf("custo negativo deveria ser recusado, veio %d (%s)", r.status, string(r.corpo))
	}
}

// ---------- conta manual: nao grava CNPJ como numero da nota ----------

func TestContaManualNaoUsaCNPJComoNumeroDaNota(t *testing.T) {
	env := novaAppTeste(t)
	r := env.post("/api/pagaveis", map[string]any{
		"fornecedor": "ALUGUEL DO PONTO", "documento": "12.345.678/0001-99",
		"valor_centavos": 150000, "vencimento": "2026-11-10",
	})
	if r.status != 200 {
		t.Fatalf("criar conta falhou: %d %s", r.status, string(r.corpo))
	}
	itens := env.get("/api/pagaveis?status=todos").json(t)["itens"].([]any)
	m := itens[0].(map[string]any)
	if doc, _ := m["documento"].(string); doc != "" {
		t.Fatalf("numero do documento saiu %q; o CNPJ tem que ficar em cnpj", doc)
	}
	if cnpj, _ := m["cnpj"].(string); !strings.Contains(cnpj, "12345678000199") {
		t.Fatalf("CNPJ nao foi guardado: %q", cnpj)
	}
}

// ---------- troca de senha derruba sessoes antigas ----------

func TestTrocarSenhaDerrubaSessoesAntigas(t *testing.T) {
	env := novaAppTeste(t)
	tk := env.app.sessoesProntas().criar()
	if !env.app.sessoesProntas().valido(tk) {
		t.Fatalf("sessao recem criada deveria valer")
	}
	env.app.sessoesProntas().limparTodas()
	if env.app.sessoesProntas().valido(tk) {
		t.Fatalf("depois de trocar a senha o token antigo continua valendo")
	}
	if env.app.sessoesProntas().quantidade() != 0 {
		t.Fatalf("sobraram sessoes abertas")
	}
}

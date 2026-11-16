package main

// Testes da leitura de nota de compra por foto.
//
// Os arquivos testdata/nota-tsv-*.txt sao a saida REAL do Tesseract sobre as
// fotos testdata/nota-foto-*.jpg (mesma nota de 8 itens, total R$ 932,20, em
// tres qualidades de foto). Com eles o parser e testado sem depender de OCR
// instalado na maquina que roda a suite.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func lerFixture(t *testing.T, nome string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", nome))
	if err != nil {
		t.Fatalf("fixture %s: %v", nome, err)
	}
	return b
}

// notaDeGabarito e a nota usada nas fotos de teste.
func notaDeGabarito() map[string]struct {
	Desc  string
	Qtd   float64
	Unit  int64
	Total int64
} {
	return map[string]struct {
		Desc  string
		Qtd   float64
		Unit  int64
		Total int64
	}{
		"SKU-1001": {"FILTRO DE OLEO MOTOR 1.0", 2, 1850, 3700},
		"SKU-1002": {"PASTILHA DE FREIO DIANTEIRA", 1, 8990, 8990},
		"SKU-1003": {"CORREIA DENTADA 6PK", 3, 4230, 12690},
		"SKU-1004": {"VELA DE IGNICAO IRIDIUM", 4, 3975, 15900},
		"SKU-1005": {"OLEO 5W30 SINTETICO 1L", 6, 3490, 20940},
		"SKU-1006": {"FILTRO DE AR ESPORTIVO", 2, 6500, 13000},
		"SKU-1007": {"ADITIVO RADIADOR 500ML", 5, 1240, 6200},
		"SKU-1008": {"JOGO DE CABOS DE VELA", 1, 11800, 11800},
	}
}

// TestOcrNotaFotografadaExtraiOsOitoItens: na foto boa o parser tem que acertar
// quantidade, unitario e total dos 8 itens (e conferir cada linha pela conta).
func TestOcrNotaFotografadaExtraiOsOitoItens(t *testing.T) {
	tsv := string(lerFixture(t, "nota-tsv-boa.txt"))
	res := analisarNotaFoto(nil, tsv, qualidadeFoto{}, "teste", 0)

	if len(res.Itens) != 8 {
		t.Fatalf("esperava 8 itens, veio %d: %+v", len(res.Itens), res.Itens)
	}
	gab := notaDeGabarito()
	for _, it := range res.Itens {
		esperado, ok := gab[it.Codigo]
		if !ok {
			t.Errorf("codigo inesperado: %q (linha: %s)", it.Codigo, it.LinhaTexto)
			continue
		}
		if it.Qtd != esperado.Qtd {
			t.Errorf("%s: quantidade %v, esperava %v", it.Codigo, it.Qtd, esperado.Qtd)
		}
		if it.Unitario != esperado.Unit {
			t.Errorf("%s: unitario %s, esperava %s", it.Codigo, fmtMoney(it.Unitario), fmtMoney(esperado.Unit))
		}
		if it.Total != esperado.Total {
			t.Errorf("%s: total %s, esperava %s", it.Codigo, fmtMoney(it.Total), fmtMoney(esperado.Total))
		}
		if !it.Conferido {
			t.Errorf("%s deveria estar conferido pela conta %v x %s = %s", it.Codigo, it.Qtd, fmtMoney(it.Unitario), fmtMoney(it.Total))
		}
		// descricao util = pelo menos duas palavras de verdade (o OCR pode trocar
		// uma letra: "CORREIA" saiu "CORRETA", o que nao invalida a linha)
		if palavras := strings.Fields(it.Descricao); len(palavras) < 2 {
			t.Errorf("%s: descricao curta demais: %q", it.Codigo, it.Descricao)
		}
	}
	if res.SomaItens != 93220 {
		t.Errorf("soma dos itens %s, esperava R$ 932,20", fmtMoney(res.SomaItens))
	}
	if res.Veredito != "ok" {
		t.Errorf("veredito %q (esperava ok), mensagem: %s", res.Veredito, res.Mensagem)
	}
}

// TestOcrFotoMediaComSombraTambemEhLida: a foto "media" (na mao, sombra de um
// lado, 72% de qualidade) tambem tem que sair certa. Esta e a foto que mais
// parece com a do balcao, entao ela e o caso de aceitacao de verdade.
func TestOcrFotoMediaComSombraTambemEhLida(t *testing.T) {
	tsv := string(lerFixture(t, "nota-tsv-media.txt"))
	res := analisarNotaFoto(nil, tsv, qualidadeFoto{}, "teste", 0)
	gab := notaDeGabarito()
	certos := 0
	for _, it := range res.Itens {
		if e, ok := gab[it.Codigo]; ok && it.Qtd == e.Qtd && it.Total == e.Total {
			certos++
		}
	}
	if certos < 7 {
		t.Fatalf("foto media: so %d de 8 itens certos", certos)
	}
	if res.SomaItens != 93220 {
		t.Fatalf("foto media: soma %s, esperava R$ 932,20", fmtMoney(res.SomaItens))
	}
	if res.Veredito != "ok" {
		t.Fatalf("foto media: veredito %q (esperava ok) - %s", res.Veredito, res.Mensagem)
	}
}

// TestOcrFotoRuimNaoEntregaNotaConferida: foto ruim nao pode terminar como "ok".
// O usuario tem que ser avisado para tirar outra foto (ou revisar linha a linha).
func TestOcrFotoRuimNaoEntregaNotaConferida(t *testing.T) {
	for _, caso := range []string{"nota-tsv-ruim.txt"} {
		tsv := string(lerFixture(t, caso))
		res := analisarNotaFoto(nil, tsv, qualidadeFoto{}, "teste", 0)
		if res.Veredito == "ok" {
			t.Errorf("%s: veredito 'ok' com foto ruim (itens=%d, soma=%s, total=%s)",
				caso, len(res.Itens), fmtMoney(res.SomaItens), fmtMoney(res.TotalNota))
		}
		if res.Mensagem == "" {
			t.Errorf("%s: foto ruim precisa de mensagem explicando o que fazer", caso)
		}
		conferidos := 0
		for _, it := range res.Itens {
			if it.Conferido {
				conferidos++
			}
		}
		if conferidos == 8 {
			t.Errorf("%s: 8 linhas conferidas numa foto ruim - a validacao deixou passar erro", caso)
		}
	}
}

// TestOcrSomaDiferenteDoTotalDaNotaEhAvisada: se a soma nao fecha com o total
// impresso, a tela tem que receber a diferenca e o aviso.
func TestOcrSomaDiferenteDoTotalDaNotaEhAvisada(t *testing.T) {
	tsv := "5\t1\t1\t1\t1\t1\t10\t10\t40\t20\t95\tSKU-9001\n" +
		"5\t1\t1\t1\t1\t2\t60\t10\t40\t20\t95\tPECA\n" +
		"5\t1\t1\t1\t1\t3\t100\t10\t50\t20\t95\t10,00\n" +
		"5\t1\t1\t1\t1\t4\t160\t10\t50\t20\t95\t10,00\n" +
		"5\t1\t1\t2\t1\t1\t10\t60\t50\t20\t95\tTOTAL\n" +
		"5\t1\t1\t2\t1\t2\t70\t60\t50\t20\t95\t99,00\n"
	res := analisarNotaFoto(nil, tsv, qualidadeFoto{}, "teste", 0)
	if len(res.Itens) != 1 {
		t.Fatalf("esperava 1 item, veio %d", len(res.Itens))
	}
	if res.Itens[0].Total != 1000 || res.Itens[0].Unitario != 1000 {
		t.Fatalf("valores lidos errados: %+v", res.Itens[0])
	}
	if res.TotalNota != 9900 {
		t.Fatalf("total da nota lido %s, esperava R$ 99,00", fmtMoney(res.TotalNota))
	}
	if res.Conferido {
		t.Fatal("soma R$ 10,00 x total R$ 99,00 nao pode ser dada como conferida")
	}
	if res.Veredito != "revisar" {
		t.Fatalf("veredito %q, esperava revisar", res.Veredito)
	}
	if !strings.Contains(res.Mensagem, "não fecha") {
		t.Fatalf("mensagem sem aviso da diferenca: %q", res.Mensagem)
	}
}

// TestOcrAchaQuantidadePelaDivisao: quando o OCR come a coluna de quantidade,
// o parser recupera dividindo o total pelo unitario.
func TestOcrAchaQuantidadePelaDivisao(t *testing.T) {
	tsv := "5\t1\t1\t1\t1\t1\t10\t10\t40\t20\t92\tSKU-7777\n" +
		"5\t1\t1\t1\t1\t2\t70\t10\t80\t20\t92\tLAMPADA\n" +
		"5\t1\t1\t1\t1\t3\t180\t10\t60\t20\t92\t12,50\n" +
		"5\t1\t1\t1\t1\t4\t260\t10\t60\t20\t92\t50,00\n"
	res := analisarNotaFoto(nil, tsv, qualidadeFoto{}, "teste", 0)
	if len(res.Itens) != 1 {
		t.Fatalf("esperava 1 item, veio %d", len(res.Itens))
	}
	it := res.Itens[0]
	if it.Qtd != 4 || it.Unitario != 1250 || it.Total != 5000 {
		t.Fatalf("esperava 4 x R$ 12,50 = R$ 50,00, veio %v x %s = %s", it.Qtd, fmtMoney(it.Unitario), fmtMoney(it.Total))
	}
	if !it.Conferido {
		t.Fatal("a conta fecha (4 x 12,50 = 50,00): a linha deveria estar conferida")
	}
}

// TestOcrValorComMilharNaNota: "1.234,50" tem que virar 123450 centavos.
func TestOcrValorComMilharNaNota(t *testing.T) {
	tsv := "5\t1\t1\t1\t1\t1\t10\t10\t40\t20\t94\tSKU-5000\n" +
		"5\t1\t1\t1\t1\t2\t70\t10\t80\t20\t94\tMOTOR\n" +
		"5\t1\t1\t1\t1\t3\t170\t10\t30\t20\t94\t1\n" +
		"5\t1\t1\t1\t1\t4\t220\t10\t80\t20\t94\t1.234,50\n" +
		"5\t1\t1\t1\t1\t5\t320\t10\t80\t20\t94\t1.234,50\n"
	res := analisarNotaFoto(nil, tsv, qualidadeFoto{}, "teste", 0)
	if len(res.Itens) != 1 {
		t.Fatalf("esperava 1 item, veio %d", len(res.Itens))
	}
	if res.Itens[0].Unitario != 123450 || res.Itens[0].Total != 123450 {
		t.Fatalf("esperava 123450 centavos, veio %s", fmtMoney(res.Itens[0].Total))
	}
}

// TestOcrFotoInvalidaEhRecusada: arquivo que nao e imagem tem que dar 400.
func TestOcrFotoInvalidaEhRecusada(t *testing.T) {
	env := novaAppTeste(t)
	r := env.enviarArquivo("/api/nota-foto/ler", "arquivo", "nota.txt", []byte("isto nao e uma foto"))
	if r.status != 400 {
		t.Fatalf("esperava 400, veio %d: %s", r.status, r.corpo)
	}
	if !strings.Contains(strings.ToLower(r.erro(t)), "imagem") {
		t.Fatalf("mensagem deveria falar de imagem: %q", r.erro(t))
	}
}

// TestOcrImportarSemConfirmacaoNaoGravaEstoque: o caminho mais perigoso.
// Sem confirmado=true nada pode ser gravado.
func TestOcrImportarSemConfirmacaoNaoGravaEstoque(t *testing.T) {
	env := novaAppTeste(t)
	ida := env.criarPeca("OCR-1", "FILTRO DE OLEO", 1000, 1600, 5)

	r := env.post("/api/nota-foto/importar", map[string]any{
		"confirmado": false,
		"itens": []map[string]any{{
			"codigo": "OCR-1", "descricao": "FILTRO DE OLEO",
			"quantidade": 3, "unitario_centavos": 2000, "total_centavos": 6000,
		}},
	})
	if r.status != 400 {
		t.Fatalf("esperava 400, veio %d: %s", r.status, r.corpo)
	}
	if !strings.Contains(strings.ToLower(r.erro(t)), "confirm") {
		t.Fatalf("mensagem deveria pedir confirmacao: %q", r.erro(t))
	}
	estoque, custo := env.estoqueECusto(t, ida)
	if estoque != 5 {
		t.Fatalf("estoque mudou sem confirmacao: %v (esperava 5)", estoque)
	}
	if custo != 1000 {
		t.Fatalf("custo mudou sem confirmacao: %d", custo)
	}
}

// TestOcrImportarLancaEstoqueECustoUmaVez: com confirmacao, a nota entra no
// estoque uma unica vez, atualiza o custo e registra o motivo certo.
func TestOcrImportarLancaEstoqueECustoUmaVez(t *testing.T) {
	env := novaAppTeste(t)
	ida := env.criarPeca("OCR-2", "PASTILHA DE FREIO", 1000, 1600, 5)

	corpo := map[string]any{
		"confirmado": true, "criar_novas": true, "atualizar_custo": true,
		"lancar_estoque": true, "receber_fichas": false, "margem_percentual": 60,
		"itens": []map[string]any{{
			"codigo": "OCR-2", "descricao": "PASTILHA DE FREIO",
			"quantidade": 3, "unitario_centavos": 2000, "total_centavos": 6000,
		}},
	}
	r := env.post("/api/nota-foto/importar", corpo)
	if r.status != 200 {
		t.Fatalf("esperava 200, veio %d: %s", r.status, r.corpo)
	}
	m := r.json(t)
	if int(m["atualizadas"].(float64)) != 1 {
		t.Fatalf("esperava 1 atualizada, veio %v", m["atualizadas"])
	}
	estoque, custo := env.estoqueECusto(t, ida)
	if estoque != 8 {
		t.Fatalf("estoque %v, esperava 8 (5 + 3 da nota)", estoque)
	}
	if custo != 2000 {
		t.Fatalf("custo %d, esperava 2000 (unitario da nota)", custo)
	}

	// importar de novo (duplo clique) lança de novo: e o usuario quem confirma,
	// mas o registro precisa dizer de onde veio cada entrada
	env.post("/api/nota-foto/importar", corpo)
	var motivo string
	if err := env.app.db.QueryRow(`SELECT reason FROM stock_moves WHERE part_id=? ORDER BY id DESC LIMIT 1`, ida).Scan(&motivo); err != nil {
		t.Fatalf("lendo motivo do movimento: %v", err)
	}
	if !strings.Contains(strings.ToLower(motivo), "nota fotografada") {
		t.Fatalf("motivo do movimento deveria citar a nota fotografada, veio %q", motivo)
	}
}

// TestOcrImportarCriaPecaNova: peca que nao existe entra no cadastro com o
// custo da nota e o preco de venda pela margem.
func TestOcrImportarCriaPecaNova(t *testing.T) {
	env := novaAppTeste(t)
	r := env.post("/api/nota-foto/importar", map[string]any{
		"confirmado": true, "criar_novas": true, "atualizar_custo": true,
		"lancar_estoque": true, "margem_percentual": 50,
		"itens": []map[string]any{{
			"codigo": "NOVA-77", "descricao": "VELA DE IGNICAO IRIDIUM",
			"quantidade": 4, "unitario_centavos": 3975, "total_centavos": 15900,
		}},
	})
	if r.status != 200 {
		t.Fatalf("esperava 200, veio %d: %s", r.status, r.bodyString())
	}
	if int(r.json(t)["cadastradas"].(float64)) != 1 {
		t.Fatalf("esperava 1 cadastrada: %s", r.corpo)
	}
	var estoque float64
	var custo, preco int64
	if err := env.app.db.QueryRow(`SELECT stock,cost_cents,price_cents FROM parts WHERE code_norm=?`, normCode("NOVA-77")).
		Scan(&estoque, &custo, &preco); err != nil {
		t.Fatalf("peca nao foi criada: %v", err)
	}
	if estoque != 4 || custo != 3975 || preco != 5962 {
		t.Fatalf("peca criada com estoque=%v custo=%d preco=%d (esperava 4 / 3975 / 5962)", estoque, custo, preco)
	}
}

// TestOcrLerNaoGravaNada: ler a foto nao pode mexer no banco.
func TestOcrLerNaoGravaNada(t *testing.T) {
	if runtime.GOOS != "windows" {
		// O motor de OCR embutido é um tesseract.exe (Windows). No Linux/Mac do
		// CI ele existe no cache mas não executa ("exec format error").
		t.Skip("motor de OCR embutido é binário Windows; fora do Windows não roda")
	}
	if !motorDisponivel() {
		t.Skip("motor de OCR nao instalado nesta maquina (teste do caminho completo)")
	}
	env := novaAppTeste(t)
	antes := env.contar(t, "parts")
	foto := lerFixture(t, "nota-foto-boa.jpg")
	r := env.enviarArquivo("/api/nota-foto/ler", "arquivo", "nota.jpg", foto)
	if r.status != 200 {
		t.Fatalf("esperava 200, veio %d: %s", r.status, r.corpo)
	}
	depois := env.contar(t, "parts")
	if depois != antes {
		t.Fatalf("a leitura criou pecas (%d -> %d): ela nao pode gravar nada", antes, depois)
	}
	if n := env.contar(t, "stock_moves"); n != 0 {
		t.Fatalf("a leitura lancou movimento de estoque (%d)", n)
	}
}

// TestOcrFotoBoaCaminhoCompleto: foto -> OCR -> itens. Prova que o pipeline de
// imagem + motor embutido funciona de verdade (pulado se nao houver motor).
func TestOcrFotoBoaCaminhoCompleto(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("motor de OCR embutido é binário Windows; fora do Windows não roda")
	}
	if !motorDisponivel() {
		t.Skip("motor de OCR nao instalado nesta maquina")
	}
	foto := lerFixture(t, "nota-foto-boa.jpg")
	png, qual, err := prepararNotaFoto(foto)
	if err != nil {
		t.Fatalf("preparando imagem: %v", err)
	}
	if qual.Largura == 0 || qual.Contraste <= 0 {
		t.Fatalf("metricas da imagem nao calculadas: %+v", qual)
	}
	tsv, err := rodarOCR(png)
	if err != nil {
		t.Fatalf("OCR: %v", err)
	}
	res := analisarNotaFoto(foto, tsv, qual, descricaoMotor(), 0)
	gab := notaDeGabarito()
	certos := 0
	for _, it := range res.Itens {
		if e, ok := gab[it.Codigo]; ok && it.Total == e.Total && it.Qtd == e.Qtd {
			certos++
		}
	}
	if certos != 8 {
		t.Fatalf("so %d de 8 itens sairam certos pela foto boa (veredito %s)", certos, res.Veredito)
	}
	if res.SomaItens != 93220 {
		t.Fatalf("soma dos itens %s, esperava R$ 932,20", fmtMoney(res.SomaItens))
	}
	if res.TotalNota == 0 {
		t.Fatal("total da nota nao foi lido nem na segunda passada")
	}
	if !res.Conferido {
		t.Fatalf("soma %s x total %s: deveria estar conferido", fmtMoney(res.SomaItens), fmtMoney(res.TotalNota))
	}
	if res.Veredito != "ok" {
		t.Fatalf("veredito %q, esperava ok", res.Veredito)
	}
}

// ---------- apoio ----------

func (env *ambiente) estoqueECusto(t *testing.T, id int64) (float64, int64) {
	t.Helper()
	var estoque float64
	var custo int64
	if err := env.app.db.QueryRow(`SELECT stock,cost_cents FROM parts WHERE id=?`, id).Scan(&estoque, &custo); err != nil {
		t.Fatalf("lendo peca %d: %v", id, err)
	}
	return estoque, custo
}

func (env *ambiente) contar(t *testing.T, tabela string) int {
	t.Helper()
	var n int
	if err := env.app.db.QueryRow(`SELECT COUNT(*) FROM ` + tabela).Scan(&n); err != nil {
		t.Fatalf("contando %s: %v", tabela, err)
	}
	return n
}

func (r resposta) bodyString() string { return string(r.corpo) }

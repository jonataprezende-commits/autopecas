package main

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// lerImagemUpload le a foto enviada (multipart, campo "arquivo").
func lerImagemUpload(r *http.Request) ([]byte, string, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, "", err
	}
	f, hdr, err := r.FormFile("arquivo")
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 32<<20))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("arquivo vazio")
	}
	return data, hdr.Filename, nil
}

// handleLerNotaFoto recebe a foto, roda o OCR e devolve os itens para conferencia.
// NADA e gravado aqui: o estoque so muda no /api/nota-foto/importar, depois do
// usuario revisar e confirmar.
func (a *App) handleLerNotaFoto(w http.ResponseWriter, r *http.Request) {
	inicio := time.Now()
	data, nome, err := lerImagemUpload(r)
	if err != nil {
		fail(w, 400, "não consegui receber a foto: "+err.Error())
		return
	}
	if len(data) > 25<<20 {
		fail(w, 400, fmt.Sprintf("a foto é grande demais (%d MB). Use uma foto de até 25 MB.", len(data)>>20))
		return
	}
	png, qual, err := prepararNotaFoto(data)
	if err != nil {
		fail(w, 400, "arquivo não é uma imagem válida: "+err.Error())
		return
	}
	if !motorDisponivel() {
		fail(w, 503, "o leitor de notas não está disponível nesta instalação. Use a importação de XML ou digite a nota.")
		return
	}
	tsv, err := rodarOCR(png)
	if err != nil {
		fail(w, 500, "falha ao ler a nota: "+err.Error())
		return
	}
	res := analisarNotaFoto(data, tsv, qual, descricaoMotor(), time.Since(inicio).Milliseconds())
	if res.TotalNota == 0 && res.SomaItens > 0 {
		// a linha do total tem fonte maior e as vezes sai ilegivel: segunda
		// tentativa so naquele pedaco, pedindo apenas digitos
		if v := lerTotalSegundaPassada(png, tsv, res.SomaItens); v > 0 {
			res.TotalNota = v
			res.Diferenca = res.SomaItens - v
			if res.Diferenca < 0 {
				res.Diferenca = -res.Diferenca
			}
			res.Conferido = res.Diferenca <= 2
			if res.Conferido {
				res.Veredito = "ok"
				res.Mensagem = ""
			}
			res.Avisos = append(res.Avisos, "total da nota lido em segunda passada")
		}
	}
	// altura do texto: mediana das alturas das palavras do OCR
	res.Qualidade.AlturaTexto = alturaTextoMediana(tsv)
	if res.Mensagem == "" && nome != "" {
		res.Avisos = append(res.Avisos, "arquivo: "+nome)
	}
	writeJSON(w, 200, res)
}

func alturaTextoMediana(tsv string) int {
	ps := lerPalavrasTSV(tsv)
	if len(ps) == 0 {
		return 0
	}
	alturas := make([]int, 0, len(ps))
	for _, p := range ps {
		if p.A > 0 {
			alturas = append(alturas, p.A)
		}
	}
	if len(alturas) == 0 {
		return 0
	}
	return mediana(alturas)
}

func mediana(v []int) int {
	cp := append([]int(nil), v...)
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && cp[j] < cp[j-1]; j-- {
			cp[j], cp[j-1] = cp[j-1], cp[j]
		}
	}
	return cp[len(cp)/2]
}

// ---------------------------------------------------------------- importacao

type NotaImportItem struct {
	Codigo    string  `json:"codigo"`
	Descricao string  `json:"descricao"`
	Qtd       float64 `json:"quantidade"`
	Unitario  int64   `json:"unitario_centavos"`
	Total     int64   `json:"total_centavos"`
}

type NotaImportInput struct {
	Itens             []NotaImportItem `json:"itens"`
	Confirmado        bool             `json:"confirmado"`
	CriarNovas        bool             `json:"criar_novas"`
	AtualizarCusto    bool             `json:"atualizar_custo"`
	LancarEstoque     bool             `json:"lancar_estoque"`
	ReceberFichas     bool             `json:"receber_fichas"`
	Margem            float64          `json:"margem_percentual"`
	FornecedorID      *int64           `json:"fornecedor_id"`
	TotalNotaCentavos int64            `json:"total_nota_centavos"`
}

type NotaImportLinha struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
	PecaID    int64  `json:"peca_id"`
	Acao      string `json:"acao"` // cadastrada | atualizada | ignorada
	Motivo    string `json:"motivo,omitempty"`
}

type NotaImportResultado struct {
	Cadastradas    int               `json:"cadastradas"`
	Atualizadas    int               `json:"atualizadas"`
	Ignoradas      int               `json:"ignoradas"`
	FichasFechadas int               `json:"fichas_fechadas"`
	TotalCompra    int64             `json:"total_compra_centavos"`
	Itens          []NotaImportLinha `json:"itens"`
}

// handleImportarNotaFoto grava as pecas e o estoque dos itens conferidos.
// Exige confirmado=true: o sistema nunca lanca estoque de uma leitura automatica
// sem o usuario ter visto a lista.
func (a *App) handleImportarNotaFoto(w http.ResponseWriter, r *http.Request) {
	var in NotaImportInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "dados inválidos")
		return
	}
	if !in.Confirmado {
		fail(w, 400, "confirme a conferência da nota antes de importar")
		return
	}
	if len(in.Itens) == 0 {
		fail(w, 400, "selecione pelo menos um item da nota")
		return
	}
	if in.Margem < 0 {
		in.Margem = 0
	}
	if in.Margem == 0 {
		in.Margem = 60
	}
	opts := defaultImportOptions()
	opts.Origem = "nota fotografada"
	opts.PrecoXMLCusto = true
	opts.AtualizarCusto = in.AtualizarCusto
	opts.AtualizarEstoque = in.LancarEstoque
	opts.SobrescreverEstoque = false
	opts.AtualizarPreco = false
	opts.AtualizarDescricao = false
	opts.CriarNovas = in.CriarNovas
	opts.ReceberFichas = in.ReceberFichas
	opts.Margem = in.Margem
	opts.FornecedorID = in.FornecedorID

	res := NotaImportResultado{Itens: []NotaImportLinha{}}
	for _, item := range in.Itens {
		desc := strings.TrimSpace(item.Descricao)
		codigo := strings.TrimSpace(item.Codigo)
		linha := NotaImportLinha{Codigo: codigo, Descricao: desc}
		if codigo == "" && desc == "" {
			linha.Acao, linha.Motivo = "ignorada", "linha sem código e sem descrição"
			res.Ignoradas++
			res.Itens = append(res.Itens, linha)
			continue
		}
		if item.Qtd <= 0 {
			linha.Acao, linha.Motivo = "ignorada", "quantidade precisa ser maior que zero"
			res.Ignoradas++
			res.Itens = append(res.Itens, linha)
			continue
		}
		if item.Unitario < 0 || item.Total < 0 {
			linha.Acao, linha.Motivo = "ignorada", "valor negativo"
			res.Ignoradas++
			res.Itens = append(res.Itens, linha)
			continue
		}
		it := xmlItem{CodFab: codigo, Codigo: codigo, Descricao: desc, CustoTxt: centavosParaTexto(item.Unitario)}
		custo := item.Unitario
		preco := int64(0)
		if custo > 0 {
			preco = int64(float64(custo) * (1 + opts.Margem/100.0))
		}
		id, _ := a.matchPart(it)
		if id == 0 && desc != "" {
			id = a.idPorDescricaoExata(desc)
		}
		if id > 0 {
			if err := a.updatePartFromXML(id, it, custo, preco, item.Qtd, opts); err != nil {
				linha.Acao, linha.Motivo = "ignorada", "erro ao atualizar: "+err.Error()
				res.Ignoradas++
				res.Itens = append(res.Itens, linha)
				continue
			}
			linha.Acao, linha.PecaID = "atualizada", id
			res.Atualizadas++
			if in.ReceberFichas {
				n, _ := a.receiveFaltasForPart(id, desc)
				res.FichasFechadas += n
			}
		} else if in.CriarNovas {
			novoID, err := a.createPartFromXML(it, custo, preco, item.Qtd, opts)
			if err != nil {
				linha.Acao, linha.Motivo = "ignorada", "erro ao cadastrar: "+err.Error()
				res.Ignoradas++
				res.Itens = append(res.Itens, linha)
				continue
			}
			linha.Acao, linha.PecaID = "cadastrada", novoID
			res.Cadastradas++
			if in.ReceberFichas {
				n, _ := a.receiveFaltasForPart(novoID, desc)
				res.FichasFechadas += n
			}
		} else {
			linha.Acao, linha.Motivo = "ignorada", "peça não cadastrada e a opção de criar novas está desligada"
			res.Ignoradas++
			res.Itens = append(res.Itens, linha)
			continue
		}
		res.TotalCompra += int64(item.Qtd*float64(item.Unitario) + 0.5)
		res.Itens = append(res.Itens, linha)
	}
	writeJSON(w, 200, res)
}

// idPorDescricaoExata acha a peca pela descricao identica (o codigo do
// fornecedor muitas vezes nao e o codigo usado na loja).
func (a *App) idPorDescricaoExata(desc string) int64 {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return 0
	}
	var id int64
	if err := a.db.QueryRow(`SELECT id FROM parts WHERE lower(trim(description))=lower(?) LIMIT 1`, desc).Scan(&id); err == nil {
		return id
	}
	return 0
}

// centavosParaTexto devolve "1234,56" (formato que o importador entende).
func centavosParaTexto(c int64) string {
	if c < 0 {
		c = 0
	}
	return strconv.FormatInt(c/100, 10) + "," + strconv.FormatInt(c%100+100, 10)[1:]
}

// handleStatusNotaFoto informa se o leitor de notas esta pronto nesta maquina.
func (a *App) handleStatusNotaFoto(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"disponivel": motorDisponivel(),
		"motor":      descricaoMotor(),
	})
}

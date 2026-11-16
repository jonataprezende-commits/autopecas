package main

// Leitura de nota de compra fotografada.
//
// Etapas:
//  1. imagem -> cinza, reduzida para no maximo 2000 px de largura
//  2. inclinacao estimada pela projecao horizontal dos pixels escuros e corrigida
//  3. binarizacao de Otsu e recorte na area com conteudo
//  4. OCR (Tesseract) devolve palavras com posicao e confianca
//  5. as palavras sao remontadas em linhas e, em cada linha, o parser procura
//     quantidade, unitario e total que satisfacam a conta qtd x unit = total
//
// A conferencia aritmetica e o que da seguranca: uma linha so e dada como certa
// quando a conta fecha. Quando nao fecha, a linha volta marcada para revisao.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------- imagem

type qualidadeFoto struct {
	Largura     int     `json:"largura"`
	Altura      int     `json:"altura"`
	Nitidez     float64 `json:"nitidez"`
	Contraste   float64 `json:"contraste"`
	Cobertura   float64 `json:"cobertura"`
	Inclinacao  float64 `json:"inclinacao_graus"`
	AlturaTexto int     `json:"altura_texto_px"`
	Recortada   bool    `json:"recortada"`
}

// decodificarImagem aceita JPEG e PNG (o que sai de celular e de scanner).
func decodificarImagem(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err == nil {
		return img, nil
	}
	// segunda tentativa explicita (alguns JPEG vem com extensao trocada)
	if img, err2 := jpeg.Decode(bytes.NewReader(data)); err2 == nil {
		return img, nil
	}
	if img, err2 := png.Decode(bytes.NewReader(data)); err2 == nil {
		return img, nil
	}
	return nil, fmt.Errorf("nao consegui abrir a imagem (%v)", err)
}

// cinzaReduzido converte para tons de cinza reduzindo a largura maxima.
func cinzaReduzido(img image.Image, maxLargura int) *image.Gray {
	b := img.Bounds()
	larg, alt := b.Dx(), b.Dy()
	if larg <= 0 || alt <= 0 {
		return image.NewGray(image.Rect(0, 0, 1, 1))
	}
	escala := 1.0
	if larg > maxLargura {
		escala = float64(maxLargura) / float64(larg)
	}
	nLarg := int(float64(larg) * escala)
	nAlt := int(float64(alt) * escala)
	if nLarg < 1 {
		nLarg = 1
	}
	if nAlt < 1 {
		nAlt = 1
	}
	saida := image.NewGray(image.Rect(0, 0, nLarg, nAlt))
	luminancia := func(x, y int) float64 {
		r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
		// luminancia ITU-R BT.601
		return 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
	}
	if escala == 1.0 {
		for y := 0; y < nAlt; y++ {
			for x := 0; x < nLarg; x++ {
				saida.Pix[y*saida.Stride+x] = uint8(luminancia(x, y) + 0.5)
			}
		}
		return saida
	}
	// media da area de origem (reducao de boa qualidade, sem serrilhado)
	for y := 0; y < nAlt; y++ {
		y0 := int(float64(y) / escala)
		y1 := int(float64(y+1)/escala) + 1
		if y1 > alt {
			y1 = alt
		}
		for x := 0; x < nLarg; x++ {
			x0 := int(float64(x) / escala)
			x1 := int(float64(x+1)/escala) + 1
			if x1 > larg {
				x1 = larg
			}
			soma, n := 0.0, 0
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					soma += luminancia(xx, yy)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			saida.Pix[y*saida.Stride+x] = uint8(soma/float64(n) + 0.5)
		}
	}
	return saida
}

// limiarOtsu calcula o limiar que melhor separa papel de tinta.
func limiarOtsu(g *image.Gray) uint8 {
	var hist [256]int
	total := 0
	for y := 0; y < g.Rect.Dy(); y++ {
		linha := g.Pix[y*g.Stride : y*g.Stride+g.Rect.Dx()]
		for _, v := range linha {
			hist[v]++
			total++
		}
	}
	if total == 0 {
		return 128
	}
	var somaTotal float64
	for i, h := range hist {
		somaTotal += float64(i) * float64(h)
	}
	somaFundo, pesoFundo := 0.0, 0
	melhor, limiar := -1.0, uint8(128)
	for i := 0; i < 256; i++ {
		pesoFundo += hist[i]
		if pesoFundo == 0 {
			continue
		}
		pesoFrente := total - pesoFundo
		if pesoFrente == 0 {
			break
		}
		somaFundo += float64(i) * float64(hist[i])
		mediaFundo := somaFundo / float64(pesoFundo)
		mediaFrente := (somaTotal - somaFundo) / float64(pesoFrente)
		d := mediaFundo - mediaFrente
		if v := float64(pesoFundo) * float64(pesoFrente) * d * d; v > melhor {
			melhor, limiar = v, uint8(i)
		}
	}
	return limiar
}

// normalizarFundo compensa sombra e iluminacao desigual: divide cada pixel pela
// media da vizinhanca (fundo estimado). Sem isso, a sombra de um lado da nota
// apaga digitos e o OCR troca numero (0 vira 8, 0 vira 9...).
func normalizarFundo(g *image.Gray) *image.Gray { return normalizarFundoRaio(g, 0) }

func normalizarFundoRaio(g *image.Gray, raioFixo int) *image.Gray {
	larg, alt := g.Rect.Dx(), g.Rect.Dy()
	if larg < 8 || alt < 8 {
		return g
	}
	// imagem integral para calcular a media da caixa em tempo constante
	integral := make([]uint64, (larg+1)*(alt+1))
	for y := 0; y < alt; y++ {
		var soma uint64
		for x := 0; x < larg; x++ {
			soma += uint64(g.Pix[y*g.Stride+x])
			integral[(y+1)*(larg+1)+x+1] = integral[y*(larg+1)+x+1] + soma
		}
	}
	mediaCaixa := func(x0, y0, x1, y1 int) uint64 {
		if x0 < 0 {
			x0 = 0
		}
		if y0 < 0 {
			y0 = 0
		}
		if x1 > larg {
			x1 = larg
		}
		if y1 > alt {
			y1 = alt
		}
		if x1 <= x0 || y1 <= y0 {
			return 0
		}
		a := integral[y0*(larg+1)+x0]
		b := integral[y0*(larg+1)+x1]
		c := integral[y1*(larg+1)+x0]
		d := integral[y1*(larg+1)+x1]
		n := uint64((x1 - x0) * (y1 - y0))
		return (d - b - c + a) / n
	}
	raio := raioFixo
	if raio <= 0 {
		raio = alt / 20
	}
	if raio < 12 {
		raio = 12
	}
	saida := image.NewGray(g.Rect)
	for y := 0; y < alt; y++ {
		for x := 0; x < larg; x++ {
			v := float64(g.Pix[y*g.Stride+x])
			fundo := float64(mediaCaixa(x-raio, y-raio, x+raio, y+raio))
			if fundo < 1 {
				fundo = 1
			}
			// tinta escura sobre papel claro: realca o que esta abaixo do fundo
			ajustado := 255 * (v / fundo)
			if ajustado > 255 {
				ajustado = 255
			}
			saida.Pix[y*saida.Stride+x] = uint8(ajustado + 0.5)
		}
	}
	return saida
}

// binarizar devolve pixels 0 (tinta) e 255 (papel).
func binarizar(g *image.Gray, limiar uint8) *image.Gray {
	saida := image.NewGray(g.Rect)
	for i, v := range g.Pix {
		if v <= limiar {
			saida.Pix[i] = 0
		} else {
			saida.Pix[i] = 255
		}
	}
	return saida
}

// inclinarGira rotaciona a imagem em torno do centro (vizinho mais proximo).
func inclinarGira(g *image.Gray, graus float64) *image.Gray {
	if math.Abs(graus) < 0.01 {
		return g
	}
	larg, alt := g.Rect.Dx(), g.Rect.Dy()
	saida := image.NewGray(image.Rect(0, 0, larg, alt))
	rad := graus * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	cx, cy := float64(larg)/2, float64(alt)/2
	for y := 0; y < alt; y++ {
		for x := 0; x < larg; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			ox := cos*dx + sin*dy + cx
			oy := -sin*dx + cos*dy + cy
			ix, iy := int(ox+0.5), int(oy+0.5)
			var v uint8 = 255
			if ix >= 0 && iy >= 0 && ix < larg && iy < alt {
				v = g.Pix[iy*g.Stride+ix]
			}
			saida.Pix[y*saida.Stride+x] = v
		}
	}
	return saida
}

// nitidezProjecao mede o quanto as linhas de texto ficam alinhadas.
// Quanto maior, mais reta esta a pagina naquele angulo.
func nitidezProjecao(bin *image.Gray) float64 {
	larg, alt := bin.Rect.Dx(), bin.Rect.Dy()
	score := 0.0
	for y := 0; y < alt; y++ {
		escuros := 0
		linha := bin.Pix[y*bin.Stride : y*bin.Stride+larg]
		for _, v := range linha {
			if v == 0 {
				escuros++
			}
		}
		score += float64(escuros * escuros)
	}
	return score
}

// estimarInclinacao procura o angulo entre -8 e +8 graus que deixa as linhas
// mais alinhadas. Nota tirada de lado e a causa mais comum de OCR ruim.
func estimarInclinacao(g *image.Gray) float64 {
	pequena := cinzaReduzido(g, 700)
	bin := binarizar(pequena, limiarOtsu(pequena))
	melhorAngulo, melhor := 0.0, -1.0
	for a := -8.0; a <= 8.0001; a += 0.5 {
		score := nitidezProjecao(inclinarGira(bin, a))
		if score > melhor {
			melhor, melhorAngulo = score, a
		}
	}
	// refino em passos menores em volta do melhor angulo
	for a := melhorAngulo - 0.5; a <= melhorAngulo+0.5001; a += 0.1 {
		score := nitidezProjecao(inclinarGira(bin, a))
		if score > melhor {
			melhor, melhorAngulo = score, a
		}
	}
	return melhorAngulo
}

// recortarFolha acha a folha de papel (a maior regiao clara) e corta o resto:
// mesa, sombra, dedo, fundo da bancada. Sem isso o OCR le a mesa como se fosse
// texto e perde as ultimas linhas da nota.
func recortarFolha(g *image.Gray) (image.Rectangle, bool) {
	larg, alt := g.Rect.Dx(), g.Rect.Dy()
	if larg < 80 || alt < 80 {
		return image.Rect(0, 0, larg, alt), false
	}
	// mascara em escala reduzida (rapido) e depois projecao para o tamanho real
	escala := 4
	if maior := larg / 300; maior > escala {
		escala = maior
	}
	pl, pa := larg/escala, alt/escala
	if pl < 20 || pa < 20 {
		return image.Rect(0, 0, larg, alt), false
	}
	mascara := make([]bool, pl*pa)
	var soma uint64
	for y := 0; y < pa; y++ {
		for x := 0; x < pl; x++ {
			v := g.Pix[(y*escala)*g.Stride+x*escala]
			mascara[y*pl+x] = true
			soma += uint64(v)
		}
	}
	// limiar por Otsu na imagem reduzida: separa papel (claro) de mesa/sombra
	// sem depender do brilho absoluto. Um limiar fixo cortava a parte da folha
	// que estava na sombra e a nota perdia as colunas da direita.
	reduzida := image.NewGray(image.Rect(0, 0, pl, pa))
	for y := 0; y < pa; y++ {
		for x := 0; x < pl; x++ {
			reduzida.Pix[y*reduzida.Stride+x] = g.Pix[(y*escala)*g.Stride+x*escala]
		}
	}
	limiar := limiarOtsu(reduzida)
	if limiar < 60 {
		limiar = 60
	}
	_ = soma
	claro := make([]bool, pl*pa)
	for y := 0; y < pa; y++ {
		for x := 0; x < pl; x++ {
			claro[y*pl+x] = reduzida.Pix[y*reduzida.Stride+x] >= limiar
		}
	}
	// maior componente claro (vizinhanca de 4)
	visto := make([]bool, pl*pa)
	melhorArea, melhor := 0, image.Rect(0, 0, 0, 0)
	fila := make([]int, 0, 1024)
	for i := range claro {
		if !claro[i] || visto[i] {
			continue
		}
		visto[i] = true
		fila = append(fila[:0], i)
		area := 0
		minX, minY, maxX, maxY := pl, pa, -1, -1
		for len(fila) > 0 {
			pos := fila[len(fila)-1]
			fila = fila[:len(fila)-1]
			x, y := pos%pl, pos/pl
			area++
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
			vizinhos := [4]int{pos - 1, pos + 1, pos - pl, pos + pl}
			if x == 0 {
				vizinhos[0] = -1
			}
			if x == pl-1 {
				vizinhos[1] = -1
			}
			if y == 0 {
				vizinhos[2] = -1
			}
			if y == pa-1 {
				vizinhos[3] = -1
			}
			for _, v := range vizinhos {
				if v < 0 || v >= len(claro) || visto[v] || !claro[v] {
					continue
				}
				visto[v] = true
				fila = append(fila, v)
			}
		}
		if area > melhorArea {
			melhorArea = area
			melhor = image.Rect(minX, minY, maxX+1, maxY+1)
		}
	}
	fracao := float64(melhorArea) / float64(pl*pa)
	if fracao < 0.15 || fracao > 0.995 {
		return image.Rect(0, 0, larg, alt), false
	}
	// devolve em coordenadas da imagem real, com folga proporcional: o texto
	// pode encostar na borda da folha e nao pode ser cortado
	folga := larg / 60
	if folga < 8 {
		folga = 8
	}
	r := image.Rect(melhor.Min.X*escala-folga, melhor.Min.Y*escala-folga,
		melhor.Max.X*escala+folga, melhor.Max.Y*escala+folga).Intersect(image.Rect(0, 0, larg, alt))
	if r.Dx() < larg/4 || r.Dy() < alt/4 {
		return image.Rect(0, 0, larg, alt), false
	}
	return r, true
}

// recortarConteudo corta as bordas vazias (mesa, fundo, margem do scanner).
func recortarConteudo(bin *image.Gray) (image.Rectangle, bool) {
	larg, alt := bin.Rect.Dx(), bin.Rect.Dy()
	minX, minY, maxX, maxY := larg, alt, -1, -1
	for y := 0; y < alt; y++ {
		for x := 0; x < larg; x++ {
			if bin.Pix[y*bin.Stride+x] == 0 {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < 0 || maxY < 0 {
		return image.Rect(0, 0, larg, alt), false
	}
	m := 12
	r := image.Rect(minX-m, minY-m, maxX+m+1, maxY+m+1).Intersect(image.Rect(0, 0, larg, alt))
	if r.Dx() < 40 || r.Dy() < 40 {
		return image.Rect(0, 0, larg, alt), false
	}
	return r, true
}

// metricas mede o que da para medir sem OCR (serve para avisar o usuario).
func metricas(g *image.Gray) (nitidez, contraste, cobertura float64) {
	larg, alt := g.Rect.Dx(), g.Rect.Dy()
	soma, soma2 := 0.0, 0.0
	n := float64(larg * alt)
	for y := 0; y < alt; y++ {
		for x := 0; x < larg; x++ {
			v := float64(g.Pix[y*g.Stride+x])
			soma += v
			soma2 += v * v
		}
	}
	media := soma / n
	contraste = math.Sqrt(math.Max(0, soma2/n-media*media))
	escuros := 0
	lap, nLap := 0.0, 0.0
	for y := 1; y < alt-1; y++ {
		for x := 1; x < larg-1; x++ {
			v := float64(g.Pix[y*g.Stride+x])
			if v < 128 {
				escuros++
			}
			l := 4*v -
				float64(g.Pix[(y-1)*g.Stride+x]) - float64(g.Pix[(y+1)*g.Stride+x]) -
				float64(g.Pix[y*g.Stride+x-1]) - float64(g.Pix[y*g.Stride+x+1])
			lap += l * l
			nLap++
		}
	}
	if n > 0 {
		cobertura = float64(escuros) / n
	}
	if nLap > 0 {
		nitidez = lap / nLap
	}
	return
}

// prepararNotaFoto devolve a imagem pronta para o OCR e o diagnostico da foto.
func prepararNotaFoto(data []byte) ([]byte, qualidadeFoto, error) {
	img, err := decodificarImagem(data)
	if err != nil {
		return nil, qualidadeFoto{}, err
	}
	cinza := cinzaReduzido(img, 2000)
	qual := qualidadeFoto{Largura: cinza.Rect.Dx(), Altura: cinza.Rect.Dy()}
	// 1) acha a folha na imagem como ela veio (mesa = escuro, papel = claro) e
	//    descarta o resto. A deteccao tem que ser feita ANTES de igualar a luz:
	//    igualando primeiro, a mesa tambem fica clara e o recorte nao acha a folha.
	reta := cinza
	if r, ok := recortarFolha(reta); ok {
		reta = subImagem(reta, r)
		qual.Recortada = true
	}
	// 2) iguala a luz dentro da folha (sombra de um lado apaga digitos)
	reta = normalizarFundo(reta)
	// 3) corrige a inclinacao: nota fotografada de lado e a causa mais comum de erro
	qual.Inclinacao = estimarInclinacao(reta)
	reta = inclinarGira(reta, qual.Inclinacao)
	// 4) corta as bordas vazias que o giro criou
	if r, ok := recortarConteudo(binarizar(reta, limiarOtsu(reta))); ok {
		reta = subImagem(reta, r)
	}
	qual.Nitidez, qual.Contraste, qual.Cobertura = metricas(reta)
	var buf bytes.Buffer
	if err := png.Encode(&buf, reta); err != nil {
		return nil, qual, fmt.Errorf("nao consegui preparar a imagem: %w", err)
	}
	return buf.Bytes(), qual, nil
}

func subImagem(g *image.Gray, r image.Rectangle) *image.Gray {
	saida := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		copy(saida.Pix[y*saida.Stride:(y+1)*saida.Stride],
			g.Pix[(y+r.Min.Y)*g.Stride+r.Min.X:(y+r.Min.Y)*g.Stride+r.Min.X+r.Dx()])
	}
	return saida
}

// ---------------------------------------------------------------- OCR -> linhas

type palavraOCR struct {
	Txt   string
	Conf  float64
	X, Y  int
	L, A  int
	Bloco int // nivel 2 do TSV
	Par   int // nivel 3
	Linha int // nivel 4: o proprio OCR diz a que linha a palavra pertence
}

type linhaNota struct {
	Y        int
	Altura   int
	Palavras []palavraOCR
	Txt      string
}

// lerPalavrasTSV interpreta a saida "tsv" do Tesseract.
func lerPalavrasTSV(tsv string) []palavraOCR {
	var out []palavraOCR
	linhas := strings.Split(strings.ReplaceAll(tsv, "\r\n", "\n"), "\n")
	for i, l := range linhas {
		if i == 0 || strings.TrimSpace(l) == "" {
			continue
		}
		campos := strings.Split(l, "\t")
		if len(campos) < 12 {
			continue
		}
		txt := strings.TrimSpace(campos[11])
		if txt == "" {
			continue
		}
		conf, err := strconv.ParseFloat(campos[10], 64)
		if err != nil {
			conf = 0
		}
		x, _ := strconv.Atoi(campos[6])
		y, _ := strconv.Atoi(campos[7])
		larg, _ := strconv.Atoi(campos[8])
		alt, _ := strconv.Atoi(campos[9])
		bloco, _ := strconv.Atoi(campos[2])
		par, _ := strconv.Atoi(campos[3])
		linha, _ := strconv.Atoi(campos[4])
		out = append(out, palavraOCR{Txt: txt, Conf: conf, X: x, Y: y, L: larg, A: alt,
			Bloco: bloco, Par: par, Linha: linha})
	}
	return out
}

// agruparLinhas monta as linhas de texto. Usa a estrutura que o proprio OCR
// devolve (bloco/paragrafo/linha): adivinhar pela coordenada vertical fundia
// linhas vizinhas quando a foto estava um pouco torta. Quando o TSV nao traz
// essa estrutura (tudo zero), cai para o agrupamento por Y.
func agruparLinhas(ps []palavraOCR) []linhaNota {
	if len(ps) == 0 {
		return nil
	}
	temEstrutura := false
	for _, p := range ps {
		if p.Linha > 0 || p.Bloco > 0 {
			temEstrutura = true
			break
		}
	}
	var linhas []linhaNota
	if temEstrutura {
		indice := map[[3]int]int{}
		for _, p := range ps {
			chave := [3]int{p.Bloco, p.Par, p.Linha}
			if i, ok := indice[chave]; ok {
				linhas[i].Palavras = append(linhas[i].Palavras, p)
				continue
			}
			indice[chave] = len(linhas)
			linhas = append(linhas, linhaNota{Palavras: []palavraOCR{p}})
		}
	} else {
		ordenadas := append([]palavraOCR(nil), ps...)
		sort.Slice(ordenadas, func(i, j int) bool {
			if ordenadas[i].Y == ordenadas[j].Y {
				return ordenadas[i].X < ordenadas[j].X
			}
			return ordenadas[i].Y < ordenadas[j].Y
		})
		for _, p := range ordenadas {
			colocada := false
			for i := range linhas {
				if abs(linhas[i].Y-(p.Y+p.A/2)) <= 12 {
					linhas[i].Palavras = append(linhas[i].Palavras, p)
					colocada = true
					break
				}
			}
			if !colocada {
				linhas = append(linhas, linhaNota{Y: p.Y + p.A/2, Palavras: []palavraOCR{p}})
			}
		}
	}
	for i := range linhas {
		sort.Slice(linhas[i].Palavras, func(a, b int) bool { return linhas[i].Palavras[a].X < linhas[i].Palavras[b].X })
		var partes []string
		topo, base, maxA := 1<<30, 0, 0
		for _, p := range linhas[i].Palavras {
			partes = append(partes, p.Txt)
			if p.Y < topo {
				topo = p.Y
			}
			if p.Y+p.A > base {
				base = p.Y + p.A
			}
			if p.A > maxA {
				maxA = p.A
			}
		}
		linhas[i].Txt = strings.Join(partes, " ")
		linhas[i].Altura = maxA
		linhas[i].Y = (topo + base) / 2
	}
	sort.Slice(linhas, func(i, j int) bool { return linhas[i].Y < linhas[j].Y })
	return linhas
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// ---------------------------------------------------------------- parser da nota

type NotaItem struct {
	Codigo     string   `json:"codigo"`
	Descricao  string   `json:"descricao"`
	Qtd        float64  `json:"quantidade"`
	Unitario   int64    `json:"unitario_centavos"`
	Total      int64    `json:"total_centavos"`
	Confianca  int      `json:"confianca"`
	Conferido  bool     `json:"conferido"`
	Avisos     []string `json:"avisos,omitempty"`
	LinhaTexto string   `json:"linha_texto"`
}

var (
	reMoedaBR   = regexp.MustCompile(`^\d{1,3}(?:\.\d{3})*,\d{2}$|^\d+,\d{2}$|^\d{1,3}(?:\.\d{3})+$|^\d+\.\d{2}$`)
	reInteiro   = regexp.MustCompile(`^\d{1,3}$`)
	reCodigo    = regexp.MustCompile(`^[A-Z]{1,6}-?\d{2,14}$`)
	reCodigoNum = regexp.MustCompile(`^\d{4,14}$`)
	reSoLetras  = regexp.MustCompile(`^[A-Za-zÁÂÃÀÉÊÍÓÔÕÚÇáâãàéêíóôõúç]{2,}$`)
)

// fragmentos que sobram quando o codigo vem separado do prefixo ("SKU-" "1001")
var lixoDeCodigo = map[string]bool{
	"SKU": true, "SKU-": true, "-": true, "/": true, "REF": true, "REF-": true,
	"COD": true, "COD-": true, "EAN": true,
}

// linhas que nunca sao item: dados do emitente, titulo da tabela ou data
var termosDeCabecalhoNota = []string{
	"CNPJ", "CPF", "DANFE", "NF-E", "NFE", "SERIE", "CHAVE", "PROTOCOLO",
	"INSCRICAO", "INSCRIÇÃO", "TELEFONE", "ENDEREÇO", "ENDERECO", "BAIRRO",
	"CEP", "MUNICIPIO", "MUNICÍPIO", "UF", "FORNECEDOR", "EMITENTE",
	"DESTINATARIO", "DESTINATÁRIO", "VALOR TOTAL", "BASE DE CALCULO",
	"BASE DE CÁLCULO", "ICMS", "IPI", "FRETE", "DESCONTO", "ASSINATURA",
}

func ehLinhaDeCabecalho(txt string) bool {
	t := strings.ToUpper(txt)
	if strings.Contains(t, "DESCRICAO") && (strings.Contains(t, "QTD") || strings.Contains(t, "UNIT")) {
		return true
	}
	if strings.Contains(t, "UNIT") && strings.Contains(t, "TOTAL") {
		return true
	}
	for _, termo := range termosDeCabecalhoNota {
		if strings.Contains(t, termo) {
			return true
		}
	}
	// data solta na linha (12/11/2026) tambem nao e item
	if reData.MatchString(t) {
		return true
	}
	return false
}

var reData = regexp.MustCompile(`\d{2}/\d{2}/\d{2,4}`)

var palavrasDeCabecalho = map[string]bool{
	"CODIGO": true, "COD": true, "DESCRICAO": true, "DESCRIÇÃO": true, "PRODUTO": true,
	"QTD": true, "QTDE": true, "QUANT": true, "QUANTIDADE": true, "UNIT": true,
	"UNITARIO": true, "UNITÁRIO": true, "TOTAL": true, "VALOR": true, "VL": true,
	"ITEM": true, "SEQ": true, "R$": true, "RS": true, "NOTA": true, "DANFE": true,
}

// numeroLido guarda um token numerico da linha e o que ele pode ser.
type numeroLido struct {
	Txt   string
	Cent  int64 // valor em centavos quando for dinheiro
	Qtd   int   // valor inteiro 1..999
	Moeda bool
}

func limparToken(t string) string {
	t = strings.TrimSpace(t)
	t = strings.Trim(t, "|;:()[]{}<>\"'")
	t = strings.ReplaceAll(t, "R$", "")
	return strings.TrimSpace(t)
}

func classificarToken(t string) (numeroLido, bool) {
	c := limparToken(t)
	if c == "" {
		return numeroLido{}, false
	}
	if reMoedaBR.MatchString(c) {
		return numeroLido{Txt: c, Cent: parseMoneyBR(c), Moeda: true}, true
	}
	if reInteiro.MatchString(c) {
		v, _ := strconv.Atoi(c)
		if v >= 0 && v <= 999 {
			return numeroLido{Txt: c, Qtd: v, Cent: int64(v) * 100}, true
		}
	}
	return numeroLido{}, false
}

// triplo procura (qtd, unitario, total) com qtd*unitario == total.
func triplo(nums []numeroLido) (qtd int, unit, total int64, ok bool) {
	bestDif := int64(1 << 62)
	for i := range nums {
		if nums[i].Moeda || nums[i].Qtd <= 0 {
			continue
		}
		q := nums[i].Qtd
		for j := range nums {
			if j == i || !nums[j].Moeda || nums[j].Cent <= 0 {
				continue
			}
			for k := range nums {
				if k == j || k == i || !nums[k].Moeda || nums[k].Cent <= 0 {
					continue
				}
				dif := int64(q)*nums[j].Cent - nums[k].Cent
				if dif < 0 {
					dif = -dif
				}
				if dif <= 2 && j < k && dif < bestDif {
					bestDif = dif
					qtd, unit, total, ok = q, nums[j].Cent, nums[k].Cent, true
				}
			}
		}
	}
	return
}

// triploPelaDivisao tenta recuperar a quantidade quando o OCR perdeu o numero.
func triploPelaDivisao(nums []numeroLido) (qtd int, unit, total int64, ok bool) {
	var moedas []numeroLido
	for _, n := range nums {
		if n.Moeda && n.Cent > 0 {
			moedas = append(moedas, n)
		}
	}
	for j := 0; j < len(moedas)-1; j++ {
		for k := j + 1; k < len(moedas); k++ {
			u, t := moedas[j].Cent, moedas[k].Cent
			if u <= 0 || t < u {
				continue
			}
			q := int(math.Round(float64(t) / float64(u)))
			if q >= 1 && q <= 999 {
				dif := int64(q)*u - t
				if dif < 0 {
					dif = -dif
				}
				if dif <= 2 {
					return q, u, t, true
				}
			}
		}
	}
	return 0, 0, 0, false
}

// prefixos que o OCR costuma separar do numero do codigo ("SKU-" "1001")
var prefixosDeCodigo = map[string]bool{
	"SKU": true, "REF": true, "COD": true, "EAN": true, "PROD": true, "ITEM": true,
}

func codigoDaLinha(palavras []palavraOCR) string {
	for i, p := range palavras {
		c := strings.ToUpper(limparToken(p.Txt))
		semTraco := strings.TrimSuffix(c, "-")
		if prefixosDeCodigo[semTraco] {
			// o OCR pode separar o prefixo por traco/espaco: "SKU - 1004"
			for j := i + 1; j < len(palavras) && j <= i+2; j++ {
				prox := strings.ToUpper(limparToken(palavras[j].Txt))
				if prox == "" || prox == "-" || prox == "/" {
					continue
				}
				if reCodigoNum.MatchString(prox) || reCodigo.MatchString(prox) {
					return semTraco + "-" + prox
				}
				break
			}
		}
		if len(c) < 3 {
			continue
		}
		if reCodigo.MatchString(c) || reCodigoNum.MatchString(c) {
			if palavrasDeCabecalho[c] || prefixosDeCodigo[c] {
				continue
			}
			return c
		}
	}
	return ""
}

func descricaoDaLinha(palavras []palavraOCR, codigo string) string {
	var partes []string
	for _, p := range palavras {
		c := limparToken(p.Txt)
		maiuscula := strings.ToUpper(c)
		if c == "" || palavrasDeCabecalho[maiuscula] || strings.EqualFold(c, codigo) {
			continue
		}
		if _, ok := classificarToken(c); ok {
			continue
		}
		if lixoDeCodigo[maiuscula] {
			continue
		}
		if reSoLetras.MatchString(c) || (len(c) > 2 && !reInteiro.MatchString(c)) {
			partes = append(partes, c)
		}
	}
	return strings.Join(partes, " ")
}

func confiancaDaLinha(palavras []palavraOCR) int {
	if len(palavras) == 0 {
		return 0
	}
	soma := 0.0
	for _, p := range palavras {
		soma += p.Conf
	}
	v := soma / float64(len(palavras))
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return int(v)
}

// ehLinhaDeTotal reconhece a linha do total da nota.
func ehLinhaDeTotal(txt string) bool {
	t := strings.ToUpper(txt)
	if !strings.Contains(t, "TOTAL") {
		return false
	}
	if strings.Contains(t, "UNIT") || strings.Contains(t, "DESCRICAO") {
		return false
	}
	return true
}

// parsearNota transforma as linhas do OCR em itens de compra.
func parsearNota(linhas []linhaNota) (itens []NotaItem, totalNota int64, avisos []string) {
	vistos := map[string]bool{}
	for _, l := range linhas {
		txt := strings.TrimSpace(l.Txt)
		if txt == "" {
			continue
		}
		if ehLinhaDeCabecalho(txt) {
			continue
		}
		if ehLinhaDeTotal(txt) {
			for i := len(l.Palavras) - 1; i >= 0; i-- {
				if n, ok := classificarToken(l.Palavras[i].Txt); ok && n.Moeda {
					totalNota = n.Cent
					break
				}
			}
			continue
		}
		var nums []numeroLido
		for _, p := range l.Palavras {
			if n, ok := classificarToken(p.Txt); ok {
				nums = append(nums, n)
			}
		}
		if len(nums) < 2 {
			continue
		}
		codigo := codigoDaLinha(l.Palavras)
		desc := descricaoDaLinha(l.Palavras, codigo)
		if codigo == "" && desc == "" {
			continue
		}
		// a linha precisa ter alguma letra: evita linhas de codigo de barras/numeros soltos
		if desc == "" && codigo == "" {
			continue
		}
		qtd, unit, total, ok := triplo(nums)
		var aviso []string
		if !ok {
			if q, u, t, ok2 := triploPelaDivisao(nums); ok2 {
				qtd, unit, total, ok = q, u, t, true
				aviso = append(aviso, "quantidade calculada pela divisao do total")
			}
		}
		if !ok {
			// palpite: menor inteiro como quantidade e os dois ultimos valores como unitario/total
			var ints []numeroLido
			var moedas []numeroLido
			for _, n := range nums {
				if n.Moeda {
					moedas = append(moedas, n)
				} else if n.Qtd > 0 {
					ints = append(ints, n)
				}
			}
			if len(moedas) >= 2 {
				unit = moedas[len(moedas)-2].Cent
				total = moedas[len(moedas)-1].Cent
			} else if len(moedas) == 1 {
				total = moedas[0].Cent
				unit = moedas[0].Cent
			}
			if len(ints) > 0 {
				qtd = ints[0].Qtd
			}
			if qtd == 0 {
				qtd = 1
			}
			aviso = append(aviso, "a conta quantidade x unitario nao fecha com o total")
		}
		chave := codigo + "|" + strconv.FormatInt(total, 10)
		if codigo != "" && vistos[chave] {
			avisos = append(avisos, "linha repetida ignorada: "+strings.TrimSpace(txt))
			continue
		}
		if codigo != "" {
			vistos[chave] = true
		}
		conf := confiancaDaLinha(l.Palavras)
		if !ok {
			aviso = append(aviso, "revisar")
		}
		itens = append(itens, NotaItem{
			Codigo: codigo, Descricao: strings.TrimSpace(desc), Qtd: float64(qtd),
			Unitario: unit, Total: total, Confianca: conf, Conferido: ok,
			Avisos: aviso, LinhaTexto: strings.TrimSpace(txt),
		})
	}
	return itens, totalNota, avisos
}

// ResultadoNota e o que a tela recebe depois de ler a foto.
type ResultadoNota struct {
	Ok        bool          `json:"ok"`
	Veredito  string        `json:"veredito"` // ok | revisar | ilegivel
	Itens     []NotaItem    `json:"itens"`
	TotalNota int64         `json:"total_nota_centavos"`
	SomaItens int64         `json:"soma_itens_centavos"`
	Diferenca int64         `json:"diferenca_centavos"`
	Conferido bool          `json:"conferido"`
	Motor     string        `json:"motor"`
	TempoMS   int64         `json:"tempo_ms"`
	Qualidade qualidadeFoto `json:"qualidade"`
	Avisos    []string      `json:"avisos"`
	TextoOCR  string        `json:"texto_ocr"`
	Mensagem  string        `json:"mensagem,omitempty"`
}

// analisarNotaFoto junta tudo: imagem -> OCR -> itens + veredito.
func analisarNotaFoto(data []byte, tsv string, qual qualidadeFoto, motor string, tempoMS int64) ResultadoNota {
	res := ResultadoNota{Ok: true, Motor: motor, TempoMS: tempoMS, Qualidade: qual, Avisos: []string{}}
	linhas := agruparLinhas(lerPalavrasTSV(tsv))
	var textos []string
	for _, l := range linhas {
		textos = append(textos, l.Txt)
	}
	res.TextoOCR = strings.Join(textos, "\n")
	itens, totalNota, avisos := parsearNota(linhas)
	res.Itens = itens
	res.TotalNota = totalNota
	res.Avisos = append(res.Avisos, avisos...)
	for _, it := range itens {
		res.SomaItens += it.Total
	}
	if len(itens) == 0 {
		res.Veredito = "ilegivel"
		res.Conferido = false
		res.Mensagem = "Não consegui ler nenhum item nesta foto. Apoie o celular, enquadre a folha inteira e use bastante luz."
		return res
	}
	if totalNota > 0 {
		res.Diferenca = res.SomaItens - totalNota
		if res.Diferenca < 0 {
			res.Diferenca = -res.Diferenca
		}
		res.Conferido = res.Diferenca <= 2
	}
	conferidos := 0
	for _, it := range itens {
		if it.Conferido {
			conferidos++
		}
	}
	switch {
	case res.Conferido && conferidos == len(itens):
		res.Veredito = "ok"
	case totalNota > 0 && !res.Conferido:
		res.Veredito = "revisar"
		res.Mensagem = fmt.Sprintf("A soma dos itens (%s) não fecha com o total da nota (%s): diferença de %s. Confira as linhas amarelas antes de importar.",
			fmtMoney(res.SomaItens), fmtMoney(totalNota), fmtMoney(res.Diferenca))
	case conferidos < len(itens):
		res.Veredito = "revisar"
		res.Mensagem = fmt.Sprintf("%d de %d linhas precisam de conferência (marcadas em amarelo).", len(itens)-conferidos, len(itens))
	default:
		res.Veredito = "ok"
	}
	if qual.AlturaTexto > 0 && qual.AlturaTexto < 14 {
		res.Avisos = append(res.Avisos, "o texto saiu pequeno na foto: aproxime o celular da nota")
	}
	return res
}

// imagemParaOCR e usado pelos testes e pela API.
var _ = color.Gray{}

// ---------------------------------------------------------------- total da nota

// lerTotalSegundaPassada tenta de novo SÓ a linha do total. A fonte do total e
// maior que a do resto da nota e muitas vezes o OCR le "R$ 932,20" como "R$ £";
// recortar o pedaco, ampliar e pedir apenas digitos resolve. O valor escolhido e
// o mais proximo da soma dos itens, que serve de conferencia.
func lerTotalSegundaPassada(preparada []byte, tsv string, somaItens int64) int64 {
	img, err := decodificarImagem(preparada)
	if err != nil {
		return 0
	}
	cinza := cinzaReduzido(img, 4000)
	linhas := agruparLinhas(lerPalavrasTSV(tsv))
	var alvo *linhaNota
	for i := range linhas {
		if strings.Contains(strings.ToUpper(linhas[i].Txt), "TOTAL") &&
			!strings.Contains(strings.ToUpper(linhas[i].Txt), "UNIT") {
			alvo = &linhas[i]
			break
		}
	}
	if alvo == nil {
		return 0
	}
	topo, base, direita := 1<<30, 0, 0
	for _, p := range alvo.Palavras {
		if p.Y < topo {
			topo = p.Y
		}
		if p.Y+p.A > base {
			base = p.Y + p.A
		}
		if strings.HasPrefix(strings.ToUpper(p.Txt), "R$") && p.X+p.L > direita {
			direita = p.X + p.L
		}
	}
	if direita == 0 { // sem "R$": pega da metade da pagina para a direita
		direita = cinza.Rect.Dx() / 2
	}
	alturaLinha := base - topo
	if alturaLinha <= 0 {
		return 0
	}
	r := image.Rect(direita+2, topo-alturaLinha/2, cinza.Rect.Dx(), base+alturaLinha/2).
		Intersect(cinza.Rect)
	if r.Dx() < 8 || r.Dy() < 6 {
		return 0
	}
	recorte := subImagem(cinza, r)
	ampliado := ampliar(recorte, 2)
	var buf bytes.Buffer
	if err := png.Encode(&buf, ampliado); err != nil {
		return 0
	}
	texto, err := rodarOCRComTexto(buf.Bytes(), "7", "0123456789.,")
	if err != nil {
		return 0
	}
	melhor, achou := int64(0), false
	for _, campo := range strings.Fields(texto) {
		campo = strings.TrimSpace(campo)
		if !reMoedaBR.MatchString(campo) && !reInteiro.MatchString(strings.TrimRight(campo, ".,")) {
			continue
		}
		v := parseMoneyBR(strings.TrimRight(campo, ".,"))
		if v <= 0 {
			continue
		}
		if somaItens > 0 {
			dif := v - somaItens
			if dif < 0 {
				dif = -dif
			}
			// aceita apenas valor proximo da soma: evita gravar numero absurdo
			if float64(dif) > 0.25*float64(somaItens)+500 {
				continue
			}
		}
		if !achou || (v-somaItens)*(v-somaItens) < (melhor-somaItens)*(melhor-somaItens) {
			melhor, achou = v, true
		}
	}
	if !achou {
		return 0
	}
	return melhor
}

// ampliar dobra (ou triplica) a imagem por interpolacao bilinear.
func ampliar(g *image.Gray, fator int) *image.Gray {
	if fator <= 1 {
		return g
	}
	larg, alt := g.Rect.Dx(), g.Rect.Dy()
	saida := image.NewGray(image.Rect(0, 0, larg*fator, alt*fator))
	for y := 0; y < saida.Rect.Dy(); y++ {
		fy := float64(y) / float64(fator)
		y0 := int(fy)
		y1 := y0 + 1
		if y1 >= alt {
			y1 = alt - 1
		}
		dy := fy - float64(y0)
		for x := 0; x < saida.Rect.Dx(); x++ {
			fx := float64(x) / float64(fator)
			x0 := int(fx)
			x1 := x0 + 1
			if x1 >= larg {
				x1 = larg - 1
			}
			dx := fx - float64(x0)
			v00 := float64(g.Pix[y0*g.Stride+x0])
			v10 := float64(g.Pix[y0*g.Stride+x1])
			v01 := float64(g.Pix[y1*g.Stride+x0])
			v11 := float64(g.Pix[y1*g.Stride+x1])
			v := (1-dx)*(1-dy)*v00 + dx*(1-dy)*v10 + (1-dx)*dy*v01 + dx*dy*v11
			saida.Pix[y*saida.Stride+x] = uint8(v + 0.5)
		}
	}
	return saida
}

package main

// Gerador de PDF simples (sem dependencias externas).
// Usa fonte Helvetica/Courier padrao do PDF, papel A4, com paginacao.

import (
	"bytes"
	"fmt"
	"strings"
)

type pdfDoc struct {
	pages [][]string // cada pagina: linhas de comandos
	cur   *[]string
}

func newPDF() *pdfDoc {
	d := &pdfDoc{}
	d.newPage()
	return d
}

func (d *pdfDoc) newPage() {
	lines := []string{}
	d.pages = append(d.pages, lines)
	d.cur = &d.pages[len(d.pages)-1]
}

// escPDF prepara o texto para o PDF. As fontes sao declaradas com
// /WinAnsiEncoding (1 byte por caractere = Latin-1), entao a string UTF-8 do Go
// precisa ser convertida: escrever "ó" (0xC3 0xB3) fazia o leitor mostrar "Ã³".
func escPDF(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r < 32:
			b.WriteByte(' ')
		case r > 255:
			b.WriteByte('?')
		case r == '\\':
			b.WriteString(`\\`)
		case r == '(':
			b.WriteString(`\(`)
		case r == ')':
			b.WriteString(`\)`)
		default:
			b.WriteByte(byte(r)) // Latin-1 / WinAnsi
		}
	}
	return b.String()
}

// larguraTexto devolve a largura aproximada (em pontos) ja em caracteres
// (1 byte cada), usada para alinhar as colunas a direita.
func larguraTexto(s string) float64 { return float64(len(escPDF(s))) * 4.6 }

func (d *pdfDoc) text(x, y float64, size float64, font, s string) {
	*d.cur = append(*d.cur, fmt.Sprintf("BT /%s %.1f Tf %.2f %.2f Td (%s) Tj ET", font, size, x, y, escPDF(s)))
}

func (d *pdfDoc) line(x1, y1, x2, y2 float64) {
	*d.cur = append(*d.cur, fmt.Sprintf("%.2f %.2f m %.2f %.2f l S", x1, y1, x2, y2))
}

func (d *pdfDoc) rect(x, y, w, h float64, gray float64) {
	*d.cur = append(*d.cur, fmt.Sprintf("%.2f g %.2f %.2f %.2f %.2f re f 0 g", gray, x, y, w, h))
}

// Build monta o PDF. Conteudo: lista de linhas; colunas com largura em caracteres.
type pdfTable struct {
	Titulo      string
	Subtitulo   string
	Empresa     string
	Colunas     []string
	Larguras    []float64 // em pontos
	Linhas      [][]string
	Alinhamento []string // "L" ou "R" por coluna
	Resumo      []string
	Rodape      string
}

const pageW, pageH = 595.0, 842.0
const margem = 36.0

func (t pdfTable) Build() []byte {
	d := newPDF()
	y := pageH - margem
	headerY := y
	first := true
	rowH := 13.0
	fontSize := 8.0

	drawHeader := func() {
		y = pageH - margem
		d.text(margem, y, 13, "F2", t.Empresa)
		y -= 16
		d.text(margem, y, 11, "F2", t.Titulo)
		y -= 12
		if t.Subtitulo != "" {
			d.text(margem, y, 8.5, "F1", t.Subtitulo)
			y -= 12
		}
		d.line(margem, y+4, pageW-margem, y+4)
		y -= 8
		// cabecalho da tabela
		d.rect(margem, y-3, pageW-2*margem, rowH, 0.88)
		x := margem + 2
		for i, c := range t.Colunas {
			d.text(x, y, fontSize, "F2", c)
			if i < len(t.Larguras) {
				x += t.Larguras[i]
			}
		}
		y -= rowH
		d.line(margem, y+9, pageW-margem, y+9)
	}
	drawHeader()

	for _, row := range t.Linhas {
		if y < margem+40 {
			// rodape
			d.text(margem, margem+18, 7.5, "F1", t.Rodape)
			d.newPage()
			y = pageH - margem
			d.text(margem, y, 11, "F2", t.Titulo+" (continuacao)")
			y -= 16
			d.rect(margem, y-3, pageW-2*margem, rowH, 0.88)
			x := margem + 2
			for i, c := range t.Colunas {
				d.text(x, y, fontSize, "F2", c)
				if i < len(t.Larguras) {
					x += t.Larguras[i]
				}
			}
			y -= rowH
		}
		x := margem + 2
		for i, cell := range row {
			if i >= len(t.Larguras) {
				break
			}
			w := t.Larguras[i]
			cell = truncar(cell, int(w/4.6))
			al := "F1"
			align := "L"
			if i < len(t.Alinhamento) {
				align = t.Alinhamento[i]
			}
			if align == "R" {
				// alinhamento a direita aproximado (medido em caracteres do PDF)
				tx := x + w - larguraTexto(cell) - 4
				if tx < x {
					tx = x
				}
				d.text(tx, y, fontSize, al, cell)
			} else {
				d.text(x, y, fontSize, al, cell)
			}
			x += w
		}
		y -= rowH
	}
	// resumo
	if len(t.Resumo) > 0 {
		y -= 8
		if y < margem+70 {
			d.text(margem, margem+18, 7.5, "F1", t.Rodape)
			d.newPage()
			y = pageH - margem
		}
		d.line(margem, y+6, pageW-margem, y+6)
		for _, r := range t.Resumo {
			y -= 13
			d.text(margem, y, 9, "F2", r)
		}
	}
	if t.Rodape != "" {
		d.text(margem, margem+18, 7.5, "F1", t.Rodape)
	}
	_ = first
	_ = headerY
	return d.bytes()
}

func truncar(s string, n int) string {
	if n <= 3 {
		n = 3
	}
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "."
}

func (d *pdfDoc) bytes() []byte {
	n := len(d.pages)
	// Numeracao fixa dos objetos:
	// 1 = F1, 2 = F2, 3 = F3, 4 = Pages, 5..4+n = paginas, 5+n..4+2n = conteudos, 5+2n = Catalog
	pagesNum := 4
	firstPage := 5
	firstContent := 5 + n
	catalogNum := 5 + 2*n
	objs := make([]string, catalogNum)
	objs[0] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"
	objs[1] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>"
	objs[2] = "<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>"
	kids := []string{}
	for i := 0; i < n; i++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", firstPage+i))
	}
	objs[pagesNum-1] = fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", n, strings.Join(kids, " "))
	for i, p := range d.pages {
		content := strings.Join(p, "\n")
		objs[firstContent-1+i] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
		objs[firstPage-1+i] = fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 1 0 R /F2 2 0 R /F3 3 0 R >> >> /Contents %d 0 R >>",
			pagesNum, pageW, pageH, firstContent+i)
	}
	objs[catalogNum-1] = fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesNum)

	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, catalogNum+1)
	for i, body := range objs {
		offsets[i+1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xrefPos := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", catalogNum+1)
	b.WriteString("0000000000 65535 f \n")
	for i := 1; i <= catalogNum; i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", catalogNum+1, catalogNum, xrefPos)
	return b.Bytes()
}

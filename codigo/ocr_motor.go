package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// O motor de OCR vai embutido no proprio .exe: um pacote gzip com o Tesseract
// para Windows (tesseract.exe + DLLs enxutas + idioma portugues). Na primeira
// leitura de nota o pacote e extraido para a pasta de cache do usuario e o
// programa chama o tesseract.exe por linha de comando. Assim continua tudo
// offline e sem instalar nada na maquina da loja.
//
//go:embed ocr/tess-ocr.tar.gz
var ocrPacote []byte

// ocrPacoteVersao muda quando o conteudo de ocr/tess-ocr.tar.gz muda; serve de
// marca no nome da pasta para forcar uma nova extracao depois de uma atualizacao.
const ocrPacoteVersao = "tess5.3.3-por-fast-1"

const ocrTimeout = 90 * time.Second

var (
	ocrUmaVez  sync.Once
	ocrDir     string
	ocrCaminho string
	ocrErro    error
)

// pastaCacheApp devolve a pasta de cache gravavel do usuario.
func pastaCacheApp() string {
	if d, err := os.UserCacheDir(); err == nil && d != "" {
		return filepath.Join(d, "AutoPecas")
	}
	return filepath.Join(os.TempDir(), "AutoPecas")
}

// prepararMotorOCR extrai o pacote uma unica vez e devolve o caminho do executavel.
func prepararMotorOCR() (string, error) {
	ocrUmaVez.Do(func() {
		// 1) motor configurado explicitamente (util para suporte/teste)
		if p := strings.TrimSpace(os.Getenv("AUTOPECAS_TESSERACT")); p != "" {
			if _, err := os.Stat(p); err == nil {
				ocrCaminho, ocrDir = p, filepath.Dir(p)
				return
			}
		}
		// 2) tesseract do sistema (Linux/macOS ou instalado no Windows pelo usuario)
		if p, err := exec.LookPath("tesseract"); err == nil {
			ocrCaminho, ocrDir = p, filepath.Dir(p)
			return
		}
		// 3) pacote embutido (padrao no Windows)
		dir, err := extrairPacoteOCR()
		if err != nil {
			ocrErro = err
			return
		}
		ocrDir = dir
		ocrCaminho = filepath.Join(dir, "tesseract.exe")
		if _, err := os.Stat(ocrCaminho); err != nil {
			ocrCaminho = filepath.Join(dir, "tesseract")
			if _, err2 := os.Stat(ocrCaminho); err2 != nil {
				ocrErro = fmt.Errorf("motor de OCR nao encontrado na pasta %s", dir)
			}
		}
	})
	return ocrCaminho, ocrErro
}

// extrairPacoteOCR descompacta ocr/tess-ocr.tar.gz em pastaCacheApp/ocr-<versao>.
func extrairPacoteOCR() (string, error) {
	if len(ocrPacote) == 0 {
		return "", errors.New("pacote do motor de OCR nao esta embutido neste build")
	}
	dir := filepath.Join(pastaCacheApp(), "ocr-"+ocrPacoteVersao)
	marca := filepath.Join(dir, ".pronto")
	if _, err := os.Stat(marca); err == nil {
		return dir, nil
	}
	// extrai numa pasta temporaria e troca no fim, para nao deixar meia extracao
	tmp := dir + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return "", fmt.Errorf("nao consegui criar a pasta do motor de OCR: %w", err)
	}
	descompactador, err := gzip.NewReader(bytes.NewReader(ocrPacote))
	if err != nil {
		return "", fmt.Errorf("pacote do motor de OCR corrompido: %w", err)
	}
	defer descompactador.Close()
	leitor := tar.NewReader(descompactador)
	for {
		cab, err := leitor.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("pacote do motor de OCR corrompido: %w", err)
		}
		nome := filepath.Clean(cab.Name)
		if strings.HasPrefix(nome, "..") || filepath.IsAbs(nome) {
			continue
		}
		destino := filepath.Join(tmp, nome)
		if cab.FileInfo().IsDir() {
			if err := os.MkdirAll(destino, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destino), 0o755); err != nil {
			return "", err
		}
		saida, err := os.OpenFile(destino, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return "", fmt.Errorf("nao consegui gravar %s: %w", nome, err)
		}
		if _, err := io.Copy(saida, leitor); err != nil {
			saida.Close()
			return "", fmt.Errorf("falha ao extrair %s: %w", nome, err)
		}
		saida.Close()
	}
	if err := os.WriteFile(filepath.Join(tmp, ".pronto"), []byte(ocrPacoteVersao), 0o644); err != nil {
		return "", err
	}
	_ = os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		// pode falhar se outra copia estiver rodando; tenta seguir com a temporaria
		if _, e2 := os.Stat(filepath.Join(tmp, ".pronto")); e2 == nil {
			return tmp, nil
		}
		return "", fmt.Errorf("nao consegui preparar a pasta do motor de OCR: %w", err)
	}
	return dir, nil
}

// motorDisponivel informa se existe algum motor de OCR utilizavel.
func motorDisponivel() bool {
	p, err := prepararMotorOCR()
	return err == nil && p != ""
}

// rodarOCR executa o motor sobre a imagem em memoria e devolve o TSV.
// O TSV traz, palavra por palavra, posicao e confianca - e disso que o parser
// precisa para remontar as colunas da nota.
func rodarOCR(png []byte) (string, error) {
	return executarOCR(png, "6", "", true)
}

// rodarOCRComTexto roda o motor devolvendo TEXTO simples (nao TSV), com modo de
// segmentacao (--psm) e lista de caracteres permitidos - usado para reler so o
// pedaco do total pedindo apenas digitos.
func rodarOCRComTexto(png []byte, psm, permitidos string) (string, error) {
	return executarOCR(png, psm, permitidos, false)
}

func executarOCR(png []byte, psm, permitidos string, tsv bool) (string, error) {
	caminho, err := prepararMotorOCR()
	if err != nil {
		return "", err
	}
	arq, err := os.CreateTemp("", "autopecas-nota-*.png")
	if err != nil {
		return "", err
	}
	defer os.Remove(arq.Name())
	if _, err := arq.Write(png); err != nil {
		arq.Close()
		return "", err
	}
	arq.Close()

	ctx, cancel := context.WithTimeout(context.Background(), ocrTimeout)
	defer cancel()
	if psm == "" {
		psm = "6"
	}
	args := []string{arq.Name(), "stdout", "-l", "por", "--psm", psm}
	if pasta := filepath.Join(ocrDir, "tessdata"); temPastaTessdata(pasta) {
		args = append(args, "--tessdata-dir", pasta)
	}
	if permitidos != "" {
		args = append(args, "-c", "tessedit_char_whitelist="+permitidos)
	}
	if tsv {
		args = append(args, "tsv")
	}
	cmd := exec.CommandContext(ctx, caminho, args...)
	cmd.Dir = ocrDir
	var saida, erroSaida bytes.Buffer
	cmd.Stdout = &saida
	cmd.Stderr = &erroSaida
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("o leitor de notas demorou demais para responder (%s)", ocrTimeout)
		}
		return "", fmt.Errorf("o motor de OCR falhou: %v %s", err, strings.TrimSpace(erroSaida.String()))
	}
	if saida.Len() == 0 {
		return "", errors.New("o motor de OCR nao devolveu texto")
	}
	return saida.String(), nil
}

func temPastaTessdata(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// descricaoMotor devolve um rotulo curto do motor em uso (mostrado na tela).
func descricaoMotor() string {
	caminho, err := prepararMotorOCR()
	if err != nil {
		return "indisponivel"
	}
	if strings.Contains(strings.ToLower(caminho), "ocr-"+ocrPacoteVersao) {
		return "embutido (Tesseract para Windows, offline)"
	}
	if runtime.GOOS == "windows" {
		return "Tesseract instalado no Windows"
	}
	return "Tesseract do sistema (" + caminho + ")"
}

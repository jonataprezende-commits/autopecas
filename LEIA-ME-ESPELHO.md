# AutoPeças — espelho do workspace no Dropbox

Esta pasta é a **cópia de segurança oficial** do projeto AutoPeças.
Tudo o que existe no workspace de trabalho (código, testes, manuais, entrega)
está aqui, e é atualizado a cada versão que eu entrego.

Conta: jonataprezende@gmail.com · pasta raiz: `/AutoPecas`

> **Regra de armazenamento (a partir da 1.0.10):** fica no Dropbox **somente a
> versão atual** (`/AutoPecas/1.0.11`) mais a pasta `workspace`. Quando sai uma
> versão nova, a pasta da versão anterior é **apagada** do Dropbox — nada de
> uploads antigos se acumulando. O histórico do código continua no `workspace`.

---

## O que tem em cada pasta

```
/AutoPecas/
├── workspace/
│   ├── LEIA-ME-ESPELHO.md        <- este arquivo
│   ├── MANIFEST-SHA256.txt       <- hash de cada arquivo (conferência)
│   ├── documentos/               <- documentos de projeto (leitura rápida)
│   │   ├── BUGS-ENCONTRADOS.md
│   │   ├── CATALOGO-FORNECEDOR.md
│   │   ├── NOTA-FOTO-OCR.md
│   │   ├── SUGESTOES-ERP.md
│   │   └── Manual-AutoPecas.pdf
│   └── projeto/
│       └── AutoPecas-codigo-fonte.zip           (269 arquivos, 37,7 MB)
└── 1.0.11/                       <- a entrega pronta para instalar (atual)
    ├── AutoPecas-Setup-1.0.11.exe
    ├── AutoPecas-portatil.exe
    ├── Manual-AutoPecas.pdf      (50 páginas)
    ├── LEIA-ME-PRIMEIRO.txt
    ├── RELATORIO-DE-TESTES.md
    ├── COMO-CONFERIR.txt
    ├── SHA-256.txt
    ├── catalogo-exemplo.csv
    └── exemplos-xml/
```

A **1.0.11** corrige o botão **"Ligar o acesso pelo celular"** (que respondia
`404 page not found`): o clique agora passa a atender a rede na hora, **sem
reiniciar o programa** — o reinício era o que fazia o sistema sumir da tela
quando a porta ainda estava ocupada. Junto vieram três testes automáticos novos
em Go e uma suíte de tela que **clica nos botões** (o que faltava para esse
defeito ter passado desde a 1.0.7).

A **1.0.10** é a **revisão geral do programa**: 20 defeitos encontrados e
corrigidos (detalhe no `RELATORIO-DE-TESTES.md`). O jeito de usar não mudou.
Os que aparecem na tela: total do carrinho da bancada acompanhando a quantidade,
aviso quando a venda deixa peça sem estoque, aviso de campo numérico errado em
Configurações, aviso de boleto que não fecha na análise da nota e ficha já
recebida que não pode mais ser apagada. Na proteção dos dados: restaurar backup
preserva a senha do celular, a faxina de backups apaga pela data do nome do
arquivo e trocar a senha derruba as sessões abertas.

A 1.0.9 fez os **backups não encherem o disco** (teto de 300 MB, limite por tipo
podendo ser 1, botão "Apagar os antigos agora", três perfis prontos).
A 1.0.8 trouxe **contas a pagar** (a nota do fornecedor lança os boletos sozinha).
A 1.0.7 trouxe o **acesso pelo celular** e o **Plano B** de backup.

---

## O que tem dentro do `AutoPecas-codigo-fonte.zip`

| Pasta | O que é |
|---|---|
| `autopecas/` | programa em Go: `main.go`, `api_*.go`, `db.go`, `sessao.go` (login do celular), `plano_b.go` (backups, retenção e teto de espaço), `nota_entrada.go` + `api_contas.go` (contas a pagar), `web/` (telas, CSS, QR), `ocr/` (motor de OCR embutido), `exemplos/`, `testdata/` |
| `qa/` | testes automáticos (Python + Playwright + Node): `teste_crit_101.py` (aceitação no `.exe`, 108 verificações), `teste_api_estresse.py` (120 verificações de API), `teste_ui_1010.py` (correções da 1.0.10 na tela), `teste_ui_botao_celular.py` (**novo na 1.0.11**, clica nos botões do celular), as demais suítes `teste_ui_*.py`, `errata_manual_109.py` / `errata_manual_1010.py` / `errata_manual_1011.py`, `montar_espelho.py` |
| `installer/` | instalador em Go (`main_windows.go`, cria atalhos, desinstalador) e `gerar_payload.py` |
| `manual/` | fonte do manual em HTML + imagens |
| `demo-dados/` | banco de exemplo para demonstração |
| `*.md` | documentos de projeto (bugs, catálogo, OCR, sugestões de ERP) |

**Não** entram no zip (são gerados a cada build, não são fonte):
`autopecas/autopecas` (binário compilado), `installer/payload/`,
`tmpwine/` (ambiente de teste no Wine) e os `.exe` de `ENTREGA/`.

---

## Como restaurar o projeto a partir do Dropbox

1. Baixe `projeto/AutoPecas-codigo-fonte.zip`.
2. Descompacte em uma pasta, por exemplo `C:\AutoPecas-projeto`.
3. Para compilar o programa (precisa do Go instalado):
   ```
   cd autopecas
   go build -o AutoPecas.exe .
   ```
4. Para gerar o instalador: `cd installer && python3 gerar_payload.py` e depois
   `go build -ldflags "-s -w -H windowsgui" -o AutoPecas-Setup.exe .`

Se o objetivo é só **usar** o sistema, não precisa de nada disso:
baixe o instalador em `/AutoPecas/1.0.11/` e dê dois cliques.

---

## Regra de conferência (importante)

O Dropbox mostra um "hash" próprio no painel — **não é** o SHA-256 do arquivo.
Para conferir um download no Windows:

```
certutil -hashfile AutoPecas-Setup-1.0.11.exe SHA256
```

O resultado tem que bater com o `MANIFEST-SHA256.txt` (ou com o `SHA-256.txt`
que vem dentro da pasta de cada versão).

---

## Privacidade

O link de compartilhamento da pasta `1.0.11` é **público para quem tiver o link**
(só leitura, não permite editar). Se preferir fechado, é só pedir: eu revogo o
link ou troco por um com senha.

Os dados da loja (banco `autopecas.db`) **não** sobem para o Dropbox — o espelho
tem só programa, código, testes e manuais.

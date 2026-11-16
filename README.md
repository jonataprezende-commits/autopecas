# AutoPecas

Sistema de balcão para loja de autopeças: cadastro de peças, estoque, vendas, fichas de
pedido para o fornecedor, contas a pagar/receber, fechamento de caixa e backup automático.
Roda **100% offline** no computador da loja (servidor Go + interface web local), e pode ser
aberto no celular da loja pela rede local, sem internet.

Versão atual: **1.0.11**

---

## O que tem neste repositório

| Pasta | Conteúdo |
|---|---|
| `codigo/` | Código-fonte do programa (Go + HTML/CSS/JS). É o projeto que gera o `AutoPecas.exe`. |
| `instalador/` | Código do instalador Windows e o script que monta o `payload` (`gerar_payload.py`). |
| `testes/` | Suíte de QA: testes de API, de interface (Playwright) e de aceitação no instalador. |
| `manual/` | Manual do usuário (fonte HTML + imagens) e o PDF final. |
| `entrega/` | Pacote de entrega da versão 1.0.11: instalador, versão portátil, manual, relatório de testes e hashes. |
| `*.md` | Documentação de apoio: bugs encontrados, catálogo de fornecedor, leitor de nota por foto, sugestões de roadmap. |

## Requisitos para compilar

- Go 1.22 ou superior
- (opcional) Python 3 para os scripts de QA e do instalador
- (opcional) No Windows: para gerar o `.exe`, `GOOS=windows GOARCH=amd64`

```bash
cd codigo
go build -o autopecas .
./autopecas                 # sobe o servidor e abre o navegador
```

O programa guarda os dados em `~/.config/AutoPecas/autopecas.db` (Linux) ou
`%APPDATA%\AutoPecas\autopecas.db` (Windows). Os backups ficam na subpasta `backups`.

Opções úteis de linha de comando:

```bash
./autopecas -porta 9000 -sem-navegador        # porta fixa
./autopecas -host 0.0.0.0 -senha-rede senha   # abrir para o celular na rede local
./autopecas -dados /caminho/para/dados        # pasta de dados alternativa
```

## Atalhos

| Tecla | Tela |
|---|---|
| `F2` | Bancada (venda) |
| `F3` | Peças |
| `F4` | Fichas / pedidos de compra |
| `F5` | Catálogo do fornecedor |
| `F8` | Fechamento do dia |
| `F9` | Finalizar venda |
| `Ctrl+B` | Backup |

## Testes

```bash
cd codigo
go test . -count=1          # 126 testes
```

Os testes de interface usam Playwright e precisam do programa no ar:

```bash
python3 testes/teste_ui_botao_celular.py ./codigo/autopecas 9009
```

## Aviso fiscal

O AutoPecas **não emite documento fiscal**. O cupom impresso ao finalizar a venda é um
comprovante interno de venda, não uma NFC-e/NF-e. Veja `SUGESTOES-ERP.md` (item 15) para o
plano de integração fiscal.

## Licença e uso

Código proprietário, de uso interno da loja. Todos os direitos reservados.

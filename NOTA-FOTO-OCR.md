# Dá para ler nota em papel pela foto? — medição, plano e o que foi entregue

> **STATUS: IMPLEMENTADO na versão 1.0.3 (Fase 1 do plano abaixo).**
> Tela **Nota por Foto**, OCR offline embutido no `.exe`, tabela de conferência e importação com
> confirmação. Resultado medido nas fotos de teste: **8/8 itens e 24/24 campos certos** na foto boa
> e na foto média (sombra), 1,9–2,3 s por nota no executável. Foto ruim é recusada com orientação.
> Evidências: `ENTREGA/RELATORIO-DE-TESTES.md` (seção 0) · testes `autopecas/nota_foto_test.go` ·
> `qa/teste_crit_101.py` (C7) · `qa/teste_ui_nota.py` · fotos em `autopecas/testdata/`.
> O plano abaixo é o registro de como a decisão foi tomada — o texto original foi mantido.

Data: 29/10/2026 · Feito sobre o AutoPeças 1.0.2 (nada foi alterado no programa)

Resposta curta: **dá, para nota impressa e com a foto razoável — mas só com uma tela de
conferência antes de gravar.** Não dá para ser 100% automático e silencioso: o OCR troca
dígito (0/8/6/9, 5/6) e um erro de R$ 0,08 numa nota de compra vira custo errado no estoque.

## O que eu testei (não é estimativa, é medição)

Montei uma nota de compra sintética com 8 itens (código, descrição, quantidade, unitário,
total — total da nota R$ 932,20), fotografei em 3 qualidades e rodei OCR **offline**
(Tesseract 5, idioma português), com o pipeline: achar a folha → endireitar em perspectiva →
cinza + contraste local → binarizar → OCR → reconstruir as colunas pela posição X das palavras.

| Foto | Folha detectada | Itens achados | Quantidade certa | Total do item certo | Confiança OCR |
|---|---|---|---|---|---|
| **Boa** (celular apoiado, luz do dia, folha inteira, ~2000 px) | sim | **8/8** | **8/8** | **8/8** | 91,4% |
| **Média** (na mão, sombra de um lado, desfoque leve) | não | 8 linhas lidas, mas valores furados | 0/8 | 0/8 | 74,4% |
| **Ruim** (torta, sombra forte, JPEG apertado) | não | 6 linhas, tudo misturado | 0/8 | 0/8 | 65,3% |

Arquivos da prova: `qa/spike-nota-mesa.jpg`, `qa/spike-foto-boa.jpg`, `qa/spike-foto-media.jpg`,
`qa/spike-foto-ruim.jpg`, scripts `qa/spike_ocr_nota*.py`, resultados em
`qa/spike-ocr-resultado.json` e `qa/spike-ocr-qualidade.json`.

Exemplos reais do que o OCR devolveu (foto média) — repare nos dígitos:

```
sKu- 1001 FILTRO ... 2  18,50  37,60      <- total certo era 37,00
sku- 1004 vELA ... 4  39,75  159,90       <- certo era 159,00
sku- 1906 RADIADOR 500ML 118,80 118,00    <- 1006 virou 1906; 500ML virou 500ML ok, valor errado
```

Sem tratamento de imagem (OCR direto na foto torta) foi pior ainda: 5 de 8 códigos lidos e
**0 de 16** campos de valor certos. Ou seja: **o tratamento de imagem é obrigatório**, e mesmo
com ele a diferença entre "foto boa" e "foto de qualquer jeito" é a diferença entre funcionar
e não funcionar.

## O que isso significa na prática (as 4 verdades)

1. **Nota impressa (DANFE, cupom de fornecedor, romaneio digitado): funciona.** Letra de forma
   impressa, tabela com colunas alinhadas — é o caso do teste acima.
2. **Nota escrita à mão: não.** OCR de manuscrito em português, offline, não é confiável para
   dinheiro. Não vou prometer isso.
3. **Foto torta/escura/desfocada derruba tudo.** O app tem que recusar e pedir outra foto
   ("não consegui ler — apoie o celular e fotografe a folha inteira"), não tentar adivinhar.
4. **A conferência automática salva o dia:** a soma dos itens tem que fechar com o total da nota
   impresso. Se não fechar, o sistema aponta a linha suspeita. Isso pega a maior parte dos erros
   de dígito antes de entrar no estoque.

## Como eu faria no AutoPeças (3 fases, cada uma útil sozinha)

**Fase 1 — "Fotografar nota" com tela de revisão (o coração do pedido).**
- Tela nova: arrastar/colar a foto (ou tirar do celular e mandar pelo navegador do PC).
- O servidor faz o tratamento de imagem, o OCR e devolve uma **tabela editável**: código,
  descrição, qtd, unitário, total — com as linhas de baixa confiança destacadas em amarelo.
- Botão **"Importar selecionados"** cria/atualiza as peças e dá entrada no estoque, exatamente
  como a importação de XML já faz hoje (mesmo caminho de código, mesmo log).
- Conferência obrigatória: soma dos itens = total da nota; se não fechar, mostra a diferença e
  não deixa "Importar tudo" sem marcar "revisei".
- Nada é gravado sem o clique final. Sem risco de sujar o estoque com leitura errada.

**Fase 2 — Aprender com o uso.**
- Guardar a "assinatura" de cada fornecedor (posição das colunas, cabeçalho). Na segunda foto do
  mesmo fornecedor o parser já entra calibrado — a taxa de acerto sobe bastante, porque a maioria
  dos erros vem de layout desconhecido.
- Sugerir o cadastro existente: código lido → peça já cadastrada? atualiza custo/estoque em vez de
  duplicar.

**Fase 3 — Atalhos de entrada (as vezes em que nem precisa de foto).**
- O fornecedor manda XML ou PDF por e-mail/WhatsApp? **Já funciona hoje** — Importar XML é mais
  confiável e mais barato que foto. A foto é o plano B para quem só tem o papel.
- Código de barras da DANFE (chave de acesso) como identificador da nota, quando a foto pegar.

## Decisão técnica que muda o tamanho da entrega

| Caminho | Precisão | Internet | Peso na entrega |
|---|---|---|---|
| **OCR embutido (Tesseract portátil no instalador)** — meu recomendado | a do teste (91% em foto boa) | **nenhuma** | +~30 MB no instalador (7,3 → ~37 MB, ainda longe do limite de 120 MB) |
| OCR do próprio Windows 10/11 (Windows.Media.Ocr, grátis, sem instalar nada) | parecida | nenhuma | 0 MB, mas depende do Windows ter o idioma pt-BR instalado e a chamada é mais frágil |
| OCR em nuvem (Google Vision/Azure) | melhor (95%+) | **precisa internet** | 0 MB, mas quebra a promessa "100% offline" e custa por foto |

## Estimativa honesta de esforço (com teste antes, como na 1.0.1/1.0.2)

- Fase 1 completa (upload, tratamento, OCR embutido, parser de tabela por colunas, tela de
  revisão, conferência de soma, entrada no estoque, testes Go + teste de UI com 3 fotos de
  qualidade diferente, instalador e manual atualizados): **o trabalho de alguns dias**, não de
  uma hora. É a maior funcionalidade desde que o sistema ficou offline-first.
- Fase 2 (calibração por fornecedor): menor, soma bem depois da Fase 1 rodando no balcão.
- Fase 3: barata, mas o XML já cobre boa parte.

## O que foi decidido (e por quê)

* **Motor:** OCR embutido no `.exe` (Tesseract 5.3.3 para Windows, idioma português, modelo *fast*
  de 2 MB). Instalador 7,28 → 19,14 MB; programa 10,90 → 23,00 MB. Escolhido porque é o único
  caminho que mantém a promessa de funcionar offline em qualquer Windows, sem depender do idioma
  instalado na máquina e sem custo por foto.
* **Entrada da foto:** upload pelo navegador (cabo, WhatsApp Web, nuvem) — sem app de celular.
* **Segurança:** nada é gravado sem `confirmado=true`; a tela mostra a tabela editável e o servidor
  exige a confirmação explícita.
* **Manuscrito:** fora de escopo, de propósito.

## O que eu preciso decidir com você antes de começar (original)

1. **Caminho:** OCR embutido (offline garantido, +30 MB) ou OCR do Windows (entrega leve,
   depende da máquina)?
2. **Quem vai fotografar:** o celular do balcão (manda a foto para o PC pelo navegador) ou uma
   webcam/scan no próprio PC?
3. **Notas que você recebe:** são impressas (DANFE/cupom) ou algumas vêm escritas à mão? Isso
   muda a expectativa — e eu prefiro te dizer agora do que entregar e frustrar.

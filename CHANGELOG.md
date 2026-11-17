# Changelog

Todas as mudanças relevantes do AutoPecas. O formato segue o
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/) e o versionamento
segue o [SemVer](https://semver.org/lang/pt-BR/).

## [1.1.0] - 2026-11-17

### Adicionado

- **Módulo "Estoque Inteligente"** (nova tela acessível pelo menu lateral):
  - **Sugestões de compra** baseadas em demanda real, ponto de reposição, estoque de segurança e lead time do fornecedor. Criticidade automática (Crítica/Média/Baixa) e quantidade sugerida calculada.
  - **Estoque parado**: itens sem saída há N dias (configurável, padrão 90), ordenados por valor parado (estoque × custo médio).
  - **Curva ABC** por receita nos últimos N dias (configurável, padrão 90): classes A (≤80%), B (≤95%), C (resto).
  - **Comparação de períodos**: dois intervalos de datas lado a lado com receita, custo, margem, nº de vendas, itens vendidos + top 10 itens com maior crescimento de receita.
  - **Configurações** ajustáveis pela tela: lead time padrão, estoque de segurança (dias), janela de demanda, dias para considerar parado, janela da Curva ABC.
  - **Geração de pedido de compra** a partir das sugestões (ou seleção manual): agrupa por fornecedor, soma quantidades de peças repetidas, não altera estoque (entra no recebimento), não usa fichas de falta.
  - **Exportação** CSV (sugestoes, parados, abc) e PDF (sugestões).
  - **Resumo em cards**: críticas, médias, baixas, custo estimado de reposição, valor/itens parados, classes ABC.

- **Prazo de entrega do fornecedor** (`suppliers.prazo_entrega_dias`, migração de dados 8): usado no cálculo do ponto de reposição; se não preenchido, usa o lead time padrão global.
- Settings padrão do Estoque Inteligente criados no `firstRun`: `ei_lead_dias=7`, `ei_seguranca_dias=3`, `ei_demanda_dias=90`, `ei_parado_dias=90`, `ei_abc_dias=90`.

### Alterado

- **Fornecedores** (`GET/POST/PUT /api/fornecedores`): passa a incluir/expor `prazo_entrega_dias`.
- **API de relatórios**: rotas de exportação CSV/PDF reutilizam a infraestrutura existente (BOM UTF-8, separador `;`).

### Corrigido

- Classificação da Curva ABC: agora usa o percentual acumulado **antes** do item (não depois), para que o item de maior receita fique na classe A mesmo quando é o único.
- Cálculo de período em "Comparar": query SQL corrigida (JOIN em vez de subquery inválida `SUM(SELECT...)`).
- Formato de data nas vendas criadas via API: `validarDataVenda` aceita `YYYY-MM-DD` e converte para timestamp completo.

## [1.0.12] - 2026-11-16

### Adicionado

- **Custo médio ponderado das compras** (`parts.avg_cost_cents`, migração de
  dados 7). O custo de compra (`cost_cents`) continua sendo o último preço
  pago; a média ponderada é calculada a cada entrada:
  `novo = (média_atual × estoque_atual + preço × quantidade) / (estoque_atual + quantidade)`.
  Atualizada em: movimento manual de estoque (entrada), recebimento de pedido
  de compra e importação de NF-e que dá entrada no estoque. Saídas e ajustes
  não alteram a média. Peças que já existiam começam com a média igual ao
  último custo conhecido, para não zerar o relatório até a próxima compra.
- **Valorização pelo custo médio** nos relatórios `estoque_valor`,
  `estoque_minimo` e no cartão "valor do estoque" do painel. Quando a média
  ainda não existe (peça sem compra registrada), cai para o último custo.
- **Integração contínua** (`.github/workflows/ci.yml`): `go build`, `go vet` e
  `go test` a cada push/PR.
- **Aviso de "carregando"** nas operações demoradas (abrir aba, importar XML,
  ler foto de nota, backup). Aparece só se a resposta passar de 250 ms — em
  chamadas instantâneas a tela não pisca — e some ao terminar. Vale para toda
  chamada feita pelo `api()` do front e para os envios de arquivo.
- **Instalador reprodutível** (`instalador/gerar_instalador.ps1`): compila o
  programa, compacta programa + manual e monta o `AutoPecas-Setup-<versão>.exe`
  em um comando. O `gerar_payload.py` deixou de depender de caminho fixo e
  aceita `--programa`, `--manual` e `--saida`.
- Campo `custo_medio_centavos` na resposta da API de peças.
- Testes: `TestCustoMedioPonderado`, `TestCustoParaValoracao`,
  `TestEntradaDeCompraAtualizaCustoMedio`, `TestRelatorioEstoqueUsaCustoMedio`,
  `TestMigracaoCustoMedioExiste`.

### Alterado

- O banco abre com **4 conexões** (`SetMaxOpenConns(4)`) em vez de uma. Com WAL,
  as leituras não bloqueiam a escrita e a tela (várias abas + backup) deixa de
  enfileirar atrás de uma única conexão. O `busy_timeout` de 8s cobre a disputa.
- Edição manual de peça (`PUT /api/pecas/{id}`) não recalcula a média (não
  houve compra); apenas preenche a média quando ela ainda não existia.

### Corrigido

- Testes que dependiam de caminho absoluto de outra máquina
  (`/home/user/ENTREGA/...`) agora leem o exemplo da pasta `entrega/` do
  próprio repositório.
- Teste de troca de endereço pulava no Windows por comportamento do SO; agora
  é ignorado só no Windows (e continua rodando no CI Linux).
- Testes de migração passam a comparar com `versaoEsquema` em vez de um número
  fixo, evitando quebra a cada nova versão de dados.

### Observação de segurança (já existente, confirmado no 1.0.12)

O servidor local já aplica, em todas as rotas `/api/`:

- cabeçalho anti-CSRF `X-AutoPecas: 1` em POST/PUT/DELETE/PATCH;
- validação de `Host`, `Origin` e `Sec-Fetch-Site`;
- cookies de sessão com `HttpOnly` + `SameSite=Strict` (+ `Secure` atrás de TLS);
- limite de tentativas de senha (8 erros em 5 min bloqueiam o aparelho).

Não foi necessário alterar: os itens já estavam implementados e cobertos por
testes (`seguranca_test.go`, `sessao_test.go`).

## Backlog (próximas versões)

Ideias levantadas e deixadas para depois, com o custo/rascunho já conhecido:

- **Caixa / turno**: abertura e fechamento com sangria, suprimento e conferência
  de valores por operador.
- **Devolução parcial de venda**: devolver 1 item de uma venda mantendo o
  restante, com entrada automática no estoque e estorno do recebível.
- **NFC-e / SAT**: emissão fiscal integrada (hoje só controle interno).
- **Orçamento → venda**: transformar um orçamento aprovado em venda com um clique.
- **Testes de concorrência do SQLite**: cenários de escrita simultânea
  (duas abas vendendo ao mesmo tempo) com `-race`.
- **HTTPS local**: certificado próprio para o modo rede, sem depender de túnel.
- **Acessibilidade / mobile**: revisão de contraste, foco e alvos de toque no
  modo celular.

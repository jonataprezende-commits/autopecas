# Changelog

Todas as mudanças relevantes do AutoPecas. O formato segue o
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/) e o versionamento
segue o [SemVer](https://semver.org/lang/pt-BR/).

## [1.0.12] - Não lançado

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

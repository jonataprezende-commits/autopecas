# AutoPeças 1.0.11 — botão "Ligar o acesso pelo celular" (404 page not found)

Data: 29/10/2026 · Versão anterior: 1.0.10 → **Versão entregue: 1.0.11**

---

## 0. O que o usuário relatou

> "404 page not found quando clico no botão ligar pelo celular"

Reproduzido no navegador, com a tela aberta. O botão não funcionava **desde a
1.0.7** — e tinha três defeitos encadeados, um escondendo o outro.

---

## 1. Os três defeitos (e por que ninguém tinha visto)

| # | Defeito | Efeito para o dono da loja | Correção |
|---|---|---|---|
| 1 | O botão chamava a rota sem o corpo da requisição; o programa montava um **GET** e a rota só aceita **POST** | **`404 page not found`** ao clicar em "Ligar", "Trocar a senha" e "Desligar" | chamada corrigida e, por segurança, `api()` agora transforma campos soltos em corpo (um `{campo: 1}` esquecido não vira mais GET) |
| 2 | Para passar a atender a rede, o programa **fechava e abria outro processo**. Se a porta ainda estivesse ocupada, o processo novo desistia, achava que "já estava aberto" e **morria calado** | o sistema **sumia da tela** e não voltava; só reabrindo pelo atalho | o programa agora **troca o endereço de escuta em tempo real**, sem reiniciar nada |
| 3 | Ao soltar o endereço antigo, o `http.Server` recebia `use of closed network connection`; o `main` tratava como erro grave e **encerrava o programa** | mesma coisa: sistema fora do ar | `Accept()` do novo listener "trocável" ignora o erro do endereço antigo enquanto a troca acontece, e `net.ErrClosed` passou a ser tratado como fechamento normal |

Ou seja: o 404 era só a ponta. Se ele fosse corrigido sozinho, o clique
derrubaria o sistema — foi exatamente o que aconteceu nos primeiros testes.

---

## 2. Como funciona agora

```
clique em "Ligar o acesso pelo celular"
   └─ POST /api/rede/aplicar  (senha conferida, gravada com sal)
      └─ trocarEndereco: 127.0.0.1:8787  ->  0.0.0.0:8787
         ├─ abre o endereço novo (tenta de novo por até 5 s)
         ├─ se não conseguir: volta ao endereço antigo e avisa na tela
         └─ servidor continua o MESMO (ninguém é desconectado)
```

* Sem reinício, sem segundo processo, sem porta "perdida".
* O QR Code e os endereços de rede aparecem na hora no painel.
* Desligar é imediato; trocar a senha vale na hora (o celular precisa entrar de
  novo com a senha nova).
* Se a porta estiver realmente ocupada por outro programa, aparece o aviso
  "*A senha foi salva, mas o programa não conseguiu passar a atender a rede…*" e
  o AutoPeças **continua funcionando neste computador**.

---

## 3. Testes desta versão

| Frente | Comando | Resultado |
|---|---|---|
| Testes do programa | `go test . -count=1` | **126 PASS** (3 novos) |
| Análise estática | `go vet` / `staticcheck` | limpo |
| **Botão de verdade, clicado** | `python3 qa/teste_ui_botao_celular.py` | **22 OK / 0 falhas** |
| Celular (senha, QR, PC sem senha) | `qa/teste_ui_celular.py` | **34 OK / 0** |
| API sob estresse | `qa/teste_api_estresse.py` | **120 OK / 0** |
| Correções da 1.0.10 na tela | `qa/teste_ui_1010.py` | **11 OK / 0** |
| Backup/Plano B/contas/catálogo/nota/1.0.2 | 6 suítes | **19+23+28+13+13+8 = 104 OK / 0** |
| Instalador instalado no Windows (Wine) | `qa/teste_crit_101.py` | **108 OK / 0** |
| Botão ligando a rede no `.exe` do Windows | POST real no programa instalado | passou a atender `*:9010`, resposta `modo: rede`, sem sair do ar |

### 3.1 Teste novo que faltava (a raiz do problema)

`qa/teste_ui_botao_celular.py` — o único teste que **clica nos botões** do
painel (Ligar, Trocar a senha, Desligar). As outras suítes chamavam a API
direto, então o botão podia estar quebrado sem ninguém notar. Ele também sobe o
próprio programa e confere que a porta continua respondendo depois de cada
clique.

### 3.2 Testes novos em Go (`revisao_1011_test.go`)

| Teste | O que garante |
|---|---|
| `TestTrocaDeEnderecoMantemProgramaNoAr` | o servidor atende antes, durante e depois de 127.0.0.1 → 0.0.0.0 → 127.0.0.1 |
| `TestSoltarListenerNaoDerrubaOServidor` | se o endereço novo falhar, volta ao antigo e **continua no ar** |
| `TestAcceptDuranteTrocaNaoRetornaErro` | o `Accept` não devolve erro no meio da troca (o erro que matava o programa) |

---

## 4. Arquivos alterados

```
main.go          listenerTrocavel + trocarEndereco/voltarAoEnderecoAntigo;
                 fim do reinicio por segundo processo; net.ErrClosed tratado
sessao.go        /api/rede/aplicar usa a troca em tempo real, devolve
                 trocou_endereco/enderecos/aviso; urlsRede(); sessoes de outros
                 aparelhos caem ao ligar/desligar
web/app.js       api() protege contra corpo esquecido (GET por engano);
                 botao do celular sem "reiniciando"; texto do painel atualizado
revisao_1011_test.go   testes novos da troca de endereco
qa/teste_ui_botao_celular.py   teste novo que clica nos botoes
qa/errata_manual_1011.py       paginas novas do manual (50 paginas no total)
```

---

## 5. Nada mais mudou

* Banco **schema v6**, sem migração: atualizar por cima não pede nada.
* Quem já tinha o acesso pelo celular ligado continua ligado, com a mesma senha.
* 100% offline, sem TecDoc, sem enviar dados para fora.

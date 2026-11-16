#!/usr/bin/env python3
"""Teste de estresse da API do AutoPecas (rota por rota, com dados invalidos).

Objetivo: achar defeitos que os testes de tela nao pegam — valor negativo,
quantidade absurda, desconto maior que o item, id que nao existe, duas baixas
da mesma conta, texto onde devia ter numero. Duas regras que sempre valem:

  1. nada que o usuario digitar pode derrubar o sistema (nunca HTTP 500);
  2. dado invalido NAO entra no banco (a resposta tem que ser 4xx).

Uso:
    python3 qa/teste_api_estresse.py http://127.0.0.1:PORT
(sobe uma instancia nova antes:  ./AutoPecas -dados /tmp/x -porta PORT -sem-navegador)
"""
import json
import random
import sys
import urllib.error
import urllib.parse
import urllib.request

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8920").rstrip("/")
API = BASE + "/api"
falhas = []
total = 0
# sufixo unico: a suite pode rodar varias vezes no mesmo banco (nao repete codigo)
SUF = "".join(random.choice("ABCDEFGHJKLMNPQRSTUVWXYZ23456789") for _ in range(4))


def req(rota, corpo=None, metodo=None):
    dados = json.dumps(corpo).encode() if corpo is not None else None
    cab = {"X-AutoPecas": "1", "Content-Type": "application/json", "Origin": BASE}
    m = metodo or ("GET" if corpo is None else "POST")
    r = urllib.request.Request(API + rota, data=dados, headers=cab, method=m)
    try:
        with urllib.request.urlopen(r, timeout=30) as resp:
            b = resp.read()
            try:
                return resp.status, json.loads(b)
            except Exception:
                return resp.status, b.decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        b = e.read()
        try:
            return e.code, json.loads(b)
        except Exception:
            return e.code, b.decode("utf-8", "replace")
    except Exception as e:  # conexao recusada, timeout...
        return 0, str(e)


def check(nome, cond, extra=""):
    global total
    total += 1
    if cond:
        print("  OK   " + nome)
    else:
        print("  FALHA " + nome + ((" — " + str(extra)) if extra != "" else ""))
        falhas.append(nome)


def secao(t):
    print("\n== " + t)


def nao_quebrou(nome, rota, corpo=None, metodo=None):
    """Regra 1: nenhuma resposta 5xx (o servidor nao pode cair)."""
    st, r = req(rota, corpo, metodo)
    check(nome + " nao derruba o sistema", st != 0 and st < 500, (st, r))
    return st, r


def main():
    st, ping = req("/ping")
    if st != 200:
        print("o sistema nao respondeu em " + BASE)
        return 1
    print(f"AutoPecas {ping.get('versao')} em {BASE}")

    secao("pecas: valores impossiveis")
    st, r = req("/config", {"permitir_estoque_negativo": "0"}, "POST")
    COD = "E" + SUF
    st, p1 = req("/pecas", {"descricao": "Pastilha", "codigo": COD, "preco_centavos": 1200,
                            "custo_centavos": 500, "estoque": 5, "minimo": 1})
    check("criar peca valida", st == 200, (st, p1))
    pid = p1.get("id") if isinstance(p1, dict) else 0
    for nome, corpo in [
        ("preco negativo", {"descricao": "X", "preco_centavos": -100}),
        ("custo negativo", {"descricao": "X", "custo_centavos": -1}),
        ("estoque negativo", {"descricao": "X", "estoque": -5}),
        ("minimo negativo", {"descricao": "X", "minimo": -2}),
        ("preco absurdo", {"descricao": "X", "preco_centavos": 1000000001}),
        ("estoque absurdo", {"descricao": "X", "estoque": 1000001}),
    ]:
        st, r = nao_quebrou("peca com " + nome, "/pecas", corpo, "POST")
        check("peca com " + nome + " e recusada", st == 400, (st, r))
        check("peca com " + nome + " explica o motivo", isinstance(r, dict) and r.get("erro"), r)

    st, r = req("/pecas/999999", {"descricao": "Fantasma", "codigo": "ZZ" + SUF}, "PUT")
    check("editar peca inexistente responde 404", st == 404, (st, r))

    st, r = req(f"/pecas/{pid}", {"descricao": "Pastilha editada", "codigo": COD,
                                  "preco_centavos": 1300, "custo_centavos": 500, "estoque": 99}, "PUT")
    check("editar peca nao mexe no estoque por engano", st == 200 and r.get("estoque") == 5, (st, r.get("estoque")))

    secao("vendas")
    _, p0 = req(f"/pecas/{pid}")
    estoque_antes = p0.get("estoque")
    st, v = req("/vendas", {"itens": [{"peca_id": pid, "quantidade": 2, "preco_centavos": 1300}],
                            "forma_pagamento": "dinheiro"})
    check("venda valida", st == 200, (st, v))
    vid = v.get("id") if isinstance(v, dict) else 0
    for nome, corpo in [
        ("quantidade zero", {"itens": [{"peca_id": pid, "quantidade": 0, "preco_centavos": 100}], "forma_pagamento": "pix"}),
        ("quantidade absurda", {"itens": [{"peca_id": pid, "quantidade": 1000001, "preco_centavos": 100}], "forma_pagamento": "pix"}),
        ("preco negativo", {"itens": [{"peca_id": pid, "quantidade": 1, "preco_centavos": -5}], "forma_pagamento": "pix"}),
        ("desconto maior que o item", {"itens": [{"peca_id": pid, "quantidade": 1, "preco_centavos": 100, "desconto_centavos": 99999}], "forma_pagamento": "pix"}),
        ("sem itens", {"itens": [], "forma_pagamento": "pix"}),
        ("data impossivel", {"itens": [{"peca_id": pid, "quantidade": 1, "preco_centavos": 100}], "forma_pagamento": "pix", "data": "2026-13-45"}),
    ]:
        st, r = nao_quebrou("venda com " + nome, "/vendas", corpo, "POST")
        check("venda com " + nome + " e recusada", st == 400, (st, r))

    st, r = req("/vendas", {"itens": [{"peca_id": pid, "quantidade": 100, "preco_centavos": 1000}],
                            "forma_pagamento": "dinheiro"})
    check("venda acima do estoque e recusada (com a trava ligada)", st == 400 and "estoque" in str(r).lower(), (st, r))

    st, r = req("/vendas", {"itens": [{"peca_id": pid, "quantidade": 1, "preco_centavos": 1000}],
                            "forma_pagamento": "dinheiro", "recebido_centavos": 100})
    check("recebido menor que o total e recusado", st == 400, (st, r))

    st, r = req("/vendas", {"itens": [{"peca_id": pid, "quantidade": 1, "preco_centavos": 1000}],
                            "forma_pagamento": "dinheiro", "desconto_centavos": 999999})
    check("desconto da venda nao deixa total negativo", st == 200 and r.get("total_centavos") == 0, (st, r.get("total_centavos")))

    st, r = req("/vendas", {"itens": [{"peca_id": pid, "quantidade": 1, "preco_centavos": 1000}],
                            "forma_pagamento": "cartao", "recebido_centavos": 5000})
    check("troco calculado", st == 200 and r.get("troco_centavos") == 4000, (st, r.get("troco_centavos")))

    secao("cancelamento de venda")
    _, pc = req(f"/pecas/{pid}")
    estoque_antes_cancel = pc.get("estoque")
    st, r = req(f"/vendas/{vid}/cancelar", {}, "POST")
    check("cancelar venda", st == 200, (st, r))
    st, p = req(f"/pecas/{pid}")
    check("estoque volta ao cancelar", p.get("estoque") == estoque_antes_cancel + 2,
          f"{p.get('estoque')} (esperava {estoque_antes_cancel} + 2)")
    st, r = req(f"/vendas/{vid}/cancelar", {}, "POST")
    check("cancelar duas vezes nao duplica estoque", st == 200, (st, r))
    st, p = req(f"/pecas/{pid}")
    check("cancelar de novo nao devolve o estoque outra vez",
          p.get("estoque") == estoque_antes_cancel + 2, p.get("estoque"))
    st, r = req("/vendas/999999/cancelar", {}, "POST")
    check("cancelar venda inexistente responde 400/404", st in (400, 404), (st, r))

    secao("estoque")
    st, p = req(f"/pecas/{pid}")
    antes = p.get("estoque")
    for nome, corpo in [
        ("quantidade absurda", {"peca_id": pid, "tipo": "entrada", "quantidade": 1000001}),
        ("custo negativo", {"peca_id": pid, "tipo": "entrada", "quantidade": 1, "custo_centavos": -10}),
        ("saida sem motivo", {"peca_id": pid, "tipo": "baixa", "quantidade": 1}),
        ("peca inexistente", {"peca_id": 999999, "tipo": "entrada", "quantidade": 1}),
        ("saida maior que o saldo", {"peca_id": pid, "tipo": "saida", "quantidade": antes + 10}),
    ]:
        st, r = nao_quebrou("estoque: " + nome, "/estoque/movimento", corpo, "POST")
        check("estoque recusa " + nome, st == 400, (st, r))
    st, r = nao_quebrou("estoque: entrada valida", "/estoque/movimento",
                        {"peca_id": pid, "tipo": "entrada", "quantidade": 3}, "POST")
    check("entrada valida entra", st == 200, (st, r))

    secao("contas a pagar")
    st, c = req("/pagaveis", {"fornecedor": "ALUGUEL " + SUF, "valor_centavos": 150000, "vencimento": "2026-11-10"})
    check("criar conta a pagar", st == 200, (st, c))
    cid = c.get("id") if isinstance(c, dict) else 0
    for nome, corpo in [
        ("sem fornecedor", {"fornecedor": "", "valor_centavos": 100, "vencimento": "2026-11-10"}),
        ("valor zero", {"fornecedor": "X", "valor_centavos": 0, "vencimento": "2026-11-10"}),
        ("valor negativo", {"fornecedor": "X", "valor_centavos": -100, "vencimento": "2026-11-10"}),
        ("vencimento impossivel", {"fornecedor": "X", "valor_centavos": 100, "vencimento": "2026-13-45"}),
    ]:
        st, r = nao_quebrou("conta: " + nome, "/pagaveis", corpo, "POST")
        check("conta recusa " + nome, st == 400, (st, r))
    st, r = req("/pagaveis", {"fornecedor": "X", "valor_centavos": 100, "vencimento": "10/11/2026"})
    check("vencimento no formato brasileiro e aceito", st == 200, (st, r))
    st, r = req(f"/pagaveis/{cid}/baixar", {"valor_pago_centavos": 155000}, "POST")
    check("baixar (pagar) a conta", st == 200, (st, r))
    st, r = req(f"/pagaveis/{cid}/baixar", {"valor_pago_centavos": 155000}, "POST")
    check("pagar duas vezes e recusado", st == 400, (st, r))
    st, r = req("/pagaveis/999999/baixar", {}, "POST")
    check("pagar conta inexistente responde 400", st == 400, (st, r))
    st, r = req("/pagaveis?status=todos")
    doc_ok = True
    for it in r.get("itens", []):
        if it.get("fornecedor") == "ALUGUEL " + SUF and it.get("documento"):
            doc_ok = False
    check("conta manual nao inventa numero de documento", doc_ok, r.get("itens"))

    secao("contas a receber")
    st, v = req("/vendas", {"itens": [{"peca_id": pid, "quantidade": 1, "preco_centavos": 1000}],
                            "forma_pagamento": "faturado", "cliente_nome": "Fulano", "vencimento_dias": 30})
    st, rec = req("/recebiveis")
    check("recebivel criado", rec.get("total_aberto_centavos", 0) == 1000, rec)
    if rec.get("itens"):
        rid = rec["itens"][0]["id"]
        st, r = req(f"/recebiveis/{rid}/baixar", {}, "POST")
        check("baixar recebivel", st == 200, (st, r))
        st, r = req(f"/recebiveis/{rid}/baixar", {}, "POST")
        check("baixar duas vezes e recusado", st == 400, (st, r))
    st, r = nao_quebrou("recebivel inexistente", "/recebiveis/999999/baixar", {}, "POST")
    check("baixar recebivel inexistente responde 400", st == 400, (st, r))

    secao("fichas de falta")
    st, f = req("/fichas", {"descricao": "Correia", "quantidade": 1, "peca_id": pid})
    check("criar ficha", st == 200, (st, f))
    fid = f.get("id") if isinstance(f, dict) else 0
    st, r = req(f"/fichas/{fid}/status", {"status": "recebida", "peca_id": pid}, "POST")
    check("receber ficha", st == 200, (st, r))
    st, r = req(f"/fichas/{fid}/status", {"status": "recebida", "peca_id": pid}, "POST")
    check("receber a mesma ficha duas vezes e recusado", st == 409, (st, r))
    st, r = req(f"/fichas/{fid}", None, "DELETE")
    check("apagar ficha recebida e recusado (mantem o historico)", st == 409, (st, r))
    st, r = nao_quebrou("ficha com status invalido", f"/fichas/{fid}/status", {"status": "banana"}, "POST")
    check("ficha com status invalido e recusada", st == 400, (st, r))

    secao("configuracoes")
    for nome, corpo in [
        ("texto no lugar de numero", {"backup_manter": "abc"}),
        ("limite negativo", {"backup_limite_mb": "-5"}),
        ("intervalo curto demais", {"backup_intervalo_minutos": "3"}),
        ("prazo vazio", {"prazo_faturado_dias": ""}),
        ("pasta relativa", {"backup_pasta_extra": "Dropbox\\Backup"}),
    ]:
        st, r = nao_quebrou("config: " + nome, "/config", corpo, "POST")
        check("config recusa " + nome, st == 400, (st, r))
    st, r = nao_quebrou("config valida", "/config", {"backup_manter_horarios": "3", "backup_limite_mb": "300"}, "POST")
    check("config valida e gravada", st == 200, (st, r))
    st, r = req("/config")
    check("config devolve o que foi gravado", r.get("backup_manter_horarios") == "3", r.get("backup_manter_horarios"))

    secao("backup")
    st, r = nao_quebrou("backup manual", "/backup", {}, "POST")
    check("backup manual gera arquivo", st == 200 and r.get("tamanho_bytes", 0) > 0, (st, r))
    st, r = nao_quebrou("saude do backup", "/saude-backup")
    check("painel do backup responde", st == 200 and r.get("total_arquivos", 0) >= 1, (st, r))
    st, r = nao_quebrou("limpar antigos", "/backup/limpar", {}, "POST")
    check("limpar antigos responde", st == 200 and "total_bytes" in r, (st, r))
    st, r = nao_quebrou("testar restauracao", "/backup/testar", {})
    check("testar restauracao abre o backup em leitura", st == 200 and r.get("ok"), (st, r))
    st, r = req("/restaurar?arquivo=nao-existe.db", {}, "POST")
    check("restaurar arquivo inexistente e recusado", st == 400, (st, r))
    st, r = req("/restaurar?arquivo=..%2F..%2Fetc%2Fpasswd", {}, "POST")
    check("restaurar nao aceita caminho fora da pasta", st in (400, 404), (st, r))

    secao("rotas com lixo (nada pode dar 500)")
    for rota in ["/painel", "/pecas/abc", "/vendas/0", "/clientes/999999", "/clientes/abc",
                 "/fornecedores/999999", "/fichas/999999", "/fichas/abc", "/pagaveis/abc",
                 "/relatorios/vendas?de=xx&ate=yy", "/relatorios/banana", "/catalogo?q=" + urllib.parse.quote(";drop table parts"),
                 "/pecas?q=%00", "/estoque/movimentos?limite=abc", "/fechamento?data=banana",
                 "/notas-entrada", "/rede"]:
        nao_quebrou("GET " + rota, rota)

    print()
    print(f"RESUMO: {total - len(falhas)} OK, {len(falhas)} falha(s) de {total} verificacoes")
    if falhas:
        for f in falhas:
            print("  - " + f)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

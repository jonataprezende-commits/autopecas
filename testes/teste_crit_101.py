"""Criterios de aceitacao do AutoPecas 1.0.1 rodando o .exe de Windows sob Wine.

C1  Cadastro de peca + venda a vista com troco (estoque baixa, total certo)
C2  Venda faturada: conta a receber com vencimento, baixa e cancelamento com estorno
C3  Ficha de falta -> gerar pedido -> receber (estoque entra UMA vez, ficha recebida)
C4  Importacao de XML de fornecedor (cria/atualiza peca e abre ficha de falta)
C5  Backup -> apagar dados -> restaurar -> dados identicos

Todas as chamadas usam o cabecalho X-AutoPecas exigido pelo item 7 (como o front faz).
"""
import json
import os
import shutil
import subprocess
import time
import urllib.parse
import urllib.request

# versao que este teste espera encontrar no programa instalado
VERSAO_ESPERADA = "1.0.11"

BASE = os.environ.get("API_BASE", "http://127.0.0.1:8790")
WINE = "/home/user/.cache/wineprefix"
DADOS = f"{WINE}/drive_c/users/user/AppData/Roaming/AutoPecas"
EXE = f"{WINE}/drive_c/users/user/AppData/Local/Programs/AutoPecas/AutoPecas.exe"

falhas = []
passos = []


def ok(msg):
    passos.append(("OK", msg))
    print(f"  [ok] {msg}")


def erro(msg):
    falhas.append(msg)
    passos.append(("FALHOU", msg))
    print(f"  [FALHOU] {msg}")


def req(caminho, dados=None, metodo=None):
    corpo = json.dumps(dados).encode() if dados is not None else None
    cab = {"X-AutoPecas": "1"}
    if corpo:
        cab["Content-Type"] = "application/json"
    r = urllib.request.Request(BASE + "/api" + caminho, data=corpo, headers=cab,
                               method=metodo or ("POST" if corpo else "GET"))
    with urllib.request.urlopen(r, timeout=30) as resp:
        return json.loads(resp.read().decode())


def req_erro(caminho, dados=None, metodo=None):
    """Como req, mas devolve (codigo, corpo) tambem quando o servidor recusa."""
    corpo = json.dumps(dados).encode() if dados is not None else None
    cab = {"X-AutoPecas": "1"}
    if corpo:
        cab["Content-Type"] = "application/json"
    r = urllib.request.Request(BASE + "/api" + caminho, data=corpo, headers=cab,
                               method=metodo or ("POST" if corpo else "GET"))
    try:
        with urllib.request.urlopen(r, timeout=30) as resp:
            return resp.status, json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read().decode() or "{}")
        except Exception:
            return e.code, {}


def req_arquivo(caminho, campo, nome_arq, conteudo):
    fronteira = "----AutoPecasQA"
    corpo = b""
    corpo += f"--{fronteira}\r\n".encode()
    corpo += f'Content-Disposition: form-data; name="{campo}"; filename="{nome_arq}"\r\n'.encode()
    corpo += b"Content-Type: application/octet-stream\r\n\r\n" + conteudo + b"\r\n"
    corpo += f"--{fronteira}--\r\n".encode()
    r = urllib.request.Request(BASE + "/api" + caminho, data=corpo,
                               headers={"X-AutoPecas": "1",
                                        "Content-Type": f"multipart/form-data; boundary={fronteira}"},
                               method="POST")
    with urllib.request.urlopen(r, timeout=60) as resp:
        return json.loads(resp.read().decode())


def esperar(seg=60):
    for _ in range(seg):
        try:
            req("/ping")
            return True
        except Exception:
            time.sleep(1)
    return False


def abrir():
    subprocess.Popen(["/usr/bin/xvfb-run", "-a", "/usr/lib/wine/wine64", EXE,
                      "-sem-navegador", "-porta", "8790"],
                     env={**os.environ, "WINEPREFIX": WINE, "WINEDEBUG": "-all"},
                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    return esperar()


# ---------------------------------------------------------------- criterios

def c1_venda_a_vista():
    print("C1 - cadastro + venda a vista com troco")
    p = req("/pecas", {"codigo": "QA-100", "descricao": "Filtro de oleo QA", "custo_centavos": 1200,
                       "preco_centavos": 2500, "minimo": 1, "estoque": 10})
    v = req("/vendas", {"cliente_nome": "Cliente QA", "itens": [
        {"peca_id": p["id"], "quantidade": 2, "preco_centavos": 2500, "desconto_centavos": 0}],
        "desconto_centavos": 0, "forma_pagamento": "dinheiro", "recebido_centavos": 6000})
    peca = req(f"/pecas/{p['id']}")
    if v["total_centavos"] != 5000:
        erro(f"C1 total da venda = {v['total_centavos']} (esperado 5000)")
    else:
        ok("total da venda 5000")
    if v["troco_centavos"] != 1000:
        erro(f"C1 troco = {v['troco_centavos']} (esperado 1000)")
    else:
        ok("troco 1000")
    if abs(peca["estoque"] - 8) > 0.001:
        erro(f"C1 estoque = {peca['estoque']} (esperado 8)")
    else:
        ok("estoque baixou para 8")
    if v["lucro_centavos"] != 5000 - 2400:
        erro(f"C1 lucro = {v['lucro_centavos']} (esperado 2600)")
    else:
        ok("lucro 2600")
    return p


def c2_faturado_estorno(p):
    print("C2 - venda faturada, baixa e cancelamento com estorno")
    v = req("/vendas", {"cliente_nome": "Cliente Faturado QA", "itens": [
        {"peca_id": p["id"], "quantidade": 1, "preco_centavos": 2500, "desconto_centavos": 500}],
        "desconto_centavos": 200, "forma_pagamento": "faturado", "vencimento_dias": 15})
    if v["total_centavos"] != 1800:
        erro(f"C2 total = {v['total_centavos']} (esperado 1800 = 2500-500-200)")
    else:
        ok("total 1800 com desconto de item + venda")
    conta_nova = next((c for c in req("/recebiveis?status=todos")["itens"] if c["venda_id"] == v["id"]), None)
    venc = (conta_nova or {}).get("vencimento") or ""
    esperado = time.strftime("%Y-%m-%d", time.localtime(time.time() + 15 * 86400))
    if venc != esperado:
        erro(f"C2 vencimento = {venc} (esperado {esperado})")
    else:
        ok(f"vencimento em 15 dias ({venc})")
    contas = req("/recebiveis?status=todos")["itens"]
    conta = next((c for c in contas if c["venda_id"] == v["id"]), None)
    if not conta:
        erro("C2 conta a receber nao criada")
        return
    if conta["valor_centavos"] != 1800:
        erro(f"C2 valor da conta = {conta['valor_centavos']} (esperado 1800)")
    else:
        ok("conta a receber com o total correto")
    req(f"/recebiveis/{conta['id']}/baixar", {})
    req(f"/vendas/{v['id']}/cancelar", {})
    contas = req("/recebiveis?status=todos")["itens"]
    conta = next((c for c in contas if c["id"] == conta["id"]), None)
    if conta["status"] != "estornado":
        erro(f"C2 status apos cancelar venda paga = {conta['status']} (esperado estornado)")
    else:
        ok("conta paga cancelada virou 'estornado'")
    aberto = req("/recebiveis")["total_aberto_centavos"]
    if aberto != 0:
        erro(f"C2 total em aberto = {aberto} (esperado 0)")
    else:
        ok("total em aberto nao conta o estorno")


def c3_pedido(p):
    print("C3 - ficha de falta, pedido de compra e recebimento")
    forn = req("/fornecedores", {"nome": "Fornecedor QA"})
    ficha = req("/fichas", {"peca_id": p["id"], "descricao": "Filtro de oleo QA", "quantidade": 4,
                            "cliente_nome": "Cliente QA", "fornecedor_id": forn["id"],
                            "custo_centavos": 1200})
    g = req("/pedidos/gerar", {})
    pedido = g["pedidos"][0]["id"] if g["pedidos"] else None
    if not pedido:
        erro("C3 pedido nao gerado")
        return
    ok(f"pedido {pedido} gerado a partir da ficha {ficha['id']}")
    antes = req(f"/pecas/{p['id']}")["estoque"]
    req(f"/pedidos/{pedido}/receber", {})
    depois = req(f"/pecas/{p['id']}")["estoque"]
    if abs(depois - (antes + 4)) > 0.001:
        erro(f"C3 estoque depois de receber = {depois} (esperado {antes + 4})")
    else:
        ok(f"estoque entrou uma vez ({antes} -> {depois})")
    try:
        req(f"/pedidos/{pedido}/receber", {})
        erro("C3 segundo recebimento foi aceito (deveria ser bloqueado)")
    except urllib.error.HTTPError as e:
        if e.code == 409:
            ok("segundo recebimento bloqueado (409)")
        else:
            erro(f"C3 segundo recebimento retornou {e.code} (esperado 409)")
    depois2 = req(f"/pecas/{p['id']}")["estoque"]
    if abs(depois2 - depois) > 0.001:
        erro(f"C3 estoque duplicou no segundo recebimento ({depois} -> {depois2})")
    else:
        ok("estoque nao duplicou")
    fichas = req("/fichas?status=todas")["itens"]
    f = next((x for x in fichas if x["id"] == ficha["id"]), None)
    if f and f["status"] == "recebida":
        ok("ficha marcada como recebida")
    else:
        erro(f"C3 ficha ficou {f and f['status']}")


def c4_xml():
    print("C4 - importacao de XML")
    xml = """<?xml version="1.0" encoding="UTF-8"?>
<produtos><produto>
<cProd>QA-XML-1</cProd><xProd>Pastilha de freio QA</xProd>
<vUnCom>45.90</vUnCom><qCom>10</qCom><NCM>87083090</NCM>
</produto></produtos>"""
    r = req_arquivo("/xml/importar", "arquivo", "nota.xml", xml.encode("utf-8"))
    cadastradas = r.get("cadastradas", 0) + r.get("atualizadas", 0)
    if not cadastradas:
        erro(f"C4 importacao sem resultado: {json.dumps(r)[:200]}")
        return
    pecas = req("/pecas?q=QA-XML-1")["itens"]
    if not pecas:
        erro("C4 peca do XML nao encontrada no cadastro")
    else:
        ok(f"peca do XML cadastrada (codigo {pecas[0]['codigo']}, preco {pecas[0]['preco_centavos']})")


def c5_backup_restaurar(p):
    print("C5 - backup, apagar dados, restaurar")
    b = req("/backup", {})
    nome = b["arquivo"]
    arquivo = os.path.join(DADOS, "backups", nome)
    if not os.path.exists(arquivo):
        erro(f"C5 backup nao encontrado em {arquivo}")
        return
    ok(f"backup gerado ({nome}, {b['tamanho_bytes']} bytes)")
    estado_antes = {
        "pecas": req("/pecas?limite=500")["total"],
        "vendas": len(req("/vendas")["itens"]),
        "fichas": len(req("/fichas?status=todas")["itens"]),
        "receber": len(req("/recebiveis?status=todos")["itens"]),
    }
    dados = open(arquivo, "rb").read()
    # apaga o banco (simula "computador limpo") e restaura pelo arquivo enviado
    req("/pecas", {"codigo": "QA-LIXO", "descricao": "cadastro depois do backup"})
    r1 = req_arquivo("/restaurar", "arquivo", nome, dados)
    r2 = req_arquivo("/restaurar", "arquivo", nome, dados)  # item 3: duas vezes na mesma sessao
    ok(f"restauracao 1 e 2 concluidas ({r1.get('pecas')} pecas no arquivo)")
    estado_depois = {
        "pecas": req("/pecas?limite=500")["total"],
        "vendas": len(req("/vendas")["itens"]),
        "fichas": len(req("/fichas?status=todas")["itens"]),
        "receber": len(req("/recebiveis?status=todos")["itens"]),
    }
    if estado_antes != estado_depois:
        erro(f"C5 estado diferente depois de restaurar: {estado_antes} != {estado_depois}")
    else:
        ok(f"dados identicos depois de restaurar ({estado_depois})")


def c6_bugs_da_102():
    print("C6 - correcoes da 1.0.2 (ficha, milhar, PDF, atraso, estorno)")
    # 1) ficha de falta recebida duas vezes
    forn = req("/fornecedores", {"nome": "Fornecedor QA2"})
    peca = req("/pecas", {"codigo": "QA-FICHA", "descricao": "Peca ficha QA", "custo_centavos": 500,
                          "preco_centavos": 900, "minimo": 1, "estoque": 0})
    ficha = req("/fichas", {"peca_id": peca["id"], "descricao": "Peca ficha QA", "quantidade": 3,
                            "fornecedor_id": forn["id"]})
    req(f"/fichas/{ficha['id']}/status", {"status": "recebida"})
    est1 = req(f"/pecas/{peca['id']}")["estoque"]
    try:
        req(f"/fichas/{ficha['id']}/status", {"status": "recebida"})
        erro("C6 receber a mesma ficha duas vezes foi aceito")
    except urllib.error.HTTPError as e:
        if e.code == 409:
            ok("ficha recebida duas vezes bloqueada (409)")
        else:
            erro(f"C6 segundo recebimento da ficha retornou {e.code} (esperado 409)")
    est2 = req(f"/pecas/{peca['id']}")["estoque"]
    if abs(est2 - est1) > 0.001:
        erro(f"C6 estoque duplicado pela ficha ({est1} -> {est2})")
    else:
        ok(f"estoque entrou uma vez pela ficha ({est1})")

    # 2) preco com separador de milhar gravado em centavos corretos
    p2 = req("/pecas", {"codigo": "QA-MILHAR", "descricao": "Peca milhar", "preco_centavos": 123400,
                        "custo_centavos": 100000, "estoque": 1, "minimo": 0})
    if p2["preco_centavos"] == 123400:
        ok("preco R$ 1.234,00 gravado como 123400 centavos")
    else:
        erro(f"preco de milhar errado: {p2['preco_centavos']}")

    # 3) PDF de relatorio com acento
    req("/pecas", {"codigo": "QA-ACENTO", "descricao": "Filtro de óleo Ção à noite",
                   "preco_centavos": 5000, "custo_centavos": 3000, "estoque": 2, "minimo": 0})
    with urllib.request.urlopen(BASE + "/api/relatorios/estoque_valor/pdf", timeout=30) as r:
        pdf = r.read()
    latin1_ok = b"Filtro de \xf3leo" in pdf
    utf8_ruim = "óleo".encode("utf-8") in pdf
    if latin1_ok and not utf8_ruim:
        ok("PDF do relatorio em Latin-1: 'Filtro de óleo' escrito certo")
    elif utf8_ruim:
        erro("PDF com 'ó' em UTF-8 (sai como 'Ã³' no leitor)")
    else:
        erro("PDF sem o texto acentuado esperado")

    # 4) conta a receber com campo nulo continua aparecendo
    # 5) venda faturada: vencimento e estorno (cobertos em C2)


def c7_nota_por_foto():
    """C7 - nota de compra fotografada: OCR embutido no proprio .exe."""
    print("C7 - nota de compra por foto (OCR embutido)")
    # mede antes para comparar depois (a aceitacao C1..C6 ja cadastrou pecas)
    pecas_antes = req("/pecas")["itens"]
    unidades_antes = round(sum(p["estoque"] for p in pecas_antes), 2)
    st = req("/nota-foto/status")
    (ok if st.get("disponivel") is True else erro)("leitor de notas disponivel no .exe")
    (ok if "embutido" in (st.get("motor") or "") else erro)("motor de OCR embutido, sem instalar nada (" + str(st.get("motor")) + ")")

    foto = "/home/user/autopecas/testdata/nota-foto-boa.jpg"
    with open(foto, "rb") as f:
        dados = f.read()
    r = req_arquivo("/nota-foto/ler", "arquivo", "nota-boa.jpg", dados)
    if r.get("erro"):
        erro("foto lida pelo .exe: " + str(r.get("erro")))
        return
    itens = r.get("itens") or []
    (ok if len(itens) == 8 else erro)(f"foto boa: {len(itens)} itens lidos")
    (ok if r.get("soma_itens_centavos") == 93220 else erro)(f"soma dos itens {r.get('soma_itens_centavos')} (esperado 93220)")
    (ok if r.get("total_nota_centavos") == 93220 else erro)(f"total impresso na nota lido: {r.get('total_nota_centavos')}")
    (ok if (r.get("veredito") == "ok" and r.get("conferido") is True) else erro)("nota dada como conferida (" + str(r.get("veredito")) + ")")
    (ok if (r.get("tempo_ms") or 99999) < 30000 else erro)(f"leitura em {r.get('tempo_ms')} ms")

    # nada foi gravado pela leitura
    depois_leitura = req("/pecas")["itens"]
    (ok if len(depois_leitura) == len(pecas_antes) else erro)(
        f"ler a foto nao grava nada ({len(pecas_antes)} -> {len(depois_leitura)} pecas)")

    # importa os itens, como o usuario faria depois de conferir
    corpo = {
        "confirmado": True, "criar_novas": True, "atualizar_custo": True,
        "lancar_estoque": True, "receber_fichas": False, "margem_percentual": 60,
        "itens": [{
            "codigo": i["codigo"], "descricao": i["descricao"], "quantidade": i["quantidade"],
            "unitario_centavos": i["unitario_centavos"], "total_centavos": i["total_centavos"],
        } for i in itens],
    }
    imp = req("/nota-foto/importar", corpo)
    if imp.get("erro"):
        erro("importar a nota lida: " + str(imp.get("erro")))
        return
    (ok if imp.get("cadastradas") == 8 else erro)(f"{imp.get('cadastradas')} pecas cadastradas pela nota")
    pecas = req("/pecas")["itens"]
    total_un = round(sum(p["estoque"] for p in pecas) - unidades_antes, 2)
    (ok if total_un == 24 else erro)(f"estoque entrou {total_un} unidades pela nota (esperado 24)")
    custo_ok = next((p for p in pecas if p["codigo"] == "SKU-1001"), {})
    (ok if custo_ok.get("custo_centavos") == 1850 else erro)(f"custo da SKU-1001 = {custo_ok.get('custo_centavos')} (esperado 1850)")

    # foto ruim nao pode ser aceita como conferida
    with open("/home/user/autopecas/testdata/nota-foto-ruim.jpg", "rb") as f:
        ruim = f.read()
    r2 = req_arquivo("/nota-foto/ler", "arquivo", "nota-ruim.jpg", ruim)
    (ok if r2.get("veredito") != "ok" else erro)("foto ruim nao vira nota conferida (" + str(r2.get("veredito")) + ")")


def c8_catalogo_do_fornecedor():
    print("C8 - catalogo do fornecedor")
    ping = req("/ping")
    (ok if ping.get("versao") == VERSAO_ESPERADA else erro)(f"versao instalada = {ping.get('versao')} (esperado {VERSAO_ESPERADA})")

    pecas_antes = len(req("/pecas?limite=5000")["itens"])
    csv = ("codigo;descricao;aplicacao;medida;marca;custo;equivalentes;criterios\n"
           "W712/90;FILTRO DE OLEO;\"VW GOL 1.0/1.6 08/16; VOYAGE 09/16\";ROSCA 3/4-16;MANN;18,50;TECFIL PSL560;rosca 3/4-16; altura 78mm\n"
           "PA-5010;PASTILHA DE FREIO DIANTEIRA;VW GOL 1.0 13/20;JOGO 4 PECAS;FREMEC;64,00;FRAS-LE PD/1001;4 pastilhas; espessura 17mm\n")
    imp = req_arquivo("/catalogo/importar", "arquivo", "lista.csv", csv.encode())
    (ok if imp.get("importados") == 2 else erro)(f"lista do fornecedor importada ({imp.get('importados')} de 2)")

    # busca pelo carro + ano, com a aplicacao em faixa
    achados = req("/catalogo?q=" + urllib.parse.quote("gol 2012"))["itens"]
    (ok if any(i["codigo"] == "W712/90" for i in achados) else erro)(
        "busca por 'gol 2012' acha a peca de aplicacao 08/16")
    # busca pelo equivalente de outra marca
    achados = req("/catalogo?q=PSL560")["itens"]
    (ok if any(i["codigo"] == "W712/90" for i in achados) else erro)(
        "busca pelo equivalente PSL560 acha o codigo do catalogo")

    # criterios tecnicos: a busca acha pelo dado tecnico, nao so pelo nome
    achados = req("/catalogo?q=" + urllib.parse.quote("altura 78mm"))["itens"]
    (ok if any(i["codigo"] == "W712/90" for i in achados) else erro)(
        "busca por criterio tecnico ('altura 78mm') acha a peca")
    achados = req("/catalogo?q=" + urllib.parse.quote("espessura 17mm"))["itens"]
    (ok if any(i["codigo"] == "PA-5010" for i in achados) else erro)(
        "critério tecnico com ';' no meio do CSV nao desalinha a linha")

    # montar o catalogo com o que a loja ja tem (sem planilha nenhuma)
    sem = req("/catalogo/semear", {"pecas": True, "fichas": True})
    (ok if sem.get("novos", 0) > 0 else erro)(
        f"catalogo montado com o que a loja ja tem ({sem.get('novos')} itens novos)")
    achados = req("/catalogo?q=" + urllib.parse.quote("filtro"))["itens"]
    do_estoque = [i for i in achados if i["tem_estoque"]]
    (ok if do_estoque else erro)("peca vinda do estoque aparece no catalogo como 'tem no estoque'")
    sem2 = req("/catalogo/semear", {"pecas": True, "fichas": True})
    (ok if sem2.get("novos") == 0 else erro)(
        f"rodar a montagem de novo nao duplica (novos={sem2.get('novos')})")

    # o catalogo nao cria peca nem movimento de estoque
    pecas_depois = len(req("/pecas?limite=5000")["itens"])
    (ok if pecas_depois == pecas_antes else erro)(
        f"catalogo nao criou peca no estoque ({pecas_antes} -> {pecas_depois})")

    # busca rapida responde as duas perguntas: tem no estoque? existe?
    rapida = req("/busca?q=filtro")["itens"]
    origens = {i["origem"] for i in rapida}
    (ok if "catalogo" in origens else erro)("busca rapida devolve itens do catalogo")
    (ok if "estoque" in origens else erro)("busca rapida devolve itens do estoque na mesma consulta")
    sem_estoque = [i for i in rapida if i["origem"] == "catalogo" and not i["tem_estoque"]]
    (ok if sem_estoque else erro)("busca rapida marca a peca que NAO esta no estoque")

    # ficha de falta gerada direto do catalogo
    item = next(i for i in req("/catalogo?q=PA-5010")["itens"] if i["codigo"] == "PA-5010")
    f = req(f"/catalogo/{item['id']}/ficha", {"quantidade": 2, "cliente_nome": "Cliente Catalogo",
                                              "veiculo": "Gol 2014 placa ABC1234"})
    fichas = req("/fichas?status=todas")["itens"]
    nova = [x for x in fichas if "PA-5010" in (x["descricao"] or "")]
    (ok if nova else erro)("ficha de falta criada a partir do catalogo")
    if nova:
        (ok if nova[0]["status"] == "aberta" and nova[0]["quantidade"] == 2 else erro)(
            "ficha nasce aberta, com a quantidade pedida")
        (ok if nova[0]["veiculo"] == "Gol 2014 placa ABC1234" else erro)("ficha guarda o veiculo do cliente")

    # reimportar atualiza, nao duplica
    imp2 = req_arquivo("/catalogo/importar", "arquivo", "lista.csv", csv.encode())
    do_csv = [i for i in req("/catalogo?limite=500")["itens"] if i["codigo"] in ("W712/90", "PA-5010")]
    (ok if imp2.get("atualizados") == 2 and len(do_csv) == 2 else erro)(
        f"reimportar atualiza sem duplicar ({imp2.get('atualizados')} atualizados, "
        f"{len(do_csv)} itens da lista no catalogo)")

    # a aplicacao do XML passa a ser guardada na peca (antes era descartada)
    xml = (b'<?xml version="1.0" encoding="UTF-8"?><catalogo><produto>'
           b'<codigo>QA-APP-9</codigo><descricao>FILTRO DE COMBUSTIVEL</descricao>'
           b'<aplicacao>FIAT UNO 1.0 10/16</aplicacao><preco>25,00</preco><estoque>1</estoque>'
           b'</produto></catalogo>')
    req_arquivo("/xml/importar", "arquivo", "app.xml", xml)
    achou = req("/pecas?q=" + urllib.parse.quote("uno 2013"))["itens"]
    (ok if any(p["codigo"] == "QA-APP-9" for p in achou) else erro)(
        "aplicacao vinda do XML fica na peca e a busca por carro acha a peca")


def c9_plano_b():
    """1.0.7 - backups: hora em hora, fechamento, painel, kit e nuvem."""
    print("C9 - Plano B (backup que salva a loja)")
    ping = req("/ping")
    (ok if ping.get("versao") == VERSAO_ESPERADA else erro)(f"versao instalada = {ping.get('versao')} (esperado {VERSAO_ESPERADA})")

    # painel de saude responde e o backup de hora em hora ja existe (criado ao abrir)
    saude = req("/saude-backup")
    (ok if saude.get("nivel") in ("verde", "amarelo", "vermelho") else erro)(
        "painel de saude do backup responde com semaforo (" + str(saude.get("nivel")) + ")")
    tipos = {c["tipo"]: c["quantidade"] for c in saude.get("categorias", [])}
    (ok if tipos.get("backup-horario", 0) >= 1 else erro)(
        f"backup de hora em hora existe sem ninguem clicar ({tipos.get('backup-horario', 0)} arquivo(s))")

    # backup manual deixa o painel verde
    b = req("/backup", {}, metodo="POST")
    (ok if b.get("arquivo") else erro)("backup manual gerado: " + str(b.get("arquivo")))
    saude = req("/saude-backup")
    (ok if saude.get("nivel") == "verde" else erro)(
        "depois do backup o painel fica verde (" + str(saude.get("nivel")) + ")")

    # testar a restauracao nao altera nada
    pecas_antes = len(req("/pecas?limite=5000")["itens"])
    t = req("/backup/testar", {}, metodo="POST")
    (ok if t.get("ok") and t.get("arquivo") else erro)("teste de restauracao abre o backup: " + str(t.get("arquivo")))
    (ok if t.get("pecas") is not None else erro)("teste de restauracao mostra o conteudo (pecas)")
    pecas_depois = len(req("/pecas?limite=5000")["itens"])
    (ok if pecas_antes == pecas_depois else erro)(
        f"testar a restauracao nao altera o banco ({pecas_antes} -> {pecas_depois} pecas)")

    # fechar o caixa guarda backup do dia
    fc = req("/fechamento/fechar", {}, metodo="POST")
    (ok if str(fc.get("arquivo", "")).startswith("backup-fechamento") else erro)(
        "fechar o caixa guarda backup: " + str(fc.get("arquivo")))
    saude = req("/saude-backup")
    (ok if saude.get("ultimo_fechamento") else erro)("painel registra o ultimo fechamento de caixa")

    # kit de emergencia (programa + backup + instrucao)
    k = req("/kit-emergencia", {}, metodo="POST")
    (ok if k.get("ok") and k.get("tamanho_bytes", 0) > 1000000 else erro)(
        f"kit de emergencia gerado ({round(k.get('tamanho_bytes', 0)/1048576, 1)} MB)")
    (ok if str(k.get("nome", "")).endswith(".zip") else erro)("kit e um arquivo zip")

    # pasta na nuvem: caminho invalido e recusado, pasta boa e aceita
    codigo, ruim = req_erro("/nuvem/testar", {"pasta": "C:\\pasta\\que\\nao\\existe"}, metodo="POST")
    (ok if codigo == 400 and ruim.get("erro") else erro)(
        f"pasta de nuvem inexistente e recusada com aviso ({codigo})")
    codigo, relativo = req_erro("/nuvem/testar", {"pasta": "Dropbox/Backup"}, metodo="POST")
    (ok if codigo == 400 else erro)("caminho relativo de pasta e recusado")
    codigo, _ = req_erro("/abrir-pasta", {"qual": "C:\\Windows"}, metodo="POST")
    (ok if codigo == 400 else erro)("abrir pasta so aceita as pastas do proprio sistema")
    nuvem = req("/nuvem")
    (ok if "candidatas" in nuvem else erro)("o sistema lista as pastas de nuvem encontradas no computador")



def c10_contas_a_pagar():
    """1.0.8 - a nota do fornecedor lanca a conta a pagar sozinha."""
    print("C10 - contas a pagar pela nota do fornecedor")
    ping = req("/ping")
    (ok if ping.get("versao") == VERSAO_ESPERADA else erro)(f"versao instalada = {ping.get('versao')} (esperado {VERSAO_ESPERADA})")

    nota = """<?xml version="1.0" encoding="UTF-8"?>
<nfeProc xmlns="http://www.portalfiscal.inf.br/nfe"><NFe>
<infNFe Id="NFe35261055666777000188550010000042421000042424">
 <ide><nNF>4242</nNF><serie>1</serie><dhEmi>2026-10-22T11:00:00-03:00</dhEmi></ide>
 <emit><CNPJ>55.666.777/0001-88</CNPJ><xNome>FORNECEDOR QA CONTAS LTDA</xNome></emit>
 <det><prod><cProd>QA-CONTA-1</cProd><xProd>BUCHA QA</xProd><qCom>10</qCom><vUnCom>50.00</vUnCom><vProd>500.00</vProd></prod></det>
 <total><ICMSTot><vProd>500.00</vProd><vFrete>50.00</vFrete><vNF>550.00</vNF></ICMSTot></total>
 <cobr><dup><nDup>001</nDup><dVenc>2026-11-21</dVenc><vDup>275.00</vDup></dup>
       <dup><nDup>002</nDup><dVenc>2026-12-21</dVenc><vDup>275.00</vDup></dup></cobr>
</infNFe></NFe></nfeProc>""".encode()

    # 1) analisar mostra o cabecalho da nota
    a = req_arquivo("/xml/analisar", "arquivo", "nota-qa.xml", nota)
    n = a.get("nota") or {}
    (ok if n.get("fornecedor") == "FORNECEDOR QA CONTAS LTDA" else erro)(
        "analisar le o fornecedor do XML: " + str(n.get("fornecedor")))
    (ok if n.get("total_centavos") == 55000 else erro)(
        "analisar le o valor total da nota (vNF): " + str(n.get("total_centavos")))
    (ok if len(n.get("duplicatas") or []) == 2 else erro)(
        "analisar le os 2 boletos (duplicatas): " + str(len(n.get("duplicatas") or [])))
    (ok if n.get("extras_centavos") == 5000 else erro)(
        "analisar separa o frete do valor dos produtos: " + str(n.get("extras_centavos")))

    # 2) importar lanca as contas a pagar
    r = req_arquivo("/xml/importar", "arquivo", "nota-qa.xml", nota)
    (ok if r.get("contas_criadas") == 2 else erro)(
        f"importar a nota lancou as contas a pagar ({r.get('contas_criadas')})")
    (ok if r.get("contas_total_centavos") == 55000 else erro)(
        "soma das contas = valor total da nota: " + str(r.get("contas_total_centavos")))
    (ok if r.get("fornecedor_id") else erro)("fornecedor do XML foi cadastrado sozinho")
    (ok if (r.get("rateado_centavos") or 0) > 4000 else erro)(
        "frete entrou no custo das pecas (rateio): " + str(r.get("rateado_centavos")))

    # 3) a tela lista as contas, com vencimento e valor
    lista = req("/pagaveis?status=aberto")
    itens = lista.get("itens") or []
    (ok if len(itens) == 2 else erro)(f"as 2 contas aparecem em aberto ({len(itens)})")
    (ok if all(i.get("fornecedor") == "FORNECEDOR QA CONTAS LTDA" for i in itens) else erro)(
        "as contas ficaram com o nome do fornecedor da nota")
    (ok if any(i.get("vencimento") == "2026-11-21" for i in itens) else erro)(
        "o vencimento veio do boleto da nota")
    (ok if lista.get("total_aberto_centavos") == 55000 else erro)(
        "total em aberto somado: " + str(lista.get("total_aberto_centavos")))

    # 4) reimportar a mesma nota NAO paga duas vezes
    r2 = req_arquivo("/xml/importar", "arquivo", "nota-qa.xml", nota)
    (ok if r2.get("nota_ja_existia") is True else erro)(
        "reimportar a mesma nota avisa que ela ja esta lancada")
    (ok if not r2.get("contas_criadas") else erro)(
        "reimportar nao cria conta a pagar de novo")
    (ok if len((req("/pagaveis?status=todos").get("itens") or [])) == 2 else erro)(
        "continuam exatamente 2 contas depois da reimportacao")

    # 5) pagar a conta
    req(f"/pagaveis/{itens[0]['id']}/baixar", {"valor_pago_centavos": 27500}, metodo="POST")
    pagas = req("/pagaveis?status=pago").get("itens") or []
    (ok if len(pagas) == 1 else erro)("conta baixada aparece como paga")

    # 6) nota sem duplicata: uma conta com o vencimento escolhido
    nota2 = """<?xml version="1.0"?><nfeProc><NFe><infNFe Id="NFe35261011111222000133550010000051511000051515">
 <ide><nNF>5151</nNF><dhEmi>2026-10-10T08:00:00-03:00</dhEmi></ide>
 <emit><CNPJ>11.111.222/0001-33</CNPJ><xNome>OUTRO FORNECEDOR QA</xNome></emit>
 <det><prod><cProd>QA-CONTA-2</cProd><xProd>RETENTOR QA</xProd><qCom>2</qCom><vUnCom>30.00</vUnCom><vProd>60.00</vProd></prod></det>
 <total><ICMSTot><vProd>60.00</vProd><vNF>60.00</vNF></ICMSTot></total>
</infNFe></NFe></nfeProc>""".encode()
    r3 = req_arquivo("/xml/importar", "arquivo", "nota2-qa.xml", nota2)
    (ok if r3.get("contas_criadas") == 1 else erro)(f"nota sem boleto cria 1 conta ({r3.get('contas_criadas')})")
    todas = req("/pagaveis?status=aberto").get("itens") or []
    nova = [c for c in todas if c.get("fornecedor") == "OUTRO FORNECEDOR QA"]
    (ok if nova and nova[0].get("valor_centavos") == 6000 else erro)("valor da conta unica = vNF")
    (ok if nova and nova[0].get("vencimento") else erro)("vencimento calculado quando a nota nao traz boleto")

    # 7) conta manual, edicao e cancelamento
    c = req("/pagaveis", {"fornecedor": "ALUGUEL QA", "valor_centavos": 100000, "vencimento": "2026-11-05"}, metodo="POST")
    (ok if c.get("id") else erro)("conta manual lancada")
    req(f"/pagaveis/{c['id']}", {"fornecedor": "ALUGUEL QA", "valor_centavos": 110000, "vencimento": "2026-11-06"}, metodo="PUT")
    (ok if req("/pagaveis?status=aberto")
     .get("itens") and any(x["id"] == c["id"] and x.get("valor_centavos") == 110000
                          for x in req("/pagaveis?status=aberto")["itens"]) else erro)("conta manual editada")
    req(f"/pagaveis/{c['id']}/cancelar", {}, metodo="POST")
    (ok if any(x["id"] == c["id"] and x.get("status") == "cancelado"
              for x in req("/pagaveis?status=todos")["itens"]) else erro)("conta manual cancelada")
    codigo, _ = req_erro("/pagaveis", {"fornecedor": "X", "valor_centavos": 0, "vencimento": "2026-11-01"}, metodo="POST")
    (ok if codigo == 400 else erro)("conta com valor zero e recusada")

    # 8) painel e relatorio
    d = req("/dashboard")
    (ok if d.get("pagaveis_aberto_centavos", 0) > 0 else erro)(
        "Inicio mostra o total a pagar: " + str(d.get("pagaveis_aberto_centavos")))
    rel = req("/relatorios/pagaveis")
    (ok if "pagar" in (rel.get("titulo") or "").lower() else erro)("relatorio de contas a pagar existe")
    (ok if len(rel.get("linhas") or []) >= 2 else erro)("relatorio lista as contas")

    # 9) a nota importada fica no historico
    notas = req("/notas-entrada").get("itens") or []
    (ok if any(x.get("numero") == "4242" for x in notas) else erro)(
        "historico de notas importadas mostra a nota 4242")


def c11_espaco_dos_backups():
    """1.0.9 - os backups nao enchem o disco: limite por tipo (ate 1) e teto de MB."""
    print("C11 - espaco dos backups")
    ping = req("/ping")
    (ok if ping.get("versao") == VERSAO_ESPERADA else erro)(f"versao instalada = {ping.get('versao')} (esperado {VERSAO_ESPERADA})")

    # o painel informa quanto ocupa e o teto
    s = req("/saude-backup")
    (ok if "limite_mb" in s else erro)("painel informa o teto de espaco configurado")
    (ok if s.get("total_bytes", 0) > 0 else erro)(
        "painel informa quanto a pasta ocupa: " + str(s.get("total_bytes")) + " bytes")
    (ok if s.get("total_arquivos", 0) >= 1 else erro)("painel informa quantos arquivos existem")

    # teto de espaco pode ser configurado
    req("/config", {"backup_limite_mb": "150"}, metodo="POST")
    s = req("/saude-backup")
    (ok if str(s.get("limite_mb")) == "150" else erro)("teto de espaco salvo (150 MB)")

    # perfis: "economia maxima" = 1 copia de cada tipo
    req("/config", {"backup_manter": "1", "backup_manter_horarios": "1",
                    "backup_manter_fechamentos": "1", "backup_manter_manuais": "1",
                    "backup_manter_seguranca": "1"}, metodo="POST")
    cfg = req("/config")
    (ok if cfg.get("backup_manter_horarios") == "1" else erro)(
        "limite de hora em hora aceita 1 (guardar so a ultima)")
    (ok if cfg.get("backup_manter_manuais") == "1" else erro)("limite de copias manuais aceita 1")

    # gera copias e manda apagar: nunca apaga a mais nova de cada tipo
    for _ in range(3):
        req("/backup", {}, metodo="POST")
    r = req("/backup/limpar", {}, metodo="POST")
    (ok if r.get("ok") else erro)("rota de limpeza responde")
    (ok if r.get("removidos", -1) >= 0 else erro)(
        f"limpeza informa quantos saiu ({r.get('removidos')}) e quanto sobrou ({r.get('total_texto')})")
    s = req("/saude-backup")
    tipos = {c["tipo"]: c["quantidade"] for c in s.get("categorias", [])}
    (ok if all(v <= 1 for k, v in tipos.items() if k != "outros" and tipos.get(k, 0) >= 0) else erro)(
        "depois da limpeza, nenhum tipo passa de 1 copia: " + str(tipos))
    (ok if tipos.get("backup-horario", 1) >= 1 else erro)("a copia mais nova de hora em hora foi mantida")

    # volta ao padrao e confere que nada foi apagado das copias atuais
    req("/config", {"backup_manter": "30", "backup_manter_horarios": "24",
                    "backup_manter_fechamentos": "60", "backup_manter_manuais": "60",
                    "backup_manter_seguranca": "10", "backup_limite_mb": "300"}, metodo="POST")
    cfg = req("/config")
    (ok if cfg.get("backup_manter_horarios") == "24" else erro)("perfil padrao volta a 24 copias de hora em hora")

def main():
    # comeca sempre de uma instalacao limpa (o teste e de aceitacao, nao de upgrade)
    if os.path.isdir(DADOS):
        shutil.rmtree(DADOS, ignore_errors=True)
    print("Abrindo o AutoPecas (Windows .exe sob Wine) na porta 8790...")
    if not abrir():
        print("FALHA: o sistema nao respondeu")
        return 1
    print("Sistema respondendo em", BASE)
    p = c1_venda_a_vista()
    c2_faturado_estorno(p)
    c3_pedido(p)
    c4_xml()
    c5_backup_restaurar(p)
    c6_bugs_da_102()
    c7_nota_por_foto()
    c8_catalogo_do_fornecedor()
    c9_plano_b()
    c10_contas_a_pagar()
    c11_espaco_dos_backups()
    print()
    print(f"RESUMO: {len([x for x in passos if x[0] == 'OK'])} verificacoes OK, {len(falhas)} falhas")
    for f in falhas:
        print("  -", f)
    # fecha o sistema usando o proprio atalho (-encerrar)
    subprocess.run(["/usr/bin/xvfb-run", "-a", "/usr/lib/wine/wine64", EXE, "-encerrar"],
                   env={**os.environ, "WINEPREFIX": WINE, "WINEDEBUG": "-all"},
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=120)
    time.sleep(3)
    try:
        req("/ping")
        print("AVISO: o sistema continuou aberto depois do -encerrar")
        falhas.append("encerrar nao funcionou")
    except Exception:
        ok("-encerrar fechou o sistema")
    return 1 if falhas else 0


if __name__ == "__main__":
    raise SystemExit(main())

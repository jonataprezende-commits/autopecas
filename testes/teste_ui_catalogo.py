"""Teste de interface da tela "Catálogo do fornecedor" (1.0.4) com navegador real.

Prova, no navegador:
  1. a tela abre pelo menu e a busca rápida do balcão aparece
  2. importar a lista do fornecedor (CSV) pela tela grava as peças
  3. "gol 2012" acha a peça pela aplicação (que está em faixa 08/16)
  4. a busca diz "não tenho" para peça fora do estoque e mostra a quantidade
     quando a peça está na prateleira
  5. gerar ficha de falta pela tela cria a ficha e NÃO mexe no estoque
  6. reimportar o mesmo arquivo não duplica
  7. busca por critério técnico e montagem do catálogo sem planilha
     ("Montar com o que já tenho")
  8. nenhum erro de JavaScript no console

Uso: UI_BASE=http://127.0.0.1:8912 python3 qa/teste_ui_catalogo.py
"""
import asyncio
import json
import os
import urllib.request

from playwright.async_api import async_playwright

BASE = os.environ.get("UI_BASE", "http://127.0.0.1:8912")
SHOT = "/home/user/qa/shots"
CSV_EXEMPLO = "/home/user/autopecas/exemplos/catalogo-exemplo.csv"
os.makedirs(SHOT, exist_ok=True)
resultado = []


def ok(msg):
    resultado.append((True, msg))
    print(f"  [ok] {msg}")


def erro(msg):
    resultado.append((False, msg))
    print(f"  [FALHOU] {msg}")


def api(caminho, dados=None, metodo=None):
    corpo = json.dumps(dados).encode() if dados is not None else None
    cab = {"X-AutoPecas": "1"}
    if corpo:
        cab["Content-Type"] = "application/json"
    r = urllib.request.Request(BASE + "/api" + caminho, data=corpo, headers=cab,
                               method=metodo or ("POST" if corpo else "GET"))
    with urllib.request.urlopen(r, timeout=120) as resp:
        return json.loads(resp.read().decode())


async def texto_da_lista(pg, seletor):
    return await pg.inner_text(seletor)


async def main():
    async with async_playwright() as p:
        nav = await p.chromium.launch()
        pg = await nav.new_page(viewport={"width": 1440, "height": 1000})
        erros_console = []
        pg.on("console", lambda m: erros_console.append(m.text) if m.type == "error" else None)
        pg.on("pageerror", lambda e: erros_console.append(str(e)))

        # ---------- 1. a tela abre pelo menu ----------
        await pg.goto(BASE + "/#/inicio", wait_until="load")
        await pg.wait_for_timeout(900)
        await pg.click('a[data-rota="catalogo"]')
        await pg.wait_for_timeout(900)
        if await pg.query_selector("#busca-rapida"):
            ok("a tela Catálogo abre pelo menu e mostra a busca rápida")
        else:
            erro("a tela Catálogo não abriu")
            await nav.close()
            return
        await pg.screenshot(path=f"{SHOT}/catalogo-1-inicial.png")

        # ---------- 2. importar a lista do fornecedor pela tela ----------
        await pg.click('button:has-text("Importar lista do fornecedor")')
        await pg.wait_for_timeout(500)
        await pg.set_input_files("#arq-catalogo", CSV_EXEMPLO)
        await pg.click('.modal-foot button:has-text("Importar")')
        await pg.wait_for_timeout(1500)
        res_import = " ".join((await texto_da_lista(pg, "#res-import-catalogo")).split())
        # o total do catálogo tem de ficar com as 5 peças da lista (a base pode já ter
        # alguma delas, então contamos novas + atualizadas)
        novas = int(res_import.split("peça(s) nova(s)")[0].split()[-1].replace(".", "") or 0) if "nova(s)" in res_import else -1
        atualizadas = int(res_import.split("peça(s) nova(s),")[1].split("atualizada(s)")[0].split()[0]) if "atualizada(s)" in res_import else -1
        total = len(api("/catalogo?limite=500")["itens"])
        if novas + atualizadas == 5 and total >= 5:
            ok(f"a lista do fornecedor entrou pela tela ({novas} nova(s), {atualizadas} atualizada(s))")
        else:
            erro("importação pela tela não confirmou as 5 peças: " + res_import[:120])
        await pg.screenshot(path=f"{SHOT}/catalogo-2-importado.png")
        await pg.click('.modal-foot button:has-text("Fechar")')
        await pg.wait_for_timeout(800)

        # ---------- 3. busca rápida por carro + ano ----------
        await pg.fill("#busca-rapida", "gol 2012")
        await pg.wait_for_timeout(900)
        txt = " ".join((await texto_da_lista(pg, "#resultado-rapida")).split())
        if "W712/90" in txt or "AF-1020" in txt:
            ok("busca “gol 2012” acha peça cuja aplicação está em faixa (08/16)")
        else:
            erro("busca “gol 2012” não achou nada: " + txt[:140])
        if "não tenho" in txt.lower():
            ok("o resultado avisa que a peça NÃO está no estoque")
        else:
            erro("o resultado não avisou que não há estoque")
        await pg.screenshot(path=f"{SHOT}/catalogo-3-busca-por-carro.png")

        # ---------- 4. busca por equivalente de outra marca ----------
        await pg.fill("#busca-rapida", "PSL560")
        await pg.wait_for_timeout(800)
        txt = " ".join((await texto_da_lista(pg, "#resultado-rapida")).split())
        if "W712/90" in txt:
            ok("busca pelo equivalente de outra marca (PSL560) acha o código do catálogo")
        else:
            erro("busca por equivalente falhou: " + txt[:140])

        # ---------- 5. peça que EXISTE no estoque mostra a quantidade ----------
        # cria a peça no estoque com o código equivalente do catálogo
        codigo_peca = "FIL-900"
        try:
            peca = api("/pecas", {"codigo": codigo_peca, "descricao": "FILTRO DE OLEO GOL",
                                  "custo_centavos": 1200, "preco_centavos": 2490,
                                  "minimo": 1, "estoque": 4, "equivalentes": ["W712/90"]})
        except Exception:
            # rodada anterior já cadastrou: reaproveita a peça existente
            peca = next(p for p in api("/pecas?limite=5000")["itens"] if p["codigo"] == codigo_peca)
        api("/pecas/" + str(peca["id"]), {"codigo": codigo_peca, "descricao": "FILTRO DE OLEO GOL",
                                         "custo_centavos": 1200, "preco_centavos": 2490,
                                         "minimo": 1, "equivalentes": ["W712/90"]}, metodo="PUT")
        await pg.fill("#busca-rapida", "W712/90")
        await pg.wait_for_timeout(900)
        txt = " ".join((await texto_da_lista(pg, "#resultado-rapida")).split())
        if "TEM no estoque" in txt or "no estoque: 4" in txt:
            ok("quando a peça existe na prateleira, a busca mostra a quantidade")
        else:
            erro("a busca não mostrou o estoque da peça: " + txt[:160])
        await pg.screenshot(path=f"{SHOT}/catalogo-4-tem-no-estoque.png")

        # ---------- 6. ficha de falta pela tela, sem mexer no estoque ----------
        estoque_antes = api("/estoque/movimentos")["itens"] if "itens" in api("/estoque/movimentos") else []
        antes = len(estoque_antes)
        await pg.fill("#busca-rapida", "PA-5010")
        await pg.wait_for_timeout(900)
        await pg.click('#resultado-rapida button:has-text("Ficha de falta")')
        await pg.wait_for_timeout(600)
        await pg.fill("#f-cliente", "Sr. Carlos")
        await pg.fill("#f-fone", "21 99999-0000")
        await pg.click('.modal-foot button:has-text("Criar ficha")')
        await pg.wait_for_timeout(1200)
        fichas = api("/fichas?status=todas")["itens"]
        nova = [f for f in fichas if "PA-5010" in (f["descricao"] or "")]
        if nova:
            ok(f"ficha de falta criada pela tela (#{nova[0]['id']}, cliente {nova[0]['cliente_nome']})")
        else:
            erro("a ficha de falta não foi criada")
        depois = len(api("/estoque/movimentos")["itens"])
        if depois == antes:
            ok("gerar ficha pelo catálogo não lançou movimento de estoque")
        else:
            erro(f"o catálogo mexeu no estoque ({antes} -> {depois} movimentos)")
        await pg.screenshot(path=f"{SHOT}/catalogo-5-ficha.png")

        # ---------- 7. reimportar não duplica ----------
        ant = len(api("/catalogo?limite=500")["itens"])
        await pg.click('button:has-text("Importar lista do fornecedor")')
        await pg.wait_for_timeout(400)
        await pg.set_input_files("#arq-catalogo", CSV_EXEMPLO)
        await pg.click('.modal-foot button:has-text("Importar")')
        await pg.wait_for_timeout(1500)
        res2 = " ".join((await texto_da_lista(pg, "#res-import-catalogo")).split())
        atual = len(api("/catalogo?limite=500")["itens"])
        if atual == ant:
            ok(f"reimportar a mesma lista atualiza sem duplicar (continuam {atual} peças)")
        else:
            erro(f"a reimportação duplicou: {ant} -> {atual}")
        await pg.click('.modal-foot button:has-text("Fechar")')
        await pg.wait_for_timeout(400)

        # ---------- 8. critérios técnicos + montar o catálogo sem planilha ----------
        # critério técnico achável ("120 dentes" não está na descrição)
        await pg.fill("#busca-rapida", "120 dentes")
        await pg.wait_for_timeout(900)
        txt = " ".join((await texto_da_lista(pg, "#resultado-rapida")).split())
        if "CF-3301" in txt:
            ok("busca por critério técnico (120 dentes) acha a peça")
        else:
            erro("busca por critério técnico não achou a correia: " + txt[:140])

        # "Montar com o que já tenho": o catálogo nasce do estoque + fichas, sem planilha
        antes = len(api("/catalogo?limite=500")["itens"])
        api("/pecas", {"codigo": "SEM-1", "descricao": "PECA SEM PLANILHA", "aplicacao": "FIAT UNO 1.0 10/16",
                       "custo_centavos": 900, "preco_centavos": 1800, "minimo": 1, "estoque": 2})
        await pg.click('button:has-text("Montar com o que já tenho")')
        await pg.wait_for_timeout(600)
        await pg.click('.modal-foot button:has-text("Montar agora")')
        await pg.wait_for_timeout(1500)
        res_semear = " ".join((await texto_da_lista(pg, "#res-semear")).split())
        depois = len(api("/catalogo?limite=500")["itens"])
        if depois > antes and "item" in res_semear:
            ok(f"“Montar com o que já tenho” cria catálogo sem planilha ({res_semear[:60]})")
        else:
            erro("a montagem automática não criou itens: " + res_semear[:120])
        achados, _ = api("/catalogo?q=" + "uno%2010")["itens"], None
        if any(x["codigo"] == "SEM-1" for x in achados):
            ok("a peça que veio do estoque é achável por aplicação no catálogo")
        else:
            erro("a peça semeada não aparece na busca por aplicação")
        await pg.screenshot(path=f"{SHOT}/catalogo-7-semear.png")

        # ---------- 9. console limpo ----------
        graves = [e for e in erros_console if "favicon" not in e.lower()]
        if not graves:
            ok("nenhum erro de JavaScript durante todo o uso da tela")
        else:
            erro("erros no console: " + " | ".join(graves[:3]))

        await pg.screenshot(path=f"{SHOT}/catalogo-6-final.png")
        await nav.close()


if __name__ == "__main__":
    asyncio.run(main())
    falhas = [m for okk, m in resultado if not okk]
    print(f"\n{len(resultado) - len(falhas)} ok / {len(falhas)} falhas")
    raise SystemExit(1 if falhas else 0)

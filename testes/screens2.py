import asyncio, os
from playwright.async_api import async_playwright
BASE = os.environ.get("UI_BASE", "http://127.0.0.1:8902")
OUT = "/home/user/manual/img"
async def main():
    async with async_playwright() as p:
        b = await p.chromium.launch()
        pg = await b.new_page(viewport={"width": 1400, "height": 880}, device_scale_factor=1.5)
        async def snap(n, e=1200):
            await pg.wait_for_timeout(e); await pg.screenshot(path=f"{OUT}/{n}.png"); print("  ", n)
        await pg.goto(f"{BASE}/#/pedidos"); await snap("19-pedidos", 1500)
        btns = await pg.query_selector_all("#lista-pedidos button")
        if btns:
            await btns[0].click(); await snap("20-pedido-detalhe", 1300)
            await pg.keyboard.press("Escape")
        await pg.goto(f"{BASE}/#/clientes"); await snap("21-clientes", 1400)
        linhas = await pg.query_selector_all("#lista-clientes-view tbody tr")
        if linhas:
            await linhas[0].click(); await snap("22-cliente-historico", 1600)
            await pg.keyboard.press("Escape")
        await pg.goto(f"{BASE}/#/fornecedores"); await snap("23-fornecedores", 1400)
        await pg.goto(f"{BASE}/#/receber"); await snap("24-receber", 1400)
        await pg.goto(f"{BASE}/#/fechamento"); await snap("25-fechamento", 1600)
        await pg.goto(f"{BASE}/#/relatorios"); await snap("26-relatorios", 1800)
        for tipo, nome in [("mais_vendidos","27-relatorio-mais-vendidos"), ("estoque_minimo","28-relatorio-estoque-minimo"),
                           ("fichas_abertas","29-relatorio-fichas"), ("estoque_valor","32-relatorio-estoque-valor"),
                           ("recebiveis","33-relatorio-recebiveis")]:
            await pg.select_option("#rel-tipo", tipo); await snap(nome, 1700)
        await pg.goto(f"{BASE}/#/backup"); await snap("30-backup", 1800)
        await b.close()
asyncio.run(main())

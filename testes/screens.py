"""Captura as telas do sistema para o manual."""
import asyncio, os, sys
from playwright.async_api import async_playwright

BASE = os.environ.get("UI_BASE", "http://127.0.0.1:8902")
OUT = "/home/user/manual/img"
os.makedirs(OUT, exist_ok=True)

async def main():
    async with async_playwright() as p:
        b = await p.chromium.launch()
        pg = await b.new_page(viewport={"width": 1400, "height": 880}, device_scale_factor=1.5)
        async def snap(nome, espera=1100):
            await pg.wait_for_timeout(espera)
            await pg.screenshot(path=f"{OUT}/{nome}.png")
            print("  ", nome)

        await pg.goto(f"{BASE}/#/inicio"); await snap("01-inicio", 1500)
        await pg.goto(f"{BASE}/#/bancada"); await snap("03-bancada")
        await pg.fill("#busca-bancada", "pastilha"); await pg.wait_for_timeout(800)
        await pg.screenshot(path=f"{OUT}/04-bancada-busca.png"); print("   04-bancada-busca")
        itens = await pg.query_selector_all("#lista-bancada .res-item")
        if itens:
            await itens[0].click(); await pg.wait_for_timeout(400)
        await pg.fill("#venda-cliente", "Maria Aparecida Souza")
        await pg.fill("#venda-placa", "KJH4E56"); await pg.fill("#venda-modelo", "Onix 1.4"); await pg.fill("#venda-ano", "2016")
        await pg.fill("#venda-recebido", "150,00")
        await snap("05-bancada-carrinho")
        await pg.click("text=Finalizar venda (F9)"); await snap("06-venda-concluida", 1400)
        cupom = await pg.query_selector("#modal-corpo .cupom")
        if cupom:
            await cupom.screenshot(path=f"{OUT}/06b-cupom.png")
        await pg.click("#modal .btn-x")

        await pg.goto(f"{BASE}/#/pecas"); await snap("07-pecas", 1400)
        await pg.fill("#busca-pecas", "filtro"); await pg.wait_for_timeout(900)
        await pg.screenshot(path=f"{OUT}/08-pecas-busca.png"); print("   08-pecas-busca")
        await pg.fill("#busca-pecas", ""); await pg.wait_for_timeout(700)
        linhas = await pg.query_selector_all("#resultado-pecas tbody tr")
        if linhas:
            await linhas[0].click(); await snap("09-peca-ficha", 1200)
            await pg.click("#modal .btn-x")

        await pg.goto(f"{BASE}/#/xml"); await snap("10-xml", 900)
        await pg.set_input_files("#arquivo-xml", "/home/user/autopecas/exemplos/exemplo-catalogo-fornecedor.xml")
        await pg.wait_for_timeout(400)
        await pg.click("#btn-analisar"); await snap("11-xml-analise", 1800)
        await pg.click("#btn-importar"); await snap("12-xml-resultado", 2200)

        await pg.goto(f"{BASE}/#/estoque"); await snap("13-estoque", 1400)
        await pg.fill("#busca-estoque", "1043"); await pg.wait_for_timeout(900)
        it = await pg.query_selector_all("#lista-estoque .res-item")
        if it:
            await it[0].click(); await snap("14-estoque-movimento", 900)
            await pg.click("#modal .btn-x")

        await pg.goto(f"{BASE}/#/fichas"); await snap("15-fichas", 1300)
        await pg.click("text=+ Nova ficha (F4)"); await snap("16-ficha-nova", 900)
        await pg.fill("#f-desc", "Sensor de rotacao do virabrequim HB20 1.0 2015")
        await pg.fill("#f-cliente", "Maria Aparecida Souza")
        await pg.fill("#f-fone", "(21) 97777-3344")
        await pg.fill("#f-veiculo", "HB20 2015")
        await pg.screenshot(path=f"{OUT}/17-ficha-preenchida.png"); print("   17-ficha-preenchida")
        await pg.click("#modal .btn-x"); await pg.wait_for_timeout(500)
        await pg.click("text=Gerar pedido por fornecedor"); await snap("18-gerar-pedido", 1200)
        await pg.click("#modal .btn-x")

        await pg.goto(f"{BASE}/#/pedidos"); await snap("19-pedidos", 1200)
        abrir = await pg.query_selector_all("#lista-pedidos tbody tr")
        if abrir:
            await abrir[0].click() if False else None
        btns = await pg.query_selector_all("#lista-pedidos button")
        if btns:
            await btns[0].click(); await snap("20-pedido-detalhe", 1200)
            await pg.click("#modal .btn-x")

        await pg.goto(f"{BASE}/#/clientes"); await snap("21-clientes", 1200)
        linhas = await pg.query_selector_all("#lista-clientes-view tbody tr")
        if linhas:
            await linhas[0].click(); await snap("22-cliente-historico", 1400)
            await pg.click("#modal .btn-x")

        await pg.goto(f"{BASE}/#/fornecedores"); await snap("23-fornecedores", 1200)
        await pg.goto(f"{BASE}/#/receber"); await snap("24-receber", 1200)
        await pg.goto(f"{BASE}/#/fechamento"); await snap("25-fechamento", 1400)
        await pg.goto(f"{BASE}/#/relatorios"); await snap("26-relatorios", 1800)
        await pg.select_option("#rel-tipo", "mais_vendidos"); await snap("27-relatorio-mais-vendidos", 1600)
        await pg.select_option("#rel-tipo", "estoque_minimo"); await snap("28-relatorio-estoque-minimo", 1600)
        await pg.select_option("#rel-tipo", "fichas_abertas"); await snap("29-relatorio-fichas", 1600)
        await pg.goto(f"{BASE}/#/backup"); await snap("30-backup", 1600)
        await pg.goto(f"{BASE}/#/inicio"); await snap("31-inicio-final", 1400)
        await b.close()

asyncio.run(main())

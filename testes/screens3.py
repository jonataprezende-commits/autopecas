import asyncio, os
from playwright.async_api import async_playwright
BASE = os.environ.get("UI_BASE", "http://127.0.0.1:8902")
OUT = "/home/user/manual/img"
async def main():
    async with async_playwright() as p:
        b = await p.chromium.launch()
        pg = await b.new_page(viewport={"width": 1240, "height": 900}, device_scale_factor=2)
        await pg.goto(f"{BASE}/#/estoque"); await pg.wait_for_timeout(1500)
        await pg.evaluate("document.getElementById('filtro-mov-data').value = new Date().toISOString().slice(0,10)")
        await pg.wait_for_timeout(1200)
        await pg.screenshot(path=f"{OUT}/13-estoque.png")
        print("13 refeito")
        await b.close()
asyncio.run(main())

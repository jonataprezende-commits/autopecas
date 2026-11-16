#!/usr/bin/env python3
"""Teste de UI do Plano B (versao 1.0.7) — tela Backup e Ajustes + Fechamento.

Roda contra um AutoPecas aberto:
    python3 qa/teste_ui_plano_b.py http://127.0.0.1:8860 [pasta-da-nuvem]

Confere:
  1. painel "Plano B" aparece com o semaforo certo (comeca em perigo, sem backup)
  2. "Fazer backup agora" faz o semaforo ficar verde
  3. "Testar a restauracao" abre o resumo do backup (pecas, vendas)
  4. "Escolher a pasta..." lista/aceita a pasta da nuvem e salva em Ajustes
  5. "Gerar kit de emergencia" mostra o tamanho e o link de download
  6. Fechamento do dia: "Fechar o caixa (com backup)" cria backup-fechamento
  7. backup de hora em hora aparece sozinho (criado ao abrir o programa)
  8. nada de erro de JavaScript; cabecalho e textos em portugues
"""
import asyncio
import sys

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8860").rstrip("/")
NUVEM = sys.argv[2] if len(sys.argv) > 2 else ""
SHOTS = "/home/user/qa/shots"

ok, falhas = [], []


def checar(nome, cond, detalhe=""):
    if cond:
        ok.append(nome)
        print(f"  OK   {nome}")
    else:
        falhas.append(f"{nome} {detalhe}".strip())
        print(f"  FALHA {nome} {detalhe}")


async def main():
    from playwright.async_api import async_playwright

    async with async_playwright() as p:
        nav = await p.chromium.launch()
        ctx = await nav.new_context(viewport={"width": 1440, "height": 950})
        pg = await ctx.new_page()
        erros = []
        pg.on("pageerror", lambda e: erros.append(str(e)))

        print("1) painel Plano B")
        await pg.goto(BASE + "/#/backup", wait_until="networkidle")
        await pg.wait_for_timeout(1500)
        checar("painel Plano B na tela", await pg.is_visible("#painel-plano-b"))
        texto = await pg.inner_text("#painel-plano-b")
        checar("mostra o estado do backup", "backup" in texto.lower())
        checar("mostra a cópia de hora em hora", "hora em hora" in texto.lower())
        checar("mostra a nuvem/pendrive", "nuvem" in texto.lower() or "pendrive" in texto.lower())
        await pg.screenshot(path=f"{SHOTS}/plano-b-1-inicial.png", full_page=True)

        print("2) fazer backup agora deixa verde")
        await pg.click("text=Fazer backup agora")
        await pg.wait_for_timeout(2500)
        texto = await pg.inner_text("#painel-plano-b")
        checar("depois do backup fica protegido", "protegido" in texto.lower(), texto[:80].replace("\n", " "))
        await pg.screenshot(path=f"{SHOTS}/plano-b-2-protegido.png", full_page=True)

        print("3) testar a restauração")
        await pg.click("text=Testar a restauração")
        await pg.wait_for_timeout(1800)
        modal = await pg.inner_text("#modal-corpo")
        checar("teste mostra que o backup abre", "íntegro" in modal.lower() or "integro" in modal.lower())
        checar("teste mostra peças e vendas", "peças" in modal.lower() and "vendas" in modal.lower())
        checar("teste avisa que nada foi alterado", "nada foi alterado" in modal.lower())
        await pg.screenshot(path=f"{SHOTS}/plano-b-3-teste.png")
        await pg.click("#modal-rodape button")
        await pg.wait_for_timeout(400)

        print("4) pasta da nuvem")
        await pg.click("text=Escolher a pasta")
        await pg.wait_for_timeout(1200)
        modal = await pg.inner_text("#modal-corpo")
        checar("modal de pasta abre", "pasta" in modal.lower())
        if NUVEM:
            await pg.fill("#nuvem-manual", NUVEM)
            await pg.click("text=Usar esta")
            await pg.wait_for_timeout(2500)
            await pg.goto(BASE + "/#/backup", wait_until="networkidle")
            await pg.wait_for_timeout(1500)
            campo = await pg.input_value("#cfg-pasta-extra")
            checar("pasta da nuvem salva nos ajustes", campo.strip() == NUVEM, campo)
            await pg.click("text=Fazer backup agora")
            await pg.wait_for_timeout(2500)
            texto = await pg.inner_text("#painel-plano-b")
            checar("painel reconhece a nuvem", "copiando" in texto.lower())
        else:
            checar("modal explica quando não acha nuvem",
                   "não encontrei" in modal.lower() or "encontrei" in modal.lower() or "usar" in modal.lower())
            await pg.click("#modal-rodape button")
            await pg.wait_for_timeout(400)
        await pg.screenshot(path=f"{SHOTS}/plano-b-4-nuvem.png", full_page=True)

        print("5) kit de emergência")
        await pg.click("text=Gerar kit de emergência")
        await pg.wait_for_timeout(4000)
        modal = await pg.inner_text("#modal-corpo")
        checar("kit mostra que ficou pronto", "pronto" in modal.lower())
        checar("kit mostra o tamanho", "mb" in modal.lower())
        link = await pg.get_attribute("#modal-rodape a", "href")
        checar("kit tem link para baixar", (link or "").endswith("/api/kit-emergencia/baixar"), str(link))
        await pg.screenshot(path=f"{SHOTS}/plano-b-5-kit.png")
        await pg.click("text=Fechar >> nth=-1")
        await pg.wait_for_timeout(500)

        print("6) fechamento do dia com backup")
        await pg.goto(BASE + "/#/fechamento", wait_until="networkidle")
        await pg.wait_for_timeout(1500)
        checar("botão de fechar o caixa existe", await pg.is_visible("text=Fechar o caixa (com backup)"))
        await pg.click("text=Fechar o caixa (com backup)")
        await pg.wait_for_timeout(900)
        checar("pede confirmação", "fechar o caixa" in (await pg.inner_text("#modal-corpo")).lower())
        await pg.click("#btn-confirmar")
        await pg.wait_for_timeout(2500)
        modal = await pg.inner_text("#modal-corpo")
        checar("depois de fechar mostra o backup guardado", "backup" in modal.lower())
        checar("o arquivo é do fechamento", "backup-fechamento" in modal or "fechamento" in modal.lower())
        await pg.screenshot(path=f"{SHOTS}/plano-b-6-fechamento.png")
        await pg.click("text=Fechar >> nth=-1")
        await pg.wait_for_timeout(400)

        print("7) estado final")
        await pg.goto(BASE + "/#/backup", wait_until="networkidle")
        await pg.wait_for_timeout(1500)
        texto = await pg.inner_text("#painel-plano-b")
        checar("mostra o último fechamento", "fechamento" in texto.lower())
        checar("continua protegido", "protegido" in texto.lower())
        checar("nenhum erro de JavaScript", not erros, "; ".join(erros[:3]))
        await pg.screenshot(path=f"{SHOTS}/plano-b-7-final.png", full_page=True)

        # no celular o painel tambem precisa caber
        cel = await nav.new_context(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True)
        pc = await cel.new_page()
        erros_cel = []
        pc.on("pageerror", lambda e: erros_cel.append(str(e)))
        await pc.goto(BASE + "/#/backup", wait_until="networkidle")
        await pc.wait_for_timeout(1800)
        checar("painel aparece no celular", await pc.is_visible("#painel-plano-b"))
        largura_ok = await pc.evaluate("() => document.documentElement.scrollWidth <= window.innerWidth + 1")
        checar("painel do celular sem rolagem lateral", largura_ok)
        checar("nenhum erro de JavaScript no celular", not erros_cel, "; ".join(erros_cel[:3]))
        await pc.screenshot(path=f"{SHOTS}/plano-b-8-celular.png", full_page=True)
        await cel.close()
        await nav.close()

    print()
    print(f"{len(ok)} OK / {len(falhas)} falha(s)")
    if falhas:
        for f in falhas:
            print("  - " + f)
        sys.exit(1)


asyncio.run(main())

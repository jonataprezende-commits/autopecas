#!/usr/bin/env python3
"""Teste de interface 1.0.9: os backups não enchem o disco.

Confere no navegador:
  1. o painel Plano B mostra quantos arquivos existem, quanto ocupam e o teto;
  2. existe o botão "Apagar os antigos agora" e ele responde;
  3. os três perfis (economia máxima / econômico / padrão) mudam os limites;
  4. depois de "Economia máxima", o painel mostra limite 1 de cada tipo;
  5. o campo do teto de espaço (MB) salva;
  6. no celular a tela não estoura a largura;
  7. nenhum erro de JavaScript.

Uso: python3 qa/teste_ui_espaco.py http://127.0.0.1:8910
"""
import sys
import time

from playwright.sync_api import sync_playwright

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8910").rstrip("/")
OK = 0
FALHAS = []
CONSOLE = []


def check(nome, cond, extra=""):
    global OK
    if cond:
        OK += 1
        print(f"  OK   {nome}")
    else:
        FALHAS.append(nome + (f" — {extra}" if extra else ""))
        print(f"  FALHOU {nome}" + (f" — {extra}" if extra else ""))


def main():
    with sync_playwright() as p:
        nav = p.chromium.launch()
        ctx = nav.new_context(viewport={"width": 1440, "height": 900})
        pg = ctx.new_page()
        pg.on("console", lambda m: CONSOLE.append(m.text) if m.type == "error" else None)
        pg.on("pageerror", lambda e: CONSOLE.append(str(e)))
        pg.goto(BASE + "/", wait_until="networkidle")
        # estado limpo: teto no padrão e limites no padrão (o teste não depende
        # de como a bancada estava antes)
        pg.evaluate("""fetch('/api/config',{method:'POST',
            headers:{'X-AutoPecas':'1','Content-Type':'application/json'},
            body:JSON.stringify({backup_limite_mb:'300',backup_manter:'30',backup_manter_horarios:'24',
              backup_manter_fechamentos:'60',backup_manter_manuais:'60',backup_manter_seguranca:'10'})})""")
        time.sleep(0.6)
        pg.reload(wait_until="networkidle")
        time.sleep(1.2)
        pg.evaluate("location.hash = '#/backup'")
        pg.wait_for_selector("#painel-plano-b", timeout=15000)
        time.sleep(0.8)

        print("\n[1] painel mostra espaço ocupado e teto")
        painel = pg.inner_text("#painel-plano-b")
        check("mostra quantidade de arquivos", "arquivo(s)" in painel, painel[:200])
        check("mostra tamanho em MB", "MB" in painel)
        check("mostra o teto configurado", "teto" in painel.lower(), painel[:300])
        check("botão de apagar antigos existe",
              pg.query_selector("button:has-text('Apagar os antigos agora')") is not None)

        print("\n[2] tabela por tipo tem coluna de limite")
        tabela = pg.inner_text("#painel-plano-b table")
        check("tabela lista os tipos", "hora em hora" in tabela, tabela[:200])
        check("tabela tem coluna Limite", "limite" in tabela.lower(), tabela[:120])

        print("\n[3] campo do teto de espaço")
        teto = pg.query_selector("#cfg-limite-mb")
        check("campo de teto existe", teto is not None)
        check("teto vem preenchido (padrão 300)", teto and teto.input_value() == "300",
              teto.input_value() if teto else "")
        # muda para 120 e salva
        pg.fill("#cfg-limite-mb", "120")
        pg.click("button:has-text('Salvar ajustes')")
        time.sleep(1.2)
        cfg = pg.evaluate("fetch('/api/config',{headers:{'X-AutoPecas':'1'}}).then(r=>r.json())")
        check("teto salvo no sistema", str(cfg.get("backup_limite_mb")) == "120", cfg.get("backup_limite_mb"))

        print("\n[4] perfil 'Economia máxima' deixa 1 cópia de cada tipo")
        pg.evaluate("location.hash = '#/backup'")
        pg.wait_for_selector("button:has-text('Economia máxima (1 de cada)')", timeout=10000)
        pg.click("button:has-text('Economia máxima (1 de cada)')")
        time.sleep(2.0)
        cfg = pg.evaluate("fetch('/api/config',{headers:{'X-AutoPecas':'1'}}).then(r=>r.json())")
        check("limite de hora em hora = 1", cfg.get("backup_manter_horarios") == "1", cfg.get("backup_manter_horarios"))
        check("limite diário = 1", cfg.get("backup_manter") == "1", cfg.get("backup_manter"))
        check("limite de fechamento = 1", cfg.get("backup_manter_fechamentos") == "1")
        check("limite manual = 1", cfg.get("backup_manter_manuais") == "1")
        check("limite de segurança = 1", cfg.get("backup_manter_seguranca") == "1")
        pg.wait_for_selector("#painel-plano-b", timeout=10000)
        time.sleep(0.8)
        tabela = pg.inner_text("#painel-plano-b table")
        check("painel passa a mostrar 'de 1'", "de 1" in tabela, tabela[:300])

        print("\n[5] botão apagar antigos responde")
        pg.click("button:has-text('Apagar os antigos agora')")
        pg.wait_for_selector("#btn-confirmar", timeout=8000)
        pg.click("#btn-confirmar")
        time.sleep(2.0)
        corpo = pg.inner_text("body")
        check("avisa o resultado da limpeza",
              ("Nada para apagar" in corpo) or ("apagado" in corpo) or ("liberado" in corpo), corpo[-300:])

        print("\n[6] voltar ao padrão deixa de novo 24")
        pg.evaluate("location.hash = '#/backup'")
        pg.wait_for_selector("button:has-text('Padrão (24 horários + 30 dias)')", timeout=10000)
        pg.click("button:has-text('Padrão (24 horários + 30 dias)')")
        time.sleep(2.0)
        cfg = pg.evaluate("fetch('/api/config',{headers:{'X-AutoPecas':'1'}}).then(r=>r.json())")
        check("perfil padrão volta a 24 horários", cfg.get("backup_manter_horarios") == "24",
              cfg.get("backup_manter_horarios"))
        pg.screenshot(path="/home/user/qa/shots/espaco-1-painel.png", full_page=True)

        print("\n[7] celular")
        cel = nav.new_context(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True)
        pm = cel.new_page()
        pm.on("pageerror", lambda e: CONSOLE.append(str(e)))
        pm.goto(BASE + "/", wait_until="networkidle")
        time.sleep(1)
        pm.evaluate("location.hash = '#/backup'")
        time.sleep(1.5)
        larg = pm.evaluate("Math.max(document.documentElement.scrollWidth, document.body.scrollWidth)")
        check(f"tela de backup cabe no celular ({larg}px)", larg <= 392, f"scrollWidth={larg}")
        pm.screenshot(path="/home/user/qa/shots/espaco-2-celular.png", full_page=True)

        print("\n[8] console")
        reais = [c for c in CONSOLE if "favicon" not in c.lower()]
        check("nenhum erro de JavaScript", not reais, "; ".join(reais[:3]))

        nav.close()

    print(f"\n{OK} OK / {len(FALHAS)} falha(s)")
    for f in FALHAS:
        print("   -", f)
    return 1 if FALHAS else 0


if __name__ == "__main__":
    sys.exit(main())

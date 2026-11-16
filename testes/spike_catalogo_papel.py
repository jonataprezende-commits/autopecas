"""Spike: dá para transformar catálogo de papel em dados de verdade?

Desenha uma pagina de catalogo de autopecas como as reais (duas colunas,
letra miuda, codigo + descricao + aplicacao + medida), fotografa em 2
qualidades e mede:

  1. quantas LINHAS de produto o OCR encontra
  2. quantos CODIGOS saem exatos
  3. quantas APLICACOES (veiculo/ano) saem aproveitaveis

Compara com o padrao alternativo: catalogo em PDF/planilha (texto digital),
que nao passa por OCR nenhum.

Uso: python3 qa/spike_catalogo_papel.py
"""
import io
import json
import re
import subprocess
import sys

import cv2
import numpy as np
from PIL import Image, ImageDraw, ImageFont

FONTE = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
FONTE_B = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"

# catalogo sintetico: 20 linhas, 2 colunas, como os de fornecedor de autopecas
PRODUTOS = [
    ("W712/90", "FILTRO DE OLEO", "VW GOL 1.0/1.6 08/16", "ROSCA 3/4-16"),
    ("W712/91", "FILTRO DE OLEO", "FIAT PALIO 1.0 96/11", "ROSCA 3/4-16"),
    ("W712/95", "FILTRO DE OLEO", "GM ONIX 1.0 12/19", "ROSCA M20X1.5"),
    ("W811/80", "FILTRO DE OLEO", "FORD KA 1.0 08/14", "ROSCA 3/4-16"),
    ("AF-1020", "FILTRO DE AR", "VW GOL G5/G6 1.0 08/16", "265X165X38"),
    ("AF-1021", "FILTRO DE AR", "FIAT UNO 1.0 10/16", "250X150X35"),
    ("AF-1022", "FILTRO DE AR", "GM COBALT 1.4 11/19", "280X170X40"),
    ("C-1101", "FILTRO DE COMBUSTIVEL", "VW SAVEIRO 1.6 09/16", "PAPEL",),
    ("C-1102", "FILTRO DE COMBUSTIVEL", "FIAT STRADA 1.4 10/20", "PAPEL"),
    ("PA-5010", "PASTILHA DE FREIO DIANT", "VW GOL 1.0 13/20", "JOGO 4 PECAS"),
    ("PA-5011", "PASTILHA DE FREIO DIANT", "FIAT PALIO 1.0 11/17", "JOGO 4 PECAS"),
    ("PA-5012", "PASTILHA DE FREIO DIANT", "GM ONIX 1.0 13/19", "JOGO 4 PECAS"),
    ("PA-5013", "PASTILHA DE FREIO TRAS", "VW VOYAGE 1.6 09/16", "JOGO 4 PECAS"),
    ("CD-3001", "CORREIA DENTADA", "VW GOL 1.0 8V 96/08", "6PK 1250"),
    ("CD-3002", "CORREIA DENTADA", "FIAT UNO 1.0 8V 04/13", "5PK 1180"),
    ("CD-3003", "CORREIA DENTADA", "GM CELTA 1.0 00/12", "6PK 1130"),
    ("VE-7001", "VELA DE IGNICAO", "VW GOL/FIAT UNO 8V", "BP6ES"),
    ("VE-7002", "VELA DE IGNICAO", "GM ONIX 1.0 2A16", "BKR6EIX"),
    ("RB-9001", "ROLAMENTO RODA DIANT", "VW GOL G5 08/16", "SKF BA2-4582"),
    ("RB-9002", "ROLAMENTO RODA TRAS", "FIAT PALIO 96/11", "SKF BA2-1204"),
]


def desenhar_catalogo():
    L, A = 1600, 2100
    img = Image.new("RGB", (L, A), "white")
    d = ImageDraw.Draw(img)
    f_tit = ImageFont.truetype(FONTE_B, 40)
    f_cab = ImageFont.truetype(FONTE_B, 20)
    f = ImageFont.truetype(FONTE, 19)

    d.text((60, 50), "CATALOGO DE FILTROS E FREIOS 2026", font=f_tit, fill="black")
    d.text((60, 100), "Distribuidora XYZ  -  pagina 47  -  filtros de oleo / ar / pastilhas", font=f, fill="black")

    colunas = [(60, 10), (L // 2 + 10, 10)]
    for col_inicio, idx in colunas:
        y = 170
        d.text((col_inicio, y), f"{'CODIGO':<12}{'DESCRICAO':<24}{'APLICACAO':<26}MEDIDA", font=f_cab, fill="black")
        d.line((col_inicio, y + 26, col_inicio + 740, y + 26), fill="black", width=2)
        y += 40
        for cod, desc, aplic, medida in PRODUTOS[idx:idx + 10]:
            d.text((col_inicio, y), f"{cod:<12}{desc:<24}{aplic:<26}{medida}", font=f, fill="black")
            y += 34
    d.text((60, A - 70), "Precos e disponibilidade: consulte a tabela do fornecedor", font=f, fill="black")
    return img


def fotografar(img, qualidade):
    presets = {
        "boa": dict(escala=1.15, angulo=1.0, sombra=0.18, blur=0.4, jpeg=90, ruido=2, persp=10),
        "celular": dict(escala=0.85, angulo=3.0, sombra=0.40, blur=0.9, jpeg=70, ruido=7, persp=25),
    }
    p = presets[qualidade]
    moldura = 300
    nl, na = img.size
    mesa = Image.new("RGB", (nl + 2 * moldura, na + 2 * moldura), (112, 108, 102))
    mesa.paste(img, (moldura, moldura))
    arr = np.array(mesa)
    H, W = arr.shape[:2]
    e = p["persp"]
    origem = np.float32([[0, 0], [W, 0], [W, H], [0, H]])
    destino = np.float32([[e * 2, e * 3], [W - e, e], [W - e * 1.5, H - e], [e / 2, H - e * 2]])
    m = cv2.getPerspectiveTransform(origem, destino)
    arr = cv2.warpPerspective(arr, m, (W, H), borderValue=(112, 108, 102))
    m2 = cv2.getRotationMatrix2D((W / 2, H / 2), p["angulo"], 1.0)
    arr = cv2.warpAffine(arr, m2, (W, H), borderValue=(112, 108, 102))
    yy, xx = np.mgrid[0:H, 0:W]
    luz = 1.0 - p["sombra"] * ((xx / W) * 0.5 + (yy / H) * 0.5)
    arr = np.clip(arr.astype(np.float32) * luz[..., None], 0, 255)
    arr = cv2.GaussianBlur(arr.astype(np.uint8), (3, 3), p["blur"])
    rng = np.random.default_rng(21)
    arr = np.clip(arr.astype(np.float32) + rng.normal(0, p["ruido"], arr.shape), 0, 255).astype(np.uint8)
    pil = Image.fromarray(arr)
    pil = pil.resize((int(W * p["escala"]), int(H * p["escala"])), Image.LANCZOS)
    buf = io.BytesIO()
    pil.save(buf, "JPEG", quality=p["jpeg"])
    return Image.open(io.BytesIO(buf.getvalue())).convert("RGB")


def ocr_linhas(pil, psm="6"):
    pil.save("/tmp/cat.png")
    tsv = subprocess.run(["tesseract", "/tmp/cat.png", "stdout", "-l", "por", "--psm", psm, "tsv"],
                         capture_output=True, text=True).stdout
    linhas = {}
    for i, l in enumerate(tsv.splitlines()[1:]):
        c = l.split("\t")
        if len(c) < 12 or not c[11].strip():
            continue
        chave = (c[2], c[3], c[4])  # bloco, paragrafo, linha do proprio OCR
        linhas.setdefault(chave, []).append({
            "txt": c[11].strip(), "x": int(c[6]), "conf": float(c[10]),
        })
    saida = []
    for chave in sorted(linhas, key=lambda k: (int(k[0]), int(k[1]), int(k[2]))):
        ps = sorted(linhas[chave], key=lambda p: p["x"])
        saida.append(" ".join(p["txt"] for p in ps))
    return saida


def avaliar(linhas, rotulo):
    texto = "\n".join(linhas)
    codigos_gab = [p[0] for p in PRODUTOS]
    aplic_gab = [p[2] for p in PRODUTOS]
    # codigos exatos (compara sem espacos/acentos)
    def norm(s):
        return re.sub(r"[^A-Z0-9]", "", s.upper())
    texto_norm = norm(texto)
    cod_ok = sum(1 for c in codigos_gab if norm(c) in texto_norm)
    # aplicacoes: exige o modelo E o ano
    apl_ok = 0
    for aplic in aplic_gab:
        modelo = aplic.split()[1] if len(aplic.split()) > 1 else ""
        anos = re.findall(r"\d{2}/\d{2}", aplic)
        achou = any(modelo.upper() in l.upper() and all(a in l for a in anos) for l in linhas)
        apl_ok += achou
    print(f"\n--- {rotulo}")
    print(f"    linhas de texto lidas: {len(linhas)}")
    print(f"    codigos exatos: {cod_ok}/{len(codigos_gab)}")
    print(f"    aplicacoes com modelo+ano: {apl_ok}/{len(aplic_gab)}")
    return {"rotulo": rotulo, "linhas": len(linhas), "codigos_ok": cod_ok,
            "codigos_total": len(codigos_gab), "aplic_ok": apl_ok, "aplic_total": len(aplic_gab)}


def main():
    catalogo = desenhar_catalogo()
    catalogo.save("/tmp/catalogo-limpo.png")
    res = []
    for qualidade in ("boa", "celular"):
        foto = fotografar(catalogo, qualidade)
        foto.save(f"/home/user/qa/spike-catalogo-{qualidade}.jpg", quality=80)
        linhas = ocr_linhas(foto)
        if qualidade == "celular":
            print("\n(amostra do que o OCR leu na foto de celular)")
            for l in linhas[:6]:
                print("   ", l[:95])
        res.append(avaliar(linhas, f"foto {qualidade}"))
    # referencia: texto digital (o que se ganha pedindo PDF/planilha)
    print("\n--- referencia: texto digital (PDF/planilha do fornecedor)")
    print("    20/20 codigos, 20/20 aplicacoes, sem OCR, sem erro de leitura")
    with open("/home/user/qa/spike-catalogo-resultado.json", "w") as f:
        json.dump(res, f, indent=2, ensure_ascii=False)


if __name__ == "__main__":
    sys.exit(main())

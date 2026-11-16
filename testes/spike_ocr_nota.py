"""Prova de conceito: quanto o OCR (Tesseract, offline) acerta numa foto de nota
de compra em papel?

1. desenha uma nota sintetica (tabela de itens: codigo, descricao, qtd, unitario, total)
2. degrada como foto de celular: perspectiva, rotacao, sombra, desfoque, JPEG
3. roda OCR e tenta extrair codigo/qtd/valor por coluna
4. compara com o gabarito e imprime acertos/erros item por item

Uso: python3 qa/spike_ocr_nota.py
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

ITENS = [
    ("SKU-1001", "FILTRO DE OLEO MOTOR 1.0", "2", "18,50", "37,00"),
    ("SKU-1002", "PASTILHA DE FREIO DIANTEIRA", "1", "89,90", "89,90"),
    ("SKU-1003", "CORREIA DENTADA 6PK", "3", "42,30", "126,90"),
    ("SKU-1004", "VELA DE IGNICAO IRIDIUM", "4", "39,75", "159,00"),
    ("SKU-1005", "OLEO 5W30 SINTETICO 1L", "6", "34,90", "209,40"),
    ("SKU-1006", "FILTRO DE AR ESPORTIVO", "2", "65,00", "130,00"),
    ("SKU-1007", "ADITIVO RADIADOR 500ML", "5", "12,40", "62,00"),
    ("SKU-1008", "JOGO DE CABOS DE VELA", "1", "118,00", "118,00"),
]
TOTAL = "932,20"


def desenhar_nota():
    """Desenha a nota limpa em alta resolucao."""
    L, A = 1400, 1900
    img = Image.new("RGB", (L, A), "white")
    d = ImageDraw.Draw(img)
    f_tit = ImageFont.truetype(FONTE_B, 40)
    f = ImageFont.truetype(FONTE, 26)
    f_p = ImageFont.truetype(FONTE, 22)

    d.text((60, 50), "AUTO PECAS CENTRAL LTDA", font=f_tit, fill="black")
    d.text((60, 105), "CNPJ 12.345.678/0001-99  -  DANFE / NOTA DE ENTRADA", font=f_p, fill="black")
    d.text((60, 140), "Fornecedor: DISTRIBUIDORA DE AUTOPECAS XYZ LTDA", font=f_p, fill="black")
    d.text((60, 172), "NF-e 000.123.456  Serie 001  Data 12/11/2026", font=f_p, fill="black")

    y = 240
    d.text((60, y), "CODIGO        DESCRICAO                          QTD   UNIT      TOTAL", font=f, fill="black")
    d.line((60, y + 34, L - 60, y + 34), fill="black", width=2)
    y += 60
    for cod, desc, qtd, unit, tot in ITENS:
        d.text((60, y), f"{cod:<12}{desc:<34}{qtd:>3}   {unit:>7}   {tot:>8}", font=f, fill="black")
        y += 52
    d.line((60, y + 6, L - 60, y + 6), fill="black", width=2)
    d.text((L - 420, y + 30), f"TOTAL DA NOTA  R$ {TOTAL}", font=f_tit, fill="black")
    d.text((60, y + 120), "Assinatura do recebedor: ______________________", font=f_p, fill="black")
    return img


def fotografar(img):
    """Simula foto de celular: perspectiva, rotacao, sombra, desfoque, JPEG."""
    w, h = img.size
    origem = np.float32([[0, 0], [w, 0], [w, h], [0, h]])
    # canto da folha torto, como quem fotografa de cima e um pouco de lado
    destino = np.float32([[70, 110], [w - 30, 30], [w - 90, h - 40], [30, h - 150]])
    m = cv2.getPerspectiveTransform(origem, destino)
    out = cv2.warpPerspective(np.array(img), m, (w, h), borderValue=(60, 60, 60))

    angulo = 2.5
    mm = cv2.getRotationMatrix2D((w / 2, h / 2), angulo, 1.0)
    out = cv2.warpAffine(out, mm, (w, h), borderValue=(60, 60, 60))

    # sombra diagonal (luz vindo do canto superior esquerdo)
    yy, xx = np.mgrid[0:h, 0:w]
    sombra = 1.0 - 0.42 * ((xx / w) * 0.55 + (yy / h) * 0.45)
    out = np.clip(out.astype(np.float32) * sombra[..., None], 0, 255).astype(np.uint8)

    out = cv2.GaussianBlur(out, (3, 3), 0.8)
    ruido = np.random.normal(0, 6, out.shape)
    out = np.clip(out.astype(np.float32) + ruido, 0, 255).astype(np.uint8)

    pil = Image.fromarray(out)
    # foto de celular reduzida para 8 MP e salva em JPEG (perde detalhe)
    pil = pil.resize((1080, 1466), Image.LANCZOS)
    buf = io.BytesIO()
    pil.save(buf, "JPEG", quality=72)
    return Image.open(io.BytesIO(buf.getvalue())).convert("RGB")


def ocr(pil):
    pil.save("/tmp/nota.png")
    tsv = subprocess.run(
        ["tesseract", "/tmp/nota.png", "stdout", "-l", "por", "--psm", "6", "tsv"],
        capture_output=True, text=True, check=True,
    ).stdout
    palavras = []
    for linha in tsv.splitlines()[1:]:
        c = linha.split("\t")
        if len(c) < 12 or not c[11].strip():
            continue
        palavras.append({
            "txt": c[11].strip(),
            "conf": float(c[10]),
            "x": int(c[6]), "y": int(c[7]), "w": int(c[8]), "h": int(c[9]),
        })
    return palavras


def agrupar_linhas(palavras, tolerancia=14):
    """Agrupa palavras em linhas de texto pela coordenada Y."""
    linhas = []
    for p in sorted(palavras, key=lambda p: (p["y"], p["x"])):
        for l in linhas:
            if abs(l["y"] - p["y"]) <= tolerancia:
                l["p"].append(p)
                break
        else:
            linhas.append({"y": p["y"], "p": [p]})
    for l in linhas:
        l["p"].sort(key=lambda p: p["x"])
        l["txt"] = " ".join(p["txt"] for p in l["p"])
    return sorted(linhas, key=lambda l: l["y"])


def main():
    limpa = desenhar_nota()
    limpa.save("/tmp/nota_limpa.png")
    foto = fotografar(limpa)
    foto.save("/home/user/qa/spike-nota-foto.jpg", quality=72)
    palavras = ocr(foto)
    linhas = agrupar_linhas(palavras)

    print("== texto que o OCR leu (foto) ==")
    for l in linhas:
        print(f"  y={l['y']:>4}  {l['txt']}")

    # --- extracao dos itens: procura SKU, depois qtd e valor na mesma linha ---
    achados = []
    for l in linhas:
        m = re.search(r"(SKU[- ]?\d{3,5})", l["txt"].replace(" ", ""))
        if not m:
            continue
        codigo = m.group(1).replace(" ", "")
        numeros = re.findall(r"\d+(?:[.,]\d{2,3})?", l["txt"].split(codigo.replace("SKU", "SKU"))[-1])
        achados.append((codigo, l["txt"], numeros))

    print("\n== itens que o parser reconheceu ==")
    acertos = 0
    for cod, desc, qtd, unit, tot in ITENS:
        achado = next((a for a in achados if a[0].replace("-", "") == cod.replace("-", "")), None)
        if not achado:
            print(f"  [FALTOU] {cod} {desc}")
            continue
        nums = achado[2]
        ok_qtd = qtd in nums
        ok_tot = tot in nums
        acertos += ok_qtd + ok_tot
        marca = "ok " if (ok_qtd and ok_tot) else "PARCIAL"
        print(f"  [{marca}] {cod} esperado qtd={qtd} total={tot} | lido: {achado[1][:60]} | nums={nums}")

    print(f"\nitens lidos: {len(achados)}/{len(ITENS)} | campos qtd+total certos: {acertos}/{2*len(ITENS)}")
    conf = [p["conf"] for p in palavras]
    print(f"confianca media do OCR: {sum(conf)/len(conf):.1f}%  (min {min(conf):.0f}%)")
    total_lido = "932,20" in " ".join(l["txt"] for l in linhas)
    print("total da nota lido certo:", total_lido)
    with open("/home/user/qa/spike-ocr-resultado.json", "w") as f:
        json.dump({"itens_lidos": len(achados), "campos_certos": acertos,
                   "conf_media": round(sum(conf)/len(conf), 1), "total_lido": total_lido}, f, indent=2)


if __name__ == "__main__":
    sys.exit(main())

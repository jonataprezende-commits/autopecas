"""Spike 2: mesma foto, mas com o pipeline serio de pre-processamento.

foto -> acha a folha (contorno) -> endireita em perspectiva -> cinza -> contraste
local -> binarizacao -> OCR (psm 6) -> reconstrucao das colunas por coordenada X.

Compara: (a) OCR cru, (b) OCR com pre-processamento.
Uso: python3 qa/spike_ocr_nota2.py
"""
import io
import json
import re
import subprocess
import sys

import cv2
import numpy as np
from PIL import Image, ImageDraw, ImageFont

sys.path.insert(0, "/home/user/qa")
from spike_ocr_nota import ITENS, TOTAL, desenhar_nota  # noqa: E402

FONTE = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"


def fotografar_mesa(img, semente=7):
    """Nota sobre a mesa: fundo cinza, folha torta, sombra, desfoque, JPEG."""
    rng = np.random.default_rng(semente)
    w, h = img.size
    margem = 220
    fundo = Image.new("RGB", (w + 2 * margem, h + 2 * margem), (105, 100, 95))
    folha = np.array(img)
    # sombra suave sobre a mesa atras da folha
    fundo_np = np.array(fundo).astype(np.float32)
    cv2.rectangle(fundo_np, (margem + 40, margem + 60), (margem + w - 10, margem + h - 10),
                  (70, 68, 66), -1)
    fundo = Image.fromarray(np.clip(fundo_np, 0, 255).astype(np.uint8))
    fundo.paste(Image.fromarray(folha), (margem, margem))

    arr = np.array(fundo)
    H, W = arr.shape[:2]
    origem = np.float32([[0, 0], [W, 0], [W, H], [0, H]])
    destino = np.float32([[60, 90], [W - 40, 20], [W - 70, H - 30], [25, H - 120]])
    m = cv2.getPerspectiveTransform(origem, destino)
    arr = cv2.warpPerspective(arr, m, (W, H), borderValue=(105, 100, 95))
    m2 = cv2.getRotationMatrix2D((W / 2, H / 2), 3.0, 1.0)
    arr = cv2.warpAffine(arr, m2, (W, H), borderValue=(105, 100, 95))

    yy, xx = np.mgrid[0:H, 0:W]
    sombra = 1.0 - 0.35 * ((xx / W) * 0.5 + (yy / H) * 0.5)
    arr = np.clip(arr.astype(np.float32) * sombra[..., None] + rng.normal(0, 5, arr.shape), 0, 255)
    arr = cv2.GaussianBlur(arr.astype(np.uint8), (3, 3), 0.7)

    pil = Image.fromarray(arr).resize((1240, 1720), Image.LANCZOS)
    buf = io.BytesIO()
    pil.save(buf, "JPEG", quality=75)
    return Image.open(io.BytesIO(buf.getvalue())).convert("RGB")


def endireitar(pil):
    """Acha o maior quadrilatero (a folha) e devolve a imagem corrigida."""
    arr = np.array(pil)
    cinza = cv2.cvtColor(arr, cv2.COLOR_RGB2GRAY)
    borrada = cv2.GaussianBlur(cinza, (7, 7), 0)
    bordas = cv2.Canny(borrada, 30, 90)
    bordas = cv2.dilate(bordas, np.ones((3, 3), np.uint8), iterations=2)
    contornos, _ = cv2.findContours(bordas, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE)
    contornos = sorted(contornos, key=cv2.contourArea, reverse=True)[:5]
    for c in contornos:
        peri = cv2.arcLength(c, True)
        aprox = cv2.approxPolyDP(c, 0.02 * peri, True)
        if len(aprox) == 4 and cv2.contourArea(aprox) > 0.2 * arr.shape[0] * arr.shape[1]:
            pts = aprox.reshape(4, 2).astype(np.float32)
            soma = pts.sum(axis=1)
            dif = np.diff(pts, axis=1).ravel()
            ordenados = np.array([pts[np.argmin(soma)], pts[np.argmin(dif)],
                                  pts[np.argmax(soma)], pts[np.argmax(dif)]], dtype=np.float32)
            larg = int(max(np.linalg.norm(ordenados[0] - ordenados[1]),
                           np.linalg.norm(ordenados[3] - ordenados[2])))
            alt = int(max(np.linalg.norm(ordenados[0] - ordenados[3]),
                          np.linalg.norm(ordenados[1] - ordenados[2])))
            destino = np.float32([[0, 0], [larg - 1, 0], [larg - 1, alt - 1], [0, alt - 1]])
            m = cv2.getPerspectiveTransform(ordenados, destino)
            return cv2.warpPerspective(arr, m, (larg, alt)), True
    return arr, False


def preparar(arr):
    """Cinza + CLAHE + binarizacao de Otsu + margem branca."""
    cinza = cv2.cvtColor(arr, cv2.COLOR_RGB2GRAY)
    clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8, 8))
    cinza = clahe.apply(cinza)
    _, bin_ = cv2.threshold(cinza, 0, 255, cv2.THRESH_BINARY + cv2.THRESH_OTSU)
    bin_ = cv2.copyMakeBorder(bin_, 30, 30, 30, 30, cv2.BORDER_CONSTANT, value=255)
    return bin_


def ocr(img_cv, psm="6"):
    cv2.imwrite("/tmp/nota_prep.png", img_cv)
    tsv = subprocess.run(["tesseract", "/tmp/nota_prep.png", "stdout", "-l", "por",
                          "--psm", psm, "tsv"], capture_output=True, text=True, check=True).stdout
    palavras = []
    for linha in tsv.splitlines()[1:]:
        c = linha.split("\t")
        if len(c) < 12 or not c[11].strip():
            continue
        palavras.append({"txt": c[11].strip(), "conf": float(c[10]),
                         "x": int(c[6]), "y": int(c[7]), "w": int(c[8]), "h": int(c[9])})
    return palavras


def reconstruir(palavras):
    """Reconstroi as linhas pelo Y e, dentro da linha, ordena pelo X."""
    linhas = []
    for p in sorted(palavras, key=lambda p: (p["y"], p["x"])):
        # tolerancia proporcional a altura da palavra (nota fotografada varia)
        tol = max(12, int(p["h"] * 0.6))
        for l in linhas:
            if abs(l["y"] - p["y"]) <= tol:
                l["p"].append(p)
                l["y"] = (l["y"] + p["y"]) // 2
                break
        else:
            linhas.append({"y": p["y"], "p": [p]})
    for l in linhas:
        l["p"].sort(key=lambda p: p["x"])
        l["txt"] = " ".join(p["txt"] for p in l["p"])
    return sorted(linhas, key=lambda l: l["y"])


def conferir(linhas, rotulo):
    print(f"\n===== {rotulo} =====")
    for l in linhas:
        print("   ", l["txt"])
    # parser: por linha, junta texto ignorando espacos e tenta achar SKU + numeros
    itens = []
    for l in linhas:
        junto = l["txt"].replace(" ", "").upper()
        m = re.search(r"SKU-?(\d{3,5})", junto)
        if not m:
            continue
        resto = l["txt"]
        # numeros do tipo 1.234,56 / 12,30 / 2 - na ordem em que aparecem
        nums = re.findall(r"\d{1,3}(?:\.\d{3})*(?:,\d{2,3})?|\d+", resto)
        nums = [n for n in nums if n not in m.group(0)]
        itens.append({"sku": "SKU-" + m.group(1), "nums": nums, "linha": l["txt"]})

    certos = 0
    for cod, desc, qtd, unit, tot in ITENS:
        achado = next((i for i in itens if i["sku"].replace("-", "") == cod.replace("-", "")), None)
        if not achado:
            print(f"  [FALTOU] {cod}")
            continue
        ok_qtd = qtd in achado["nums"]
        ok_unit = unit in achado["nums"]
        ok_tot = tot in achado["nums"]
        certos += ok_qtd + ok_unit + ok_tot
        marca = "ok " if (ok_qtd and ok_unit and ok_tot) else "PARCIAL"
        print(f"  [{marca}] {cod} qtd={qtd} unit={unit} tot={tot} | lido: {achado['nums']}")
    lido_total = any(TOTAL in l["txt"] for l in linhas)
    conf = [p["conf"] for p in palavras_global]
    media = sum(conf) / len(conf)
    print(f"  itens reconhecidos: {len(itens)}/{len(ITENS)} | campos certos: {certos}/{3*len(ITENS)}"
          f" | total da nota lido: {lido_total} | confianca media: {media:.1f}%")
    return {"rotulo": rotulo, "itens": len(itens), "campos_certos": certos,
            "campos_total": 3 * len(ITENS), "total_lido": bool(lido_total),
            "conf_media": round(media, 1)}


resultado = []

if __name__ == "__main__":
    limpa = desenhar_nota()
    foto = fotografar_mesa(limpa)
    foto.save("/home/user/qa/spike-nota-mesa.jpg", quality=75)

    # (a) OCR cru na foto
    palavras_global = ocr(np.array(cv2.cvtColor(np.array(foto), cv2.COLOR_RGB2GRAY)))
    resultado.append(conferir(reconstruir(palavras_global), "A) OCR cru na foto (sem tratamento)"))

    # (b) OCR com endireitamento + binarizacao
    corrigida, achou = endireitar(foto)
    cv2.imwrite("/home/user/qa/spike-nota-endireitada.png", cv2.cvtColor(corrigida, cv2.COLOR_RGB2BGR))
    preparada = preparar(corrigida)
    palavras_global = ocr(preparada, psm="6")
    resultado.append(conferir(reconstruir(palavras_global),
                              f"B) Endireitada (folha detectada={achou}) + binarizada"))

    with open("/home/user/qa/spike-ocr-resultado.json", "w") as f:
        json.dump(resultado, f, indent=2, ensure_ascii=False)
    print("\nresumo:", json.dumps(resultado, ensure_ascii=False))

"""Spike 3: quanto a QUALIDADE da foto muda o resultado do OCR.

Tres cenarios da mesma nota:
  boa   = celular apoiado, luz do dia, 2000px, so uma leve rotacao
  media = na mao, sombra de um lado, 1240px, desfoque leve
  ruim  = torta, sombra forte, 1080px, JPEG apertado

Objetivo: saber se "fotografe bem" resolve, e quanto sobra de erro nos numeros.
Uso: python3 qa/spike_ocr_nota3.py
"""
import io
import json
import re
import subprocess
import sys

import cv2
import numpy as np
from PIL import Image

sys.path.insert(0, "/home/user/qa")
from spike_ocr_nota import ITENS, TOTAL, desenhar_nota  # noqa: E402
from spike_ocr_nota2 import endireitar, preparar, reconstruir  # noqa: E402

PRESETS = {
    "boa":   dict(escala=1.45, angulo=1.0, sombra=0.12, blur=0.4, jpeg=92, ruido=2, persp=0.02),
    "media": dict(escala=0.95, angulo=3.0, sombra=0.35, blur=0.7, jpeg=75, ruido=5, persp=0.05),
    "ruim":  dict(escala=0.80, angulo=5.0, sombra=0.50, blur=1.1, jpeg=60, ruido=9, persp=0.08),
}


def fotografar(img, p):
    arr = np.array(img)
    H, W = arr.shape[:2]
    e = p["persp"]
    origem = np.float32([[0, 0], [W, 0], [W, H], [0, H]])
    destino = np.float32([[W * e, H * (e + 0.01)], [W * (1 - e / 2), H * e / 3],
                          [W * (1 - e), H * (1 - e)], [W * e / 3, H * (1 - e / 2)]])
    m = cv2.getPerspectiveTransform(origem, destino)
    arr = cv2.warpPerspective(arr, m, (W, H), borderValue=(110, 105, 100))
    m2 = cv2.getRotationMatrix2D((W / 2, H / 2), p["angulo"], 1.0)
    arr = cv2.warpAffine(arr, m2, (W, H), borderValue=(110, 105, 100))

    yy, xx = np.mgrid[0:H, 0:W]
    s = p["sombra"]
    luz = 1.0 - s * ((xx / W) * 0.55 + (yy / H) * 0.45)
    arr = np.clip(arr.astype(np.float32) * luz[..., None], 0, 255)
    arr = cv2.GaussianBlur(arr.astype(np.uint8), (3, 3), p["blur"])
    rng = np.random.default_rng(3)
    arr = np.clip(arr.astype(np.float32) + rng.normal(0, p["ruido"], arr.shape), 0, 255).astype(np.uint8)

    pil = Image.fromarray(arr)
    pil = pil.resize((int(W * p["escala"]), int(H * p["escala"])), Image.LANCZOS)
    buf = io.BytesIO()
    pil.save(buf, "JPEG", quality=p["jpeg"])
    return Image.open(io.BytesIO(buf.getvalue())).convert("RGB")


def ocr(img_cv):
    cv2.imwrite("/tmp/n3.png", img_cv)
    tsv = subprocess.run(["tesseract", "/tmp/n3.png", "stdout", "-l", "por", "--psm", "6", "tsv"],
                         capture_output=True, text=True, check=True).stdout
    palavras = []
    for linha in tsv.splitlines()[1:]:
        c = linha.split("\t")
        if len(c) < 12 or not c[11].strip():
            continue
        palavras.append({"txt": c[11].strip(), "conf": float(c[10]),
                         "x": int(c[6]), "y": int(c[7]), "w": int(c[8]), "h": int(c[9])})
    return palavras


def avaliar(pil, nome):
    arr, achou = endireitar(pil)
    palavras = ocr(preparar(arr))
    if not palavras:
        return {"foto": nome, "itens": 0, "qtd_ok": 0, "valor_ok": 0, "total_ok": 0,
                "soma_confere": False, "obs": "OCR nao leu nada"}
    linhas = reconstruir(palavras)
    texto = "\n".join(l["txt"] for l in linhas)

    # reconstrucao por coluna: usa o X das palavras para separar descricao qtd unit total
    qtd_ok = valor_ok = 0
    detalhes = []
    for cod, desc, qtd, unit, tot in ITENS:
        linha = next((l for l in linhas
                      if re.search(r"S\s*[Kk]\s*U\s*-?\s*" + cod.split("-")[1], l["txt"].replace(" ", ""))
                      or re.search(r"S\s*[Kk]\s*U\s*-?\s*" + cod.split("-")[1], l["txt"])), None)
        if not linha:
            detalhes.append((cod, "nao achou", None, None))
            continue
        # separa por posicao: os tres ultimos "numeros" da linha, da direita para a esquerda
        nums = [p["txt"] for p in linha["p"] if re.fullmatch(r"[\d.,]+", p["txt"])]
        q_lido = nums[-3] if len(nums) >= 3 else None
        t_lido = nums[-1] if nums else None
        okq = q_lido is not None and q_lido.strip(".,") == qtd
        okt = t_lido is not None and t_lido == tot
        qtd_ok += okq
        valor_ok += okt
        detalhes.append((cod, "ok" if okq else f"qtd lida {q_lido} (esperado {qtd})",
                         "ok" if okt else f"total lido {t_lido} (esperado {tot})", None))

    # conferencia: soma dos totais lidos == total da nota lido
    total_lido = None
    for l in linhas:
        m = re.search(r"TOTAL.*?R\$\s*([\d.,]+)", l["txt"])
        if m:
            total_lido = m.group(1)
    soma_itens = 0.0
    for cod, desc, qtd, unit, tot in ITENS:
        soma_itens += float(tot.replace(".", "").replace(",", "."))
    conf = [p["conf"] for p in palavras]
    print(f"\n--- foto {nome}: folha detectada={achou}, {len(linhas)} linhas, "
          f"conf {sum(conf)/len(conf):.1f}%")
    for d in detalhes:
        print("    ", d)
    print(f"     qtd certa: {qtd_ok}/{len(ITENS)} | total do item certo: {valor_ok}/{len(ITENS)}"
          f" | total da nota lido: {total_lido} | soma do gabarito: {soma_itens:.2f}")
    return {"foto": nome, "itens": len(ITENS), "qtd_ok": qtd_ok, "valor_ok": valor_ok,
            "total_nota_lido": total_lido, "conf_media": round(sum(conf)/len(conf), 1)}


if __name__ == "__main__":
    limpa = desenhar_nota()
    res = []
    for nome, p in PRESETS.items():
        pil = fotografar(limpa, p)
        pil.save(f"/home/user/qa/spike-foto-{nome}.jpg", quality=p["jpeg"])
        res.append(avaliar(pil, nome))
    with open("/home/user/qa/spike-ocr-qualidade.json", "w") as f:
        json.dump(res, f, indent=2, ensure_ascii=False)
    print("\nRESUMO")
    for r in res:
        print(f"  {r['foto']:<6} qtd {r['qtd_ok']}/8  valor {r['valor_ok']}/8  "
              f"conf {r.get('conf_media')}%  total_nota={r.get('total_nota_lido')}")

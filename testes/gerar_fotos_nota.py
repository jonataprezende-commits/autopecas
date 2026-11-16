"""Gera as fotos de teste da nota de compra (3 qualidades) para o leitor de notas.

A nota sintetica tem 8 itens e total R$ 932,20 (mesmo gabarito usado nos testes
Go: autopecas/nota_foto_test.go). A folha fica inteira dentro do quadro, com
mesa em volta - como numa foto de celular de verdade. As tres variantes:

  boa   = celular apoiado, luz boa, alta resolucao
  media = na mao, sombra de um lado, desfoque leve
  ruim  = torta, sombra forte, JPEG apertado

Uso: python3 qa/gerar_fotos_nota.py
"""
import io
import os

import cv2
import numpy as np
from PIL import Image, ImageDraw, ImageFont

FONTE = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
FONTE_B = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"
DESTINO = "/home/user/autopecas/testdata"

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

PRESETS = {
    "boa": dict(escala=1.0, angulo=1.2, sombra=0.18, blur=0.4, jpeg=90, ruido=2, persp=12),
    "media": dict(escala=0.72, angulo=3.0, sombra=0.38, blur=0.8, jpeg=72, ruido=6, persp=26),
    "ruim": dict(escala=0.58, angulo=5.0, sombra=0.55, blur=1.2, jpeg=58, ruido=10, persp=40),
}


def desenhar_nota():
    L, A = 1500, 2000
    img = Image.new("RGB", (L, A), "white")
    d = ImageDraw.Draw(img)
    f_tit = ImageFont.truetype(FONTE_B, 42)
    f = ImageFont.truetype(FONTE, 27)
    f_p = ImageFont.truetype(FONTE, 23)

    d.text((70, 60), "AUTO PECAS CENTRAL LTDA", font=f_tit, fill="black")
    d.text((70, 118), "CNPJ 12.345.678/0001-99  -  DANFE / NOTA DE ENTRADA", font=f_p, fill="black")
    d.text((70, 154), "Fornecedor: DISTRIBUIDORA DE AUTOPECAS XYZ LTDA", font=f_p, fill="black")
    d.text((70, 188), "NF-e 000.123.456  Serie 001  Data 12/11/2026", font=f_p, fill="black")

    y = 270
    d.text((70, y), "CODIGO        DESCRICAO                          QTD   UNIT      TOTAL", font=f, fill="black")
    d.line((70, y + 36, L - 70, y + 36), fill="black", width=2)
    y += 64
    for cod, desc, qtd, unit, tot in ITENS:
        d.text((70, y), f"{cod:<12}{desc:<34}{qtd:>3}   {unit:>7}   {tot:>8}", font=f, fill="black")
        y += 56
    d.line((70, y + 8, L - 70, y + 8), fill="black", width=2)
    # total alinhado a direita DENTRO da folha (medindo o texto: antes ele saia
    # pela borda e o valor ficava fora da foto)
    texto_total = f"TOTAL DA NOTA  R$ {TOTAL}"
    larg_texto = d.textlength(texto_total, font=f_tit)
    d.text((L - 70 - larg_texto, y + 40), texto_total, font=f_tit, fill="black")
    d.text((70, y + 150), "Assinatura do recebedor: ______________________", font=f_p, fill="black")
    return img


def fotografar(nota, p):
    """Coloca a nota sobre a mesa, aplica perspectiva, giro, sombra e JPEG."""
    rng = np.random.default_rng(11)
    nl, na = nota.size
    # mesa grande em volta da folha: nada da nota pode sair do quadro
    moldura = 420
    mesa = Image.new("RGB", (nl + 2 * moldura, na + 2 * moldura), (108, 103, 97))
    mesa.paste(nota, (moldura, moldura))
    arr = np.array(mesa)
    H, W = arr.shape[:2]

    e = p["persp"]
    origem = np.float32([[0, 0], [W, 0], [W, H], [0, H]])
    destino = np.float32([
        [e * 2, e * 3],
        [W - e, e],
        [W - e * 1.5, H - e],
        [e / 2, H - e * 2],
    ])
    m = cv2.getPerspectiveTransform(origem, destino)
    arr = cv2.warpPerspective(arr, m, (W, H), borderValue=(108, 103, 97))
    m2 = cv2.getRotationMatrix2D((W / 2, H / 2), p["angulo"], 1.0)
    arr = cv2.warpAffine(arr, m2, (W, H), borderValue=(108, 103, 97))

    yy, xx = np.mgrid[0:H, 0:W]
    s = p["sombra"]
    luz = 1.0 - s * ((xx / W) * 0.5 + (yy / H) * 0.5)
    arr = np.clip(arr.astype(np.float32) * luz[..., None], 0, 255)
    arr = cv2.GaussianBlur(arr.astype(np.uint8), (3, 3), p["blur"])
    arr = np.clip(arr.astype(np.float32) + rng.normal(0, p["ruido"], arr.shape), 0, 255).astype(np.uint8)

    pil = Image.fromarray(arr)
    pil = pil.resize((int(W * p["escala"]), int(H * p["escala"])), Image.LANCZOS)
    buf = io.BytesIO()
    pil.save(buf, "JPEG", quality=p["jpeg"])
    return Image.open(io.BytesIO(buf.getvalue())).convert("RGB")


if __name__ == "__main__":
    os.makedirs(DESTINO, exist_ok=True)
    nota = desenhar_nota()
    nota.save(os.path.join(DESTINO, "nota-limpa.png"))
    for nome, preset in PRESETS.items():
        foto = fotografar(nota, preset)
        caminho = os.path.join(DESTINO, f"nota-foto-{nome}.jpg")
        foto.save(caminho, quality=preset["jpeg"])
        print(f"{nome}: {foto.size} -> {caminho} ({os.path.getsize(caminho)//1024} KB)")

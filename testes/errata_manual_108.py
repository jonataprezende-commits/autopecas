"""Gera as paginas de novidades da versao 1.0.8 (contas a pagar pela nota) e
anexa ao Manual-AutoPecas.pdf.

A 1.0.8 acrescenta:
  - contas a pagar: a nota do fornecedor vira boleto dentro do sistema;
  - rateio de frete/IPI/ST no custo da peca (lucro honesto);
  - trava contra lancar a mesma nota duas vezes;
  - tela "Contas a Pagar", conta manual (aluguel/luz), relatorio e alerta no Inicio.

Uso:  python3 qa/errata_manual_108.py [entrada.pdf] [saida.pdf]
"""
import sys

from pypdf import PdfReader, PdfWriter

from errata_manual_102 import anexar

LINHAS_108_A = [
    ("Novidades da versão 1.0.8 — contas a pagar pela nota", 18, True, 0),
    ("", 6, False, 0),
    ("Até aqui o sistema só olhava o estoque da nota: as peças entravam, mas o", 11, False, 10),
    ("boleto que você ia pagar continuava no papel. Agora, ao importar o XML da", 11, False, 2),
    ("nota, o AutoPeças lê quem emitiu, o número da nota, o valor total e os", 11, False, 2),
    ("boletos — e lança a conta a pagar sozinho.", 11, False, 2),

    ("1. Como funciona (nada muda no que você já faz)", 13, True, 18),
    ("   Importar XML → escolher o arquivo → “Analisar arquivo”.", 11, False, 4),
    ("   Se o arquivo for nota fiscal, aparece o painel:", 11, False, 2),
    ("", 6, False, 2),
    ("     📄 Nota do fornecedor encontrada                                   ", 10, False, 10),
    ("     Fornecedor  DISTRIBUIDORA DE PEÇAS LTDA    Nota  #12345/1         ", 10, False, 2),
    ("     Valor total (vNF)  R$ 1.750,00             Frete/IPI/ST  R$ 150,00 ", 10, False, 2),
    ("     001/3  19/11/2026  R$ 583,33                                       ", 10, False, 2),
    ("     002/3  19/12/2026  R$ 583,33                                       ", 10, False, 2),
    ("     003/3  19/01/2027  R$ 583,34                                       ", 10, False, 2),
    ("", 6, False, 2),
    ("   Confira e clique em “Importar agora”. As peças entram no estoque e os", 11, False, 4),
    ("   três boletos entram em Contas a Pagar, com vencimento e valor de cada um.", 11, False, 2),

    ("2. O que o XML precisa ter", 13, True, 18),
    ("   • emitente com nome (e CNPJ, se tiver) → nome do fornecedor na conta;", 11, False, 4),
    ("   • <vNF> (valor total da nota) → valor a pagar;", 11, False, 2),
    ("   • <cobr><dup> (os boletos) → uma conta por boleto, com vencimento;", 11, False, 2),
    ("   • chave da NF-e → serve de “impressão digital” contra lançar duas vezes.", 11, False, 2),
    ("   Se a nota não trouxer os boletos, entra UMA conta com o valor total e o", 11, False, 2),
    ("   vencimento que você escolher (o sistema sugere emissão + 30 dias).", 11, False, 2),
    ("   Lista de produtos (catálogo, cotação) continua igual: não cria conta.", 11, False, 2),
]

LINHAS_108_B = [
    ("3. O fornecedor é cadastrado sozinho (sem duplicar)", 13, True, 0),
    ("   O sistema procura o fornecedor pelo CNPJ do XML. Se já existe, usa o", 11, False, 10),
    ("   cadastro que você tem (mesmo com pontuação diferente). Se não existe,", 11, False, 2),
    ("   cria com o nome que veio na nota — você não digita nada.", 11, False, 2),
    ("   As peças da nota já entram ligadas a esse fornecedor.", 11, False, 2),

    ("4. Frete, IPI e ST entram no custo da peça (importante)", 13, True, 18),
    ("   O valor total da nota quase nunca é igual à soma dos produtos: frete, IPI", 11, False, 4),
    ("   e substituição tributária vem por fora. Se o sistema ignorasse isso, o", 11, False, 2),
    ("   custo da peça ficaria menor do que foi pago e o lucro mostrado seria maior", 11, False, 2),
    ("   do que o dinheiro que sobra no caixa.", 11, False, 2),
    ("   Por isso a diferença é distribuída no custo de cada peça, na proporção do", 11, False, 2),
    ("   valor de cada item. Exemplo da nota acima:", 11, False, 2),
    ("     Pastilha: R$ 100,00 de produto + R$ 9,38 de frete/IPI = custo R$ 109,38", 10, False, 10),
    ("     Disco:    R$ 120,00 de produto + R$ 11,25 de frete/IPI = custo R$ 131,25", 10, False, 2),
    ("   O preço de venda é recalculado com a sua margem em cima do custo real.", 11, False, 4),
    ("   Se você não quiser o rateio em alguma nota, é só desmarcar a caixa", 11, False, 2),
    ("   “Somar frete/IPI/ST no custo das peças”.", 11, False, 2),

    ("5. A mesma nota não paga duas vezes", 13, True, 18),
    ("   Cada nota importada fica guardada com a chave de acesso. Se você importar", 11, False, 4),
    ("   o mesmo arquivo de novo (para atualizar preço, por exemplo), o sistema", 11, False, 2),
    ("   atualiza o estoque e avisa: “esta nota já estava lançada — não criei outra", 11, False, 2),
    ("   conta a pagar”. Botão “Notas importadas” mostra o histórico.", 11, False, 2),
]

LINHAS_108_C = [
    ("6. A tela Contas a Pagar", 13, True, 0),
    ("   Menu esquerdo → Contas a Pagar.", 11, False, 10),
    ("   • Em aberto / Vencidas / Pagas / Todas — igual à tela de Contas a Receber;", 11, False, 4),
    ("   • mostra fornecedor, número da nota, parcela (001/3), valor e vencimento;", 11, False, 2),
    ("   • botão “Pagar”: você confirma o valor realmente pago (juros e desconto", 11, False, 2),
    ("     entram aqui) e a data do pagamento;", 11, False, 2),
    ("   • botão “Editar” corrige valor e vencimento de conta em aberto;", 11, False, 2),
    ("   • botão “Cancelar” tira a conta do total sem apagar o histórico;", 11, False, 2),
    ("   • “+ Nova conta” lança a despesa que não vem de nota: aluguel, luz,", 11, False, 2),
    ("     salário, frete avulso;", 11, False, 2),
    ("   • “Notas importadas” lista as notas que já entraram no estoque.", 11, False, 2),

    ("7. Onde mais isso aparece", 13, True, 18),
    ("   • Início: alerta vermelho “boletos de fornecedor vencidos” e cartão", 11, False, 4),
    ("     “A pagar em aberto” com o total e quantos estão vencidos;", 11, False, 2),
    ("   • Relatórios: “Contas a pagar (boletos de fornecedor)”, com total em", 11, False, 2),
    ("     aberto e vencido, exportável em CSV (Excel) e PDF;", 11, False, 2),
    ("   • Backup: as contas a pagar e as notas entram no backup como o resto.", 11, False, 2),

    ("8. O que ele NÃO faz (de propósito)", 13, True, 18),
    ("   • não paga boleto sozinho nem conecta no banco — só controla;", 11, False, 4),
    ("   • não adivinha juros, multa ou nota cancelada: você ajusta na baixa;", 11, False, 2),
    ("   • Nota por Foto não lança conta a pagar (o OCR não lê o valor total com", 11, False, 2),
    ("     segurança): nesse caso use “+ Nova conta”.", 11, False, 2),
]

LINHAS_108_D = [
    ("Como atualizar para a 1.0.8 (não perde nada)", 13, True, 0),
    ("   1. Encerrar o AutoPeças e fazer backup (Backup e Ajustes → Fazer backup agora).", 11, False, 10),
    ("   2. Rodar o AutoPecas-Setup-1.0.8.exe por cima. O banco é convertido sozinho", 11, False, 2),
    ("      (o programa guarda antes uma cópia “antes-de-migrar-…”).", 11, False, 2),
    ("   3. Abrir o sistema: o rodapé do menu mostra v1.0.8.", 11, False, 2),
    ("   4. Importar a próxima nota do fornecedor e conferir o painel da nota.", 11, False, 2),
    ("   5. Menu Contas a Pagar → conferir os boletos lançados.", 11, False, 2),

    ("Primeiros passos para começar hoje", 13, True, 18),
    ("   a) Importe a nota que está em aberto agora: os boletos entram sozinhos;", 11, False, 4),
    ("   b) lance o aluguel e a luz por “+ Nova conta” (leva 20 segundos cada);", 11, False, 2),
    ("   c) toda segunda, abra Contas a Pagar → “Vencidas” e veja o que está atrasado;", 11, False, 2),
    ("   d) no fechamento do dia, o que saiu de dinheiro já está no sistema.", 11, False, 2),
    ("", 6, False, 4),
    ("Dúvida comum: “lancei a nota mas o valor do boleto veio diferente do", 11, False, 8),
    ("sistema”. O XML traz o valor de face. Se o boleto foi pago com juros, use", 11, False, 2),
    ("“Pagar” e informe o valor real — o valor da nota continua registrado.", 11, False, 2),
]


def remover_ultimas(entrada, saida, quantas):
    """Recorta as ultimas paginas (as da versao anterior) e grava o PDF base."""
    leitor = PdfReader(entrada)
    escritor = PdfWriter()
    total = len(leitor.pages)
    for i, pagina in enumerate(leitor.pages):
        if i >= total - quantas:
            break
        escritor.add_page(pagina)
    with open(saida, "wb") as f:
        escritor.write(f)
    print(f"base: {total} paginas -> {total - quantas}")


def main():
    entrada = sys.argv[1] if len(sys.argv) > 1 else "/home/user/Manual-AutoPecas.pdf"
    saida = sys.argv[2] if len(sys.argv) > 2 else entrada
    base = "/tmp/manual-base-107.pdf"
    remover_ultimas(entrada, base, 4)   # tira as 4 paginas da 1.0.7
    anexar(base, saida, [LINHAS_108_A, LINHAS_108_B, LINHAS_108_C, LINHAS_108_D])


if __name__ == "__main__":
    main()

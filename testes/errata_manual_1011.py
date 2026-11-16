"""Pagina de novidades da versao 1.0.11 (botao "Ligar o acesso pelo celular")
anexada ao Manual-AutoPecas.pdf.

O que a 1.0.11 corrige:
  - o botao respondia "404 page not found" (mandava GET numa rota de POST);
  - o programa "reiniciava" para passar a atender a rede e as vezes nao voltava;
  - agora o endereco muda na hora, sem reiniciar e sem derrubar quem esta usando.

Uso:  python3 qa/errata_manual_1011.py [entrada.pdf] [saida.pdf]
"""
import sys

from errata_manual_102 import anexar

LINHAS_1011_A = [
    ("Novidades da versão 1.0.11 — o botão “Ligar o acesso pelo celular”", 18, True, 0),
    ("", 6, False, 0),
    ("Se você clicou em “Ligar o acesso pelo celular” e apareceu a mensagem", 11, False, 10),
    ("“404 page not found”, o problema era no programa — e está corrigido.", 11, False, 2),
    ("Nada mudou no jeito de usar; o botão passou a funcionar.", 11, False, 2),

    ("1. O que estava errado", 13, True, 18),
    ("   a) O botão pedia a informação ao programa do jeito errado (mandava um", 11, False, 4),
    ("      pedido de “leitura” quando o programa esperava um pedido de “gravação”).", 11, False, 2),
    ("      O programa respondia “404 page not found” e nada acontecia.", 11, False, 2),
    ("   b) Mesmo quando o pedido chegava, o programa precisava fechar e abrir de", 11, False, 2),
    ("      novo para passar a atender a rede. Se a porta ainda estivesse ocupada", 11, False, 2),
    ("      por um instante, o programa novo desistia e o sistema simplesmente", 11, False, 2),
    ("      saía do ar — a tela do navegador ficava girando e não voltava.", 11, False, 2),

    ("2. Como funciona agora", 13, True, 18),
    ("   • Clicou em “Ligar o acesso pelo celular”, escolheu a senha e confirmou:", 11, False, 4),
    ("     o sistema passa a atender os celulares na mesma hora. Aparece o QR", 11, False, 2),
    ("     Code e os endereços de rede para digitar no celular.", 11, False, 2),
    ("   • Ninguém é desconectado e a tela não recarrega: o programa continua o", 11, False, 2),
    ("     mesmo, só muda o “endereço de atendimento”. Se o computador estava só", 11, False, 2),
    ("     atendendo a si próprio, passa a atender a loja toda.", 11, False, 2),
    ("   • Desligar também é imediato: o celular perde o acesso na hora.", 11, False, 2),
    ("   • Trocar a senha vale na hora; quem estava conectado pelo celular precisa", 11, False, 2),
    ("     entrar de novo com a senha nova.", 11, False, 2),
    ("   • Se, por algum motivo, o computador não conseguir atender a rede (outro", 11, False, 2),
    ("     programa usando a mesma porta, por exemplo), aparece um aviso claro na", 11, False, 2),
    ("     tela e o sistema continua funcionando neste computador — antes ele saía", 11, False, 2),
    ("     do ar sem explicar nada.", 11, False, 2),
]

LINHAS_1011_B = [
    ("3. Como conferir que está funcionando", 13, True, 18),
    ("   1. Abra o AutoPeças neste computador (Início → Acessar pelo celular).", 11, False, 4),
    ("   2. Digite a senha (mínimo 4 caracteres), repita e clique em", 11, False, 2),
    ("      “Ligar o acesso pelo celular”.", 11, False, 2),
    ("   3. O painel deve mostrar “Ligado.” com um QR Code e um ou mais endereços", 11, False, 2),
    ("      parecidos com http://192.168.0.10:8787.", 11, False, 2),
    ("   4. Aponte a câmera do celular para o QR Code (o celular precisa estar na", 11, False, 2),
    ("      mesma Wi-Fi da loja) e entre com a senha.", 11, False, 2),
    ("   5. Para conferir o computador: a tela aqui continua aberta, sem pedir", 11, False, 2),
    ("      senha, exatamente como antes.", 11, False, 2),
    ("", 6, False, 6),
    ("   Se o painel disser “O computador não está em nenhuma rede”, ligue o cabo", 11, False, 8),
    ("   ou conecte a Wi-Fi da loja e clique de novo em ligar.", 11, False, 2),

    ("Como atualizar para a 1.0.11 (não perde nada)", 13, True, 18),
    ("   1. Encerrar o AutoPeças e fazer um backup (Backup e Ajustes -> Fazer", 11, False, 10),
    ("      backup agora).", 11, False, 2),
    ("   2. Rodar o AutoPecas-Setup-1.0.11.exe por cima do que já está instalado.", 11, False, 2),
    ("      O banco é convertido sozinho e o sistema guarda antes uma cópia", 11, False, 2),
    ("      “antes-de-migrar-…”.", 11, False, 2),
    ("   3. Abrir o sistema: o rodapé do menu mostra v1.0.11.", 11, False, 2),
    ("   4. Se o acesso pelo celular já estava ligado, ele continua ligado — a senha", 11, False, 2),
    ("      e os aparelhos conectados não mudam.", 11, False, 2),
]


def main():
    entrada = sys.argv[1] if len(sys.argv) > 1 else "/home/user/Manual-AutoPecas.pdf"
    saida = sys.argv[2] if len(sys.argv) > 2 else entrada
    anexar(entrada, saida, [LINHAS_1011_A, LINHAS_1011_B])


if __name__ == "__main__":
    main()

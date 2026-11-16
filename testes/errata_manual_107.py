"""Gera as paginas de novidades da versao 1.0.7 e anexa ao Manual-AutoPecas.pdf.

A 1.0.7 traz dois blocos:
  - acesso pelo celular (Wi-Fi da loja, QR Code, senha, fora da loja com VPN/tunel);
  - Plano B: backup de hora em hora, backup ao fechar o caixa, painel de saude,
    teste de restauracao, kit de emergencia e pasta na nuvem (Dropbox/Drive/OneDrive).

Uso:  python3 qa/errata_manual_107.py [entrada.pdf] [saida.pdf]
"""
import sys

from pypdf import PdfReader, PdfWriter

from errata_manual_102 import anexar

LINHAS_107_A = [
    ("Novidades da versão 1.0.7 — o sistema no celular", 18, True, 0),
    ("", 6, False, 0),
    ("O celular não guarda dados nenhum: ele mostra a mesma tela do computador da", 11, False, 10),
    ("loja. Quem manda continua sendo o computador — por isso ele precisa estar", 11, False, 2),
    ("ligado e com o AutoPeças aberto.", 11, False, 2),

    ("1. Ligar o acesso (uma vez, no computador)", 13, True, 18),
    ("   Tela Início → painel “Acessar pelo celular” → escolha uma senha (mínimo 4", 11, False, 4),
    ("   caracteres) → botão “Ligar o acesso pelo celular”.", 11, False, 2),
    ("   O programa reinicia sozinho (uns 3 segundos) e volta já mostrando o", 11, False, 2),
    ("   QR Code e o endereço. Na primeira vez o Windows pergunta sobre o", 11, False, 2),
    ("   Firewall: marque “Rede privada” e clique em “Permitir acesso”.", 11, False, 2),

    ("2. Usar no celular", 13, True, 18),
    ("   Aponte a câmera do celular para o QR Code → o navegador abre → digite a", 11, False, 4),
    ("   senha. A tela se adapta: o menu vira a gaveta (☰), as tabelas rolam dentro", 11, False, 2),
    ("   do quadro e os campos ficam grandes para o dedo.", 11, False, 2),
    ("   Dá para consultar preço e estoque, fechar venda, gerar orçamento e usar o", 11, False, 2),
    ("   Nota por Foto (tira a foto da nota do fornecedor com a câmera).", 11, False, 2),

    ("3. Quem pede senha", 13, True, 18),
    ("   • o computador da loja (127.0.0.1) NÃO pede senha — abre direto, como sempre;", 11, False, 4),
    ("   • celular e qualquer outro computador da rede pedem senha (vale 12 horas);", 11, False, 2),
    ("   • depois de 8 senhas erradas, aquele aparelho fica bloqueado por 5 minutos;", 11, False, 2),
    ("   • o botão “Sair do sistema” só aparece em quem entrou por senha.", 11, False, 2),
    ("   A senha fica gravada embaralhada (hash com sal) dentro do banco: nem quem", 11, False, 2),
    ("   abrir o arquivo consegue ler.", 11, False, 2),
]

LINHAS_107_B = [
    ("Acessar de fora da loja (4G / outra rede)", 13, True, 0),
    ("O celular só enxerga o sistema na mesma rede. Para usar na rua existem dois", 11, False, 10),
    ("caminhos — nos dois o computador da loja precisa estar ligado.", 11, False, 2),
    ("", 6, False, 4),
    ("   A) VPN gratuita (Tailscale) — mais seguro", 12, True, 4),
    ("      1. Crie a conta gratuita em tailscale.com e instale no computador da loja.", 11, False, 6),
    ("      2. Instale também no celular e entre com a MESMA conta.", 11, False, 2),
    ("      3. O aplicativo mostra um endereço tipo 100.x.x.x — abra no celular:", 11, False, 2),
    ("         http://100.x.x.x:8787 e entre com a senha do sistema.", 11, False, 2),
    ("      Nada fica exposto na internet: só os seus aparelhos se enxergam.", 10, False, 6),
    ("", 6, False, 4),
    ("   B) Túnel com endereço próprio (Cloudflare Tunnel / ngrok)", 12, True, 4),
    ("      O sistema ganha um endereço (ex.: autopecas-sualoja.com) que abre de", 11, False, 6),
    ("      qualquer lugar, sem instalar nada no celular. Duas travas obrigatórias:", 11, False, 2),
    ("      1. senha longa (8+ caracteres) em “Acessar pelo celular”;", 11, False, 2),
    ("      2. abrir o programa com o endereço liberado, senão ele recusa a conexão:", 11, False, 2),
    ("         AutoPecas.exe -proxy -hosts-permitidos autopecas-sualoja.com", 10, False, 6),
    ("      Aqui a loja fica na internet: a proteção passa a ser a senha. Se puder,", 10, False, 6),
    ("      prefira a opção A.", 10, False, 2),
]

LINHAS_107_C = [
    ("Novidades da versão 1.0.7 — Plano B: o backup que salva a loja", 18, True, 0),
    ("", 6, False, 0),
    ("Antes, se o computador quebrasse às 17h, o backup do dia tinha sido feito às 8h", 11, False, 10),
    ("e o dia inteiro de vendas se perdia. Agora não.", 11, False, 2),

    ("1. Backup de hora em hora (de verdade)", 13, True, 18),
    ("   Uma cópia por hora, guardando as 24 últimas. Na pior das hipóteses você", 11, False, 4),
    ("   perde 1 hora de movimento, não o dia. O intervalo é ajustável em Backup e", 11, False, 2),
    ("   Ajustes (mínimo 15 minutos).", 11, False, 2),

    ("2. Backup ao fechar o caixa", 13, True, 18),
    ("   Na tela Fechamento do Dia existe o botão “Fechar o caixa (com backup)”:", 11, False, 4),
    ("   ele fecha o dia e guarda a cópia daquele momento. Fim do dia = backup", 11, False, 2),
    ("   garantido, sem depender da memória de ninguém.", 11, False, 2),

    ("3. Painel “Plano B” (Backup e Ajustes)", 13, True, 18),
    ("   Um semáforo diz na hora se você está protegido:", 11, False, 4),
    ("   • VERDE: último backup há menos de 1 hora;", 11, False, 2),
    ("   • AMARELO: mais de 1 hora;", 11, False, 2),
    ("   • VERMELHO: mais de 1 dia (ou nenhum backup).", 11, False, 2),
    ("   Mostra também: última cópia de hora em hora, último fechamento, quanto há", 11, False, 2),
    ("   em cada tipo de cópia e o espaço livre no disco.", 11, False, 2),

    ("4. Testar a restauração (sem risco)", 13, True, 18),
    ("   O botão “Testar a restauração” abre o backup mais recente em modo somente", 11, False, 4),
    ("   leitura e mostra: quantas peças, quantas vendas, clientes e a última venda", 11, False, 2),
    ("   registrada. Nada é alterado — é só para você ter certeza de que o arquivo", 11, False, 2),
    ("   presta antes de precisar dele.", 11, False, 2),
]

LINHAS_107_D = [
    ("5. Kit de emergência", 13, True, 0),
    ("   O botão “Gerar kit de emergência” monta um único arquivo .zip com:", 11, False, 10),
    ("   • o programa (AutoPecas.exe) pronto para rodar sem instalar;", 11, False, 4),
    ("   • o backup mais recente;", 11, False, 2),
    ("   • um LEIA-ME com o passo a passo de 5 minutos para voltar a funcionar.", 11, False, 2),
    ("   Copie para um pendrive e guarde. Se o computador morrer, é só descompactar", 11, False, 2),
    ("   em outro computador, abrir o programa e restaurar o backup que vem dentro.", 11, False, 2),
    ("   O mesmo arquivo continua atendendo o celular no mesmo endereço: o celular", 11, False, 2),
    ("   é só tela, os dados são do computador.", 11, False, 2),

    ("6. Cópia na nuvem (Dropbox, Google Drive, OneDrive ou pendrive)", 13, True, 18),
    ("   Em Backup e Ajustes → “Escolher a pasta…” o sistema mostra as pastas de", 11, False, 4),
    ("   nuvem que já existem no computador. Escolha uma (ou aponte o pendrive):", 11, False, 2),
    ("   depois de cada backup ele copia o mesmo arquivo para lá, e o aplicativo da", 11, False, 2),
    ("   nuvem faz o envio sozinho quando a internet volta.", 11, False, 2),
    ("   Importante: o AutoPeças NÃO usa internet em momento nenhum. Ele grava numa", 11, False, 2),
    ("   pasta; quem envia para a nuvem é o Dropbox/Drive/OneDrive instalado.", 11, False, 2),
    ("   Se a pasta sumir (pendrive removido, conta desconectada), o backup local", 11, False, 2),
    ("   continua funcionando e o painel mostra o aviso para corrigir.", 11, False, 2),

    ("Se acabar a luz", 13, True, 18),
    ("   O banco não corrompe (o sistema usa gravação protegida): a venda ou entrou", 11, False, 4),
    ("   inteira ou não entrou. O que estava sendo digitado e não foi salvo se perde.", 11, False, 2),
    ("   O único investimento que resolve de verdade é um nobreak de 600 VA, que", 11, False, 2),
    ("   segura o computador por 10 a 20 minutos — tempo de fechar o caixa.", 11, False, 2),

    ("Como atualizar para a 1.0.7 (não perde nada)", 13, True, 18),
    ("   1. Encerrar o AutoPeças e fazer backup (Backup e Ajustes → Fazer backup agora).", 11, False, 4),
    ("   2. Rodar o AutoPecas-Setup-1.0.7.exe por cima. O banco é migrado sozinho.", 11, False, 2),
    ("   3. Abrir: o rodapé do menu mostra v1.0.7.", 11, False, 2),
    ("   4. Início → “Acessar pelo celular” → definir a senha e ligar.", 11, False, 2),
    ("   5. Backup e Ajustes → conferir o painel Plano B (deve ficar verde) e escolher", 11, False, 2),
    ("      a pasta da nuvem.", 11, False, 2),
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
    base = "/tmp/manual-base-106.pdf"
    remover_ultimas(entrada, base, 2)   # tira as 2 paginas da 1.0.6
    anexar(base, saida, [LINHAS_107_A, LINHAS_107_B, LINHAS_107_C, LINHAS_107_D])


if __name__ == "__main__":
    main()

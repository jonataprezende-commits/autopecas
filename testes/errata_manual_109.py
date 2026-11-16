"""Gera as paginas de novidades da versao 1.0.9 (espaco dos backups) e anexa
ao Manual-AutoPecas.pdf, SEM apagar as paginas da 1.0.8 (contas a pagar
continua documentado).

A 1.0.9 acrescenta:
  - limite por tipo de copia pode ser 1 (guardar so a ultima);
  - teto de espaco em MB (padrao 300) — o disco nunca enche;
  - botao "Apagar os antigos agora";
  - tres perfis prontos: Economia maxima / Economico e seguro / Padrao;
  - painel do Plano B mostra quanto a pasta de backups ocupa.

Uso:  python3 qa/errata_manual_109.py [entrada.pdf] [saida.pdf]
"""
import sys

from errata_manual_102 import anexar

LINHAS_109_A = [
    ("Novidades da versão 1.0.9 — os backups não enchem o disco", 18, True, 0),
    ("", 6, False, 0),
    ("O sistema faz uma cópia de segurança de hora em hora e guarda também uma", 11, False, 10),
    ("cópia de cada dia, cada fechamento de caixa e cada restauração. Isso é o", 11, False, 2),
    ("que salva o dia quando o computador dá problema — mas, se nada for apagado,", 11, False, 2),
    ("a pasta de backups cresce para sempre. Na 1.0.9 o próprio sistema faz a", 11, False, 2),
    ("faxina: guarda as cópias mais novas e apaga as velhas, sozinho.", 11, False, 2),

    ("1. O que já era (e continua igual)", 13, True, 18),
    ("   Toda vez que nasce uma cópia, o sistema já apagava as mais antigas do", 11, False, 4),
    ("   mesmo tipo. Os limites de fábrica são:", 11, False, 2),
    ("      de hora em hora ......................... 24 cópias (1 dia)", 10, False, 10),
    ("      diário (ao abrir o sistema) ............. 30 cópias (1 mês)", 10, False, 2),
    ("      ao fechar o caixa ....................... 60 cópias", 10, False, 2),
    ("      manual (Ctrl+B) ......................... 60 cópias", 10, False, 2),
    ("      segurança (antes de restaurar) .......... 10 cópias", 10, False, 2),

    ("2. O que mudou agora", 13, True, 18),
    ("   a) Qualquer limite pode ser 1. Antes o sistema não deixava descer de 2;", 11, False, 4),
    ("      agora você pode mandar guardar só a última cópia de cada tipo.", 11, False, 2),
    ("   b) Teto de espaço (novo). Além dos limites por tipo, existe um teto em", 11, False, 2),
    ("      megabytes para a pasta toda — de fábrica, 300 MB. Quando a pasta", 11, False, 2),
    ("      chega nesse tamanho, o sistema apaga as cópias mais antigas até", 11, False, 2),
    ("      voltar a caber. Ele nunca apaga a última cópia de um tipo: sempre", 11, False, 2),
    ("      sobra alguma coisa para restaurar. Teto 0 = sem teto.", 11, False, 2),
    ("   c) Botão “Apagar os antigos agora”: faz a limpeza na hora e diz quantos", 11, False, 2),
    ("      arquivos saíram e quanto de disco foi liberado.", 11, False, 2),
    ("   d) O painel do Plano B mostra quantos arquivos existem, quanto ocupam e", 11, False, 2),
    ("      qual é o teto — e a tabela por tipo ganhou a coluna “Limite”.", 11, False, 2),
]

LINHAS_109_B = [
    ("3. Quanto isso ocupa, em números de verdade", 13, True, 0),
    ("   Uma cópia do banco hoje tem cerca de 0,23 MB (é um arquivo pequeno; vai", 11, False, 10),
    ("   crescendo devagar, conforme entram notas e vendas). Com os limites de", 11, False, 2),
    ("   fábrica:", 11, False, 2),
    ("      24 cópias por dia .................. ~5,7 MB por dia", 10, False, 10),
    ("      1 mês de cópias diárias ............ ~6 MB", 10, False, 2),
    ("      fechamentos + manuais + segurança .. poucos MB", 10, False, 2),
    ("      pasta inteira ...................... ~28 MB (bem abaixo do teto)", 10, False, 2),
    ("   Mesmo que o banco cresça 10 vezes, o teto de 300 MB segura a pasta.", 11, False, 6),
    ("   Só para comparar: sem faxina nenhuma, guardar uma cópia por hora por um", 11, False, 2),
    ("   ano daria mais de 2 GB.", 11, False, 2),

    ("4. Os três perfis prontos (Configurações)", 13, True, 18),
    ("   Menu Configurações -> bloco “Quanto espaço os backups ocupam” -> um", 11, False, 4),
    ("   clique escolhe o perfil:", 11, False, 2),
    ("     ECONOMIA MÁXIMA — 1 de hora em hora, 1 diário, 1 fechamento,", 10, False, 10),
    ("     1 manual, 1 segurança. Ocupa ~1 MB. A pasta fica com 4 a 5 arquivos.", 10, False, 2),
    ("     Atenção: guardando 1 cópia por hora e 1 por dia, você tem poucas", 10, False, 2),
    ("     voltas atrás. Se o banco corromper de manhã e ninguém perceber até a", 10, False, 2),
    ("     tarde, a cópia boa do dia anterior talvez já tenha sido apagada.", 10, False, 2),
    ("     ECONÔMICO E SEGURO (recomendado) — 3 de hora em hora, 7 diários,", 10, False, 10),
    ("     30 fechamentos, 10 manuais, 3 de segurança. Ocupa ~12 MB. São 3 horas de", 10, False, 2),
    ("     hora em hora e uma semana de cópias diárias: dá para voltar atrás com", 10, False, 2),
    ("     folga, e o disco nem sente.", 10, False, 2),
    ("     PADRÃO — 24 / 30 / 60 / 60 / 10 (os limites de fábrica). ~28 MB.", 10, False, 10),
    ("   Para escolher números no dedo: Configurações -> campos “Manter cópias”,", 11, False, 8),
    ("   “Manter cópias manuais”, “Manter cópias de segurança” e “Espaço máximo", 11, False, 2),
    ("   dos backups (MB)”.", 11, False, 2),
]

LINHAS_109_C = [
    ("5. Fazer a faxina agora", 13, True, 0),
    ("   Menu Backup e Ajustes (Plano B) -> botão “Apagar os antigos agora”.", 11, False, 10),
    ("   Aparece “Apagados N arquivos — liberados X MB”, e o painel é atualizado.", 11, False, 2),
    ("   É seguro: a limpeza respeita os limites e o teto, e nunca deixa um tipo", 11, False, 2),
    ("   de cópia sem nenhum arquivo.", 11, False, 2),
    ("   Você também pode limpar pelo atalho do Plano B sempre que quiser — não", 11, False, 2),
    ("   precisa esperar o próximo backup.", 11, False, 4),

    ("6. Onde os backups ficam", 13, True, 18),
    ("   Na pasta de dados do sistema, subpasta “backups”. No Plano B você vê o", 11, False, 4),
    ("   caminho completo. Se você usa a pasta de sincronização (Dropbox/OneDrive", 11, False, 2),
    ("   da sua escolha), a cópia que vai para lá é uma só — a pasta de backups do", 11, False, 2),
    ("   PC é que é limpa.", 11, False, 2),
    ("   Regra de ouro: o backup só serve se estiver em outro lugar. Mantenha a", 11, False, 2),
    ("   cópia externa (pendrive, HD ou pasta de sincronização) — a faxina mexe", 11, False, 2),
    ("   só na pasta local do computador.", 11, False, 4),

    ("Como atualizar para a 1.0.9 (não perde nada)", 13, True, 18),
    ("   1. Encerrar o AutoPeças e fazer um backup (Backup e Ajustes -> Fazer", 11, False, 10),
    ("      backup agora).", 11, False, 2),
    ("   2. Rodar o AutoPecas-Setup-1.0.9.exe por cima. O banco é convertido", 11, False, 2),
    ("      sozinho e o sistema guarda antes uma cópia “antes-de-migrar-…”.", 11, False, 2),
    ("   3. Abrir o sistema: o rodapé do menu mostra v1.0.9.", 11, False, 2),
    ("   4. Configurações -> escolher “Econômico e seguro” (recomendado) ou", 11, False, 2),
    ("      deixar o Padrão.", 11, False, 2),
    ("   5. Backup e Ajustes -> conferir o painel (arquivos, tamanho, teto) e", 11, False, 2),
    ("      clicar em “Apagar os antigos agora” uma vez.", 11, False, 2),
    ("", 6, False, 6),
    ("Dúvida comum: “apago tudo e guardo só 1 backup?”. Pode — é o perfil", 11, False, 8),
    ("Economia máxima. Mas o barato pode sair caro: com uma cópia só, um", 11, False, 2),
    ("problema que ninguém viu na hora já substituiu a única cópia boa. Por isso", 11, False, 2),
    ("o recomendado é o “Econômico e seguro”: 3 de hora em hora e 7 diários, que", 11, False, 2),
    ("ocupam ~12 MB — nada para um computador — e salvam de verdade.", 11, False, 2),
]


def main():
    entrada = sys.argv[1] if len(sys.argv) > 1 else "/home/user/Manual-AutoPecas.pdf"
    saida = sys.argv[2] if len(sys.argv) > 2 else entrada
    anexar(entrada, saida, [LINHAS_109_A, LINHAS_109_B, LINHAS_109_C])


if __name__ == "__main__":
    main()

"""Compacta o programa e o manual para dentro do instalador (payload/)."""
import zlib
def comprimir(origem, destino):
    dados = open(origem, 'rb').read()
    c = zlib.compressobj(9, zlib.DEFLATED, -15)   # deflate cru (flate do Go)
    saida = c.compress(dados) + c.flush()
    open(destino, 'wb').write(saida)
    print(f"{destino}: {len(dados)/1024/1024:.1f} MB -> {len(saida)/1024/1024:.1f} MB")
comprimir('/home/user/dist/AutoPecas.exe', 'payload/programa.deflate')
comprimir('/home/user/Manual-AutoPecas.pdf', 'payload/manual.deflate')

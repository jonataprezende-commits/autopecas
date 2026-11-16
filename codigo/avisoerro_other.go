//go:build !windows

package main

// Nas outras plataformas (usadas nos testes automatizados) o erro so vai para
// o log/erro.log; nao existe caixa de mensagem nativa.
func mostrarCaixaErro(msg string) {}

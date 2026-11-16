package main

// Testes de compatibilidade que NÃO são bugs conhecidos: garantem que backup
// antigo (1.0.0) restaura e migra sem perda.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestaurarBackupDa100(t *testing.T) {
	env := novaAppTeste(t)
	// gera um banco 1.0.0 (sem user_version e sem order_items.received_at)
	antigo := filepath.Join(t.TempDir(), "backup-1.0.0.db")
	criarBanco100(t, antigo)
	destino := filepath.Join(env.dir, "backups", "backup-1.0.0.db")
	dados := env.lerArquivo(antigo)
	if err := os.WriteFile(destino, dados, 0o644); err != nil {
		t.Fatalf("copiando backup antigo: %v", err)
	}

	r := env.post("/api/restaurar?arquivo=backup-1.0.0.db", map[string]any{})
	if r.status != 200 {
		t.Fatalf("restaurar backup da 1.0.0: %d %s", r.status, r.corpo)
	}
	if got := env.qint(`SELECT COUNT(*) FROM parts`); got != 1 {
		t.Errorf("pecas apos restaurar = %d, esperado 1", got)
	}
	if got := env.qstr(`SELECT description FROM parts WHERE id=1`); got != "Peca antiga" {
		t.Errorf("descricao = %q", got)
	}
	if got := env.qstr(`SELECT value FROM settings WHERE key='loja_nome'`); got != "Loja Antiga" {
		t.Errorf("configuracao da loja perdida: %q", got)
	}
	if got := env.qint(`SELECT COUNT(*) FROM order_items WHERE IFNULL(received_at,'x')=''`); got != 1 {
		t.Errorf("item do pedido antigo deveria ficar com received_at vazio (got %d)", got)
	}
	// continua utilizavel: vender a peca restaurada
	if v := env.post("/api/vendas", corpoVenda(1, 1, 2500, 0, 0, "dinheiro", map[string]any{"recebido_centavos": 2500})); v.status != 200 {
		t.Errorf("venda depois de restaurar backup antigo: %d %s", v.status, v.corpo)
	}
}

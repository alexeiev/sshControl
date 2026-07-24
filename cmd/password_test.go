package cmd

import "testing"

func TestResolvePassword(t *testing.T) {
	t.Run("envPassword lê da variável SCPW", func(t *testing.T) {
		t.Setenv(EnvPasswordVar, "segredo123")

		got, err := ResolvePassword(false, true, "prompt: ")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if got != "segredo123" {
			t.Fatalf("senha = %q, esperado %q", got, "segredo123")
		}
	})

	t.Run("envPassword tem precedência sobre askPassword", func(t *testing.T) {
		t.Setenv(EnvPasswordVar, "do-env")

		got, err := ResolvePassword(true, true, "prompt: ")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if got != "do-env" {
			t.Fatalf("senha = %q, esperado %q", got, "do-env")
		}
	})

	t.Run("envPassword com SCPW ausente retorna erro", func(t *testing.T) {
		t.Setenv(EnvPasswordVar, "")

		_, err := ResolvePassword(false, true, "prompt: ")
		if err == nil {
			t.Fatal("esperado erro quando SCPW está vazia, mas não houve")
		}
	})

	t.Run("sem flags retorna senha vazia", func(t *testing.T) {
		got, err := ResolvePassword(false, false, "prompt: ")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if got != "" {
			t.Fatalf("senha = %q, esperado vazia", got)
		}
	})
}

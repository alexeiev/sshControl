package cmd

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// EnvPasswordVar é o nome da variável de ambiente usada pela flag -P para
// fornecer a senha SSH sem prompt interativo.
const EnvPasswordVar = "SCPW"

// ResolvePassword resolve a senha a ser usada para autenticação SSH.
//
// Prioridade:
//  1. Flag -P (envPassword): lê a senha da variável de ambiente SCPW.
//     Retorna erro se a variável não estiver definida ou estiver vazia.
//  2. Flag -a (askPassword): solicita a senha interativamente usando o prompt.
//  3. Nenhuma flag: retorna senha vazia (autenticação por chave SSH ou agent).
//
// O parâmetro prompt é a mensagem exibida ao solicitar a senha interativamente.
func ResolvePassword(askPassword, envPassword bool, prompt string) (string, error) {
	if envPassword {
		password, ok := os.LookupEnv(EnvPasswordVar)
		if !ok || password == "" {
			return "", fmt.Errorf("variável de ambiente %s não definida ou vazia", EnvPasswordVar)
		}
		return password, nil
	}

	if askPassword {
		fmt.Print(prompt)
		passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("erro ao ler senha: %w", err)
		}
		return string(passwordBytes), nil
	}

	return "", nil
}

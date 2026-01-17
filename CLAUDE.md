# CLAUDE.md

Este arquivo fornece orientações ao Claude Code (claude.ai/code) ao trabalhar com código neste repositório.

## Visão Geral do Projeto

sshControl (`sc`) é um gerenciador de conexões SSH escrito em Go que oferece modos interativo (TUI) e CLI direto para gerenciar conexões SSH. Utiliza o framework Bubble Tea da Charm para a interface interativa e suporta conexões através de jump hosts.

## Comandos de Build

```bash
# Compila binários para Linux (amd64) e macOS (arm64)
make build

# Binários gerados em:
# - bin/amd64/sc (Linux)
# - bin/arm64/sc (macOS)

# Execução direta durante desenvolvimento
go run .

```

## Exemplos de Uso

### Modo Interativo (TUI)
```bash
# Abre menu interativo para selecionar host
sc

# Menu interativo usando usuário específico
sc -u ubuntu

# Menu interativo com jump host habilitado
sc -s
```

### Modo Direto (Sessão Interativa)
```bash
# Conecta a host do config.yaml
sc webserver

# Conecta diretamente a IP
sc 192.168.1.50

# Conecta com usuário e porta específicos
sc ubuntu@192.168.1.50:2222

# Conecta via jump host
sc -s production-db
```

### Execução de Comando Remoto (Host Único)
```bash
# Executa comando em host do config.yaml
sc -c "uptime" webserver

# Executa comando em IP direto
sc -c "df -h" 192.168.1.50

# Executa comando com usuário específico
sc -u deploy -c "systemctl status nginx" webserver

# Executa comando via jump host
sc -s -c "cat /var/log/app.log" production-app
```

### Execução de Comando em Múltiplos Hosts
```bash
# Executa comando em múltiplos hosts do config
sc -c "uptime" -l web1 web2 web3

# Executa comando em múltiplos IPs
sc -c "free -h" -l 192.168.1.10 192.168.1.11 192.168.1.12

# Combina hosts do config e IPs diretos
sc -c "hostname" -l web1 192.168.1.50 ubuntu@192.168.1.51

# Executa em múltiplos hosts via jump host
sc -s -c "df -h" -l db1 db2 db3

# Com usuário específico
sc -u admin -c "systemctl status nginx" -l web1 web2 web3
```

## Configuração

A aplicação utiliza um arquivo de configuração YAML localizado em `~/.sshControl/config.yaml`. Na primeira execução, este arquivo é criado automaticamente com um template.

Estrutura da configuração:
- `config.default_user`: Usuário SSH padrão a ser usado
- `config.users[]`: Lista de usuários com suas chaves SSH
- `config.jump_hosts`: Endereço do jump host para conexões proxadas
- `hosts[]`: Lista de hosts SSH com nome, endereço do host e porta

## Arquitetura

### Estrutura de Pacotes

**main.go**: Ponto de entrada que gerencia flags CLI e roteamento:
- `-u <username>`: Especifica qual usuário do config usar
- `-s`: Habilita modo de conexão via jump host
- `-c "<comando>"`: Executa comando remoto (requer especificar host)
- `-l`: Executa comando em múltiplos hosts (requer `-c`)
- Modo direto: `sc [flags] <host>` conecta imediatamente
- Modo múltiplos hosts: `sc -c "comando" -l <host1> <host2> ...` executa comando em paralelo
- Modo interativo: `sc [flags]` exibe menu TUI

**Pacote config/**: Gerenciamento de configuração
- `config.go`: Carregamento de config YAML, busca de usuário/host e funções auxiliares para expansão de chaves SSH
- `init.go`: Inicializa automaticamente o diretório `~/.sshControl/` e cria template de config padrão na primeira execução

**Pacote cmd/**: Lógica de conexão e UI
- `ssh.go`: Implementação central da conexão SSH (struct `SSHConnection`)
  - Gerencia autenticação com fallback automático: chave SSH → SSH agent → senha interativa
  - Implementa conexões proxy via jump host
  - Gerencia sessões PTY interativas com suporte a redimensionamento de terminal
  - Métodos `Connect()` para sessão interativa e `ExecuteCommand()` para execução de comandos remotos
- `direct.go`: Analisa strings de conexão direta (suporta formatos como `user@host:port`, `host`, etc.)
- `menu.go`: Implementação TUI com Bubble Tea para seleção interativa de hosts com filtragem
- `multiple.go`: Gerencia execução paralela de comandos em múltiplos hosts
  - Usa goroutines e sync.WaitGroup para execução concorrente
  - Coleta e formata resultados de forma organizada com indicadores de sucesso/falha

### Fluxo de Conexão

1. **Carregamento de Config**: `InitializeConfigDir()` garante que `~/.sshControl/config.yaml` existe
2. **Resolução de Usuário**: Prioridade é flag `-u` > `default_user` > primeiro usuário da lista
3. **Resolução de Host**: Busca por nome no config ou analisa string de conexão direta
4. **Conexão SSH**: `SSHConnection.Connect()` gerencia:
   - Criação de config SSH com métodos de autenticação (chave → agent → senha)
   - Dial direto ou proxy via jump host através de `dial()`
   - Sessão PTY interativa com tratamento adequado de terminal

### Padrões de Design Principais

- **Precedência de Usuário**: O método `GetEffectiveUser()` implementa seleção cascata de usuário
- **Padrão Jump Host**: Quando habilitado, cria duas conexões SSH - primeira ao jump host, depois disca o alvo através dele
- **Análise de Conexão Direta**: Parser baseado em regex em `direct.go` lida com formatos flexíveis de string de conexão
- **Gerenciamento de Terminal**: Salva/restaura adequadamente o modo raw do terminal e monitora SIGWINCH para eventos de redimensionamento

### Dependências

- `github.com/charmbracelet/bubbletea`: Framework TUI para menu interativo
- `github.com/charmbracelet/bubbles`: Componentes de UI (list, textinput)
- `github.com/charmbracelet/lipgloss`: Estilização de terminal
- `golang.org/x/crypto/ssh`: Implementação do protocolo SSH
- `golang.org/x/term`: Controle de terminal para modo raw e PTY
- `gopkg.in/yaml.v3`: Parse de configuração YAML

## Notas de Desenvolvimento

- Comentários e nomes de variáveis estão em português
- Não existem arquivos de teste atualmente no codebase
- Verificação de host key SSH usa `InsecureIgnoreHostKey()` - não adequado para produção sem verificação adequada de host key
- Implementação do SSH Agent em `ssh.go:238` está com stub e retorna nil

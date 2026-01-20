# CLAUDE.md

Este arquivo fornece orientações ao Claude Code (claude.ai/code) ao trabalhar com código neste repositório.

## Visão Geral do Projeto

sshControl (`sc`) é um gerenciador de conexões SSH escrito em Go que oferece modos interativo (TUI) e CLI direto para gerenciar conexões SSH. Utiliza o framework Bubble Tea da Charm para a interface interativa e suporta conexões através de jump hosts.

## Comandos de Build

```bash
# Compila binários para Linux (amd64) e macOS (arm64)
# Injeta automaticamente informações de versão usando git
make build

# Binários gerados em:
# - bin/amd64/sc (Linux)
# - bin/arm64/sc (macOS)

# Build com versão customizada
VERSION=v1.0.0 make build

# Execução direta durante desenvolvimento (sem injeção de versão)
go run .

```

O Makefile injeta automaticamente as seguintes informações durante o build:
- **version**: Obtida via `git describe --tags --always --dirty` (ou "dev" se não houver git)
- **buildDate**: Data e hora do build em formato UTC
- **gitCommit**: Hash curto do commit atual

## Exemplos de Uso

### Informações do Sistema
```bash
# Exibe a versão do sshControl
sc -v
sc --version
```

### Listagem de Jump Hosts e Servidores
```bash
# Lista todos os jump hosts e servidores cadastrados no config.yaml
# Jump hosts são exibidos com seus índices para facilitar o uso com -j
sc -s
```

### Modo Interativo (TUI)
```bash
# Abre menu interativo para selecionar host
sc

# Menu interativo usando usuário específico
sc -u ubuntu
```

### Modo Direto (Sessão Interativa)
```bash
# Conecta a host do config.yaml
sc webserver

# Conecta diretamente a IP
sc 192.168.1.50

# Conecta com usuário e porta específicos
sc ubuntu@192.168.1.50:2222

# Conecta via jump host (por nome)
sc -j production-jump webserver

# Conecta via jump host (por índice)
sc -j 1 webserver
```

### Execução de Comando Remoto (Host Único)
```bash
# Executa comando em host do config.yaml
sc -c "uptime" webserver

# Executa comando em IP direto
sc -c "df -h" 192.168.1.50

# Executa comando com usuário específico
sc -u deploy -c "systemctl status nginx" webserver

# Executa comando via jump host (por nome)
sc -j production-jump -c "cat /var/log/app.log" production-app

# Executa comando via jump host (por índice)
sc -j 1 -c "uptime" webserver
```

### Execução de Comando em Múltiplos Hosts
```bash
# Executa comando em múltiplos hosts do config
sc -c "uptime" -l web1 web2 web3

# Executa comando em múltiplos IPs
sc -c "free -h" -l 192.168.1.10 192.168.1.11 192.168.1.12

# Combina hosts do config e IPs diretos
sc -c "hostname" -l web1 192.168.1.50 ubuntu@192.168.1.51

# Executa em múltiplos hosts via jump host (por nome)
sc -j production-jump -c "df -h" -l db1 db2 db3

# Executa em múltiplos hosts via jump host (por índice)
sc -j 1 -c "df -h" -l db1 db2 db3

# Com usuário específico
sc -u admin -c "systemctl status nginx" -l web1 web2 web3
```

## Configuração

A aplicação utiliza um arquivo de configuração YAML localizado em `~/.sshControl/config.yaml`. Na primeira execução, este arquivo é criado automaticamente com um template.

Estrutura da configuração:
- `config.default_user`: Usuário SSH padrão a ser usado
- `config.users[]`: Lista de usuários com suas chaves SSH
- `config.jump_hosts[]`: Lista de jump hosts configurados, cada um com:
  - `name`: Nome identificador do jump host
  - `host`: Endereço do jump host
  - `user`: Usuário para autenticação no jump host
  - `port`: Porta SSH do jump host
- `hosts[]`: Lista de hosts SSH com nome, endereço do host e porta

### Exemplo de Configuração

```yaml
config:
  default_user: ubuntu
  users:
    - name: ubuntu
      ssh_keys:
        - ~/.ssh/id_rsa
        - ~/.ssh/id_ed25519
    - name: devops
      ssh_keys:
        - ~/.ssh/id_rsa
  jump_hosts:
    - name: production-jump
      host: jump.production.example.com
      user: ubuntu
      port: 22
    - name: staging-jump
      host: jump.staging.example.com
      user: ubuntu
      port: 22

hosts:
  - name: dns
    host: 192.168.1.31
    port: 22
  - name: traefik
    host: 192.168.1.32
    port: 22
```

## Arquitetura

### Estrutura de Pacotes

**main.go**: Ponto de entrada que gerencia flags CLI e roteamento usando Cobra:
- `-v, --version`: Exibe a versão do sshControl, data de build e commit hash
- `-u, --user <username>`: Especifica qual usuário do config usar
- `-j, --jump <jump_host>`: Especifica qual jump host usar (nome ou índice, ex: production-jump ou 1)
- `-c, --command "<comando>"`: Executa comando remoto (requer especificar host)
- `-l, --list`: Executa comando em múltiplos hosts (requer `-c`)
- `-s, --servers`: Lista todos os servidores cadastrados no config.yaml
- `-h, --help`: Exibe ajuda com exemplos de uso
- Modo direto: `sc [flags] <host>` conecta imediatamente
- Modo múltiplos hosts: `sc -c "comando" -l <host1> <host2> ...` executa comando em paralelo
- Modo interativo: `sc [flags]` exibe menu TUI

**Pacote config/**: Gerenciamento de configuração
- `config.go`: Carregamento de config YAML, busca de usuário/host e funções auxiliares para expansão de chaves SSH. Inclui funções para resolver jump hosts por nome (`FindJumpHost`) ou índice (`GetJumpHostByIndex`) e função consolidada `ResolveJumpHost` que aceita ambos
- `init.go`: Inicializa automaticamente o diretório `~/.sshControl/` e cria template de config padrão na primeira execução com exemplos de jump hosts

**Pacote cmd/**: Lógica de conexão e UI
- `ssh.go`: Implementação central da conexão SSH (struct `SSHConnection`)
  - Gerencia autenticação com fallback automático: chave SSH → SSH agent → senha interativa
  - Implementa conexões proxy via jump host
  - Gerencia sessões PTY interativas com suporte a redimensionamento de terminal
  - Métodos `Connect()` para sessão interativa e `ExecuteCommand()` para execução de comandos remotos
- `direct.go`: Analisa strings de conexão direta (suporta formatos como `user@host:port`, `host`, etc.) e implementa função `ListServers()` para exibir servidores cadastrados
- `menu.go`: Implementação TUI com Bubble Tea para seleção interativa de hosts com filtragem
- `multiple.go`: Gerencia execução paralela de comandos em múltiplos hosts
  - Usa goroutines e sync.WaitGroup para execução concorrente
  - Coleta e formata resultados de forma organizada com indicadores de sucesso/falha

### Fluxo de Conexão

1. **Carregamento de Config**: `InitializeConfigDir()` garante que `~/.sshControl/config.yaml` existe
2. **Resolução de Usuário**: Prioridade é flag `-u` > `default_user` > primeiro usuário da lista
3. **Resolução de Jump Host**: Se especificado via `-j`, usa `ResolveJumpHost()` que aceita nome (ex: "production-jump") ou índice numérico (ex: "1")
4. **Resolução de Host**: Busca por nome no config ou analisa string de conexão direta
5. **Conexão SSH**: `SSHConnection.Connect()` gerencia:
   - Criação de config SSH com métodos de autenticação (chave → agent → senha)
   - Dial direto ou proxy via jump host através de `dial()`
   - Sessão PTY interativa com tratamento adequado de terminal

### Padrões de Design Principais

- **Precedência de Usuário**: O método `GetEffectiveUser()` implementa seleção cascata de usuário
- **Seleção de Jump Host**: Suporta múltiplos jump hosts configurados, selecionáveis por nome ou índice numérico (1-based). A função `ResolveJumpHost()` tenta primeiro parsear como número, depois busca por nome
- **Padrão Jump Host**: Quando configurado, cria duas conexões SSH - primeira ao jump host especificado (usando suas credenciais), depois disca o alvo através dele
- **Análise de Conexão Direta**: Parser baseado em regex em `direct.go` lida com formatos flexíveis de string de conexão
- **Gerenciamento de Terminal**: Salva/restaura adequadamente o modo raw do terminal e monitora SIGWINCH para eventos de redimensionamento
- **Execução Paralela**: Em modo múltiplos hosts com `-l`, executa comandos simultaneamente em goroutines e exibe tempo total de execução no resumo final

### Dependências

- `github.com/spf13/cobra`: Framework CLI para gerenciamento de comandos e flags com help automático
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
